// Package proxy defines an interface for managing a proxy service.
package proxy

import "context"

// Proxy is an interface that defines the methods required to manage a
// proxy service.
type Proxy interface {

	// Start the proxy service, which should include any necessary setup and
	// configuration. Start must not block.
	//
	// waitHealthy is a function that the proxy can call to wait for the service
	// to be ready to accept connections.
	//
	// If waitReady returns with nil error, the service is considered up and
	// ready to accept connections.
	Start(waitHealthy func(ctx context.Context) error) error

	// IsServiceHealthy checks if the service is healthy and ready to
	// accept connections.
	IsServiceHealthy(ctx context.Context, host string) (bool, error)

	// Kill instructs the proxy to stop and clean up any resources it has
	// allocated.Kill must block until the proxy is completely stopped.
	Kill(ctx context.Context) error
}
