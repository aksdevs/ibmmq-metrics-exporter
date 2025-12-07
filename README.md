# IBM MQ Statistics and Accounting Collector

A high-performance Go application that collects IBM MQ statistics and accounting data from queue managers and exposes them as Prometheus metrics with OpenTelemetry observability.

## Overview

This collector gathers operational metrics from IBM MQ queue managers in real-time, including application-level MQI statistics and queue-specific operations. All metrics include queue context (queue names) where available, enabling precise identification of which operations were performed on which queues.

Key capabilities:
- Collects 41 MQI statistics parameters from IBM MQ accounting and statistics messages
- Tracks per-queue application activity (puts, gets, messages sent/received)
- Real-time queue depth monitoring via direct MQINQ API calls
- Automatic application/process detection for queues
- Prometheus metrics export with comprehensive labels
- OpenTelemetry distributed tracing support
- Flexible YAML configuration
- Both one-time and continuous collection modes

## Understanding Queue Name Labels

The collector includes `queue_name` labels in all metrics to answer: "Which queue was this operation performed on?"

### Queue Name Values

When metrics show up in Prometheus:

```
ibmmq_mqi_puts{queue_manager="MQQM1",queue_name="unknown",application_name="app1",...} 30
ibmmq_mqi_gets{queue_manager="MQQM1",queue_name="TEST.QUEUE",application_name="app1",...} 15
```

- `queue_name="unknown"`: Connection-level aggregate statistics from IBM MQ's application-level accounting
- `queue_name="TEST.QUEUE"`: Queue-specific statistics when IBM MQ provides queue-level accounting data

### Why "unknown" Appears

IBM MQ provides two levels of accounting information:

1. **Application-level (Connection-level)**: Aggregate statistics for an application/connection across all queues. These show as `queue_name="unknown"` because the individual queue operations are aggregated together.

2. **Queue-level**: Detailed breakdown of operations per queue. These show actual queue names like `queue_name="TEST.QUEUE"`.

The collector receives whichever data IBM MQ is configured to send. To get queue-level metrics with actual queue names, you must enable queue-level accounting on your queues.

## Enabling Queue-Level Accounting

### Step 1: Enable at Queue Manager Level

Connect to your queue manager and enable MQI statistics:

```bash
# On Windows/Linux with MQSC command line:
runmqsc MQQM1

ALTER QMGR STATMQI(ON) ACCTQ(ON)
```

This enables statistics collection. For queue-specific data, you also need Step 2.

Reference: IBM MQ Knowledge Center - Accounting and statistics for IBM MQ for Linux, UNIX and Windows
https://www.ibm.com/docs/en/ibm-mq/latest?topic=qm-accounting-statistics

### Step 2: Enable on Individual Queues

For each queue where you want detailed operation tracking:

```bash
# Still in runmqsc prompt:
ALTER QLOCAL(TEST.QUEUE) STATQ(ON) ACCTQ(ON)
ALTER QLOCAL(REPLY.QUEUE) STATQ(ON) ACCTQ(ON)

# View current settings:
DISPLAY QLOCAL(TEST.QUEUE) STATQ ACCTQ
```

Parameters explained:

- `STATQ(ON)`: Queue-level statistics collection enabled
- `ACCTQ(ON)`: Queue-level accounting data collection enabled
- `STATQ(OFF)`: Disables (default is OFF on many systems)
- `ACCTQ(OFF)`: Disables

Once enabled, IBM MQ will include queue-level groups in its accounting PCF messages, and the collector will automatically extract the actual queue names instead of showing "unknown".

Reference: IBM MQ Knowledge Center - ALTER QLOCAL command
https://www.ibm.com/docs/en/ibm-mq/latest?topic=reference-alter-qlocal

## Metrics Collected

### Connection-Level MQI Metrics (41 total)

These are application/connection aggregate statistics. Show as `queue_name="unknown"` unless queue-level accounting is enabled.

Reference: IBM MQ Knowledge Center - Statistics and accounting data
https://www.ibm.com/docs/en/ibm-mq/latest?topic=qm-statistics-accounting-data

**Core Operations** (6 metrics):
```
ibmmq_mqi_opens                 - MQI OPEN operations
ibmmq_mqi_closes                - MQI CLOSE operations
ibmmq_mqi_puts                  - MQI PUT operations
ibmmq_mqi_gets                  - MQI GET operations
ibmmq_mqi_commits               - MQI COMMIT operations
ibmmq_mqi_backouts              - MQI BACKOUT operations
```

**Additional Operations** (3 metrics):
```
ibmmq_mqi_browses               - MQI BROWSE operations
ibmmq_mqi_inqs                  - MQI INQUIRE operations
ibmmq_mqi_sets                  - MQI SET operations
```

**Message Counts** (5 metrics):
```
ibmmq_mqi_msgs_received         - Messages received
ibmmq_mqi_msgs_sent             - Messages sent
ibmmq_mqi_persistent_msgs       - Persistent message count
ibmmq_mqi_non_persistent_msgs   - Non-persistent message count
ibmmq_mqi_long_msgs             - Long messages
```

Plus 27 additional metrics covering timing, errors, connection states, and performance indicators.

**Label Set**: `queue_manager`, `queue_name`, `application_name`, `application_tag`, `user_identifier`, `connection_name`, `channel_name`

### Queue-Level Metrics (4 metrics)

These metrics have actual queue names when queue-level accounting is enabled:

```
ibmmq_queue_app_puts_total          - PUTs per queue per application
ibmmq_queue_app_gets_total          - GETs per queue per application
ibmmq_queue_app_msgs_received_total - Messages received per queue per application
ibmmq_queue_app_msgs_sent_total     - Messages sent per queue per application
```

**Label Set**: `queue_manager`, `queue_name`, `application_name`, `source_ip`, `user_identifier`

### Direct Queue Metrics (MQINQ API)

Real-time queue attributes via direct API queries:

```
ibmmq_queue_depth_current       - Current message count in queue
ibmmq_queue_input_handles       - Number of getters/consumers
ibmmq_queue_output_handles      - Number of putters/producers
```

**Label Set**: `queue_manager`, `queue_name`

Reference: IBM MQ Knowledge Center - MQINQ (Inquire queue object)
https://www.ibm.com/docs/en/ibm-mq/latest?topic=reference-mqinq

### Handle-Level Metrics (Application/Process Details)

Similar to the MQSC command `DIS QS(queue_name) TYPE(HANDLE) ALL`, these metrics provide application/process information for queues:

```
ibmmq_queue_handle_count   - Total number of open handles (processes) on a queue
ibmmq_queue_handle_info    - Details about open handles (app name, user, open mode)
```

**Label Sets**:
- `ibmmq_queue_handle_count`: `queue_manager`, `queue_name`, `handle_type` (input/output)
- `ibmmq_queue_handle_info`: `queue_manager`, `queue_name`, `application_name`, `user_identifier`, `open_mode`, `handle_state`

**Example Output**:
```
ibmmq_queue_handle_count{queue_manager="MQQM1",queue_name="TEST.QUEUE",handle_type="input"} 2
ibmmq_queue_handle_count{queue_manager="MQQM1",queue_name="TEST.QUEUE",handle_type="output"} 1
ibmmq_queue_handle_info{queue_manager="MQQM1",queue_name="TEST.QUEUE",application_name="amqsget",user_identifier="user1",open_mode="Input",handle_state="Open"} 1
ibmmq_queue_handle_info{queue_manager="MQQM1",queue_name="TEST.QUEUE",application_name="amqsput",user_identifier="user2",open_mode="Output",handle_state="Open"} 1
```

These metrics help you:
- Identify which applications are actively using each queue
- Monitor the number of producers and consumers per queue
- Track user IDs accessing queues for security/audit purposes
- Detect stuck or long-running processes

## How the Code is Ready for Future IBM MQ Enhancements

The implementation has forward compatibility built in:

### Parser Infrastructure

In `pkg/pcf/parser.go`, the `QueueAppOperation` struct can be extended for future fields:

```go
type QueueAppOperation struct {
    QueueName       string
    ApplicationName string
    ConnectionName  string
    UserIdentifier  string
    Puts            int32
    Gets            int32
    MsgsReceived    int32    // Recently added
    MsgsSent        int32    // Recently added
    // Future fields can be added here without breaking existing code
}
```

When IBM MQ adds new fields to queue-level accounting:
1. Add the field to the struct
2. Add extraction logic in `parseAccounting()` (around line 590)
3. Create corresponding Prometheus metrics
4. Update metric collection in `processAccountingMessage()`

Reference: IBM MQ Knowledge Center - PCF (Programmable Command Format)
https://www.ibm.com/docs/en/ibm-mq/latest?topic=reference-pcf-programmable-command-format

### Metric Label Framework

In `pkg/prometheus/collector.go`, MQI metrics use dynamic label construction:

```go
// Current: 7 labels including queue_name
mqlLabels := []string{
    "queue_manager", 
    "queue_name",           // Positioned early for future queue-specific filtering
    "application_name", 
    "application_tag", 
    "user_identifier", 
    "connection_name", 
    "channel_name",
}
```

If IBM MQ adds queue context in future versions, the label structure already supports it. The parser and collector will automatically populate actual queue names instead of "unknown".

Reference: IBM MQ Knowledge Center - MQGET (Get message)
https://www.ibm.com/docs/en/ibm-mq/latest?topic=reference-mqget

### Processing Pipeline

The accounting message processor checks both connection-level and queue-level data:

```go
// Connection-level stats (current all systems)
if ops := acct.Operations; ops != nil {
    // Uses queue_name="unknown"
}

// Queue-level stats (when available)
if acct.QueueOperations != nil {
    for _, qa := range acct.QueueOperations {
        // Uses actual queue_name from qa.QueueName
    }
}
```

This dual-path approach means as IBM MQ implementations evolve, real queue names will automatically appear when available.

Reference: IBM MQ Knowledge Center - Accounting and statistics groups
https://www.ibm.com/docs/en/ibm-mq/latest?topic=qm-what-are-accounting-statistics-groups

## Generating Test Statistics

The `test-activity` tool generates realistic IBM MQ activity to test metric collection:

### Building the Test Tool

```bash
go build -o test-activity.exe ./cmd/test-activity
```

### Running Test Activity

Basic test - 50 messages on TEST.QUEUE:

```bash
# Windows PowerShell
.\test-activity.exe -config configs/default.yaml -messages 50

# Linux/Mac
./test-activity -config configs/default.yaml -messages 50
```

### What the Test Tool Does

1. **Creates test queue** with STATQ(ON) to enable queue-level statistics
2. **Enables queue manager accounting** with ALTER QMGR STATMQI(ON) ACCTQ(ON)
3. **Enables queue-level accounting** with ALTER QLOCAL STATQ(ON) ACCTQ(ON)
4. **Sends test messages** using the IBM MQ amqsput utility
5. **Consumes messages** using amqsget to generate both PUT and GET activity

### Example Output

```
=== IBM MQ Test Activity Generator (MQSC) ===
Queue Manager: MQQM1
Connection: 127.0.0.1(5200) via APP1.SVRCONN
Test Queue: TEST.QUEUE
Message Count: 50

Step 1: Creating test queue...
5724-H72 (C) Copyright IBM Corp. 1994, 2025.
Starting MQSC for queue manager MQQM1.

     1 : DEFINE QLOCAL(TEST.QUEUE) REPLACE STATQ(ON)
AMQ8006I: IBM MQ queue created.
Queue created/verified

Step 2: Enabling MQ statistics and accounting...
ALTER QMGR STATMQI(ON)
Statistics and accounting enabled at QMgr level

Step 2b: Enabling queue-specific statistics and accounting...
5724-H72 (C) Copyright IBM Corp. 1994, 2025.

     1 : ALTER QLOCAL(TEST.QUEUE) STATQ(ON) ACCTQ(ON)
AMQ8008I: IBM MQ queue changed.
Queue-specific statistics and accounting enabled

Step 3: Sending 50 test messages using amqsput...
Test Message 1 - 2025-12-06T13:19:12.4586908-05:00
Test Message 2 - 2025-12-06T13:19:12.5078628-05:00
... (messages 3-49)
Test Message 50 - 2025-12-06T13:19:13.9243813-05:00
PUT messages completed

Step 4: Getting 15 messages to generate GET activity...
Sample AMQSGET0 start
... (reading messages)
Sample AMQSGET0 end
```

### Understanding Test Output

The test program uses native IBM MQ utilities:
- `runmqsc`: Command-line interface for MQSC commands
- `amqsput`: Standard IBM MQ message PUT utility
- `amqsget`: Standard IBM MQ message GET utility

This ensures generated statistics match real application behavior.

Reference: IBM MQ Knowledge Center - MQSC administrative interface
https://www.ibm.com/docs/en/ibm-mq/latest?topic=reference-mqsc-administrative-interface

### Viewing Results in Collector

While test is running or after completion, check collector output:

```bash
curl http://localhost:9091/metrics | grep ibmmq_mqi_puts
```

Example metrics after test:

```
ibmmq_mqi_puts{queue_manager="MQQM1",queue_name="TEST.QUEUE",application_name="amqsput",...} 50
ibmmq_mqi_gets{queue_manager="MQQM1",queue_name="TEST.QUEUE",application_name="amqsget",...} 15
ibmmq_queue_depth_current{queue_manager="MQQM1",queue_name="TEST.QUEUE"} 35
ibmmq_queue_input_handles{queue_manager="MQQM1",queue_name="TEST.QUEUE"} 1
```

The metrics clearly show:
- 50 messages put to TEST.QUEUE
- 15 messages retrieved from TEST.QUEUE
- 35 messages remaining in queue
- 1 consumer connected

## Complete Workflow Example

### 1. Build the Applications

```bash
cd ibmmq-go-stat-otel

# Build collector
go build -o collector.exe cmd/collector/main.go

# Build test tool
go build -o test-activity.exe cmd/test-activity/main.go
```

### 2. Start the Collector

```bash
# In one terminal/PowerShell window:
.\collector.exe -c configs/default.yaml --continuous --interval 30s --prometheus-port 9091
```

Expected output:
```
{"level":"info","msg":"Starting IBM MQ Statistics Collector","time":"2025-12-06T13:18:54-05:00"}
{"level":"info","msg":"Successfully connected to IBM MQ","time":"2025-12-06T13:18:54-05:00"}
{"level":"info","msg":"Opened statistics queue","queue":"SYSTEM.ADMIN.STATISTICS.QUEUE"}
{"level":"info","msg":"Opened accounting queue","queue":"SYSTEM.ADMIN.ACCOUNTING.QUEUE"}
{"level":"info","msg":"Starting Prometheus metrics HTTP server","path":"/metrics"}
```

### 3. Generate Test Activity

```bash
# In another terminal:
.\test-activity.exe -config configs/default.yaml -messages 50
```

### 4. Query Metrics

```bash
# In another terminal:
# Check collection happened
curl http://localhost:9091/metrics | findstr "queue_depth_current"

# Example output:
# ibmmq_queue_depth_current{queue_manager="MQQM1",queue_name="TEST.QUEUE"} 35
```

### 5. Interpret Results

After running test with 50 messages put and 15 retrieved:

| Metric | Value | Meaning |
|--------|-------|---------|
| ibmmq_mqi_puts | 50 | 50 messages sent to queue |
| ibmmq_mqi_gets | 15 | 15 messages retrieved from queue |
| ibmmq_queue_depth_current | 35 | 35 messages remaining in queue |
| ibmmq_mqi_opens | 2+ | Application connections |
| ibmmq_mqi_closes | 0 | No disconnections during test |

## Prerequisites

- IBM MQ Client libraries installed
- Go 1.21 or higher
- Access to IBM MQ Queue Manager
- MQSC command-line tools (runmqsc, amqsput, amqsget)
- Appropriate permissions to:
  - Read statistics and accounting queues
  - Define and alter queues
  - Execute MQSC commands

Reference: IBM MQ Knowledge Center - Installing IBM MQ
https://www.ibm.com/docs/en/ibm-mq/latest?topic=installing-ibm-mq

## Installation

### From Source

```bash
git clone https://github.com/aksdevs/ibmmq-go-stat-otel.git
cd ibmmq-go-stat-otel
go build -o collector.exe cmd/collector/main.go
```

### Configuration

Edit `configs/default.yaml`:

```yaml
mq:
  queue_manager: "MQQM1"           # Your queue manager name
  channel: "APP1.SVRCONN"          # SVRCONN channel name
  connection_name: "127.0.0.1(5200)"  # Host and port
  user: ""                         # User (empty if using current user)
  password: ""                     # Password (leave empty for Windows auth)

prometheus:
  namespace: "ibmmq"
  subsystem: "mq"
  port: 9091
```

## Running the Collector

### Continuous Mode (Recommended)

Collects metrics every 30 seconds and exposes them on port 9091:

```bash
.\collector.exe -c configs/default.yaml --continuous --interval 30s --prometheus-port 9091
```

Then query metrics:

```bash
curl http://localhost:9091/metrics
```

Reference: Prometheus - Exposition Format
https://prometheus.io/docs/instrumenting/exposition_formats/

### Single Collection

Collect once and exit:

```bash
.\collector.exe -c configs/default.yaml
```

## Architecture

### Data Flow

1. **Collector** connects to IBM MQ queue manager
2. **Opens** SYSTEM.ADMIN.STATISTICS.QUEUE and SYSTEM.ADMIN.ACCOUNTING.QUEUE
3. **Receives** PCF (Programmable Command Format) messages from IBM MQ
4. **Parses** message structure and extracts metric values
5. **Correlates** connection-level and queue-level data
6. **Exports** as Prometheus metrics on HTTP endpoint

Reference: IBM MQ Knowledge Center - System queues
https://www.ibm.com/docs/en/ibm-mq/latest?topic=queues-system-queue-definitions

### Key Components

**pkg/mqclient** - IBM MQ Connection
- Creates and manages connection to queue manager
- Opens/reads statistics and accounting queues
- Provides MQINQ API for direct queue queries
- Retrieves handle-level (application/process) information similar to `DIS QS TYPE(HANDLE)`

**pkg/pcf** - Message Parsing
- Parses PCF message headers and parameters
- Extracts MQI statistics and accounting data
- Builds queue-level operation records

**pkg/prometheus** - Metrics Export
- Manages Prometheus metric gauges (40+ metrics)
- Registers metrics with Prometheus registry
- Updates metric values from parsed data
- Collects handle-level metrics for active applications/processes on queues
- Exposes HTTP metrics endpoint

**cmd/collector** - Main Application
- Orchestrates collection cycle (messages, queue stats, handles)
- Manages YAML configuration
- Handles continuous vs one-time collection
- Controls HTTP server lifecycle

**cmd/test-activity** - Test Utility
- Generates realistic IBM MQ activity
- Configures test queues with statistics enabled
- Uses native IBM MQ tools (amqsput, amqsget)

## Understanding the Output

### Collector Logs

The collector outputs JSON-formatted logs:

```json
{"level":"info","msg":"Starting metrics collection","time":"2025-12-06T13:18:54-05:00"}
{"count":6,"level":"info","msg":"Retrieved messages from queue","queue_type":"stats"}
{"count":14,"level":"info","msg":"Retrieved messages from queue","queue_type":"accounting"}
{"accounting_messages":14,"level":"info","msg":"Completed metrics collection"}
```

Key events:
- "Starting metrics collection" - Collection cycle beginning
- "Retrieved messages from queue" - PCF messages received
- "Completed metrics collection" - Cycle finished
- "Invalid accounting data type" - Connection-level only (no queue groups)

### Metrics Endpoint

Access at `http://localhost:9091/metrics`:

```prometheus
# HELP ibmmq_mqi_puts Total number of MQI PUT operations
# TYPE ibmmq_mqi_puts gauge
ibmmq_mqi_puts{...,queue_name="unknown",...} 0
ibmmq_mqi_puts{...,queue_name="TEST.QUEUE",application_name="amqsput",...} 50

# HELP ibmmq_queue_depth_current Current depth of IBM MQ queue
# TYPE ibmmq_queue_depth_current gauge
ibmmq_queue_depth_current{queue_manager="MQQM1",queue_name="TEST.QUEUE"} 35
```

The format is standard Prometheus text exposition format, consumable by any Prometheus server or monitoring tool.

## Troubleshooting

### No Queue Names (All Show "unknown")

Likely causes:
1. Queue-level accounting not enabled
2. Statistics messages only (not accounting)
3. IBM MQ version doesn't support queue-level groups

Solution:
```bash
runmqsc MQQM1
ALTER QMGR STATMQI(ON) ACCTQ(ON)
ALTER QLOCAL(YOUR.QUEUE) STATQ(ON) ACCTQ(ON)
END
```

Then restart collector and run test activity.

Reference: IBM MQ Knowledge Center - Displaying queue manager attributes
https://www.ibm.com/docs/en/ibm-mq/latest?topic=reference-display-qmgr

### Metrics Not Updating

Check:
1. Is collector running? Check logs for errors
2. Is IBM MQ accessible? Run test connection
3. Are statistics queues configured?
4. Try running test-activity to generate data

### Connection Failures

```json
{"level":"error","msg":"failed to connect to IBM MQ","reason":"..."}
```

Check:
- Queue manager is running
- Connection string is correct
- User has proper permissions
- SVRCONN channel is defined and configured

## Future Enhancements

As IBM MQ evolves, the collector can be extended with:

1. **Additional queue-level metrics**: When IBM MQ provides new queue attributes in accounting data
2. **Channel-level metrics**: Similar queue-level approach for channel operations
3. **Custom fields**: Parse customer-specific fields added to PCF messages
4. **Real-time alerts**: Trigger Prometheus alerts on metric thresholds
5. **Grafana dashboards**: Pre-built dashboards for visualization

The codebase is structured to accommodate these enhancements without breaking existing functionality.

## License

MIT License - See LICENSE file

## Support

For issues or questions, refer to IBM MQ documentation or this project's issue tracker.
