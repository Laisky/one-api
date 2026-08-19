package model

import (
	"context"
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

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/common/random"
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
		if err := DB.Create(&rootUser).Error; err != nil {
			return errors.Wrap(err, "create root user")
		}
		if config.InitialRootToken != "" {
			logger.Logger.Info("creating initial root token as requested")
			token := Token{
				Id:             1,
				UserId:         rootUser.Id,
				UserUUID:       &rootUser.UUID,
				Key:            config.InitialRootToken,
				Status:         TokenStatusEnabled,
				Name:           "Initial Root Token",
				CreatedTime:    helper.GetTimestamp(),
				AccessedTime:   helper.GetTimestamp(),
				ExpiredTime:    -1,
				RemainQuota:    500000000000000,
				UnlimitedQuota: true,
			}
			if err := DB.Create(&token).Error; err != nil {
				return errors.Wrap(err, "create initial root token")
			}
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

	// WAL lets readers run concurrently with a writer; synchronous=NORMAL
	// pairs safely with WAL (a crash loses at most the last commit, never
	// corrupts the DB). busy_timeout is the cap a connection waits when the
	// writer slot is held — combined with the existing sqlite_retry helper
	// this is the standard recipe for SQLite under multi-goroutine workloads.
	dsn := fmt.Sprintf("%s?_busy_timeout=%d&_journal_mode=WAL&_synchronous=NORMAL", sqlitePath, common.SQLiteBusyTimeout)
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

// InitDB initializes the primary database and runs its schema and data migrations.
// It retains the pre-patch contract for callers that never invoke InitLogDB by running a
// primary-only, marker-free catch-up. That path can never finalize: only the global
// coordinator reached through InitLogDB or InitDatabases has completion authority.
// Parameters: none.
//
// Return values: none; terminal errors are fatal at this bootstrap boundary.
func InitDB() {
	if err := initPrimaryDatabase(); err != nil {
		logger.Logger.Fatal("failed to initialize database", zap.Error(err))
		return
	}
	if !config.IsMasterNode {
		return
	}
	if err := runCompatibilityCatchUp(context.Background()); err != nil {
		logger.Logger.Fatal("failed to run primary external uuid catch-up", zap.Error(err))
		return
	}
}

// initPrimaryDatabase opens the primary database and applies schema and data migrations.
// It performs no UUID reconciliation so the bootstrap orchestrator can decide when the
// global coordinator runs.
// Parameters: none.
//
// Return values:
//   - error: wrapped error when the handle cannot be opened or a migration fails.
func initPrimaryDatabase() error {
	// Opening a new primary handle starts a new initialization generation, which clears the
	// previous generation's reconciliation ownership and stops its worker. Without this a
	// reinitialized process would keep a claim, and a topology, that point at replaced or
	// closed handles. The compact loops are stopped for the same reason and in the required
	// order: mutation workers are joined before health monitors, and both before the handle
	// they are issuing statements against is replaced.
	stopUUIDCatchUpWorker()
	stopCompactLoops()
	beginInitGeneration()
	setDatabaseTopology(nil)

	var err error
	DB, err = chooseDB(config.SQLDSN)
	if err != nil {
		return errors.Wrap(err, "open primary database")
	}

	if config.OpenTelemetryEnabled {
		if err = enableGormOpenTelemetry(DB, "primary"); err != nil {
			return errors.Wrap(err, "enable OpenTelemetry for primary database")
		}
	}

	if config.DebugSQLEnabled {
		logger.Logger.Debug("debug sql enabled")
		DB = DB.Debug()
	}

	sqlDB := setDBConns(DB)

	if !config.IsMasterNode {
		return nil
	}

	if common.UsingMySQL.Load() {
		_, _ = sqlDB.Exec("DROP INDEX idx_channels_key ON channels;") // TODO: delete this line when most users have upgraded
	}

	logger.Logger.Info("database migration started")

	// STEP 1: AutoMigrate on all models to create/update tables and columns.
	// GORM's AutoMigrate is contractually idempotent, but gorm.io/driver/sqlite's
	// ColumnTypes() introspection (regex-based parseDDL over sqlite_master.sql) is
	// known to mis-parse certain DDL states after ALTER TABLE ADD COLUMN. Calling
	// AutoMigrate more than once per process can therefore fail with
	// "duplicate column name" on SQLite. Keep this to a single invocation.
	if err = migrateDB(); err != nil {
		return errors.Wrap(err, "migrate database schema")
	}
	logger.Logger.Info("database schema migrated")

	// STEP 2: Custom migrations that normalize or adjust EXISTING columns/data.
	// None of these add new columns or tables — those live in the struct
	// definitions and are handled by STEP 1's AutoMigrate. Each is idempotent
	// and safe to run on every startup.

	// 2a) Normalize legacy ability suspend_until column values / type.
	if err = MigrateAbilitySuspendUntilColumn(); err != nil {
		return errors.Wrap(err, "migrate ability suspend_until column")
	}

	// 2b) Make MySQL ability model identity case-sensitive at the schema and index level.
	if err = MigrateAbilityModelCollation(); err != nil {
		return errors.Wrap(err, "migrate ability model collation")
	}

	// 2c) Convert ModelConfigs / ModelMapping columns from varchar(1024) to text on legacy MySQL/PG installs.
	if err = MigrateChannelFieldsToText(); err != nil {
		return errors.Wrap(err, "migrate channel field types")
	}

	// 2d) Ensure traces.url can store long URLs (Turnstile tokens, etc.).
	if err = MigrateTraceURLColumnToText(); err != nil {
		return errors.Wrap(err, "migrate traces.url column")
	}

	// 2e) Ensure user_request_costs has a unique index on request_id and deduplicate old data quietly.
	if err = MigrateUserRequestCostEnsureUniqueRequestID(); err != nil {
		return errors.Wrap(err, "migrate user_request_costs unique index")
	}

	// STEP 3: Data-format migrations (schema is already correct at this point).
	if err = MigrateCustomChannelsToOpenAICompatible(); err != nil {
		return errors.Wrap(err, "migrate custom channels")
	}

	if err = MigrateAllChannelModelConfigs(); err != nil {
		logger.Logger.Error("failed to migrate channel ModelConfigs", zap.Error(err))
		// Don't fail startup for this migration, just log the error
	}

	if err = MigrateChannelLegacyImagePricing(); err != nil {
		logger.Logger.Error("failed to migrate legacy image pricing", zap.Error(err))
	}

	logger.Logger.Info("database migration completed")
	return nil
}

func migrateDB() error {
	var err error
	if err = DB.AutoMigrate(&Channel{}); err != nil {
		if !shouldIgnoreDuplicateColumn(err, "hidden_models") {
			return errors.Wrapf(err, "failed to migrate Channel")
		}
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
	// In split mode LOG_DB is the only authoritative owner of logs, so the primary must not
	// gain or keep evolving a stale logs table. migrateLOGDB owns that schema instead. A
	// logs table left over from a unified deployment is simply ignored; every log read and
	// write in this package goes through LOG_DB.
	if config.LogSQLDSN == "" {
		if err = DB.AutoMigrate(&Log{}); err != nil {
			return errors.Wrapf(err, "failed to migrate Log")
		}
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
	if err = DB.AutoMigrate(&PasskeyCredential{}); err != nil {
		return errors.Wrapf(err, "failed to migrate PasskeyCredential")
	}
	if err = DB.AutoMigrate(&PaymentOrder{}); err != nil {
		return errors.Wrapf(err, "failed to migrate PaymentOrder")
	}
	if err = DB.AutoMigrate(&StripeWebhookEvent{}); err != nil {
		return errors.Wrapf(err, "failed to migrate StripeWebhookEvent")
	}
	if err = DB.AutoMigrate(&DataMigration{}); err != nil {
		return errors.Wrapf(err, "failed to migrate DataMigration")
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

// InitLogDB completes topology initialization and may invoke the global coordinator.
// Unlike the primary-only InitDB catch-up, this path can finalize split-database state.
// Parameters: none.
//
// Return values: none; terminal errors are fatal at this bootstrap boundary.
func InitLogDB() {
	topology, err := initLogDatabase()
	if err != nil {
		logger.Logger.Fatal("failed to initialize secondary database", zap.Error(err))
		return
	}
	setDatabaseTopology(topology)

	if !config.IsMasterNode {
		return
	}
	if err := runWrapperUUIDMigration(context.Background(), topology); err != nil {
		logger.Logger.Fatal("failed to migrate external resource uuids", zap.Error(err))
		return
	}
}

// initLogDatabase opens the log database when configured and returns the explicit topology.
// Unified mode is selected by the configuration path that assigns LOG_DB to DB; split mode
// is selected by a dedicated log DSN. Neither decision compares gorm.DB pointers, so a
// deployment pointing both DSNs at one physical server is still treated as split.
// Parameters: none.
//
// Return values:
//   - *databaseTopology: explicitly constructed topology.
//   - error: wrapped error when the handle cannot be opened or the schema fails to migrate.
func initLogDatabase() (*databaseTopology, error) {
	if DB == nil {
		return nil, errors.New("primary database must be initialized before the log database")
	}
	if config.LogSQLDSN == "" {
		LOG_DB = DB
		return newUnifiedTopology(DB)
	}

	logger.Logger.Info("using secondary database for table logs")
	var err error
	LOG_DB, err = chooseDB(config.LogSQLDSN)
	if err != nil {
		return nil, errors.Wrap(err, "open secondary database")
	}

	if config.OpenTelemetryEnabled {
		if err = enableGormOpenTelemetry(LOG_DB, "log"); err != nil {
			return nil, errors.Wrap(err, "enable OpenTelemetry for log database")
		}
	}

	setDBConns(LOG_DB)

	if config.IsMasterNode {
		logger.Logger.Info("secondary database migration started")
		if err = migrateLOGDB(); err != nil {
			return nil, errors.Wrap(err, "migrate secondary database")
		}
		logger.Logger.Info("secondary database migrated")
	}
	return newSplitTopology(DB, LOG_DB)
}

// runWrapperUUIDMigration runs the coordinator for the InitDB plus InitLogDB wrapper path.
// A unified deployment whose primary-only catch-up already ran in this process does not
// repeat the identical catch-up; finalizer mode always runs because the compatibility path
// can never finalize.
// Parameters:
//   - ctx: context bounding the migration and any background worker.
//   - topology: explicitly constructed database topology.
//
// Return values:
//   - error: wrapped error when finalizer-mode migration fails.
func runWrapperUUIDMigration(ctx context.Context, topology *databaseTopology) error {
	ctx = withUUIDMigrationLogger(ctx)
	if topology.mode == uuidTopologyUnified && !externalUUIDBackfillFinalizerEnabled && compatibilityCatchUpAlreadyRan() {
		return nil
	}
	return startExternalUUIDMigration(ctx, topology)
}

func migrateLOGDB() error {
	var err error
	if err = LOG_DB.AutoMigrate(&Log{}); err != nil {
		return errors.Wrap(err, "auto migrate log database")
	}
	if err = LOG_DB.AutoMigrate(&DataMigration{}); err != nil {
		return errors.Wrap(err, "auto migrate log data migrations")
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
	// Cancel and join every background loop before either database is closed. Both migration
	// generations own workers that issue statements, so a loop still in flight would run
	// against a closed pool.
	stopUUIDCatchUpWorker()
	stopCompactLoops()
	// LOG_DB is nil for an InitDB-only caller that never initialized the log database, so it
	// must be checked before use rather than only compared against DB.
	if LOG_DB != nil && LOG_DB != DB {
		err := closeDB(LOG_DB)
		if err != nil {
			return errors.Wrap(err, "close log database")
		}
	}
	if DB == nil {
		return nil
	}
	return closeDB(DB)
}
