// Package instance provides an interface for managing the lifecycle of an
// instance.
package instance

import "context"

// Status represents the status of an instance.
type Status string

const (
	StatusStarting Status = "starting"
	StatusStarted  Status = "started"
	StatusStopping Status = "stopping"
	StatusStopped  Status = "stopped"
	StatusUnknown  Status = "unknown"
)

// Instance is an interface that defines the methods required to manage an
// instance's lifecycle. It includes methods to start and stop the instance, as
// well as a method to retrieve its current status. Implementations of this
// interface should handle the specifics of starting and stopping the instance,
type Instance interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Status(ctx context.Context) (Status, error)
}
