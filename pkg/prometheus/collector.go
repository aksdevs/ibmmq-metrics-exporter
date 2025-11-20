package prometheus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"github.com/skatul/ibmmq-go-stat-otel/pkg/config"
	"github.com/skatul/ibmmq-go-stat-otel/pkg/mqclient"
	"github.com/skatul/ibmmq-go-stat-otel/pkg/pcf"
)

// MetricsCollector handles collection and export of IBM MQ metrics to Prometheus
type MetricsCollector struct {
	config    *config.Config
	mqClient  *mqclient.MQClient
	pcfParser *pcf.Parser
	logger    *logrus.Logger
	registry  *prometheus.Registry

	// Prometheus metrics
	queueDepthGauge       *prometheus.GaugeVec
	queueHighDepthGauge   *prometheus.GaugeVec
	queueEnqueueGauge     *prometheus.GaugeVec
	queueDequeueGauge     *prometheus.GaugeVec
	queueInputCountGauge  *prometheus.GaugeVec
	queueOutputCountGauge *prometheus.GaugeVec
	queueReadersGauge     *prometheus.GaugeVec
	queueWritersGauge     *prometheus.GaugeVec

	channelMessagesGauge *prometheus.GaugeVec
	channelBytesGauge    *prometheus.GaugeVec
	channelBatchesGauge  *prometheus.GaugeVec

	mqiOpensGauge    *prometheus.GaugeVec
	mqiClosesGauge   *prometheus.GaugeVec
	mqiPutsGauge     *prometheus.GaugeVec
	mqiGetsGauge     *prometheus.GaugeVec
	mqiCommitsGauge  *prometheus.GaugeVec
	mqiBackoutsGauge *prometheus.GaugeVec

	collectionInfoGauge *prometheus.GaugeVec
	lastCollectionTime  *prometheus.GaugeVec

	mu sync.RWMutex
}

// NewMetricsCollector creates a new Prometheus metrics collector
func NewMetricsCollector(cfg *config.Config, mqClient *mqclient.MQClient, logger *logrus.Logger) *MetricsCollector {
	registry := prometheus.NewRegistry()

	collector := &MetricsCollector{
		config:    cfg,
		mqClient:  mqClient,
		pcfParser: pcf.NewParser(logger),
		logger:    logger,
		registry:  registry,
	}

	collector.initMetrics()
	return collector
}

// initMetrics initializes all Prometheus metrics
func (c *MetricsCollector) initMetrics() {
	namespace := c.config.Prometheus.Namespace
	subsystem := c.config.Prometheus.Subsystem

	// Queue metrics
	c.queueDepthGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_depth_current",
			Help:      "Current depth of IBM MQ queue",
		},
		[]string{"queue_manager", "queue_name"},
	)

	c.queueHighDepthGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_depth_high",
			Help:      "High water mark of IBM MQ queue depth",
		},
		[]string{"queue_manager", "queue_name"},
	)

	c.queueEnqueueGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_enqueue_count",
			Help:      "Total number of messages enqueued to IBM MQ queue",
		},
		[]string{"queue_manager", "queue_name"},
	)

	c.queueDequeueGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_dequeue_count",
			Help:      "Total number of messages dequeued from IBM MQ queue",
		},
		[]string{"queue_manager", "queue_name"},
	)

	c.queueInputCountGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_input_handles",
			Help:      "Number of input handles open for IBM MQ queue",
		},
		[]string{"queue_manager", "queue_name"},
	)

	c.queueOutputCountGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_output_handles",
			Help:      "Number of output handles open for IBM MQ queue",
		},
		[]string{"queue_manager", "queue_name"},
	)

	c.queueReadersGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_has_readers",
			Help:      "Whether IBM MQ queue has active readers (1=yes, 0=no)",
		},
		[]string{"queue_manager", "queue_name"},
	)

	c.queueWritersGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_has_writers",
			Help:      "Whether IBM MQ queue has active writers (1=yes, 0=no)",
		},
		[]string{"queue_manager", "queue_name"},
	)

	// Channel metrics
	c.channelMessagesGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "channel_messages_total",
			Help:      "Total number of messages sent through IBM MQ channel",
		},
		[]string{"queue_manager", "channel_name", "connection_name"},
	)

	c.channelBytesGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "channel_bytes_total",
			Help:      "Total number of bytes sent through IBM MQ channel",
		},
		[]string{"queue_manager", "channel_name", "connection_name"},
	)

	c.channelBatchesGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "channel_batches_total",
			Help:      "Total number of batches sent through IBM MQ channel",
		},
		[]string{"queue_manager", "channel_name", "connection_name"},
	)

	// MQI operation metrics
	c.mqiOpensGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "mqi_opens_total",
			Help:      "Total number of MQI OPEN operations",
		},
		[]string{"queue_manager", "application_name"},
	)

	c.mqiClosesGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "mqi_closes_total",
			Help:      "Total number of MQI CLOSE operations",
		},
		[]string{"queue_manager", "application_name"},
	)

	c.mqiPutsGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "mqi_puts_total",
			Help:      "Total number of MQI PUT operations",
		},
		[]string{"queue_manager", "application_name"},
	)

	c.mqiGetsGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "mqi_gets_total",
			Help:      "Total number of MQI GET operations",
		},
		[]string{"queue_manager", "application_name"},
	)

	c.mqiCommitsGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "mqi_commits_total",
			Help:      "Total number of MQI COMMIT operations",
		},
		[]string{"queue_manager", "application_name"},
	)

	c.mqiBackoutsGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "mqi_backouts_total",
			Help:      "Total number of MQI BACKOUT operations",
		},
		[]string{"queue_manager", "application_name"},
	)

	// Collection info metrics
	c.collectionInfoGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "collection_info",
			Help:      "Information about the collection process",
		},
		[]string{"queue_manager", "channel", "collector_version"},
	)

	c.lastCollectionTime = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "last_collection_timestamp",
			Help:      "Timestamp of the last successful collection",
		},
		[]string{"queue_manager"},
	)

	// Register all metrics
	c.registry.MustRegister(
		c.queueDepthGauge,
		c.queueHighDepthGauge,
		c.queueEnqueueGauge,
		c.queueDequeueGauge,
		c.queueInputCountGauge,
		c.queueOutputCountGauge,
		c.queueReadersGauge,
		c.queueWritersGauge,
		c.channelMessagesGauge,
		c.channelBytesGauge,
		c.channelBatchesGauge,
		c.mqiOpensGauge,
		c.mqiClosesGauge,
		c.mqiPutsGauge,
		c.mqiGetsGauge,
		c.mqiCommitsGauge,
		c.mqiBackoutsGauge,
		c.collectionInfoGauge,
		c.lastCollectionTime,
	)
}

// CollectMetrics collects metrics from IBM MQ and updates Prometheus gauges
func (c *MetricsCollector) CollectMetrics(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Info("Starting metrics collection")

	statsMessages, err := c.collectMessages("stats")
	if err != nil {
		c.logger.WithError(err).Error("Failed to collect statistics messages")
		return err
	}

	accountingMessages, err := c.collectMessages("accounting")
	if err != nil {
		c.logger.WithError(err).Error("Failed to collect accounting messages")
		return err
	}

	// Update metrics from collected data
	c.updateMetricsFromMessages(statsMessages, accountingMessages)

	// Update collection timestamp
	c.lastCollectionTime.WithLabelValues(c.config.MQ.QueueManager).Set(float64(time.Now().Unix()))

	c.logger.WithFields(logrus.Fields{
		"stats_messages":      len(statsMessages),
		"accounting_messages": len(accountingMessages),
	}).Info("Completed metrics collection")

	return nil
}

// collectMessages collects messages from specified queue type
func (c *MetricsCollector) collectMessages(queueType string) ([]*mqclient.MQMessage, error) {
	messages, err := c.mqClient.GetAllMessages(queueType)
	if err != nil {
		return nil, fmt.Errorf("failed to get %s messages: %w", queueType, err)
	}

	c.logger.WithFields(logrus.Fields{
		"queue_type": queueType,
		"count":      len(messages),
	}).Debug("Collected messages")

	return messages, nil
}

// updateMetricsFromMessages processes messages and updates Prometheus metrics
func (c *MetricsCollector) updateMetricsFromMessages(statsMessages, accountingMessages []*mqclient.MQMessage) {
	// Process statistics messages
	for _, msg := range statsMessages {
		c.processStatisticsMessage(msg)
	}

	// Process accounting messages
	for _, msg := range accountingMessages {
		c.processAccountingMessage(msg)
	}

	// Update collection info
	c.collectionInfoGauge.WithLabelValues(
		c.config.MQ.QueueManager,
		c.config.MQ.Channel,
		"1.0.0", // collector version
	).Set(1)
}

// processStatisticsMessage processes a single statistics message
func (c *MetricsCollector) processStatisticsMessage(msg *mqclient.MQMessage) {
	data, err := c.pcfParser.ParseMessage(msg.Data, "statistics")
	if err != nil {
		c.logger.WithError(err).Error("Failed to parse statistics message")
		return
	}

	stats, ok := data.(*pcf.StatisticsData)
	if !ok {
		c.logger.Error("Invalid statistics data type")
		return
	}

	qmgr := stats.QueueManager
	if qmgr == "" {
		qmgr = c.config.MQ.QueueManager
	}

	// Update queue statistics
	if queueStats := stats.QueueStats; queueStats != nil {
		labels := []string{qmgr, queueStats.QueueName}

		c.queueDepthGauge.WithLabelValues(labels...).Set(float64(queueStats.CurrentDepth))
		c.queueHighDepthGauge.WithLabelValues(labels...).Set(float64(queueStats.HighDepth))
		c.queueEnqueueGauge.WithLabelValues(labels...).Set(float64(queueStats.EnqueueCount))
		c.queueDequeueGauge.WithLabelValues(labels...).Set(float64(queueStats.DequeueCount))
		c.queueInputCountGauge.WithLabelValues(labels...).Set(float64(queueStats.InputCount))
		c.queueOutputCountGauge.WithLabelValues(labels...).Set(float64(queueStats.OutputCount))

		// Set reader/writer flags
		if queueStats.HasReaders {
			c.queueReadersGauge.WithLabelValues(labels...).Set(1)
		} else {
			c.queueReadersGauge.WithLabelValues(labels...).Set(0)
		}

		if queueStats.HasWriters {
			c.queueWritersGauge.WithLabelValues(labels...).Set(1)
		} else {
			c.queueWritersGauge.WithLabelValues(labels...).Set(0)
		}
	}

	// Update channel statistics
	if channelStats := stats.ChannelStats; channelStats != nil {
		labels := []string{qmgr, channelStats.ChannelName, channelStats.ConnectionName}

		c.channelMessagesGauge.WithLabelValues(labels...).Set(float64(channelStats.Messages))
		c.channelBytesGauge.WithLabelValues(labels...).Set(float64(channelStats.Bytes))
		c.channelBatchesGauge.WithLabelValues(labels...).Set(float64(channelStats.Batches))
	}

	// Update MQI statistics
	if mqiStats := stats.MQIStats; mqiStats != nil {
		labels := []string{qmgr, mqiStats.ApplicationName}

		c.mqiOpensGauge.WithLabelValues(labels...).Set(float64(mqiStats.Opens))
		c.mqiClosesGauge.WithLabelValues(labels...).Set(float64(mqiStats.Closes))
		c.mqiPutsGauge.WithLabelValues(labels...).Set(float64(mqiStats.Puts))
		c.mqiGetsGauge.WithLabelValues(labels...).Set(float64(mqiStats.Gets))
		c.mqiCommitsGauge.WithLabelValues(labels...).Set(float64(mqiStats.Commits))
		c.mqiBackoutsGauge.WithLabelValues(labels...).Set(float64(mqiStats.Backouts))
	}
}

// processAccountingMessage processes a single accounting message
func (c *MetricsCollector) processAccountingMessage(msg *mqclient.MQMessage) {
	data, err := c.pcfParser.ParseMessage(msg.Data, "accounting")
	if err != nil {
		c.logger.WithError(err).Error("Failed to parse accounting message")
		return
	}

	acct, ok := data.(*pcf.AccountingData)
	if !ok {
		c.logger.Error("Invalid accounting data type")
		return
	}

	qmgr := acct.QueueManager
	if qmgr == "" {
		qmgr = c.config.MQ.QueueManager
	}

	// Update MQI operation counts from accounting data
	if ops := acct.Operations; ops != nil {
		appName := ""
		if acct.ConnectionInfo != nil {
			appName = acct.ConnectionInfo.ApplicationName
		}

		labels := []string{qmgr, appName}

		c.mqiOpensGauge.WithLabelValues(labels...).Add(float64(ops.Opens))
		c.mqiClosesGauge.WithLabelValues(labels...).Add(float64(ops.Closes))
		c.mqiPutsGauge.WithLabelValues(labels...).Add(float64(ops.Puts))
		c.mqiGetsGauge.WithLabelValues(labels...).Add(float64(ops.Gets))
		c.mqiCommitsGauge.WithLabelValues(labels...).Add(float64(ops.Commits))
		c.mqiBackoutsGauge.WithLabelValues(labels...).Add(float64(ops.Backouts))
	}
}

// GetRegistry returns the Prometheus registry
func (c *MetricsCollector) GetRegistry() *prometheus.Registry {
	return c.registry
}

// ResetMetrics clears all metrics
func (c *MetricsCollector) ResetMetrics() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Reset all gauges by creating new instances
	// This is more efficient than iterating through all label combinations
	c.queueDepthGauge.Reset()
	c.queueHighDepthGauge.Reset()
	c.queueEnqueueGauge.Reset()
	c.queueDequeueGauge.Reset()
	c.queueInputCountGauge.Reset()
	c.queueOutputCountGauge.Reset()
	c.queueReadersGauge.Reset()
	c.queueWritersGauge.Reset()
	c.channelMessagesGauge.Reset()
	c.channelBytesGauge.Reset()
	c.channelBatchesGauge.Reset()
	c.mqiOpensGauge.Reset()
	c.mqiClosesGauge.Reset()
	c.mqiPutsGauge.Reset()
	c.mqiGetsGauge.Reset()
	c.mqiCommitsGauge.Reset()
	c.mqiBackoutsGauge.Reset()

	c.logger.Info("Reset all metrics")
}
