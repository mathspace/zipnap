package httpproxy

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"slices"
	"time"

	"github.com/mathspace/zipnap/activator"
	"github.com/mathspace/zipnap/config"
)

const (
	// Duration to hold the host awake after the last wake trigger.
	wakeAndHoldTimeout = 20 * time.Second
)

var (
	//go:embed waiting.html
	waitingPageBytes []byte
	waitingPageTpl   = template.Must(template.New("").Parse(string(waitingPageBytes)))
)

// HTTPProxy is a HTTP proxy activator that upon receiving a request, wakes up
// the host and proxies the request to it.
type HTTPProxy struct {
	cfg    config.Activator
	logger *log.Logger
	cb     activator.Callbacks

	healthCheckCh chan struct{}
	wakeAndHoldCh chan struct{}
}

func New(cfg config.Activator, logger *log.Logger) *HTTPProxy {
	return &HTTPProxy{
		cfg:    cfg,
		logger: logger,

		healthCheckCh: make(chan struct{}, 1),
		wakeAndHoldCh: make(chan struct{}, 1),
	}
}

func (p *HTTPProxy) RegisterCallbacks(cb activator.Callbacks) {
	p.cb = cb
}

func (p *HTTPProxy) triggerHealthCheck(ctx context.Context) error {

}

func (p *HTTPProxy) runHealthChecker(ctx context.Context) {

}

// triggerWakeAndHold triggers the wake and hold mechanism. It sends a signal to
// the wake and hold channel, which will wake up the host and hold it awake for
// a short period of time after the last trigger is received.
func (p *HTTPProxy) triggerWakeAndHold() {
	select {
	case p.wakeAndHoldCh <- struct{}{}:
	default:
		// If the channel is full, it means we are already waiting to wake and hold.
	}
}

// runWakeAndHold starts a goroutine that will wake up the host and hold it
// awake for a short period of time after the last trigger is received.
func (p *HTTPProxy) runWakeAndHold(ctx context.Context) {
	timer := time.NewTimer(0)
	timer.Stop()
	var release func()

	for {
		select {

		case <-ctx.Done():
			timer.Stop()
			if release != nil {
				release()
			}
			return

		case <-p.wakeAndHoldCh:
			if release == nil {
				var err error
				release, err = p.cb.AcquireWakeLock(ctx)
				if err != nil {
					continue
				}
			}
			timer.Reset(wakeAndHoldTimeout)

		case <-timer.C:
			if release != nil {
				release()
				release = nil
			}

		}
	}
}

func (p *HTTPProxy) HealthCheck(ctx context.Context, hostName string) (healthy bool, err error) {
	if p.cfg.HTTPProxy.HealthCheck == nil {
		return true, nil // No health check configured, assume healthy.
	}
	u := fmt.Sprintf("http://%s:%d%s", hostName, p.cfg.HTTPProxy.HostPort, p.cfg.HTTPProxy.HealthCheck.Path)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	resp.Body.Close()
	return slices.Contains(p.cfg.HTTPProxy.HealthCheck.StatusCodes, resp.StatusCode), nil
}

// handleHTTP is the HTTP handler that processes incoming requests.
func (p *HTTPProxy) handleHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancelCause(r.Context())
	if p.cfg.HTTPProxy.ShowWaitingPage {
		// If we are showing the waiting page, we don't want to block waiting
		// for the host and health check to complete.
		cancel()
	}
	release, err := p.cb.AcquireWakeLock(ctx)

	healthy, hostName, err := p.cb.Healthy(ctx, !p.cfg.HTTPProxy.ShowWaitingPage, true)
	if err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		return
	}

	// Show waiting page if health check errored, or if the service is not
	// healthy and the waiting page is enabled in the configuration.

	if err != nil || (!healthy && p.cfg.HTTPProxy.ShowWaitingPage) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		waitingPageTpl.Execute(w, map[string]any{
			"Name": p.cfg.ID,
		})
		return
	}

	// Proxy the request to the host instance otherwise.

	u := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s:%d", hostName, p.cfg.HTTPProxy.HostPort),
	}
	httputil.NewSingleHostReverseProxy(u).ServeHTTP(w, r)
}

// RunProxy starts the HTTP proxy server and blocks until the context is
// cancelled or an error occurs.
func (p *HTTPProxy) Run(ctx context.Context) error {

	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", p.cfg.HTTPProxy.ProxyHost, p.cfg.HTTPProxy.ProxyPort),
		Handler: http.HandlerFunc(p.handleHTTP),
	}

	ctx, cancel := context.WithCancelCause(ctx)

	go func() {
		p.logger.Print("starting ...")
		cancel(server.ListenAndServe())
	}()

	<-ctx.Done()
	cause := context.Cause(ctx)
	if ctx.Err() != cause {
		// If we are here, it means cancel() was called above *before* we
		// started shutting down the server. This indicates an error
		// condition, so we return the error from the context.
		return cause
	}

	p.logger.Print("shutting down ...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("failed to shutdown server gracefully: %w", err)
	}
	return cause
}
