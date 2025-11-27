package otel

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/aksdevs/ibmmq-go-stat-otel/pkg/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

// OTelProvider manages OpenTelemetry metrics provider and Prometheus exporter
// For now, this is a simplified version that focuses on Prometheus integration
type OTelProvider struct {
	config   *config.Config
	logger   *logrus.Logger
	registry *prometheus.Registry
	server   *http.Server
}

// NewOTelProvider creates a new OpenTelemetry provider
func NewOTelProvider(cfg *config.Config, logger *logrus.Logger) (*OTelProvider, error) {
	provider := &OTelProvider{
		config:   cfg,
		logger:   logger,
		registry: prometheus.NewRegistry(),
	}

	logger.Info("OpenTelemetry provider initialized successfully")
	return provider, nil
}

// StartHTTPServer starts the Prometheus metrics HTTP server
func (p *OTelProvider) StartHTTPServer(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", p.config.Prometheus.Port)

	mux := http.NewServeMux()
	mux.Handle(p.config.Prometheus.Path, promhttp.HandlerFor(p.registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/health", p.healthHandler)
	mux.HandleFunc("/ready", p.readyHandler)

	p.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	p.logger.WithFields(logrus.Fields{
		"address": addr,
		"path":    p.config.Prometheus.Path,
	}).Info("Starting Prometheus metrics HTTP server")

	// Start server in a goroutine
	go func() {
		if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			p.logger.WithError(err).Error("Prometheus HTTP server failed")
		}
	}()

	// Wait for context cancellation to shutdown
	go func() {
		<-ctx.Done()
		p.logger.Info("Shutting down Prometheus HTTP server")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := p.server.Shutdown(shutdownCtx); err != nil {
			p.logger.WithError(err).Error("Error shutting down HTTP server")
		}
	}()

	return nil
}

// healthHandler returns health status
func (p *OTelProvider) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"healthy","timestamp":"%s"}`, time.Now().Format(time.RFC3339))
}

// readyHandler returns readiness status
func (p *OTelProvider) readyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ready","timestamp":"%s"}`, time.Now().Format(time.RFC3339))
}

// RecordQueueMetrics records queue-related metrics (simplified version)
func (p *OTelProvider) RecordQueueMetrics(ctx context.Context, queueManager, queueName string, depth, enqCount, deqCount int64) {
	// For now, this is a no-op - metrics are handled by the Prometheus collector
	p.logger.WithFields(logrus.Fields{
		"queue_manager": queueManager,
		"queue_name":    queueName,
		"depth":         depth,
		"enqueue_count": enqCount,
		"dequeue_count": deqCount,
	}).Debug("Recording queue metrics")
}

// RecordChannelMetrics records channel-related metrics (simplified version)
func (p *OTelProvider) RecordChannelMetrics(ctx context.Context, queueManager, channelName, connectionName string, messages, bytes int64) {
	p.logger.WithFields(logrus.Fields{
		"queue_manager":   queueManager,
		"channel_name":    channelName,
		"connection_name": connectionName,
		"messages":        messages,
		"bytes":           bytes,
	}).Debug("Recording channel metrics")
}

// RecordMQIMetrics records MQI operation metrics (simplified version)
func (p *OTelProvider) RecordMQIMetrics(ctx context.Context, queueManager, appName, operation string, count int64) {
	p.logger.WithFields(logrus.Fields{
		"queue_manager":    queueManager,
		"application_name": appName,
		"operation":        operation,
		"count":            count,
	}).Debug("Recording MQI metrics")
}

// GetRegistry returns the Prometheus registry for integration with existing collectors
func (p *OTelProvider) GetRegistry() *prometheus.Registry {
	return p.registry
}

// Shutdown gracefully shuts down the OTel provider
func (p *OTelProvider) Shutdown(ctx context.Context) error {
	p.logger.Info("Shutting down OpenTelemetry provider")

	if p.server != nil {
		if err := p.server.Shutdown(ctx); err != nil {
			p.logger.WithError(err).Error("Error shutting down HTTP server")
		}
	}

	p.logger.Info("OpenTelemetry provider shut down successfully")
	return nil
}

// ForceFlush forces a flush of all metrics (simplified version)
func (p *OTelProvider) ForceFlush(ctx context.Context) error {
	// No-op for now
	return nil
}
