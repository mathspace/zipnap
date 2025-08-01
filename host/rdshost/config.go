package rdshost

import "fmt"

// Config represents the configuration for an RDS instance.
type Config struct {
	InstanceID string `yaml:"instance_id"`
}

func (c Config) Validate() error {
	if c.InstanceID == "" {
		return fmt.Errorf("instance_id must be specified")
	}
	return nil
}
