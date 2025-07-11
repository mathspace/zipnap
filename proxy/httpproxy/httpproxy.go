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
)

var (
	//go:embed waiting.html
	waitingPageBytes []byte

	waitingPageTpl = template.Must(template.New("").Parse(string(waitingPageBytes)))
)

type HTTPProxy struct {
	cfg config.HTTP
	svc config.Service
}

func New(cfg config.HTTP, svc config.Service) *HTTPProxy {
	return &HTTPProxy{
		svc: svc,
		cfg: cfg,
	}
}

func (p *HTTPProxy) Run(ctx context.Context, waitHealthy func(ctx context.Context) (string, error)) error {

	// Create a handler that will wait for the service to be healthy and then
	// proxy the request to the EC2 instance.

	handler := func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		showWaitingPage := errors.New("show waiting page")
		if p.cfg.ShowWaitingPage {
			var cancel func()
			ctx, cancel = context.WithTimeoutCause(ctx, 1*time.Second, showWaitingPage)
			defer cancel()
		}

		hostName, err := waitHealthy(ctx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) && context.Cause(ctx) == showWaitingPage {

				// Show waiting page.

				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusServiceUnavailable)
				waitingPageTpl.Execute(w, map[string]any{
					"Name": p.svc.Name,
				})
				return
			}
			http.Error(w, fmt.Sprintf("error waiting for service to be healthy: %v", err), http.StatusInternalServerError)
			return
		}

		// Proxy the request to the EC2 instance.

		u := &url.URL{
			Scheme: "http",
			Host:   fmt.Sprintf("%s:%d", hostName, p.cfg.ServicePort),
		}
		httputil.NewSingleHostReverseProxy(u).ServeHTTP(w, r)
	}

	// Setup the HTTP server with the handler.

	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", p.cfg.ProxyHost, p.cfg.ProxyPort),
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
		log.Printf("http-proxy(%d->%d): shutting down ...", p.cfg.ProxyPort, p.cfg.ServicePort)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		errCh <- server.Shutdown(shutdownCtx)
	}()

	go func() {
		log.Printf("http-proxy(%d->%d): starting ...", p.cfg.ProxyPort, p.cfg.ServicePort)
		errCh <- server.ListenAndServe()
		close(stopping)
	}()

	for range 2 {
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	log.Printf("http-proxy(%d->%d): gracefully shut down", p.cfg.ProxyPort, p.cfg.ServicePort)
	return nil
}

func (p *HTTPProxy) IsServiceHealthy(ctx context.Context, hostName string) (bool, error) {
	u := fmt.Sprintf("http://%s:%d/", hostName, p.cfg.ServicePort)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, nil
	}
	resp.Body.Close()
	return resp.StatusCode < 500, nil
}
