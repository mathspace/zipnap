// Package instance provides a representation of an instance's state.
package instance

// State represents the state of an instance.
type State struct {
	// Addr is the address of the instance, if available.
	Addr string
	// Healthy indicates if the instance is awake and healthy.
	Healthy bool
}

// UnhealthyState is a predefined state indicating that the instance is not
// healthy.
var UnhealthyState = State{}
