// Package activator provides an interface for an activator.
package activator

import (
	"context"

	"github.com/mathspace/zipnap/instance"
)

type Callbacks struct {
	// HealthyLock blocks until it's acquired a healthy lock or the context is
	// done (no other errors are returned). A healthy lock waits for the
	// instance to wake up and pass health check, and then forces the instance
	// to stay awake until the lock is released. If the instance is not awake,
	// it is awoken if wake is true. It is not guaranteed that instance will be
	// healthy entire time the lock is held. The returned function must be
	// called to unlock the lock. The instance will not necessarily sleep
	// immediately after the lock is unlocked. If instance is awake and healthy
	// at the time of the call, it will return immediately without blocking or
	// returning an error even if the context is done.
	HealthyLock func(ctx context.Context, wake bool) (unlock func(), err error)

	// State returns the current state of the instance.
	State func() instance.State
}

// Activator is an interface for an activator that manages the lifecycle of an
// instance, including waking it up and keeping it awake.
type Activator interface {
	// RegisterCallbacks registers the callbacks.
	RegisterCallbacks(Callbacks)
	// Run starts the activator and blocks until context is cancelled. It will
	// return nil if it is stopped because of context cancellation and it's
	// gracefully shutdown.
	Run(context.Context) error
}
