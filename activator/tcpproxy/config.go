package tcpproxy

import "fmt"

// Config represents the configuration for a TCPProxy proxy activator.
type Config struct {
	HostPort   int    `yaml:"host_port"`
	ListenPort int    `yaml:"listen_port"`
	ListenAddr string `yaml:"listen_addr,omitempty"`
}

// Validate checks the TCP configuration for validity.
func (c *Config) Validate() error {
	if c.HostPort <= 0 || c.HostPort > 65535 {
		return fmt.Errorf("invalid host port %d, must be between 1 and 65535", c.HostPort)
	}
	if c.ListenPort <= 0 || c.ListenPort > 65535 {
		return fmt.Errorf("invalid listen port %d, must be between 1 and 65535", c.ListenPort)
	}
	return nil
}
