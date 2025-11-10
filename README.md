# IBM MQ Statistics and Accounting Collector

[![Go Report Card](https://goreportcard.com/badge/github.com/atulksin/ibmmq-go-stat-otel)](https://goreportcard.com/report/github.com/atulksin/ibmmq-go-stat-otel)
[![License](https://img.shields.io/github/license/atulksin/ibmmq-go-stat-otel)](LICENSE)

A high-performance Go application that collects IBM MQ statistics and accounting data from IBM MQ queue managers and exposes them as Prometheus metrics with OpenTelemetry observability.

## Features

🚀 **High Performance**: Built in Go with efficient IBM MQ client integration  
📊 **Prometheus Metrics**: Exposes all IBM MQ stats as Prometheus gauges with `ibmmq` prefix  
🔍 **Reader/Writer Detection**: Identifies applications that read from or write to queues  
� **OpenTelemetry**: Full observability with distributed tracing  
⚙️ **Flexible Configuration**: YAML configuration with dynamic connection building  
🐳 **Docker Ready**: Multi-stage Docker builds with BuildKit optimization  
🔄 **Multiple Modes**: One-time collection or continuous monitoring  
📈 **Rich Metrics**: Statistics and accounting data from IBM MQ queues  
🛡️ **Robust**: Comprehensive error handling and logging  
🧪 **Testing Tools**: Includes scripts to generate test activity on multiple platforms

## Prerequisites

- IBM MQ Client libraries installed
- Go 1.21 or higher
- Access to IBM MQ Queue Manager
- Appropriate permissions to read statistics and accounting queues

## Installation

### From Source

```bash
git clone https://github.com/atulksin/ibmmq-go-stat-otel.git
cd ibmmq-go-stat-otel
go build -o ibmmq-collector ./cmd/collector
```

### Using Go Install

```bash
go install github.com/atulksin/ibmmq-go-stat-otel/cmd/collector@latest
```

## Quick Start

1. **Generate Configuration File**:
   ```bash
   ./ibmmq-collector config generate > config.yaml
   ```

2. **Edit Configuration** to match your IBM MQ environment:
   ```yaml
   mq:
     queue_manager: "MQQM1"
     channel: "APP1.SVRCONN"
     connection_name: "localhost(1414)"
     user: ""
     password: ""
   ```

3. **Test Connection**:
   ```bash
   ./ibmmq-collector test -c config.yaml
   ```

4. **Start Collector**:
   ```bash
   ./ibmmq-collector -c config.yaml --continuous
   ```

5. **View Metrics**:
   ```bash
   curl http://localhost:9090/metrics
   ```

## Configuration

### Configuration File (config.yaml)

```yaml
mq:
  queue_manager: "MQQM1"
  channel: "APP1.SVRCONN"
  connection_name: "localhost(1414)"
  user: ""
  password: ""
  key_repository: ""  # SSL/TLS key repository
  cipher_spec: ""     # SSL/TLS cipher spec

collector:
  stats_queue: "SYSTEM.ADMIN.STATISTICS.QUEUE"
  accounting_queue: "SYSTEM.ADMIN.ACCOUNTING.QUEUE"
  reset_stats: false
  interval: "60s"
  max_cycles: 0  # 0 = infinite
  continuous: false

prometheus:
  port: 9090
  path: "/metrics"
  namespace: "ibmmq"
  subsystem: ""
  enable_otel: true

logging:
  level: "info"
  format: "json"
  output_file: ""
  verbose: false
```

### Environment Variables

All configuration can be set via environment variables with the `IBMMQ_` prefix:

```bash
export IBMMQ_QUEUE_MANAGER="MQQM1"
export IBMMQ_CHANNEL="APP1.SVRCONN"
export IBMMQ_CONNECTION_NAME="localhost(1414)"
export IBMMQ_USER="mquser"
export IBMMQ_PASSWORD="mqpass"
```

### Command Line Flags

```bash
# Basic usage
./ibmmq-collector --config config.yaml

# Continuous monitoring
./ibmmq-collector --continuous --interval 30s

# Custom Prometheus port
./ibmmq-collector --prometheus-port 8080

# Verbose logging
./ibmmq-collector --verbose --log-level debug

# Limited cycles
./ibmmq-collector --continuous --max-cycles 100
```

## Usage Examples

### One-time Collection

```bash
./ibmmq-collector -c config.yaml
```

### Continuous Monitoring

```bash
./ibmmq-collector -c config.yaml --continuous --interval 60s
```

### Production Monitoring with Custom Settings

```bash
./ibmmq-collector \
  --config /etc/ibmmq-collector/config.yaml \
  --continuous \
  --interval 30s \
  --prometheus-port 9090 \
  --log-level info \
  --log-format json
```

### Using Environment Variables Only

```bash
export IBMMQ_QUEUE_MANAGER="PROD_QM"
export IBMMQ_CHANNEL="PROD.SVRCONN"
export IBMMQ_CONNECTION_NAME="mq.company.com(1414)"
export IBMMQ_USER="collector"
export IBMMQ_PASSWORD="secret"

./ibmmq-collector --continuous --interval 60s
```

## Prometheus Metrics

The collector exposes the following metrics with the `ibmmq` namespace:

### Queue Metrics

- `ibmmq_queue_depth_current` - Current depth of IBM MQ queue
- `ibmmq_queue_depth_high` - High water mark of IBM MQ queue depth
- `ibmmq_queue_enqueue_count` - Total number of messages enqueued to IBM MQ queue
- `ibmmq_queue_dequeue_count` - Total number of messages dequeued from IBM MQ queue
- `ibmmq_queue_input_handles` - Number of input handles open for IBM MQ queue
- `ibmmq_queue_output_handles` - Number of output handles open for IBM MQ queue
- `ibmmq_queue_has_readers` - Whether IBM MQ queue has active readers (1=yes, 0=no)
- `ibmmq_queue_has_writers` - Whether IBM MQ queue has active writers (1=yes, 0=no)

### Channel Metrics

- `ibmmq_channel_messages_total` - Total number of messages sent through IBM MQ channel
- `ibmmq_channel_bytes_total` - Total number of bytes sent through IBM MQ channel
- `ibmmq_channel_batches_total` - Total number of batches sent through IBM MQ channel

### MQI Operation Metrics

- `ibmmq_mqi_opens_total` - Total number of MQI OPEN operations
- `ibmmq_mqi_closes_total` - Total number of MQI CLOSE operations
- `ibmmq_mqi_puts_total` - Total number of MQI PUT operations
- `ibmmq_mqi_gets_total` - Total number of MQI GET operations
- `ibmmq_mqi_commits_total` - Total number of MQI COMMIT operations
- `ibmmq_mqi_backouts_total` - Total number of MQI BACKOUT operations

### Collection Metadata

- `ibmmq_collection_info` - Information about the collection process
- `ibmmq_last_collection_timestamp` - Timestamp of the last successful collection

### Metric Labels

All metrics include relevant labels:

- `queue_manager` - IBM MQ Queue Manager name
- `queue_name` - Queue name (for queue metrics)
- `channel_name` - Channel name (for channel metrics)
- `connection_name` - Connection name (for channel metrics)
- `application_name` - Application name (for MQI metrics)

## Prometheus Configuration

Add the following to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'ibmmq-collector'
    static_configs:
      - targets: ['localhost:9090']
    metrics_path: '/metrics'
    scrape_interval: 30s
```

## Grafana Dashboard

Example Grafana queries:

### Queue Depth Over Time
```promql
ibmmq_queue_depth_current{queue_manager="MQQM1"}
```

### Message Rate
```promql
rate(ibmmq_queue_enqueue_count[5m])
```

### Queue Activity
```promql
ibmmq_queue_has_readers + ibmmq_queue_has_writers
```

## Docker

### Building with Docker

The repository includes a configurable multi-stage Dockerfile optimized for production use with IBM MQ client libraries:

#### Quick Build (Downloads MQ Client)
```bash
# Build with BuildKit for optimal caching and parallel builds
export DOCKER_BUILDKIT=1
docker build -t ibmmq-collector .
```

#### Configurable Build with Local IBM MQ

The Dockerfile supports configurable IBM MQ paths for different platforms:

**Windows (PowerShell):**
```powershell
# Use local IBM MQ installation
.\scripts\docker-build.ps1 -LocalMQ

# Custom paths
.\scripts\docker-build.ps1 -LocalMQ -IncludePath "C:\IBM\MQ\tools\c\include" -LibPath "C:\IBM\MQ\bin64"
```

**Linux/Unix (Bash):**
```bash
# Use local IBM MQ installation
./scripts/docker-build.sh --local-mq

# Custom paths
./scripts/docker-build.sh --local-mq --include-path "/opt/mqm/inc" --lib-path "/opt/mqm/lib64"
```

**Manual Build Arguments:**
```bash
docker build \
  --build-arg USE_LOCAL_MQ=true \
  --build-arg MQ_INCLUDE_PATH="/opt/mqm/inc" \
  --build-arg MQ_LIB_PATH="/opt/mqm/lib64" \
  -t ibmmq-collector .
```

#### Dockerfile Features:
- 🏗️ **Multi-stage build** for minimal final image size
- ⚙️ **Configurable IBM MQ paths** supporting Windows/Linux installations
- 🔧 **64-bit GCC compilation** with proper library linking
- 🚀 **BuildKit optimization** with mount caches for faster builds
- 🧪 **Parallel test execution** during build process
- 🛡️ **Security hardening** with non-root user
- 📦 **Minimal runtime** based on Debian Bullseye slim
- 🔄 **Fallback mechanisms** for missing MQ installations

### Docker Compose

```yaml
version: '3.8'
services:
  ibmmq-collector:
    build: .
    ports:
      - "9090:9090"
    environment:
      - IBMMQ_QUEUE_MANAGER=MQQM1
      - IBMMQ_CHANNEL=APP1.SVRCONN
      - IBMMQ_CONNECTION_NAME=mq:1414
      - IBMMQ_USER=mquser
      - IBMMQ_PASSWORD=mqpass
    command: ["./ibmmq-collector", "--continuous", "--interval", "60s"]

  prometheus:
    image: prom/prometheus
    ports:
      - "9091:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
```

## Building from Source

### Prerequisites

1. **Install Go 1.21+**
2. **Install IBM MQ Client Libraries**:
   - Download IBM MQ client from IBM website
   - Install to standard location (e.g., `/opt/mqm` on Linux)
   - Set environment variables:
     ```bash
     export MQ_INSTALLATION_PATH=/opt/mqm
     export CGO_CFLAGS=-I$MQ_INSTALLATION_PATH/inc
     export CGO_LDFLAGS_ALLOW="-Wl,-rpath.*"
     ```

### Build Steps

```bash
# Clone repository
git clone https://github.com/atulksin/ibmmq-go-stat-otel.git
cd ibmmq-go-stat-otel

# Download dependencies
go mod download

# Build
go build -o ibmmq-collector ./cmd/collector

# Run tests (requires IBM MQ libraries)
go test ./...

# Build for different platforms
GOOS=linux GOARCH=amd64 go build -o ibmmq-collector-linux ./cmd/collector
GOOS=windows GOARCH=amd64 go build -o ibmmq-collector.exe ./cmd/collector
```

## Testing

### Unit Tests

```bash
go test ./pkg/config -v
go test ./pkg/pcf -v
go test ./pkg/collector -v
```

### Integration Tests

```bash
# Requires running IBM MQ instance
go test ./... -tags=integration
```

### Connection Test

```bash
./ibmmq-collector test -c config.yaml
```

### Test Activity Generation

The repository includes cross-platform scripts to generate IBM MQ activity for testing:

#### Windows (PowerShell)
```powershell
# Generate activity with default settings (50 messages per queue)
.\sample-runs\generate-test-activity.ps1

# Custom message count and queues
.\sample-runs\generate-test-activity.ps1 -MessageCount 100 -Queues @("QUEUE1", "QUEUE2")
```

#### Linux/Unix (Bash)
```bash
# Make script executable
chmod +x sample-runs/generate-test-activity.sh

# Source IBM MQ environment (if needed)
source /opt/mqm/bin/setmqenv -s

# Generate activity
./sample-runs/generate-test-activity.sh -m 100 -Q "QUEUE1 QUEUE2"
```

These scripts:
- ✅ Create **writer activity** using `amqsput` (generates opprocs statistics)
- ✅ Create **reader activity** using `amqsget` (generates ipprocs statistics)  
- ✅ Display queue status and depths
- ✅ Work with any IBM MQ installation

After running the scripts, use the collector and PCF dumper to verify detection:
```bash
# Analyze raw PCF data
./pcf-dumper.exe -c configs/default.yaml

# Collect metrics
./collector.exe test -c configs/default.yaml
```

## IBM MQ Setup

### Enable Statistics

```mqsc
ALTER QMGR STATQ(ON) STATCHL(ON) STATACLS(ON)
ALTER QLOCAL('YOUR.QUEUE') STATQ(ON)
```

### Enable Accounting

```mqsc
ALTER QMGR ACCTQ(ON) ACCTCONO(ENABLED) ACCTMQI(ON)
```

### Create User and Permissions

```mqsc
# Create user
DEFINE CHANNEL(COLLECTOR.SVRCONN) CHLTYPE(SVRCONN)
SET CHLAUTH(COLLECTOR.SVRCONN) TYPE(ADDRESSMAP) ADDRESS('*') USERSRC(CHANNEL) CHCKCLNT(NONE)

# Grant permissions
SET AUTHREC PROFILE('SYSTEM.ADMIN.STATISTICS.QUEUE') OBJTYPE(QUEUE) PRINCIPAL('mqcollector') AUTHADD(GET,BROWSE)
SET AUTHREC PROFILE('SYSTEM.ADMIN.ACCOUNTING.QUEUE') OBJTYPE(QUEUE) PRINCIPAL('mqcollector') AUTHADD(GET,BROWSE)
```

## Project Structure

```
ibmmq-go-stat-otel/
├── cmd/
│   ├── collector/          # Main statistics collector
│   │   └── main.go
│   └── pcf-dumper/         # PCF data analysis tool
│       └── main.go
├── pkg/
│   ├── config/            # Configuration management with YAML loading
│   │   ├── config.go
│   │   └── config_test.go
│   ├── mqclient/          # IBM MQ client wrapper
│   │   ├── client.go
│   │   └── client_test.go
│   ├── pcf/               # PCF message parser and decoder
│   │   ├── parser.go
│   │   └── parser_test.go
│   ├── collector/         # Main collector logic
│   │   ├── collector.go
│   │   └── collector_test.go
│   └── prometheus/        # Prometheus metrics integration
│       ├── collector.go
│       └── collector_test.go
├── internal/
│   └── otel/              # OpenTelemetry integration
│       └── provider.go
├── configs/               # Configuration files
│   └── default.yaml       # Default configuration template
├── sample-runs/           # Testing and validation scripts
│   ├── generate-test-activity.ps1  # Windows PowerShell script
│   ├── generate-test-activity.sh   # Linux/Unix bash script
│   └── README.md          # Testing documentation
├── docs/                  # Project documentation
├── test/                  # Integration tests
├── scripts/               # Build and utility scripts
├── Dockerfile             # Multi-stage Docker build with IBM MQ client
├── docker-compose.yml     # Docker Compose configuration
├── go.mod
├── go.sum
└── README.md
```

## API Reference

### Command Line Interface

```
Usage:
  ibmmq-collector [flags]
  ibmmq-collector [command]

Available Commands:
  config      Configuration management commands
  help        Help about any command
  test        Test IBM MQ connection and configuration
  version     Print version information

Flags:
  -c, --config string         Configuration file path
      --continuous            Run continuous monitoring
  -h, --help                  help for ibmmq-collector
      --interval duration     Collection interval for continuous mode (default 1m0s)
      --log-format string     Log format (json, text) (default "json")
      --log-level string      Log level (debug, info, warn, error) (default "info")
      --max-cycles int        Maximum number of collection cycles (0 = infinite)
      --otel                  Enable OpenTelemetry integration (default true)
      --prometheus-port int   Prometheus metrics HTTP server port (default 9090)
      --reset-stats           Reset statistics after reading
  -v, --verbose               Enable verbose logging
      --version               version for ibmmq-collector
```

### Configuration Commands

```bash
# Generate sample configuration
./ibmmq-collector config generate

# Validate configuration
./ibmmq-collector config validate -c config.yaml
```

### Test Commands

```bash
# Test connection
./ibmmq-collector test -c config.yaml
```

## Monitoring and Alerting

### Prometheus Alerting Rules

```yaml
groups:
  - name: ibmmq
    rules:
      - alert: IBMMQHighQueueDepth
        expr: ibmmq_queue_depth_current > 1000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High queue depth on {{ $labels.queue_name }}"
          
      - alert: IBMMQCollectorDown
        expr: up{job="ibmmq-collector"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "IBM MQ collector is down"
```

## Performance Considerations

- **Collection Interval**: Adjust based on your monitoring needs and MQ load
- **Message Buffering**: The collector processes messages in batches for efficiency
- **Memory Usage**: Monitor memory usage with high-volume message queues
- **Network**: Consider network latency between collector and MQ server

## Troubleshooting

### Common Issues

1. **Connection Failed**
   - Check queue manager name, channel, and network connectivity
   - Verify IBM MQ client libraries are installed
   - Check user permissions

2. **Permission Denied**
   - Ensure user has access to statistics and accounting queues
   - Check MQ authentication configuration

3. **No Messages**
   - Statistics/accounting may not be enabled on the queue manager
   - Check if queues have activity to generate statistics

4. **Parse Errors**
   - Some message formats may not be fully supported
   - Enable debug logging to investigate

### Debug Mode

```bash
./ibmmq-collector --verbose --log-level debug -c config.yaml
```

### Health Checks

```bash
# Check collector health
curl http://localhost:9090/health

# Check readiness
curl http://localhost:9090/ready

# Check metrics endpoint
curl http://localhost:9090/metrics
```

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Development Setup

```bash
git clone https://github.com/atulksin/ibmmq-go-stat-otel.git
cd ibmmq-go-stat-otel
go mod download
go test ./...
```

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Support

- Create an issue on GitHub for bugs or feature requests
- Check the documentation for configuration help
- Use debug logging for troubleshooting

## Changelog

See CHANGELOG.md for version history and changes.

## Related Projects

- [IBM MQ Go Client](https://github.com/ibm-messaging/mq-golang)
- [Prometheus](https://prometheus.io/)
- [OpenTelemetry Go](https://github.com/open-telemetry/opentelemetry-go)
- [Original Python Implementation](https://github.com/atulksin/ibm-mq-statnacct)