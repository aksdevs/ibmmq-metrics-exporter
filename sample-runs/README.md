# IBM MQ Test Activity Generation Scripts

This directory contains scripts to generate test activity on IBM MQ queues for testing the statistics and accounting collection functionality.

## Scripts Available

### PowerShell Script (Windows)
**File:** `generate-test-activity.ps1`

**Usage:**
```powershell
.\generate-test-activity.ps1 [-MessageCount 50] [-QueueManager "MQQM1"] [-Channel "APP1.SVRCONN"] [-ConnName "127.0.0.1(5200)"] [-Queues @("APP1.REQ", "APP2.REQ")]
```

**Examples:**
```powershell
# Default parameters (50 messages per queue)
.\generate-test-activity.ps1

# Custom message count
.\generate-test-activity.ps1 -MessageCount 100

# Custom queue manager and queues
.\generate-test-activity.ps1 -QueueManager "TESTQM" -Queues @("QUEUE1", "QUEUE2", "QUEUE3")
```

### Bash Script (Linux/Unix)
**File:** `generate-test-activity.sh`

**Usage:**
```bash
./generate-test-activity.sh [-m MESSAGE_COUNT] [-q QUEUE_MANAGER] [-c CHANNEL] [-n CONN_NAME] [-Q 'QUEUE1 QUEUE2']
```

**Setup (Linux/Unix):**
```bash
# Make script executable
chmod +x generate-test-activity.sh

# Source IBM MQ environment (if needed)
source /opt/mqm/bin/setmqenv -s
```

**Examples:**
```bash
# Default parameters (50 messages per queue)
./generate-test-activity.sh

# Custom message count
./generate-test-activity.sh -m 100

# Custom parameters
./generate-test-activity.sh -m 25 -q "TESTQM" -Q "QUEUE1 QUEUE2 QUEUE3"

# Help
./generate-test-activity.sh -h
```

## What These Scripts Do

Both scripts perform the same operations:

1. **Put Messages**: Uses `amqsput` to place test messages on specified queues
2. **Get Messages**: Uses `amqsget` to retrieve approximately half the messages (simulates readers)
3. **Display Status**: Uses `runmqsc` to show queue depths and statistics

## Generated Activity

The scripts create IBM MQ activity that will be detected by the statistics collector:

- **Writer Activity**: Messages put to queues create output process (opprocs) statistics
- **Reader Activity**: Messages retrieved from queues create input process (ipprocs) statistics
- **Queue Statistics**: Both operations update queue depth and throughput metrics

## Prerequisites

### Windows
- IBM MQ Client or Server installed
- `amqsput.exe`, `amqsget.exe`, and `runmqsc.exe` in PATH or at default location:
  - `C:\Program Files\IBM\MQ\bin64\`

### Linux/Unix
- IBM MQ Client or Server installed
- `amqsput`, `amqsget`, and `runmqsc` in PATH
- Typical locations:
  - `/opt/mqm/samp/bin/` (sample programs)
  - `/opt/mqm/bin/` (administration tools)

## Output

Both scripts will show:
- Connection details
- Progress of message operations
- Success/error status for each queue
- Final queue status with depths and statistics

## Integration with Collector

After running either script:
1. The generated MQ activity will appear in IBM MQ statistics
2. Run the PCF dumper: `./pcf-dumper.exe`
3. Run the main collector: `./main.exe`
4. Check Prometheus metrics at `http://localhost:8080/metrics`

The collector should identify reader/writer applications and tag metrics accordingly.