// Package host provides an interface for managing the lifecycle of a host.
package host

import "context"

// Status represents the status of a host.
type Status string

const (
	StatusStarting Status = "starting"
	StatusStarted  Status = "started"
	StatusStopping Status = "stopping"
	StatusStopped  Status = "stopped"
	StatusUnknown  Status = "unknown"
)

// State represents the current state of a host, including its status and
// hostname. The hostname is typically the public IP address or DNS name of the
// host, which can be used to access the service running on it.
type State struct {
	Status   Status
	HostName string
}

// Host is an interface that defines the methods required to manage a host's
// lifecycle. It includes methods to start and stop the host, as well as a
// method to retrieve its current state. Implementations of this interface
// should handle the specifics of starting and stopping the host,
type Host interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	State(ctx context.Context) (State, error)
}
