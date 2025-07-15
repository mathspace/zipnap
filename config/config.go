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

// EC2 represents the configuration for an EC2 instance.
type EC2 struct {
	InstanceID string `yaml:"instance_id"`
}

// StoreForwardRule represents a rule for storing and forwarding HTTP requests
// while the host is down.
type StoreForwardRule struct {
	// Method is the HTTP method (GET, POST, etc.) that this rule applies to.
	// Leave empty to match all methods.
	Method string `yaml:"http_method"`
	// Path is the path that this rule applies to. It can be a full path or a
	// prefix. If it is a prefix, it should end with a slash (e.g., "/api/").
	Path string `yaml:"path"`
}

type HTTPHealthCheck struct {
	Interval    Duration `yaml:"interval"`
	Path        string   `yaml:"path"`
	StatusCodes []int    `yaml:"status_codes"`
}

// HTTP represents the configuration for an HTTP service. The service is assumed
// to be active when request sent to path / on given port returns a 2xx-3xx
// status code.
type HTTP struct {
	ServicePort       int                `yaml:"service_port"`
	ProxyPort         int                `yaml:"proxy_port"`
	ProxyHost         string             `yaml:"proxy_host,omitempty"`
	ShowWaitingPage   bool               `yaml:"show_waiting_page,omitempty"`
	StoreForwardRules []StoreForwardRule `yaml:"store_forward_rules,omitempty"`
	HealthCheck       *HTTPHealthCheck   `yaml:"health_check,omitempty"`
}

// Validate checks the HTTP configuration for validity.
func (h *HTTP) Validate() error {
	if h.ServicePort <= 0 || h.ServicePort > 65535 {
		return fmt.Errorf("invalid service port %d, must be between 1 and 65535", h.ServicePort)
	}
	if h.ProxyPort <= 0 || h.ProxyPort > 65535 {
		return fmt.Errorf("invalid proxy port %d, must be between 1 and 65535", h.ProxyPort)
	}
	if h.HealthCheck != nil {
		if h.HealthCheck.Path == "" {
			return fmt.Errorf("health check path must not be empty")
		}
		if len(h.HealthCheck.StatusCodes) == 0 {
			return fmt.Errorf("health check must have at least one status code")
		}
		if h.HealthCheck.Interval.Duration <= 0 {
			return fmt.Errorf("health check interval must be a positive duration")
		}
	}
	for _, rule := range h.StoreForwardRules {
		if rule.Method == "" {
			return fmt.Errorf("store forward rule method must not be empty")
		}
		if rule.Path == "" {
			return fmt.Errorf("store forward rule path must not be empty")
		}
	}
	return nil
}

// TCP represents the configuration for a TCP service. The service is assumed to
// be active when a TCP connection to the service port is established
// successfully.
type TCP struct {
	ServicePort int    `yaml:"service_port"`
	ProxyPort   int    `yaml:"proxy_port"`
	ProxyHost   string `yaml:"proxy_host,omitempty"`
}

// Validate checks the TCP configuration for validity.
func (t *TCP) Validate() error {
	if t.ServicePort <= 0 || t.ServicePort > 65535 {
		return fmt.Errorf("invalid service port %d, must be between 1 and 65535", t.ServicePort)
	}
	if t.ProxyPort <= 0 || t.ProxyPort > 65535 {
		return fmt.Errorf("invalid proxy port %d, must be between 1 and 65535", t.ProxyPort)
	}
	return nil
}

// ServiceType represents the type of service.
const (
	ServiceTypeHTTP = "http"
	ServiceTypeTCP  = "tcp"
)

// Service represents a service that is to be proxied.
type Service struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`

	HTTP *HTTP `yaml:"http,omitempty"`
	TCP  *TCP  `yaml:"tcp,omitempty"`
}

func (s *Service) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("service name must not be empty")
	}
	if s.Type == "" {
		return fmt.Errorf("service type must not be empty")
	}

	switch s.Type {
	case ServiceTypeHTTP:
		if s.HTTP == nil {
			return fmt.Errorf("HTTP service must have HTTP configuration")
		}
		return s.HTTP.Validate()
	case ServiceTypeTCP:
		if s.TCP == nil {
			return fmt.Errorf("TCP service must have TCP configuration")
		}
		return s.TCP.Validate()
	default:
		return fmt.Errorf("unsupported service type %q", s.Type)
	}
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
	Name      string              `yaml:"name"`
	Type      InstanceType        `yaml:"type"`
	EC2       *EC2                `yaml:"ec2,omitempty"`
	Timeout   Duration            `yaml:"timeout"`
	Services  map[string]Service  `yaml:"services,omitempty"`
	Schedules map[string]Schedule `yaml:"schedules,omitempty"`
}

// Config represents the configuration for the application.
type Config struct {
	Instances map[string]Instance `yaml:"instances"`
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

	return &config, config.Validate()
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
