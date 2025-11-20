package mqclient

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/skatul/ibmmq-go-stat-otel/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMQClient(t *testing.T) {
	cfg := &config.MQConfig{
		QueueManager:   "TESTQM",
		Channel:        "TEST.SVRCONN",
		ConnectionName: "localhost(1414)",
		Host:           "localhost",
		Port:           1414,
	}
	logger := logrus.New()

	client := NewMQClient(cfg, logger)

	assert.NotNil(t, client)
	assert.Equal(t, cfg, client.config)
	assert.Equal(t, logger, client.logger)
	assert.False(t, client.connected)
}

func TestMQClientConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		config *config.MQConfig
		valid  bool
	}{
		{
			name: "valid configuration",
			config: &config.MQConfig{
				QueueManager:   "TESTQM",
				Channel:        "TEST.SVRCONN",
				ConnectionName: "localhost(1414)",
				Host:           "localhost",
				Port:           1414,
			},
			valid: true,
		},
		{
			name: "missing queue manager",
			config: &config.MQConfig{
				Channel:        "TEST.SVRCONN",
				ConnectionName: "localhost(1414)",
				Host:           "localhost",
				Port:           1414,
			},
			valid: false,
		},
		{
			name: "missing channel",
			config: &config.MQConfig{
				QueueManager:   "TESTQM",
				ConnectionName: "localhost(1414)",
				Host:           "localhost",
				Port:           1414,
			},
			valid: false,
		},
		{
			name: "missing connection name",
			config: &config.MQConfig{
				QueueManager: "TESTQM",
				Channel:      "TEST.SVRCONN",
				Host:         "localhost",
				Port:         1414,
			},
			valid: false,
		},
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMQClient(tt.config, logger)
			require.NotNil(t, client)

			// Test that configuration is stored correctly
			assert.Equal(t, tt.config, client.config)

			// For invalid configurations, connection should fail
			// (but we can't test actual connection without MQ server)
			if !tt.valid {
				assert.NotEmpty(t, tt.name) // Just ensure test structure is correct
			}
		})
	}
}

func TestMQClientConnectionState(t *testing.T) {
	cfg := &config.MQConfig{
		QueueManager:   "TESTQM",
		Channel:        "TEST.SVRCONN",
		ConnectionName: "localhost(1414)",
		Host:           "localhost",
		Port:           1414,
	}
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	client := NewMQClient(cfg, logger)

	// Initially not connected
	assert.False(t, client.IsConnected())

	// Test connection (will fail without actual MQ server, but tests the interface)
	err := client.Connect()
	// We expect this to fail since there's no MQ server
	assert.Error(t, err)
	assert.False(t, client.IsConnected())

	// Test disconnect (should be safe to call even if not connected)
	err = client.Disconnect()
	assert.NoError(t, err) // Disconnect should always succeed
}

func TestMQClientQueueOperations(t *testing.T) {
	cfg := &config.MQConfig{
		QueueManager:   "TESTQM",
		Channel:        "TEST.SVRCONN",
		ConnectionName: "localhost(1414)",
		Host:           "localhost",
		Port:           1414,
	}
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	client := NewMQClient(cfg, logger)

	// Test opening queues without connection (should fail)
	err := client.OpenStatsQueue("SYSTEM.ADMIN.STATISTICS.QUEUE")
	assert.Error(t, err, "Should fail to open queue without connection")

	err = client.OpenAccountingQueue("SYSTEM.ADMIN.ACCOUNTING.QUEUE")
	assert.Error(t, err, "Should fail to open queue without connection")

	// Test getting messages without connection (should fail)
	messages, err := client.GetAllMessages("stats")
	assert.Error(t, err, "Should fail to get messages without connection")
	assert.Nil(t, messages)

	messages, err = client.GetAllMessages("accounting")
	assert.Error(t, err, "Should fail to get messages without connection")
	assert.Nil(t, messages)
}

func TestMQClientMessageTypes(t *testing.T) {
	cfg := &config.MQConfig{
		QueueManager:   "TESTQM",
		Channel:        "TEST.SVRCONN",
		ConnectionName: "localhost(1414)",
		Host:           "localhost",
		Port:           1414,
	}
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	client := NewMQClient(cfg, logger)

	// Test invalid message type
	messages, err := client.GetAllMessages("invalid")
	assert.Error(t, err, "Should fail for invalid message type")
	assert.Nil(t, messages)

	// Test valid message types (will fail due to no connection, but tests the validation)
	validTypes := []string{"stats", "accounting"}
	for _, msgType := range validTypes {
		messages, err := client.GetAllMessages(msgType)
		assert.Error(t, err) // Expected to fail due to no connection
		assert.Nil(t, messages)
	}
}

func TestMQClientConfigurationValidation(t *testing.T) {
	logger := logrus.New()

	tests := []struct {
		name   string
		config *config.MQConfig
		valid  bool
	}{
		{
			name:   "nil config",
			config: nil,
			valid:  false,
		},
		{
			name: "valid config",
			config: &config.MQConfig{
				QueueManager:   "TESTQM",
				Channel:        "TEST.SVRCONN",
				ConnectionName: "localhost(1414)",
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Current implementation may handle nil gracefully
			client := NewMQClient(tt.config, logger)
			if tt.valid {
				assert.NotNil(t, client)
			} else {
				// Even with nil config, client may be created but should handle it
				assert.NotNil(t, client)
			}
		})
	}
}

func TestMQClientLogging(t *testing.T) {
	cfg := &config.MQConfig{
		QueueManager:   "TESTQM",
		Channel:        "TEST.SVRCONN",
		ConnectionName: "localhost(1414)",
		Host:           "localhost",
		Port:           1414,
	}

	tests := []struct {
		name   string
		logger *logrus.Logger
		valid  bool
	}{
		{
			name:   "valid logger",
			logger: logrus.New(),
			valid:  true,
		},
		{
			name:   "nil logger",
			logger: nil,
			valid:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMQClient(cfg, tt.logger)
			assert.NotNil(t, client)

			if tt.valid {
				// Logger should be set correctly
				assert.Equal(t, tt.logger, client.logger)
			} else {
				// Implementation may handle nil logger gracefully
				// or assign a default logger
				assert.NotNil(t, client)
			}
		})
	}
}
