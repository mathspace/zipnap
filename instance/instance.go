package instance

// State represents the state of an instance.
type State struct {
	// Addr is the address of the instance, if available.
	Addr string
	// Healthy indicates if the instance is awake and healthy.
	Healthy bool
}

var UnhealthyState = State{}
