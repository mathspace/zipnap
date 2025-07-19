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
	"sync"
	"sync/atomic"
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

	healthy       atomic.Bool // Indicates if the host is healthy.
	healthyCond   *sync.Cond  // Condition variable to wait for host health.
	healthCheckCh chan struct{}
}

func New(cfg config.Activator, logger *log.Logger) *HTTPProxy {
	return &HTTPProxy{
		cfg:    cfg,
		logger: logger,

		healthyCond:   &sync.Cond{L: &sync.Mutex{}},
		healthCheckCh: make(chan struct{}, 1),
	}
}

func (p *HTTPProxy) RegisterCallbacks(cb activator.Callbacks) {
	p.cb = cb
}

// pingHealth checks if the host is healthy by performing a health check
// request to the configured health check endpoint. It returns true if the
// host is healthy, false if it is not, and an error if the health check
// request fails or context is done.
//
// A wake lock must be held before calling this function, as it will
// perform a network request to the host.
func (p *HTTPProxy) pingHealth(ctx context.Context) (healthy bool, err error) {
	if p.cfg.HTTPProxy.HealthCheck == nil {
		return true, nil // No health check configured, assume healthy if host is up.
	}
	u := fmt.Sprintf("http://%s:%d%s", p.cb.HostName(), p.cfg.HTTPProxy.HostPort, p.cfg.HTTPProxy.HealthCheck.Path)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	resp.Body.Close()
	return slices.Contains(p.cfg.HTTPProxy.HealthCheck.StatusCodes, resp.StatusCode), nil
}

// runHealthChecker periodically checks host health and triggers wake/hold as
// needed.
func (p *HTTPProxy) runHealthChecker(ctx context.Context) {

	hc := func(ctx context.Context, wake bool) {
		var release func()
		var err error
		wakeCtx := ctx
		if !wake {
			// If not waking, use cancelled context to avoid waiting.
			var cancel context.CancelFunc
			wakeCtx, cancel = context.WithCancel(wakeCtx)
			cancel()
		}
		release, err = p.cb.AcquireWakeLock(wakeCtx)
		if err != nil {
			p.healthy.Store(false)
			return
		}
		defer release()

		healthy, err := p.pingHealth(ctx)
		if err != nil {
			p.healthy.Store(false)
			return
		}
		p.healthy.Store(healthy)
		if healthy {
			p.healthyCond.Broadcast()
		}
	}

	ticker := time.NewTicker(p.cfg.HTTPProxy.HealthCheck.Interval.Duration)

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.healthCheckCh:
			p.logger.Printf("performing immediate health check for %s", p.cfg.ID)
			hc(ctx, true)
		case <-ticker.C:
			p.logger.Printf("performing periodic health check for %s", p.cfg.ID)
			ctx, cancel := context.WithTimeout(ctx, p.cfg.HTTPProxy.HealthCheck.Interval.Duration-time.Second)
			hc(ctx, false)
			cancel()
		}
	}
}

// waitAndRunOnHealthy waits until the host is healthy and then runs the
// provided function. It blocks until the host is healthy or the context is
// done. If the context is done before the host is healthy, it returns the
// context error. The host is kept awake during the wait and while fn is
// running.
func (p *HTTPProxy) waitAndRunOnHealthy(ctx context.Context, fn func() error) error {
	// Hold the healthyCond lock before registering AfterFunc to prevent
	// missing signals if the context is cancelled between checking healthy
	// status and waiting on the condition variable indefinitely.
	p.healthyCond.L.Lock()
	defer p.healthyCond.L.Unlock()

	stop := context.AfterFunc(ctx, func() {
		p.healthyCond.L.Lock()
		defer p.healthyCond.L.Unlock()
		p.healthyCond.Broadcast()
	})
	defer stop()

	for !p.healthy.Load() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		select {
		case p.healthCheckCh <- struct{}{}:
		default:
		}
		p.healthyCond.Wait()
	}
	return fn()
}

// serveWaitingPage serves a waiting page to the client. It is used when the
// host is not healthy or when the host is not awake and the ShowWaitingPage
// configuration is enabled.
func (p *HTTPProxy) serveWaitingPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	waitingPageTpl.Execute(w, map[string]any{
		"Name": p.cfg.ID,
	})
}

// handleHTTP is the HTTP handler that processes incoming requests.
func (p *HTTPProxy) handleHTTP(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()

	//

	showWaitingPage := errors.New("")
	lockCtx, cancel := context.WithTimeoutCause(ctx, p.cfg.HTTPProxy.ShowWaitingPageAfter.Duration, showWaitingPage)
	unlock, hostName, err := p.cb.WakeLock(lockCtx, true)
	cancel()
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			if context.Cause(lockCtx) == showWaitingPage {
				p.serveWaitingPage(w)
			}
			return
		}
		p.logger.Printf("failed to wake host %s: %v", p.cfg.ID, err)
		http.Error(w, fmt.Sprintf("failed to wake host %s", p.cfg.ID), http.StatusInternalServerError)
		return
	}

	defer unlock()

	// Get a wake lock on the host.

	var releaseWakeLock func()
	if p.cfg.HTTPProxy.ShowWaitingPage {
		p.triggerWakeAndHold()
		releaseWakeLock = p.cb.TryWakeLock()
		if releaseWakeLock == nil {
			p.serveWaitingPage(w, r)
			return
		}
	} else {
		var err error
		releaseWakeLock, err = p.cb.AcquireWakeLock(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			p.logger.Printf("failed to wake host: %v", err)
			http.Error(w, "failed to wake host", http.StatusInternalServerError)
			return
		}
	}
	defer releaseWakeLock()

	// Wait until host is healthy.

	p.triggerHealthCheck()
	if p.cfg.HTTPProxy.ShowWaitingPage {
		if !p.healthy.Load() {
			p.serveWaitingPage(w)
			return
		}
	} else {
		if err := p.waitHealthy(ctx); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			p.logger.Printf("failed to check host health: %v", err)
			http.Error(w, "failed to check host health", http.StatusInternalServerError)
			return
		}
	}

	// Proxy the request.

	u := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s:%d", p.cb.HostName(), p.cfg.HTTPProxy.HostPort),
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
