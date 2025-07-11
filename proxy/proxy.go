// Package proxy defines an interface for managing a proxy service.
package proxy

import "context"

// Proxy is an interface that defines the methods required to manage a
// proxy service.
type Proxy interface {

	// Starts the proxy service and blocks until context is cancelled. It will
	// return nil if the service is stopped because of context cancellation and
	// it's gracefully shutdown.
	//
	// waitHealthy is a function that the proxy can call to wait for the service
	// to be ready to accept connections.
	//
	// If waitReady returns with nil error, the service is considered up and
	// ready to accept connections.
	Run(ctx context.Context, waitHealthy func(ctx context.Context) error) error

	// IsServiceHealthy checks if the service is healthy and ready to
	// accept connections.
	IsServiceHealthy(ctx context.Context, hostName string) (bool, error)
}
