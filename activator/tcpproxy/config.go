package tcpproxy

import "fmt"

// Config represents the configuration for a TCPProxy proxy activator.
type Config struct {
	HostPort  int    `yaml:"host_port"`
	ProxyPort int    `yaml:"proxy_port"`
	ProxyHost string `yaml:"proxy_host,omitempty"`
}

// Validate checks the TCP configuration for validity.
func (t *Config) Validate() error {
	if t.HostPort <= 0 || t.HostPort > 65535 {
		return fmt.Errorf("invalid host port %d, must be between 1 and 65535", t.HostPort)
	}
	if t.ProxyPort <= 0 || t.ProxyPort > 65535 {
		return fmt.Errorf("invalid proxy port %d, must be between 1 and 65535", t.ProxyPort)
	}
	return nil
}
