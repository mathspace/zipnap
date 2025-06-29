// Package config defines the configuration structures for the application.
package config

import "time"

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

// Service represents a service that is to be proxied.
type Service struct {
	Name string      `json:"name"`
	Type ServiceType `json:"type"`
	HTTP *HTTP       `json:"http,omitempty"`
}

// Schedule represents a time period during which the backend should be spun up.
type Schedule struct {
	Name     string        `json:"name"`
	Start    string        `json:"start"`
	Duration time.Duration `json:"duration"`
}

type InstanceType string

// InstanceType represents the type of instance.
const (
	InstanceTypeEC2 InstanceType = "ec2"
)

// Instances represents a backend machine that runs services to be proxied.
type Instances struct {
	Name      string        `json:"name"`
	Type      InstanceType  `json:"type"`
	EC2       *EC2          `json:"ec2,omitempty"`
	Timeout   time.Duration `json:"timeout"`
	Services  []Service     `json:"services,omitempty"`
	Schedules []Schedule    `json:"schedules,omitempty"`
}

// Config represents the configuration for the application.
type Config struct {
	Instances []Instances `json:"backends"`
}
