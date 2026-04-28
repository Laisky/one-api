package model

import (
	"time"

	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/metrics"
)

// PrometheusDBHook implements GORM's plugin interface for monitoring database operations
type PrometheusDBHook struct{}

func (p *PrometheusDBHook) Name() string {
	return "prometheus-db-hook"
}

func (p *PrometheusDBHook) Initialize(db *gorm.DB) error {
	// Register callbacks for different database operations

	// Before callbacks to record start time
	db.Callback().Create().Before("gorm:create").Register("prometheus:before_create", p.beforeCallback)
	db.Callback().Query().Before("gorm:query").Register("prometheus:before_query", p.beforeCallback)
	db.Callback().Update().Before("gorm:update").Register("prometheus:before_update", p.beforeCallback)
	db.Callback().Delete().Before("gorm:delete").Register("prometheus:before_delete", p.beforeCallback)
	db.Callback().Row().Before("gorm:row").Register("prometheus:before_row", p.beforeCallback)
	db.Callback().Raw().Before("gorm:raw").Register("prometheus:before_raw", p.beforeCallback)

	// After callbacks to record metrics
	db.Callback().Create().After("gorm:create").Register("prometheus:after_create", p.afterCallback("create"))
	db.Callback().Query().After("gorm:query").Register("prometheus:after_query", p.afterCallback("query"))
	db.Callback().Update().After("gorm:update").Register("prometheus:after_update", p.afterCallback("update"))
	db.Callback().Delete().After("gorm:delete").Register("prometheus:after_delete", p.afterCallback("delete"))
	db.Callback().Row().After("gorm:row").Register("prometheus:after_row", p.afterCallback("row"))
	db.Callback().Raw().After("gorm:raw").Register("prometheus:after_raw", p.afterCallback("raw"))

	return nil
}

func (p *PrometheusDBHook) beforeCallback(db *gorm.DB) {
	db.Set("prometheus_start_time", time.Now())
}

func (p *PrometheusDBHook) afterCallback(operation string) func(*gorm.DB) {
	return func(db *gorm.DB) {
		startTimeInterface, exists := db.Get("prometheus_start_time")
		if !exists {
			return
		}

		startTime, ok := startTimeInterface.(time.Time)
		if !ok {
			return
		}

		// Get table name
		tableName := "unknown"
		if db.Statement != nil && db.Statement.Table != "" {
			tableName = db.Statement.Table
		} else if db.Statement != nil && db.Statement.Schema != nil {
			tableName = db.Statement.Schema.Table
		}

		// Check if operation was successful
		success := db.Error == nil

		// Record metrics
		metrics.GlobalRecorder.RecordDBQuery(startTime, operation, tableName, success)
	}
}

// UpdateDBConnectionMetrics updates database connection pool metrics
func UpdateDBConnectionMetrics() {
	if DB != nil {
		sqlDB, err := DB.DB()
		if err == nil {
			stats := sqlDB.Stats()
			metrics.GlobalRecorder.UpdateDBConnectionMetrics(stats.InUse, stats.Idle)
		}
	}
}

// InitPrometheusDBMonitoring initializes database monitoring
func InitPrometheusDBMonitoring() error {
	if DB != nil {
		hook := &PrometheusDBHook{}
		return DB.Use(hook)
	}
	return nil
}
