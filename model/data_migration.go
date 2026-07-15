package model

import (
	"context"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"gorm.io/gorm"
)

const (
	// externalUUIDPrimaryMigrationKeyV2 is the superseded primary completion key. It is
	// retained only so operators can recognize historical rows. A v2 marker never proves
	// that cross-database UUID reconciliation completed, because the v2 coordinator could
	// write the log marker before primary owners had UUIDs, so it must never suppress
	// current-generation reconciliation.
	externalUUIDPrimaryMigrationKeyV2 = "external_uuid_backfill_v2_primary"
	// externalUUIDLogMigrationKeyV2 is the superseded split-log completion key.
	externalUUIDLogMigrationKeyV2 = "external_uuid_backfill_v2_log"

	// externalUUIDPrimaryMigrationKey is the current-generation primary completion key.
	externalUUIDPrimaryMigrationKey = "external_uuid_backfill_v3_primary"
	// externalUUIDLogMigrationKey is the current-generation split-log completion key.
	externalUUIDLogMigrationKey = "external_uuid_backfill_v3_log"
)

// DataMigration records completed one-time data migrations.
type DataMigration struct {
	MigrationKey string    `gorm:"primaryKey;size:128;column:migration_key"`
	CompletedAt  time.Time `gorm:"not null;column:completed_at"`
}

// TableName returns the database table name used for completed migration markers.
// Parameters: none.
//
// Return values:
//   - string: the table name for completed data migrations.
func (DataMigration) TableName() string {
	return "data_migrations"
}

// uuidCompletionKeyForRole returns the current-generation completion key for a database role.
// Parameters:
//   - role: physical database role carrying the marker.
//
// Return values:
//   - string: versioned completion key.
func uuidCompletionKeyForRole(role uuidDBRole) string {
	if role == uuidRoleLog {
		return externalUUIDLogMigrationKey
	}
	return externalUUIDPrimaryMigrationKey
}

// uuidMarkerState records which current-generation markers exist for a topology.
type uuidMarkerState struct {
	// present maps each applicable role to whether its v3 marker already exists.
	present map[uuidDBRole]bool
}

// allPresent reports whether every applicable current-generation marker exists.
// Parameters: none.
//
// Return values:
//   - bool: true only when no applicable marker is missing.
func (state uuidMarkerState) allPresent() bool {
	for _, complete := range state.present {
		if !complete {
			// When either split marker is absent the coordinator reruns global
			// reconciliation and validation; the other database's marker never permits
			// skipping dependency checks.
			return false
		}
	}
	return len(state.present) > 0
}

// isPrimaryOnlyRecovery reports the inconsistent split state where only the primary
// marker exists. It is the sole exception to primary-last insertion: the existing primary
// marker's timestamp is preserved and the missing log marker is written after global
// validation.
// Parameters: none.
//
// Return values:
//   - bool: true when the primary marker exists but the log marker does not.
func (state uuidMarkerState) isPrimaryOnlyRecovery() bool {
	logComplete, hasLog := state.present[uuidRoleLog]
	return hasLog && !logComplete && state.present[uuidRolePrimary]
}

// readUUIDMarkerState performs exactly one indexed marker lookup per marker-carrying
// database and never consults a v2 marker.
// Parameters:
//   - ctx: context controlling the marker lookups.
//   - topology: explicitly constructed database topology.
//
// Return values:
//   - uuidMarkerState: observed marker presence per applicable role.
//   - error: wrapped database error when a lookup fails.
func readUUIDMarkerState(ctx context.Context, topology *databaseTopology) (uuidMarkerState, error) {
	state := uuidMarkerState{present: map[uuidDBRole]bool{}}
	for _, role := range topology.markerRoles() {
		key := uuidCompletionKeyForRole(role)
		complete, err := isDataMigrationComplete(ctx, topology.handle(role), key)
		if err != nil {
			return state, errors.Wrapf(err, "check %s uuid completion marker", role)
		}
		state.present[role] = complete
	}
	return state, nil
}

// writeUUIDCompletionMarkers writes markers after global validation succeeds.
// Cross-database marker writes cannot be atomic, so normal split finalization writes the
// log marker first and the primary marker last as the coordinator's terminal marker. A
// crash between the two leaves partial marker state that the next finalizer recovers by
// rerunning every phase and global validation. Primary-only state is the documented
// recovery exception: it never rewrites the existing primary timestamp and is reported as
// a warning because it means a crash, a manual deletion, or an earlier defect.
// Parameters:
//   - ctx: context controlling the marker writes.
//   - topology: explicitly constructed database topology.
//   - observed: marker state read at coordinator entry.
//
// Return values:
//   - error: wrapped database error when a marker cannot be written or confirmed.
func writeUUIDCompletionMarkers(ctx context.Context, topology *databaseTopology, observed uuidMarkerState) error {
	log := uuidMigrationLogger(ctx)
	if observed.isPrimaryOnlyRecovery() {
		log.Warn("recovering inconsistent external uuid marker state: primary marker exists without log marker",
			zap.String("topology", string(topology.mode)))
	}
	if topology.mode == uuidTopologySplit {
		if err := markDataMigrationComplete(ctx, topology.log, externalUUIDLogMigrationKey); err != nil {
			return errors.Wrap(err, "mark log uuid migration complete")
		}
	}
	// markDataMigrationComplete is a no-op when the marker already exists, so an existing
	// primary timestamp survives the recovery path untouched.
	if err := markDataMigrationComplete(ctx, topology.primary, externalUUIDPrimaryMigrationKey); err != nil {
		return errors.Wrap(err, "mark primary uuid migration complete")
	}
	log.Info("external uuid completion markers written",
		zap.String("topology", string(topology.mode)),
		zap.Int("marker_count", len(topology.markerRoles())))
	return nil
}

// isDataMigrationComplete reports whether a migration completion marker exists.
// Parameters:
//   - ctx: context controlling the database lookup.
//   - db: database handle containing data_migrations.
//   - key: versioned migration key to read.
//
// Return values:
//   - bool: true when the completion marker exists.
//   - error: wrapped database error when the lookup fails.
func isDataMigrationComplete(ctx context.Context, db *gorm.DB, key string) (bool, error) {
	if db == nil {
		return false, errors.New("database is nil")
	}
	var count int64
	err := db.WithContext(ctx).
		Model(&DataMigration{}).
		Where("migration_key = ?", key).
		Count(&count).Error
	if err != nil {
		return false, errors.Wrapf(err, "read data migration marker %s", key)
	}
	return count > 0, nil
}

// markDataMigrationComplete inserts a completion marker after final validation succeeds.
// A concurrent worker racing on the same key is tolerated only when the database
// classifies the failure as a duplicate object and a reread confirms the exact marker;
// every other database error is returned.
// Parameters:
//   - ctx: context controlling the database write.
//   - db: database handle containing data_migrations.
//   - key: versioned migration key to write.
//
// Return values:
//   - error: wrapped database error when the marker cannot be written or confirmed.
func markDataMigrationComplete(ctx context.Context, db *gorm.DB, key string) error {
	if db == nil {
		return errors.New("database is nil")
	}
	complete, err := isDataMigrationComplete(ctx, db, key)
	if err != nil {
		return err
	}
	if complete {
		// Repeated finalizer invocations are marker-only no-ops and preserve the
		// original completion timestamp.
		return nil
	}

	marker := &DataMigration{MigrationKey: key, CompletedAt: time.Now().UTC()}
	if err := db.WithContext(ctx).Create(marker).Error; err != nil {
		if !isDuplicateObjectError(err) {
			return errors.Wrapf(err, "insert data migration marker %s", key)
		}
		complete, rereadErr := isDataMigrationComplete(ctx, db, key)
		if rereadErr != nil {
			return errors.Wrap(rereadErr, "confirm data migration marker after insert race")
		}
		if !complete {
			return errors.Wrapf(err, "insert data migration marker %s", key)
		}
	}
	return nil
}

// isDuplicateObjectError classifies a database error as a duplicate-object race.
// Only these errors may be converted into idempotent success, and only after a
// metadata or row reread verifies the exact expected object. Classification is by
// dialect message because the migration path deliberately does not import driver
// error types for every supported backend.
// Parameters:
//   - err: database error returned by an insert or DDL statement.
//
// Return values:
//   - bool: true when the error means the object already exists.
func isDuplicateObjectError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	switch {
	// SQLite.
	case strings.Contains(message, "unique constraint failed"),
		strings.Contains(message, "constraint failed: unique"),
		// MySQL 1062 duplicate entry, 1061 duplicate key name, 1050 table exists.
		strings.Contains(message, "duplicate entry"),
		strings.Contains(message, "duplicate key name"),
		// PostgreSQL 23505 unique_violation, 42P07 duplicate_table/duplicate_object.
		strings.Contains(message, "duplicate key value violates unique constraint"),
		strings.Contains(message, "already exists"):
		return true
	default:
		return false
	}
}
