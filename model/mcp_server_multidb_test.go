package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
)

// TestUpdateMCPServerMultiDB validates the UpdateMCPServer() empty-value
// contract on every supported dialect: in-memory SQLite (always), MySQL (gated
// on MYSQL_DSN) and PostgreSQL (gated on PG_DSN).
//
// The contract:
//
//   - When a column is flagged in ProvidedFields and the struct value is the
//     zero/empty form (string "", JSONStringMap{}, JSONStringSlice(nil),
//     MCPToolPricingMap(nil), false), the update path must persist it instead of
//     letting GORM's struct-based Updates skip it.
//   - For non-pointer string columns (Description), the column ends up '' (NOT
//     NULL) on every backend.
//   - For JSON-serialized columns whose Value() returns (nil, nil) on empty
//     input (Headers, ToolWhitelist, ToolPricing), the column ends up SQL NULL
//     on PG/MySQL; SQLite may store NULL or an empty string and either is
//     acceptable.
//   - When a column is NOT in ProvidedFields, GORM's zero-value-skip behavior
//     must keep the previous value intact.
func TestUpdateMCPServerMultiDB(t *testing.T) {
	dialects := []string{"sqlite", "mysql", "postgres"}
	for _, dialect := range dialects {
		dialect := dialect
		t.Run(dialect, func(t *testing.T) {
			// Capture global state FIRST, before openMCPServerBackend (which
			// calls openBackend → mutates common.Using* flags). Otherwise the
			// "original" snapshot reflects swapped flags and cleanup leaks
			// dialect state into later tests in the package.
			originalDB := DB
			originalUsingSQLite := common.UsingSQLite.Load()
			originalUsingMySQL := common.UsingMySQL.Load()
			originalUsingPostgreSQL := common.UsingPostgreSQL.Load()
			t.Cleanup(func() {
				DB = originalDB
				common.UsingSQLite.Store(originalUsingSQLite)
				common.UsingMySQL.Store(originalUsingMySQL)
				common.UsingPostgreSQL.Store(originalUsingPostgreSQL)
			})

			db := openMCPServerBackend(t, dialect)
			if db == nil {
				switch dialect {
				case "mysql":
					t.Skip("MYSQL_DSN not set, skipping MySQL matrix test")
				case "postgres":
					t.Skip("PG_DSN not set, skipping PostgreSQL matrix test")
				default:
					t.Fatalf("openMCPServerBackend returned nil for dialect %q", dialect)
				}
				return
			}

			DB = db
			switch dialect {
			case "sqlite":
				common.UsingSQLite.Store(true)
				common.UsingMySQL.Store(false)
				common.UsingPostgreSQL.Store(false)
			case "mysql":
				common.UsingSQLite.Store(false)
				common.UsingMySQL.Store(true)
				common.UsingPostgreSQL.Store(false)
			case "postgres":
				common.UsingSQLite.Store(false)
				common.UsingMySQL.Store(false)
				common.UsingPostgreSQL.Store(true)
			}

			runMCPServerMatrixCases(t, db, dialect)
		})
	}
}

// openMCPServerBackend opens a fresh GORM connection and AutoMigrates MCPServer
// for the requested dialect. Returns nil when the gating env var is not set.
func openMCPServerBackend(t *testing.T, dialect string) *gorm.DB {
	t.Helper()

	var db *gorm.DB
	switch dialect {
	case "sqlite":
		conn, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, errors.Wrap(err, "open sqlite for mcp matrix"))
		db = conn
	case "mysql", "postgres":
		db = openBackend(t, dialect)
		if db == nil {
			return nil
		}
		_ = db.Migrator().DropTable("mcp_servers")
	default:
		t.Fatalf("unknown dialect %q", dialect)
	}

	require.NoError(t, errors.Wrapf(
		db.AutoMigrate(&MCPServer{}),
		"auto-migrate mcp_servers table for %s", dialect))

	if dialect != "sqlite" {
		t.Cleanup(func() {
			_ = db.Migrator().DropTable("mcp_servers")
		})
	}
	return db
}

// runMCPServerMatrixCases drives the per-dialect sub-tests through the same
// scenarios so coverage is uniform across backends.
func runMCPServerMatrixCases(t *testing.T, db *gorm.DB, dialect string) {
	insertSeededServer := func(t *testing.T, label string) *MCPServer {
		t.Helper()
		name := fmt.Sprintf("mcp-%s-%s-%d", dialect, label, time.Now().UnixNano())
		server := &MCPServer{
			Name:                    name,
			Description:             "seeded description",
			BaseURL:                 "https://example.com/mcp",
			Protocol:                MCPProtocolStreamableHTTP,
			AuthType:                MCPAuthTypeNone,
			Headers:                 JSONStringMap{"X-Seed": "yes"},
			ToolWhitelist:           JSONStringSlice{"tool-a", "tool-b"},
			ToolPricing:             MCPToolPricingMap{"tool-a": ToolPricingLocal{UsdPerCall: 0.01}},
			AutoSyncEnabled:         true,
			AutoSyncIntervalMinutes: 60,
			Status:                  MCPServerStatusEnabled,
		}
		require.NoError(t, errors.Wrapf(CreateMCPServer(server),
			"insert seeded mcp server for dialect=%s label=%s", dialect, label))

		var reloaded MCPServer
		require.NoError(t, errors.Wrapf(
			db.First(&reloaded, "id = ?", server.Id).Error,
			"reload seeded mcp server id=%d", server.Id))
		require.Equal(t, "seeded description", reloaded.Description)
		require.NotEmpty(t, reloaded.Headers)
		require.NotEmpty(t, reloaded.ToolWhitelist)
		require.NotEmpty(t, reloaded.ToolPricing)
		return &reloaded
	}

	t.Run("clear via empty/false/zero + provided flag", func(t *testing.T) {
		server := insertSeededServer(t, "clear")

		server.Description = ""
		server.Headers = JSONStringMap{}
		server.ToolWhitelist = nil
		server.ToolPricing = nil
		server.AutoSyncEnabled = false
		server.ProvidedFields = map[string]bool{
			"description":       true,
			"headers":           true,
			"tool_whitelist":    true,
			"tool_pricing":      true,
			"auto_sync_enabled": true,
		}
		require.NoError(t, errors.Wrap(UpdateMCPServer(server),
			"update mcp server to clear fields"))

		// description is non-pointer string => '' (NOT NULL) on every dialect.
		desc := readNullStringColumn(t, db, "mcp_servers", "description", server.Id)
		require.True(t, desc.Valid,
			"%s: description must be NOT NULL after clear", dialect)
		assert.Equal(t, "", desc.String,
			"%s: description must be empty string after clear", dialect)

		// JSON columns: Value() returns (nil, nil) for empty input. On PG/MySQL
		// that means SQL NULL. On SQLite, NULL or empty string are both fine.
		jsonColumns := []string{"headers", "tool_whitelist", "tool_pricing"}
		for _, col := range jsonColumns {
			got := readNullStringColumn(t, db, "mcp_servers", col, server.Id)
			switch dialect {
			case "sqlite":
				if got.Valid {
					assert.Equal(t, "", got.String,
						"sqlite: %s should be NULL or empty after clear", col)
				}
			default:
				assert.False(t, got.Valid,
					"%s: column %s must be SQL NULL after clear (got %q)",
					dialect, col, got.String)
			}
		}

		// auto_sync_enabled must reflect the false write.
		var enabled bool
		require.NoError(t, errors.Wrap(
			db.Raw("SELECT auto_sync_enabled FROM mcp_servers WHERE id = ?", server.Id).
				Scan(&enabled).Error,
			"raw select auto_sync_enabled"))
		assert.False(t, enabled,
			"%s: auto_sync_enabled must be false after clear", dialect)
	})

	t.Run("omitted preserves previous JSON values", func(t *testing.T) {
		server := insertSeededServer(t, "preserve")

		// Frontend only changed Description. JSON columns are left as the
		// reloaded values; ProvidedFields stays empty so GORM's struct-based
		// Updates path is the only thing that runs.
		server.Description = "rewritten description"
		server.ProvidedFields = nil
		require.NoError(t, errors.Wrap(UpdateMCPServer(server),
			"update mcp server with no provided fields"))

		desc := readNullStringColumn(t, db, "mcp_servers", "description", server.Id)
		require.True(t, desc.Valid)
		assert.Equal(t, "rewritten description", desc.String,
			"%s: description must reflect the struct write", dialect)

		// Read JSON columns back through the model so we exercise the Scan
		// path (the actual contract is "value preserved", not "specific raw
		// SQL representation").
		var reloaded MCPServer
		require.NoError(t, errors.Wrapf(
			db.First(&reloaded, "id = ?", server.Id).Error,
			"reload mcp server id=%d after preserve update", server.Id))
		require.NotEmpty(t, reloaded.Headers,
			"%s: headers must be preserved when not provided", dialect)
		assert.Equal(t, "yes", reloaded.Headers["X-Seed"])
		require.NotEmpty(t, reloaded.ToolWhitelist,
			"%s: tool_whitelist must be preserved when not provided", dialect)
		assert.Contains(t, reloaded.ToolWhitelist, "tool-a")
		require.NotEmpty(t, reloaded.ToolPricing,
			"%s: tool_pricing must be preserved when not provided", dialect)
		_, ok := reloaded.ToolPricing["tool-a"]
		assert.True(t, ok, "%s: tool_pricing must still contain tool-a", dialect)
	})
}

// readNullStringColumn is defined in channel_nullable_update_multidb_test.go
// (same package); reused here for raw SELECTs that distinguish SQL NULL from
// empty string without struct-mapping ambiguity.
