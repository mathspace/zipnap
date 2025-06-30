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

	"github.com/kaptinlin/jsonschema"
	"sigs.k8s.io/yaml"
)

//go:embed schema.json
var schemaBytes []byte

var schema *jsonschema.Schema

func init() {
	var err error
	schema, err = jsonschema.NewCompiler().Compile(schemaBytes)
	if err != nil {
		panic(fmt.Sprintf("failed to compile schema: %v", err))
	}
}

// EC2 represents the configuration for an EC2 instance.
type EC2 struct {
	InstanceID string `json:"instance_id"`
	// Shutdown indicates whether the instance should be shut down or
	// hibernated.
	Shutdown bool `json:"hibernate"`
}

// HTTP represents the configuration for an HTTP service. The service is assumed
// to be active when request sent to path / on given port returns a 2xx-3xx
// status code.
type HTTP struct {
	Port int `json:"port"`
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
	Name string      `json:"name"`
	Type ServiceType `json:"type"`
	HTTP *HTTP       `json:"http,omitempty"`
}

// Schedule represents a time period during which the instance should be spun up.
type Schedule struct {
	Name     string   `json:"name"`
	Start    string   `json:"start"`
	Duration Duration `json:"duration"`
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
	Name      string       `json:"name"`
	Type      InstanceType `json:"type"`
	EC2       *EC2         `json:"ec2,omitempty"`
	Timeout   Duration     `json:"timeout"`
	Services  []Service    `json:"services,omitempty"`
	Schedules []Schedule   `json:"schedules,omitempty"`
}

// Config represents the configuration for the application.
type Config struct {
	Instances []Instance `json:"instances"`
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
