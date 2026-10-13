package controller_test

import (
	"context"
	"database/sql"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// billingTransportAuditBackends runs unchanged accounting scenarios on each supported backend without parallel global-state mutation.
func billingTransportAuditBackends(t *testing.T, run func(*testing.T, string)) {
	t.Helper()
	for _, backend := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			if backend != "sqlite" {
				billingTransportAuditDSN(t, backend)
			}
			run(t, backend)
		})
	}
}

// billingTransportAuditDSN returns an existing test DSN and fails required qualification when either live engine is unavailable.
func billingTransportAuditDSN(t *testing.T, backend string) string {
	t.Helper()
	env := "MYSQL_DSN"
	if backend == "postgres" {
		env = "PG_DSN"
	}
	dsn := os.Getenv(env)
	if dsn == "" {
		if os.Getenv("ONEAPI_REQUIRE_DB_BACKENDS") == "1" {
			t.Fatalf("%s is required for transport accounting qualification", env)
		}
		t.Skip(env + " not configured")
	}
	return dsn
}

// billingTransportAuditLoopback pins localhost to a literal loopback address and rejects remote endpoints without DNS resolution.
func billingTransportAuditLoopback(t *testing.T, host string) string {
	t.Helper()
	if host == "localhost" {
		return "127.0.0.1"
	}
	ip := net.ParseIP(host)
	require.True(t, ip != nil && ip.IsLoopback(), "transport billing fixtures require literal loopback or localhost database endpoints")
	return host
}

// billingTransportAuditStores opens isolated stores and registers closure/drop before the caller registers global restoration.
// The admin connection only creates/drops generated databases; no original database table is read, migrated or removed.
func billingTransportAuditStores(t *testing.T, backend string) (*gorm.DB, *gorm.DB) {
	t.Helper()
	if backend == "sqlite" {
		return billingTransportAuditOpenSQLite(t), billingTransportAuditOpenSQLite(t)
	}
	require.Contains(t, []string{"mysql", "postgres"}, backend)
	dsn := billingTransportAuditDSN(t, backend)
	open := billingTransportAuditLiveOpener(t, backend, dsn)
	admin := open("")
	create := func() *gorm.DB {
		name := "transportbill_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		// This identifier is generated entirely here and contains only lowercase ASCII letters, digits and underscore.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		require.NoError(t, admin.WithContext(ctx).Exec("CREATE DATABASE "+name).Error)
		t.Cleanup(func() {
			billingTransportAuditDrain(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			require.NoError(t, admin.WithContext(ctx).Exec("DROP DATABASE "+name).Error)
		})
		return open(name)
	}
	return create(), create()
}

// billingTransportAuditOpenSQLite opens an owned single-connection store and closes it after tracked work has drained.
func billingTransportAuditOpenSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Discard})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() {
		billingTransportAuditDrain(t)
		require.NoError(t, pool.Close())
	})
	return db
}

// billingTransportAuditLiveOpener parses and pins every endpoint before returning an opener for the admin or a generated database.
// Parser/open failures report no DSN or credential-bearing driver message. Both existing PostgreSQL DSN formats are accepted.
func billingTransportAuditLiveOpener(t *testing.T, backend, dsn string) func(string) *gorm.DB {
	t.Helper()
	if backend == "mysql" {
		cfg, err := mysqldriver.ParseDSN(dsn)
		require.True(t, err == nil, "MYSQL_DSN could not be parsed")
		require.Equal(t, "tcp", cfg.Net, "transport billing fixtures require MySQL TCP loopback")
		host, port, err := net.SplitHostPort(cfg.Addr)
		require.True(t, err == nil, "MYSQL_DSN must have a host:port endpoint")
		cfg.Addr = net.JoinHostPort(billingTransportAuditLoopback(t, host), port)
		cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 5*time.Second, 15*time.Second, 15*time.Second
		return func(name string) *gorm.DB {
			cloned := cfg.Clone()
			if name != "" {
				cloned.DBName = name
			}
			pool, err := sql.Open("mysql", cloned.FormatDSN())
			require.True(t, err == nil, "could not open isolated MySQL fixture pool")
			return billingTransportAuditOpenPool(t, pool, mysql.New(mysql.Config{Conn: pool, SkipInitializeWithVersion: true}))
		}
	}
	cfg, err := pgx.ParseConfig(dsn)
	require.True(t, err == nil, "PG_DSN could not be parsed")
	cfg.Host = billingTransportAuditLoopback(t, cfg.Host)
	for _, fallback := range cfg.Fallbacks {
		fallback.Host = billingTransportAuditLoopback(t, fallback.Host)
	}
	cfg.ConnectTimeout = 5 * time.Second
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	return func(name string) *gorm.DB {
		cloned := cfg.Copy()
		if name != "" {
			cloned.Database = name
		}
		pool := stdlib.OpenDB(*cloned)
		return billingTransportAuditOpenPool(t, pool, postgres.New(postgres.Config{Conn: pool, PreferSimpleProtocol: true}))
	}
}

// billingTransportAuditOpenPool owns a bounded pool, verifies connectivity and returns a silent ORM handle without inspecting existing tables.
func billingTransportAuditOpenPool(t *testing.T, pool *sql.DB, dialect gorm.Dialector) *gorm.DB {
	t.Helper()
	pool.SetMaxOpenConns(4)
	pool.SetMaxIdleConns(4)
	t.Cleanup(func() {
		billingTransportAuditDrain(t)
		require.NoError(t, pool.Close())
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.True(t, pool.PingContext(ctx) == nil, "loopback test database connection failed")
	db, err := gorm.Open(dialect, &gorm.Config{Logger: gormlogger.Discard, DisableAutomaticPing: true})
	require.True(t, err == nil, "could not initialize isolated database fixture")
	return db
}
