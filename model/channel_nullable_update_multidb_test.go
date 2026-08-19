package model

import (
	"database/sql"
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

// shortDialect returns a compact label for use inside a varchar(32) column.
func shortDialect(d string) string {
	switch d {
	case "postgres":
		return "pg"
	case "mysql":
		return "my"
	default:
		return "sl"
	}
}

// TestChannelUpdateNullableFieldsMultiDB exercises the Channel.Update() nullable
// text-column contract (model_mapping, model_configs, system_prompt,
// inference_profile_arn_map) on every supported dialect: in-memory SQLite (always),
// MySQL (gated on MYSQL_DSN) and PostgreSQL (gated on PG_DSN).
//
// The single-DB SQLite-only counterpart lives in channel_nullable_update_test.go;
// this matrix variant locks in the same behavior on real backends so cross-dialect
// regressions in GORM's Updates(map) handling of nil/empty are caught before they
// reach production.
func TestChannelUpdateNullableFieldsMultiDB(t *testing.T) {
	dialects := []string{"sqlite", "mysql", "postgres"}
	for _, dialect := range dialects {
		dialect := dialect
		t.Run(dialect, func(t *testing.T) {
			// Capture global state FIRST, before openChannelNullableBackend
			// (which calls openBackend, which mutates common.Using* flags).
			// Otherwise our "original" snapshot already reflects the swapped
			// flags and cleanup leaks dialect state into later tests.
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

			db := openChannelNullableBackend(t, dialect)
			if db == nil {
				switch dialect {
				case "mysql":
					t.Skip("MYSQL_DSN not set, skipping MySQL matrix test")
				case "postgres":
					t.Skip("PG_DSN not set, skipping PostgreSQL matrix test")
				default:
					t.Fatalf("openChannelNullableBackend returned nil for dialect %q", dialect)
				}
				return
			}

			// Swap globals so model code uses the right DB and dialect flags.
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

			runChannelNullableMatrixCases(t, db, dialect)
		})
	}
}

// openChannelNullableBackend opens a fresh GORM connection for the requested
// dialect, AutoMigrates Channel + Ability, and returns the *gorm.DB. For the
// shared MySQL/PG instances, it drops and re-creates the channel/ability
// tables so each run starts from a clean slate. Returns nil when the gating
// env var is not set so callers can Skip cleanly.
func openChannelNullableBackend(t *testing.T, dialect string) *gorm.DB {
	t.Helper()

	var db *gorm.DB
	switch dialect {
	case "sqlite":
		conn, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, errors.Wrap(err, "open sqlite for channel nullable matrix"))
		db = conn
	case "mysql", "postgres":
		db = openBackend(t, dialect)
		if db == nil {
			return nil
		}
		// Best-effort cleanup: a previous run may have left tables behind.
		_ = db.Migrator().DropTable("abilities", "channels")
	default:
		t.Fatalf("unknown dialect %q", dialect)
	}

	require.NoError(t, errors.Wrapf(
		db.AutoMigrate(&Channel{}, &Ability{}),
		"auto-migrate channel/ability tables for %s", dialect))

	if dialect != "sqlite" {
		// Make repeat runs idempotent without leaking the schema.
		t.Cleanup(func() {
			_ = db.Migrator().DropTable("abilities", "channels")
		})
	}
	return db
}

// runChannelNullableMatrixCases wraps the per-dialect sub-tests so each dialect
// goes through the same three scenarios.
func runChannelNullableMatrixCases(t *testing.T, db *gorm.DB, dialect string) {
	insertSeededChannel := func(t *testing.T, label string) *Channel {
		t.Helper()
		// Channel.Group is varchar(32) — keep label+suffix well under that on
		// strict dialects (MySQL/PostgreSQL truncate-rejecting; SQLite is permissive).
		ts := time.Now().UnixNano()
		group := fmt.Sprintf("g-%s-%s-%d", shortDialect(dialect), label, ts%1_000_000)
		ch := &Channel{
			Name:                   fmt.Sprintf("nullable-%s-%s-%d", dialect, label, ts),
			Type:                   1,
			Status:                 ChannelStatusEnabled,
			Models:                 "gpt-3.5-turbo",
			Group:                  group,
			ModelMapping:           stringPtr(`{"foo":"bar"}`),
			ModelConfigs:           stringPtr(`{"gpt-3.5-turbo":{"ratio":1}}`),
			SystemPrompt:           stringPtr("you are a helpful assistant"),
			InferenceProfileArnMap: stringPtr(`{"gpt-3.5-turbo":"arn:aws:bedrock:us-east-1:123:inference-profile/foo"}`),
		}
		require.NoError(t, errors.Wrapf(ch.Insert(),
			"insert seeded channel for dialect=%s label=%s", dialect, label))

		var reloaded Channel
		require.NoError(t, errors.Wrapf(
			db.First(&reloaded, "id = ?", ch.Id).Error,
			"reload seeded channel id=%d", ch.Id))
		require.NotNil(t, reloaded.ModelMapping)
		require.NotNil(t, reloaded.ModelConfigs)
		require.NotNil(t, reloaded.SystemPrompt)
		require.NotNil(t, reloaded.InferenceProfileArnMap)
		return &reloaded
	}

	t.Run("clear via nil pointer + provided flag", func(t *testing.T) {
		channel := insertSeededChannel(t, "clear")

		channel.ModelMapping = nil
		channel.ModelConfigs = nil
		channel.SystemPrompt = nil
		channel.InferenceProfileArnMap = nil
		channel.NullableFieldsProvided = map[string]bool{
			"model_mapping":             true,
			"model_configs":             true,
			"system_prompt":             true,
			"inference_profile_arn_map": true,
		}
		require.NoError(t, errors.Wrap(channel.Update(),
			"update channel to clear nullable fields"))

		// Read raw column values to inspect SQL NULL vs empty string directly.
		columns := []string{
			"model_mapping",
			"model_configs",
			"system_prompt",
			"inference_profile_arn_map",
		}
		for _, col := range columns {
			got := readNullStringColumn(t, db, "channels", col, channel.Id)
			switch dialect {
			case "sqlite":
				// On SQLite, NULL is expected, but if a layer normalizes to
				// empty string treat that as acceptable (mirrors permissive
				// assertion in channel_nullable_update_test.go).
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
	})

	t.Run("set via empty-string pointer", func(t *testing.T) {
		channel := insertSeededChannel(t, "empty")

		empty := ""
		channel.ModelMapping = &empty
		channel.ModelConfigs = stringPtr("")
		channel.SystemPrompt = stringPtr("")
		channel.InferenceProfileArnMap = stringPtr("")
		channel.NullableFieldsProvided = map[string]bool{
			"model_mapping":             true,
			"model_configs":             true,
			"system_prompt":             true,
			"inference_profile_arn_map": true,
		}
		require.NoError(t, errors.Wrap(channel.Update(),
			"update channel with empty-string pointers"))

		columns := []string{
			"model_mapping",
			"model_configs",
			"system_prompt",
			"inference_profile_arn_map",
		}
		for _, col := range columns {
			got := readNullStringColumn(t, db, "channels", col, channel.Id)
			require.True(t, got.Valid,
				"%s: column %s must be NOT NULL after empty-string write", dialect, col)
			assert.Equal(t, "", got.String,
				"%s: column %s must be empty string after empty-string write", dialect, col)
		}
	})

	t.Run("omitted preserves previous values", func(t *testing.T) {
		channel := insertSeededChannel(t, "preserve")

		origModelMapping := *channel.ModelMapping
		origModelConfigs := *channel.ModelConfigs
		origSystemPrompt := *channel.SystemPrompt
		origInferenceArn := *channel.InferenceProfileArnMap

		// Frontend omitted these fields entirely. nil pointers + empty
		// NullableFieldsProvided map. Tweak Name to ensure the row is
		// actually rewritten by GORM's struct-based Updates path.
		channel.ModelMapping = nil
		channel.ModelConfigs = nil
		channel.SystemPrompt = nil
		channel.InferenceProfileArnMap = nil
		channel.NullableFieldsProvided = map[string]bool{}
		channel.Name = channel.Name + "-renamed"
		require.NoError(t, errors.Wrap(channel.Update(),
			"update channel without provided flags"))

		got := readNullStringColumn(t, db, "channels", "model_mapping", channel.Id)
		require.True(t, got.Valid, "%s: model_mapping must be preserved", dialect)
		assert.Equal(t, origModelMapping, got.String)

		got = readNullStringColumn(t, db, "channels", "model_configs", channel.Id)
		require.True(t, got.Valid, "%s: model_configs must be preserved", dialect)
		assert.Equal(t, origModelConfigs, got.String)

		got = readNullStringColumn(t, db, "channels", "system_prompt", channel.Id)
		require.True(t, got.Valid, "%s: system_prompt must be preserved", dialect)
		assert.Equal(t, origSystemPrompt, got.String)

		got = readNullStringColumn(t, db, "channels", "inference_profile_arn_map", channel.Id)
		require.True(t, got.Valid, "%s: inference_profile_arn_map must be preserved", dialect)
		assert.Equal(t, origInferenceArn, got.String)
	})
}

// readNullStringColumn issues a raw SELECT for a single column so the test sees
// the SQL NULL vs empty string state without any GORM struct-mapping ambiguity.
func readNullStringColumn(t *testing.T, db *gorm.DB, table, column string, id int) sql.NullString {
	t.Helper()
	var v sql.NullString
	query := fmt.Sprintf("SELECT %s FROM %s WHERE id = ?", column, table)
	require.NoError(t, errors.Wrapf(
		db.Raw(query, id).Scan(&v).Error,
		"raw select %s.%s for id=%d", table, column, id))
	return v
}
