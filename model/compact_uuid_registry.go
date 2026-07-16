package model

// This registry is the compile-time source of truth for every compact UUID shadow column.
// It is derived from uuidOwnedRegistry and uuidFKRegistry rather than duplicated as an
// unrelated list (AUTO-001), so a UUID column added to the external-UUID registries cannot
// silently miss compact schema, trigger, backfill, validation, index, or lookup coverage.
//
// Compact shadows are additive, database-derived data. The legacy text column is permanently
// authoritative; nothing here may drop, rename, retype, or repurpose a legacy column or index.

import (
	"sort"

	"github.com/Laisky/errors/v2"
)

// compactColumnSuffix is appended to a legacy UUID column name to name its shadow.
// The proposal fixes the naming scheme and forbids ever changing it, so it is a constant
// rather than a configurable value.
const compactColumnSuffix = "_compact"

// compactObjectVersion versions every generated compact object: columns, triggers, indexes,
// the legacy-index manifest, and the completion markers. A change to any generated body must
// bump this so verification rejects a stale object instead of accepting it by name.
const compactObjectVersion = "v1"

// compactMigrationGeneration names this migration generation in markers and diagnostics.
const compactMigrationGeneration = "compact_uuid_storage_" + compactObjectVersion

// compactTargetKind distinguishes an owned UUID from a denormalized FK UUID.
// The kind selects the compact index's uniqueness and the derivation rule applied to a NULL
// or empty legacy value: a missing owned UUID blocks completion, while a NULL FK is a valid
// terminal state.
type compactTargetKind string

const (
	// compactKindOwned identifies the UUID that externally identifies the row itself. It is
	// non-nullable in practice: a NULL, empty, or malformed owned value blocks completion.
	compactKindOwned compactTargetKind = "owned"
	// compactKindFK identifies a denormalized UUID copied from a referenced row. NULL and
	// empty are valid terminal states that derive compact NULL.
	compactKindFK compactTargetKind = "fk"
)

// compactTarget describes one authoritative legacy UUID column and its derived shadow.
type compactTarget struct {
	// role is the database that authoritatively owns the table. In split mode a stale
	// primary logs table is never expanded, triggered, scanned, indexed, or marked, because
	// the log targets resolve exclusively through the log role.
	role uuidDBRole
	// table is the trusted table name, taken from the external UUID registry.
	table string
	// model is the GORM model used for schema and index introspection.
	model any
	// legacyColumn is the permanently authoritative text column.
	legacyColumn string
	// compactColumn is the derived shadow column name.
	compactColumn string
	// kind selects uniqueness and null semantics.
	kind compactTargetKind
}

// unique reports whether this target's compact index is unique.
// Owned UUIDs externally identify a row and are therefore unique; denormalized FK UUIDs
// repeat across rows and must not be.
// Parameters: none.
//
// Return values:
//   - bool: true when the compact index is unique.
func (target compactTarget) unique() bool {
	return target.kind == compactKindOwned
}

// nullable reports whether a NULL or empty legacy value is a valid terminal state.
// Parameters: none.
//
// Return values:
//   - bool: true when NULL/empty legacy text derives compact NULL without blocking completion.
func (target compactTarget) nullable() bool {
	return target.kind == compactKindFK
}

// indexName returns the deterministic compact index name for this target.
// The names are fixed by the proposal: owned targets use idx_<table>_uuid_compact_unique and
// FK targets use idx_<table>_<legacy_column>_compact.
// Parameters: none.
//
// Return values:
//   - string: compile-time index name built only from registry identifiers.
func (target compactTarget) indexName() string {
	if target.kind == compactKindOwned {
		return "idx_" + target.table + "_uuid_compact_unique"
	}
	return "idx_" + target.table + "_" + target.legacyColumn + "_compact"
}

// id returns the bounded metric/log label identifying this target.
// It is built from compile-time registry identifiers only, so it is safe as a metric label.
// Parameters: none.
//
// Return values:
//   - string: "<table>.<legacy_column>" identifier.
func (target compactTarget) id() string {
	return target.table + "." + target.legacyColumn
}

// compactColumnFor returns the shadow column name for a legacy UUID column.
// Parameters:
//   - legacyColumn: authoritative legacy column name.
//
// Return values:
//   - string: derived shadow column name.
func compactColumnFor(legacyColumn string) string {
	return legacyColumn + compactColumnSuffix
}

// compactRegistry returns every compact target, derived from the external UUID registries.
//
// The derivation is the point: owned targets come from uuidOwnedRegistry and FK targets from
// uuidFKRegistry, so the compact inventory cannot drift from the source of truth for external
// UUIDs. Targets are ordered by role, then table, then legacy column, which gives the
// coordinator, validation, and the golden DDL tests one stable order.
// Parameters: none.
//
// Return values:
//   - []compactTarget: all 27 compact targets in deterministic order.
func compactRegistry() []compactTarget {
	targets := make([]compactTarget, 0, len(uuidOwnedRegistry())+len(uuidFKRegistry()))

	for _, owned := range uuidOwnedRegistry() {
		targets = append(targets, compactTarget{
			role:          owned.role,
			table:         owned.table,
			model:         owned.model,
			legacyColumn:  "uuid",
			compactColumn: compactColumnFor("uuid"),
			kind:          compactKindOwned,
		})
	}

	for _, fk := range uuidFKRegistry() {
		targets = append(targets, compactTarget{
			// The target's own role, not refRole: the shadow lives beside the denormalized
			// column, and the worker derives it from that column's own text. It never
			// re-resolves the relationship across databases.
			role:          fk.role,
			table:         fk.table,
			model:         fk.model,
			legacyColumn:  fk.uuidColumn,
			compactColumn: compactColumnFor(fk.uuidColumn),
			kind:          compactKindFK,
		})
	}

	sort.SliceStable(targets, func(i, j int) bool {
		if targets[i].role != targets[j].role {
			return targets[i].role < targets[j].role
		}
		if targets[i].table != targets[j].table {
			return targets[i].table < targets[j].table
		}
		return targets[i].legacyColumn < targets[j].legacyColumn
	})
	return targets
}

// compactTargetsForRole returns the compact targets one database role authoritatively owns.
// In split mode this is what keeps a stale primary logs table unreachable: the log targets
// only ever resolve through uuidRoleLog.
// Parameters:
//   - role: database role to filter by.
//
// Return values:
//   - []compactTarget: targets owned by role, in registry order.
func compactTargetsForRole(role uuidDBRole) []compactTarget {
	targets := make([]compactTarget, 0, len(compactRegistry()))
	for _, target := range compactRegistry() {
		if target.role == role {
			targets = append(targets, target)
		}
	}
	return targets
}

// compactTable groups every compact target that shares one authoritative table.
// Expansion, trigger installation, and trigger verification are per-table operations: one
// trigger set derives all of a table's shadows from the final legacy text of the same row.
type compactTable struct {
	// role is the database that authoritatively owns the table.
	role uuidDBRole
	// table is the trusted table name.
	table string
	// model is the GORM model used for schema introspection.
	model any
	// targets are the table's compact targets in registry order.
	targets []compactTarget
}

// compactTablesForRole returns the authoritative tables one role owns, each with its targets.
// Parameters:
//   - role: database role to filter by.
//
// Return values:
//   - []compactTable: tables owned by role, in registry order, each with at least one target.
func compactTablesForRole(role uuidDBRole) []compactTable {
	tables := make([]compactTable, 0, len(uuidOwnedRegistry()))
	index := map[string]int{}
	for _, target := range compactTargetsForRole(role) {
		position, seen := index[target.table]
		if !seen {
			index[target.table] = len(tables)
			tables = append(tables, compactTable{
				role:    target.role,
				table:   target.table,
				model:   target.model,
				targets: []compactTarget{target},
			})
			continue
		}
		tables[position].targets = append(tables[position].targets, target)
	}
	return tables
}

// compactTablesForTopology returns every authoritative table the topology must expand.
// Parameters:
//   - topology: explicitly constructed database topology.
//
// Return values:
//   - []compactTable: authoritative tables across every marker-carrying role.
func compactTablesForTopology(topology *databaseTopology) []compactTable {
	tables := make([]compactTable, 0, len(uuidOwnedRegistry()))
	for _, role := range topology.targetRoles() {
		tables = append(tables, compactTablesForRole(role)...)
	}
	return tables
}

// compactTargetByID looks up a compact target by its bounded identifier.
// Parameters:
//   - id: "<table>.<legacy_column>" identifier produced by compactTarget.id.
//
// Return values:
//   - compactTarget: matched target.
//   - error: wrapped error when no registry target carries that identifier.
func compactTargetByID(id string) (compactTarget, error) {
	for _, target := range compactRegistry() {
		if target.id() == id {
			return target, nil
		}
	}
	return compactTarget{}, errors.Errorf("unknown compact uuid target %q", id)
}

// compactTargetIDs returns every registry target identifier in deterministic order.
// The bounded metric label set and the operational probe suite are both built from this, so
// a new registry column automatically gains coverage.
// Parameters: none.
//
// Return values:
//   - []string: all target identifiers.
func compactTargetIDs() []string {
	ids := make([]string, 0, len(compactRegistry()))
	for _, target := range compactRegistry() {
		ids = append(ids, target.id())
	}
	return ids
}
