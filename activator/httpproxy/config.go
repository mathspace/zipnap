package httpproxy

import (
	"fmt"
	"time"

	"github.com/mathspace/zipnap/config/duration"
	"gopkg.in/yaml.v3"
)

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
	Interval    duration.Duration `yaml:"interval"`
	Path        string            `yaml:"path"`
	StatusCodes []int             `yaml:"status_codes"`
}

// Config represents the configuration for an HTTPProxy proxy activator.
type Config struct {
	HostPort             int                `yaml:"host_port"`
	ProxyPort            int                `yaml:"proxy_port"`
	ProxyHost            string             `yaml:"proxy_host,omitempty"`
	StoreForwardRules    []StoreForwardRule `yaml:"store_forward_rules,omitempty"`
	ShowWaitingPageAfter duration.Duration  `yaml:"show_waiting_page_after,omitempty"`
	HealthCheck          *HTTPHealthCheck   `yaml:"health_check,omitempty"`
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
	if c.ProxyPort <= 0 || c.ProxyPort > 65535 {
		return fmt.Errorf("invalid proxy port %d, must be between 1 and 65535", c.ProxyPort)
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
	for _, rule := range c.StoreForwardRules {
		if rule.Method == "" {
			return fmt.Errorf("store forward rule method must not be empty")
		}
		if rule.Path == "" {
			return fmt.Errorf("store forward rule path must not be empty")
		}
	}
	if c.ShowWaitingPageAfter.Duration < 0 {
		return fmt.Errorf("show waiting page after must be a non-negative duration")
	}
	return nil
}
