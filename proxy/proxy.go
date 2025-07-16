// Package proxy defines an interface for managing a proxy service.
package proxy

import (
	"context"
)

type Callbacks struct {
	// HostReady indicates whether the host is started. If wait is true, it will
	// block until the host is started or the context is cancelled. If wait is
	// false, it will return immediately with the readiness status. hostName is
	// the host name where the service is running.
	HostReady func(ctx context.Context, wait bool, wakeup bool) (ready bool, hostName string)
	// ConnDelta is a function type that is used to signal changes in the number
	// of active connections. It takes an integer delta that indicates the change in
	// the number of connections. A positive delta indicates an increase in
	// connections, while a negative delta indicates a decrease.
	ConnDelta func(delta int)
}

// Proxy is an interface that defines the methods required to manage a
// proxy service.
type Proxy interface {
	// RegisterCallbacks registers the callbacks that will be used to notify
	// the proxy service about the host state and connection changes.
	// This is called before Run.
	RegisterCallbacks(Callbacks)
	// Run starts the proxy service and blocks until context is cancelled. It
	// will return nil if the service is stopped because of context cancellation
	// and it's gracefully shutdown.
	Run(context.Context) error
}
