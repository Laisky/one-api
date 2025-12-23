package monitor

import (
	"runtime"
	"time"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/metrics"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/monitor/otel"
	"github.com/songquanpeng/one-api/monitor/prometheus"
)

// InitMonitoring initializes all monitoring components
func InitMonitoring(version, buildTime, goVersion string, startTime time.Time) error {
	var recorders []metrics.MetricsRecorder

	// Set up the Prometheus recorder if enabled
	if config.EnablePrometheusMetrics {
		recorders = append(recorders, &prometheus.PrometheusRecorder{})
	}

	// Set up the OpenTelemetry recorder if enabled
	if config.OpenTelemetryEnabled {
		otelRecorder, err := otel.NewOtelRecorder()
		if err != nil {
			return err
		}
		recorders = append(recorders, otelRecorder)
	}

	if len(recorders) == 0 {
		metrics.GlobalRecorder = &metrics.NoOpRecorder{}
		return nil
	}

	if len(recorders) == 1 {
		metrics.GlobalRecorder = recorders[0]
	} else {
		metrics.GlobalRecorder = &metrics.MultiRecorder{Recorders: recorders}
	}

	// Initialize system metrics
	metrics.GlobalRecorder.InitSystemMetrics(version, buildTime, goVersion, startTime)

	// Start background metric collection
	go collectSystemMetrics()
	go collectChannelMetrics()
	go collectUserMetrics()
	go collectDashboardMetrics()

	return nil
}

// collectSystemMetrics collects system-wide metrics periodically
func collectSystemMetrics() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Update memory and runtime metrics (can be extended)
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		// You can add custom metrics for memory usage if needed
	}
}

// collectChannelMetrics collects channel-related metrics periodically
func collectChannelMetrics() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		// Channel metrics are now updated through the metrics interface
		// when actual requests are made
	}
}

// collectUserMetrics collects user-related metrics periodically
func collectUserMetrics() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		// User metrics are recorded per-request through the metrics interface
	}
}

// collectDashboardMetrics collects site-wide dashboard metrics periodically
func collectDashboardMetrics() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		totalQuota, usedQuota, _, err := model.GetSiteWideQuotaStats()
		if err != nil {
			continue
		}

		// Get user counts
		var totalUsers, activeUsers int64
		model.DB.Model(&model.User{}).Where("status != ?", model.UserStatusDeleted).Count(&totalUsers)
		model.DB.Model(&model.User{}).Where("status = ?", model.UserStatusEnabled).Count(&activeUsers)

		metrics.GlobalRecorder.UpdateSiteWideStats(totalQuota, usedQuota, int(totalUsers), int(activeUsers))
	}
}
