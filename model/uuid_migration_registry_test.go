package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// uuidRegistryOwnerID is the shared row id of every reference owner in the legacy fixture.
// Owner rows carry explicit canonical UUIDs so each legacy row's FK UUID is fillable and its
// expected value is known before the coordinator runs.
const uuidRegistryOwnerID = 900

const (
	// uuidRegistryOwnerUserUUID is the pre-populated owned UUID of the fixture's users owner.
	uuidRegistryOwnerUserUUID = "018f0900-0000-7000-8000-000000000001"
	// uuidRegistryOwnerChannelUUID is the pre-populated owned UUID of the fixture's channels owner.
	uuidRegistryOwnerChannelUUID = "018f0900-0000-7000-8000-000000000002"
	// uuidRegistryOwnerTokenUUID is the pre-populated owned UUID of the fixture's tokens owner.
	uuidRegistryOwnerTokenUUID = "018f0900-0000-7000-8000-000000000003"
	// uuidRegistryOwnerServerUUID is the pre-populated owned UUID of the fixture's mcp_servers owner.
	uuidRegistryOwnerServerUUID = "018f0900-0000-7000-8000-000000000004"
	// uuidRegistryOwnerLogUUID is the pre-populated owned UUID of the fixture's logs owner.
	uuidRegistryOwnerLogUUID = "018f0900-0000-7000-8000-000000000005"
	// uuidRegistryOwnerTokenName is the unambiguous (user_id, name) key backing logs.token_uuid.
	uuidRegistryOwnerTokenName = "uuid-registry-owner-token"
)

// uuidRegistryNullRowID is the legacy row whose UUID columns are SQL NULL.
const uuidRegistryNullRowID = 1

// uuidRegistryEmptyRowID is the legacy row whose UUID columns are the empty string.
const uuidRegistryEmptyRowID = 2

// uuidRegistryLegacyIDs lists the two legacy row ids seeded into every registry table:
// the NULL pass first, the empty-string pass second.
var uuidRegistryLegacyIDs = []int{uuidRegistryNullRowID, uuidRegistryEmptyRowID}

// uuidRegistryExpectedTables is the authoritative owned-UUID table list from proposal section 5.1.
var uuidRegistryExpectedTables = []string{
	"users",
	"tokens",
	"channels",
	"redemptions",
	"logs",
	"token_transactions",
	"user_request_costs",
	"traces",
	"async_task_bindings",
	"mcp_servers",
	"mcp_tools",
	"passkey_credentials",
}

// uuidRegistryFKKey identifies one FK registry entry without its GORM model pointer.
// The registry allocates a fresh model value on every call, so entries are compared by their
// schema-describing fields rather than by struct equality.
type uuidRegistryFKKey struct {
	role       uuidDBRole
	table      string
	fkColumn   string
	uuidColumn string
	refRole    uuidDBRole
	refTable   string
	resolver   uuidResolverKind
}

// uuidRegistryKeyOf projects one FK registry entry onto its comparable key.
// Parameters:
//   - target: FK registry entry.
//
// Return values:
//   - uuidRegistryFKKey: comparable projection of target.
func uuidRegistryKeyOf(target uuidFKTarget) uuidRegistryFKKey {
	return uuidRegistryFKKey{
		role:       target.role,
		table:      target.table,
		fkColumn:   target.fkColumn,
		uuidColumn: target.uuidColumn,
		refRole:    target.refRole,
		refTable:   target.refTable,
		resolver:   target.resolver,
	}
}

// uuidRegistryExpectedFKColumns lists the 15 (table, uuidColumn) pairs from proposal section 5.2.
// Parameters: none.
//
// Return values:
//   - [][2]string: every denormalized FK UUID column as a (table, column) pair.
func uuidRegistryExpectedFKColumns() [][2]string {
	return [][2]string{
		{"users", "inviter_uuid"},
		{"tokens", "user_uuid"},
		{"redemptions", "user_uuid"},
		{"token_transactions", "token_uuid"},
		{"token_transactions", "user_uuid"},
		{"token_transactions", "log_uuid"},
		{"user_request_costs", "user_uuid"},
		{"async_task_bindings", "user_uuid"},
		{"async_task_bindings", "token_uuid"},
		{"async_task_bindings", "channel_uuid"},
		{"mcp_tools", "server_uuid"},
		{"passkey_credentials", "user_uuid"},
		{"logs", "user_uuid"},
		{"logs", "channel_uuid"},
		{"logs", "token_uuid"},
	}
}

// TestUUIDRegistryIsTheSingleSourceOfTruth covers UUID-A20: the registry drives every owned UUID,
// FK UUID, validation, and index target, and no target exists in only a subset of the phases.
func TestUUIDRegistryIsTheSingleSourceOfTruth(t *testing.T) {
	t.Run("owned registry matches proposal 5.1", func(t *testing.T) {
		owned := uuidOwnedRegistry()
		require.Len(t, owned, 12, "the owned registry must carry exactly the 12 tables of section 5.1")

		tables := make(map[string]int, len(owned))
		for _, target := range owned {
			tables[target.table]++
		}
		require.Len(t, tables, 12, "no owned table may appear twice")
		for _, table := range uuidRegistryExpectedTables {
			require.Equal(t, 1, tables[table], "owned registry must contain %s exactly once", table)
		}
		for table := range tables {
			require.Contains(t, uuidRegistryExpectedTables, table, "unexpected owned table %s", table)
		}
	})

	t.Run("logs is the only authoritative log owner", func(t *testing.T) {
		// P0 guarantee: in split mode the primary database's stale logs table is never a
		// target, because logs is served exclusively through the log role.
		logTables := []string{}
		primaryTables := []string{}
		for _, target := range uuidOwnedRegistry() {
			switch target.role {
			case uuidRoleLog:
				logTables = append(logTables, target.table)
			case uuidRolePrimary:
				primaryTables = append(primaryTables, target.table)
			default:
				require.Failf(t, "unknown owned role", "table %s has role %q", target.table, target.role)
			}
		}
		require.Equal(t, []string{"logs"}, logTables, "logs must be the only uuidRoleLog owned target")
		require.Len(t, primaryTables, 11, "the other 11 owned tables must be uuidRolePrimary")
		require.NotContains(t, primaryTables, "logs", "a primary logs table must never be an owned target")

		require.Len(t, ownedTargetsForRole(uuidRoleLog), 1)
		require.Equal(t, "logs", ownedTargetsForRole(uuidRoleLog)[0].table)
		require.Len(t, ownedTargetsForRole(uuidRolePrimary), 11)
	})

	t.Run("fk registry matches proposal 5.2", func(t *testing.T) {
		fks := uuidFKRegistry()
		require.Len(t, fks, 15, "the FK registry must carry exactly the 15 columns of section 5.2")

		got := make(map[[2]string]int, len(fks))
		for _, target := range fks {
			got[[2]string{target.table, target.uuidColumn}]++
		}
		require.Len(t, got, 15, "no (table, uuid column) pair may appear twice")
		for _, pair := range uuidRegistryExpectedFKColumns() {
			require.Equal(t, 1, got[pair], "FK registry must contain %s.%s exactly once", pair[0], pair[1])
		}
		for pair := range got {
			require.Contains(t, uuidRegistryExpectedFKColumns(), pair, "unexpected FK column %s.%s", pair[0], pair[1])
		}
	})

	t.Run("phase order covers the fk registry exactly once", func(t *testing.T) {
		want := map[uuidRegistryFKKey]struct{}{}
		for _, target := range uuidFKRegistry() {
			want[uuidRegistryKeyOf(target)] = struct{}{}
		}

		got := map[uuidRegistryFKKey]struct{}{}
		total := 0
		for _, phase := range uuidFKPhaseOrder() {
			for _, target := range fkTargetsForRoles(phase.role, phase.refRole) {
				key := uuidRegistryKeyOf(target)
				_, duplicate := got[key]
				require.False(t, duplicate, "%s.%s is reconciled by more than one phase", target.table, target.uuidColumn)
				got[key] = struct{}{}
				total++
			}
		}
		require.Equal(t, want, got, "the phase sequence must cover the FK registry exactly")
		require.Equal(t, 15, total, "the phase sequence must run all 15 FK targets exactly once")
	})

	t.Run("cross-database dependency direction", func(t *testing.T) {
		ownedByRole := map[uuidDBRole]map[string]struct{}{
			uuidRolePrimary: {},
			uuidRoleLog:     {},
		}
		for _, target := range uuidOwnedRegistry() {
			ownedByRole[target.role][target.table] = struct{}{}
		}

		for _, target := range uuidFKRegistry() {
			require.Contains(t, ownedByRole[target.refRole], target.refTable,
				"%s.%s references %s, which must be an owned-registry table of role %s",
				target.table, target.uuidColumn, target.refTable, target.refRole)

			switch {
			case target.table == "token_transactions" && target.uuidColumn == "log_uuid":
				// The only primary target resolved from the authoritative log database.
				require.Equal(t, uuidRolePrimary, target.role)
				require.Equal(t, uuidRoleLog, target.refRole)
				require.Equal(t, "logs", target.refTable)
			case target.table == "logs":
				// Every log FK target is resolved from primary owners, never the reverse.
				require.Equal(t, uuidRoleLog, target.role)
				require.Equal(t, uuidRolePrimary, target.refRole)
			default:
				require.Equal(t, uuidRolePrimary, target.role)
				require.Equal(t, uuidRolePrimary, target.refRole)
			}
		}
	})

	t.Run("every entry is well formed", func(t *testing.T) {
		for _, target := range uuidOwnedRegistry() {
			require.NotEmpty(t, target.table, "an owned entry must name its table")
			require.NotNil(t, target.model, "%s must supply a model for schema introspection", target.table)
			require.Contains(t, []uuidDBRole{uuidRolePrimary, uuidRoleLog}, target.role,
				"%s has unknown role %q", target.table, target.role)
		}

		knownResolvers := []uuidResolverKind{uuidResolverIntFK, uuidResolverNullableFK, uuidResolverTokenName}
		for _, target := range uuidFKRegistry() {
			label := target.table + "." + target.uuidColumn
			require.NotEmpty(t, target.table, "an FK entry must name its table")
			require.NotEmpty(t, target.uuidColumn, "%s must name its uuid column", target.table)
			require.NotEmpty(t, target.refTable, "%s must name its reference table", label)
			require.NotNil(t, target.model, "%s must supply a model for schema introspection", label)
			require.Contains(t, knownResolvers, target.resolver, "%s has unknown resolver %q", label, target.resolver)

			switch target.resolver {
			case uuidResolverIntFK, uuidResolverNullableFK:
				require.NotEmpty(t, target.fkColumn, "%s resolves an integer reference and must name its fk column", label)
			case uuidResolverTokenName:
				// The composite (user_id, token_name) resolver is the only entry without an
				// observed integer reference column.
				require.Empty(t, target.fkColumn, "%s uses the composite resolver and must not name an fk column", label)
				require.Equal(t, "logs", target.table)
				require.Equal(t, "token_uuid", target.uuidColumn)
			}
		}
	})

	t.Run("registry cannot drift from the schema", func(t *testing.T) {
		db, _ := newUnifiedTestTopology(t)

		for _, target := range uuidOwnedRegistry() {
			require.True(t, db.Migrator().HasTable(target.model),
				"owned registry table %s must exist in the migrated schema", target.table)
			require.True(t, db.Migrator().HasColumn(target.model, "uuid"),
				"owned registry table %s must declare a uuid column", target.table)
		}

		for _, target := range uuidFKRegistry() {
			require.True(t, db.Migrator().HasTable(target.model),
				"FK registry table %s must exist in the migrated schema", target.table)
			require.True(t, db.Migrator().HasColumn(target.model, target.uuidColumn),
				"FK registry table %s must declare column %s", target.table, target.uuidColumn)
			if target.fkColumn != "" {
				require.True(t, db.Migrator().HasColumn(target.model, target.fkColumn),
					"FK registry table %s must declare reference column %s", target.table, target.fkColumn)
			}
		}
	})
}

// seedUUIDRegistryLegacyFixture inserts the reference owners plus, for every registry table, one
// legacy row whose UUID columns are NULL and one whose UUID columns are the empty string.
// Rows are inserted with raw SQL so the models' BeforeCreate hooks never auto-generate a UUID:
// the fixture must reproduce a genuinely pre-migration database. Owner rows carry explicit
// canonical UUIDs so every seeded FK UUID has a known expected value.
// Parameters:
//   - t: test handle used for assertions.
//   - db: primary handle of a unified topology with the full schema migrated.
//
// Return values: none.
func seedUUIDRegistryLegacyFixture(t *testing.T, db *gorm.DB) {
	t.Helper()

	// Reference owners. inviter_id stays 0 on the users owner so it is never an FK candidate.
	require.NoError(t, db.Exec(
		"INSERT INTO users (id, uuid, username, password, inviter_id) VALUES (?, ?, 'uuid-registry-owner', 'password-hash', 0)",
		uuidRegistryOwnerID, uuidRegistryOwnerUserUUID).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO channels (id, uuid, type, name, models, config) VALUES (?, ?, 1, 'uuid-registry-owner-channel', 'gpt-4o', '{}')",
		uuidRegistryOwnerID, uuidRegistryOwnerChannelUUID).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO tokens (id, uuid, user_id, user_uuid, `key`, name) VALUES (?, ?, ?, ?, 'uuid-registry-owner-key', ?)",
		uuidRegistryOwnerID, uuidRegistryOwnerTokenUUID, uuidRegistryOwnerID, uuidRegistryOwnerUserUUID,
		uuidRegistryOwnerTokenName).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO mcp_servers (id, uuid, name, base_url) VALUES (?, ?, 'uuid-registry-owner-server', 'https://example.invalid/mcp')",
		uuidRegistryOwnerID, uuidRegistryOwnerServerUUID).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO logs (id, uuid, user_id, user_uuid, channel_id, channel_uuid, token_name, token_uuid, type, content) "+
			"VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, 'uuid registry owner log')",
		uuidRegistryOwnerID, uuidRegistryOwnerLogUUID, uuidRegistryOwnerID, uuidRegistryOwnerUserUUID,
		uuidRegistryOwnerID, uuidRegistryOwnerChannelUUID, uuidRegistryOwnerTokenName, uuidRegistryOwnerTokenUUID).Error)

	// users: owned uuid plus inviter_uuid.
	require.NoError(t, db.Exec(
		"INSERT INTO users (id, uuid, username, password, inviter_id, inviter_uuid) VALUES "+
			"(?, NULL, 'uuid-registry-null-user', 'password-hash', ?, NULL), "+
			"(?, '', 'uuid-registry-empty-user', 'password-hash', ?, '')",
		uuidRegistryNullRowID, uuidRegistryOwnerID, uuidRegistryEmptyRowID, uuidRegistryOwnerID).Error)

	// tokens: owned uuid plus user_uuid. `key` is reserved, so it stays quoted.
	require.NoError(t, db.Exec(
		"INSERT INTO tokens (id, uuid, user_id, user_uuid, `key`, name) VALUES "+
			"(?, NULL, ?, NULL, 'uuid-registry-null-token-key', 'uuid-registry-null-token'), "+
			"(?, '', ?, '', 'uuid-registry-empty-token-key', 'uuid-registry-empty-token')",
		uuidRegistryNullRowID, uuidRegistryOwnerID, uuidRegistryEmptyRowID, uuidRegistryOwnerID).Error)

	// channels: owned uuid only; the registry declares no FK UUID column on channels.
	require.NoError(t, db.Exec(
		"INSERT INTO channels (id, uuid, type, name, models, config) VALUES "+
			"(?, NULL, 1, 'uuid-registry-null-channel', 'gpt-4o', '{}'), "+
			"(?, '', 1, 'uuid-registry-empty-channel', 'gpt-4o', '{}')",
		uuidRegistryNullRowID, uuidRegistryEmptyRowID).Error)

	// redemptions: owned uuid plus user_uuid. `key` is reserved, so it stays quoted.
	require.NoError(t, db.Exec(
		"INSERT INTO redemptions (id, uuid, user_id, user_uuid, `key`, name) VALUES "+
			"(?, NULL, ?, NULL, 'uuid-registry-null-redemption', 'gift'), "+
			"(?, '', ?, '', 'uuid-registry-empty-redemption', 'gift')",
		uuidRegistryNullRowID, uuidRegistryOwnerID, uuidRegistryEmptyRowID, uuidRegistryOwnerID).Error)

	// logs: owned uuid plus user_uuid, channel_uuid, and the composite-resolved token_uuid.
	require.NoError(t, db.Exec(
		"INSERT INTO logs (id, uuid, user_id, user_uuid, channel_id, channel_uuid, token_name, token_uuid, type, content) VALUES "+
			"(?, NULL, ?, NULL, ?, NULL, ?, NULL, 1, 'uuid registry null log'), "+
			"(?, '', ?, '', ?, '', ?, '', 1, 'uuid registry empty log')",
		uuidRegistryNullRowID, uuidRegistryOwnerID, uuidRegistryOwnerID, uuidRegistryOwnerTokenName,
		uuidRegistryEmptyRowID, uuidRegistryOwnerID, uuidRegistryOwnerID, uuidRegistryOwnerTokenName).Error)

	// token_transactions: owned uuid plus token_uuid, user_uuid, and the nullable log_uuid.
	require.NoError(t, db.Exec(
		"INSERT INTO token_transactions (id, uuid, transaction_id, token_id, token_uuid, user_id, user_uuid, log_id, log_uuid, status, pre_quota) VALUES "+
			"(?, NULL, 'uuid-registry-null-txn', ?, NULL, ?, NULL, ?, NULL, 1, 10), "+
			"(?, '', 'uuid-registry-empty-txn', ?, '', ?, '', ?, '', 1, 10)",
		uuidRegistryNullRowID, uuidRegistryOwnerID, uuidRegistryOwnerID, uuidRegistryOwnerID,
		uuidRegistryEmptyRowID, uuidRegistryOwnerID, uuidRegistryOwnerID, uuidRegistryOwnerID).Error)

	// user_request_costs: owned uuid plus user_uuid. request_id is unique and capped at 32 chars.
	require.NoError(t, db.Exec(
		"INSERT INTO user_request_costs (id, uuid, created_time, user_id, user_uuid, request_id, quota) VALUES "+
			"(?, NULL, 0, ?, NULL, 'uuid-registry-null-cost', 0), "+
			"(?, '', 0, ?, '', 'uuid-registry-empty-cost', 0)",
		uuidRegistryNullRowID, uuidRegistryOwnerID, uuidRegistryEmptyRowID, uuidRegistryOwnerID).Error)

	// traces: owned uuid only; the registry declares no FK UUID column on traces.
	require.NoError(t, db.Exec(
		"INSERT INTO traces (id, uuid, trace_id, url, method) VALUES "+
			"(?, NULL, 'uuid-registry-null-trace', 'https://example.invalid/v1/chat', 'POST'), "+
			"(?, '', 'uuid-registry-empty-trace', 'https://example.invalid/v1/chat', 'POST')",
		uuidRegistryNullRowID, uuidRegistryEmptyRowID).Error)

	// async_task_bindings: owned uuid plus user_uuid, token_uuid, and channel_uuid.
	require.NoError(t, db.Exec(
		"INSERT INTO async_task_bindings (id, uuid, task_id, task_type, user_id, user_uuid, token_id, token_uuid, channel_id, channel_uuid, channel_type) VALUES "+
			"(?, NULL, 'uuid-registry-null-task', 'video', ?, NULL, ?, NULL, ?, NULL, 1), "+
			"(?, '', 'uuid-registry-empty-task', 'video', ?, '', ?, '', ?, '', 1)",
		uuidRegistryNullRowID, uuidRegistryOwnerID, uuidRegistryOwnerID, uuidRegistryOwnerID,
		uuidRegistryEmptyRowID, uuidRegistryOwnerID, uuidRegistryOwnerID, uuidRegistryOwnerID).Error)

	// mcp_servers: owned uuid only; the registry declares no FK UUID column on mcp_servers.
	require.NoError(t, db.Exec(
		"INSERT INTO mcp_servers (id, uuid, name, base_url) VALUES "+
			"(?, NULL, 'uuid-registry-null-server', 'https://example.invalid/mcp'), "+
			"(?, '', 'uuid-registry-empty-server', 'https://example.invalid/mcp')",
		uuidRegistryNullRowID, uuidRegistryEmptyRowID).Error)

	// mcp_tools: owned uuid plus server_uuid.
	require.NoError(t, db.Exec(
		"INSERT INTO mcp_tools (id, uuid, server_id, server_uuid, name) VALUES "+
			"(?, NULL, ?, NULL, 'uuid-registry-null-tool'), "+
			"(?, '', ?, '', 'uuid-registry-empty-tool')",
		uuidRegistryNullRowID, uuidRegistryOwnerID, uuidRegistryEmptyRowID, uuidRegistryOwnerID).Error)

	// passkey_credentials: owned uuid plus user_uuid. credential_id and public_key are
	// NOT NULL blobs and credential_id is unique, so each row needs distinct bytes.
	require.NoError(t, db.Exec(
		"INSERT INTO passkey_credentials (id, uuid, user_id, user_uuid, credential_name, credential_id, public_key) VALUES "+
			"(?, NULL, ?, NULL, 'uuid registry null passkey', ?, ?), "+
			"(?, '', ?, '', 'uuid registry empty passkey', ?, ?)",
		uuidRegistryNullRowID, uuidRegistryOwnerID,
		[]byte("uuid-registry-null-credential-id"), []byte("uuid-registry-null-public-key"),
		uuidRegistryEmptyRowID, uuidRegistryOwnerID,
		[]byte("uuid-registry-empty-credential-id"), []byte("uuid-registry-empty-public-key")).Error)
}

// uuidRegistryExpectedFKOwners maps every registry FK column to the owner UUID it must resolve to.
// It is keyed by "table.column" and is asserted to cover the whole FK registry, so a new FK
// column cannot be added to the registry without being seeded and checked here.
// Parameters: none.
//
// Return values:
//   - map[string]string: expected owner UUID for each seeded FK column.
func uuidRegistryExpectedFKOwners() map[string]string {
	return map[string]string{
		"users.inviter_uuid":               uuidRegistryOwnerUserUUID,
		"tokens.user_uuid":                 uuidRegistryOwnerUserUUID,
		"redemptions.user_uuid":            uuidRegistryOwnerUserUUID,
		"token_transactions.token_uuid":    uuidRegistryOwnerTokenUUID,
		"token_transactions.user_uuid":     uuidRegistryOwnerUserUUID,
		"token_transactions.log_uuid":      uuidRegistryOwnerLogUUID,
		"user_request_costs.user_uuid":     uuidRegistryOwnerUserUUID,
		"async_task_bindings.user_uuid":    uuidRegistryOwnerUserUUID,
		"async_task_bindings.token_uuid":   uuidRegistryOwnerTokenUUID,
		"async_task_bindings.channel_uuid": uuidRegistryOwnerChannelUUID,
		"mcp_tools.server_uuid":            uuidRegistryOwnerServerUUID,
		"passkey_credentials.user_uuid":    uuidRegistryOwnerUserUUID,
		"logs.user_uuid":                   uuidRegistryOwnerUserUUID,
		"logs.channel_uuid":                uuidRegistryOwnerChannelUUID,
		"logs.token_uuid":                  uuidRegistryOwnerTokenUUID,
	}
}

// readUUIDRegistryColumn reads one nullable string column for one row id.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle owning the table.
//   - table: trusted table name.
//   - column: trusted string column name.
//   - id: row id to read.
//
// Return values:
//   - *string: column value, nil when the column is SQL NULL.
func readUUIDRegistryColumn(t *testing.T, db *gorm.DB, table string, column string, id int) *string {
	t.Helper()
	rows := []struct {
		Value *string `gorm:"column:uuid_value"`
	}{}
	require.NoError(t, db.Table(table).
		Select(quoteIdentifier(db, column)+" AS "+quoteIdentifier(db, "uuid_value")).
		Where(quoteIdentifier(db, "id")+" = ?", id).
		Find(&rows).Error)
	require.Len(t, rows, 1, "%s row %d must exist", table, id)
	return rows[0].Value
}

// requireUUIDRegistryColumnFilled asserts a column holds a canonical UUID for every supplied id.
// Parameters:
//   - t: test handle used for assertions.
//   - db: handle owning the table.
//   - table: trusted table name.
//   - column: trusted UUID column name.
//   - ids: row ids to inspect, in order.
//
// Return values:
//   - []string: the populated values, in the order of ids.
func requireUUIDRegistryColumnFilled(t *testing.T, db *gorm.DB, table string, column string, ids []int) []string {
	t.Helper()
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		value := readUUIDRegistryColumn(t, db, table, column, id)
		require.NotNil(t, value, "%s.%s must be populated for row %d", table, column, id)
		require.NotEmpty(t, *value, "%s.%s must not stay empty for row %d", table, column, id)
		requireHyphenatedUUID(t, *value)
		values = append(values, *value)
	}
	return values
}

// TestRegistryLegacyRowsFillFromNullAndEmpty covers UUID-A21: all 12 owned UUID columns and all 15
// denormalized FK UUID columns are backfilled for both NULL and empty-string legacy values.
func TestRegistryLegacyRowsFillFromNullAndEmpty(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, true)
	seedUUIDRegistryLegacyFixture(t, db)

	// Both legacy passes must be genuinely missing before the coordinator runs, otherwise the
	// test would prove nothing about the NULL and empty-string predicates.
	for _, target := range uuidOwnedRegistry() {
		require.Nil(t, readUUIDRegistryColumn(t, db, target.table, "uuid", uuidRegistryNullRowID),
			"%s.uuid must start as NULL", target.table)
		empty := readUUIDRegistryColumn(t, db, target.table, "uuid", uuidRegistryEmptyRowID)
		require.NotNil(t, empty)
		require.Empty(t, *empty, "%s.uuid must start as an empty string", target.table)
	}

	_, err := runFinalizer(t, topology)
	require.NoError(t, err)

	t.Run("owned uuid columns", func(t *testing.T) {
		for _, target := range uuidOwnedRegistry() {
			t.Run(target.table, func(t *testing.T) {
				values := requireUUIDRegistryColumnFilled(t, db, target.table, "uuid", uuidRegistryLegacyIDs)
				require.NotEqual(t, values[0], values[1],
					"the NULL and empty-string rows of %s must receive distinct owned uuids", target.table)
			})
		}
	})

	t.Run("denormalized fk uuid columns", func(t *testing.T) {
		owners := uuidRegistryExpectedFKOwners()
		require.Len(t, owners, len(uuidFKRegistry()),
			"the fixture expectation table must cover every registry FK column")

		for _, target := range uuidFKRegistry() {
			label := target.table + "." + target.uuidColumn
			t.Run(label, func(t *testing.T) {
				owner, ok := owners[label]
				require.True(t, ok, "registry FK column %s is not seeded by the fixture", label)
				values := requireUUIDRegistryColumnFilled(t, db, target.table, target.uuidColumn, uuidRegistryLegacyIDs)
				require.Equal(t, owner, values[0], "the NULL pass of %s must resolve to its live owner", label)
				require.Equal(t, owner, values[1], "the empty-string pass of %s must resolve to its live owner", label)
			})
		}
	})

	// A completed finalizer is the strongest available statement that nothing was left behind:
	// a single unfilled owned uuid or fillable fk gap would have blocked the marker.
	requireMarker(t, db, externalUUIDPrimaryMigrationKey, true)
	require.NoError(t, validateExternalUUIDs(context.Background(), topology))
}

// TestUUIDIndexValidationIsRegistryDriven covers UUID-A46 and UUID-046: catch-up, validation, and
// index target lists all derive from the shared registry, so index validation cannot miss a target.
func TestUUIDIndexValidationIsRegistryDriven(t *testing.T) {
	db, topology := newUnifiedTestTopology(t)
	withFinalizerEnabled(t, true)
	seedUUIDRegistryLegacyFixture(t, db)

	_, err := runFinalizer(t, topology)
	require.NoError(t, err)

	// Every registry-derived index exists on a fully finalized topology: the owned unique
	// indexes, the denormalized FK indexes, and the token-name lookup index.
	issues, err := validateUUIDIndexes(context.Background(), topology)
	require.NoError(t, err)
	require.Empty(t, issues, "a finalized topology must report no index issues")

	// Dropping exactly one registry-derived index must be reported, which is only possible if
	// the validation target list is the registry itself rather than a hand-maintained copy.
	const droppedTable = "tokens"
	require.NoError(t, db.Exec("DROP INDEX "+
		dropIndexSuffix(db, droppedTable, uuidUniqueIndexName(droppedTable))).Error)

	issues, err = validateUUIDIndexes(context.Background(), topology)
	require.NoError(t, err)
	require.Len(t, issues, 1, "exactly the dropped target must be reported")
	require.Equal(t, droppedTable, issues[0].table)
	require.Equal(t, "uuid", issues[0].column)
	require.Contains(t, issues[0].kind, "owned uuid unique index")

	// The same finding blocks the topology-wide validation, so it can never be silently ignored.
	require.ErrorContains(t, validateExternalUUIDs(context.Background(), topology),
		droppedTable+".uuid: missing or invalid owned uuid unique index")
}
