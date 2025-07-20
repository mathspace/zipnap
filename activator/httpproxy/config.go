package httpproxy

import (
	"fmt"

	"github.com/mathspace/zipnap/config/duration"
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

// Validate checks the HTTP configuration for validity.
func (h *Config) Validate() error {
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
