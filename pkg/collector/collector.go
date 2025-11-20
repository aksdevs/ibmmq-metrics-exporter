package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/skatul/ibmmq-go-stat-otel/internal/otel"
	"github.com/skatul/ibmmq-go-stat-otel/pkg/config"
	"github.com/skatul/ibmmq-go-stat-otel/pkg/mqclient"
	"github.com/skatul/ibmmq-go-stat-otel/pkg/pcf"
	"github.com/skatul/ibmmq-go-stat-otel/pkg/prometheus"
)

// Collector is the main IBM MQ statistics collector
type Collector struct {
	config              *config.Config
	logger              *logrus.Logger
	mqClient            *mqclient.MQClient
	pcfParser           *pcf.Parser
	prometheusCollector *prometheus.MetricsCollector
	otelProvider        *otel.OTelProvider

	// Runtime state
	running        bool
	cycleCount     int
	lastCollection time.Time

	// Collection statistics
	totalStatsMessages      int64
	totalAccountingMessages int64
	totalCollections        int64
	errorCount              int64
}

// NewCollector creates a new IBM MQ statistics collector
func NewCollector(cfg *config.Config, logger *logrus.Logger) (*Collector, error) {
	// Create MQ client
	mqClient := mqclient.NewMQClient(&cfg.MQ, logger)

	// Create PCF parser
	pcfParser := pcf.NewParser(logger)

	// Create Prometheus collector
	prometheusCollector := prometheus.NewMetricsCollector(cfg, mqClient, logger)

	// Create OpenTelemetry provider if enabled
	var otelProvider *otel.OTelProvider
	var err error
	if cfg.Prometheus.EnableOTel {
		otelProvider, err = otel.NewOTelProvider(cfg, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTel provider: %w", err)
		}
	}

	collector := &Collector{
		config:              cfg,
		logger:              logger,
		mqClient:            mqClient,
		pcfParser:           pcfParser,
		prometheusCollector: prometheusCollector,
		otelProvider:        otelProvider,
		running:             false,
		cycleCount:          0,
	}

	logger.WithFields(logrus.Fields{
		"queue_manager": cfg.MQ.QueueManager,
		"channel":       cfg.MQ.Channel,
		"otel_enabled":  cfg.Prometheus.EnableOTel,
	}).Info("Created IBM MQ statistics collector")

	return collector, nil
}

// Start starts the collector and begins collecting metrics
func (c *Collector) Start(ctx context.Context) error {
	if c.running {
		return fmt.Errorf("collector is already running")
	}

	c.logger.Info("Starting IBM MQ statistics collector")

	// Connect to IBM MQ
	if err := c.mqClient.Connect(); err != nil {
		return fmt.Errorf("failed to connect to IBM MQ: %w", err)
	}

	// Open statistics queue
	if err := c.mqClient.OpenStatsQueue(c.config.Collector.StatsQueue); err != nil {
		c.logger.WithError(err).Warn("Failed to open statistics queue, continuing without it")
	}

	// Open accounting queue
	if err := c.mqClient.OpenAccountingQueue(c.config.Collector.AccountingQueue); err != nil {
		c.logger.WithError(err).Warn("Failed to open accounting queue, continuing without it")
	}

	// Start OpenTelemetry HTTP server if enabled
	if c.otelProvider != nil {
		if err := c.otelProvider.StartHTTPServer(ctx); err != nil {
			return fmt.Errorf("failed to start OTel HTTP server: %w", err)
		}
	}

	c.running = true

	// Start collection based on configuration
	if c.config.Collector.Continuous {
		return c.runContinuous(ctx)
	} else {
		return c.runOnce(ctx)
	}
}

// Stop stops the collector
func (c *Collector) Stop(ctx context.Context) error {
	if !c.running {
		return nil
	}

	c.logger.Info("Stopping IBM MQ statistics collector")
	c.running = false

	// Shutdown OpenTelemetry provider
	if c.otelProvider != nil {
		if err := c.otelProvider.Shutdown(ctx); err != nil {
			c.logger.WithError(err).Error("Error shutting down OTel provider")
		}
	}

	// Disconnect from IBM MQ
	if err := c.mqClient.Disconnect(); err != nil {
		c.logger.WithError(err).Error("Error disconnecting from IBM MQ")
		return err
	}

	c.logger.WithFields(logrus.Fields{
		"total_collections":         c.totalCollections,
		"total_stats_messages":      c.totalStatsMessages,
		"total_accounting_messages": c.totalAccountingMessages,
		"error_count":               c.errorCount,
	}).Info("IBM MQ statistics collector stopped")

	return nil
}

// runOnce executes a single collection cycle
func (c *Collector) runOnce(ctx context.Context) error {
	c.logger.Info("Running single collection cycle")

	err := c.collectMetrics(ctx)
	if err != nil {
		c.errorCount++
		return fmt.Errorf("collection failed: %w", err)
	}

	c.logger.Info("Single collection cycle completed successfully")
	return nil
}

// runContinuous runs continuous collection based on configured interval
func (c *Collector) runContinuous(ctx context.Context) error {
	c.logger.WithFields(logrus.Fields{
		"interval":   c.config.Collector.Interval,
		"max_cycles": c.config.Collector.MaxCycles,
	}).Info("Starting continuous collection")

	ticker := time.NewTicker(c.config.Collector.Interval)
	defer ticker.Stop()

	// Run initial collection immediately
	if err := c.collectMetrics(ctx); err != nil {
		c.logger.WithError(err).Error("Initial collection failed")
		c.errorCount++
	}

	for c.running {
		select {
		case <-ctx.Done():
			c.logger.Info("Context cancelled, stopping continuous collection")
			return ctx.Err()

		case <-ticker.C:
			if err := c.collectMetrics(ctx); err != nil {
				c.logger.WithError(err).Error("Collection cycle failed")
				c.errorCount++
				// Continue running even if a cycle fails
			}

			c.cycleCount++

			// Check if we've reached maximum cycles
			if c.config.Collector.MaxCycles > 0 && c.cycleCount >= c.config.Collector.MaxCycles {
				c.logger.WithField("cycles", c.cycleCount).Info("Reached maximum cycles, stopping")
				c.running = false
				return nil
			}
		}
	}

	return nil
}

// collectMetrics performs a single metrics collection cycle
func (c *Collector) collectMetrics(ctx context.Context) error {
	c.logger.Debug("Starting metrics collection cycle")
	startTime := time.Now()

	// Collect from Prometheus collector
	if err := c.prometheusCollector.CollectMetrics(ctx); err != nil {
		return fmt.Errorf("prometheus collection failed: %w", err)
	}

	// Get messages for OTel processing if enabled
	if c.otelProvider != nil {
		if err := c.collectForOTel(ctx); err != nil {
			c.logger.WithError(err).Error("OTel collection failed")
			// Don't return error, continue with prometheus-only collection
		}
	}

	c.totalCollections++
	c.lastCollection = time.Now()

	duration := time.Since(startTime)
	c.logger.WithFields(logrus.Fields{
		"duration":          duration,
		"cycle_count":       c.cycleCount,
		"total_collections": c.totalCollections,
	}).Info("Metrics collection cycle completed")

	// Reset statistics if configured
	if c.config.Collector.ResetStats {
		c.logger.Debug("Resetting statistics as configured")
		// Note: Actual MQ statistics reset would require additional MQ administration commands
		// This is a placeholder for that functionality
	}

	return nil
}

// collectForOTel collects and records metrics specifically for OpenTelemetry
func (c *Collector) collectForOTel(ctx context.Context) error {
	// Get statistics messages
	statsMessages, err := c.mqClient.GetAllMessages("stats")
	if err != nil {
		return fmt.Errorf("failed to get stats messages: %w", err)
	}

	// Get accounting messages
	accountingMessages, err := c.mqClient.GetAllMessages("accounting")
	if err != nil {
		return fmt.Errorf("failed to get accounting messages: %w", err)
	}

	c.totalStatsMessages += int64(len(statsMessages))
	c.totalAccountingMessages += int64(len(accountingMessages))

	// Process statistics messages for OTel
	for _, msg := range statsMessages {
		if err := c.processStatsMessageForOTel(ctx, msg); err != nil {
			c.logger.WithError(err).Error("Failed to process stats message for OTel")
		}
	}

	// Process accounting messages for OTel
	for _, msg := range accountingMessages {
		if err := c.processAccountingMessageForOTel(ctx, msg); err != nil {
			c.logger.WithError(err).Error("Failed to process accounting message for OTel")
		}
	}

	// Force flush metrics
	if err := c.otelProvider.ForceFlush(ctx); err != nil {
		c.logger.WithError(err).Error("Failed to flush OTel metrics")
	}

	return nil
}

// processStatsMessageForOTel processes a statistics message for OpenTelemetry
func (c *Collector) processStatsMessageForOTel(ctx context.Context, msg *mqclient.MQMessage) error {
	data, err := c.pcfParser.ParseMessage(msg.Data, "statistics")
	if err != nil {
		return fmt.Errorf("failed to parse statistics message: %w", err)
	}

	stats, ok := data.(*pcf.StatisticsData)
	if !ok {
		return fmt.Errorf("invalid statistics data type")
	}

	qmgr := stats.QueueManager
	if qmgr == "" {
		qmgr = c.config.MQ.QueueManager
	}

	// Record queue metrics
	if queueStats := stats.QueueStats; queueStats != nil {
		c.otelProvider.RecordQueueMetrics(
			ctx,
			qmgr,
			queueStats.QueueName,
			int64(queueStats.CurrentDepth),
			int64(queueStats.EnqueueCount),
			int64(queueStats.DequeueCount),
		)
	}

	// Record channel metrics
	if channelStats := stats.ChannelStats; channelStats != nil {
		c.otelProvider.RecordChannelMetrics(
			ctx,
			qmgr,
			channelStats.ChannelName,
			channelStats.ConnectionName,
			int64(channelStats.Messages),
			channelStats.Bytes,
		)
	}

	// Record MQI metrics
	if mqiStats := stats.MQIStats; mqiStats != nil {
		c.otelProvider.RecordMQIMetrics(ctx, qmgr, mqiStats.ApplicationName, "opens", int64(mqiStats.Opens))
		c.otelProvider.RecordMQIMetrics(ctx, qmgr, mqiStats.ApplicationName, "closes", int64(mqiStats.Closes))
		c.otelProvider.RecordMQIMetrics(ctx, qmgr, mqiStats.ApplicationName, "puts", int64(mqiStats.Puts))
		c.otelProvider.RecordMQIMetrics(ctx, qmgr, mqiStats.ApplicationName, "gets", int64(mqiStats.Gets))
		c.otelProvider.RecordMQIMetrics(ctx, qmgr, mqiStats.ApplicationName, "commits", int64(mqiStats.Commits))
		c.otelProvider.RecordMQIMetrics(ctx, qmgr, mqiStats.ApplicationName, "backouts", int64(mqiStats.Backouts))
	}

	return nil
}

// processAccountingMessageForOTel processes an accounting message for OpenTelemetry
func (c *Collector) processAccountingMessageForOTel(ctx context.Context, msg *mqclient.MQMessage) error {
	data, err := c.pcfParser.ParseMessage(msg.Data, "accounting")
	if err != nil {
		return fmt.Errorf("failed to parse accounting message: %w", err)
	}

	acct, ok := data.(*pcf.AccountingData)
	if !ok {
		return fmt.Errorf("invalid accounting data type")
	}

	qmgr := acct.QueueManager
	if qmgr == "" {
		qmgr = c.config.MQ.QueueManager
	}

	// Record MQI operation metrics from accounting data
	if ops := acct.Operations; ops != nil {
		appName := ""
		if acct.ConnectionInfo != nil {
			appName = acct.ConnectionInfo.ApplicationName
		}

		c.otelProvider.RecordMQIMetrics(ctx, qmgr, appName, "opens", int64(ops.Opens))
		c.otelProvider.RecordMQIMetrics(ctx, qmgr, appName, "closes", int64(ops.Closes))
		c.otelProvider.RecordMQIMetrics(ctx, qmgr, appName, "puts", int64(ops.Puts))
		c.otelProvider.RecordMQIMetrics(ctx, qmgr, appName, "gets", int64(ops.Gets))
		c.otelProvider.RecordMQIMetrics(ctx, qmgr, appName, "commits", int64(ops.Commits))
		c.otelProvider.RecordMQIMetrics(ctx, qmgr, appName, "backouts", int64(ops.Backouts))
	}

	return nil
}

// GetStats returns collection statistics
func (c *Collector) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"running":                   c.running,
		"cycle_count":               c.cycleCount,
		"last_collection":           c.lastCollection,
		"total_collections":         c.totalCollections,
		"total_stats_messages":      c.totalStatsMessages,
		"total_accounting_messages": c.totalAccountingMessages,
		"error_count":               c.errorCount,
		"queue_manager":             c.config.MQ.QueueManager,
		"channel":                   c.config.MQ.Channel,
	}
}

// IsRunning returns true if the collector is currently running
func (c *Collector) IsRunning() bool {
	return c.running
}
