// Package activator provides an interface for an activator.
package activator

import (
	"context"
)

type Callbacks struct {
	// AcquireWakeLock is called to acquire a wake lock. It will block until the
	// host is awake. If the host is not awake, it was awoken. While the lock is
	// held, the host will not go to sleep. The returned function must be called
	// to release the lock. The host will not necessarily to to sleep
	// immediately after the lock is released.
	//
	// If the lock can be acquired immediately, cancellation of the context will
	// not have any effect. If the context is cancelled, the function will
	// return the context error.
	AcquireWakeLock func(ctx context.Context) (release func(), err error)
	// IsAwake is called to check if the host is awake. It does not block.
	IsAwake func() bool
	// HostName is called to get the name of the host. The value is not
	// guaranteed to be valid if a wake lock is not held. It may return an empty
	// string if the host is not awake or host name is unknown.
	HostName func() string
}

// Activator is an interface for an activator. It is used to wake up the host
// when it is asleep, and to keep it awake while the activator is running.
type Activator interface {
	// RegisterCallbacks registers the callbacks.
	RegisterCallbacks(Callbacks)
	// Run starts the activator and blocks until context is cancelled. It will
	// return nil if it is stopped because of context cancellation and it's
	// gracefully shutdown.
	Run(context.Context) error
}
