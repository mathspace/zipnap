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

// HTTPProxy represents the configuration for an HTTPProxy proxy activator.
type HTTPProxy struct {
	HostPort             int                `yaml:"host_port"`
	ProxyPort            int                `yaml:"proxy_port"`
	ProxyHost            string             `yaml:"proxy_host,omitempty"`
	StoreForwardRules    []StoreForwardRule `yaml:"store_forward_rules,omitempty"`
	ShowWaitingPageAfter Duration           `yaml:"show_waiting_page_after,omitempty"`
	HealthCheck          *HTTPHealthCheck   `yaml:"health_check,omitempty"`
}

// Validate checks the HTTP configuration for validity.
func (h *HTTPProxy) Validate() error {
	if h.HostPort <= 0 || h.HostPort > 65535 {
		return fmt.Errorf("invalid host port %d, must be between 1 and 65535", h.HostPort)
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
	if h.ShowWaitingPageAfter.Duration < 0 {
		return fmt.Errorf("show waiting page after must be a non-negative duration")
	}
	return nil
}

// TCPProxy represents the configuration for a TCPProxy proxy activator.
type TCPProxy struct {
	HostPort  int    `yaml:"host_port"`
	ProxyPort int    `yaml:"proxy_port"`
	ProxyHost string `yaml:"proxy_host,omitempty"`
}

// Validate checks the TCP configuration for validity.
func (t *TCPProxy) Validate() error {
	if t.HostPort <= 0 || t.HostPort > 65535 {
		return fmt.Errorf("invalid host port %d, must be between 1 and 65535", t.HostPort)
	}
	if t.ProxyPort <= 0 || t.ProxyPort > 65535 {
		return fmt.Errorf("invalid proxy port %d, must be between 1 and 65535", t.ProxyPort)
	}
	return nil
}

// Activator represents a host activator that can wake up a host.
type Activator struct {
	ID string `yaml:"-"`

	HTTPProxy *HTTPProxy `yaml:"http,omitempty"`
	TCPProxy  *TCPProxy  `yaml:"tcp,omitempty"`
}

func (s *Activator) Validate() error {
	typeCount := 0
	for _, v := range []any{s.HTTPProxy, s.TCPProxy} {
		if v != nil {
			typeCount++
		}
	}
	if typeCount != 1 {
		return fmt.Errorf("activator must have exactly one type defined (HTTP or TCP), found %d", typeCount)
	}
	return nil
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

// Instance represents a machine.
type Instance struct {
	ID         string               `yaml:"-"`
	EC2        *EC2                 `yaml:"ec2,omitempty"`
	Timeout    Duration             `yaml:"timeout"`
	Activators map[string]Activator `yaml:"activators,omitempty"`
}

func (i *Instance) UnmarshalYAML(n *yaml.Node) error {
	type alias Instance
	var a alias
	if err := n.Decode(&a); err != nil {
		return err
	}
	for id, svc := range a.Activators {
		svc.ID = id
		a.Activators[id] = svc
	}
	*i = Instance(a)
	return nil
}

func (i *Instance) Validate() error {
	if i.Timeout.Duration <= 0 {
		return fmt.Errorf("timeout must be a positive duration")
	}

	typeCount := 0
	for _, v := range []any{i.EC2} {
		if v != nil {
			typeCount++
		}
	}
	if typeCount != 1 {
		return fmt.Errorf("instance must have exactly one type defined (EC2), found %d", typeCount)
	}

	for svcID, svc := range i.Activators {
		if err := svc.Validate(); err != nil {
			return fmt.Errorf("activator %q: %w", svcID, err)
		}
	}

	return nil
}

// Config represents the configuration for the application.
type Config struct {
	Instances map[string]Instance `yaml:"instances"`
}

func (c *Config) UnmarshalYAML(n *yaml.Node) error {
	inst := make(map[string]Instance)
	if err := n.Decode(&inst); err != nil {
		return err
	}
	for id, i := range inst {
		i.ID = id
		inst[id] = i
	}
	c.Instances = inst
	return nil
}

func (c *Config) Validate() error {
	for id, inst := range c.Instances {
		if err := inst.Validate(); err != nil {
			return fmt.Errorf("instance %q: %w", id, err)
		}
	}
	return nil
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
