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
	"sync"
	"sync/atomic"
	"time"

	"github.com/mathspace/zipnap/config"
	"github.com/mathspace/zipnap/proxy"
)

var (
	//go:embed waiting.html
	waitingPageBytes []byte

	waitingPageTpl = template.Must(template.New("").Parse(string(waitingPageBytes)))
)

type HTTPProxy struct {
	// RunProxy should be callable multiple times on the same instance and thus
	// no state should be kept in here, only config.
	cfg    config.Service
	logger *log.Logger
}

func New(cfg config.Service, logger *log.Logger) *HTTPProxy {
	return &HTTPProxy{
		cfg:    cfg,
		logger: logger,
	}
}

type proxyRun struct {
	p           *HTTPProxy
	cb          proxy.Callbacks
	healthy     atomic.Bool
	healthyCond *sync.Cond
	hostName    atomic.Value
}

func (pr *proxyRun) runHealthcheck(ctx context.Context) {

	hostReadyCh := make(chan struct{}, 1)

	go func() {
		for {
			_, hostName := pr.cb.HostReady(ctx, true)
			pr.hostName.Store(hostName)
			select {
			case hostReadyCh <- struct{}{}:
			default:
			}
		}

	}()

	timer := time.NewTicker(pr.p.cfg.HTTP.HealthCheck.Interval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

	}

}

func (pr *proxyRun) handleHTTP(w http.ResponseWriter, r *http.Request) {
	pr.cb.ConnDelta(1)
	defer pr.cb.ConnDelta(-1)
	ctx := r.Context()

	ready, hostName := pr.cb.HostReady(ctx, !pr.p.cfg.HTTP.ShowWaitingPage)

	// Show waiting page if service not ready.

	if !ready && pr.p.cfg.HTTP.ShowWaitingPage {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		waitingPageTpl.Execute(w, map[string]any{
			"Name": pr.p.cfg.Name,
		})
		return
	}

	// Proxy the request to the EC2 instance otherwise.

	u := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s:%d", hostName, pr.p.cfg.HTTP.ServicePort),
	}
	httputil.NewSingleHostReverseProxy(u).ServeHTTP(w, r)
}

func (p *HTTPProxy) RunProxy(ctx context.Context, cb proxy.Callbacks) error {

	run := &proxyRun{
		p:           p,
		cb:          cb,
		healthy:     atomic.Bool{},
		healthyCond: sync.NewCond(&sync.Mutex{}),
	}
	run.hostName.Store("")
	go run.runHealthcheck(ctx)

	// Setup the HTTP server with the handler.

	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", p.cfg.HTTP.ProxyHost, p.cfg.HTTP.ProxyPort),
		Handler: http.HandlerFunc(run.handleHTTP),
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

func (p *HTTPProxy) Ping(ctx context.Context, hostName string) (bool, error) {
	u := fmt.Sprintf("http://%s:%d/", hostName, p.cfg.HTTP.ServicePort)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, nil
	}
	resp.Body.Close()
	return resp.StatusCode < 500, nil
}
