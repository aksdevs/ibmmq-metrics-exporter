package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aksdevs/ibmmq-go-stat-otel/pkg/config"
	"github.com/aksdevs/ibmmq-go-stat-otel/pkg/mqclient"
	ibmmq "github.com/ibm-messaging/mq-golang/v5/ibmmq"
	"github.com/sirupsen/logrus"
)

func main() {
	configPath := flag.String("config", "configs/default.yaml", "Configuration file path")
	queueName := flag.String("queue", "TEST.Q1", "Queue name to keep open")
	duration := flag.Duration("duration", 0, "Duration to keep handle open (0 = indefinite)")
	inputMode := flag.Bool("input", false, "Open queue in input mode (default is output)")
	flag.Parse()

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Create logger
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	logger.SetOutput(os.Stdout)

	// Create MQ client
	client := mqclient.NewMQClient(&cfg.MQ, logger)

	// Connect to MQ
	if err := client.Connect(); err != nil {
		fmt.Printf("Failed to connect to MQ: %v\n", err)
		os.Exit(1)
	}
	defer client.Disconnect()

	logger.Printf("Connected to IBM MQ queue manager: %s", cfg.MQ.QueueManager)

	// Open the queue with a persistent handle
	openMode := ibmmq.MQOO_OUTPUT | ibmmq.MQOO_FAIL_IF_QUIESCING
	modeStr := "OUTPUT"
	if *inputMode {
		openMode = ibmmq.MQOO_INPUT_SHARED | ibmmq.MQOO_FAIL_IF_QUIESCING
		modeStr = "INPUT (SHARED)"
	}

	od := ibmmq.NewMQOD()
	od.ObjectName = *queueName
	od.ObjectType = ibmmq.MQOT_Q

	qmgr := client.GetQueueManager()
	queue, err := qmgr.Open(od, openMode)
	if err != nil {
		fmt.Printf("Failed to open queue %s in %s mode: %v\n", *queueName, modeStr, err)
		os.Exit(1)
	}

	logger.Printf("✓ Opened queue '%s' in %s mode", *queueName, modeStr)
	logger.Printf("✓ Handle is now OPEN and visible to DISPLAY QSTATUS command")
	logger.Printf("✓ Application name: %s", os.Args[0])

	// Display queue status info
	displayQueueStatus(client, *queueName, logger)

	// Set up signal handler for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Keep the handle open for the specified duration or until interrupted
	var endTime time.Time
	if *duration > 0 {
		endTime = time.Now().Add(*duration)
		logger.Printf("Will keep handle open for %v (until %s)", *duration, endTime.Format("15:04:05"))
	} else {
		logger.Printf("Will keep handle open indefinitely. Press Ctrl+C to close.")
	}

	// Wait for signal or timeout
	for {
		select {
		case <-sigChan:
			logger.Printf("Received shutdown signal")
			queue.Close(0)
			logger.Printf("✓ Queue handle closed")
			return
		case <-time.After(10 * time.Second):
			// Periodically display queue status
			displayQueueStatus(client, *queueName, logger)
			if *duration > 0 && time.Now().After(endTime) {
				logger.Printf("Duration expired, closing handle")
				queue.Close(0)
				logger.Printf("✓ Queue handle closed")
				return
			}
		}
	}
}

func displayQueueStatus(client *mqclient.MQClient, queueName string, logger *logrus.Logger) {
	info, err := client.GetQueueInfo(queueName)
	if err != nil {
		logger.WithError(err).Errorf("Failed to get queue status for %s", queueName)
		return
	}

	logger.Printf("\n=== Queue Status: %s ===", queueName)
	logger.Printf("  Current Depth:      %d", info.CurrentDepth)
	logger.Printf("  Input Handles:      %d", info.OpenInputCount)
	logger.Printf("  Output Handles:     %d", info.OpenOutputCount)
	logger.Printf("  Max Queue Depth:    %d", info.MaxQueueDepth)
	logger.Printf("==========================================\n")
}
