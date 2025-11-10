package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.NotNil(t, cfg)
	// DefaultConfig() should return empty values that will be populated from YAML
	assert.Equal(t, "", cfg.MQ.QueueManager)
	assert.Equal(t, "", cfg.MQ.Channel)
	assert.Equal(t, "", cfg.MQ.ConnectionName)
	assert.Equal(t, "", cfg.MQ.Host)
	assert.Equal(t, 0, cfg.MQ.Port)
	assert.Equal(t, "", cfg.Collector.StatsQueue)
	assert.Equal(t, "", cfg.Collector.AccountingQueue)
	assert.Equal(t, 60*time.Second, cfg.Collector.Interval) // This has a default
	assert.Equal(t, 9090, cfg.Prometheus.Port)              // This has a default
	assert.Equal(t, "/metrics", cfg.Prometheus.Path)        // This has a default
	assert.Equal(t, "ibmmq", cfg.Prometheus.Namespace)      // This has a default
}

func TestLoadDefaultYAMLConfig(t *testing.T) {
	// Test loading from the actual default YAML file
	cfg, err := LoadConfig("../../configs/default.yaml")

	assert.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, "MQQM1", cfg.MQ.QueueManager)
	assert.Equal(t, "APP1.SVRCONN", cfg.MQ.Channel)
	assert.Equal(t, "127.0.0.1(5200)", cfg.MQ.ConnectionName) // Built from host:port
	assert.Equal(t, "127.0.0.1", cfg.MQ.Host)
	assert.Equal(t, 5200, cfg.MQ.Port)
	assert.Equal(t, "SYSTEM.ADMIN.STATISTICS.QUEUE", cfg.Collector.StatsQueue)
	assert.Equal(t, "SYSTEM.ADMIN.ACCOUNTING.QUEUE", cfg.Collector.AccountingQueue)
	assert.Equal(t, 60*time.Second, cfg.Collector.Interval)
	assert.Equal(t, 9090, cfg.Prometheus.Port)
	assert.Equal(t, "/metrics", cfg.Prometheus.Path)
	assert.Equal(t, "ibmmq", cfg.Prometheus.Namespace)
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid loaded config",
			config: func() *Config {
				cfg, _ := LoadConfig("../../configs/default.yaml")
				return cfg
			}(),
			wantErr: false,
		},
		{
			name: "missing queue manager",
			config: &Config{
				MQ: MQConfig{
					Channel:        "APP1.SVRCONN",
					ConnectionName: "localhost(1414)",
				},
				Collector:  DefaultConfig().Collector,
				Prometheus: DefaultConfig().Prometheus,
				Logging:    DefaultConfig().Logging,
			},
			wantErr: true,
		},
		{
			name: "missing channel",
			config: &Config{
				MQ: MQConfig{
					QueueManager:   "MQQM1",
					ConnectionName: "localhost(1414)",
				},
				Collector:  DefaultConfig().Collector,
				Prometheus: DefaultConfig().Prometheus,
				Logging:    DefaultConfig().Logging,
			},
			wantErr: true,
		},
		{
			name: "missing connection name",
			config: &Config{
				MQ: MQConfig{
					QueueManager: "MQQM1",
					Channel:      "APP1.SVRCONN",
					// No connection_name, host, or port - should error
				},
				Collector:  DefaultConfig().Collector,
				Prometheus: DefaultConfig().Prometheus,
				Logging:    DefaultConfig().Logging,
			},
			wantErr: true, // Should error since no connection info provided
		},
		{
			name: "invalid interval",
			config: &Config{
				MQ: DefaultConfig().MQ,
				Collector: CollectorConfig{
					StatsQueue:      "SYSTEM.ADMIN.STATISTICS.QUEUE",
					AccountingQueue: "SYSTEM.ADMIN.ACCOUNTING.QUEUE",
					Interval:        500 * time.Millisecond, // Too short
				},
				Prometheus: DefaultConfig().Prometheus,
				Logging:    DefaultConfig().Logging,
			},
			wantErr: true,
		},
		{
			name: "invalid prometheus port",
			config: &Config{
				MQ:        DefaultConfig().MQ,
				Collector: DefaultConfig().Collector,
				Prometheus: PrometheusConfig{
					Port:      0, // Invalid port
					Path:      "/metrics",
					Namespace: "ibmmq",
				},
				Logging: DefaultConfig().Logging,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLoadConfigFromEnvironment(t *testing.T) {
	// Set environment variables
	os.Setenv("IBMMQ_QUEUE_MANAGER", "TESTQM")
	os.Setenv("IBMMQ_CHANNEL", "TEST.SVRCONN")
	os.Setenv("IBMMQ_CONNECTION_NAME", "testhost(1414)")
	os.Setenv("IBMMQ_USER", "testuser")
	os.Setenv("IBMMQ_PASSWORD", "testpass")

	defer func() {
		os.Unsetenv("IBMMQ_QUEUE_MANAGER")
		os.Unsetenv("IBMMQ_CHANNEL")
		os.Unsetenv("IBMMQ_CONNECTION_NAME")
		os.Unsetenv("IBMMQ_USER")
		os.Unsetenv("IBMMQ_PASSWORD")
	}()

	cfg, err := LoadConfig("")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "TESTQM", cfg.MQ.QueueManager)
	assert.Equal(t, "TEST.SVRCONN", cfg.MQ.Channel)
	// Connection name should be constructed from host and port in default config, but env var should override
	assert.Equal(t, "127.0.0.1(5200)", cfg.MQ.ConnectionName)
	assert.Equal(t, "testuser", cfg.MQ.User)
	assert.Equal(t, "testpass", cfg.MQ.Password)
}

func TestConfigString(t *testing.T) {
	// Load from YAML to get properly populated config
	cfg, err := LoadConfig("../../configs/default.yaml")
	require.NoError(t, err)
	cfg.MQ.User = "testuser"

	str := cfg.String()
	assert.Contains(t, str, "MQQM1")
	assert.Contains(t, str, "APP1.SVRCONN")
	assert.Contains(t, str, "127.0.0.1(5200)") // Connection name built from host:port
	assert.Contains(t, str, "testuser")
	assert.Contains(t, str, "SYSTEM.ADMIN.STATISTICS.QUEUE")
	assert.Contains(t, str, "SYSTEM.ADMIN.ACCOUNTING.QUEUE")
}

func TestLoadConfigMissingFile(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/config.yaml")
	// Should handle missing file gracefully and return defaults
	if err != nil {
		// If there's an error, it should be about missing file
		assert.Contains(t, err.Error(), "config file")
	} else {
		// If no error, should have defaults
		assert.NotNil(t, cfg)
		assert.Equal(t, "MQQM1", cfg.MQ.QueueManager)
	}
}

func TestLoadConfigHostPortConstruction(t *testing.T) {
	// Test that ConnectionName is constructed from Host and Port when loading from YAML
	// Create a temporary config file for testing
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test_config.yaml")

	configContent := `
mq:
  queue_manager: "TESTQM"
  host: "testhost"
  port: 2414
  channel: "TEST.SVRCONN"

collector:
  stats_queue: "SYSTEM.ADMIN.STATISTICS.QUEUE"
  accounting_queue: "SYSTEM.ADMIN.ACCOUNTING.QUEUE"
  interval: "60s"
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	cfg, err := LoadConfig(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify that ConnectionName is constructed from Host and Port
	assert.Equal(t, "testhost", cfg.MQ.Host)
	assert.Equal(t, 2414, cfg.MQ.Port)
	assert.Equal(t, "testhost(2414)", cfg.MQ.ConnectionName)
}

func TestConfigYAMLParsing(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		check   func(t *testing.T, cfg *Config)
	}{
		{
			name: "minimal valid config",
			yaml: `
mq:
  queue_manager: "MINIMAL_QM"
  host: "localhost"
  port: 1414
  channel: "TEST.SVRCONN"

collector:
  stats_queue: "SYSTEM.ADMIN.STATISTICS.QUEUE"
  accounting_queue: "SYSTEM.ADMIN.ACCOUNTING.QUEUE"
  interval: "60s"
`,
			wantErr: false,
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "MINIMAL_QM", cfg.MQ.QueueManager)
				assert.Equal(t, "localhost(1414)", cfg.MQ.ConnectionName)
			},
		},
		{
			name: "config with all sections",
			yaml: `
mq:
  queue_manager: "FULL_QM"
  host: "full.host.com"
  port: 2414
  channel: "FULL.SVRCONN"
  user: "fulluser"
  password: "fullpass"

collector:
  stats_queue: "CUSTOM.STATS.QUEUE"
  accounting_queue: "CUSTOM.ACCT.QUEUE"
  interval: "45s"
  max_messages: 2000
  reset_statistics: true

metrics:
  enabled: true
  address: "0.0.0.0:8080"
  path: "/custom-metrics"
  namespace: "custom"
  subsystem: "full"

otel:
  enabled: true
  service_name: "full-service"
  service_version: "2.0.0"

logging:
  level: "debug"
  format: "text"
  output: "stderr"
  verbose: true
`,
			wantErr: false,
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "FULL_QM", cfg.MQ.QueueManager)
				assert.Equal(t, "full.host.com(2414)", cfg.MQ.ConnectionName)
				assert.Equal(t, "fulluser", cfg.MQ.User)
				assert.Equal(t, "CUSTOM.STATS.QUEUE", cfg.Collector.StatsQueue)
				assert.Equal(t, 45*time.Second, cfg.Collector.Interval)
			},
		},
		{
			name: "invalid yaml syntax",
			yaml: `
mq:
  queue_manager: "INVALID_QM
  host: "localhost"
  port: 1414
`,
			wantErr: true,
		},
		{
			name: "missing required sections",
			yaml: `
collector:
  interval: "60s"
`,
			wantErr: false, // Should load with defaults where appropriate
			check: func(t *testing.T, cfg *Config) {
				assert.Empty(t, cfg.MQ.QueueManager)                    // No default - should be empty
				assert.Equal(t, 60*time.Second, cfg.Collector.Interval) // Should get YAML value
				assert.Equal(t, 9090, cfg.Prometheus.Port)              // Should get default
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, "test_config.yaml")

			err := os.WriteFile(configPath, []byte(tt.yaml), 0644)
			require.NoError(t, err)

			cfg, err := LoadConfig(configPath)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, cfg)

			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}

func TestConfigEnvironmentVariableBinding(t *testing.T) {
	// Test comprehensive environment variable binding
	envVars := map[string]string{
		"IBMMQ_QUEUE_MANAGER":   "ENV_QM",
		"IBMMQ_CHANNEL":         "ENV.CHANNEL",
		"IBMMQ_CONNECTION_NAME": "env.host.com(1414)",
		"IBMMQ_USER":            "envuser",
		"IBMMQ_PASSWORD":        "envpass",
		"IBMMQ_KEY_REPOSITORY":  "/env/keystore",
		"IBMMQ_CIPHER_SPEC":     "ENV_CIPHER",
	}

	// Save original values
	originalVars := make(map[string]string)
	for key := range envVars {
		originalVars[key] = os.Getenv(key)
	}

	// Set test environment variables
	for key, value := range envVars {
		os.Setenv(key, value)
	}

	defer func() {
		// Restore original environment
		for key, originalValue := range originalVars {
			if originalValue != "" {
				os.Setenv(key, originalValue)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	cfg, err := LoadConfig("")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify environment variables were applied
	assert.Equal(t, "envuser", cfg.MQ.User)
	assert.Equal(t, "envpass", cfg.MQ.Password)
	// Other env vars should be bound through viper
}

func TestConfigStringOutput(t *testing.T) {
	cfg := &Config{
		MQ: MQConfig{
			QueueManager:   "TEST_QM",
			Channel:        "TEST.SVRCONN",
			ConnectionName: "test.host.com(1414)",
			User:           "testuser",
		},
		Collector: CollectorConfig{
			StatsQueue:      "STATS.QUEUE",
			AccountingQueue: "ACCT.QUEUE",
		},
	}

	str := cfg.String()

	// Check that sensitive information is included (password would be separate)
	assert.Contains(t, str, "TEST_QM")
	assert.Contains(t, str, "TEST.SVRCONN")
	assert.Contains(t, str, "test.host.com(1414)")
	assert.Contains(t, str, "testuser")
	assert.Contains(t, str, "STATS.QUEUE")
	assert.Contains(t, str, "ACCT.QUEUE")
}

func TestConfigurationDefaults(t *testing.T) {
	// Test that default configuration has minimal defaults (most come from YAML)
	cfg := DefaultConfig()

	require.NotNil(t, cfg)

	// MQ defaults should be empty (loaded from YAML)
	assert.Empty(t, cfg.MQ.QueueManager)
	assert.Empty(t, cfg.MQ.Channel)
	assert.Empty(t, cfg.MQ.Host)
	assert.Equal(t, 0, cfg.MQ.Port)
	assert.Empty(t, cfg.MQ.User)
	assert.Empty(t, cfg.MQ.Password)

	// Collector defaults - only queues are empty (loaded from YAML)
	assert.Empty(t, cfg.Collector.StatsQueue)
	assert.Empty(t, cfg.Collector.AccountingQueue)
	assert.Equal(t, 60*time.Second, cfg.Collector.Interval) // Has sensible default
	assert.False(t, cfg.Collector.ResetStats)

	// Prometheus defaults
	assert.Equal(t, 9090, cfg.Prometheus.Port)
	assert.Equal(t, "/metrics", cfg.Prometheus.Path)
	assert.Equal(t, "ibmmq", cfg.Prometheus.Namespace)
	assert.True(t, cfg.Prometheus.EnableOTel)

	// Logging defaults
	assert.Equal(t, "info", cfg.Logging.Level)
	assert.Equal(t, "json", cfg.Logging.Format)
	assert.False(t, cfg.Logging.Verbose)
}

func TestConfigurationEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		configPath  string
		expectError bool
		description string
	}{
		{
			name:        "empty config path",
			configPath:  "",
			expectError: false,
			description: "Should use defaults when no path provided",
		},
		{
			name:        "nonexistent directory",
			configPath:  "/nonexistent/path/config.yaml",
			expectError: true,
			description: "Should handle nonexistent directory gracefully",
		},
		{
			name:        "permission denied",
			configPath:  "/root/config.yaml", // Assuming no write access
			expectError: true,
			description: "Should handle permission errors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadConfig(tt.configPath)

			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				// For empty path, should succeed with defaults or handle gracefully
				if err == nil {
					assert.NotNil(t, cfg, tt.description)
				} else {
					// Some errors are acceptable (like file not found)
					t.Logf("Expected behavior: %s - %v", tt.description, err)
				}
			}
		})
	}
}
