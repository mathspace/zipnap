// Package activator provides an interface for an activator.
package activator

import (
	"context"
)

// WakeLocker defines a function that acquires a wake lock. It will block until the
// target is awake. If the target is not awake, it is awoken. While the lock is
// held, the target is not allowed go to sleep (it is not guaranteed that target
// will be available in the entire time the lock is held). The returned function
// must be called to release the lock. The target will not necessarily sleep
// immediately after the lock is released.
//
// If the lock can be acquired immediately, cancellation of the context will
// not return an error. It's therefore possible to "attempt" to acquire a
// lock by passing an already cancelled context.
type WakeLocker func(ctx context.Context) (release func(), err error)

type Callbacks struct {
	// AcquireWakeLock acquires a wake lock on the host.
	AcquireWakeLock WakeLocker
	// IsAwake returns true if the host is awake. It does not block.
	IsAwake func() bool
	// HostName returns the host name of the host. It is not guaranteed to be
	// valid if a wake lock is not held. It may return an empty string if the
	// host is not awake or host name is unknown.
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
