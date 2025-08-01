// Package config defines the configuration structures for the application.
package config

import (
	"fmt"
	"io"
	"os"
	"reflect"

	"github.com/mathspace/zipnap/activator/httpproxy"
	"github.com/mathspace/zipnap/activator/schedule"
	"github.com/mathspace/zipnap/activator/tcpproxy"
	"github.com/mathspace/zipnap/config/duration"
	"github.com/mathspace/zipnap/host/ec2host"
	"github.com/mathspace/zipnap/host/rdshost"
	"gopkg.in/yaml.v3"
)

func isValidatorNil(v Validator) bool {
	if v == nil {
		return true
	}
	val := reflect.ValueOf(v)
	return val.IsNil()
}

type Validator interface {
	Validate() error
}

// Activator represents a host activator that can wake up a host.
type Activator struct {
	ID string `yaml:"-"`

	HTTPProxy *httpproxy.Config `yaml:"http_proxy,omitempty"`
	TCPProxy  *tcpproxy.Config  `yaml:"tcp_proxy,omitempty"`
	Schedule  *schedule.Config  `yaml:"schedule,omitempty"`
}

func (s *Activator) Validate() error {
	typeCount := 0
	for _, v := range []Validator{s.HTTPProxy, s.TCPProxy, s.Schedule} {
		if !isValidatorNil(v) {
			typeCount++
			if err := v.Validate(); err != nil {
				return fmt.Errorf("activator %q: %w", s.ID, err)
			}
		}
	}
	if typeCount != 1 {
		return fmt.Errorf("activator must have exactly one type defined, found %d", typeCount)
	}
	return nil
}

// Instance represents a machine.
type Instance struct {
	ID         string               `yaml:"-"`
	EC2        *ec2host.Config      `yaml:"ec2,omitempty"`
	RDS        *rdshost.Config      `yaml:"rds,omitempty"`
	Timeout    duration.Duration    `yaml:"timeout"`
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
		return fmt.Errorf("timeout is required and must be a positive duration")
	}

	typeCount := 0
	for _, v := range []Validator{i.EC2, i.RDS} {
		if !isValidatorNil(v) {
			typeCount++
			if err := v.Validate(); err != nil {
				return fmt.Errorf("instance %q: %w", i.ID, err)
			}
		}
	}
	if typeCount != 1 {
		return fmt.Errorf("instance must have exactly one type defined, found %d", typeCount)
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
	Instances map[string]*Instance `yaml:"instances"`
}

func (c *Config) UnmarshalYAML(n *yaml.Node) error {
	type alias Config
	cfg := alias{}
	if err := n.Decode(&cfg); err != nil {
		return err
	}
	for id, i := range cfg.Instances {
		i.ID = id
	}
	*c = Config(cfg)
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
