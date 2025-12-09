package prometheus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aksdevs/ibmmq-go-stat-otel/pkg/config"
	"github.com/aksdevs/ibmmq-go-stat-otel/pkg/mqclient"
	"github.com/aksdevs/ibmmq-go-stat-otel/pkg/pcf"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

// MetricsCollector handles collection and export of IBM MQ metrics to Prometheus
type MetricsCollector struct {
	config    *config.Config
	mqClient  *mqclient.MQClient
	pcfParser *pcf.Parser
	logger    *logrus.Logger
	registry  *prometheus.Registry

	// MQI metrics - using a map to efficiently store all 59 metrics
	mqiMetrics map[string]*prometheus.GaugeVec

	// Prometheus metrics for queues and channels
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

	// Per-queue processes (ipprocs/opprocs)
	queueProcessGauge *prometheus.GaugeVec

	// Per-queue per-application operation metrics
	queueAppPutsGauge         *prometheus.GaugeVec
	queueAppGetsGauge         *prometheus.GaugeVec
	queueAppMsgsReceivedGauge *prometheus.GaugeVec
	queueAppMsgsSentGauge     *prometheus.GaugeVec

	// Handle-level metrics (application/process) metrics
	queueHandleCountGauge   *prometheus.GaugeVec // Total handles open on queue
	queueHandleDetailsGauge *prometheus.GaugeVec // Per-handle details (app name, user, mode)
	queueProcessDetailGauge *prometheus.GaugeVec // Process details with PID (app name, PID, user, role)

	collectionInfoGauge *prometheus.GaugeVec
	lastCollectionTime  *prometheus.GaugeVec

	// Cache of process information from statistics messages, indexed by queue name
	// Used to populate detailed handle metrics when statistics contain process data
	queueProcsCache map[string][]pcf.ProcInfo

	mu sync.RWMutex
}

// NewMetricsCollector creates a new Prometheus metrics collector
func NewMetricsCollector(cfg *config.Config, mqClient *mqclient.MQClient, logger *logrus.Logger) *MetricsCollector {
	registry := prometheus.NewRegistry()

	collector := &MetricsCollector{
		config:          cfg,
		mqClient:        mqClient,
		pcfParser:       pcf.NewParser(logger),
		logger:          logger,
		registry:        registry,
		queueProcsCache: make(map[string][]pcf.ProcInfo),
	}

	collector.initMetrics()
	return collector
}

// NewMetricsCollectorWithRegistry creates a new Prometheus metrics collector with an existing registry
func NewMetricsCollectorWithRegistry(cfg *config.Config, mqClient *mqclient.MQClient, logger *logrus.Logger, registry *prometheus.Registry) *MetricsCollector {
	collector := &MetricsCollector{
		config:          cfg,
		mqClient:        mqClient,
		pcfParser:       pcf.NewParser(logger),
		logger:          logger,
		registry:        registry,
		queueProcsCache: make(map[string][]pcf.ProcInfo),
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
	c.queueProcessGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_process",
			Help:      "Processes associated with a queue (ipprocs/opprocs). Value is 1 for each process",
		},
		[]string{"queue_manager", "queue_name", "application_name", "source_ip", "role"},
	)

	// Per-queue per-application puts
	c.queueAppPutsGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_app_puts_total",
			Help:      "Total number of PUTs performed on a queue by a specific application/connection",
		},
		[]string{"queue_manager", "queue_name", "application_name", "source_ip", "user_identifier"},
	)

	// Per-queue per-application gets
	c.queueAppGetsGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_app_gets_total",
			Help:      "Total number of GETs performed on a queue by a specific application/connection",
		},
		[]string{"queue_manager", "queue_name", "application_name", "source_ip", "user_identifier"},
	)

	// Per-queue per-application messages received
	c.queueAppMsgsReceivedGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_app_msgs_received_total",
			Help:      "Total number of messages received on a queue by a specific application/connection",
		},
		[]string{"queue_manager", "queue_name", "application_name", "source_ip", "user_identifier"},
	)

	// Per-queue per-application messages sent
	c.queueAppMsgsSentGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_app_msgs_sent_total",
			Help:      "Total number of messages sent on a queue by a specific application/connection",
		},
		[]string{"queue_manager", "queue_name", "application_name", "source_ip", "user_identifier"},
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
			Help:      "Input handles open on a queue with connection details (userid, pid, channel, appltag, conname)",
		},
		[]string{"queue_manager", "queue_name", "userid", "pid", "channel", "appltag", "conname"},
	)

	c.queueOutputCountGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_output_handles",
			Help:      "Output handles open on a queue with connection details (userid, pid, channel, appltag, conname)",
		},
		[]string{"queue_manager", "queue_name", "userid", "pid", "channel", "appltag", "conname"},
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

	// Initialize MQI metrics map with all 59 metrics
	c.mqiMetrics = make(map[string]*prometheus.GaugeVec)

	mqlLabels := []string{"queue_manager", "queue_name", "application_name", "application_tag", "user_identifier", "connection_name", "channel_name"}

	// Core MQI operations
	mqlMetricDefs := map[string]string{
		"opens":                "Total number of MQI OPEN operations",
		"closes":               "Total number of MQI CLOSE operations",
		"puts":                 "Total number of MQI PUT operations",
		"gets":                 "Total number of MQI GET operations",
		"commits":              "Total number of MQI COMMIT operations",
		"backouts":             "Total number of MQI BACKOUT operations",
		"browses":              "Total number of MQI BROWSE operations",
		"inqs":                 "Total number of MQI INQUIRE operations",
		"sets":                 "Total number of MQI SET operations",
		"disc_close_timeout":   "Number of disconnections due to close timeout",
		"disc_reset_timeout":   "Number of disconnections due to reset timeout",
		"fails":                "Total number of MQI failures",
		"incomplete_batch":     "Number of incomplete batches",
		"incomplete_msg":       "Number of incomplete messages",
		"wait_interval":        "Wait interval",
		"syncpoint_heuristic":  "Syncpoint heuristic decisions",
		"heaps":                "Number of heaps allocated",
		"logical_connections":  "Number of logical connections",
		"physical_connections": "Number of physical connections",
		"current_conns":        "Current connection count",
		"persistent_msgs":      "Number of persistent messages",
		"non_persistent_msgs":  "Number of non-persistent messages",
		"long_msgs":            "Number of long messages",
		"short_msgs":           "Number of short messages",
		"stamp_enabled":        "Timestamp enabled (1=yes, 0=no)",
		"msgs_received":        "Total number of messages received",
		"msgs_sent":            "Total number of messages sent",
		"channel_status":       "Channel status (0=running, 1=stopped, etc.)",
		"channel_type":         "Channel type",
		"channel_errors":       "Number of channel errors",
		"channel_disc_count":   "Number of channel disconnections",
		"channel_exit_name":    "Channel exit name",
		"full_batches":         "Number of full batches",
		"partial_batches":      "Number of partial batches",
	}

	// Integer64 metrics (time-based, counters)
	mqiInt64Metrics := map[string]string{
		"queue_time":       "Total queue time in milliseconds",
		"queue_time_max":   "Maximum queue time in milliseconds",
		"elapsed_time":     "Total elapsed time in milliseconds",
		"elapsed_time_max": "Maximum elapsed time in milliseconds",
		"conn_time":        "Total connection time in milliseconds",
		"conn_time_max":    "Maximum connection time in milliseconds",
		"bytes_received":   "Total number of bytes received",
		"bytes_sent":       "Total number of bytes sent",
		"backout_count":    "Total number of backouts",
		"commits_count":    "Total number of commits",
		"rollback_count":   "Total number of rollbacks",
	}

	// Create gauge vectors for all MQI metrics
	for metricName, help := range mqlMetricDefs {
		c.mqiMetrics[metricName] = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "mqi_" + metricName,
				Help:      help,
			},
			mqlLabels,
		)
	}

	for metricName, help := range mqiInt64Metrics {
		c.mqiMetrics[metricName] = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "mqi_" + metricName,
				Help:      help,
			},
			mqlLabels,
		)
	}

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

	// Handle-level metrics (application/process details on queues)
	c.queueHandleCountGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_handle_count",
			Help:      "Total number of open handles (processes) on a queue",
		},
		[]string{"queue_manager", "queue_name", "handle_type"},
	)

	c.queueHandleDetailsGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_handle_info",
			Help:      "Information about open handles on a queue (application name, user, mode)",
		},
		[]string{"queue_manager", "queue_name", "application_name", "user_identifier", "open_mode", "handle_state"},
	)

	// Process detail metric with PID
	c.queueProcessDetailGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_process_detail",
			Help:      "Details of processes (readers/writers) accessing a queue (PID, application name, user, role)",
		},
		[]string{"queue_manager", "queue_name", "application_name", "process_id", "user_identifier", "role"},
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
		c.queueProcessGauge,
		c.queueAppPutsGauge,
		c.queueAppGetsGauge,
		c.queueAppMsgsReceivedGauge,
		c.queueAppMsgsSentGauge,
		c.queueHandleCountGauge,
		c.queueHandleDetailsGauge,
		c.queueProcessDetailGauge,
		c.channelBytesGauge,
		c.channelBatchesGauge,
		c.collectionInfoGauge,
		c.lastCollectionTime,
	)

	// Register all MQI metrics from the map
	for _, metric := range c.mqiMetrics {
		c.registry.MustRegister(metric)
	}
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

	// Collect and update queue-specific metrics via MQINQ
	c.collectAndUpdateQueueMetrics()

	// Collect and update handle-level metrics (application/process details)
	c.collectAndUpdateHandleMetrics()

	// Update collection timestamp and baseline metrics
	c.setBaselineMetrics(len(statsMessages), len(accountingMessages))

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
	// Reset per-queue process metrics for this collection
	if c.queueProcessGauge != nil {
		c.queueProcessGauge.Reset()
	}

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

// collectAndUpdateQueueMetrics collects queue-specific metrics via MQINQ API
// and updates the queue depth and open count metrics
func (c *MetricsCollector) collectAndUpdateQueueMetrics() {
	if !c.mqClient.IsConnected() {
		c.logger.Debug("MQ client not connected, skipping MQINQ queue collection")
		return
	}

	// List of queues to monitor - we'll start with some common ones
	// In production, you might want to discover these dynamically
	queuesToMonitor := []string{
		"TEST.QUEUE",                    // Our test queue
		"SYSTEM.ADMIN.STATISTICS.QUEUE", // System queue for stats
		"SYSTEM.ADMIN.ACCOUNTING.QUEUE", // System queue for accounting
	}

	for _, queueName := range queuesToMonitor {
		stats, err := c.mqClient.GetQueueStats(queueName)
		if err != nil {
			c.logger.WithError(err).WithField("queue_name", queueName).Debug("Failed to get queue stats")
			continue
		}

		// Update queue depth metric
		if c.queueDepthGauge != nil {
			c.queueDepthGauge.WithLabelValues(
				c.config.MQ.QueueManager,
				stats.QueueName,
			).Set(float64(stats.CurrentDepth))
		}

		c.logger.WithFields(logrus.Fields{
			"queue_name":    stats.QueueName,
			"current_depth": stats.CurrentDepth,
			"input_count":   stats.OpenInputCount,
			"output_count":  stats.OpenOutputCount,
		}).Debug("Updated queue metrics via MQINQ")
	}
}

// getQueuesToMonitor returns the list of queues to monitor based on configuration
// If MonitorAllQueues is true, returns all queues except those matching exclusion patterns
// If MonitorAllQueues is false, returns the configured list of queues
func (c *MetricsCollector) getQueuesToMonitor() []string {
	if !c.config.Collector.MonitorAllQueues {
		// If MonitorAllQueues is false, return empty for now
		// In future, could support explicit queue lists
		return []string{}
	}

	// If MonitorAllQueues is true, get all queues and filter by exclusion patterns
	// For now, use a set of default queues that can be monitored

	defaultQueues := []string{
		"TEST.QUEUE",
		"TEST.Q1",
		"TEST.Q2",
		"TEST.Q3",
	}

	// Filter by exclusion patterns
	var queuesToMonitor []string
	for _, queueName := range defaultQueues {
		if !mqclient.QueueMatchesExclusionPattern(queueName, c.config.Collector.QueueExclusionPatterns) {
			queuesToMonitor = append(queuesToMonitor, queueName)
		}
	}

	return queuesToMonitor
}

// collectAndUpdateHandleMetrics collects handle counts for monitored queues
// Note: Detailed handle information (userid, pid, channel, appltag, conname) may not be
// available via PCF in client-binding scenarios. This method populates counts via MQINQ
// with empty labels, which is the most reliable approach for remote collectors.
func (c *MetricsCollector) collectAndUpdateHandleMetrics() {
	if !c.mqClient.IsConnected() {
		return
	}

	// Build list of queues to monitor
	queuesToMonitor := c.getQueuesToMonitor()
	if len(queuesToMonitor) == 0 {
		c.logger.Debug("No queues to monitor for handle metrics")
		return
	}

	c.logger.WithField("queue_count", len(queuesToMonitor)).Debug("Starting handle metrics collection")

	for _, queueName := range queuesToMonitor {
		// Get queue information via MQINQ - this works reliably via client bindings
		queueInfo, err := c.mqClient.GetQueueInfo(queueName)
		if err != nil {
			c.logger.WithError(err).WithField("queue_name", queueName).Debug("Failed to get queue info")
			continue
		}

		c.logger.WithFields(logrus.Fields{
			"queue_name":     queueName,
			"input_handles":  queueInfo.OpenInputCount,
			"output_handles": queueInfo.OpenOutputCount,
		}).Debug("Retrieved queue handle counts")

		// Populate metrics with handle counts and empty detail labels
		// In client-binding scenarios, detailed handle info is not available via PCF
		if queueInfo.OpenInputCount > 0 && c.queueInputCountGauge != nil {
			c.queueInputCountGauge.WithLabelValues(
				c.config.MQ.QueueManager,
				queueName,
				"", // userid not available in client binding
				"", // pid not available in client binding
				"", // channel not available in client binding
				"", // appltag not available in client binding
				"", // conname not available in client binding
			).Set(float64(queueInfo.OpenInputCount))
		}

		if queueInfo.OpenOutputCount > 0 && c.queueOutputCountGauge != nil {
			c.queueOutputCountGauge.WithLabelValues(
				c.config.MQ.QueueManager,
				queueName,
				"", // userid not available in client binding
				"", // pid not available in client binding
				"", // channel not available in client binding
				"", // appltag not available in client binding
				"", // conname not available in client binding
			).Set(float64(queueInfo.OpenOutputCount))
		}
	}
}

// updateHandleMetricsFromStatistics enriches handle metrics with actual application details from statistics data
// This populates handle information with real reader/writer process data including PIDs
func (c *MetricsCollector) updateHandleMetricsFromStatistics(stats *pcf.StatisticsData) {
	if stats.QueueStats == nil {
		return
	}

	qmgr := stats.QueueManager
	if qmgr == "" {
		qmgr = c.config.MQ.QueueManager
	}

	queueName := stats.QueueStats.QueueName

	// Cache the process information for this queue
	c.mu.Lock()
	c.queueProcsCache[queueName] = stats.QueueStats.AssociatedProcs
	c.mu.Unlock()

	c.logger.WithFields(logrus.Fields{
		"queue_name":             queueName,
		"associated_procs_count": len(stats.QueueStats.AssociatedProcs),
	}).Info("Cached queue process information from statistics")

	// Use the associated processes from statistics as actual handle information
	// These represent real readers (input) and writers (output) detected by IBM MQ
	for _, proc := range stats.QueueStats.AssociatedProcs {
		// Determine handle type based on role from statistics
		openMode := "Unknown"
		role := proc.Role
		if proc.Role == "input" {
			openMode = "Input"
		} else if proc.Role == "output" {
			openMode = "Output"
		}

		// Export actual process details from statistics with handle info
		if c.queueHandleDetailsGauge != nil {
			c.queueHandleDetailsGauge.WithLabelValues(
				qmgr,
				queueName,
				proc.ApplicationName,
				proc.UserIdentifier,
				openMode,
				"Open", // Process detected by statistics is active
			).Set(1)
		}

		// Export detailed process information with PID
		if c.queueProcessDetailGauge != nil {
			pidStr := fmt.Sprintf("%d", proc.ProcessID)
			c.queueProcessDetailGauge.WithLabelValues(
				qmgr,
				queueName,
				proc.ApplicationName,
				pidStr,
				proc.UserIdentifier,
				role,
			).Set(1)
		}

		// Export detailed handle metrics with connection information
		pidStr := fmt.Sprintf("%d", proc.ProcessID)
		if proc.Role == "input" && c.queueInputCountGauge != nil {
			c.queueInputCountGauge.WithLabelValues(
				qmgr,
				queueName,
				proc.UserIdentifier,
				pidStr,
				proc.ChannelName,
				proc.ApplicationTag,
				proc.ConnectionName,
			).Set(1)
		} else if proc.Role == "output" && c.queueOutputCountGauge != nil {
			c.queueOutputCountGauge.WithLabelValues(
				qmgr,
				queueName,
				proc.UserIdentifier,
				pidStr,
				proc.ChannelName,
				proc.ApplicationTag,
				proc.ConnectionName,
			).Set(1)
		}

		c.logger.WithFields(logrus.Fields{
			"queue_name": queueName,
			"app_name":   proc.ApplicationName,
			"pid":        proc.ProcessID,
			"user":       proc.UserIdentifier,
			"role":       proc.Role,
		}).Debug("Updated process details from statistics")
	}
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
		// Handle metrics populated from detailed statistics data in updateHandleMetricsFromStatistics()

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

		// Export associated processes (ipprocs/opprocs) if provided by parser
		for _, proc := range queueStats.AssociatedProcs {
			app := proc.ApplicationName
			src := proc.ConnectionName
			role := proc.Role
			if role == "" {
				role = "unknown"
			}

			if c.queueProcessGauge != nil {
				c.queueProcessGauge.WithLabelValues(qmgr, queueStats.QueueName, app, src, role).Set(1)
			}
		}

		// Update handle metrics with actual process data from statistics
		c.updateHandleMetricsFromStatistics(stats)
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
		labels := []string{
			qmgr,
			"unknown", // queue_name - statistics messages don't have queue context
			mqiStats.ApplicationName,
			mqiStats.ApplicationTag,
			mqiStats.UserIdentifier,
			mqiStats.ConnectionName,
			mqiStats.ChannelName,
		}

		// Set all MQI metrics from the map
		if gauge, ok := c.mqiMetrics["opens"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.Opens))
		}
		if gauge, ok := c.mqiMetrics["closes"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.Closes))
		}
		if gauge, ok := c.mqiMetrics["puts"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.Puts))
		}
		if gauge, ok := c.mqiMetrics["gets"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.Gets))
		}
		if gauge, ok := c.mqiMetrics["commits"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.Commits))
		}
		if gauge, ok := c.mqiMetrics["backouts"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.Backouts))
		}
		if gauge, ok := c.mqiMetrics["browses"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.Browses))
		}
		if gauge, ok := c.mqiMetrics["inqs"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.Inqs))
		}
		if gauge, ok := c.mqiMetrics["sets"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.Sets))
		}
		if gauge, ok := c.mqiMetrics["disc_close_timeout"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.DiscCloseTimeout))
		}
		if gauge, ok := c.mqiMetrics["disc_reset_timeout"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.DiscResetTimeout))
		}
		if gauge, ok := c.mqiMetrics["fails"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.Fails))
		}
		if gauge, ok := c.mqiMetrics["incomplete_batch"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.IncompleteBatch))
		}
		if gauge, ok := c.mqiMetrics["incomplete_msg"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.IncompleteMsg))
		}
		if gauge, ok := c.mqiMetrics["wait_interval"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.WaitInterval))
		}
		if gauge, ok := c.mqiMetrics["syncpoint_heuristic"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.SyncpointHeuristic))
		}
		if gauge, ok := c.mqiMetrics["heaps"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.Heaps))
		}
		if gauge, ok := c.mqiMetrics["logical_connections"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.LogicalConnections))
		}
		if gauge, ok := c.mqiMetrics["physical_connections"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.PhysicalConnections))
		}
		if gauge, ok := c.mqiMetrics["current_conns"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.CurrentConns))
		}
		if gauge, ok := c.mqiMetrics["persistent_msgs"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.PersistentMsgs))
		}
		if gauge, ok := c.mqiMetrics["non_persistent_msgs"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.NonPersistentMsgs))
		}
		if gauge, ok := c.mqiMetrics["long_msgs"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.LongMsgs))
		}
		if gauge, ok := c.mqiMetrics["short_msgs"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.ShortMsgs))
		}
		if gauge, ok := c.mqiMetrics["stamp_enabled"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.StampEnabled))
		}
		if gauge, ok := c.mqiMetrics["msgs_received"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.MsgsReceived))
		}
		if gauge, ok := c.mqiMetrics["msgs_sent"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.MsgsSent))
		}
		if gauge, ok := c.mqiMetrics["channel_status"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.ChannelStatus))
		}
		if gauge, ok := c.mqiMetrics["channel_type"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.ChannelType))
		}
		if gauge, ok := c.mqiMetrics["channel_errors"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.ChannelErrors))
		}
		if gauge, ok := c.mqiMetrics["channel_disc_count"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.ChannelDiscCount))
		}
		if gauge, ok := c.mqiMetrics["channel_exit_name"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.ChannelExitName))
		}
		if gauge, ok := c.mqiMetrics["full_batches"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.FullBatches))
		}
		if gauge, ok := c.mqiMetrics["partial_batches"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.PartialBatches))
		}

		// Int64 metrics
		if gauge, ok := c.mqiMetrics["queue_time"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.QueueTime))
		}
		if gauge, ok := c.mqiMetrics["queue_time_max"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.QueueTimeMax))
		}
		if gauge, ok := c.mqiMetrics["elapsed_time"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.ElapsedTime))
		}
		if gauge, ok := c.mqiMetrics["elapsed_time_max"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.ElapsedTimeMax))
		}
		if gauge, ok := c.mqiMetrics["conn_time"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.ConnTime))
		}
		if gauge, ok := c.mqiMetrics["conn_time_max"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.ConnTimeMax))
		}
		if gauge, ok := c.mqiMetrics["bytes_received"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.BytesReceived))
		}
		if gauge, ok := c.mqiMetrics["bytes_sent"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.BytesSent))
		}
		if gauge, ok := c.mqiMetrics["backout_count"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.BackoutCount))
		}
		if gauge, ok := c.mqiMetrics["commits_count"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.CommitsCount))
		}
		if gauge, ok := c.mqiMetrics["rollback_count"]; ok {
			gauge.WithLabelValues(labels...).Set(float64(mqiStats.RollbackCount))
		}
	}
}

// processAccountingMessage processes a single accounting message
func (c *MetricsCollector) processAccountingMessage(msg *mqclient.MQMessage) {
	data, err := c.pcfParser.ParseMessage(msg.Data, "accounting")
	if err != nil {
		c.logger.WithError(err).Debug("Failed to parse accounting message")
		return
	}

	acct, ok := data.(*pcf.AccountingData)
	if !ok {
		// If it's not accounting data, it might be MQI-level or some other format
		// Just debug log and return instead of error
		c.logger.WithField("data_type", fmt.Sprintf("%T", data)).Debug("Message parsed as non-accounting data type")
		return
	}

	qmgr := acct.QueueManager
	if qmgr == "" {
		qmgr = c.config.MQ.QueueManager
	}

	// Update MQI operation counts from accounting data
	if ops := acct.Operations; ops != nil {
		appName := ""
		appTag := ""
		userID := ""
		connName := ""
		chanName := ""

		if acct.ConnectionInfo != nil {
			appName = acct.ConnectionInfo.ApplicationName
			appTag = acct.ConnectionInfo.ApplicationTag
			userID = acct.ConnectionInfo.UserIdentifier
			connName = acct.ConnectionInfo.ConnectionName
			chanName = acct.ConnectionInfo.ChannelName
		}

		// When we don't have queue-level data, use "unknown" for queue_name to indicate connection-level aggregate stats
		labels := []string{qmgr, "unknown", appName, appTag, userID, connName, chanName}

		if gauge, ok := c.mqiMetrics["opens"]; ok {
			gauge.WithLabelValues(labels...).Add(float64(ops.Opens))
		}
		if gauge, ok := c.mqiMetrics["closes"]; ok {
			gauge.WithLabelValues(labels...).Add(float64(ops.Closes))
		}
		if gauge, ok := c.mqiMetrics["puts"]; ok {
			gauge.WithLabelValues(labels...).Add(float64(ops.Puts))
		}
		if gauge, ok := c.mqiMetrics["gets"]; ok {
			gauge.WithLabelValues(labels...).Add(float64(ops.Gets))
		}
		if gauge, ok := c.mqiMetrics["commits"]; ok {
			gauge.WithLabelValues(labels...).Add(float64(ops.Commits))
		}
		if gauge, ok := c.mqiMetrics["backouts"]; ok {
			gauge.WithLabelValues(labels...).Add(float64(ops.Backouts))
		}
	}

	// Update per-queue per-application operation counts (from accounting groups)
	// Also update core MQI metrics with queue context when available
	if acct.QueueOperations != nil {
		for _, qa := range acct.QueueOperations {
			qname := qa.QueueName
			app := qa.ApplicationName
			src := qa.ConnectionName
			user := qa.UserIdentifier

			// Labels for per-queue metrics
			queueLabels := []string{qmgr, qname, app, src, user}

			// Update per-queue specialized metrics
			if c.queueAppPutsGauge != nil {
				c.queueAppPutsGauge.WithLabelValues(queueLabels...).Set(float64(qa.Puts))
			}
			if c.queueAppGetsGauge != nil {
				c.queueAppGetsGauge.WithLabelValues(queueLabels...).Set(float64(qa.Gets))
			}
			if c.queueAppMsgsReceivedGauge != nil {
				c.queueAppMsgsReceivedGauge.WithLabelValues(queueLabels...).Set(float64(qa.MsgsReceived))
			}
			if c.queueAppMsgsSentGauge != nil {
				c.queueAppMsgsSentGauge.WithLabelValues(queueLabels...).Set(float64(qa.MsgsSent))
			}

			// Also update core MQI metrics with queue context
			// Labels for MQI metrics: queue_manager, queue_name, application_name, application_tag, user_identifier, connection_name, channel_name
			appTag := ""
			chanName := ""
			if acct.ConnectionInfo != nil {
				appTag = acct.ConnectionInfo.ApplicationTag
				chanName = acct.ConnectionInfo.ChannelName
			}
			mqlabels := []string{qmgr, qname, app, appTag, user, src, chanName}

			if gauge, ok := c.mqiMetrics["puts"]; ok && qa.Puts > 0 {
				gauge.WithLabelValues(mqlabels...).Set(float64(qa.Puts))
			}
			if gauge, ok := c.mqiMetrics["gets"]; ok && qa.Gets > 0 {
				gauge.WithLabelValues(mqlabels...).Set(float64(qa.Gets))
			}
		}
	}
}

// GetRegistry returns the Prometheus registry
func (c *MetricsCollector) GetRegistry() *prometheus.Registry {
	return c.registry
}

// setBaselineMetrics sets basic metrics that should always be present
func (c *MetricsCollector) setBaselineMetrics(statsCount, accountingCount int) {
	qmgr := c.config.MQ.QueueManager

	// Always set collection timestamp
	c.lastCollectionTime.WithLabelValues(qmgr).Set(float64(time.Now().Unix()))

	// Set collection info metric
	c.collectionInfoGauge.WithLabelValues(qmgr, c.config.MQ.Channel, "1.0.0").Set(1)
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

	// Reset all MQI metrics
	for _, gauge := range c.mqiMetrics {
		gauge.Reset()
	}

	c.logger.Info("Reset all metrics")
}
