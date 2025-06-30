// Package config defines the configuration structures for the application.
package config

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"sigs.k8s.io/yaml"
)

var schema *jsonschema.Schema

func init() {
}

// EC2 represents the configuration for an EC2 instance.
type EC2 struct {
	InstanceID string `json:"instance_id" jsonschema:"required,title=Instance ID,description=The EC2 instance identifier"`
	// Shutdown indicates whether the instance should be shut down or
	// hibernated.
	Shutdown bool `json:"hibernate" jsonschema:"title=Hibernate,description=Whether the instance should be hibernated instead of shut down"`
}

// HTTP represents the configuration for an HTTP service. The service is assumed
// to be active when request sent to path / on given port returns a 2xx-3xx
// status code.
type HTTP struct {
	Port int `json:"port" jsonschema:"required,title=Port,description=The port number for the HTTP service,minimum=1,maximum=65535"`
}

type ServiceType string

// ServiceType represents the type of service.
const (
	ServiceTypeHTTP ServiceType = "http"
)

// UnmarshalJSON implements the json.Unmarshaler interface for ServiceType
func (st *ServiceType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*st = ServiceType(s)
	return nil
}

// Service represents a service that is to be proxied.
type Service struct {
	Name string      `json:"name" jsonschema:"required,title=Service Name,description=The name of the service"`
	Type ServiceType `json:"type" jsonschema:"required,title=Service Type,description=The type of service to proxy"`
	HTTP *HTTP       `json:"http,omitempty" jsonschema:"title=HTTP Configuration,description=HTTP service configuration (required when type is http)"`
}

// Schedule represents a time period during which the instance should be spun up.
type Schedule struct {
	Name     string   `json:"name" jsonschema:"required,title=Schedule Name,description=The name of the schedule"`
	Start    string   `json:"start" jsonschema:"required,title=Start Time,description=The start time in cron format"`
	Duration Duration `json:"duration" jsonschema:"required,title=Duration,description=How long the instance should remain active"`
}

// Duration wraps time.Duration to provide custom JSON marshalling
type Duration struct {
	time.Duration
}

// MarshalJSON implements the json.Marshaler interface
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Duration.String())
}

// UnmarshalJSON implements the json.Unmarshaler interface
func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
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

// UnmarshalJSON implements the json.Unmarshaler interface for InstanceType
func (it *InstanceType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*it = InstanceType(s)
	return nil
}

// Instance represents a machine that runs services to be proxied.
type Instance struct {
	Name      string       `json:"name" jsonschema:"required,title=Instance Name,description=The name of the instance"`
	Type      InstanceType `json:"type" jsonschema:"required,title=Instance Type,description=The type of instance"`
	EC2       *EC2         `json:"ec2,omitempty" jsonschema:"title=EC2 Configuration,description=EC2 instance configuration (required when type is ec2)"`
	Timeout   Duration     `json:"timeout" jsonschema:"required,title=Timeout,description=How long to wait for the instance to become ready"`
	Services  []Service    `json:"services,omitempty" jsonschema:"title=Services,description=List of services running on this instance"`
	Schedules []Schedule   `json:"schedules,omitempty" jsonschema:"title=Schedules,description=List of schedules for automatic instance management"`
}

// Config represents the configuration for the application.
type Config struct {
	Instances []Instance `json:"instances" jsonschema:"required,title=Instances,description=List of instances to manage,minItems=1"`
}

// Load reads the configuration from the provided io.Reader, validates it
// against the schema, and returns a Config object.
func Load(r io.Reader) (*Config, error) {
	var config Config
	yb, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}
	jb, err := yaml.YAMLToJSONStrict(yb)
	if err != nil {
		return nil, fmt.Errorf("failed to convert YAML to JSON: %w", err)
	}
	valResults := schema.ValidateJSON(jb)
	if !valResults.IsValid() {
		var buf bytes.Buffer
		for _, err := range valResults.Errors {
			buf.WriteString(fmt.Sprintf("Validation error: %s\n", err))
		}
		return nil, fmt.Errorf("config validation failed:\n%s", buf.String())
	}
	decoder := json.NewDecoder(bytes.NewReader(jb))
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
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
