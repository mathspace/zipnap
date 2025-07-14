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
	cfg    config.Service
	logger *log.Logger
}

func New(cfg config.Service, logger *log.Logger) *HTTPProxy {
	return &HTTPProxy{
		cfg:    cfg,
		logger: logger,
	}
}

func (p *HTTPProxy) Run(ctx context.Context, cb proxy.Callbacks) error {

	// Create a handler that will wait for the service to be healthy and then
	// proxy the request to the EC2 instance.

	handler := func(w http.ResponseWriter, r *http.Request) {
		cb.ConnDelta(1)
		defer cb.ConnDelta(-1)
		ctx := r.Context()

		ready, hostName, err := cb.Ready(ctx, !p.cfg.HTTP.ShowWaitingPage)
		if err != nil {
			http.Error(w, fmt.Sprintf("error waiting for service to be healthy: %v", err), http.StatusInternalServerError)
			return
		}

		// Show waiting page if service not ready.

		if !ready && p.cfg.HTTP.ShowWaitingPage {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			waitingPageTpl.Execute(w, map[string]any{
				"Name": p.cfg.Name,
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

	// Setup the HTTP server with the handler.

	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", p.cfg.HTTP.ProxyHost, p.cfg.HTTP.ProxyPort),
		Handler: http.HandlerFunc(handler),
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
