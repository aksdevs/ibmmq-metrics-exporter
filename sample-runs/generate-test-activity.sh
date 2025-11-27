#!/bin/bash
# Bash script to generate test messages on local IBM MQ queues
# This will create activity for statistics and accounting collection

# Default parameters
MESSAGE_COUNT=5
QUEUE_MANAGER="MQQM1"
CHANNEL="APP1.SVRCONN"
CONN_NAME="127.0.0.1(5200)"
QUEUES=("APP1.REQ" "APP2.REQ")

# Function to display usage
usage() {
    echo "Usage: $0 [-m MESSAGE_COUNT] [-q QUEUE_MANAGER] [-c CHANNEL] [-n CONN_NAME] [-Q 'QUEUE1 QUEUE2']"
    echo "  -m: Message count (default: 5)"
    echo "  -q: Queue Manager (default: MQQM1)"
    echo "  -c: Channel (default: APP1.SVRCONN)"
    echo "  -n: Connection Name (default: 127.0.0.1(5200))"
    echo "  -Q: Queues space-separated in quotes (default: 'APP1.REQ APP2.REQ')"
    exit 1
}

# Parse command line arguments
while getopts "m:q:c:n:Q:h" opt; do
    case $opt in
        m) MESSAGE_COUNT="$OPTARG";;
        q) QUEUE_MANAGER="$OPTARG";;
        c) CHANNEL="$OPTARG";;
        n) CONN_NAME="$OPTARG";;
        Q) IFS=' ' read -ra QUEUES <<< "$OPTARG";;
        h) usage;;
        *) usage;;
    esac
done

echo "Generating test activity on IBM MQ..."
echo "Queue Manager: $QUEUE_MANAGER"
echo "Channel: $CHANNEL"
echo "Connection: $CONN_NAME"
echo "Queues: ${QUEUES[*]}"
echo "Messages per queue: $MESSAGE_COUNT"
echo ""

# Set environment variable for client connection
export MQSERVER="$CHANNEL/TCP/$CONN_NAME"

# Get current timestamp
timestamp=$(date "+%Y-%m-%d %H:%M:%S")

# Function to check if amqsput is available
check_mq_commands() {
    if ! command -v amqsput &> /dev/null; then
        echo "Error: amqsput not found. Please ensure IBM MQ client is installed and in PATH."
        echo "On Linux, it's typically in /opt/mqm/samp/bin/"
        echo "You may need to source /opt/mqm/bin/setmqenv -s"
        exit 1
    fi
}

# Check MQ commands availability
check_mq_commands

# Generate activity for each queue
for queue in "${QUEUES[@]}"; do
    echo "Processing queue: $queue"
    
    # Put messages using amqsput
    echo "  Putting $MESSAGE_COUNT messages..."
    
    # Create temporary file with messages
    temp_messages=$(mktemp)
    for ((i=1; i<=MESSAGE_COUNT; i++)); do
        echo "Test message $i for $queue - $timestamp" >> "$temp_messages"
    done
    echo "" >> "$temp_messages"  # Empty line to terminate amqsput
    
    # Execute amqsput
    if cat "$temp_messages" | amqsput "$queue" "$QUEUE_MANAGER" > /dev/null 2>&1; then
        echo "  Successfully put $MESSAGE_COUNT messages"
    else
        echo "  Error putting messages (Exit code: $?)"
    fi
    
    # Clean up temporary file
    rm -f "$temp_messages"
    
    # Get some messages back using amqsget to create GET statistics
    get_count=$((MESSAGE_COUNT > 1 ? MESSAGE_COUNT / 2 : 1))
    echo "  Getting $get_count messages to create reader statistics..."
    
    retrieved=0
    for ((i=1; i<=get_count; i++)); do
        if amqsget "$queue" "$QUEUE_MANAGER" > /dev/null 2>&1; then
            retrieved=$((retrieved + 1))
        else
            break
        fi
    done
    
    if [ $retrieved -gt 0 ]; then
        echo "  Retrieved $retrieved messages"
    else
        echo "  No messages retrieved"
    fi
    echo ""
done

# Display queue depths
echo "Checking queue depths..."

# Create MQSC script for checking queue status
depth_script=$(mktemp)
for queue in "${QUEUES[@]}"; do
    echo "DISPLAY QLOCAL('$queue') CURDEPTH MAXDEPTH" >> "$depth_script"
done

if runmqsc "$QUEUE_MANAGER" < "$depth_script" 2>/dev/null | grep CURDEPTH; then
    :
else
    echo "Failed to check queue depths"
fi

# Clean up
rm -f "$depth_script"

echo ""
echo "Test activity generation complete!"
echo "You can now run the collector to gather statistics from this activity."