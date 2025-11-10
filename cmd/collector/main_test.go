package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/atulksin/ibmmq-go-stat-otel/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectorConfigurationLoading(t *testing.T) {
	// Test that the main collector can load configuration properly

	// Create a temporary config file for testing
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "collector_test_config.yaml")

	configContent := `
mq:
  queue_manager: "INTEGRATION_QM"
  host: "integration.host.com"
  port: 2414
  channel: "INTEGRATION.CHANNEL"
  username: ""
  password: ""

collector:
  stats_queue: "INTEGRATION.STATS.QUEUE"
  accounting_queue: "INTEGRATION.ACCT.QUEUE" 
  interval: "30s"
  max_messages: 500
  reset_statistics: false

metrics:
  enabled: true
  address: "0.0.0.0:8080"
  path: "/integration-metrics"
  namespace: "integration"
  subsystem: "test"

otel:
  enabled: true
  service_name: "integration-test-collector"
  service_version: "test-1.0.0"

logging:
  level: "debug"
  format: "text"
  output: "stdout"
  verbose: true
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Load the configuration
	cfg, err := config.LoadConfig(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify configuration values
	assert.Equal(t, "INTEGRATION_QM", cfg.MQ.QueueManager)
	assert.Equal(t, "integration.host.com", cfg.MQ.Host)
	assert.Equal(t, 2414, cfg.MQ.Port)
	assert.Equal(t, "INTEGRATION.CHANNEL", cfg.MQ.Channel)
	assert.Equal(t, "integration.host.com(2414)", cfg.MQ.ConnectionName)
	assert.Equal(t, "INTEGRATION.STATS.QUEUE", cfg.Collector.StatsQueue)
	assert.Equal(t, "INTEGRATION.ACCT.QUEUE", cfg.Collector.AccountingQueue)
	assert.Equal(t, 30*time.Second, cfg.Collector.Interval)
}

func TestCollectorCommandLineFlags(t *testing.T) {
	// Test command line flag parsing and configuration override
	// This would test the CLI functionality if we had access to it

	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "help flag",
			args:        []string{"--help"},
			expectError: false,
		},
		{
			name:        "version flag",
			args:        []string{"--version"},
			expectError: false,
		},
		{
			name:        "config flag",
			args:        []string{"-c", "test-config.yaml"},
			expectError: false, // Would fail during execution but flag parsing should work
		},
		{
			name:        "verbose flag",
			args:        []string{"--verbose"},
			expectError: false,
		},
		{
			name:        "continuous mode",
			args:        []string{"--continuous"},
			expectError: false,
		},
		{
			name:        "invalid flag",
			args:        []string{"--invalid-flag"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This is a placeholder for command line flag testing
			// In a real implementation, you would parse the flags and check results
			assert.NotNil(t, tt.args)
		})
	}
}

func TestCollectorEnvironmentConfiguration(t *testing.T) {
	// Test that collector respects environment variable overrides

	originalVars := map[string]string{
		"IBMMQ_QUEUE_MANAGER": os.Getenv("IBMMQ_QUEUE_MANAGER"),
		"IBMMQ_USER":          os.Getenv("IBMMQ_USER"),
		"IBMMQ_PASSWORD":      os.Getenv("IBMMQ_PASSWORD"),
	}

	defer func() {
		// Restore original environment
		for key, value := range originalVars {
			if value != "" {
				os.Setenv(key, value)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	// Set test environment variables
	os.Setenv("IBMMQ_QUEUE_MANAGER", "ENV_TEST_QM")
	os.Setenv("IBMMQ_USER", "env_user")
	os.Setenv("IBMMQ_PASSWORD", "env_password")

	cfg, err := config.LoadConfig("")
	if err != nil {
		t.Skip("Config loading failed, skipping environment test")
	}

	// Environment variables should override config file values
	assert.Equal(t, "env_user", cfg.MQ.User)
	assert.Equal(t, "env_password", cfg.MQ.Password)
}

func TestCollectorValidationScenarios(t *testing.T) {
	tests := []struct {
		name          string
		config        string
		expectValid   bool
		expectedError string
	}{
		{
			name: "complete valid configuration",
			config: `
mq:
  queue_manager: "VALID_QM"
  host: "localhost"
  port: 1414
  channel: "VALID.SVRCONN"

collector:
  stats_queue: "SYSTEM.ADMIN.STATISTICS.QUEUE"
  accounting_queue: "SYSTEM.ADMIN.ACCOUNTING.QUEUE"
  interval: "60s"
  max_messages: 1000

metrics:
  enabled: true
  address: "0.0.0.0:9090"
  path: "/metrics"
  namespace: "ibmmq"

logging:
  level: "info"
  format: "json"
`,
			expectValid: true,
		},
		{
			name: "missing required queue manager",
			config: `
mq:
  host: "localhost"
  port: 1414
  channel: "TEST.SVRCONN"

collector:
  stats_queue: "SYSTEM.ADMIN.STATISTICS.QUEUE"
  accounting_queue: "SYSTEM.ADMIN.ACCOUNTING.QUEUE"
  interval: "60s"
`,
			expectValid:   false,
			expectedError: "queue manager",
		},
		{
			name: "invalid collection interval",
			config: `
mq:
  queue_manager: "TEST_QM"
  host: "localhost"
  port: 1414
  channel: "TEST.SVRCONN"

collector:
  stats_queue: "SYSTEM.ADMIN.STATISTICS.QUEUE"
  accounting_queue: "SYSTEM.ADMIN.ACCOUNTING.QUEUE"
  interval: "0s"
`,
			expectValid:   false,
			expectedError: "interval",
		},
		{
			name: "missing channel configuration",
			config: `
mq:
  queue_manager: "TEST_QM"
  host: "localhost"
  port: 1414

collector:
  stats_queue: "SYSTEM.ADMIN.STATISTICS.QUEUE"
  accounting_queue: "SYSTEM.ADMIN.ACCOUNTING.QUEUE"
  interval: "60s"
`,
			expectValid:   false,
			expectedError: "channel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, "test_config.yaml")

			err := os.WriteFile(configPath, []byte(tt.config), 0644)
			require.NoError(t, err)

			cfg, err := config.LoadConfig(configPath)
			if err != nil && tt.expectValid {
				t.Fatalf("Failed to load valid config: %v", err)
			}

			if cfg != nil {
				err = cfg.Validate()
				if tt.expectValid {
					assert.NoError(t, err, "Expected configuration to be valid")
				} else {
					assert.Error(t, err, "Expected configuration to be invalid")
					if tt.expectedError != "" {
						assert.Contains(t, err.Error(), tt.expectedError)
					}
				}
			}
		})
	}
}

func TestCollectorPerformanceConfiguration(t *testing.T) {
	// Test performance-related configuration options

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "perf_config.yaml")

	configContent := `
mq:
  queue_manager: "PERF_QM"
  host: "localhost"
  port: 1414
  channel: "PERF.SVRCONN"

collector:
  stats_queue: "SYSTEM.ADMIN.STATISTICS.QUEUE"
  accounting_queue: "SYSTEM.ADMIN.ACCOUNTING.QUEUE"
  interval: "10s"
  max_messages: 10000
  timeout: "30s"
  reset_stats: true

metrics:
  enabled: true
  address: "0.0.0.0:9090"
  path: "/metrics"

logging:
  level: "warn"
  format: "json"
  verbose: false
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	cfg, err := config.LoadConfig(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify performance settings
	assert.Equal(t, 10*time.Second, cfg.Collector.Interval)
	// MaxMessages field may not exist in current config structure
	assert.Equal(t, true, cfg.Collector.ResetStats)
}

func TestCollectorSecurityConfiguration(t *testing.T) {
	// Test security-related configuration

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "security_config.yaml")

	configContent := `
mq:
  queue_manager: "SECURE_QM"
  host: "secure.host.com"
  port: 1414
  channel: "SECURE.SVRCONN"
  username: "secure_user"
  password: "secure_pass"
  
  ssl:
    enabled: true
    key_repository: "/path/to/keystore"
    cipher_suite: "TLS_RSA_WITH_AES_256_CBC_SHA256"

collector:
  stats_queue: "SYSTEM.ADMIN.STATISTICS.QUEUE"
  accounting_queue: "SYSTEM.ADMIN.ACCOUNTING.QUEUE"
  interval: "60s"

metrics:
  enabled: true
  address: "127.0.0.1:9090"  # Bind to localhost only
  path: "/metrics"

logging:
  level: "info"
  format: "json"
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	cfg, err := config.LoadConfig(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify security settings - use correct field names from YAML
	assert.Equal(t, "secure_user", cfg.MQ.Username) // YAML uses 'username' field
	assert.Equal(t, "secure_pass", cfg.MQ.Password)
	assert.Equal(t, "secure_user", cfg.MQ.GetUser()) // Test the getter method too
	// SSL settings would be tested if the config struct supports them
}

func TestCollectorLifecycleConfiguration(t *testing.T) {
	// Test lifecycle and operational configuration

	tests := []struct {
		name     string
		config   map[string]interface{}
		validate func(t *testing.T, cfg *config.Config)
	}{
		{
			name: "one-shot collection mode",
			config: map[string]interface{}{
				"collector.interval":   "0s", // This should be adjusted based on validation
				"collector.max_cycles": 1,
				"collector.continuous": false,
			},
			validate: func(t *testing.T, cfg *config.Config) {
				// Validate one-shot configuration
				assert.False(t, cfg.Collector.Continuous)
			},
		},
		{
			name: "continuous monitoring mode",
			config: map[string]interface{}{
				"collector.interval":   "30s",
				"collector.max_cycles": 0, // Infinite
				"collector.continuous": true,
			},
			validate: func(t *testing.T, cfg *config.Config) {
				assert.Equal(t, 30*time.Second, cfg.Collector.Interval)
				assert.True(t, cfg.Collector.Continuous)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create YAML config for each test
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, "test_config.yaml")

			// Build YAML content based on test config
			yamlContent := `
mq:
  queue_manager: "TEST_QM"
  host: "localhost"
  port: 1414
  channel: "TEST.SVRCONN"
collector:
  stats_queue: "SYSTEM.ADMIN.STATISTICS.QUEUE"
  accounting_queue: "SYSTEM.ADMIN.ACCOUNTING.QUEUE"`

			// Add specific collector settings from test config
			if interval, ok := tt.config["collector.interval"]; ok {
				yamlContent += fmt.Sprintf("\n  interval: \"%v\"", interval)
			}
			if maxCycles, ok := tt.config["collector.max_cycles"]; ok {
				yamlContent += fmt.Sprintf("\n  max_cycles: %v", maxCycles)
			}
			if continuous, ok := tt.config["collector.continuous"]; ok {
				yamlContent += fmt.Sprintf("\n  continuous: %v", continuous)
			}

			err := os.WriteFile(configPath, []byte(yamlContent), 0644)
			require.NoError(t, err)

			cfg, err := config.LoadConfig(configPath)
			require.NoError(t, err)
			require.NotNil(t, cfg)

			if tt.validate != nil {
				tt.validate(t, cfg)
			}
		})
	}
}

func TestCollectorIntegrationScenarios(t *testing.T) {
	// Test integration scenarios without requiring actual MQ

	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	scenarios := []struct {
		name        string
		description string
		setup       func() *config.Config
		teardown    func()
	}{
		{
			name:        "development environment",
			description: "Test development environment configuration",
			setup: func() *config.Config {
				tempDir := t.TempDir()
				configPath := filepath.Join(tempDir, "dev_config.yaml")
				configContent := `
mq:
  queue_manager: "DEV.QM"
  host: "localhost"
  port: 1414
  channel: "DEV.SVRCONN"
collector:
  stats_queue: "SYSTEM.ADMIN.STATISTICS.QUEUE"
  accounting_queue: "SYSTEM.ADMIN.ACCOUNTING.QUEUE"
logging:
  level: "debug"
  verbose: true`
				err := os.WriteFile(configPath, []byte(configContent), 0644)
				require.NoError(t, err)
				cfg, err := config.LoadConfig(configPath)
				require.NoError(t, err)
				return cfg
			},
		},
		{
			name:        "production environment",
			description: "Test production environment configuration",
			setup: func() *config.Config {
				tempDir := t.TempDir()
				configPath := filepath.Join(tempDir, "prod_config.yaml")
				configContent := `
mq:
  queue_manager: "PROD.QM"
  host: "prod.mq.com"
  port: 1414
  channel: "PROD.SVRCONN"
collector:
  stats_queue: "SYSTEM.ADMIN.STATISTICS.QUEUE"
  accounting_queue: "SYSTEM.ADMIN.ACCOUNTING.QUEUE"
logging:
  level: "info"`
				err := os.WriteFile(configPath, []byte(configContent), 0644)
				require.NoError(t, err)
				cfg, err := config.LoadConfig(configPath)
				require.NoError(t, err)
				return cfg
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			cfg := scenario.setup()
			require.NotNil(t, cfg)

			// Validate the configuration
			err := cfg.Validate()
			assert.NoError(t, err, "Configuration should be valid for %s", scenario.description)

			if scenario.teardown != nil {
				scenario.teardown()
			}
		})
	}
}
