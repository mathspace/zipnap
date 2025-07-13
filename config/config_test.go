package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoad_ValidConfig(t *testing.T) {
	configYAML := `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: "i-1234567890abcdef0"
    services:
      - name: "web"
        type: "http"
        http:
          service_port: 8080
          proxy_port: 8080
    schedules:
      - name: "daily"
        start: "0 9 * * *"
        duration: "8h"
`

	config, err := Load(strings.NewReader(configYAML))
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(config.Instances) != 1 {
		t.Fatalf("Expected 1 instance, got %d", len(config.Instances))
	}

	instance := config.Instances[0]
	if instance.Name != "web-server" {
		t.Errorf("Expected instance name 'web-server', got %q", instance.Name)
	}
	if instance.Type != InstanceTypeEC2 {
		t.Errorf("Expected instance type %q, got %q", InstanceTypeEC2, instance.Type)
	}
	if instance.Timeout.Duration != 30*time.Second {
		t.Errorf("Expected timeout 30s, got %v", instance.Timeout.Duration)
	}

	if instance.EC2 == nil {
		t.Fatal("Expected EC2 config to be present")
	}
	if instance.EC2.InstanceID != "i-1234567890abcdef0" {
		t.Errorf("Expected instance ID 'i-1234567890abcdef0', got %q", instance.EC2.InstanceID)
	}

	if len(instance.Services) != 1 {
		t.Fatalf("Expected 1 service, got %d", len(instance.Services))
	}

	service := instance.Services[0]
	if service.Name != "web" {
		t.Errorf("Expected service name 'web', got %q", service.Name)
	}
	if service.Type != ServiceTypeHTTP {
		t.Errorf("Expected service type %q, got %q", ServiceTypeHTTP, service.Type)
	}
	if service.HTTP == nil {
		t.Fatal("Expected HTTP config to be present")
	}
	if service.HTTP.ServicePort != 8080 {
		t.Errorf("Expected HTTP service port 8080, got %d", service.HTTP.ServicePort)
	}
	if service.HTTP.ProxyPort != 8080 {
		t.Errorf("Expected HTTP proxy port 8080, got %d", service.HTTP.ProxyPort)
	}

	if len(instance.Schedules) != 1 {
		t.Fatalf("Expected 1 schedule, got %d", len(instance.Schedules))
	}

	schedule := instance.Schedules[0]
	if schedule.Name != "daily" {
		t.Errorf("Expected schedule name 'daily', got %q", schedule.Name)
	}
	if schedule.Duration.Duration != 8*time.Hour {
		t.Errorf("Expected duration 8h, got %v", schedule.Duration.Duration)
	}
}

func TestLoad_MinimalValidConfig(t *testing.T) {
	configYAML := `
instances:
  - name: "minimal"
    type: "ec2"
    timeout: "1s"
    ec2:
      instance_id: "i-1234567890abcdef0"
`

	config, err := Load(strings.NewReader(configYAML))
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(config.Instances) != 1 {
		t.Fatalf("Expected 1 instance, got %d", len(config.Instances))
	}

	instance := config.Instances[0]
	if instance.Name != "minimal" {
		t.Errorf("Expected instance name 'minimal', got %q", instance.Name)
	}
	if len(instance.Services) != 0 {
		t.Errorf("Expected 0 services, got %d", len(instance.Services))
	}
	if len(instance.Schedules) != 0 {
		t.Errorf("Expected 0 schedules, got %d", len(instance.Schedules))
	}
}

func TestLoad_MultipleInstances(t *testing.T) {
	configYAML := `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: "i-1234567890abcdef0"
    services:
      - name: "web"
        type: "http"
        http:
          service_port: 8080
          proxy_port: 8080
  - name: "api-server"
    type: "ec2"
    timeout: "45s"
    ec2:
      instance_id: "i-0987654321fedcba0"
    services:
      - name: "api"
        type: "http"
        http:
          service_port: 3000
          proxy_port: 3000
      - name: "metrics"
        type: "http"
        http:
          service_port: 9090
          proxy_port: 9090
`

	config, err := Load(strings.NewReader(configYAML))
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(config.Instances) != 2 {
		t.Fatalf("Expected 2 instances, got %d", len(config.Instances))
	}

	// Check first instance
	instance1 := config.Instances[0]
	if instance1.Name != "web-server" {
		t.Errorf("Expected first instance name 'web-server', got %q", instance1.Name)
	}

	// Check second instance
	instance2 := config.Instances[1]
	if instance2.Name != "api-server" {
		t.Errorf("Expected second instance name 'api-server', got %q", instance2.Name)
	}
	if len(instance2.Services) != 2 {
		t.Errorf("Expected 2 services in second instance, got %d", len(instance2.Services))
	}
}

func TestLoad_EmptyConfig(t *testing.T) {
	configYAML := `instances: []`

	config, err := Load(strings.NewReader(configYAML))
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(config.Instances) != 0 {
		t.Errorf("Expected 0 instances, got %d", len(config.Instances))
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	configYAML := `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    invalid_field: "should cause error"
`

	_, err := Load(strings.NewReader(configYAML))
	if err == nil {
		t.Fatal("Expected error for invalid YAML field, got nil")
	}
}

func TestLoad_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		configYAML  string
		expectedErr string
	}{
		{
			name: "missing instance name",
			configYAML: `
instances:
  - type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: "i-1234567890abcdef0"
`,
			expectedErr: "instance must have a name",
		},
		{
			name: "zero timeout",
			configYAML: `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "0s"
    ec2:
      instance_id: "i-1234567890abcdef0"
`,
			expectedErr: "must have a positive timeout",
		},
		{
			name: "negative timeout",
			configYAML: `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "-30s"
    ec2:
      instance_id: "i-1234567890abcdef0"
`,
			expectedErr: "must have a positive timeout",
		},
		{
			name: "unsupported instance type",
			configYAML: `
instances:
  - name: "web-server"
    type: "gcp"
    timeout: "30s"
`,
			expectedErr: "has unsupported type",
		},
		{
			name: "missing EC2 config",
			configYAML: `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
`,
			expectedErr: "must have EC2 configuration",
		},
		{
			name: "missing EC2 instance ID",
			configYAML: `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    ec2: {}
`,
			expectedErr: "must have a valid instance_id",
		},
		{
			name: "empty EC2 instance ID",
			configYAML: `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: ""
`,
			expectedErr: "must have a valid instance_id",
		},
		{
			name: "missing service name",
			configYAML: `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: "i-1234567890abcdef0"
    services:
      - type: "http"
        http:
          service_port: 8080
          proxy_port: 8080
`,
			expectedErr: "service in instance \"web-server\" must have a name",
		},
		{
			name: "empty service name",
			configYAML: `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: "i-1234567890abcdef0"
    services:
      - name: ""
        type: "http"
        http:
          service_port: 8080
          proxy_port: 8080
`,
			expectedErr: "service in instance \"web-server\" must have a name",
		},
		{
			name: "unsupported service type",
			configYAML: `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: "i-1234567890abcdef0"
    services:
      - name: "web"
        type: "grpc"
`,
			expectedErr: "has unsupported type",
		},
		{
			name: "missing HTTP config",
			configYAML: `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: "i-1234567890abcdef0"
    services:
      - name: "web"
        type: "http"
`,
			expectedErr: "must have HTTP configuration",
		},
		{
			name: "invalid HTTP service port - zero",
			configYAML: `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: "i-1234567890abcdef0"
    services:
      - name: "web"
        type: "http"
        http:
          service_port: 0
          proxy_port: 8080
`,
			expectedErr: "has invalid service http port",
		},
		{
			name: "invalid HTTP service port - negative",
			configYAML: `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: "i-1234567890abcdef0"
    services:
      - name: "web"
        type: "http"
        http:
          service_port: -1
          proxy_port: 8080
`,
			expectedErr: "has invalid service http port",
		},
		{
			name: "invalid HTTP service port - too high",
			configYAML: `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: "i-1234567890abcdef0"
    services:
      - name: "web"
        type: "http"
        http:
          service_port: 65536
          proxy_port: 8080
`,
			expectedErr: "has invalid service http port",
		},
		{
			name: "invalid HTTP proxy port - zero",
			configYAML: `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: "i-1234567890abcdef0"
    services:
      - name: "web"
        type: "http"
        http:
          service_port: 8080
          proxy_port: 0
`,
			expectedErr: "has invalid proxy http port",
		},
		{
			name: "invalid HTTP proxy port - negative",
			configYAML: `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: "i-1234567890abcdef0"
    services:
      - name: "web"
        type: "http"
        http:
          service_port: 8080
          proxy_port: -1
`,
			expectedErr: "has invalid proxy http port",
		},
		{
			name: "invalid HTTP proxy port - too high",
			configYAML: `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: "i-1234567890abcdef0"
    services:
      - name: "web"
        type: "http"
        http:
          service_port: 8080
          proxy_port: 65536
`,
			expectedErr: "has invalid proxy http port",
		},
		{
			name: "missing schedule name",
			configYAML: `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: "i-1234567890abcdef0"
    schedules:
      - start: "0 9 * * *"
        duration: "8h"
`,
			expectedErr: "schedule in instance \"web-server\" must have a name",
		},
		{
			name: "empty schedule name",
			configYAML: `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: "i-1234567890abcdef0"
    schedules:
      - name: ""
        start: "0 9 * * *"
        duration: "8h"
`,
			expectedErr: "schedule in instance \"web-server\" must have a name",
		},
		{
			name: "zero schedule duration",
			configYAML: `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: "i-1234567890abcdef0"
    schedules:
      - name: "daily"
        start: "0 9 * * *"
        duration: "0s"
`,
			expectedErr: "must have a positive duration",
		},
		{
			name: "negative schedule duration",
			configYAML: `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: "i-1234567890abcdef0"
    schedules:
      - name: "daily"
        start: "0 9 * * *"
        duration: "-1h"
`,
			expectedErr: "must have a positive duration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(strings.NewReader(tt.configYAML))
			if err == nil {
				t.Fatal("Expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.expectedErr) {
				t.Errorf("Expected error containing %q, got %v", tt.expectedErr, err)
			}
		})
	}
}

func TestCronSchedule_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name        string
		cronExpr    string
		expectError bool
	}{
		{
			name:        "valid daily cron",
			cronExpr:    "0 9 * * *",
			expectError: false,
		},
		{
			name:        "valid weekday cron",
			cronExpr:    "0 9 * * 1-5",
			expectError: false,
		},
		{
			name:        "valid hourly cron",
			cronExpr:    "0 * * * *",
			expectError: false,
		},
		{
			name:        "invalid cron - wrong format",
			cronExpr:    "invalid",
			expectError: true,
		},
		{
			name:        "invalid cron - too many fields",
			cronExpr:    "0 0 9 * * * *",
			expectError: true,
		},
		{
			name:        "invalid cron - too few fields",
			cronExpr:    "0 9",
			expectError: true,
		},
		{
			name:        "empty cron expression",
			cronExpr:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configYAML := `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: "i-1234567890abcdef0"
    schedules:
      - name: "test"
        start: "` + tt.cronExpr + `"
        duration: "8h"
`

			_, err := Load(strings.NewReader(configYAML))
			if tt.expectError && err == nil {
				t.Fatal("Expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
		})
	}
}

func TestCronSchedule_Next(t *testing.T) {
	configYAML := `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: "i-1234567890abcdef0"
    schedules:
      - name: "daily"
        start: "0 9 * * *"
        duration: "8h"
`

	config, err := Load(strings.NewReader(configYAML))
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	schedule := config.Instances[0].Schedules[0]
	now := time.Date(2023, 12, 1, 8, 0, 0, 0, time.UTC)
	next := schedule.Start.Next(now)

	expected := time.Date(2023, 12, 1, 9, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("Expected next time %v, got %v", expected, next)
	}

	// Test when current time is after the scheduled time
	now = time.Date(2023, 12, 1, 10, 0, 0, 0, time.UTC)
	next = schedule.Start.Next(now)
	expected = time.Date(2023, 12, 2, 9, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("Expected next time %v, got %v", expected, next)
	}
}

func TestDuration_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name        string
		duration    string
		expected    time.Duration
		expectError bool
	}{
		{
			name:        "seconds",
			duration:    "30s",
			expected:    30 * time.Second,
			expectError: false,
		},
		{
			name:        "minutes",
			duration:    "5m",
			expected:    5 * time.Minute,
			expectError: false,
		},
		{
			name:        "hours",
			duration:    "8h",
			expected:    8 * time.Hour,
			expectError: false,
		},
		{
			name:        "complex duration",
			duration:    "1h30m45s",
			expected:    1*time.Hour + 30*time.Minute + 45*time.Second,
			expectError: false,
		},
		{
			name:        "nanoseconds",
			duration:    "500ns",
			expected:    500 * time.Nanosecond,
			expectError: false,
		},
		{
			name:        "microseconds",
			duration:    "100us",
			expected:    100 * time.Microsecond,
			expectError: false,
		},
		{
			name:        "milliseconds",
			duration:    "250ms",
			expected:    250 * time.Millisecond,
			expectError: false,
		},
		{
			name:        "very small duration",
			duration:    "1ns",
			expected:    1 * time.Nanosecond,
			expectError: false,
		},
		{
			name:        "invalid duration",
			duration:    "invalid",
			expectError: true,
		},
		{
			name:        "empty duration",
			duration:    "",
			expectError: true,
		},
		{
			name:        "invalid unit",
			duration:    "30x",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configYAML := `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "` + tt.duration + `"
    ec2:
      instance_id: "i-1234567890abcdef0"
`

			config, err := Load(strings.NewReader(configYAML))
			if tt.expectError {
				if err == nil {
					t.Fatal("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}

			if config.Instances[0].Timeout.Duration != tt.expected {
				t.Errorf("Expected duration %v, got %v", tt.expected, config.Instances[0].Timeout.Duration)
			}
		})
	}
}

func TestServiceType_UnmarshalYAML(t *testing.T) {
	// Test that valid service types work in unmarshaling
	configYAML := `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: "i-1234567890abcdef0"
    services:
      - name: "web"
        type: "http"
        http:
          service_port: 8080
          proxy_port: 8080
`

	config, err := Load(strings.NewReader(configYAML))
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if config.Instances[0].Services[0].Type != ServiceTypeHTTP {
		t.Errorf("Expected service type %v, got %v", ServiceTypeHTTP, config.Instances[0].Services[0].Type)
	}
}

func TestInstanceType_UnmarshalYAML(t *testing.T) {
	// Test that valid instance types work in unmarshaling
	configYAML := `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: "i-1234567890abcdef0"
`

	config, err := Load(strings.NewReader(configYAML))
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if config.Instances[0].Type != InstanceTypeEC2 {
		t.Errorf("Expected instance type %v, got %v", InstanceTypeEC2, config.Instances[0].Type)
	}
}

func TestLoadFile_Success(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "config.yaml")

	validYAML := `
instances:
  - name: "test-server"
    type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: "i-test123"
`

	if err := os.WriteFile(tmpFile, []byte(validYAML), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	config, err := LoadFile(tmpFile)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(config.Instances) != 1 {
		t.Errorf("Expected 1 instance, got %d", len(config.Instances))
	}

	if config.Instances[0].Name != "test-server" {
		t.Errorf("Expected instance name 'test-server', got %q", config.Instances[0].Name)
	}
}

func TestLoadFile_NonExistentFile(t *testing.T) {
	_, err := LoadFile("/non/existent/path/config.yaml")
	if err == nil {
		t.Fatal("Expected error for non-existent file, got nil")
	}
	if !strings.Contains(err.Error(), "failed to open config file") {
		t.Errorf("Expected error about opening file, got %v", err)
	}
}

func TestLoadFile_InvalidYAMLFile(t *testing.T) {
	// Create a temporary file with invalid YAML
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "invalid.yaml")

	invalidYAML := `
instances:
  - name: "test-server"
    invalid_yaml: [[[
`

	if err := os.WriteFile(tmpFile, []byte(invalidYAML), 0644); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	_, err := LoadFile(tmpFile)
	if err == nil {
		t.Fatal("Expected error for invalid YAML file, got nil")
	}
}

func TestConstants(t *testing.T) {
	// Test that constants have expected values
	if ServiceTypeHTTP != "http" {
		t.Errorf("Expected ServiceTypeHTTP to be 'http', got %q", ServiceTypeHTTP)
	}

	if InstanceTypeEC2 != "ec2" {
		t.Errorf("Expected InstanceTypeEC2 to be 'ec2', got %q", InstanceTypeEC2)
	}
}

func TestLoad_PortBoundaries(t *testing.T) {
	tests := []struct {
		name        string
		port        int
		expectError bool
	}{
		{
			name:        "port 1 (minimum valid)",
			port:        1,
			expectError: false,
		},
		{
			name:        "port 65535 (maximum valid)",
			port:        65535,
			expectError: false,
		},
		{
			name:        "port 8080 (common)",
			port:        8080,
			expectError: false,
		},
		{
			name:        "port 0 (invalid)",
			port:        0,
			expectError: true,
		},
		{
			name:        "port 65536 (invalid)",
			port:        65536,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configYAML := fmt.Sprintf(`
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: "i-1234567890abcdef0"
    services:
      - name: "web"
        type: "http"
        http:
          service_port: %d
          proxy_port: %d
`, tt.port, tt.port)

			_, err := Load(strings.NewReader(configYAML))
			if tt.expectError && err == nil {
				t.Fatal("Expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
		})
	}
}

func TestLoad_MultipleSchedules(t *testing.T) {
	configYAML := `
instances:
  - name: "web-server"
    type: "ec2"
    timeout: "30s"
    ec2:
      instance_id: "i-1234567890abcdef0"
    schedules:
      - name: "morning"
        start: "0 9 * * 1-5"
        duration: "4h"
      - name: "afternoon"
        start: "0 13 * * 1-5"
        duration: "4h"
      - name: "weekend"
        start: "0 10 * * 0,6"
        duration: "6h"
`

	config, err := Load(strings.NewReader(configYAML))
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	instance := config.Instances[0]
	if len(instance.Schedules) != 3 {
		t.Fatalf("Expected 3 schedules, got %d", len(instance.Schedules))
	}

	expectedSchedules := []struct {
		name     string
		duration time.Duration
	}{
		{"morning", 4 * time.Hour},
		{"afternoon", 4 * time.Hour},
		{"weekend", 6 * time.Hour},
	}

	for i, expected := range expectedSchedules {
		schedule := instance.Schedules[i]
		if schedule.Name != expected.name {
			t.Errorf("Expected schedule %d name %q, got %q", i, expected.name, schedule.Name)
		}
		if schedule.Duration.Duration != expected.duration {
			t.Errorf("Expected schedule %d duration %v, got %v", i, expected.duration, schedule.Duration.Duration)
		}
	}
}

func TestLoad_ComplexConfiguration(t *testing.T) {
	configYAML := `
instances:
  - name: "web-cluster"
    type: "ec2"
    timeout: "2m30s"
    ec2:
      instance_id: "i-web123456789abcdef0"
    services:
      - name: "frontend"
        type: "http"
        http:
          service_port: 3000
          proxy_port: 3000
      - name: "backend"
        type: "http"
        http:
          service_port: 8080
          proxy_port: 8080
      - name: "metrics"
        type: "http"
        http:
          service_port: 9090
          proxy_port: 9090
    schedules:
      - name: "business-hours"
        start: "0 8 * * 1-5"
        duration: "10h"
      - name: "maintenance"
        start: "0 2 * * 0"
        duration: "2h"
  - name: "database"
    type: "ec2"
    timeout: "5m"
    ec2:
      instance_id: "i-db123456789abcdef0"
    services:
      - name: "postgres"
        type: "http"
        http:
          service_port: 5432
          proxy_port: 5432
    schedules:
      - name: "always-on"
        start: "0 0 * * *"
        duration: "24h"
`

	config, err := Load(strings.NewReader(configYAML))
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(config.Instances) != 2 {
		t.Fatalf("Expected 2 instances, got %d", len(config.Instances))
	}

	// Test first instance (web-cluster)
	webInstance := config.Instances[0]
	if webInstance.Name != "web-cluster" {
		t.Errorf("Expected first instance name 'web-cluster', got %q", webInstance.Name)
	}
	if webInstance.Timeout.Duration != 2*time.Minute+30*time.Second {
		t.Errorf("Expected timeout 2m30s, got %v", webInstance.Timeout.Duration)
	}
	if len(webInstance.Services) != 3 {
		t.Errorf("Expected 3 services in web-cluster, got %d", len(webInstance.Services))
	}
	if len(webInstance.Schedules) != 2 {
		t.Errorf("Expected 2 schedules in web-cluster, got %d", len(webInstance.Schedules))
	}

	// Test second instance (database)
	dbInstance := config.Instances[1]
	if dbInstance.Name != "database" {
		t.Errorf("Expected second instance name 'database', got %q", dbInstance.Name)
	}
	if dbInstance.Timeout.Duration != 5*time.Minute {
		t.Errorf("Expected timeout 5m, got %v", dbInstance.Timeout.Duration)
	}
	if len(dbInstance.Services) != 1 {
		t.Errorf("Expected 1 service in database, got %d", len(dbInstance.Services))
	}
	if len(dbInstance.Schedules) != 1 {
		t.Errorf("Expected 1 schedule in database, got %d", len(dbInstance.Schedules))
	}

	// Verify specific service ports
	expectedPorts := []int{3000, 8080, 9090}
	for i, expectedPort := range expectedPorts {
		if webInstance.Services[i].HTTP.ServicePort != expectedPort {
			t.Errorf("Expected service %d service port %d, got %d", i, expectedPort, webInstance.Services[i].HTTP.ServicePort)
		}
		if webInstance.Services[i].HTTP.ProxyPort != expectedPort {
			t.Errorf("Expected service %d proxy port %d, got %d", i, expectedPort, webInstance.Services[i].HTTP.ProxyPort)
		}
	}

	// Verify database service port
	if dbInstance.Services[0].HTTP.ServicePort != 5432 {
		t.Errorf("Expected database service port 5432, got %d", dbInstance.Services[0].HTTP.ServicePort)
	}
	if dbInstance.Services[0].HTTP.ProxyPort != 5432 {
		t.Errorf("Expected database proxy port 5432, got %d", dbInstance.Services[0].HTTP.ProxyPort)
	}
}
