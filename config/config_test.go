package config

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDuration_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		duration Duration
		expected string
	}{
		{
			name:     "5 minutes",
			duration: Duration{5 * time.Minute},
			expected: `"5m0s"`,
		},
		{
			name:     "1 hour 30 minutes",
			duration: Duration{90 * time.Minute},
			expected: `"1h30m0s"`,
		},
		{
			name:     "30 seconds",
			duration: Duration{30 * time.Second},
			expected: `"30s"`,
		},
		{
			name:     "zero duration",
			duration: Duration{0},
			expected: `"0s"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.duration.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON() error = %v", err)
			}
			if string(result) != tt.expected {
				t.Errorf("MarshalJSON() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestDuration_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		expected    Duration
		expectError bool
	}{
		{
			name:     "5 minutes",
			json:     `"5m"`,
			expected: Duration{5 * time.Minute},
		},
		{
			name:     "1 hour 30 minutes",
			json:     `"1h30m"`,
			expected: Duration{90 * time.Minute},
		},
		{
			name:     "30 seconds",
			json:     `"30s"`,
			expected: Duration{30 * time.Second},
		},
		{
			name:     "complex duration",
			json:     `"2h15m30s"`,
			expected: Duration{2*time.Hour + 15*time.Minute + 30*time.Second},
		},
		{
			name:        "invalid format",
			json:        `"invalid"`,
			expectError: true,
		},
		{
			name:        "empty string",
			json:        `""`,
			expectError: true,
		},
		{
			name:        "invalid json",
			json:        `invalid`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d Duration
			err := d.UnmarshalJSON([]byte(tt.json))

			if tt.expectError {
				if err == nil {
					t.Errorf("UnmarshalJSON() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("UnmarshalJSON() error = %v", err)
			}

			if d.Duration != tt.expected.Duration {
				t.Errorf("UnmarshalJSON() = %v, want %v", d.Duration, tt.expected.Duration)
			}
		})
	}
}

func TestServiceType_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		expected    ServiceType
		expectError bool
	}{
		{
			name:     "http service type",
			json:     `"http"`,
			expected: ServiceTypeHTTP,
		},
		{
			name:     "custom service type",
			json:     `"custom"`,
			expected: ServiceType("custom"),
		},
		{
			name:        "invalid json",
			json:        `invalid`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var st ServiceType
			err := st.UnmarshalJSON([]byte(tt.json))

			if tt.expectError {
				if err == nil {
					t.Errorf("UnmarshalJSON() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("UnmarshalJSON() error = %v", err)
			}

			if st != tt.expected {
				t.Errorf("UnmarshalJSON() = %v, want %v", st, tt.expected)
			}
		})
	}
}

func TestInstanceType_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		expected    InstanceType
		expectError bool
	}{
		{
			name:     "ec2 instance type",
			json:     `"ec2"`,
			expected: InstanceTypeEC2,
		},
		{
			name:     "custom instance type",
			json:     `"custom"`,
			expected: InstanceType("custom"),
		},
		{
			name:        "invalid json",
			json:        `invalid`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var it InstanceType
			err := it.UnmarshalJSON([]byte(tt.json))

			if tt.expectError {
				if err == nil {
					t.Errorf("UnmarshalJSON() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("UnmarshalJSON() error = %v", err)
			}

			if it != tt.expected {
				t.Errorf("UnmarshalJSON() = %v, want %v", it, tt.expected)
			}
		})
	}
}

func TestLoad_ValidConfigs(t *testing.T) {
	tests := []struct {
		name   string
		yaml   string
		verify func(*testing.T, *Config)
	}{
		{
			name: "minimal valid config",
			yaml: `
instances:
  - name: test-instance
    type: ec2
    ec2:
      instance_id: i-1234567890abcdef0
      hibernate: true
    timeout: 5m
`,
			verify: func(t *testing.T, c *Config) {
				if len(c.Instances) != 1 {
					t.Errorf("Expected 1 instance, got %d", len(c.Instances))
				}
				inst := c.Instances[0]
				if inst.Name != "test-instance" {
					t.Errorf("Expected name 'test-instance', got '%s'", inst.Name)
				}
				if inst.Type != InstanceTypeEC2 {
					t.Errorf("Expected type 'ec2', got '%s'", inst.Type)
				}
				if inst.EC2 == nil {
					t.Fatal("Expected EC2 config to be present")
				}
				if inst.EC2.InstanceID != "i-1234567890abcdef0" {
					t.Errorf("Expected instance ID 'i-1234567890abcdef0', got '%s'", inst.EC2.InstanceID)
				}
				if !inst.EC2.Shutdown {
					t.Error("Expected hibernate to be true")
				}
				if inst.Timeout.Duration != 5*time.Minute {
					t.Errorf("Expected timeout 5m, got %v", inst.Timeout.Duration)
				}
			},
		},
		{
			name: "config with services and schedules",
			yaml: `
instances:
  - name: web-server
    type: ec2
    ec2:
      instance_id: i-abcdef1234567890
      hibernate: false
    timeout: 10m
    services:
      - name: web-app
        type: http
        http:
          port: 8080
      - name: api
        type: http
        http:
          port: 3000
    schedules:
      - name: business-hours
        start: "0 9 * * 1-5"
        duration: 8h
      - name: weekend-maintenance
        start: "@weekly"
        duration: 2h
`,
			verify: func(t *testing.T, c *Config) {
				if len(c.Instances) != 1 {
					t.Errorf("Expected 1 instance, got %d", len(c.Instances))
				}
				inst := c.Instances[0]

				// Verify services
				if len(inst.Services) != 2 {
					t.Errorf("Expected 2 services, got %d", len(inst.Services))
				}
				if inst.Services[0].Name != "web-app" {
					t.Errorf("Expected first service name 'web-app', got '%s'", inst.Services[0].Name)
				}
				if inst.Services[0].HTTP.Port != 8080 {
					t.Errorf("Expected first service port 8080, got %d", inst.Services[0].HTTP.Port)
				}

				// Verify schedules
				if len(inst.Schedules) != 2 {
					t.Errorf("Expected 2 schedules, got %d", len(inst.Schedules))
				}
				if inst.Schedules[0].Name != "business-hours" {
					t.Errorf("Expected first schedule name 'business-hours', got '%s'", inst.Schedules[0].Name)
				}
				if inst.Schedules[0].Duration.Duration != 8*time.Hour {
					t.Errorf("Expected first schedule duration 8h, got %v", inst.Schedules[0].Duration.Duration)
				}
			},
		},
		{
			name: "multiple instances",
			yaml: `
instances:
  - name: instance1
    type: ec2
    ec2:
      instance_id: i-1111111111111111
      hibernate: true
    timeout: 5m
  - name: instance2
    type: ec2
    ec2:
      instance_id: i-2222222222222222
      hibernate: false
    timeout: 15m
`,
			verify: func(t *testing.T, c *Config) {
				if len(c.Instances) != 2 {
					t.Errorf("Expected 2 instances, got %d", len(c.Instances))
				}
				if c.Instances[0].Name != "instance1" {
					t.Errorf("Expected first instance name 'instance1', got '%s'", c.Instances[0].Name)
				}
				if c.Instances[1].Name != "instance2" {
					t.Errorf("Expected second instance name 'instance2', got '%s'", c.Instances[1].Name)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.yaml)
			config, err := Load(reader)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if config == nil {
				t.Fatal("Load() returned nil config")
			}
			tt.verify(t, config)
		})
	}
}

func TestLoad_InvalidConfigs(t *testing.T) {
	tests := []struct {
		name          string
		yaml          string
		expectedError string
	}{
		{
			name:          "empty config",
			yaml:          "",
			expectedError: "config validation failed",
		},
		{
			name: "missing instances",
			yaml: `
other_field: value
`,
			expectedError: "config validation failed",
		},
		{
			name: "missing required instance fields",
			yaml: `
instances:
  - name: test
`,
			expectedError: "config validation failed",
		},
		{
			name: "invalid instance type",
			yaml: `
instances:
  - name: test-instance
    type: invalid
    timeout: 5m
`,
			expectedError: "config validation failed",
		},
		{
			name: "missing EC2 config for EC2 instance",
			yaml: `
instances:
  - name: test-instance
    type: ec2
    timeout: 5m
`,
			expectedError: "config validation failed",
		},
		{
			name: "invalid EC2 instance ID",
			yaml: `
instances:
  - name: test-instance
    type: ec2
    ec2:
      instance_id: invalid-id
      hibernate: true
    timeout: 5m
`,
			expectedError: "config validation failed",
		},
		{
			name: "invalid timeout format",
			yaml: `
instances:
  - name: test-instance
    type: ec2
    ec2:
      instance_id: i-1234567890abcdef0
      hibernate: true
    timeout: invalid
`,
			expectedError: "config validation failed",
		},
		{
			name: "invalid service type",
			yaml: `
instances:
  - name: test-instance
    type: ec2
    ec2:
      instance_id: i-1234567890abcdef0
      hibernate: true
    timeout: 5m
    services:
      - name: test-service
        type: invalid
`,
			expectedError: "config validation failed",
		},
		{
			name: "missing HTTP config for HTTP service",
			yaml: `
instances:
  - name: test-instance
    type: ec2
    ec2:
      instance_id: i-1234567890abcdef0
      hibernate: true
    timeout: 5m
    services:
      - name: test-service
        type: http
`,
			expectedError: "config validation failed",
		},
		{
			name: "invalid port number",
			yaml: `
instances:
  - name: test-instance
    type: ec2
    ec2:
      instance_id: i-1234567890abcdef0
      hibernate: true
    timeout: 5m
    services:
      - name: test-service
        type: http
        http:
          port: 70000
`,
			expectedError: "config validation failed",
		},
		{
			name: "invalid YAML syntax",
			yaml: `
instances:
  - name: test
    invalid: [
`,
			expectedError: "failed to convert YAML to JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.yaml)
			config, err := Load(reader)
			if err == nil {
				t.Errorf("Load() expected error but got none, config: %+v", config)
				return
			}
			if !strings.Contains(err.Error(), tt.expectedError) {
				t.Errorf("Load() error = %v, expected to contain %s", err, tt.expectedError)
			}
		})
	}
}

func TestLoad_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		expectError bool
	}{
		{
			name: "empty instances array",
			yaml: `
instances: []
`,
			expectError: false,
		},
		{
			name: "empty services array",
			yaml: `
instances:
  - name: test-instance
    type: ec2
    ec2:
      instance_id: i-1234567890abcdef0
      hibernate: true
    timeout: 5m
    services: []
`,
			expectError: false,
		},
		{
			name: "empty schedules array",
			yaml: `
instances:
  - name: test-instance
    type: ec2
    ec2:
      instance_id: i-1234567890abcdef0
      hibernate: true
    timeout: 5m
    schedules: []
`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.yaml)
			config, err := Load(reader)

			if tt.expectError {
				if err == nil {
					t.Errorf("Load() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Load() unexpected error = %v", err)
				return
			}

			if config == nil {
				t.Error("Load() returned nil config")
			}
		})
	}
}

func TestLoadFile_NonExistentFile(t *testing.T) {
	config, err := LoadFile("non-existent-file.yaml")
	if err == nil {
		t.Errorf("LoadFile() expected error for non-existent file but got none, config: %+v", config)
	}
	if !strings.Contains(err.Error(), "failed to open config file") {
		t.Errorf("LoadFile() error = %v, expected to contain 'failed to open config file'", err)
	}
}

func TestJSONMarshaling(t *testing.T) {
	// Test that our custom types can be marshaled and unmarshaled to/from JSON
	config := &Config{
		Instances: []Instance{
			{
				Name:    "test-instance",
				Type:    InstanceTypeEC2,
				Timeout: Duration{5 * time.Minute},
				EC2: &EC2{
					InstanceID: "i-1234567890abcdef0",
					Shutdown:   true,
				},
				Services: []Service{
					{
						Name: "web-service",
						Type: ServiceTypeHTTP,
						HTTP: &HTTP{Port: 8080},
					},
				},
				Schedules: []Schedule{
					{
						Name:     "business-hours",
						Start:    "0 9 * * 1-5",
						Duration: Duration{8 * time.Hour},
					},
				},
			},
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	// Unmarshal back to Config
	var unmarshaledConfig Config
	err = json.Unmarshal(jsonData, &unmarshaledConfig)
	if err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	// Verify the unmarshaled config matches the original
	if len(unmarshaledConfig.Instances) != 1 {
		t.Errorf("Expected 1 instance, got %d", len(unmarshaledConfig.Instances))
	}

	inst := unmarshaledConfig.Instances[0]
	if inst.Name != "test-instance" {
		t.Errorf("Expected name 'test-instance', got '%s'", inst.Name)
	}
	if inst.Timeout.Duration != 5*time.Minute {
		t.Errorf("Expected timeout 5m, got %v", inst.Timeout.Duration)
	}
	if len(inst.Services) != 1 {
		t.Errorf("Expected 1 service, got %d", len(inst.Services))
	}
	if inst.Services[0].HTTP.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", inst.Services[0].HTTP.Port)
	}
}

func TestLoadFile_ValidFile(t *testing.T) {
	// Create a temporary file with valid config
	validConfig := `
instances:
  - name: test-instance
    type: ec2
    ec2:
      instance_id: i-1234567890abcdef0
      hibernate: true
    timeout: 5m
`

	// Create temporary file
	tempFile := t.TempDir() + "/config.yaml"
	err := writeFile(tempFile, validConfig)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	// Test LoadFile
	config, err := LoadFile(tempFile)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if config == nil {
		t.Fatal("LoadFile() returned nil config")
	}
	if len(config.Instances) != 1 {
		t.Errorf("Expected 1 instance, got %d", len(config.Instances))
	}
	if config.Instances[0].Name != "test-instance" {
		t.Errorf("Expected name 'test-instance', got '%s'", config.Instances[0].Name)
	}
}

func TestLoadFile_InvalidFile(t *testing.T) {
	// Create a temporary file with invalid config
	invalidConfig := `
instances:
  - name: test
    type: invalid
`

	// Create temporary file
	tempFile := t.TempDir() + "/invalid-config.yaml"
	err := writeFile(tempFile, invalidConfig)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	// Test LoadFile
	config, err := LoadFile(tempFile)
	if err == nil {
		t.Errorf("LoadFile() expected error but got none, config: %+v", config)
	}
	if !strings.Contains(err.Error(), "config validation failed") {
		t.Errorf("LoadFile() error = %v, expected to contain 'config validation failed'", err)
	}
}

// Helper function to write content to a file
func writeFile(filename, content string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(content)
	return err
}
