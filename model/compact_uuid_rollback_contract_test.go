package model

import (
	"crypto/md5"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The rollback contract, stated once and enforced by every pinned-build suite.
//
// A supported rollback build runs its own GORM AutoMigrate against a schema the current build
// created. AutoMigrate creates every index its models declare and never drops one, so whenever
// the current build stops declaring an index — because a better one replaced it — a rollback
// build puts the old one back. That is not a compatibility failure; it is what the rollback build
// is. Refusing it would leave two ways to make the suites pass, both wrong: keep declaring the
// redundant index on every deployment forever, or stop testing rollbacks.
//
// So the contract is exact rather than byte-identical:
//
//   - nothing the current build created may be dropped, renamed, retyped, or rewritten — every
//     catalog line captured before the rollback build ran must be present, unchanged, after it;
//   - the only additions allowed are indexes the rollback build declares and the current build
//     deliberately superseded, listed below by name AND exact definition; and
//   - that list cannot go stale, because TestCompactRollbackSupersededIndexesAreReallySuperseded
//     proves on every run that each entry is absent from the current schema and that its
//     replacement is present and still leads with the same column.
//
// Compact and legacy UUID objects are therefore still required to be byte-identical: none of them
// can appear in this list.

// compactRollbackSupersededIndex is one index a supported rollback build still declares, which the
// current build replaced.
type compactRollbackSupersededIndex struct {
	// table owns the index.
	table string
	// name is what the rollback build's AutoMigrate creates.
	name string
	// column is the indexed column; the replacement must lead with it.
	column string
	// definition is PostgreSQL's exact pg_indexes.indexdef for what the rollback build creates.
	definition string
	// supersededBy is the current build's index that serves every lookup this one served.
	supersededBy string
	// commit is where the replacement happened.
	commit string
}

// compactRollbackSupersededIndexes lists every index a supported rollback build may re-create.
var compactRollbackSupersededIndexes = []compactRollbackSupersededIndex{
	{
		// Both pinned rollback builds declare `LastAccessedAt ... gorm:"index"`. 32df60cf replaced
		// it with the composite (last_accessed_at, created_at) the retention sweep plans against,
		// deliberately without dropping the old one from existing deployments. A rollback onto a
		// schema the current build created therefore ends where every upgraded deployment already
		// is: both indexes present.
		table:        "async_task_bindings",
		name:         "idx_async_task_bindings_last_accessed_at",
		column:       "last_accessed_at",
		definition:   "CREATE INDEX idx_async_task_bindings_last_accessed_at ON public.async_task_bindings USING btree (last_accessed_at)",
		supersededBy: "idx_async_task_retention",
		commit:       "32df60cf",
	},
}

// compactCatalogLineForIndex renders the fingerprint line compactCatalogFingerprint produces for
// one PostgreSQL index definition.
// Parameters:
//   - name: index name.
//   - definition: exact pg_indexes.indexdef.
//
// Return values:
//   - string: the fingerprint line.
func compactCatalogLineForIndex(name string, definition string) string {
	sum := md5.Sum([]byte(definition))
	return "IDX|" + name + "|" + hex.EncodeToString(sum[:])
}

// compactRollbackCatalogViolations lists everything a rollback build did to the catalog that the
// contract forbids.
//
// It is pure, so the contract itself is unit-tested without a database or a pinned build.
// Parameters:
//   - before: catalog fingerprint captured before the rollback build ran.
//   - after: catalog fingerprint captured after it ran.
//   - ownIndexPrefixes: `IDX|<name>` prefixes of indexes this particular artifact is known to
//     re-add under any definition; nil for none.
//
// Return values:
//   - []string: human-readable violations, empty when the contract held.
func compactRollbackCatalogViolations(before string, after string, ownIndexPrefixes map[string]struct{}) []string {
	superseded := map[string]struct{}{}
	for _, index := range compactRollbackSupersededIndexes {
		superseded[compactCatalogLineForIndex(index.name, index.definition)] = struct{}{}
	}

	violations := []string{}
	afterLines := map[string]struct{}{}
	for _, line := range strings.Split(after, "\n") {
		afterLines[line] = struct{}{}
	}
	for _, line := range strings.Split(before, "\n") {
		if _, ok := afterLines[line]; !ok {
			violations = append(violations, "dropped, renamed, retyped, or rewrote "+line)
		}
	}
	for _, line := range compactPrevAddedLines(before, after) {
		if _, ok := superseded[line]; ok {
			continue
		}
		prefix := line
		if cut := strings.LastIndex(line, "|"); cut > 0 {
			prefix = line[:cut]
		}
		if _, ok := ownIndexPrefixes[prefix]; ok {
			continue
		}
		violations = append(violations, "added "+line+", which is not an index this build declares and the current build superseded")
	}
	return violations
}

// requireRollbackCatalogContract asserts the rollback contract for one pinned-build run.
// Parameters:
//   - t: test handle used for assertions.
//   - artifact: description of the rollback build, for the failure message.
//   - before: catalog fingerprint captured before it ran.
//   - after: catalog fingerprint captured after it ran.
//   - ownIndexPrefixes: `IDX|<name>` prefixes this artifact is known to re-add; nil for none.
//
// Return values: none.
func requireRollbackCatalogContract(t *testing.T, artifact string, before string, after string,
	ownIndexPrefixes map[string]struct{}) {
	t.Helper()
	violations := compactRollbackCatalogViolations(before, after, ownIndexPrefixes)
	require.Empty(t, violations,
		"%s broke the rollback contract:\n%s", artifact, strings.Join(violations, "\n"))
}

// TestCompactRollbackCatalogContract unit-tests the contract itself, so a later edit cannot loosen
// it without a red test: it must accept exactly the listed superseded indexes and the artifact's own
// declared indexes, and reject everything else.
func TestCompactRollbackCatalogContract(t *testing.T) {
	require.NotEmpty(t, compactRollbackSupersededIndexes)
	superseded := compactRollbackSupersededIndexes[0]
	supersededLine := compactCatalogLineForIndex(superseded.name, superseded.definition)
	before := strings.Join([]string{
		"COL|users|uuid_compact|uuid|YES",
		"IDX|idx_users_uuid_unique|aaaa",
		"TRG|cuuid_v1_users_sync|users|5|O|false|bbbb",
	}, "\n")

	t.Run("an unchanged catalog holds the contract", func(t *testing.T) {
		require.Empty(t, compactRollbackCatalogViolations(before, before, nil))
	})
	t.Run("re-adding a listed superseded index is allowed", func(t *testing.T) {
		require.Empty(t, compactRollbackCatalogViolations(before, before+"\n"+supersededLine, nil))
	})
	t.Run("the artifact's own declared index is allowed only when named", func(t *testing.T) {
		after := before + "\nIDX|idx_users_uuid|cccc"
		require.NotEmpty(t, compactRollbackCatalogViolations(before, after, nil))
		require.Empty(t, compactRollbackCatalogViolations(before, after,
			map[string]struct{}{"IDX|idx_users_uuid": {}}))
	})
	t.Run("a superseded index under a different definition is rejected", func(t *testing.T) {
		wrong := compactCatalogLineForIndex(superseded.name, superseded.definition+" WHERE true")
		violations := compactRollbackCatalogViolations(before, before+"\n"+wrong, nil)
		require.Len(t, violations, 1, "only the exact definition the rollback build declares is allowed")
	})
	t.Run("an unlisted index is rejected", func(t *testing.T) {
		violations := compactRollbackCatalogViolations(before, before+"\nIDX|idx_logs_something_new|dddd", nil)
		require.Len(t, violations, 1)
	})
	t.Run("a compact or legacy object that changes is rejected", func(t *testing.T) {
		after := strings.Replace(before, "TRG|cuuid_v1_users_sync|users|5|O|false|bbbb",
			"TRG|cuuid_v1_users_sync|users|5|D|false|bbbb", 1)
		violations := compactRollbackCatalogViolations(before, after, nil)
		require.Len(t, violations, 2, "a rewrite is a removal of the old line and an unlisted addition")
	})
	t.Run("a dropped object is rejected", func(t *testing.T) {
		after := strings.Replace(before, "\nIDX|idx_users_uuid_unique|aaaa", "", 1)
		require.Len(t, compactRollbackCatalogViolations(before, after, nil), 1)
	})
}

// TestCompactRollbackSupersededIndexesAreReallySuperseded keeps the allowlist honest. An entry is
// only justified while the current build does not declare the index, and while its replacement
// exists and leads with the same column, so that every lookup the old index served is still
// served. If either stops being true, the entry has become a hole in the rollback contract, and
// this fails on every ordinary test run rather than only in the live pinned-build suites.
func TestCompactRollbackSupersededIndexesAreReallySuperseded(t *testing.T) {
	db, _ := newUnifiedTestTopology(t)
	migrator := db.Migrator()
	for _, index := range compactRollbackSupersededIndexes {
		// The table-name form, never the model form: see the GORM HasIndex(model) race.
		require.False(t, migrator.HasIndex(index.table, index.name),
			"%s is listed as superseded, but the current build still declares it; remove the entry", index.name)
		require.True(t, migrator.HasIndex(index.table, index.supersededBy),
			"%s is listed as superseded by %s, which the current build no longer declares", index.name, index.supersededBy)

		columns := []struct {
			Seqno int    `gorm:"column:seqno"`
			Name  string `gorm:"column:name"`
		}{}
		require.NoError(t, db.Raw("SELECT seqno, name FROM pragma_index_info(?)", index.supersededBy).Scan(&columns).Error)
		require.NotEmpty(t, columns)
		require.Equal(t, index.column, columns[0].Name,
			"%s must lead with %s to serve every lookup %s served", index.supersededBy, index.column, index.name)

		require.True(t, strings.HasPrefix(index.definition, "CREATE INDEX "+index.name+" ON public."+index.table+" "),
			"the recorded definition must be the plain index the rollback build declares on %s", index.table)
		require.True(t, strings.HasSuffix(index.definition, "("+index.column+")"),
			"the recorded definition must index exactly %s", index.column)
	}
}
