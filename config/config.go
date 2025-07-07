// Package config defines the configuration structures for the application.
package config

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
)

// EC2 represents the configuration for an EC2 instance.
type EC2 struct {
	InstanceID string `yaml:"instance_id"`
	// Shutdown indicates whether the instance should be shut down or
	// hibernated.
	Shutdown bool `yaml:"shutdown"`
}

// HTTP represents the configuration for an HTTP service. The service is assumed
// to be active when request sent to path / on given port returns a 2xx-3xx
// status code.
type HTTP struct {
	ServicePort     int  `yaml:"service_port"`
	ProxyPort       int  `yaml:"proxy_port"`
	ShowWaitingPage bool `yaml:"show_waiting_page,omitempty"`
}

type ServiceType string

// ServiceType represents the type of service.
const (
	ServiceTypeHTTP ServiceType = "http"
)

func (st *ServiceType) UnmarshalYAML(n *yaml.Node) error {
	var s string
	if err := n.Decode(&s); err != nil {
		return err
	}
	*st = ServiceType(s)
	return nil
}

// Service represents a service that is to be proxied.
type Service struct {
	Name string      `yaml:"name"`
	Type ServiceType `yaml:"type"`
	HTTP *HTTP       `yaml:"http,omitempty"`
}

type CronSchedule struct {
	cron.Schedule
}

// UnmarshalYAML implements custom unmarshalling for CronSchedule to handle
// cron expressions.
func (cs *CronSchedule) UnmarshalYAML(n *yaml.Node) error {
	var expr string
	if err := n.Decode(&expr); err != nil {
		return fmt.Errorf("failed to decode cron expression: %w", err)
	}

	schedule, err := cron.ParseStandard(expr)
	if err != nil {
		return fmt.Errorf("invalid cron expression %q: %w", expr, err)
	}

	cs.Schedule = schedule
	return nil
}

// Schedule represents a time period during which the instance should be spun up.
type Schedule struct {
	Name     string       `yaml:"name"`
	Start    CronSchedule `yaml:"start"`
	Duration Duration     `yaml:"duration"`
}

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

type InstanceType string

// InstanceType represents the type of instance.
const (
	InstanceTypeEC2 InstanceType = "ec2"
)

func (it *InstanceType) UnmarshalYAML(n *yaml.Node) error {
	var s string
	if err := n.Decode(&s); err != nil {
		return err
	}
	*it = InstanceType(s)
	return nil
}

// Instance represents a machine that runs services to be proxied.
type Instance struct {
	Name      string       `yaml:"name"`
	Type      InstanceType `yaml:"type"`
	EC2       *EC2         `yaml:"ec2,omitempty"`
	Timeout   Duration     `yaml:"timeout"`
	Services  []Service    `yaml:"services,omitempty"`
	Schedules []Schedule   `yaml:"schedules,omitempty"`
}

// Config represents the configuration for the application.
type Config struct {
	Instances []Instance `yaml:"instances"`
}

// Load reads the configuration from the provided io.Reader, validates it
// against the schema, and returns a Config object.
func Load(r io.Reader) (*Config, error) {

	var config Config

	decoder := yaml.NewDecoder(r)
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Validate the configuration

	for _, i := range config.Instances {
		if i.Name == "" {
			return nil, fmt.Errorf("instance must have a name")
		}
		if i.Timeout.Duration <= 0 {
			return nil, fmt.Errorf("instance %q must have a positive timeout", i.Name)
		}
		if i.Type != InstanceTypeEC2 {
			return nil, fmt.Errorf("instance %q has unsupported type %q", i.Name, i.Type)
		}
		if i.Type == InstanceTypeEC2 && i.EC2 == nil {
			return nil, fmt.Errorf("instance %q of type %q must have EC2 configuration", i.Name, i.Type)
		}
		if i.EC2 != nil {
			if i.EC2.InstanceID == "" {
				return nil, fmt.Errorf("instance %q of type %q must have a valid instance_id", i.Name, i.Type)
			}
		}
		for _, s := range i.Services {
			if s.Name == "" {
				return nil, fmt.Errorf("service in instance %q must have a name", i.Name)
			}
			if s.Type != ServiceTypeHTTP {
				return nil, fmt.Errorf("service %q in instance %q has unsupported type %q", s.Name, i.Name, s.Type)
			}
			if s.Type == ServiceTypeHTTP && s.HTTP == nil {
				return nil, fmt.Errorf("service %q in instance %q of type %q must have HTTP configuration", s.Name, i.Name, s.Type)
			}
			if s.HTTP != nil {
				if s.HTTP.ServicePort <= 0 || s.HTTP.ServicePort > 65535 {
					return nil, fmt.Errorf("service %q in instance %q has invalid service http port %d", s.Name, i.Name, s.HTTP.Port)
				}
				if s.HTTP.ProxyPort <= 0 || s.HTTP.ProxyPort > 65535 {
					return nil, fmt.Errorf("service %q in instance %q has invalid proxy http port %d", s.Name, i.Name, s.HTTP.Port)
				}
			}
		}
		for _, s := range i.Schedules {
			if s.Name == "" {
				return nil, fmt.Errorf("schedule in instance %q must have a name", i.Name)
			}
			if s.Start.Schedule == nil {
				return nil, fmt.Errorf("schedule %q in instance %q must have a start time", s.Name, i.Name)
			}
			if s.Duration.Duration <= 0 {
				return nil, fmt.Errorf("schedule %q in instance %q must have a positive duration", s.Name, i.Name)
			}
		}
	}

	return &config, nil
}

// LoadFile reads the configuration from a file at the specified path,
// validates it against the schema, and returns a Config object.
func LoadFile(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	return Load(file)
}
