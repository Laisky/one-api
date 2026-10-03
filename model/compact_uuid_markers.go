package model

// This file owns the compact completion markers and the v3 prerequisite check (AUTO-007).
//
// Compact markers differ from the external UUID backfill's in one contractual way: they record
// that the historical installation completed, and nothing more. They never stop the audit
// loop, never authorize an unverified compact read, and are never deleted or rewritten when
// drift is later detected. Health degrades; the marker's timestamp stays stable.

import (
	"context"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
)

const (
	// compactPrimaryMigrationKey is the primary completion key for this generation.
	compactPrimaryMigrationKey = compactMigrationGeneration + "_primary"
	// compactLogMigrationKey is the split-log completion key for this generation.
	compactLogMigrationKey = compactMigrationGeneration + "_log"
)

// compactCompletionKeyForRole returns the compact completion key for a database role.
// Parameters:
//   - role: physical database role carrying the marker.
//
// Return values:
//   - string: versioned completion key.
func compactCompletionKeyForRole(role uuidDBRole) string {
	if role == uuidRoleLog {
		return compactLogMigrationKey
	}
	return compactPrimaryMigrationKey
}

// compactMarkerState records which compact markers exist for a topology.
type compactMarkerState struct {
	// present maps each applicable role to whether its compact marker already exists.
	present map[uuidDBRole]bool
}

// allPresent reports whether every applicable compact marker exists.
// Parameters: none.
//
// Return values:
//   - bool: true only when no applicable marker is missing.
func (state compactMarkerState) allPresent() bool {
	for _, complete := range state.present {
		if !complete {
			return false
		}
	}
	return len(state.present) > 0
}

// anyPresent reports whether at least one applicable compact marker exists.
// Parameters: none.
//
// Return values:
//   - bool: true when any marker is present.
func (state compactMarkerState) anyPresent() bool {
	for _, complete := range state.present {
		if complete {
			return true
		}
	}
	return false
}

// isPartial reports the split state where exactly one marker exists.
//
// Partial state means a crash between the two cross-database writes, or a manual deletion. It
// is recovered by rerunning global validation and inserting only the missing marker; the
// existing marker's timestamp is never rewritten.
// Parameters: none.
//
// Return values:
//   - bool: true when some but not all applicable markers exist.
func (state compactMarkerState) isPartial() bool {
	return state.anyPresent() && !state.allPresent()
}

// readCompactMarkerState performs one indexed marker lookup per marker-carrying database.
// Parameters:
//   - ctx: context controlling the marker lookups.
//   - topology: explicitly constructed database topology.
//
// Return values:
//   - compactMarkerState: observed marker presence per applicable role.
//   - error: wrapped database error when a lookup fails.
func readCompactMarkerState(ctx context.Context, topology *databaseTopology) (compactMarkerState, error) {
	state := compactMarkerState{present: map[uuidDBRole]bool{}}
	for _, role := range topology.markerRoles() {
		key := compactCompletionKeyForRole(role)
		complete, err := isDataMigrationComplete(ctx, topology.handle(role), key)
		if err != nil {
			return state, errors.Wrapf(err, "check %s compact uuid completion marker", role)
		}
		state.present[role] = complete
	}
	return state, nil
}

// compactPrerequisiteMet reports whether every applicable v3 external-UUID marker exists.
//
// This is the source prerequisite: compact storage derives its shadows from legacy text, and
// the v3 backfill is what guarantees that text is populated and canonical. A v2 marker never
// satisfies it, which readUUIDMarkerState already enforces by only ever reading v3 keys.
//
// The compact worker may still expand, install triggers, and backfill while v3 is running —
// those are additive and safe. What it must not do is write a completion marker before v3's
// markers and its own validation both pass.
// Parameters:
//   - ctx: context controlling the marker lookups.
//   - topology: explicitly constructed database topology.
//
// Return values:
//   - bool: true when every applicable v3 marker is present.
//   - error: wrapped database error when a lookup fails.
func compactPrerequisiteMet(ctx context.Context, topology *databaseTopology) (bool, error) {
	markers, err := readUUIDMarkerState(ctx, topology)
	if err != nil {
		return false, errors.Wrap(err, "check external uuid v3 prerequisite markers")
	}
	return markers.allPresent(), nil
}

// writeCompactCompletionMarkers writes the missing compact markers after validation succeeds.
//
// Split ordering is log-first, primary-last: the primary marker is the coordinator's terminal
// marker, so a crash between the two can only ever leave the recoverable log-only state rather
// than a primary marker that overstates completion. markDataMigrationComplete is a no-op when
// a marker already exists, so partial recovery inserts only what is missing and every existing
// timestamp survives untouched.
// Parameters:
//   - ctx: context controlling the marker writes.
//   - topology: explicitly constructed database topology.
//   - observed: marker state read at cycle entry.
//
// Return values:
//   - error: wrapped database error when a marker cannot be written or confirmed.
func writeCompactCompletionMarkers(ctx context.Context, topology *databaseTopology, observed compactMarkerState) error {
	log := compactLogger(ctx)
	if observed.isPartial() {
		log.Warn("recovering partial compact uuid marker state after global revalidation",
			zap.String("topology", string(topology.mode)))
	}
	if topology.mode == uuidTopologySplit {
		if err := markDataMigrationComplete(ctx, topology.log, compactLogMigrationKey); err != nil {
			return errors.Wrap(err, "mark log compact uuid migration complete")
		}
	}
	if err := markDataMigrationComplete(ctx, topology.primary, compactPrimaryMigrationKey); err != nil {
		return errors.Wrap(err, "mark primary compact uuid migration complete")
	}
	log.Info("compact uuid completion markers written",
		zap.String("topology", string(topology.mode)),
		zap.Int("marker_count", len(topology.markerRoles())))
	return nil
}
