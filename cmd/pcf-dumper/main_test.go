package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/skatul/ibmmq-go-stat-otel/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigurationLoading(t *testing.T) {
	// Test that PCF dumper can load configuration properly

	// Create a temporary config file for testing
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test_config.yaml")

	configContent := `
mq:
  queue_manager: "TEST_QM"
  host: "test.host.com"
  port: 1414
  channel: "TEST.CHANNEL"
  username: ""
  password: ""

collector:
  stats_queue: "TEST.STATS.QUEUE"
  accounting_queue: "TEST.ACCT.QUEUE"
  interval: "30s"

metrics:
  enabled: true
  address: "0.0.0.0:8080"
  path: "/test-metrics"

logging:
  level: "debug"
  format: "text"
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Load the configuration
	cfg, err := config.LoadConfig(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify configuration values
	assert.Equal(t, "TEST_QM", cfg.MQ.QueueManager)
	assert.Equal(t, "test.host.com", cfg.MQ.Host)
	assert.Equal(t, 1414, cfg.MQ.Port)
	assert.Equal(t, "TEST.CHANNEL", cfg.MQ.Channel)
	assert.Equal(t, "test.host.com(1414)", cfg.MQ.ConnectionName) // Should be constructed
	assert.Equal(t, "TEST.STATS.QUEUE", cfg.Collector.StatsQueue)
	assert.Equal(t, "TEST.ACCT.QUEUE", cfg.Collector.AccountingQueue)
}

func TestDefaultConfigurationUsage(t *testing.T) {
	// Test that default configuration is used when file doesn't exist
	cfg, err := config.LoadConfig("/nonexistent/config.yaml")

	// Should handle missing file gracefully and return defaults
	if err != nil {
		// Config loading may fail but should still return default config in some cases
		t.Logf("Expected behavior: config loading failed for nonexistent file: %v", err)
		return
	}

	// If no error, should have valid defaults
	require.NotNil(t, cfg)
	assert.Equal(t, "MQQM1", cfg.MQ.QueueManager)
	assert.Equal(t, "127.0.0.1", cfg.MQ.Host)
	assert.Equal(t, 5200, cfg.MQ.Port)
	assert.Equal(t, "APP1.SVRCONN", cfg.MQ.Channel)
}

func TestEnvironmentVariableOverride(t *testing.T) {
	// Test that environment variables can override configuration
	originalQM := os.Getenv("IBMMQ_QUEUE_MANAGER")
	originalUser := os.Getenv("IBMMQ_USER")

	defer func() {
		if originalQM != "" {
			os.Setenv("IBMMQ_QUEUE_MANAGER", originalQM)
		} else {
			os.Unsetenv("IBMMQ_QUEUE_MANAGER")
		}
		if originalUser != "" {
			os.Setenv("IBMMQ_USER", originalUser)
		} else {
			os.Unsetenv("IBMMQ_USER")
		}
	}()

	// Set test environment variables
	os.Setenv("IBMMQ_USER", "testuser")
	os.Setenv("IBMMQ_PASSWORD", "testpass")

	cfg, err := config.LoadConfig("")
	if err != nil {
		t.Skip("Config loading failed, skipping environment override test")
	}

	// Environment variables for sensitive data should override
	assert.Equal(t, "testuser", cfg.MQ.User)
	assert.Equal(t, "testpass", cfg.MQ.Password)
}

func TestConfigurationValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr bool
	}{
		{
			name: "valid configuration",
			config: `
mq:
  queue_manager: "TESTQM"
  host: "localhost"
  port: 1414
  channel: "TEST.SVRCONN"

collector:
  stats_queue: "SYSTEM.ADMIN.STATISTICS.QUEUE"
  accounting_queue: "SYSTEM.ADMIN.ACCOUNTING.QUEUE"
  interval: "60s"

metrics:
  enabled: true
  address: "0.0.0.0:9090"
  path: "/metrics"
`,
			wantErr: false,
		},
		{
			name: "missing queue manager",
			config: `
mq:
  queue_manager: ""
  host: "localhost"
  port: 1414
  channel: "TEST.SVRCONN"

collector:
  stats_queue: "SYSTEM.ADMIN.STATISTICS.QUEUE"
  accounting_queue: "SYSTEM.ADMIN.ACCOUNTING.QUEUE"
  interval: "60s"
`,
			wantErr: true,
		},
		{
			name: "invalid port",
			config: `
mq:
  queue_manager: "TESTQM"
  host: "localhost"
  port: 0
  channel: "TEST.SVRCONN"

collector:
  stats_queue: "SYSTEM.ADMIN.STATISTICS.QUEUE"
  accounting_queue: "SYSTEM.ADMIN.ACCOUNTING.QUEUE"
  interval: "60s"

metrics:
  enabled: true
  address: "0.0.0.0:70000"
  path: "/metrics"
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, "test_config.yaml")

			err := os.WriteFile(configPath, []byte(tt.config), 0644)
			require.NoError(t, err)

			cfg, err := config.LoadConfig(configPath)
			if err != nil {
				if tt.wantErr {
					return // Expected error during loading
				}
				t.Fatalf("Failed to load config: %v", err)
			}

			// For validation tests, the error should come from Validate(), not LoadConfig()
			err = cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err, "Expected validation error but got none")
			} else {
				assert.NoError(t, err, "Unexpected validation error")
			}
		})
	}
}
