package model

import (
	"time"

	"github.com/Laisky/errors/v2"
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
		metrics.Recorder().RecordDBQuery(startTime, operation, tableName, success)
	}
}

// UpdateDBConnectionMetrics updates database connection pool metrics
func UpdateDBConnectionMetrics() {
	if DB != nil {
		sqlDB, err := DB.DB()
		if err == nil {
			stats := sqlDB.Stats()
			metrics.Recorder().UpdateDBConnectionMetrics(stats.InUse, stats.Idle)
		}
	}
}

// InitPrometheusDBMonitoring attaches the database metrics hook to the primary
// handle. It is idempotent and normally a no-op: initPrimaryDatabase registers
// the hook right after the handle is opened, before any migration or background
// worker can issue a query through it.
//
// Why: gorm callback registration mutates the shared callback chain without
// locking, so registering the hook after InitDB has started workers (UUID
// catch-up, option/channel sync) is a data race with every query they run.
// Parameters: none. Return value: wrapped error when registration fails.
func InitPrometheusDBMonitoring() error {
	if DB == nil {
		return nil
	}
	return registerDBMetricsHook(DB)
}

// registerDBMetricsHook registers PrometheusDBHook on db unless it is already
// present. It must run before any other goroutine can use db.
// Parameters: db is the handle to instrument. Return value: wrapped error when
// gorm rejects the plugin.
func registerDBMetricsHook(db *gorm.DB) error {
	hook := &PrometheusDBHook{}
	if _, registered := db.Config.Plugins[hook.Name()]; registered {
		return nil
	}
	if err := db.Use(hook); err != nil {
		return errors.Wrap(err, "register database metrics hook")
	}
	return nil
}
