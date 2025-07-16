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
	"time"

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
	cfg    config.Service
	logger *log.Logger
	cb     proxy.Callbacks
}

// New creates a new HTTPProxy instance with the given configuration and logger.
func New(cfg config.Service, logger *log.Logger) *HTTPProxy {
	return &HTTPProxy{
		cfg:    cfg,
		logger: logger,
	}
}

// RegisterCallbacks registers the callbacks that will be used to notify the
// proxy service about the host state and connection changes. This is called
// before Run.
func (p *HTTPProxy) RegisterCallbacks(cb proxy.Callbacks) {
	p.cb = cb
}

// HealthCheck performs a health check on the service. It returns true if the
// service is healthy, false otherwise. If an error occurs during the health
// check, it returns false and the error. The health check is performed by
// sending a request to the health check path configured in the service.
func (p *HTTPProxy) HealthCheck(ctx context.Context, hostName string) (healthy bool, err error) {
	if p.cfg.HTTP.HealthCheck == nil {
		return true, nil // No health check configured, assume healthy.
	}
	u := fmt.Sprintf("http://%s:%d%s", hostName, p.cfg.HTTP.ServicePort, p.cfg.HTTP.HealthCheck.Path)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	resp.Body.Close()
	return slices.Contains(p.cfg.HTTP.HealthCheck.StatusCodes, resp.StatusCode), nil
}

// handleHTTP is the HTTP handler that processes incoming requests.
func (p *HTTPProxy) handleHTTP(w http.ResponseWriter, r *http.Request) {
	p.cb.ConnDelta(1)
	defer p.cb.ConnDelta(-1)
	ctx := r.Context()

	healthy, hostName, err := p.cb.Healthy(ctx, !p.cfg.HTTP.ShowWaitingPage, true)
	if err != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		return
	}

	// Show waiting page if health check errored, or if the service is not
	// healthy and the waiting page is enabled in the configuration.

	if err != nil || (!healthy && p.cfg.HTTP.ShowWaitingPage) {
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
		Host:   fmt.Sprintf("%s:%d", hostName, p.cfg.HTTP.ServicePort),
	}
	httputil.NewSingleHostReverseProxy(u).ServeHTTP(w, r)
}

// RunProxy starts the HTTP proxy server and blocks until the context is
// cancelled or an error occurs.
func (p *HTTPProxy) RunProxy(ctx context.Context, cb proxy.Callbacks) error {

	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", p.cfg.HTTP.ProxyHost, p.cfg.HTTP.ProxyPort),
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
