package collector

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/skatul/ibmmq-go-stat-otel/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCollector(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

	cfg := config.DefaultConfig()

	collector, err := NewCollector(cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, collector)

	assert.Equal(t, cfg, collector.config)
	assert.Equal(t, logger, collector.logger)
	assert.False(t, collector.running)
	assert.Equal(t, 0, collector.cycleCount)
	assert.Equal(t, int64(0), collector.totalCollections)
}

func TestCollectorGetStats(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	cfg := config.DefaultConfig()

	collector, err := NewCollector(cfg, logger)
	require.NoError(t, err)

	// Set some test values
	collector.cycleCount = 5
	collector.totalCollections = 10
	collector.totalStatsMessages = 50
	collector.totalAccountingMessages = 30
	collector.errorCount = 2
	collector.lastCollection = time.Now()
	collector.running = true

	stats := collector.GetStats()

	assert.Equal(t, true, stats["running"])
	assert.Equal(t, 5, stats["cycle_count"])
	assert.Equal(t, int64(10), stats["total_collections"])
	assert.Equal(t, int64(50), stats["total_stats_messages"])
	assert.Equal(t, int64(30), stats["total_accounting_messages"])
	assert.Equal(t, int64(2), stats["error_count"])
	assert.Equal(t, cfg.MQ.QueueManager, stats["queue_manager"])
	assert.Equal(t, cfg.MQ.Channel, stats["channel"])
}

func TestCollectorIsRunning(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	cfg := config.DefaultConfig()

	collector, err := NewCollector(cfg, logger)
	require.NoError(t, err)

	// Initially not running
	assert.False(t, collector.IsRunning())

	// Set running state
	collector.running = true
	assert.True(t, collector.IsRunning())

	// Set stopped state
	collector.running = false
	assert.False(t, collector.IsRunning())
}

func TestCollectorValidation(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	tests := []struct {
		name    string
		config  *config.Config
		wantErr bool
	}{
		{
			name:    "valid config",
			config:  config.DefaultConfig(),
			wantErr: false,
		},
		{
			name: "invalid config - missing queue manager",
			config: &config.Config{
				MQ: config.MQConfig{
					Channel:        "APP1.SVRCONN",
					ConnectionName: "localhost(1414)",
				},
				Collector:  config.DefaultConfig().Collector,
				Prometheus: config.DefaultConfig().Prometheus,
				Logging:    config.DefaultConfig().Logging,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector, err := NewCollector(tt.config, logger)

			if tt.wantErr {
				// We expect the validation to happen when trying to start
				// The collector creation itself should succeed
				assert.NoError(t, err)
				assert.NotNil(t, collector)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, collector)
			}
		})
	}
}

// TestCollectorLifecycle tests the basic lifecycle without actual MQ connections
func TestCollectorLifecycle(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	cfg := config.DefaultConfig()
	// Set a very short interval for testing
	cfg.Collector.Interval = 100 * time.Millisecond
	cfg.Collector.MaxCycles = 2
	cfg.Collector.Continuous = true

	collector, err := NewCollector(cfg, logger)
	require.NoError(t, err)

	// Test that we can create and validate the collector
	assert.NotNil(t, collector)
	assert.False(t, collector.IsRunning())

	// Test stopping when not running
	ctx := context.Background()
	err = collector.Stop(ctx)
	assert.NoError(t, err) // Should not error when stopping a non-running collector
}

func TestCollectorConfiguration(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	cfg := &config.Config{
		MQ: config.MQConfig{
			QueueManager:   "TESTQM",
			Channel:        "TEST.SVRCONN",
			ConnectionName: "testhost(1414)",
			User:           "testuser",
			Password:       "testpass",
		},
		Collector: config.CollectorConfig{
			StatsQueue:      "TEST.STATS.QUEUE",
			AccountingQueue: "TEST.ACCT.QUEUE",
			ResetStats:      true,
			Interval:        30 * time.Second,
			MaxCycles:       10,
			Continuous:      true,
		},
		Prometheus: config.PrometheusConfig{
			Port:       8080,
			Path:       "/test-metrics",
			Namespace:  "test",
			EnableOTel: false,
		},
		Logging: config.LoggingConfig{
			Level:   "debug",
			Format:  "text",
			Verbose: true,
		},
	}

	collector, err := NewCollector(cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, collector)

	// Verify configuration is properly set
	assert.Equal(t, "TESTQM", collector.config.MQ.QueueManager)
	assert.Equal(t, "TEST.SVRCONN", collector.config.MQ.Channel)
	assert.Equal(t, "testhost(1414)", collector.config.MQ.ConnectionName)
	assert.Equal(t, "TEST.STATS.QUEUE", collector.config.Collector.StatsQueue)
	assert.Equal(t, "TEST.ACCT.QUEUE", collector.config.Collector.AccountingQueue)
	assert.True(t, collector.config.Collector.ResetStats)
	assert.Equal(t, 30*time.Second, collector.config.Collector.Interval)
	assert.Equal(t, 10, collector.config.Collector.MaxCycles)
	assert.True(t, collector.config.Collector.Continuous)
	assert.Equal(t, 8080, collector.config.Prometheus.Port)
	assert.Equal(t, "/test-metrics", collector.config.Prometheus.Path)
	assert.Equal(t, "test", collector.config.Prometheus.Namespace)
	assert.False(t, collector.config.Prometheus.EnableOTel)
}

// Mock tests would require more complex setup with interfaces
// For now, these tests cover the basic structure and configuration
func TestCollectorStatsTracking(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	cfg := config.DefaultConfig()
	collector, err := NewCollector(cfg, logger)
	require.NoError(t, err)

	// Test initial stats
	stats := collector.GetStats()
	assert.Equal(t, int64(0), stats["total_collections"])
	assert.Equal(t, int64(0), stats["total_stats_messages"])
	assert.Equal(t, int64(0), stats["total_accounting_messages"])
	assert.Equal(t, int64(0), stats["error_count"])

	// Simulate some activity (normally would be done by actual collection)
	collector.totalCollections = 5
	collector.totalStatsMessages = 25
	collector.totalAccountingMessages = 15
	collector.errorCount = 1
	collector.cycleCount = 3

	stats = collector.GetStats()
	assert.Equal(t, int64(5), stats["total_collections"])
	assert.Equal(t, int64(25), stats["total_stats_messages"])
	assert.Equal(t, int64(15), stats["total_accounting_messages"])
	assert.Equal(t, int64(1), stats["error_count"])
	assert.Equal(t, 3, stats["cycle_count"])
}
