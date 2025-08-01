// Package httpproxy implements a HTTP proxy activator that wakes up a host and
// proxies requests to it. It supports health checks and can serve a waiting
// page when the host is not healthy or not awake.
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
)

var (
	//go:embed waiting.html
	waitingPageBytes []byte
	waitingPageTpl   = template.Must(template.New("").Parse(string(waitingPageBytes)))
)

// HTTPProxy is a HTTP proxy activator that upon receiving a request, wakes up
// the host and proxies the request to it.
type HTTPProxy struct {
	cfg    *Config
	name   string
	logger *log.Logger
	cb     activator.Callbacks

	healthy     atomic.Bool // Indicates if the host is healthy.
	healthyCond *sync.Cond  // Condition variable to wait for host health.
}

func New(cfg *Config, name string, logger *log.Logger) *HTTPProxy {
	return &HTTPProxy{
		cfg:    cfg,
		name:   name,
		logger: logger,

		healthyCond: &sync.Cond{L: &sync.Mutex{}},
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
func (p *HTTPProxy) pingHealth(ctx context.Context, hostName string) (healthy bool, err error) {
	u := fmt.Sprintf("http://%s:%d%s", hostName, p.cfg.HostPort, p.cfg.HealthCheck.Path)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	resp.Body.Close()
	return slices.Contains(p.cfg.HealthCheck.StatusCodes, resp.StatusCode), nil
}

// runHealthChecker starts a goroutine that periodically checks the health of
// the host by pinging the health check endpoint. It waits for the host to wake up
// before performing the health check.
func (p *HTTPProxy) runHealthChecker(ctx context.Context) {
	for ctx.Err() == nil {
		toCtx, cancel := context.WithTimeout(ctx, p.cfg.HealthCheck.Interval.Duration)
		st := p.cb.State()
		if st.Healthy {
			healthy, err := p.pingHealth(toCtx, st.Addr)
			if err != nil {
				p.healthy.Store(false)
			} else {
				p.healthy.Store(healthy)
				if healthy {
					p.healthyCond.Broadcast()
				}
			}
		} else {
			p.healthy.Store(false)
		}
		// Kill time until we reach the end of the interval.
		<-toCtx.Done()
		cancel()
	}
}

// waitAndRunOnHealthy waits until the host is healthy and then runs the
// provided function. It blocks until the host is healthy or the context is
// done (no other errors are returned).
func (p *HTTPProxy) waitHealthy(ctx context.Context) error {
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
		p.healthyCond.Wait()
	}
	return nil
}

// serveWaitingPage serves a waiting page to the client if the page cannot be
// served in the timely manner due to the host not being healthy.
func (p *HTTPProxy) serveWaitingPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	waitingPageTpl.Execute(w, map[string]any{
		"Name": p.name,
	})
}

// handleHTTP is the HTTP handler that processes incoming requests.
func (p *HTTPProxy) handleHTTP(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()

	// Wake up the host, keep it awake and wait for it to become healthy.

	showWaitingPage := errors.New("")
	waitCtx, cancel := context.WithTimeoutCause(ctx, p.cfg.ShowWaitingPageAfter.Duration, showWaitingPage)
	unlock, err := func() (func(), error) {
		unlock, err := p.cb.WakeLock(waitCtx, true)
		if err != nil {
			return nil, err
		}
		return unlock, p.waitHealthy(waitCtx)
	}()
	if unlock != nil {
		defer unlock()
	}
	cancel()
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			if context.Cause(waitCtx) == showWaitingPage {
				// Waiting page will open a SSE connection to wait for
				// host to become healthy.
				p.serveWaitingPage(w)
			}
			return
		}
		p.logger.Printf("failed to wake host: %v", err)
		http.Error(w, "failed to wake host", http.StatusInternalServerError)
		return
	}

	// Proxy the request.

	u := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s:%d", p.cb.State().Addr, p.cfg.HostPort),
	}
	httputil.NewSingleHostReverseProxy(u).ServeHTTP(w, r)
}

// waitReadySSEHandler handles the /_zipnap/waitready endpoint for server-sent
// events (SSE) to notify clients when the host is ready. It sends a
// "ready" event when the host becomes healthy.
func (p *HTTPProxy) waitReadySSEHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()
	unlock, err := p.cb.WakeLock(ctx, true)
	if err != nil {
		return
	}
	defer unlock()
	if err := p.waitHealthy(ctx); err != nil {
		return
	}

	fmt.Fprintf(w, "event: ready\ndata: Host is ready\n\n")
}

// Run starts the HTTP proxy server and listens for incoming requests. It also
// starts a health checker that periodically checks the health of the host. The
// server will run until the context is done or an error occurs.
func (p *HTTPProxy) Run(ctx context.Context) error {

	mux := http.NewServeMux()
	mux.HandleFunc("GET /_zipnap/waitready", p.waitReadySSEHandler)
	mux.HandleFunc("/", p.handleHTTP)

	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", p.cfg.ListenAddr, p.cfg.ListenPort),
		Handler: mux,
	}

	ctx, cancel := context.WithCancelCause(ctx)

	go func() {
		p.logger.Print("starting ...")
		cancel(server.ListenAndServe())
	}()

	go p.runHealthChecker(ctx)

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
