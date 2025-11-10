package config

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
)

// MQConfig holds IBM MQ connection configuration
type MQConfig struct {
	QueueManager   string `mapstructure:"queue_manager" yaml:"queue_manager" json:"queue_manager"`
	Channel        string `mapstructure:"channel" yaml:"channel" json:"channel"`
	ConnectionName string `mapstructure:"connection_name" yaml:"connection_name" json:"connection_name"`
	Host           string `mapstructure:"host" yaml:"host" json:"host"`
	Port           int    `mapstructure:"port" yaml:"port" json:"port"`
	User           string `mapstructure:"user" yaml:"user" json:"user"`
	Username       string `mapstructure:"username" yaml:"username" json:"username"` // Alternative field name
	Password       string `mapstructure:"password" yaml:"password" json:"password"`
	KeyRepository  string `mapstructure:"key_repository" yaml:"key_repository" json:"key_repository"`
	CipherSpec     string `mapstructure:"cipher_spec" yaml:"cipher_spec" json:"cipher_spec"`
}

// GetConnectionName returns the connection name, building it from host/port if connection_name is empty
func (m *MQConfig) GetConnectionName() string {
	if m.ConnectionName != "" {
		return m.ConnectionName
	}
	if m.Host != "" && m.Port > 0 {
		return fmt.Sprintf("%s(%d)", m.Host, m.Port)
	}
	return "127.0.0.1(5200)" // fallback default for our environment
}

// GetUser returns the user, preferring username over user field
func (m *MQConfig) GetUser() string {
	if m.Username != "" {
		return m.Username
	}
	return m.User
}

// CollectorConfig holds collector-specific configuration
type CollectorConfig struct {
	StatsQueue      string        `mapstructure:"stats_queue" yaml:"stats_queue" json:"stats_queue"`
	AccountingQueue string        `mapstructure:"accounting_queue" yaml:"accounting_queue" json:"accounting_queue"`
	ResetStats      bool          `mapstructure:"reset_stats" yaml:"reset_stats" json:"reset_stats"`
	Interval        time.Duration `mapstructure:"interval" yaml:"interval" json:"interval"`
	MaxCycles       int           `mapstructure:"max_cycles" yaml:"max_cycles" json:"max_cycles"`
	Continuous      bool          `mapstructure:"continuous" yaml:"continuous" json:"continuous"`
}

// PrometheusConfig holds Prometheus exporter configuration
type PrometheusConfig struct {
	Port       int    `mapstructure:"port" yaml:"port" json:"port"`
	Path       string `mapstructure:"path" yaml:"path" json:"path"`
	Namespace  string `mapstructure:"namespace" yaml:"namespace" json:"namespace"`
	Subsystem  string `mapstructure:"subsystem" yaml:"subsystem" json:"subsystem"`
	EnableOTel bool   `mapstructure:"enable_otel" yaml:"enable_otel" json:"enable_otel"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level      string `mapstructure:"level" yaml:"level" json:"level"`
	Format     string `mapstructure:"format" yaml:"format" json:"format"`
	OutputFile string `mapstructure:"output_file" yaml:"output_file" json:"output_file"`
	Verbose    bool   `mapstructure:"verbose" yaml:"verbose" json:"verbose"`
}

// Config holds the complete application configuration
type Config struct {
	MQ         MQConfig         `mapstructure:"mq" yaml:"mq" json:"mq"`
	Collector  CollectorConfig  `mapstructure:"collector" yaml:"collector" json:"collector"`
	Prometheus PrometheusConfig `mapstructure:"prometheus" yaml:"prometheus" json:"prometheus"`
	Logging    LoggingConfig    `mapstructure:"logging" yaml:"logging" json:"logging"`
}

// DefaultConfig returns a configuration with minimal defaults
func DefaultConfig() *Config {
	return &Config{
		MQ: MQConfig{
			QueueManager:   "MQQM1",
			Channel:        "APP1.SVRCONN",
			ConnectionName: "localhost(1414)",
			Host:           "127.0.0.1",
			Port:           5200,
			User:           "",
			Username:       "",
			Password:       "",
			KeyRepository:  "",
			CipherSpec:     "",
		},
		Collector: CollectorConfig{
			StatsQueue:      "", // Will be loaded from YAML
			AccountingQueue: "", // Will be loaded from YAML
			ResetStats:      false,
			Interval:        60 * time.Second,
			MaxCycles:       0, // 0 means infinite
			Continuous:      false,
		},
		Prometheus: PrometheusConfig{
			Port:       9090,
			Path:       "/metrics",
			Namespace:  "ibmmq",
			Subsystem:  "",
			EnableOTel: true,
		},
		Logging: LoggingConfig{
			Level:      "info",
			Format:     "json",
			OutputFile: "",
			Verbose:    false,
		},
	}
}

// LoadConfig loads configuration from file, environment variables, and defaults
func LoadConfig(configPath string) (*Config, error) {
	config := DefaultConfig()

	viper.SetConfigType("yaml")

	// Set configuration file path if provided
	if configPath != "" {
		viper.SetConfigFile(configPath)
	} else {
		// Look for config files in standard locations
		viper.SetConfigName("config")
		viper.AddConfigPath(".")
		viper.AddConfigPath("./config")
		viper.AddConfigPath("$HOME/.ibmmq-collector")
		viper.AddConfigPath("/etc/ibmmq-collector")
	}

	// Set environment variable prefix
	viper.SetEnvPrefix("IBMMQ")
	viper.AutomaticEnv()

	// Bind environment variables
	viper.BindEnv("mq.queue_manager", "IBMMQ_QUEUE_MANAGER")
	viper.BindEnv("mq.channel", "IBMMQ_CHANNEL")
	viper.BindEnv("mq.connection_name", "IBMMQ_CONNECTION_NAME")
	viper.BindEnv("mq.user", "IBMMQ_USER")
	viper.BindEnv("mq.password", "IBMMQ_PASSWORD")
	viper.BindEnv("mq.key_repository", "IBMMQ_KEY_REPOSITORY")
	viper.BindEnv("mq.cipher_spec", "IBMMQ_CIPHER_SPEC")

	// Read configuration file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found is okay, we'll use defaults and env vars
	}

	// Unmarshal configuration
	if err := viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Construct ConnectionName from Host and Port if not explicitly set
	if config.MQ.Host != "" && config.MQ.Port != 0 {
		config.MQ.ConnectionName = fmt.Sprintf("%s(%d)", config.MQ.Host, config.MQ.Port)
	}

	// Override with environment variables for sensitive data
	if user := os.Getenv("IBMMQ_USER"); user != "" {
		config.MQ.User = user
	}
	if password := os.Getenv("IBMMQ_PASSWORD"); password != "" {
		config.MQ.Password = password
	}

	return config, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.MQ.QueueManager == "" {
		return fmt.Errorf("queue manager name is required")
	}

	if c.MQ.Channel == "" {
		return fmt.Errorf("channel name is required")
	}

	if c.MQ.GetConnectionName() == "" {
		return fmt.Errorf("connection name is required (provide either connection_name or host/port)")
	}

	if c.Collector.Interval < time.Second {
		return fmt.Errorf("collection interval must be at least 1 second")
	}

	if c.Prometheus.Port < 1 || c.Prometheus.Port > 65535 {
		return fmt.Errorf("prometheus port must be between 1 and 65535")
	}

	return nil
}

// String returns a string representation of the config (without sensitive data)
func (c *Config) String() string {
	return fmt.Sprintf("QM: %s, Channel: %s, Connection: %s, User: %s, StatsQueue: %s, AccountingQueue: %s",
		c.MQ.QueueManager,
		c.MQ.Channel,
		c.MQ.ConnectionName,
		c.MQ.User,
		c.Collector.StatsQueue,
		c.Collector.AccountingQueue)
}
