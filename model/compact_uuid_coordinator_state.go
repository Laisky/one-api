package model

// This file holds the compact coordinator's epoch, ownership, and generation helpers
// (AUTO-008). They are split out of compact_uuid_migration.go purely to keep both files inside
// the proposal's 600-line limit (section 9.3); the cycle itself lives next door.

import (
	"context"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

// recordCleanPass counts one clean pass against the current epoch.
//
// The epoch's owner component is the worker's stable token, not the ownership claim's: the two
// passes that authorize completion are consecutive cycles of the SAME worker, and each cycle
// legitimately re-acquires the lock. See compactCoordinator.workerToken.
// Parameters:
//   - ownership: ownership claim held for this pass, proving the worker still owns the work.
//
// Return values: none.
func (coordinator *compactCoordinator) recordCleanPass(ownership *compactOwnership) {
	_ = ownership
	epoch := compactPassEpoch{
		generation:    currentInitGeneration(),
		owner:         coordinator.workerToken,
		topology:      coordinator.topology.mode,
		objectVersion: compactObjectVersion,
	}
	if !coordinator.epoch.equal(epoch) {
		coordinator.epoch = epoch
		coordinator.cleanPasses = 0
	}
	coordinator.cleanPasses++
}

// resetEpoch discards the clean-pass streak after anything that changes the observed world.
// Parameters: none.
//
// Return values: none.
func (coordinator *compactCoordinator) resetEpoch() {
	coordinator.cleanPasses = 0
	coordinator.epoch = compactPassEpoch{}
}

// requireOwnership verifies the claim is still held before and after each side effect.
//
// Ownership loss cancels the cycle before another side effect starts. It is returned as an
// error so the worker backs off, but it is not a correctness dependency: a lost lock cannot
// corrupt anything, because every side effect is independently safe.
// Parameters:
//   - ctx: context bounding the verification query.
//   - ownership: ownership claim to verify.
//
// Return values:
//   - error: wrapped error when the claim is lost or cannot be verified.
func requireOwnership(ctx context.Context, ownership *compactOwnership) error {
	if ownership == nil {
		return errors.New("compact migration cycle has no ownership claim")
	}
	held, err := ownership.verify(ctx)
	if err != nil {
		return errors.Wrap(err, "verify compact migration ownership")
	}
	if !held {
		return errors.New("compact migration ownership was lost during the cycle")
	}
	return nil
}

// currentInitGeneration returns the process's current database init generation.
// Parameters: none.
//
// Return values:
//   - uint64: init generation, used as part of the clean-pass epoch.
func currentInitGeneration() uint64 {
	bootstrapMu.Lock()
	defer bootstrapMu.Unlock()
	return initGeneration
}

// compactHandleForRole returns the authoritative handle for a role, or nil.
// Parameters:
//   - topology: explicitly constructed database topology.
//   - role: database role.
//
// Return values:
//   - *gorm.DB: authoritative handle.
func compactHandleForRole(topology *databaseTopology, role uuidDBRole) *gorm.DB {
	return topology.handle(role)
}
