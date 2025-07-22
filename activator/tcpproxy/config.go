package tcpproxy

import "fmt"

// Config represents the configuration for a TCPProxy proxy activator.
type Config struct {
	HostPort  int    `yaml:"host_port"`
	ProxyPort int    `yaml:"proxy_port"`
	ProxyHost string `yaml:"proxy_host,omitempty"`
}

// Validate checks the TCP configuration for validity.
func (c *Config) Validate() error {
	if c.HostPort <= 0 || c.HostPort > 65535 {
		return fmt.Errorf("invalid host port %d, must be between 1 and 65535", c.HostPort)
	}
	if c.ProxyPort <= 0 || c.ProxyPort > 65535 {
		return fmt.Errorf("invalid proxy port %d, must be between 1 and 65535", c.ProxyPort)
	}
	return nil
}
