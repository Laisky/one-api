package model

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/logger"
)

// RequestIDMaxLen is the maximum length of request_id column to enforce indexing
const RequestIDMaxLen = 32

type UserRequestCost struct {
	Id          int   `json:"id"`
	CreatedTime int64 `json:"created_time" gorm:"bigint"`
	UserID      int   `json:"user_id"`
	// Enforce uniqueness to avoid duplicate rows for the same request
	RequestID string  `json:"request_id" gorm:"size:32;uniqueIndex"` // size must match RequestIDMaxLen
	Quota     int64   `json:"quota"`
	CostUSD   float64 `json:"cost_usd" gorm:"-"`
	CreatedAt int64   `json:"created_at" gorm:"bigint;autoCreateTime:milli"`
	UpdatedAt int64   `json:"updated_at" gorm:"bigint;autoUpdateTime:milli"`
}

// NewUserRequestCost create a new UserRequestCost
func NewUserRequestCost(userID int, quotaID string, quota int64) *UserRequestCost {
	return &UserRequestCost{
		CreatedTime: helper.GetTimestamp(),
		UserID:      userID,
		RequestID:   quotaID,
		Quota:       quota,
	}
}

func (docu *UserRequestCost) Insert() error {
	go removeOldRequestCost()

	err := DB.Create(docu).Error
	return errors.Wrap(err, "failed to insert UserRequestCost")
}

// UpdateUserRequestCostQuotaByRequestID updates the quota for an existing request-cost record by request_id.
// If the record does not exist, it will create a new one with the provided userID and quota.
func UpdateUserRequestCostQuotaByRequestID(userID int, requestID string, quota int64) error {
	if requestID == "" {
		return errors.New("request id is empty")
	}

	go removeOldRequestCost()

	// Update-first approach to avoid unique conflict races without using clause.OnConflict
	// 1) Try update by request_id
	tx := DB.Model(&UserRequestCost{}).
		Where("request_id = ?", requestID).
		Update("quota", quota)
	if tx.Error != nil {
		return errors.Wrap(tx.Error, "failed to update UserRequestCost quota")
	}
	affected := tx.RowsAffected
	if affected > 0 {
		return nil
	}

	docu := &UserRequestCost{
		CreatedTime: helper.GetTimestamp(),
		UserID:      userID,
		RequestID:   requestID,
		Quota:       quota,
	}
	if err := DB.Create(docu).Error; err == nil {
		return nil
	}
	// If create failed (possibly due to unique race), retry update once
	if err2 := DB.Model(&UserRequestCost{}).
		Where("request_id = ?", requestID).
		Update("quota", quota).Error; err2 != nil {
		return errors.Wrap(err2, "failed to update UserRequestCost quota after create race")
	}
	return nil
}

// GetCostByRequestId get cost by request id
func GetCostByRequestId(reqid string) (*UserRequestCost, error) {
	if reqid == "" {
		return nil, errors.New("request id is empty")
	}

	docu := &UserRequestCost{RequestID: reqid}
	var err error = nil
	if err = DB.First(docu, "request_id = ?", reqid).Error; err != nil {
		return nil, errors.Wrap(err, "failed to get cost by request id")
	}

	docu.CostUSD = float64(docu.Quota) / 500000
	return docu, nil
}

var muRemoveOldRequestCost sync.Mutex

// removeOldRequestCost remove old request cost data,
// this function will be executed every 1/1000 times.
func removeOldRequestCost() {
	if rand.Float32() > 0.001 {
		return
	}

	if ok := muRemoveOldRequestCost.TryLock(); !ok {
		return
	}
	defer muRemoveOldRequestCost.Unlock()

	err := DB.
		Where("created_time < ?", helper.GetTimestamp()-3600*24*7).
		Delete(&UserRequestCost{}).Error
	if err != nil {
		logger.Logger.Error("failed to remove old request cost", zap.Error(err))
	}
}

// MigrateUserRequestCostEnsureUniqueRequestID ensures a unique index on request_id and deduplicates prior data.
// It is safe to run multiple times and should be invoked before AutoMigrate in InitDB.
func MigrateUserRequestCostEnsureUniqueRequestID() error {
	// If table does not exist yet, skip quietly; AutoMigrate will create it with the unique index from tags
	tableExists := false
	var err error
	if common.UsingMySQL {
		tableExists, err = mysqlTableExists("user_request_costs")
		if err != nil {
			return errors.Wrap(err, "check user_request_costs existence (mysql)")
		}
	} else {
		tableExists = DB.Migrator().HasTable(&UserRequestCost{})
	}
	if !tableExists {
		return nil
	}

	// Early return if migration is already complete
	indexName := "idx_user_request_costs_request_id"
	var indexExists bool
	if common.UsingMySQL {
		indexExists, err = mysqlIndexExists("user_request_costs", indexName)
	} else {
		indexExists = DB.Migrator().HasIndex(&UserRequestCost{}, indexName)
	}
	if err != nil {
		return errors.Wrap(err, "check user_request_costs index existence")
	}

	// Check if column is already sized correctly
	var columnSized bool
	if common.UsingMySQL {
		columnSized, err = isMySQLRequestIDColumnSized()
		if err != nil {
			return err
		}
	} else if common.UsingPostgreSQL {
		columnSized, err = isPostgresRequestIDColumnSized()
		if err != nil {
			return err
		}
	} else {
		// SQLite doesn't need column sizing
		columnSized = true
	}

	// If both index exists and column is sized correctly, skip migration
	if indexExists && columnSized {
		return nil
	}

	// Dedup rows prior to creating the unique index. Depending on the legacy schema, the
	// table may lack updated_at/created_at columns, so pick the newest available marker.
	markerColumns := []string{"updated_at", "created_at", "created_time", "id"}
	var dedupColumn string
	for _, col := range markerColumns {
		var hasColumn bool
		if common.UsingMySQL {
			hasColumn, err = mysqlColumnExists("user_request_costs", col)
		} else {
			hasColumn = DB.Migrator().HasColumn(&UserRequestCost{}, col)
		}
		if err != nil {
			return errors.Wrapf(err, "check column %s existence", col)
		}
		if hasColumn {
			dedupColumn = col
			break
		}
	}
	if dedupColumn == "" {
		return errors.New("user_request_costs table missing expected columns for deduplication")
	}

	logger.Logger.Info("deduplicating user_request_costs", zap.String("dedup_column", dedupColumn))

	selectExpr := fmt.Sprintf("request_id, MAX(%s) as max_marker", dedupColumn)
	type pair struct {
		RequestID string
		MaxMarker int64 `gorm:"column:max_marker"`
	}
	var latest []pair
	if err := DB.Table("user_request_costs").
		Select(selectExpr).
		Group("request_id").
		Scan(&latest).Error; err != nil {
		return errors.Wrap(err, "scan latest user_request_costs per request_id failed")
	}

	if len(latest) > 0 {
		keep := make(map[string]int64, len(latest))
		for _, p := range latest {
			keep[p.RequestID] = p.MaxMarker
		}

		cond := fmt.Sprintf("%s < ?", dedupColumn)
		batchSize := 1000
		for reqID, marker := range keep {
			if err := DB.Where("request_id = ? AND "+cond, reqID, marker).
				Delete(&UserRequestCost{}).Error; err != nil {
				logger.Logger.Warn("dedup delete failed", zap.Error(err))
			}
			batchSize--
			if batchSize == 0 {
				time.Sleep(10 * time.Millisecond)
				batchSize = 1000
			}
		}
	}

	if err = deleteLongUserRequestCostRequestIDs(); err != nil {
		return err
	}

	if common.UsingMySQL && !columnSized {
		if err = ensureMySQLRequestIDColumnSized(); err != nil {
			return err
		}
	} else if common.UsingPostgreSQL && !columnSized {
		if err = ensurePostgresRequestIDColumnSized(); err != nil {
			return err
		}
	}

	if indexExists {
		return nil
	}

	// 3) Create unique index if missing. Use generic SQL with dialect-aware fallbacks.
	switch {
	case common.UsingPostgreSQL:
		if err = DB.Exec(fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS %s ON user_request_costs (request_id)", indexName)).Error; err != nil {
			return errors.Wrap(err, "create unique index on user_request_costs.request_id failed (postgres)")
		}
	case common.UsingMySQL:
		if err = DB.Exec(fmt.Sprintf("ALTER TABLE user_request_costs ADD UNIQUE INDEX %s (request_id)", indexName)).Error; err != nil {
			return errors.Wrap(err, "create unique index on user_request_costs.request_id failed (mysql)")
		}
	case common.UsingSQLite:
		if err = DB.Exec(fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS %s ON user_request_costs (request_id)", indexName)).Error; err != nil {
			return errors.Wrap(err, "create unique index on user_request_costs.request_id failed (sqlite)")
		}
	default:
		if err = DB.Exec(fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS %s ON user_request_costs (request_id)", indexName)).Error; err != nil {
			return errors.Wrap(err, "create unique index on user_request_costs.request_id failed")
		}
	}
	return nil
}

// deleteLongUserRequestCostRequestIDs removes rows whose request_id exceeds 32 characters across supported dialects.
func deleteLongUserRequestCostRequestIDs() error {
	var query string
	switch {
	case common.UsingMySQL, common.UsingPostgreSQL:
		query = fmt.Sprintf("DELETE FROM user_request_costs WHERE CHAR_LENGTH(request_id) > %d", RequestIDMaxLen)
	case common.UsingSQLite:
		query = fmt.Sprintf("DELETE FROM user_request_costs WHERE LENGTH(request_id) > %d", RequestIDMaxLen)
	default:
		query = fmt.Sprintf("DELETE FROM user_request_costs WHERE LENGTH(request_id) > %d", RequestIDMaxLen)
	}

	if err := DB.Exec(query).Error; err != nil {
		return errors.Wrap(err, "delete user_request_costs entries with request_id longer than max len")
	}

	return nil
}

// mysqlTableExists returns whether the given table is present in the current MySQL schema.
func mysqlTableExists(table string) (bool, error) {
	type result struct {
		Count int `gorm:"column:count"`
	}
	var res result
	query := "SELECT COUNT(*) AS count FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?"
	if err := DB.Raw(query, table).Scan(&res).Error; err != nil {
		return false, err
	}
	return res.Count > 0, nil
}

// mysqlColumnExists reports whether the provided column exists for the table in the current MySQL schema.
func mysqlColumnExists(table, column string) (bool, error) {
	type result struct {
		Count int `gorm:"column:count"`
	}
	var res result
	query := "SELECT COUNT(*) AS count FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?"
	if err := DB.Raw(query, table, column).Scan(&res).Error; err != nil {
		return false, err
	}
	return res.Count > 0, nil
}

// mysqlIndexExists reports whether the provided index exists for the table in the current MySQL schema.
func mysqlIndexExists(table, index string) (bool, error) {
	type result struct {
		Count int `gorm:"column:count"`
	}
	var res result
	query := "SELECT COUNT(*) AS count FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?"
	if err := DB.Raw(query, table, index).Scan(&res).Error; err != nil {
		return false, err
	}
	return res.Count > 0, nil
}

// isMySQLRequestIDColumnSized checks if request_id column is already VARCHAR (not TEXT).
func isMySQLRequestIDColumnSized() (bool, error) {
	var dataType string
	query := "SELECT DATA_TYPE FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?"
	if err := DB.Raw(query, "user_request_costs", "request_id").Scan(&dataType).Error; err != nil {
		return false, errors.Wrap(err, "query user_request_costs.request_id column type")
	}
	dataType = strings.ToLower(dataType)
	if dataType == "" {
		return false, nil
	}
	return !strings.Contains(dataType, "text"), nil
}

// ensureMySQLRequestIDColumnSized converts legacy TEXT request_id columns to VARCHAR(32) for index support.
func ensureMySQLRequestIDColumnSized() error {
	var dataType string
	query := "SELECT DATA_TYPE FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?"
	if err := DB.Raw(query, "user_request_costs", "request_id").Scan(&dataType).Error; err != nil {
		return errors.Wrap(err, "query user_request_costs.request_id column type")
	}
	dataType = strings.ToLower(dataType)
	if dataType == "" {
		return nil
	}
	if strings.Contains(dataType, "text") {
		logger.Logger.Info("migrating user_request_costs.request_id to VARCHAR(%d) for unique index",
			zap.String("column_type", dataType), zap.Int("max_len", RequestIDMaxLen))
		alter := fmt.Sprintf("ALTER TABLE user_request_costs MODIFY request_id VARCHAR(%d) NOT NULL", RequestIDMaxLen)
		if err := DB.Exec(alter).Error; err != nil {
			return errors.Wrap(err, "alter user_request_costs.request_id to VARCHAR(max_len)")
		}
	}
	return nil
}

// isPostgresRequestIDColumnSized checks if request_id column is already the correct VARCHAR type.
func isPostgresRequestIDColumnSized() (bool, error) {
	var dataType string
	query := "SELECT data_type FROM information_schema.columns WHERE table_schema = CURRENT_SCHEMA() AND table_name = ? AND column_name = ?"
	if err := DB.Raw(query, "user_request_costs", "request_id").Scan(&dataType).Error; err != nil {
		return false, errors.Wrap(err, "query user_request_costs.request_id column type (postgres)")
	}
	dataType = strings.ToLower(dataType)
	if dataType == "" {
		return false, nil
	}
	return strings.Contains(dataType, "character varying") || strings.Contains(dataType, "varchar"), nil
}

// ensurePostgresRequestIDColumnSized enforces a VARCHAR(32) type for request_id in PostgreSQL deployments.
func ensurePostgresRequestIDColumnSized() error {
	alter := fmt.Sprintf("ALTER TABLE user_request_costs ALTER COLUMN request_id TYPE VARCHAR(%d)", RequestIDMaxLen)
	if err := DB.Exec(alter).Error; err != nil {
		return errors.Wrap(err, "alter user_request_costs.request_id to VARCHAR(max_len) (postgres)")
	}
	return nil
}
