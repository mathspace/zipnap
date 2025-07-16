// Package httpproxy provides a HTTP proxy implementation for a service.
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
	"sync/atomic"
	"time"

	"github.com/oxplot/valuewaiter"

	"github.com/mathspace/zipnap/config"
	"github.com/mathspace/zipnap/proxy"
)

var (
	//go:embed waiting.html
	waitingPageBytes []byte

	waitingPageTpl = template.Must(template.New("").Parse(string(waitingPageBytes)))
)

// HTTPProxy implements the proxy.Proxy interface for HTTP services.
type HTTPProxy struct {
	cfg      config.Service
	logger   *log.Logger
	cb       proxy.Callbacks
	hostName atomic.Value
	healthy  *valuewaiter.ValueWaiter[bool]
}

// New creates a new HTTPProxy instance with the given configuration and logger.
func New(cfg config.Service, logger *log.Logger) *HTTPProxy {
	p := HTTPProxy{
		cfg:     cfg,
		logger:  logger,
		healthy: valuewaiter.New(false),
	}
	p.hostName.Store("")
	return &p
}

// RegisterCallbacks registers the callbacks that will be used to notify the
// proxy service about the host state and connection changes. This is called
// before Run.
func (p *HTTPProxy) RegisterCallbacks(cb proxy.Callbacks) {
	p.cb = cb
}

// ping checks if the HTTP service is healthy by sending a request to the health
// check path and verifying the response status code.
func (p *HTTPProxy) ping(ctx context.Context) bool {
	u := fmt.Sprintf("http://%s:%d%s", p.hostName, p.cfg.HTTP.ServicePort, p.cfg.HTTP.HealthCheck.Path)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return slices.Contains(p.cfg.HTTP.HealthCheck.StatusCodes, resp.StatusCode)
}

// runHealthcheckLoop runs a loop that periodically checks the health of the
// HTTP service by pinging the health check endpoint. It updates the hostName
// and healthy status in the callbacks. If the service is healthy, it broadcasts
// the healthy condition to any waiting goroutines.
func (p *HTTPProxy) runHealthcheckLoop(ctx context.Context) {
	for {
		_, hostName := p.cb.HostReady(ctx, true, false)
		if ctx.Err() != nil {
			return
		}
		p.hostName.Store(hostName)

		if p.cfg.HTTP.HealthCheck == nil {

		}
		healthy := p.ping(ctx)
		p.healthy.Store(healthy)
		if healthy {
			p.healthyCond.Broadcast()
		}

		select {
		case <-ctx.Done():
			return
		case <-time.NewTimer(p.p.cfg.HTTP.HealthCheck.Interval).C:
		}
	}
}

func (p *HTTPProxy) handleHTTP(w http.ResponseWriter, r *http.Request) {
	p.cb.ConnDelta(1)
	defer p.cb.ConnDelta(-1)
	ctx := r.Context()

	ready, hostName := p.cb.WaitHostReady(ctx, !p.p.cfg.HTTP.ShowWaitingPage)

	// Show waiting page if service not ready.

	if !ready && p.cfg.HTTP.ShowWaitingPage {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		waitingPageTpl.Execute(w, map[string]any{
			"Name": p.cfg.ID,
		})
		return
	}

	// Proxy the request to the EC2 instance otherwise.

	u := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s:%d", hostName, p.cfg.HTTP.ServicePort),
	}
	httputil.NewSingleHostReverseProxy(u).ServeHTTP(w, r)
}

func (p *HTTPProxy) RunProxy(ctx context.Context, cb proxy.Callbacks) error {

	go p.runHealthcheckLoop(ctx)

	// Setup the HTTP server with the handler.

	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", p.cfg.HTTP.ProxyHost, p.cfg.HTTP.ProxyPort),
		Handler: http.HandlerFunc(p.handleHTTP),
	}

	stopping := make(chan struct{})
	errCh := make(chan error, 2)

	go func() {
		select {
		case <-ctx.Done():
		// Provided context may never be cancelled but if the server doesn't
		// start successfully, we still want this goroutine to exit.
		case <-stopping:
		}
		p.logger.Print("shutting down ...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		errCh <- server.Shutdown(shutdownCtx)
	}()

	go func() {
		p.logger.Print("starting ...")
		errCh <- server.ListenAndServe()
		close(stopping)
	}()

	for range 2 {
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	p.logger.Print("gracefully shut down")
	return nil
}
