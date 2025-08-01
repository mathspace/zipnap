// Package tcpproxy provides a simple TCP proxy activator.
package tcpproxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/mathspace/zipnap/activator"
)

// TCPProxy is a TCP proxy activator that upon receiving a request, wakes up
// the host and proxies the request to it.
type TCPProxy struct {
	cfg    *Config
	logger *log.Logger
	cb     activator.Callbacks
}

func New(cfg *Config, logger *log.Logger) *TCPProxy {
	return &TCPProxy{
		cfg:    cfg,
		logger: logger,
	}
}

func (p *TCPProxy) RegisterCallbacks(cb activator.Callbacks) {
	p.cb = cb
}

func (p *TCPProxy) handleConnection(ctx context.Context, clientConn *net.TCPConn) {
	// Aggressive keep-alive that lets us detect dead connections early and
	// let the host go back to sleep.
	clientConn.SetKeepAliveConfig(net.KeepAliveConfig{
		Enable:   true,
		Idle:     10 * time.Second,
		Interval: 3 * time.Second,
		Count:    3,
	})

	unlock, err := p.cb.WakeLock(ctx, true)
	if err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			p.logger.Printf("failed to acquire wake lock: %v", err)
		}
		return
	}
	defer unlock()

	state := p.cb.State()

	hostAddr := net.JoinHostPort(state.Addr, fmt.Sprintf("%d", p.cfg.HostPort))
	hostConn, err := net.DialTimeout("tcp", hostAddr, 10*time.Second)
	if err != nil {
		p.logger.Printf("failed to connect to host: %v", err)
		return
	}
	defer hostConn.Close()

	go func() {
		<-ctx.Done()
		go clientConn.Close()
		go hostConn.Close()
	}()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, err := io.Copy(hostConn, clientConn)
		if err != nil && ctx.Err() == nil {
			p.logger.Printf("error copying client to host: %v", err)
		}
		hostConn.Close()
		clientConn.Close()
	}()

	go func() {
		defer wg.Done()
		_, err := io.Copy(clientConn, hostConn)
		if err != nil && ctx.Err() == nil {
			p.logger.Printf("error copying host to client: %v", err)
		}
		hostConn.Close()
		clientConn.Close()
	}()

	wg.Wait()
}

// Run starts the TCP proxy server, listening for incoming connections on the
// configured address and port. It accepts connections, wakes the host, and
// proxies the data between the client and the host.
func (p *TCPProxy) Run(ctx context.Context) error {
	listenAddr := fmt.Sprintf("%s:%d", p.cfg.ListenAddr, p.cfg.ListenPort)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", listenAddr, err)
	}

	go func() {
		p.logger.Printf("TCP proxy listening on %s", listenAddr)

		for {
			conn, err := listener.Accept()
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				p.logger.Printf("failed to accept connection: %v", err)
				continue
			}
			go p.handleConnection(ctx, conn.(*net.TCPConn))
		}
	}()

	// We don't implement a graceful shutdown for the TCP proxy because TCP
	// connections are typically long-lived.

	<-ctx.Done()
	return ctx.Err()
}
