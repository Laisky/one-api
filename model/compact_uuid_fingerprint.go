package model

// This file computes the global validation fingerprints (AUTO-006, proposal section 8.5).
//
// Two streams are computed in the same row scan, and the distinction between them is the whole
// point:
//
//   - The EQUALITY stream encodes the derived semantic value. A nullable FK's SQL NULL and its
//     empty text both encode as derived NULL, and valid text and a decoded compact value both
//     encode as the same canonical UUID. Its two digests must match.
//   - The RAW-SOURCE stream tags SQL NULL, empty text, and non-empty text distinctly. It
//     exists to prove the legacy states were actually observed without conflation — without
//     it, an implementation that silently coerced NULL to empty would still produce matching
//     equality digests and look correct.
//
// Neither stream ever materializes a table, and no value or digest byte is ever logged.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"hash"
	"strconv"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

// compactFingerprint is the evidence produced by one fingerprint pass over a database role.
type compactFingerprint struct {
	// LegacyDigest is the equality digest computed from the authoritative text column.
	LegacyDigest string
	// CompactDigest is the equality digest computed from the derived shadow column.
	CompactDigest string
	// RawSourceDigest tags NULL, empty, and non-empty legacy states distinctly.
	RawSourceDigest string
	// Rows counts rows contributing to the streams.
	Rows int
	// NullSources counts observed SQL NULL legacy values.
	NullSources int
	// EmptySources counts observed empty-text legacy values.
	EmptySources int
	// PopulatedSources counts observed non-empty legacy values.
	PopulatedSources int
}

// matches reports whether the two equality digests agree.
// Parameters: none.
//
// Return values:
//   - bool: true when the legacy and compact equality streams are identical.
func (fingerprint compactFingerprint) matches() bool {
	return fingerprint.LegacyDigest == fingerprint.CompactDigest
}

// computeCompactFingerprints runs the bounded fingerprint pass for one database role.
//
// The pass runs inside a repeatable-read snapshot per role and selects the legacy and compact
// values in the same row scan. Both details matter: reading them in separate scans would let a
// concurrent trigger-atomic write land between the two and produce a spurious mismatch, and an
// unsnapshotted scan would compare rows from different points in time.
// Parameters:
//   - ctx: context bounding the pass.
//   - topology: explicitly constructed database topology.
//   - role: database role to fingerprint.
//
// Return values:
//   - compactFingerprint: digests and exact source-state counts.
//   - error: wrapped error when the snapshot or a query fails.
func computeCompactFingerprints(ctx context.Context, topology *databaseTopology,
	role uuidDBRole) (compactFingerprint, error) {
	fingerprint := compactFingerprint{}
	db := topology.handle(role)
	if db == nil {
		return fingerprint, errors.Errorf("%s database handle is nil", role)
	}

	err := withCompactSnapshot(ctx, db, func(tx *gorm.DB) error {
		legacy := sha256.New()
		compact := sha256.New()
		raw := sha256.New()

		for _, target := range compactTargetsForRole(role) {
			counts, err := streamCompactFingerprint(ctx, tx, target, legacy, compact, raw)
			if err != nil {
				return err
			}
			fingerprint.Rows += counts.rows
			fingerprint.NullSources += counts.nullSources
			fingerprint.EmptySources += counts.emptySources
			fingerprint.PopulatedSources += counts.populatedSources
		}

		fingerprint.LegacyDigest = hex.EncodeToString(legacy.Sum(nil))
		fingerprint.CompactDigest = hex.EncodeToString(compact.Sum(nil))
		fingerprint.RawSourceDigest = hex.EncodeToString(raw.Sum(nil))
		return nil
	})
	if err != nil {
		return compactFingerprint{}, err
	}
	return fingerprint, nil
}

// withCompactSnapshot runs fn inside a bounded repeatable-read transaction.
//
// SQLite's default deferred transaction already gives a stable read view for the duration of
// the transaction, so only the server engines need an explicit isolation level.
// Parameters:
//   - ctx: context bounding the transaction.
//   - db: database handle to snapshot.
//   - fn: work to run inside the snapshot.
//
// Return values:
//   - error: wrapped error when the snapshot cannot be started or fn fails.
func withCompactSnapshot(ctx context.Context, db *gorm.DB, fn func(tx *gorm.DB) error) error {
	validationCtx, cancel := context.WithTimeout(ctx, compactValidationTimeout())
	defer cancel()

	session := db.WithContext(validationCtx)
	switch dialectName(db) {
	case "postgres", "mysql":
		// The isolation level is requested at BEGIN through the driver rather than with a
		// SET inside the callback. MySQL rejects the latter outright — "Transaction
		// characteristics can't be changed while a transaction is in progress" — because
		// SET TRANSACTION configures the NEXT transaction, and GORM has already issued BEGIN
		// by the time the callback runs.
		return session.Transaction(fn, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	default:
		// SQLite's driver rejects a non-default isolation level, and it does not need one:
		// its deferred transaction already gives a stable read view for the transaction's
		// lifetime, which is exactly the snapshot this pass requires.
		return session.Transaction(fn)
	}
}

// compactSourceCounts are the exact raw legacy source-state counts for one target.
type compactSourceCounts struct {
	// rows counts rows contributing to the streams.
	rows int
	// nullSources counts observed SQL NULL legacy values.
	nullSources int
	// emptySources counts observed empty-text legacy values.
	emptySources int
	// populatedSources counts observed non-empty legacy values.
	populatedSources int
}

// streamCompactFingerprint folds one target's rows into the three running digests.
//
// Rows are streamed and folded one at a time; the pass never materializes a table. Each stream
// shares the `table | id | logical-column` prefix, so a value that moved between rows or
// columns cannot cancel out against another.
// Parameters:
//   - ctx: context bounding the query.
//   - tx: snapshot transaction to read within.
//   - target: registry target to stream.
//   - legacy: running equality digest fed from authoritative text.
//   - compact: running equality digest fed from the derived shadow.
//   - raw: running digest tagging distinct legacy source states.
//
// Return values:
//   - compactSourceCounts: exact counts for this target.
//   - error: wrapped error when the query or a scan fails.
func streamCompactFingerprint(ctx context.Context, tx *gorm.DB, target compactTarget,
	legacy hash.Hash, compact hash.Hash, raw hash.Hash) (compactSourceCounts, error) {
	counts := compactSourceCounts{}

	sql := "SELECT " + quoteIdentifier(tx, "id") + " AS id, " +
		quoteIdentifier(tx, target.legacyColumn) + " AS legacy_value, " +
		quoteIdentifier(tx, target.compactColumn) + " AS compact_value" +
		" FROM " + quoteIdentifier(tx, target.table) +
		" ORDER BY " + quoteIdentifier(tx, "id") + " ASC"

	rows, err := tx.WithContext(ctx).Raw(sql).Rows()
	if err != nil {
		return counts, errors.Wrapf(err, "stream fingerprint for %s", target.id())
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return counts, errors.Wrapf(err, "stream fingerprint for %s", target.id())
		}
		candidate := compactCandidate{}
		if err := rows.Scan(&candidate.id, &candidate.legacy, &candidate.compact); err != nil {
			return counts, errors.Wrapf(err, "scan fingerprint row for %s", target.id())
		}
		counts.rows++

		prefix := target.table + "|" + strconv.FormatInt(candidate.id, 10) + "|" + target.legacyColumn + "|"

		// Equality stream: both sides encode the derived semantic value, so a nullable FK's
		// NULL and empty text compare equal to a NULL shadow, and valid text compares equal
		// to its decoded bytes.
		derived, _ := deriveCompactFromLegacy(target, candidate.legacy)
		writeDigest(legacy, prefix+encodeDerived(derived))
		writeDigest(compact, prefix+encodeDerived(candidate.compact))

		// Raw-source stream: NULL, empty, and non-empty are tagged distinctly so the
		// equality stream cannot hide a conflation of the two legacy null states.
		switch {
		case !candidate.legacy.valid:
			counts.nullSources++
			writeDigest(raw, prefix+"null")
		case isBlankLegacyValue(candidate.legacy.value):
			// Blank rather than empty, so the raw-source stream reports the same source state
			// on every dialect. PostgreSQL stores an empty CHAR(36) as 36 spaces and bpchar
			// equality ignores trailing spaces, so the two ARE one state there; classifying
			// the padded form as "text" would make the same data fingerprint differently on
			// PostgreSQL than on MySQL or SQLite.
			counts.emptySources++
			writeDigest(raw, prefix+"empty")
		default:
			counts.populatedSources++
			// The raw stream records the length and the derived canonical form rather than
			// the source text, which keeps an unparseable value out of the digest input
			// while still distinguishing every distinct populated state.
			writeDigest(raw, prefix+"text|"+strconv.Itoa(len(candidate.legacy.value))+"|"+encodeDerived(derived))
		}
	}
	if err := rows.Err(); err != nil {
		return counts, errors.Wrapf(err, "iterate fingerprint rows for %s", target.id())
	}
	return counts, nil
}

// encodeDerived renders one derived shadow value for a digest stream.
//
// A NULL shadow gets an explicit sentinel rather than an empty string so that a NULL and a
// hypothetical empty rendering cannot collide in the stream.
// Parameters:
//   - value: derived or observed shadow value.
//
// Return values:
//   - string: canonical UUID text, or the NULL sentinel.
func encodeDerived(value nullCompactUUID) string {
	if !value.valid {
		return "\x00null"
	}
	return value.value.canonical()
}

// writeDigest folds one record into a running hash with an explicit terminator.
//
// The terminator prevents boundary ambiguity: without it, adjacent records could be re-split
// at a different offset and produce the same digest for a different row set.
// Parameters:
//   - digest: running hash.
//   - record: record text to fold in.
//
// Return values: none.
func writeDigest(digest hash.Hash, record string) {
	// hash.Hash's Write never returns an error, which is why the result is discarded here.
	_, _ = digest.Write([]byte(record))
	_, _ = digest.Write([]byte{'\n'})
}

// verifyCompactFingerprints computes and compares fingerprints for every marker-carrying role.
// Parameters:
//   - ctx: context bounding the passes.
//   - topology: explicitly constructed database topology.
//
// Return values:
//   - map[uuidDBRole]compactFingerprint: retained evidence per role.
//   - bool: true when every role's equality digests match.
//   - error: wrapped error when a pass fails.
func verifyCompactFingerprints(ctx context.Context, topology *databaseTopology) (
	map[uuidDBRole]compactFingerprint, bool, error) {
	evidence := map[uuidDBRole]compactFingerprint{}
	matched := true
	for _, role := range topology.targetRoles() {
		fingerprint, err := computeCompactFingerprints(ctx, topology, role)
		if err != nil {
			return nil, false, err
		}
		evidence[role] = fingerprint
		if !fingerprint.matches() {
			matched = false
		}
	}
	return evidence, matched, nil
}
