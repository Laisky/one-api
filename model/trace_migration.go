package model

import (
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/logger"
)

var traceURLMigrationOnce sync.Once
var traceURLMigrated atomic.Bool

// MigrateTraceURLColumnToText ensures traces.url can store arbitrarily long URLs by migrating legacy VARCHAR columns to TEXT.
// Legacy deployments created the column with a 512 character limit, which breaks when query strings contain large Turnstile tokens
// or other security artefacts. This migration upgrades MySQL and PostgreSQL schemas in-place while remaining a no-op for SQLite.
func MigrateTraceURLColumnToText() error {
	var runErr error
	traceURLMigrationOnce.Do(func() {
		if traceURLMigrated.Load() {
			return
		}

		needsMigration, checkErr := traceURLColumnNeedsMigration()
		if checkErr != nil {
			runErr = errors.Wrap(checkErr, "determine trace url column migration need")
			return
		}

		if !needsMigration {
			traceURLMigrated.Store(true)
			return
		}

		logger.Logger.Info("migrating traces.url column to TEXT type")

		tx := DB.Begin()
		if tx.Error != nil {
			runErr = errors.Wrap(tx.Error, "start trace url migration transaction")
			return
		}

		defer func() {
			if runErr != nil {
				if rbErr := tx.Rollback().Error; rbErr != nil {
					logger.Logger.Error("failed to rollback trace url migration transaction",
						zap.Error(rbErr))
				}
			}
		}()

		var alterErr error
		switch {
		case common.UsingMySQL.Load():
			alterErr = tx.Exec("ALTER TABLE traces MODIFY COLUMN url TEXT NOT NULL").Error
		case common.UsingPostgreSQL.Load():
			alterErr = tx.Exec("ALTER TABLE traces ALTER COLUMN url TYPE TEXT").Error
		default:
			alterErr = nil
		}

		if alterErr != nil {
			runErr = errors.Wrap(alterErr, "alter traces.url column type")
			return
		}

		if err := tx.Commit().Error; err != nil {
			runErr = errors.Wrap(err, "commit trace url migration transaction")
			return
		}

		traceURLMigrated.Store(true)
		logger.Logger.Info("traces.url column migrated to TEXT type")
	})

	return runErr
}

// traceURLColumnNeedsMigration reports whether the traces.url column is still backed by a fixed-length VARCHAR type.
func traceURLColumnNeedsMigration() (bool, error) {
	switch {
	case common.UsingMySQL.Load():
		var tableExists int
		if err := DB.Raw(`SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'traces'`).Scan(&tableExists).Error; err != nil {
			return false, errors.Wrap(err, "check traces table existence (mysql)")
		}
		if tableExists == 0 {
			return false, nil
		}

		var columnType string
		if err := DB.Raw(`SELECT DATA_TYPE FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'traces' AND COLUMN_NAME = 'url'`).Scan(&columnType).Error; err != nil {
			return false, errors.Wrap(err, "lookup traces.url column type (mysql)")
		}
		if columnType == "" {
			return false, nil
		}
		return !strings.Contains(columnType, "text"), nil

	case common.UsingPostgreSQL.Load():
		var tableExists int
		if err := DB.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'traces'`).Scan(&tableExists).Error; err != nil {
			return false, errors.Wrap(err, "check traces table existence (postgres)")
		}
		if tableExists == 0 {
			return false, nil
		}

		var columnType string
		if err := DB.Raw(`SELECT data_type FROM information_schema.columns WHERE table_name = 'traces' AND column_name = 'url'`).Scan(&columnType).Error; err != nil {
			return false, errors.Wrap(err, "lookup traces.url column type (postgres)")
		}
		if columnType == "" {
			return false, nil
		}
		return columnType == "character varying", nil

	case common.UsingSQLite.Load():
		return false, nil
	default:
		return false, nil
	}
}
