# Zipnap

Zipnap is a smart cloud resource management tool that automatically starts and
stops various types of instances based on demand. It acts as a proxy that wakes
up sleeping instances when they're needed and puts them back to sleep when idle,
helping you save on cloud costs while maintaining availability.

## Features

- **Smart Wake-Up**: Automatically starts instances when requests arrive
- **HTTP Proxy**: Routes HTTP traffic to target instances with health checks
- **Schedule-Based Activation**: Wake up instances based on cron schedules
- **Idle Shutdown**: Automatically stops instances after periods of inactivity
- **Health Monitoring**: Continuous health checks with configurable endpoints
- **Waiting Pages**: Shows user-friendly waiting pages while instances start up
- **Multiple Activators**: Support for HTTP proxy, TCP proxy, and scheduled activations

## How It Works

1. **Request Arrives**: A request comes in through one of the configured activators (HTTP proxy, schedule, etc.)
2. **Wake Up**: If the target instance is stopped, Zipnap starts it automatically
3. **Health Check**: Monitors the instance until it's healthy and ready to serve traffic
4. **Proxy Traffic**: Routes requests to the healthy instance
5. **Idle Detection**: Tracks activity and stops the instance after a configured timeout period
6. **Cost Savings**: Instance only runs when needed, reducing cloud costs

## Installation

```bash
go install github.com/mathspace/zipnap@latest
```

Or build from source:

```bash
git clone https://github.com/mathspace/zipnap.git
cd zipnap
go build -o zipnap
```

## Configuration

Create a `zipnap.yaml` configuration file:

```yaml
instances:
  my-app:
    timeout: 5m  # Stop instance after 5 minutes of inactivity
    ec2:
      instance_id: "i-1234567890abcdef0"
    activators:
      web:
        httpproxy:
          proxy_host: "0.0.0.0"
          proxy_port: 8080
          host_port: 3000
          show_waiting_page_after: 3s
          health_check:
            path: "/health"
            interval: 5s
            status_codes: [200]
      scheduler:
        schedule:
          cron: "0 9 * * 1-5"  # Wake up at 9 AM on weekdays
          keep_awake: 1h # Keep awake for 1 hour after waking up
```

## Usage

Run Zipnap with your configuration:

```bash
zipnap -config zipnap.yaml
```

## Activator Types

### HTTP Proxy
Routes HTTP requests to your instance and shows waiting pages during startup:
- Configurable health checks
- Custom waiting pages
- SSE-based ready notifications

### Schedule
Wakes up instances based on cron expressions:
- Standard cron syntax
- Multiple schedules per instance
- Timezone support

### TCP Proxy (Coming Soon)
Routes TCP traffic to instances.

## Example Use Cases

- **Development Environments**: Automatically start/stop development instances
- **Batch Processing**: Wake up compute instances for scheduled jobs
- **Cost Optimization**: Reduce costs for infrequently used applications
- **Demo Applications**: Keep demo apps available on-demand without 24/7 costs
