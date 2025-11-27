package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aksdevs/ibmmq-go-stat-otel/pkg/collector"
	"github.com/aksdevs/ibmmq-go-stat-otel/pkg/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	version = "1.0.0"
	commit  = "dev"
	date    = "unknown"
)

// Global flags
var (
	configFile     string
	verbose        bool
	logLevel       string
	logFormat      string
	continuous     bool
	interval       time.Duration
	maxCycles      int
	resetStats     bool
	prometheusPort int
	otelEnabled    bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "ibmmq-collector",
		Short: "IBM MQ Statistics and Accounting Collector for Prometheus",
		Long: `A Go-based collector that reads IBM MQ statistics and accounting data
from system queues and exposes them as Prometheus metrics with OpenTelemetry support.

This collector connects to IBM MQ, reads from SYSTEM.ADMIN.STATISTICS.QUEUE
and SYSTEM.ADMIN.ACCOUNTING.QUEUE, parses PCF messages, and exposes the
data as Prometheus metrics with the 'ibmmq' prefix.`,
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
		RunE:    runCollector,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "Configuration file path")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "json", "Log format (json, text)")

	// Collection flags
	rootCmd.Flags().BoolVar(&continuous, "continuous", false, "Run continuous monitoring")
	rootCmd.Flags().DurationVar(&interval, "interval", 60*time.Second, "Collection interval for continuous mode")
	rootCmd.Flags().IntVar(&maxCycles, "max-cycles", 0, "Maximum number of collection cycles (0 = infinite)")
	rootCmd.Flags().BoolVar(&resetStats, "reset-stats", false, "Reset statistics after reading")

	// Prometheus flags
	rootCmd.Flags().IntVar(&prometheusPort, "prometheus-port", 9090, "Prometheus metrics HTTP server port")
	rootCmd.Flags().BoolVar(&otelEnabled, "otel", true, "Enable OpenTelemetry integration")

	// Add subcommands
	rootCmd.AddCommand(createVersionCmd())
	rootCmd.AddCommand(createTestCmd())
	rootCmd.AddCommand(createConfigCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runCollector(cmd *cobra.Command, args []string) error {
	// Setup logging
	logger := setupLogger()

	logger.WithFields(logrus.Fields{
		"version": version,
		"commit":  commit,
		"date":    date,
	}).Info("Starting IBM MQ Statistics Collector")

	// Load configuration
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Override config with command line flags
	overrideConfigWithFlags(cfg)

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	logger.WithField("config", cfg.String()).Info("Configuration loaded successfully")

	// Create collector
	col, err := collector.NewCollector(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create collector: %w", err)
	}

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigChan

		logger.WithField("signal", sig).Info("Received shutdown signal")
		cancel()
	}()

	// Start collector
	logger.Info("Starting collector...")
	if err := col.Start(ctx); err != nil {
		if err == context.Canceled {
			logger.Info("Collector stopped by user")
			return nil
		}
		return fmt.Errorf("collector failed: %w", err)
	}

	// Stop collector
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := col.Stop(shutdownCtx); err != nil {
		logger.WithError(err).Error("Error during collector shutdown")
		return err
	}

	logger.Info("IBM MQ Statistics Collector stopped successfully")
	return nil
}

func setupLogger() *logrus.Logger {
	logger := logrus.New()

	// Set log level
	level, err := logrus.ParseLevel(logLevel)
	if err != nil {
		level = logrus.InfoLevel
	}
	if verbose {
		level = logrus.DebugLevel
	}
	logger.SetLevel(level)

	// Set log format
	switch logFormat {
	case "json":
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
		})
	case "text":
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: time.RFC3339,
		})
	default:
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
		})
	}

	return logger
}

func overrideConfigWithFlags(cfg *config.Config) {
	// Override with command line flags
	if continuous {
		cfg.Collector.Continuous = continuous
	}
	if interval != 60*time.Second {
		cfg.Collector.Interval = interval
	}
	if maxCycles != 0 {
		cfg.Collector.MaxCycles = maxCycles
	}
	if resetStats {
		cfg.Collector.ResetStats = resetStats
	}
	if prometheusPort != 9090 {
		cfg.Prometheus.Port = prometheusPort
	}
	cfg.Prometheus.EnableOTel = otelEnabled

	// Override logging config
	cfg.Logging.Verbose = verbose
	if logLevel != "info" {
		cfg.Logging.Level = logLevel
	}
	if logFormat != "json" {
		cfg.Logging.Format = logFormat
	}
}

func createVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("IBM MQ Statistics Collector\n")
			fmt.Printf("Version: %s\n", version)
			fmt.Printf("Commit: %s\n", commit)
			fmt.Printf("Built: %s\n", date)
		},
	}
}

func createTestCmd() *cobra.Command {
	testCmd := &cobra.Command{
		Use:   "test",
		Short: "Test IBM MQ connection and configuration",
		RunE:  runConnectionTest,
	}

	testCmd.Flags().StringVarP(&configFile, "config", "c", "", "Configuration file path")

	return testCmd
}

func createConfigCmd() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Configuration management commands",
	}

	generateCmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate sample configuration file",
		RunE:  generateConfig,
	}

	validateCmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration file",
		RunE:  validateConfig,
	}

	configCmd.AddCommand(generateCmd, validateCmd)
	return configCmd
}

func runConnectionTest(cmd *cobra.Command, args []string) error {
	logger := setupLogger()

	logger.Info("Testing IBM MQ connection")

	// Load configuration
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Create collector (this will test the connection)
	col, err := collector.NewCollector(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create collector: %w", err)
	}

	// Test connection by starting and immediately stopping
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a test collector that just connects and disconnects
	logger.Info("Attempting connection to IBM MQ...")

	// This is a simplified test - in practice you might want to create a separate test method
	go func() {
		time.Sleep(2 * time.Second) // Give it time to connect
		cancel()                    // Cancel to stop the test
	}()

	err = col.Start(ctx)
	if err != nil && err != context.Canceled {
		return fmt.Errorf("connection test failed: %w", err)
	}

	logger.Info("IBM MQ connection test completed successfully")
	return nil
}

func generateConfig(cmd *cobra.Command, args []string) error {
	cfg := config.DefaultConfig()

	// You would implement YAML marshaling here
	fmt.Println("# IBM MQ Statistics Collector Configuration")
	fmt.Println("# Save this as config.yaml")
	fmt.Println()
	fmt.Println("mq:")
	fmt.Printf("  queue_manager: %s\n", cfg.MQ.QueueManager)
	fmt.Printf("  channel: %s\n", cfg.MQ.Channel)
	fmt.Printf("  connection_name: %s\n", cfg.MQ.ConnectionName)
	fmt.Printf("  user: %s\n", cfg.MQ.User)
	fmt.Printf("  password: %s\n", cfg.MQ.Password)
	fmt.Println()
	fmt.Println("collector:")
	fmt.Printf("  stats_queue: %s\n", cfg.Collector.StatsQueue)
	fmt.Printf("  accounting_queue: %s\n", cfg.Collector.AccountingQueue)
	fmt.Printf("  reset_stats: %t\n", cfg.Collector.ResetStats)
	fmt.Printf("  interval: %s\n", cfg.Collector.Interval)
	fmt.Printf("  continuous: %t\n", cfg.Collector.Continuous)
	fmt.Println()
	fmt.Println("prometheus:")
	fmt.Printf("  port: %d\n", cfg.Prometheus.Port)
	fmt.Printf("  path: %s\n", cfg.Prometheus.Path)
	fmt.Printf("  namespace: %s\n", cfg.Prometheus.Namespace)
	fmt.Printf("  enable_otel: %t\n", cfg.Prometheus.EnableOTel)
	fmt.Println()
	fmt.Println("logging:")
	fmt.Printf("  level: %s\n", cfg.Logging.Level)
	fmt.Printf("  format: %s\n", cfg.Logging.Format)

	return nil
}

func validateConfig(cmd *cobra.Command, args []string) error {
	logger := setupLogger()

	if configFile == "" {
		return fmt.Errorf("configuration file path is required")
	}

	logger.WithField("config_file", configFile).Info("Validating configuration")

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	logger.Info("Configuration is valid")
	fmt.Printf("✓ Configuration file '%s' is valid\n", configFile)
	fmt.Printf("✓ Queue Manager: %s\n", cfg.MQ.QueueManager)
	fmt.Printf("✓ Channel: %s\n", cfg.MQ.Channel)
	fmt.Printf("✓ Connection: %s\n", cfg.MQ.ConnectionName)
	fmt.Printf("✓ Prometheus Port: %d\n", cfg.Prometheus.Port)

	return nil
}
