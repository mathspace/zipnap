package httpproxy

import (
	"fmt"
	"time"

	"github.com/mathspace/zipnap/config/duration"
	"gopkg.in/yaml.v3"
)

type HTTPHealthCheck struct {
	Interval    duration.Duration `yaml:"interval"`
	Path        string            `yaml:"path"`
	StatusCodes []int             `yaml:"status_codes"`
}

// Config represents the configuration for an HTTPProxy proxy activator.
type Config struct {
	HostPort             int               `yaml:"host_port"`
	ListenPort           int               `yaml:"listen_port"`
	ListenAddr           string            `yaml:"listen_addr,omitempty"`
	ShowWaitingPageAfter duration.Duration `yaml:"show_waiting_page_after,omitempty"`
	HealthCheck          *HTTPHealthCheck  `yaml:"health_check,omitempty"`
}

func (c *Config) UnmarshalYAML(n *yaml.Node) error {
	type alias Config
	var cfg alias
	if err := n.Decode(&cfg); err != nil {
		return err
	}
	if cfg.ShowWaitingPageAfter.Duration < 0 {
		return fmt.Errorf("show waiting page after must be a non-negative duration, got %s", cfg.ShowWaitingPageAfter.Duration)
	}
	if cfg.HealthCheck == nil {
		cfg.HealthCheck = &HTTPHealthCheck{}
	}
	if cfg.HealthCheck.Interval.Duration == 0 {
		cfg.HealthCheck.Interval.Duration = 3 * time.Second // Default health check interval
	}
	if len(cfg.HealthCheck.StatusCodes) == 0 {
		cfg.HealthCheck.StatusCodes = []int{200} // Default status codes for health check
	}
	if cfg.HealthCheck.Path == "" {
		cfg.HealthCheck.Path = "/" // Default health check path
	}
	*c = Config(cfg)
	return nil
}

// Validate checks the HTTP configuration for validity.
func (c *Config) Validate() error {
	if c.HostPort <= 0 || c.HostPort > 65535 {
		return fmt.Errorf("invalid host port %d, must be between 1 and 65535", c.HostPort)
	}
	if c.ListenPort <= 0 || c.ListenPort > 65535 {
		return fmt.Errorf("invalid proxy port %d, must be between 1 and 65535", c.ListenPort)
	}
	if c.HealthCheck == nil {
		return fmt.Errorf("health check configuration must not be nil")
	}
	if c.HealthCheck.Path == "" {
		return fmt.Errorf("health check path must not be empty")
	}
	if len(c.HealthCheck.StatusCodes) == 0 {
		return fmt.Errorf("health check must have at least one status code")
	}
	if c.HealthCheck.Interval.Duration <= 0 {
		return fmt.Errorf("health check interval must be a positive duration")
	}
	if c.ShowWaitingPageAfter.Duration < 0 {
		return fmt.Errorf("show waiting page after must be a non-negative duration")
	}
	return nil
}
