// Package httpproxy provides a HTTP proxy implementation for a service.
package httpproxy

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/mathspace/zipnap/config"
)

type HTTPProxy struct {
	cfg config.HTTP
}

func New(c config.HTTP) *HTTPProxy {
	return &HTTPProxy{
		cfg: c,
	}
}

func (p *HTTPProxy) Run(ctx context.Context, waitHealthy func(ctx context.Context) error) error {
	handler := func(w http.ResponseWriter, r *http.Request) {
	}

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

func (p *HTTPProxy) IsServiceHealthy(ctx context.Context, host string) (bool, error) {
	return false, nil
}
