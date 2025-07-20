// Package duration provides a custom type for handling durations in YAML.
package duration

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration wraps time.Duration to provide custom JSON marshalling
type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	var s string
	if err := n.Decode(&s); err != nil {
		return err
	}
	duration, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration format: %v", err)
	}
	d.Duration = duration
	return nil
}
