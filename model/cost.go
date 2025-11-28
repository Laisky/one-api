package model

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"

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
// It is safe to run multiple times and should be invoked before AutoMigrate in InitDB. The migration depends on
// information_schema metadata for MySQL/PostgreSQL and will fail fast when the database user lacks permission,
// surfacing the missing privilege to operators explicitly.
func MigrateUserRequestCostEnsureUniqueRequestID() error {
	logger.Logger.Info("Starting user_request_costs request_id migration")
	// If table does not exist yet, skip quietly; AutoMigrate will create it with the unique index from tags
	tableExists := false
	var err error
	if common.UsingMySQL.Load() {
		tableExists, err = mysqlTableExists("user_request_costs")
		if err != nil {
			return errors.Wrap(err, "check user_request_costs existence (mysql)")
		}
	} else {
		tableExists = DB.Migrator().HasTable(&UserRequestCost{})
	}
	if !tableExists {
		logger.Logger.Debug("user_request_costs table not found, skipping request_id migration")
		return nil
	}

	indexName := "idx_user_request_costs_request_id"
	checkIndexExists := func() (bool, error) {
		if common.UsingMySQL.Load() {
			return mysqlIndexExists("user_request_costs", indexName)
		}
		return DB.Migrator().HasIndex(&UserRequestCost{}, indexName), nil
	}

	hasIndex, err := checkIndexExists()
	if err != nil {
		return errors.Wrap(err, "check user_request_costs index existence")
	}
	if hasIndex {
		logger.Logger.Debug("Unique index already present on user_request_costs.request_id; skipping deduplication")
		return nil
	}

	// Dedup rows prior to creating the unique index. Depending on the legacy schema, the
	// table may lack updated_at/created_at columns, so pick the newest available marker.
	markerColumns := []string{"updated_at", "created_at", "created_time", "id"}
	var dedupColumn string
	for _, col := range markerColumns {
		var hasColumn bool
		if common.UsingMySQL.Load() {
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

	logger.Logger.Debug("Deduplicating user_request_costs", zap.String("dedup_column", dedupColumn))

	selectExpr := fmt.Sprintf("request_id, MAX(%s) as max_marker", dedupColumn)
	latestQuery := DB.Table("user_request_costs").
		Select(selectExpr).
		Group("request_id")

	hasIDColumn, err := userRequestCostHasIDColumn()
	if err != nil {
		return errors.Wrap(err, "check user_request_costs id column existence")
	}

	var duplicateCount int
	if hasIDColumn {
		staleQuery := DB.Table("user_request_costs AS stale").
			Joins("JOIN (?) AS keep ON keep.request_id = stale.request_id", latestQuery).
			Where(fmt.Sprintf("stale.%s < keep.max_marker", dedupColumn))

		var staleIDs []int
		if err := staleQuery.Pluck("stale.id", &staleIDs).Error; err != nil {
			return errors.Wrap(err, "select duplicate user_request_costs ids")
		}

		duplicateCount = len(staleIDs)
		if duplicateCount > 0 {
			const deleteBatchSize = 1000
			for start := 0; start < len(staleIDs); start += deleteBatchSize {
				end := min(start+deleteBatchSize, len(staleIDs))
				if err := DB.Where("id IN ?", staleIDs[start:end]).Delete(&UserRequestCost{}).Error; err != nil {
					return errors.Wrap(err, "delete duplicate user_request_costs batch")
				}
			}
		}
	} else {
		logger.Logger.Debug("user_request_costs table missing id column, using request_id fallback for dedup")

		cond := fmt.Sprintf("%s < ?", dedupColumn)
		rows, err := latestQuery.Rows()
		if err != nil {
			return errors.Wrap(err, "query latest user_request_costs per request_id")
		}
		var keepRows []struct {
			requestID string
			maxMarker any
		}
		for rows.Next() {
			var (
				rid    string
				marker any
			)
			if err := rows.Scan(&rid, &marker); err != nil {
				_ = rows.Close()
				return errors.Wrap(err, "scan latest user_request_costs per request_id")
			}
			keepRows = append(keepRows, struct {
				requestID string
				maxMarker any
			}{requestID: rid, maxMarker: marker})
		}
		if err := rows.Close(); err != nil {
			return errors.Wrap(err, "close latest user_request_costs rows")
		}
		if err := rows.Err(); err != nil {
			return errors.Wrap(err, "iterate latest user_request_costs rows")
		}

		for _, row := range keepRows {
			result := DB.Where("request_id = ? AND "+cond, row.requestID, row.maxMarker).
				Delete(&UserRequestCost{})
			if result.Error != nil {
				return errors.Wrap(result.Error, "delete duplicate user_request_costs row (fallback)")
			}
			duplicateCount += int(result.RowsAffected)
		}
	}

	if duplicateCount > 0 {
		logger.Logger.Debug("Removed duplicate user_request_costs rows", zap.Int("duplicate_count", duplicateCount))
	} else {
		logger.Logger.Debug("No duplicate user_request_costs rows detected")
	}

	deletedLongCount, err := deleteLongUserRequestCostRequestIDs()
	if err != nil {
		return err
	}
	if deletedLongCount > 0 {
		logger.Logger.Debug("Removed user_request_costs rows with oversized request_id",
			zap.Int64("deleted_count", deletedLongCount),
			zap.Int("max_length", RequestIDMaxLen))
	} else {
		logger.Logger.Debug("No user_request_costs rows exceeded request_id length limit",
			zap.Int("max_length", RequestIDMaxLen))
	}

	columnAltered := false
	if common.UsingMySQL.Load() {
		var altered bool
		altered, err = ensureMySQLRequestIDColumnSized()
		if err != nil {
			return err
		}
		columnAltered = columnAltered || altered
	} else if common.UsingPostgreSQL.Load() {
		var altered bool
		altered, err = ensurePostgresRequestIDColumnSized()
		if err != nil {
			return err
		}
		columnAltered = columnAltered || altered
	}

	hasIndex, err = checkIndexExists()
	if err != nil {
		return errors.Wrap(err, "re-check user_request_costs index existence")
	}
	if hasIndex {
		logger.Logger.Debug("Unique index already present on user_request_costs.request_id")
		logger.Logger.Debug("Completed user_request_costs request_id migration",
			zap.Int("duplicates_removed", duplicateCount),
			zap.Int64("long_request_ids_removed", deletedLongCount),
			zap.Bool("column_altered", columnAltered))
		return nil
	}

	// 3) Create unique index if missing. Use generic SQL with dialect-aware fallbacks.
	logger.Logger.Debug("Creating unique index on user_request_costs.request_id",
		zap.String("index", indexName))
	switch {
	case common.UsingPostgreSQL.Load():
		if err = DB.Exec(fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS %s ON user_request_costs (request_id)", indexName)).Error; err != nil {
			return errors.Wrap(err, "create unique index on user_request_costs.request_id failed (postgres)")
		}
	case common.UsingMySQL.Load():
		if err = DB.Exec(fmt.Sprintf("ALTER TABLE user_request_costs ADD UNIQUE INDEX %s (request_id)", indexName)).Error; err != nil {
			return errors.Wrap(err, "create unique index on user_request_costs.request_id failed (mysql)")
		}
	case common.UsingSQLite.Load():
		if err = DB.Exec(fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS %s ON user_request_costs (request_id)", indexName)).Error; err != nil {
			return errors.Wrap(err, "create unique index on user_request_costs.request_id failed (sqlite)")
		}
	default:
		if err = DB.Exec(fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS %s ON user_request_costs (request_id)", indexName)).Error; err != nil {
			return errors.Wrap(err, "create unique index on user_request_costs.request_id failed")
		}
	}
	logger.Logger.Debug("Unique index created on user_request_costs.request_id")

	logger.Logger.Debug("Completed user_request_costs request_id migration",
		zap.Int("duplicates_removed", duplicateCount),
		zap.Int64("long_request_ids_removed", deletedLongCount),
		zap.Bool("column_altered", columnAltered))
	return nil
}

// deleteLongUserRequestCostRequestIDs removes rows whose request_id exceeds 32 characters across supported dialects.
func deleteLongUserRequestCostRequestIDs() (int64, error) {
	var query string
	switch {
	case common.UsingMySQL.Load(), common.UsingPostgreSQL.Load():
		query = fmt.Sprintf("DELETE FROM user_request_costs WHERE CHAR_LENGTH(request_id) > %d", RequestIDMaxLen)
	case common.UsingSQLite.Load():
		query = fmt.Sprintf("DELETE FROM user_request_costs WHERE LENGTH(request_id) > %d", RequestIDMaxLen)
	default:
		query = fmt.Sprintf("DELETE FROM user_request_costs WHERE LENGTH(request_id) > %d", RequestIDMaxLen)
	}

	result := DB.Exec(query)
	if result.Error != nil {
		return 0, errors.Wrap(result.Error, "delete user_request_costs entries with request_id longer than max len")
	}

	return result.RowsAffected, nil
}

// userRequestCostHasIDColumn reports whether user_request_costs currently exposes an id column
// across supported SQL dialects. SQLite requires a PRAGMA table_info scan because GORM's
// HasColumn helper treats the implicit rowid as an id column.
func userRequestCostHasIDColumn() (bool, error) {
	switch {
	case common.UsingMySQL.Load():
		return mysqlColumnExists("user_request_costs", "id")
	case common.UsingPostgreSQL.Load():
		type result struct {
			Count int `gorm:"column:count"`
		}
		var res result
		query := "SELECT COUNT(*) AS count FROM information_schema.columns WHERE table_name = ? AND column_name = ?"
		if err := DB.Raw(query, "user_request_costs", "id").Scan(&res).Error; err != nil {
			return false, errors.Wrap(err, "query postgres information_schema for user_request_costs.id")
		}
		return res.Count > 0, nil
	case common.UsingSQLite.Load():
		rows, err := DB.Raw("PRAGMA table_info(user_request_costs)").Rows()
		if err != nil {
			return false, errors.Wrap(err, "query sqlite table info for user_request_costs")
		}
		defer rows.Close()

		for rows.Next() {
			var (
				cid       int
				name      string
				ctype     string
				notnull   int
				dfltValue any
				pk        int
			)
			if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
				return false, errors.Wrap(err, "scan sqlite table info row")
			}
			if strings.EqualFold(name, "id") {
				return true, nil
			}
		}
		if err := rows.Err(); err != nil {
			return false, errors.Wrap(err, "iterate sqlite table info")
		}
		return false, nil
	default:
		return DB.Migrator().HasColumn("user_request_costs", "id"), nil
	}
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

// ensureMySQLRequestIDColumnSized converts legacy TEXT request_id columns to VARCHAR(32) for index support.
func ensureMySQLRequestIDColumnSized() (bool, error) {
	type result struct {
		DataType string `gorm:"column:data_type"`
		CharLen  *int64 `gorm:"column:character_maximum_length"`
	}
	var res result
	query := "SELECT DATA_TYPE AS data_type, CHARACTER_MAXIMUM_LENGTH AS character_maximum_length FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?"
	if err := DB.Raw(query, "user_request_costs", "request_id").Scan(&res).Error; err != nil {
		return false, errors.Wrap(err, "query user_request_costs.request_id column type")
	}
	dataType := strings.ToLower(res.DataType)
	if dataType == "" {
		logger.Logger.Debug("MySQL request_id column not found during sizing check - skipping alter")
		return false, nil
	}
	if strings.Contains(dataType, "text") {
		logger.Logger.Debug("migrating user_request_costs.request_id to VARCHAR for unique index",
			zap.String("column_type", dataType), zap.Int("max_len", RequestIDMaxLen))
		alter := fmt.Sprintf("ALTER TABLE user_request_costs MODIFY request_id VARCHAR(%d) NOT NULL", RequestIDMaxLen)
		if err := DB.Exec(alter).Error; err != nil {
			return false, errors.Wrap(err, "alter user_request_costs.request_id to VARCHAR(max_len)")
		}
		return true, nil
	}
	if res.CharLen != nil && *res.CharLen != int64(RequestIDMaxLen) {
		logger.Logger.Debug("Adjusting MySQL request_id column length",
			zap.Int64("current_length", *res.CharLen),
			zap.Int("target_length", RequestIDMaxLen))
		alter := fmt.Sprintf("ALTER TABLE user_request_costs MODIFY request_id VARCHAR(%d) NOT NULL", RequestIDMaxLen)
		if err := DB.Exec(alter).Error; err != nil {
			return false, errors.Wrap(err, "alter user_request_costs.request_id length to target size")
		}
		return true, nil
	}

	logger.Logger.Debug("MySQL request_id column already sized correctly",
		zap.String("column_type", dataType),
		zap.Any("char_length", res.CharLen))
	return false, nil
}

// ensurePostgresRequestIDColumnSized enforces a VARCHAR(32) type for request_id in PostgreSQL deployments.
func ensurePostgresRequestIDColumnSized() (bool, error) {
	type result struct {
		DataType string `gorm:"column:data_type"`
		CharLen  *int64 `gorm:"column:character_maximum_length"`
	}
	var res result
	query := "SELECT data_type, character_maximum_length FROM information_schema.columns WHERE table_name = 'user_request_costs' AND column_name = 'request_id'"
	if err := DB.Raw(query).Scan(&res).Error; err != nil {
		return false, errors.Wrap(err, "query postgres user_request_costs.request_id column type")
	}
	dataType := strings.ToLower(res.DataType)
	if dataType == "" {
		logger.Logger.Debug("PostgreSQL request_id column not found during sizing check - skipping alter")
		return false, nil
	}

	switch {
	case strings.Contains(dataType, "text"):
		logger.Logger.Debug("PostgreSQL request_id column already TEXT, no sizing change required")
		return false, nil
	case strings.Contains(dataType, "character varying"):
		if res.CharLen != nil && *res.CharLen == int64(RequestIDMaxLen) {
			logger.Logger.Debug("PostgreSQL request_id column already sized correctly",
				zap.Int("target_length", RequestIDMaxLen))
			return false, nil
		}
		logger.Logger.Debug("Adjusting PostgreSQL request_id column length",
			zap.Any("current_length", res.CharLen),
			zap.Int("target_length", RequestIDMaxLen))
		alter := fmt.Sprintf("ALTER TABLE user_request_costs ALTER COLUMN request_id TYPE VARCHAR(%d)", RequestIDMaxLen)
		if err := DB.Exec(alter).Error; err != nil {
			return false, errors.Wrap(err, "alter user_request_costs.request_id to VARCHAR(max_len) (postgres)")
		}
		return true, nil
	default:
		logger.Logger.Debug("PostgreSQL request_id column has unexpected type, attempting to coerce",
			zap.String("data_type", dataType))
		alter := fmt.Sprintf("ALTER TABLE user_request_costs ALTER COLUMN request_id TYPE VARCHAR(%d)", RequestIDMaxLen)
		if err := DB.Exec(alter).Error; err != nil {
			return false, errors.Wrap(err, "coerce user_request_costs.request_id to VARCHAR(max_len) (postgres)")
		}
		return true, nil
	}
}
