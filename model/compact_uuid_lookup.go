package model

// This file implements the safe runtime lookup contract (AUTO-003 and AUTO-010, proposal
// section 7).
//
// The rule that makes compact reads safe is simple and absolute: an unverified compact
// candidate is NEVER returned. The compact index only ever nominates a candidate row; the
// row's own authoritative legacy text decides whether that candidate is the answer. If the
// compact probe finds nothing, or finds a row whose text disagrees, the lookup falls back to
// the legacy text index before returning.
//
// That is what makes both stale-not-found and stale-row impossible when a trigger is missing or
// a shadow is corrupted, and it is why a committed old-binary write is visible immediately
// through its text column even before any repair runs.
//
// Legacy text remains the response and correctness source everywhere else: free-text search,
// reports, relationship rendering, caches, and ordinary model reads never mention a compact
// column.

import (
	"context"
	"strings"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/idresolve"
)

// compactLookupProjection is the private read-only projection used for exact compact lookups.
//
// The tags are the contract. `->` makes the field read-only so the application can never write
// a compact column — the database derives it — and `-:migration` keeps AutoMigrate from owning
// compact DDL, which is what lets a pinned old binary's AutoMigrate leave the shadows entirely
// untouched. The type is private and never embedded in a shared or public model struct, so no
// compact column can leak into an API payload or a cache entry.
type compactLookupProjection struct {
	// ID is the row's internal primary key.
	ID int64 `json:"-" gorm:"column:id;->;-:migration"`
	// Legacy is the row's authoritative UUID text, which decides every lookup.
	Legacy string `json:"-" gorm:"column:uuid;->;-:migration"`
}

// resolveIDByUUID resolves one owned UUID to its internal row ID under the safe contract.
//
// The algorithm is the proposal's, in order: canonicalize the request input, use the compact
// predicate only when this process has a fresh healthy audit, verify any candidate against its
// authoritative text, and fall back to the legacy index on any miss, disagreement, or
// capability error.
// Parameters:
//   - ctx: context bounding the lookups.
//   - db: authoritative handle for the target table.
//   - target: registry target identifying the table and its owned UUID column.
//   - ref: raw external identifier from the request.
//
// Return values:
//   - int64: internal row ID.
//   - error: idresolve.ErrInvalidRef for malformed input, idresolve.ErrNotFound for a
//     canonical unknown UUID, or a wrapped database error.
func resolveIDByUUID(ctx context.Context, db *gorm.DB, target compactTarget, ref string) (int64, error) {
	// Trimming happens here, at the request boundary, not in the codec: the codec's accept
	// boundary must match the trigger's byte-for-byte.
	canonical, err := canonicalizeLookupRef(ref)
	if err != nil {
		return 0, err
	}

	// repairResolved records that the compact index failed to find a row the text index may
	// still find. If the text index does find it, that row is proven to have a missing or wrong
	// shadow, and only then is there anything for the worker to repair.
	repairResolved := false
	// queuedMismatch records that this lookup already handed the worker a wrongly-nominated row.
	queuedMismatch := false
	if enabled, _ := compactReadsEnabled(target.role); enabled {
		id, found, reason, err := probeCompactCandidate(ctx, db, target, canonical)
		switch {
		case err != nil:
			// A capability race — the column or index vanished under us — is handled once,
			// by falling back. It is not both logged and returned.
			recordCompactLookupFallback(target.role, compactFallbackCapability)
			disableCompactReads(target.role, compactStateDegraded, "compact lookup hit a capability error")
			signalCompactRepair()
		case found:
			return id, nil
		default:
			// The probe reports WHY it did not produce an answer, and that reason is
			// recorded exactly once. Recording a fixed reason here instead would count a
			// text-disagreement as both a mismatch and a miss, and the two mean different
			// things: a miss is a gap to fill, a mismatch is a wrong shadow to repair.
			recordCompactLookupFallback(target.role, reason)
			if reason == compactFallbackMismatch {
				// The index nominated this row for an identifier its own text does not hold.
				enqueueCompactRowRepair(target, id)
				queuedMismatch = true
			}
			repairResolved = true
		}
	} else {
		// No fresh healthy audit, so the compact predicate is not used at all. Section 7 lists
		// expired-health as a fallback reason, and it is recorded even on a deployment where
		// compact has never completed: the counter is the operator's evidence that this
		// process is serving legacy text, and staying silent there would make "compact is off"
		// and "compact is broken" look identical.
		recordCompactLookupFallback(target.role, compactFallbackExpiredHealth)
	}

	resolved, err := resolveIDByLegacyUUID(ctx, db, target, canonical, strings.TrimSpace(ref))
	if err == nil && repairResolved {
		// The text index found a row the compact index could not: that row's shadow is proven
		// missing or wrong. An identifier that exists nowhere reaches this point with an error
		// instead, so a stream of unknown identifiers never asks the worker to do anything.
		enqueueCompactRowRepair(target, resolved)
		signalCompactRepair()
	} else if queuedMismatch {
		// The identifier exists nowhere, but a row's shadow claimed it, and that row is queued.
		signalCompactRepair()
	}
	return resolved, err
}

// canonicalizeLookupRef trims, validates, and canonicalizes one external identifier.
//
// Invalid input returns the existing ErrInvalidRef, unchanged from the pre-compact behavior, so
// no caller's error handling has to know compact storage exists.
// Parameters:
//   - ref: raw external identifier from the request.
//
// Return values:
//   - compactUUID: canonical parsed value.
//   - error: idresolve.ErrInvalidRef when the input is not a canonical UUIDv7.
func canonicalizeLookupRef(ref string) (compactUUID, error) {
	value, err := parseCompactUUID(strings.TrimSpace(ref))
	if err != nil {
		// The offending value is deliberately not included: it is request input and may be
		// attacker-supplied.
		return compactUUID{}, idresolve.ErrInvalidRef
	}
	return value, nil
}

// probeCompactCandidate loads a candidate through the compact index and verifies its text.
//
// The verification is the entire point of this function. The compact column is derived data; if
// a trigger were missing or a shadow corrupted, the index could nominate the wrong row or no
// row at all. Only the row's own authoritative text can confirm the candidate, so the query
// selects the legacy column and compares it before the ID is trusted.
// Parameters:
//   - ctx: context bounding the query.
//   - db: authoritative handle for the target table.
//   - target: registry target identifying the table and columns.
//   - canonical: parsed request identifier.
//   - original: trimmed request text before canonicalization.
//
// Return values:
//   - int64: internal row ID when a candidate verified; on a mismatch, the ID of the row whose
//     shadow nominated it, which is never an answer and only ever a repair target.
//   - bool: true when a candidate verified against its authoritative text.
//   - string: compile-time fallback reason when no candidate verified.
//   - error: raw database error, so the caller can classify a capability race.
func probeCompactCandidate(ctx context.Context, db *gorm.DB, target compactTarget,
	canonical compactUUID) (int64, bool, string, error) {
	dialect := dialectName(db)
	rows := []compactLookupProjection{}

	// The compact column is compared directly against a bound value: no cast and no function
	// wraps the column, so the predicate stays sargable against the compact index.
	sql := "SELECT " + quoteIdentifier(db, "id") + " AS id, " +
		quoteIdentifier(db, target.legacyColumn) + " AS uuid" +
		" FROM " + quoteIdentifier(db, target.table) +
		" WHERE " + quoteIdentifier(db, target.compactColumn) + " = ? LIMIT 1"

	if err := db.WithContext(ctx).Raw(sql, compactBindValue(dialect, canonical)).Scan(&rows).Error; err != nil {
		return 0, false, "", err
	}
	if len(rows) == 0 {
		// The shadow for this identifier is missing: a gap for the worker to fill.
		return 0, false, compactFallbackMissing, nil
	}

	// Canonicalize the stored text before comparing: a legacy value written in a different
	// case is still the same identifier, and the codec normalizes exactly as the trigger does.
	stored, err := parseCompactUUID(strings.TrimSpace(rows[0].Legacy))
	if err != nil || stored != canonical {
		// The shadow nominated a row whose authoritative text says otherwise. Never return
		// it as the answer; report its ID alongside found=false so the caller can have exactly
		// that row repaired.
		recordCompactMismatchBacklog(target, 1)
		return rows[0].ID, false, compactFallbackMismatch, nil
	}
	return rows[0].ID, true, "", nil
}

// resolveIDByLegacyUUID resolves one owned UUID through the authoritative text index.
//
// This is the pre-compact behavior, preserved exactly. It is the correctness source and the
// permanent fallback, and it is what every unmodified caller already relies on.
// Parameters:
//   - ctx: context bounding the query.
//   - db: authoritative handle for the target table.
//   - target: registry target identifying the table and its owned UUID column.
//   - canonical: parsed request identifier.
//
// Return values:
//   - int64: internal row ID.
//   - error: idresolve.ErrNotFound for a canonical unknown UUID, or a wrapped database error.
func resolveIDByLegacyUUID(ctx context.Context, db *gorm.DB, target compactTarget,
	canonical compactUUID, original string) (int64, error) {
	id, err := resolveIDByLegacyText(ctx, db, target, original)
	if err == nil || !errors.Is(err, idresolve.ErrNotFound) || original == canonical.canonical() {
		return id, err
	}
	return resolveIDByLegacyText(ctx, db, target, canonical.canonical())
}

// resolveIDByLegacyText resolves one owned UUID through the authoritative text index without
// changing its stored representation. This preserves legacy behavior while compact health is off.
// Parameters:
//   - ctx: context bounding the query.
//   - db: authoritative handle for the target table.
//   - target: registry target identifying the table and its owned UUID column.
//   - text: trimmed legacy identifier in the caller's original representation.
//
// Return values:
//   - int64: internal row ID.
//   - error: idresolve.ErrNotFound or a wrapped database error.
func resolveIDByLegacyText(ctx context.Context, db *gorm.DB, target compactTarget, text string) (int64, error) {
	rows := []compactLookupProjection{}
	sql := "SELECT " + quoteIdentifier(db, "id") + " AS id, " +
		quoteIdentifier(db, target.legacyColumn) + " AS uuid" +
		" FROM " + quoteIdentifier(db, target.table) +
		" WHERE " + quoteIdentifier(db, target.legacyColumn) + " = ? LIMIT 1"

	if err := db.WithContext(ctx).Raw(sql, text).Scan(&rows).Error; err != nil {
		// Cancellation, connection, serialization, and other general database errors keep
		// their existing behavior and are never relabeled as a miss.
		return 0, errors.Wrapf(err, "resolve %s by legacy uuid", target.id())
	}
	if len(rows) == 0 {
		return 0, idresolve.ErrNotFound
	}
	return rows[0].ID, nil
}

// compactLookupTarget returns the owned registry target for one authoritative table.
// Parameters:
//   - table: trusted registry table name.
//
// Return values:
//   - compactTarget: the table's owned UUID target.
//   - error: wrapped error when the table owns no UUID in the registry.
func compactLookupTarget(table string) (compactTarget, error) {
	return compactTargetByID(table + ".uuid")
}

// resolvePublicIDByUUID routes a compatibility lookup through the verified compact contract.
// Parameters:
//   - ctx: context bounding compact and legacy database queries.
//   - db: authoritative database handle for the table.
//   - table: trusted owned-target table name from the compile-time registry.
//   - ref: raw external UUID reference.
//
// Return values:
//   - int: internal primary key.
//   - error: wrapped target lookup, validation, not-found, or database error.
func resolvePublicIDByUUID(ctx context.Context, db *gorm.DB, table string, ref string) (int, error) {
	target, err := compactLookupTarget(table)
	if err != nil {
		return 0, errors.Wrapf(err, "resolve compact lookup target for %s", table)
	}
	if enabled, _ := compactReadsEnabled(target.role); !enabled {
		trimmed := strings.TrimSpace(ref)
		if trimmed == "" {
			return 0, errors.New("uuid is empty")
		}
		recordCompactLookupFallback(target.role, compactFallbackExpiredHealth)
		id, legacyErr := resolveIDByLegacyText(ctx, db, target, trimmed)
		if legacyErr != nil {
			return 0, legacyErr
		}
		return int(id), nil
	}
	id, err := resolveIDByUUID(ctx, db, target, ref)
	if err != nil {
		return 0, err
	}
	return int(id), nil
}
