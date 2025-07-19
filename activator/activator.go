// Package activator provides an interface for an activator.
package activator

import (
	"context"
)

type Callbacks struct {
	// WakeLock attempts to acquire a wake lock. If a lock cannot be acquired,
	// it will return nil. Otherwise, it will return a release function that
	// must be called to release the lock. The host will not necessarily sleep
	// immediately after the lock is released. The host is not guaranteed to be
	// awake the whole time the lock is held.
	WakeLock func() (release func())
	// WakeLockContext acquires a wake lock. It will block until the host is
	// awake. If the host is not awake, it is awoken. While the lock is held,
	// the host is not allowed go to sleep (it is not guaranteed that host will
	// be available in the entire time the lock is held). The returned function
	// must be called to release the lock. The host will not necessarily sleep
	// immediately after the lock is released.
	WakeLockContext func(ctx context.Context) (release func(), err error)
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
