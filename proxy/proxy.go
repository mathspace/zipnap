// Package proxy defines an interface for managing a proxy service.
package proxy

import (
	"context"
)

type Callbacks struct {
	// Healthy is a function type that is used to check the combined health of
	// the host and the service. If wait is true, it will block until it returns
	// either ready==true or context is cancelled. If wakeup is true, it will
	// wake up the host if it is not ready.
	Healthy func(ctx context.Context, wait bool, wakeup bool) (ready bool, hostName string, err error)
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
	// HealthCheck performs a health check on the service. It returns true if
	// the service is healthy, false otherwise. If an error occurs during the
	// health check, it returns false and the error.
	HealthCheck(ctx context.Context, hostName string) (healthy bool, err error)
}
