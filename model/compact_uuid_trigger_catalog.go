package model

// This file reads each dialect's catalog to verify an installed synchronization trigger set
// (AUTO-004). It is the "a matching name alone is insufficient" half of the trigger contract:
// timing, event, table, canonical body hash, security/definer properties, and enabled state
// are all compared against the compile-time expectation.
//
// Every mismatch reason is bounded and value-free. A reason names the object and the property
// that disagreed, never a row value, a UUID, or a body.

import (
	"context"
	"strings"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

// =============================================================================
// POSTGRESQL
// =============================================================================

// verifyPostgresCompactTriggers checks the function body and the trigger's catalog properties.
//
// The properties are read rather than assumed because each one can be changed independently of
// the name: tgenabled can be set to disabled by a restore or by ALTER TABLE ... DISABLE
// TRIGGER, and prosecdef flipping to SECURITY DEFINER would change whose privileges the
// derivation runs under.
// Parameters:
//   - ctx: context bounding the catalog reads.
//   - db: PostgreSQL handle owning the table.
//   - table: registry table to verify.
//
// Return values:
//   - compactTriggerState: observed health with a bounded reason on mismatch.
//   - error: wrapped error when the catalog cannot be read.
func verifyPostgresCompactTriggers(ctx context.Context, db *gorm.DB, table compactTable) (compactTriggerState, error) {
	expected, err := expectedCompactTriggerHash(db, table, compactObjectPostgresFunction)
	if err != nil {
		return compactTriggerState{}, err
	}

	rows := []struct {
		Body      string `gorm:"column:prosrc"`
		SecDef    bool   `gorm:"column:prosecdef"`
		Enabled   string `gorm:"column:tgenabled"`
		TableName string `gorm:"column:relname"`
		Type      int16  `gorm:"column:tgtype"`
		Internal  bool   `gorm:"column:tgisinternal"`
	}{}
	sql := "SELECT p.prosrc, p.prosecdef, t.tgenabled, c.relname, t.tgtype, t.tgisinternal" +
		" FROM pg_trigger t" +
		" JOIN pg_class c ON c.oid = t.tgrelid" +
		" JOIN pg_namespace n ON n.oid = c.relnamespace" +
		" JOIN pg_proc p ON p.oid = t.tgfoid" +
		" WHERE t.tgname = ? AND n.nspname = CURRENT_SCHEMA()"
	if err := db.WithContext(ctx).Raw(sql, compactSyncTriggerName(table.table)).Scan(&rows).Error; err != nil {
		return compactTriggerState{}, errors.Wrapf(err, "read postgres trigger metadata for %s", table.table)
	}
	if len(rows) == 0 {
		return compactTriggerState{reason: "trigger is absent"}, nil
	}

	row := rows[0]
	switch {
	case !equalFoldASCII(row.TableName, table.table):
		return compactTriggerState{reason: "trigger is attached to an unexpected table"}, nil
	case row.SecDef:
		return compactTriggerState{reason: "trigger function is SECURITY DEFINER, not SECURITY INVOKER"}, nil
	case row.Enabled != "O" && row.Enabled != "A":
		// 'O' is origin-and-local (the default) and 'A' is always. 'D' is disabled, which
		// would silently stop derivation while leaving the object in place.
		return compactTriggerState{reason: "trigger is disabled"}, nil
	case row.Internal:
		return compactTriggerState{reason: "trigger is an internal constraint trigger"}, nil
	case !postgresTriggerTimingMatches(row.Type):
		return compactTriggerState{reason: "trigger timing or event does not match BEFORE INSERT OR UPDATE FOR EACH ROW"}, nil
	case triggerBodyHash(row.Body) != expected:
		return compactTriggerState{reason: "trigger function body does not match the expected version"}, nil
	}
	return compactTriggerState{installed: true}, nil
}

// PostgreSQL pg_trigger.tgtype bit flags, from the server's catalog definition.
const (
	// pgTriggerRow marks a FOR EACH ROW trigger; without it the trigger is statement-level.
	pgTriggerRow int16 = 1 << 0
	// pgTriggerBefore marks BEFORE timing; without it the trigger is AFTER or INSTEAD OF.
	pgTriggerBefore int16 = 1 << 1
	// pgTriggerInsert marks the INSERT event.
	pgTriggerInsert int16 = 1 << 2
	// pgTriggerUpdate marks the UPDATE event.
	pgTriggerUpdate int16 = 1 << 4
)

// postgresTriggerTimingMatches reports whether tgtype encodes BEFORE INSERT OR UPDATE FOR EACH ROW.
//
// Timing is checked, not assumed, because an AFTER trigger cannot assign to NEW: the object
// would exist under the right name and derive nothing at all.
// Parameters:
//   - tgtype: pg_trigger.tgtype bit mask.
//
// Return values:
//   - bool: true when the timing and events match the contract exactly.
func postgresTriggerTimingMatches(tgtype int16) bool {
	required := pgTriggerRow | pgTriggerBefore | pgTriggerInsert | pgTriggerUpdate
	return tgtype&required == required
}

// =============================================================================
// MYSQL
// =============================================================================

// verifyMySQLCompactTriggers checks both triggers' timing, event, body, sql_mode, and definer.
// Parameters:
//   - ctx: context bounding the catalog reads.
//   - db: MySQL handle owning the table.
//   - table: registry table to verify.
//
// Return values:
//   - compactTriggerState: observed health with a bounded reason on mismatch.
//   - error: wrapped error when the catalog cannot be read.
func verifyMySQLCompactTriggers(ctx context.Context, db *gorm.DB, table compactTable) (compactTriggerState, error) {
	for _, spec := range []struct {
		name   string
		event  string
		object compactTriggerObject
	}{
		{name: compactInsertTriggerName(table.table), event: "INSERT", object: compactObjectInsertTrigger},
		{name: compactUpdateTriggerName(table.table), event: "UPDATE", object: compactObjectUpdateTrigger},
	} {
		state, err := verifyOneMySQLCompactTrigger(ctx, db, table, spec.name, spec.event, spec.object)
		if err != nil || !state.installed {
			return state, err
		}
	}
	return compactTriggerState{installed: true}, nil
}

// verifyOneMySQLCompactTrigger checks a single MySQL trigger against its contract.
//
// ACTION_ORDER is checked because MySQL allows several triggers on the same table and event:
// a foreign trigger ordered before this one could rewrite the legacy column after the
// derivation ran, leaving a shadow that disagrees with the text the row actually stores.
// Parameters:
//   - ctx: context bounding the catalog read.
//   - db: MySQL handle owning the table.
//   - table: registry table.
//   - name: expected trigger name.
//   - event: expected manipulation event.
//   - object: generated object whose body hash is expected.
//
// Return values:
//   - compactTriggerState: observed health with a bounded reason on mismatch.
//   - error: wrapped error when the catalog cannot be read.
func verifyOneMySQLCompactTrigger(ctx context.Context, db *gorm.DB, table compactTable,
	name string, event string, object compactTriggerObject) (compactTriggerState, error) {
	expected, err := expectedCompactTriggerHash(db, table, object)
	if err != nil {
		return compactTriggerState{}, err
	}

	rows := []struct {
		Statement string `gorm:"column:ACTION_STATEMENT"`
		Timing    string `gorm:"column:ACTION_TIMING"`
		Event     string `gorm:"column:EVENT_MANIPULATION"`
		Table     string `gorm:"column:EVENT_OBJECT_TABLE"`
		Order     int64  `gorm:"column:ACTION_ORDER"`
		Definer   string `gorm:"column:DEFINER"`
		SQLMode   string `gorm:"column:SQL_MODE"`
	}{}
	sql := "SELECT ACTION_STATEMENT, ACTION_TIMING, EVENT_MANIPULATION, EVENT_OBJECT_TABLE," +
		" ACTION_ORDER, DEFINER, SQL_MODE FROM information_schema.TRIGGERS" +
		" WHERE TRIGGER_SCHEMA = DATABASE() AND TRIGGER_NAME = ?"
	if err := db.WithContext(ctx).Raw(sql, name).Scan(&rows).Error; err != nil {
		return compactTriggerState{}, errors.Wrapf(err, "read mysql trigger metadata for %s", table.table)
	}
	if len(rows) == 0 {
		return compactTriggerState{reason: "trigger is absent"}, nil
	}

	row := rows[0]
	switch {
	case !equalFoldASCII(row.Table, table.table):
		return compactTriggerState{reason: "trigger is attached to an unexpected table"}, nil
	case !equalFoldASCII(row.Timing, "BEFORE"):
		return compactTriggerState{reason: "trigger timing is not BEFORE"}, nil
	case !equalFoldASCII(row.Event, event):
		return compactTriggerState{reason: "trigger event does not match its contract"}, nil
	case row.Order != 1:
		return compactTriggerState{reason: "trigger action order is not first for its table and event"}, nil
	case !mysqlDefinerAllowed(row.Definer):
		return compactTriggerState{reason: "trigger definer is not an approved migration definer"}, nil
	case !mysqlTriggerSQLModeAllowed(row.SQLMode):
		return compactTriggerState{reason: "trigger sql_mode contains a prohibited setting"}, nil
	case triggerBodyHash(row.Statement) != expected:
		return compactTriggerState{reason: "trigger body does not match the expected version"}, nil
	}
	return compactTriggerState{installed: true}, nil
}

// mysqlDefinerAllowed reports whether a durable trigger definer is approved.
//
// The definer is a policy property, not a body property. A restore under a different approved
// definer must change this verified result and not the canonical body hash, which is exactly
// why the two are compared separately: the derivation is identical, only the privilege context
// differs, and an unapproved definer means the trigger may fail or run with wrong privileges.
// Parameters:
//   - definer: DEFINER value reported by information_schema, in user@host form.
//
// Return values:
//   - bool: true when the definer is a non-empty, well-formed account.
func mysqlDefinerAllowed(definer string) bool {
	// The migration cannot enumerate every deployment's account names, so the durable policy
	// is structural: a definer must exist and be well formed. A restore-time definer rewrite
	// to another real account is an approved supported case; an empty or malformed definer
	// means the trigger cannot execute reliably and must be reinstalled.
	definer = strings.TrimSpace(definer)
	if definer == "" {
		return false
	}
	at := strings.LastIndex(definer, "@")
	return at > 0 && at < len(definer)-1
}

// mysqlTriggerSQLModeAllowed rejects a stored sql_mode that would change derivation semantics.
//
// A trigger records the sql_mode in force at creation and always executes under it. Two modes
// matter here: PAD_CHAR_TO_FULL_LENGTH changes how a CHAR(36) legacy column is read, which
// would make an otherwise valid UUID fail the length check, and ANSI_QUOTES reinterprets the
// double-quoted identifiers the generator emits.
// Parameters:
//   - mode: SQL_MODE value reported by information_schema.
//
// Return values:
//   - bool: true when no prohibited mode is present.
func mysqlTriggerSQLModeAllowed(mode string) bool {
	upper := strings.ToUpper(mode)
	for _, prohibited := range []string{"PAD_CHAR_TO_FULL_LENGTH", "ANSI_QUOTES"} {
		if strings.Contains(upper, prohibited) {
			return false
		}
	}
	return true
}

// =============================================================================
// SQLITE
// =============================================================================

// verifySQLiteCompactTriggers checks both persistent triggers against their contract.
//
// The lookup is restricted to the main schema. A trigger in temp would exist only for the
// connection that created it, so an old binary's writes would never fire it, and accepting one
// as evidence would leave every other connection deriving nothing.
// Parameters:
//   - ctx: context bounding the catalog reads.
//   - db: SQLite handle owning the table.
//   - table: registry table to verify.
//
// Return values:
//   - compactTriggerState: observed health with a bounded reason on mismatch.
//   - error: wrapped error when the catalog cannot be read.
func verifySQLiteCompactTriggers(ctx context.Context, db *gorm.DB, table compactTable) (compactTriggerState, error) {
	for _, spec := range []struct {
		name   string
		object compactTriggerObject
	}{
		{name: compactInsertTriggerName(table.table), object: compactObjectInsertTrigger},
		{name: compactUpdateTriggerName(table.table), object: compactObjectUpdateTrigger},
	} {
		expected, err := expectedCompactTriggerHash(db, table, spec.object)
		if err != nil {
			return compactTriggerState{}, err
		}

		rows := []struct {
			SQL       string `gorm:"column:sql"`
			TableName string `gorm:"column:tbl_name"`
		}{}
		query := "SELECT sql, tbl_name FROM main.sqlite_master WHERE type = 'trigger' AND name = ?"
		if err := db.WithContext(ctx).Raw(query, spec.name).Scan(&rows).Error; err != nil {
			return compactTriggerState{}, errors.Wrapf(err, "read sqlite trigger metadata for %s", table.table)
		}
		if len(rows) == 0 {
			return compactTriggerState{reason: "trigger is absent from the main schema"}, nil
		}
		if !equalFoldASCII(rows[0].TableName, table.table) {
			return compactTriggerState{reason: "trigger is attached to an unexpected table"}, nil
		}
		if triggerBodyHash(rows[0].SQL) != expected {
			return compactTriggerState{reason: "trigger body does not match the expected version"}, nil
		}
	}
	return compactTriggerState{installed: true}, nil
}

// compactSQLiteCapable reports whether this process's SQLite can host the persistent triggers.
//
// The probe is required by the proposal (AUTO-T27) and is not paranoia. unhex() landed in
// SQLite 3.41.0, and the persistent trigger deliberately uses only core SQL so that a pinned
// old binary's writes still derive shadows. That means the trigger runs inside whichever SQLite
// each supported binary links, so an older engine would turn every insert into an error rather
// than a derivation. A failed probe blocks compact work and leaves legacy service untouched.
// Parameters:
//   - ctx: context bounding the probe.
//   - db: SQLite handle to probe.
//
// Return values:
//   - bool: true when unhex is present and round-trips the golden vector.
//   - string: bounded, value-free reason when the probe fails.
//   - error: wrapped error when the probe cannot be executed at all.
func compactSQLiteCapable(ctx context.Context, db *gorm.DB) (bool, string, error) {
	var probe string
	err := db.WithContext(ctx).Raw("SELECT hex(unhex(?))", compactUUIDGoldenHex).Scan(&probe).Error
	if err != nil {
		// An engine without unhex reports "no such function"; that is a blocker, not a
		// database failure, so it is classified rather than returned.
		if strings.Contains(strings.ToLower(err.Error()), "no such function") {
			return false, "sqlite engine does not provide the core unhex function (requires 3.41.0 or newer)", nil
		}
		return false, "", errors.Wrap(err, "probe sqlite unhex capability")
	}
	if !equalFoldASCII(probe, compactUUIDGoldenHex) {
		return false, "sqlite unhex golden probe did not round-trip the fixed vector", nil
	}
	return true, "", nil
}
