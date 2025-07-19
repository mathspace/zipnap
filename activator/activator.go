// Package activator provides an interface for an activator.
package activator

import (
	"context"
)

type Callbacks struct {
	// AcquireWakeLock blocks until it's acquired a wake lock or the context is
	// done. If the host is not awake, it is awoken if wake is true. While the
	// lock is held, the host is not allowed go to sleep (it is not guaranteed
	// that host will be available in the entire time the lock is held). The
	// returned function must be called to unlock the lock. The host will not
	// necessarily sleep immediately after the lock is unlocked. If host is
	// awake at the time of the call, it will return immediately without
	// blocking or returning an error even if the context is done.
	WakeLock func(ctx context.Context, wake bool) (unlock func(), hostName string, err error)
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
