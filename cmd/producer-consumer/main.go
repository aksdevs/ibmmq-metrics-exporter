package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/aksdevs/ibmmq-go-stat-otel/pkg/config"
	"github.com/aksdevs/ibmmq-go-stat-otel/pkg/mqclient"
	ibmmq "github.com/ibm-messaging/mq-golang/v5/ibmmq"
	"github.com/sirupsen/logrus"
)

func main() {
	configPath := flag.String("config", "configs/default.yaml", "Configuration file path")
	queueName := flag.String("queue", "TEST.QUEUE", "Queue name to connect to")
	appName := flag.String("app", "producer-consumer-test", "Application name (visible in MQ)")
	messageInterval := flag.Duration("interval", 5*time.Second, "Interval between producer messages (0 = no messages)")
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

	// Create MQ client for connections
	client := mqclient.NewMQClient(&cfg.MQ, logger)

	// Connect to MQ
	if err := client.Connect(); err != nil {
		fmt.Printf("Failed to connect to MQ: %v\n", err)
		os.Exit(1)
	}
	defer client.Disconnect()

	logger.Printf("Connected to IBM MQ queue manager: %s", cfg.MQ.QueueManager)

	// Create producer and consumer handles
	producerHandle, err := createProducerHandle(client, *queueName, logger)
	if err != nil {
		fmt.Printf("Failed to create producer handle: %v\n", err)
		os.Exit(1)
	}
	defer producerHandle.Close(0)

	consumerHandle, err := createConsumerHandle(client, *queueName, logger)
	if err != nil {
		fmt.Printf("Failed to create consumer handle: %v\n", err)
		os.Exit(1)
	}
	defer consumerHandle.Close(0)

	logger.Infof("✓ Producer handle opened on queue '%s'", *queueName)
	logger.Infof("✓ Consumer handle opened on queue '%s'", *queueName)
	logger.Infof("✓ Application name: %s", *appName)

	// Set up signal handler for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start message producer in background (if interval > 0)
	stopProducer := make(chan bool)
	var producerWg sync.WaitGroup

	if *messageInterval > 0 {
		producerWg.Add(1)
		go produceMessages(producerHandle, *queueName, *messageInterval, stopProducer, &producerWg, logger)
		logger.Infof("Message producer started (interval: %v)", *messageInterval)
	} else {
		logger.Infof("Message producer disabled (interval: 0)")
	}

	// Start message consumer in background
	stopConsumer := make(chan bool)
	var consumerWg sync.WaitGroup
	consumerWg.Add(1)
	go consumeMessages(consumerHandle, *queueName, stopConsumer, &consumerWg, logger)
	logger.Infof("Message consumer started")

	// Periodic status display - just log that handles are open
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	logger.Infof("Both producer and consumer connections are OPEN and visible to DISPLAY QSTATUS")
	logger.Infof("Press Ctrl+C to close connections")

	// Wait for signal
	for {
		select {
		case <-sigChan:
			logger.Infof("Received shutdown signal")
			stopProducer <- true
			stopConsumer <- true
			producerWg.Wait()
			consumerWg.Wait()
			logger.Infof("✓ All handles closed")
			return
		case <-ticker.C:
			logger.Infof("Handles still open on %s (producer and consumer active)", *queueName)
		}
	}
}

// createProducerHandle creates an output (put) handle on the queue
func createProducerHandle(client *mqclient.MQClient, queueName string, logger *logrus.Logger) (ibmmq.MQObject, error) {
	od := ibmmq.NewMQOD()
	od.ObjectName = queueName
	od.ObjectType = ibmmq.MQOT_Q

	qmgr := client.GetQueueManager()
	queue, err := qmgr.Open(od, ibmmq.MQOO_OUTPUT|ibmmq.MQOO_FAIL_IF_QUIESCING)
	if err != nil {
		return ibmmq.MQObject{}, fmt.Errorf("failed to open queue %s for output: %w", queueName, err)
	}

	return queue, nil
}

// createConsumerHandle creates an input (get) handle on the queue
func createConsumerHandle(client *mqclient.MQClient, queueName string, logger *logrus.Logger) (ibmmq.MQObject, error) {
	od := ibmmq.NewMQOD()
	od.ObjectName = queueName
	od.ObjectType = ibmmq.MQOT_Q

	qmgr := client.GetQueueManager()
	queue, err := qmgr.Open(od, ibmmq.MQOO_INPUT_SHARED|ibmmq.MQOO_FAIL_IF_QUIESCING)
	if err != nil {
		return ibmmq.MQObject{}, fmt.Errorf("failed to open queue %s for input: %w", queueName, err)
	}

	return queue, nil
}

// produceMessages sends messages at regular intervals
func produceMessages(queue ibmmq.MQObject, queueName string, interval time.Duration, stop chan bool, wg *sync.WaitGroup, logger *logrus.Logger) {
	defer wg.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	msgCount := 0

	for {
		select {
		case <-stop:
			logger.Infof("Producer stopping after %d messages", msgCount)
			return
		case <-ticker.C:
			msgCount++
			timestamp := time.Now().Format("15:04:05.000")
			messageData := fmt.Sprintf("Test message %d at %s", msgCount, timestamp)

			// Create message descriptor
			md := ibmmq.NewMQMD()
			pmo := ibmmq.NewMQPMO()
			pmo.Options = ibmmq.MQPMO_NO_SYNCPOINT | ibmmq.MQPMO_FAIL_IF_QUIESCING

			// Put message on queue
			err := queue.Put(md, pmo, []byte(messageData))
			if err != nil {
				logger.Debugf("Put message %d: %v", msgCount, err)
			} else {
				logger.Debugf("Produced message %d on %s", msgCount, queueName)
			}
		}
	}
}

// consumeMessages receives messages from the queue
func consumeMessages(queue ibmmq.MQObject, queueName string, stop chan bool, wg *sync.WaitGroup, logger *logrus.Logger) {
	defer wg.Done()

	msgCount := 0
	gmo := ibmmq.NewMQGMO()
	gmo.Options = ibmmq.MQGMO_NO_SYNCPOINT | ibmmq.MQGMO_FAIL_IF_QUIESCING

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			logger.Infof("Consumer stopping after %d messages", msgCount)
			return
		case <-ticker.C:
			md := ibmmq.NewMQMD()
			msgBuffer := make([]byte, 1024)

			// Try to get a message without waiting
			length, err := queue.Get(md, gmo, msgBuffer)
			if err != nil {
				// No message available is not an error, just continue
				logger.Debugf("Get: %v", err)
				continue
			}

			msgCount++
			message := string(msgBuffer[:length])
			logger.Debugf("Consumed message %d from %s: %s", msgCount, queueName, message)
		}
	}
}

// displayQueueStatus shows the current queue metrics
func displayQueueStatus(client *mqclient.MQClient, queueName string, logger *logrus.Logger) {
	info, err := client.GetQueueInfo(queueName)
	if err != nil {
		logger.WithError(err).Errorf("Failed to get queue status for %s", queueName)
		return
	}

	logger.Infof("Queue Status %s: Depth=%d, InputHandles=%d, OutputHandles=%d, MaxDepth=%d",
		queueName, info.CurrentDepth, info.OpenInputCount, info.OpenOutputCount, info.MaxQueueDepth)
}
