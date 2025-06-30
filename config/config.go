// Package config defines the configuration structures for the application.
package config

import (
	"encoding/json"
	"fmt"
	"time"
)

// EC2 represents the configuration for an EC2 instance.
type EC2 struct {
	InstanceID string `json:"instance_id"`
	// Shutdown indicates whether the instance should be shut down or
	// hibernated.
	Shutdown bool `json:"hibernate"`
}

// HTTP represents the configuration for an HTTP service. The service is assumed
// to be active when request sent to path / on given port returns a 2xx-3xx
// status code.
type HTTP struct {
	Port int `json:"port"`
}

type ServiceType string

// ServiceType represents the type of service.
const (
	ServiceTypeHTTP ServiceType = "http"
)

// UnmarshalJSON implements the json.Unmarshaler interface for ServiceType
func (st *ServiceType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*st = ServiceType(s)
	return nil
}

// Service represents a service that is to be proxied.
type Service struct {
	Name string      `json:"name"`
	Type ServiceType `json:"type"`
	HTTP *HTTP       `json:"http,omitempty"`
}

// Schedule represents a time period during which the instance should be spun up.
type Schedule struct {
	Name     string   `json:"name"`
	Start    string   `json:"start"`
	Duration Duration `json:"duration"`
}

// Duration wraps time.Duration to provide custom JSON marshalling
type Duration struct {
	time.Duration
}

// MarshalJSON implements the json.Marshaler interface
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Duration.String())
}

// UnmarshalJSON implements the json.Unmarshaler interface
func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	duration, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration format: %v", err)
	}
	d.Duration = duration
	return nil
}

type InstanceType string

// InstanceType represents the type of instance.
const (
	InstanceTypeEC2 InstanceType = "ec2"
)

// UnmarshalJSON implements the json.Unmarshaler interface for InstanceType
func (it *InstanceType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*it = InstanceType(s)
	return nil
}

// Instance represents a machine that runs services to be proxied.
type Instance struct {
	Name      string       `json:"name"`
	Type      InstanceType `json:"type"`
	EC2       *EC2         `json:"ec2,omitempty"`
	Timeout   Duration     `json:"timeout"`
	Services  []Service    `json:"services,omitempty"`
	Schedules []Schedule   `json:"schedules,omitempty"`
}

// Config represents the configuration for the application.
type Config struct {
	Instances []Instance `json:"instances"`
}
