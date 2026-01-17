package model

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"go.opentelemetry.io/otel"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/common/random"
	// glogger "gorm.io/gorm/logger"
)

var DB *gorm.DB
var LOG_DB *gorm.DB

func CreateRootAccountIfNeed() error {
	var user User
	//if user.Status != util.UserStatusEnabled {
	if err := DB.First(&user).Error; err != nil {
		logger.Logger.Info("no user exists, creating a root user for you: username is root, password is 123456")
		hashedPassword, err := common.Password2Hash("123456")
		if err != nil {
			return errors.WithStack(err)
		}
		accessToken := random.GetUUID()
		if config.InitialRootAccessToken != "" {
			accessToken = config.InitialRootAccessToken
		}
		rootUser := User{
			Username:    "root",
			Password:    hashedPassword,
			Role:        RoleRootUser,
			Status:      UserStatusEnabled,
			DisplayName: "Root User",
			AccessToken: accessToken,
			Quota:       500000000000000,
		}
		DB.Create(&rootUser)
		if config.InitialRootToken != "" {
			logger.Logger.Info("creating initial root token as requested")
			token := Token{
				Id:             1,
				UserId:         rootUser.Id,
				Key:            config.InitialRootToken,
				Status:         TokenStatusEnabled,
				Name:           "Initial Root Token",
				CreatedTime:    helper.GetTimestamp(),
				AccessedTime:   helper.GetTimestamp(),
				ExpiredTime:    -1,
				RemainQuota:    500000000000000,
				UnlimitedQuota: true,
			}
			DB.Create(&token)
		}
	}
	return nil
}

func chooseDB(dsn string) (*gorm.DB, error) {
	switch {
	case strings.HasPrefix(dsn, "postgres://"):
		// Use PostgreSQL
		return openPostgreSQL(dsn)
	case dsn != "":
		// Use MySQL
		return openMySQL(dsn)
	default:
		// Use SQLite
		return openSQLite()
	}
}

func openPostgreSQL(dsn string) (*gorm.DB, error) {
	logger.Logger.Info("using PostgreSQL as database")
	common.UsingPostgreSQL.Store(true)
	return gorm.Open(postgres.New(postgres.Config{
		DSN:                  dsn,
		PreferSimpleProtocol: true, // disables implicit prepared statement usage
	}), &gorm.Config{
		PrepareStmt: true, // precompile SQL
		// Logger: glogger.Default.LogMode(glogger.Info),  // debug sql
	})
}

func openMySQL(dsn string) (*gorm.DB, error) {
	logger.Logger.Info("using MySQL as database")
	common.UsingMySQL.Store(true)
	normalized, err := common.NormalizeMySQLDSN(dsn)
	if err != nil {
		return nil, errors.Wrap(err, "normalize MySQL DSN")
	}

	return gorm.Open(mysql.Open(normalized), &gorm.Config{
		PrepareStmt: true, // precompile SQL
	})
}

func openSQLite() (*gorm.DB, error) {
	logger.Logger.Info("SQL_DSN not set, using SQLite as database")
	common.UsingSQLite.Store(true)
	sqlitePath, err := ensureSQLitePath()
	if err != nil {
		return nil, errors.Wrap(err, "prepare sqlite path")
	}

	logger.Logger.Debug("using SQLite database", zap.String("path", sqlitePath), zap.Int("busy_timeout_ms", common.SQLiteBusyTimeout))

	dsn := fmt.Sprintf("%s?_busy_timeout=%d", sqlitePath, common.SQLiteBusyTimeout)
	return gorm.Open(sqlite.Open(dsn), &gorm.Config{
		PrepareStmt: true, // precompile SQL
	})
}

// ensureSQLitePath prepares the SQLite file path by creating the parent directory if needed
// and verifying basic write access so startup can surface permission issues early.
func ensureSQLitePath() (string, error) {
	absPath, err := filepath.Abs(common.SQLitePath)
	if err != nil {
		return "", errors.Wrap(err, "resolve sqlite path")
	}

	parentDir := filepath.Dir(absPath)
	if err = os.MkdirAll(parentDir, 0o770); err != nil {
		return "", errors.Wrap(err, "create sqlite directory")
	}

	probeFile := filepath.Join(parentDir, ".sqlite-permission-check")
	probe, err := os.OpenFile(probeFile, os.O_CREATE|os.O_RDWR, 0o660)
	if err != nil {
		return "", errors.Wrap(err, "sqlite directory not writable")
	}

	if closeErr := probe.Close(); closeErr != nil {
		return "", errors.Wrap(closeErr, "close sqlite permission probe")
	}

	if rmErr := os.Remove(probeFile); rmErr != nil && !os.IsNotExist(rmErr) {
		logger.Logger.Debug("failed to remove sqlite probe file", zap.Error(rmErr), zap.String("path", probeFile))
	}

	return absPath, nil
}

// enableGormOpenTelemetry attaches the OpenTelemetry plugin to the provided GORM DB instance.
func enableGormOpenTelemetry(db *gorm.DB, dbName string) error {
	if !config.OpenTelemetryEnabled {
		return nil
	}

	if db == nil {
		return errors.Errorf("gorm db is nil for OpenTelemetry registration (%s)", dbName)
	}

	plugin := tracing.NewPlugin(
		tracing.WithTracerProvider(otel.GetTracerProvider()),
	)

	if err := db.Use(plugin); err != nil {
		return errors.Wrapf(err, "attach OpenTelemetry plugin to %s database", dbName)
	}

	return nil
}

func InitDB() {
	var err error
	DB, err = chooseDB(config.SQLDSN)
	if err != nil {
		logger.Logger.Fatal("failed to initialize database", zap.Error(err))
		return
	}

	if config.OpenTelemetryEnabled {
		if err = enableGormOpenTelemetry(DB, "primary"); err != nil {
			logger.Logger.Fatal("failed to enable OpenTelemetry for primary database", zap.Error(err))
			return
		}
	}

	if config.DebugSQLEnabled {
		logger.Logger.Debug("debug sql enabled")
		DB = DB.Debug()
	}

	sqlDB := setDBConns(DB)

	if !config.IsMasterNode {
		return
	}

	if common.UsingMySQL.Load() {
		_, _ = sqlDB.Exec("DROP INDEX idx_channels_key ON channels;") // TODO: delete this line when most users have upgraded
	}

	logger.Logger.Info("database migration started")

	// STEP 0: Ensure GORM has created every table/column before bespoke migrations touch them.
	// AutoMigrate adds any missing schema elements without attempting destructive changes, giving
	// a stable baseline so subsequent migrations can safely assume column presence.
	if err = migrateDB(); err != nil {
		logger.Logger.Fatal("failed to ensure base database schema", zap.Error(err))
		return
	}
	logger.Logger.Info("database base schema ensured")

	// STEP 1: Schema normalization prior to the main AutoMigrate pass
	// 1a) Normalize legacy ability suspend_until column types before AutoMigrate touches the table
	if err = MigrateAbilitySuspendUntilColumn(); err != nil {
		logger.Logger.Fatal("failed to migrate ability suspend_until column", zap.Error(err))
		return
	}

	// 1b) Migrate ModelConfigs and ModelMapping columns from varchar(1024) to text
	// This must run BEFORE AutoMigrate to ensure schema compatibility
	if err = MigrateChannelFieldsToText(); err != nil {
		logger.Logger.Fatal("failed to migrate channel field types", zap.Error(err))
		return
	}

	// 1c) Ensure traces.url can store long URLs (Turnstile tokens, etc.)
	if err = MigrateTraceURLColumnToText(); err != nil {
		logger.Logger.Fatal("failed to migrate traces.url column", zap.Error(err))
		return
	}

	// 1d) Ensure user_request_costs has a unique index on request_id and deduplicate old data quietly
	if err = MigrateUserRequestCostEnsureUniqueRequestID(); err != nil {
		logger.Logger.Fatal("failed to migrate user_request_costs unique index", zap.Error(err))
		return
	}

	// STEP 2: Run GORM AutoMigrate on all models to pick up any structural changes introduced above
	if err = migrateDB(); err != nil {
		logger.Logger.Fatal("failed to migrate database", zap.Error(err))
		return
	}
	logger.Logger.Info("database schema migrated")

	// Run post-migration adjustments to ensure new installs have expected schema specifics.
	if err = MigrateUserRequestCostEnsureUniqueRequestID(); err != nil {
		logger.Logger.Fatal("failed to finalize user_request_costs unique index", zap.Error(err))
		return
	}

	// STEP 3: Migrate existing ModelConfigs data from old format to new format
	// This handles data format changes after schema is correct
	if err = MigrateCustomChannelsToOpenAICompatible(); err != nil {
		logger.Logger.Fatal("failed to migrate custom channels", zap.Error(err))
		return
	}

	if err = MigrateAllChannelModelConfigs(); err != nil {
		logger.Logger.Error("failed to migrate channel ModelConfigs", zap.Error(err))
		// Don't fail startup for this migration, just log the error
	}

	if err = MigrateChannelLegacyImagePricing(); err != nil {
		logger.Logger.Error("failed to migrate legacy image pricing", zap.Error(err))
	}

	logger.Logger.Info("database migration completed")
}

func migrateDB() error {
	var err error
	if err = DB.AutoMigrate(&Channel{}); err != nil {
		return errors.Wrapf(err, "failed to migrate Channel")
	}
	if err = DB.AutoMigrate(&Token{}); err != nil {
		return errors.Wrapf(err, "failed to migrate Token")
	}
	if err = DB.AutoMigrate(&User{}); err != nil {
		if !shouldIgnoreDuplicateColumn(err, "mcp_tool_blacklist") {
			return errors.Wrapf(err, "failed to migrate User")
		}
	}
	if err = DB.AutoMigrate(&Option{}); err != nil {
		return errors.Wrapf(err, "failed to migrate Option")
	}
	if err = DB.AutoMigrate(&Redemption{}); err != nil {
		return errors.Wrapf(err, "failed to migrate Redemption")
	}
	if err = DB.AutoMigrate(&Ability{}); err != nil {
		return errors.Wrapf(err, "failed to migrate Ability")
	}
	if err = DB.AutoMigrate(&Log{}); err != nil {
		return errors.Wrapf(err, "failed to migrate Log")
	}
	if err = DB.AutoMigrate(&TokenTransaction{}); err != nil {
		return errors.Wrapf(err, "failed to migrate TokenTransaction")
	}
	if err = DB.AutoMigrate(&UserRequestCost{}); err != nil {
		return errors.Wrapf(err, "failed to migrate UserRequestCost")
	}
	if err = DB.AutoMigrate(&Trace{}); err != nil {
		return errors.Wrapf(err, "failed to migrate Trace")
	}
	if err = DB.AutoMigrate(&AsyncTaskBinding{}); err != nil {
		return errors.Wrapf(err, "failed to migrate AsyncTaskBinding")
	}
	if err = DB.AutoMigrate(&MCPServer{}); err != nil {
		if !shouldIgnoreDuplicateColumn(err, "priority") {
			return errors.Wrapf(err, "failed to migrate MCPServer")
		}
	}
	if err = DB.AutoMigrate(&MCPTool{}); err != nil {
		return errors.Wrapf(err, "failed to migrate MCPTool")
	}
	return nil
}

// shouldIgnoreDuplicateColumn reports whether a migration error can be ignored.
// This avoids startup failures when a column already exists.
func shouldIgnoreDuplicateColumn(err error, column string) bool {
	if err == nil || strings.TrimSpace(column) == "" {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate column") && strings.Contains(message, strings.ToLower(column))
}

func InitLogDB() {
	if config.LogSQLDSN == "" {
		LOG_DB = DB
		return
	}

	logger.Logger.Info("using secondary database for table logs")
	var err error
	LOG_DB, err = chooseDB(config.LogSQLDSN)
	if err != nil {
		logger.Logger.Fatal("failed to initialize secondary database", zap.Error(err))
		return
	}

	if config.OpenTelemetryEnabled && LOG_DB != DB {
		if err = enableGormOpenTelemetry(LOG_DB, "log"); err != nil {
			logger.Logger.Fatal("failed to enable OpenTelemetry for log database", zap.Error(err))
			return
		}
	}

	setDBConns(LOG_DB)

	if !config.IsMasterNode {
		return
	}

	logger.Logger.Info("secondary database migration started")
	err = migrateLOGDB()
	if err != nil {
		logger.Logger.Fatal("failed to migrate secondary database", zap.Error(err))
		return
	}
	logger.Logger.Info("secondary database migrated")
}

func migrateLOGDB() error {
	var err error
	if err = LOG_DB.AutoMigrate(&Log{}); err != nil {
		return errors.Wrap(err, "auto migrate log database")
	}
	return nil
}

func setDBConns(db *gorm.DB) *sql.DB {
	sqlDB, err := db.DB()
	if err != nil {
		logger.Logger.Fatal("failed to connect database", zap.Error(err))
		return nil
	}

	// Increase default connection pool sizes to handle billing load better
	maxIdleConns := config.SQLMaxIdleConns      // Increased from 100
	maxOpenConns := config.SQLMaxOpenConns      // Increased from 1000
	maxLifetime := config.SQLMaxLifetimeSeconds // Increased from 60 seconds

	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Second * time.Duration(maxLifetime))

	// Log connection pool settings for monitoring
	logger.Logger.Info("Database connection pool configured",
		zap.Int("max_idle_conns", maxIdleConns),
		zap.Int("max_open_conns", maxOpenConns),
		zap.Int("max_lifetime_secs", maxLifetime))

	// Start connection pool monitoring goroutine
	go monitorDBConnections(sqlDB)

	return sqlDB
}

// monitorDBConnections monitors database connection pool health
func monitorDBConnections(sqlDB *sql.DB) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		stats := sqlDB.Stats()

		// Log warning if connection pool is under stress
		if stats.InUse > int(float64(stats.MaxOpenConnections)*0.8) {
			usagePercent := float64(stats.InUse) / float64(stats.MaxOpenConnections) * 100
			logger.Logger.Error("HIGH DB CONNECTION USAGE",
				zap.Int("in_use", stats.InUse),
				zap.Int("max_open", stats.MaxOpenConnections),
				zap.Float64("usage_percent", usagePercent),
				zap.Int("idle", stats.Idle),
				zap.Int64("wait_count", stats.WaitCount),
				zap.Duration("wait_duration", stats.WaitDuration))
		}

		// Log critical error if we're hitting connection limits
		if stats.WaitCount > 0 && stats.WaitDuration > time.Second {
			logger.Logger.Error("CRITICAL DB CONNECTION BOTTLENECK - Consider increasing SQL_MAX_OPEN_CONNS",
				zap.Int64("wait_count", stats.WaitCount),
				zap.Duration("wait_duration", stats.WaitDuration))
		}
	}
}

func closeDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return errors.WithStack(err)
	}
	err = sqlDB.Close()
	return errors.WithStack(err)
}

func CloseDB() error {
	if LOG_DB != DB {
		err := closeDB(LOG_DB)
		if err != nil {
			return errors.Wrap(err, "close log database")
		}
	}
	return closeDB(DB)
}
