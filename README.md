# IBM MQ Metrics Exporter for Prometheus

A Prometheus exporter for IBM MQ queue managers, written in C++. Collects metrics from queue managers running on any platform (Windows, Linux, AIX, z/OS, IBM MQ Appliance) via binding mode or client connections.

## Features

- **Queue metrics**: depth, high watermark, enqueue/dequeue counts, open handles
- **Channel status**: running/stopped state, messages transferred, bytes sent/received
- **Topic and subscription status**: publisher/subscriber counts, durable subscriptions
- **Queue manager status**: operational state, connection count, CHINIT status
- **Cluster status**: cluster queue manager state and type
- **z/OS metrics**: buffer pool and page set utilization
- **MQI statistics**: per-application operation counts from statistics/accounting messages
- **Reconnection**: survives queue manager restarts, reports `qmgr_status=0` while disconnected
- **Dynamic discovery**: periodically rediscovers queues and channels matching patterns
- **Grafana dashboards**: 7 pre-built dashboards for channels, queues, topics, QM status, z/OS, and overview
- **Cross-platform**: builds on Windows, Linux, macOS; connects to MQ on any supported platform
- **Container-ready**: multi-stage Dockerfile included

## Quick Start

### Prerequisites

- CMake 3.20+
- C++20 compiler (GCC 11+, Clang 14+, MSVC 2022+)
- IBM MQ client libraries (optional; stub mode available for building without MQ)

### Build

```bash
# With real IBM MQ client libraries
cmake -S . -B build
cmake --build build

# Without IBM MQ (stub mode for development/testing)
cmake -S . -B build -DIBMMQ_EXPORTER_USE_STUB_MQ=ON
cmake --build build
```

### Run

```bash
# Generate a sample configuration file
./build/ibmmq-exporter config generate > config.yaml

# Edit config.yaml with your queue manager details, then:
./build/ibmmq-exporter -c config.yaml --continuous

# Metrics available at http://localhost:9091/metrics
```

### Docker

```bash
# Build and run with Docker Compose
docker compose up -d

# With monitoring stack (Prometheus + Grafana)
docker compose --profile monitoring up -d
```

## Configuration

Configuration is via YAML file with environment variable overrides.

### Environment Variables

| Variable | Description |
|---|---|
| `IBMMQ_QUEUE_MANAGER` | Queue manager name |
| `IBMMQ_CHANNEL` | Channel name |
| `IBMMQ_CONNECTION_NAME` | Connection name (`host(port)`) |
| `IBMMQ_USER` | Authentication user |
| `IBMMQ_PASSWORD` | Authentication password |

### Configuration File

Generate a full configuration template:

```bash
./build/ibmmq-exporter config generate
```

See `configs/config.collector.yaml` for a fully documented configuration file with all available options.

Key configuration sections:

- **mq**: Connection parameters (queue manager, channel, host, port, TLS)
- **collector**: Collection behavior (interval, patterns, status polling, reconnection)
- **prometheus**: HTTP server (port, path, namespace, TLS)
- **metadata**: Extra labels added to every metric
- **logging**: Log level, format, output file

### Connection Modes

**Client mode** (remote queue manager):
```yaml
mq:
  queue_manager: "QM1"
  channel: "APP.SVRCONN"
  host: "mq-server.example.com"
  port: 1414
```

**Binding mode** (local queue manager):
```yaml
mq:
  queue_manager: "QM1"
  # Leave channel, host, and port empty for binding mode
```

### Monitored Object Patterns

```yaml
collector:
  monitored_queues:
    - "APP.*"        # All queues starting with APP.
    - "!SYSTEM.*"    # Exclude system queues
  monitored_channels:
    - "*"            # All channels
  monitored_topics:
    - "#"            # All topics
```

### Resource Monitoring (CPU/Memory/Disk Metrics)

To collect CPU, memory, and disk utilization metrics from your queue manager, resource monitoring must be enabled:

```bash
# On the queue manager, enable resource monitoring:
ALTER QMGR MONQ(MEDIUM) MONCHL(MEDIUM)

# Then restart the queue manager
```

**Note:** This is different from `STATMQI(ON)` and `STATQ(ON)` which enable statistics messages to admin queues. Both are independent features.

Once enabled, the exporter will discover and collect:
- CPU utilization metrics (`ibmmq_qmgr_cpu_*`)
- Queue manager memory metrics (`ibmmq_qmgr_memory_*`)
- Filesystem usage metrics (`ibmmq_qmgr_disk_*`)
- Per-queue publication metrics from STATQ class

To verify resource monitoring is working:
1. Look for log messages: `"Found monitor class 'CPU'"`, `"Found monitor class 'DISK'"`
2. Check metrics output for `ibmmq_qmgr_cpu_*` and related metrics
3. If discovery fails, run the `ALTER QMGR` command above and restart the queue manager

## Metrics

All metrics use the configured namespace (default: `ibmmq`).

### Queue Metrics

| Metric | Labels | Description |
|---|---|---|
| `ibmmq_queue_depth` | qmgr, platform, queue | Current queue depth |
| `ibmmq_queue_high_depth` | qmgr, platform, queue | High watermark depth |
| `ibmmq_queue_enqueue_count` | qmgr, platform, queue | Messages enqueued |
| `ibmmq_queue_dequeue_count` | qmgr, platform, queue | Messages dequeued |
| `ibmmq_queue_input_handles` | qmgr, platform, queue | Open input handles |
| `ibmmq_queue_output_handles` | qmgr, platform, queue | Open output handles |

### Channel Status Metrics

| Metric | Labels | Description |
|---|---|---|
| `ibmmq_channel_status` | qmgr, platform, channel, type, connname, rqmname | Channel status code |
| `ibmmq_channel_status_msgs` | qmgr, platform, channel, type | Messages transferred |
| `ibmmq_channel_bytes_sent` | qmgr, platform, channel, type | Bytes sent |
| `ibmmq_channel_bytes_received` | qmgr, platform, channel, type | Bytes received |

### Queue Manager Metrics

| Metric | Labels | Description |
|---|---|---|
| `ibmmq_qmgr_status` | qmgr, platform | Queue manager status (1=running) |
| `ibmmq_qmgr_connection_count` | qmgr, platform | Active connections |
| `ibmmq_qmgr_chinit_status` | qmgr, platform | Channel initiator status |

### Topic and Subscription Metrics

| Metric | Labels | Description |
|---|---|---|
| `ibmmq_topic_pub_count` | qmgr, platform, topic | Publisher count |
| `ibmmq_topic_sub_count` | qmgr, platform, topic | Subscriber count |
| `ibmmq_sub_durable` | qmgr, platform, subscription, topic | Durable flag |
| `ibmmq_sub_type` | qmgr, platform, subscription, topic | Subscription type |

### z/OS Metrics

| Metric | Labels | Description |
|---|---|---|
| `ibmmq_usage_bp_free_buffers` | qmgr, platform, bufferpool | Free buffers |
| `ibmmq_usage_bp_total_buffers` | qmgr, platform, bufferpool | Total buffers |
| `ibmmq_usage_ps_total_pages` | qmgr, platform, pageset | Total pages |
| `ibmmq_usage_ps_unused_pages` | qmgr, platform, pageset | Unused pages |

## Grafana Dashboards

Pre-built dashboards are in the `dashboards/` directory:

| Dashboard | Description |
|---|---|
| `MQ_Prometheus_Overview.json` | Overview: queue activity, CPU, log latency, channels |
| `Queue_Status.json` | Per-queue depth, message rates, response times |
| `Channel_Status.json` | Channel status, bytes, messages, network timing |
| `Queue_Manager_Status.json` | QM status, connections, cluster, Native HA |
| `Topic_Status.json` | Topic and subscription monitoring |
| `zOS_Status.json` | z/OS buffer pools, page sets |
| `Logging.json` | HA replica and CRR recovery metrics |

Import via Grafana UI: Dashboards > Import > Upload JSON file.

## MQ Service Integration

To run as an MQ service (auto-start/stop with queue manager):

```bash
# Install binary
sudo cp build/ibmmq-exporter /usr/local/bin/ibmmq-exporter/
sudo cp scripts/ibmmq_exporter.sh /usr/local/bin/ibmmq-exporter/
sudo cp configs/config.collector.yaml /usr/local/bin/ibmmq-exporter/config.yaml

# Define MQ service
runmqsc QM1 < scripts/ibmmq_exporter.mqsc
```

## Build Options

| CMake Option | Default | Description |
|---|---|---|
| `IBMMQ_EXPORTER_USE_STUB_MQ` | OFF | Build without real MQ libraries |
| `IBMMQ_EXPORTER_ENABLE_TLS` | OFF | Enable TLS for metrics endpoint (requires OpenSSL) |
| `MQ_HOME` | auto-detect | Path to IBM MQ installation |

## CLI Reference

```
ibmmq-exporter [OPTIONS] [SUBCOMMAND]

Options:
  -c, --config       Configuration file path
  -v, --verbose      Enable verbose logging
  --log-level        Log level (debug, info, warn, error)
  --log-format       Log format (json, text)
  --continuous       Run continuous monitoring
  --interval         Collection interval in seconds
  --max-cycles       Maximum collection cycles (0=infinite)
  --reset-stats      Reset statistics after reading
  --prometheus-port  Prometheus HTTP port

Subcommands:
  version            Print version information
  test               Test IBM MQ connection
  config generate    Generate sample configuration
  config validate    Validate configuration file
```

## License

See [LICENSE](LICENSE) for details.
