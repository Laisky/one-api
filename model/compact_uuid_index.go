package model

// This file creates and verifies the additive compact indexes, and owns the legacy-index
// manifest (AUTO-005).
//
// The manifest is the mechanism behind the compatibility contract's hardest promise: that
// every pre-migration UUID-related index survives expansion byte-for-byte. Normalized metadata
// for every such index is captured and checksummed BEFORE the first compact DDL, and the
// worker re-verifies it before every later DDL or repair. An absent or mismatched manifest
// blocks compact mutation rather than risking a silently altered legacy index.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

// compactManifestKeyPrefix namespaces the manifest rows stored in data_migrations.
//
// The manifest reuses the existing DataMigration table rather than adding a schema object: an
// extra table would itself be additive DDL that a pinned old binary's AutoMigrate has never
// seen, and the whole point of the manifest is to exist before any compact DDL runs.
const compactManifestKeyPrefix = compactMigrationGeneration + "_index_manifest_"

// compactManifestKeyForRole returns the manifest storage key for one database role.
// Parameters:
//   - role: physical database role whose legacy indexes the manifest describes.
//
// Return values:
//   - string: versioned manifest key.
func compactManifestKeyForRole(role uuidDBRole) string {
	return compactManifestKeyPrefix + string(role)
}

// legacyIndexRecord is the normalized metadata captured for one pre-migration index.
type legacyIndexRecord struct {
	// Table is the authoritative table the index belongs to.
	Table string `json:"table"`
	// Name is the index name.
	Name string `json:"name"`
	// Columns are the indexed columns in their defined order.
	Columns []string `json:"columns"`
	// Unique reports whether the index enforces uniqueness.
	Unique bool `json:"unique"`
}

// legacyIndexManifest is the versioned, checksummed snapshot of one role's legacy indexes.
type legacyIndexManifest struct {
	// Version is the compact object version that produced this manifest.
	Version string `json:"version"`
	// Role is the database role the manifest describes.
	Role string `json:"role"`
	// Indexes are the captured records in deterministic order.
	Indexes []legacyIndexRecord `json:"indexes"`
	// Checksum is the SHA-256 over the canonical encoding of Indexes.
	Checksum string `json:"checksum"`
}

// captureLegacyIndexes reads normalized metadata for every UUID-related legacy index of a role.
//
// Only indexes that touch a legacy UUID column are captured. That is the scope the contract
// protects, and a narrower snapshot is also a stabler one: unrelated indexes churn with
// ordinary feature work and would make the manifest mismatch for reasons that have nothing to
// do with compact storage.
//
// Compact indexes are deliberately excluded. They are additive objects created by this
// migration, so including them would make the manifest disagree with itself the moment the
// first compact index is built.
// Parameters:
//   - ctx: context bounding the metadata reads.
//   - db: authoritative handle for the role.
//   - role: database role whose tables are captured.
//
// Return values:
//   - []legacyIndexRecord: captured records in deterministic order.
//   - error: wrapped error when metadata cannot be read.
func captureLegacyIndexes(ctx context.Context, db *gorm.DB, role uuidDBRole) ([]legacyIndexRecord, error) {
	ctx, cancel := withCompactMetadataDeadline(ctx)
	defer cancel()
	records := []legacyIndexRecord{}
	for _, table := range compactTablesForRole(role) {
		if !db.WithContext(ctx).Migrator().HasTable(table.table) {
			continue
		}
		indexes, err := db.WithContext(ctx).Migrator().GetIndexes(table.model)
		if err != nil {
			return nil, errors.Wrapf(err, "read legacy index metadata for %s", table.table)
		}
		for _, index := range indexes {
			if strings.Contains(index.Name(), compactColumnSuffix) {
				continue
			}
			columns := index.Columns()
			if !indexTouchesLegacyUUID(table, columns) {
				continue
			}
			unique, _ := index.Unique()
			records = append(records, legacyIndexRecord{
				Table:   table.table,
				Name:    index.Name(),
				Columns: append([]string{}, columns...),
				Unique:  unique,
			})
		}
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].Table != records[j].Table {
			return records[i].Table < records[j].Table
		}
		return records[i].Name < records[j].Name
	})
	return records, nil
}

// indexTouchesLegacyUUID reports whether an index covers any of a table's legacy UUID columns.
// Parameters:
//   - table: registry table whose legacy columns define the scope.
//   - columns: observed indexed columns.
//
// Return values:
//   - bool: true when at least one indexed column is a legacy UUID column.
func indexTouchesLegacyUUID(table compactTable, columns []string) bool {
	for _, column := range columns {
		for _, target := range table.targets {
			if equalFoldASCII(column, target.legacyColumn) {
				return true
			}
		}
	}
	return false
}

// checksumLegacyIndexes returns the SHA-256 over the canonical encoding of captured records.
// Parameters:
//   - records: captured records in deterministic order.
//
// Return values:
//   - string: lowercase hex checksum.
//   - error: wrapped error when the records cannot be encoded.
func checksumLegacyIndexes(records []legacyIndexRecord) (string, error) {
	encoded, err := json.Marshal(records)
	if err != nil {
		return "", errors.Wrap(err, "encode legacy index manifest for checksum")
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

// ensureLegacyIndexManifest persists the manifest on first use and verifies it afterwards.
//
// The ordering is the contract: this must succeed before the first compact DDL of a role, and
// it is re-checked before every later DDL or repair. A mismatch means a legacy UUID index
// changed shape after the baseline was taken, which the compatibility contract forbids, so the
// caller blocks compact mutation instead of proceeding.
//
// Verification is deliberately SUBSET, not set-equality: every index the baseline captured must
// still exist with an identical shape, but an index appearing that the baseline did not capture
// is not a violation. That distinction is load-bearing rather than lenient, and a real rollback
// proved it. The immediately preceding production build (`ed15a144`) declares
// `gorm:"...;index;column:uuid"` on every owned UUID column, so ITS AutoMigrate legitimately
// recreates `idx_<table>_uuid` on all 12 tables — an index the v3 generation had dropped after
// promoting the stronger unique one. Under set-equality that ordinary, supported rollback added
// 12 indexes, the checksum diverged, and compact blocked permanently on a database nothing had
// damaged, breaking section 10.2's promise that redeploying resumes and reconverges.
//
// Section 5 asks that each captured index's "name, columns/order, uniqueness, predicate/prefix,
// collation, visibility, and validity ... remain unchanged" — a statement about the indexes in
// the baseline, not a prohibition on any index ever being added. An old binary re-adding an
// index it owns is additive, is on the legacy column the contract exists to protect, and takes
// nothing away.
// Parameters:
//   - ctx: context bounding the metadata reads and the manifest write.
//   - db: authoritative handle for the role.
//   - role: database role to snapshot or verify.
//
// Return values:
//   - bool: true when every captured legacy index still exists unchanged.
//   - string: bounded, value-free reason when one does not.
//   - error: wrapped error when metadata or the manifest row cannot be read or written.
func ensureLegacyIndexManifest(ctx context.Context, db *gorm.DB, role uuidDBRole) (bool, string, error) {
	observed, err := captureLegacyIndexes(ctx, db, role)
	if err != nil {
		return false, "", err
	}

	stored, found, err := readLegacyIndexManifest(ctx, db, role)
	if err != nil {
		return false, "", err
	}
	if !found {
		checksum, err := checksumLegacyIndexes(observed)
		if err != nil {
			return false, "", err
		}
		manifest := legacyIndexManifest{
			Version:  compactObjectVersion,
			Role:     string(role),
			Indexes:  observed,
			Checksum: checksum,
		}
		if err := writeLegacyIndexManifest(ctx, db, role, manifest); err != nil {
			return false, "", err
		}
		return true, "", nil
	}

	if stored.Version != compactObjectVersion {
		return false, "legacy index manifest was written by a different compact object version", nil
	}
	// The manifest is never rewritten to match reality: that would launder exactly the change
	// the contract forbids. Compact mutation blocks until the legacy index is restored.
	return legacyIndexesPreserved(stored.Indexes, observed)
}

// legacyIndexesPreserved reports whether every captured legacy index survives unchanged.
// Parameters:
//   - captured: the baseline records from the durable manifest.
//   - observed: the records read from the database now.
//
// Return values:
//   - bool: true when every captured record has an identical counterpart.
//   - string: bounded, value-free reason naming the first index that changed or vanished.
//   - error: always nil; the signature matches its caller's contract.
func legacyIndexesPreserved(captured []legacyIndexRecord, observed []legacyIndexRecord) (bool, string, error) {
	current := make(map[string]legacyIndexRecord, len(observed))
	for _, record := range observed {
		current[record.Table+"."+record.Name] = record
	}

	for _, want := range captured {
		got, present := current[want.Table+"."+want.Name]
		if !present {
			return false, "legacy uuid index " + want.Name + " on " + want.Table +
				" was dropped after the pre-expansion baseline was taken", nil
		}
		if !sameLegacyIndexShape(want, got) {
			return false, "legacy uuid index " + want.Name + " on " + want.Table +
				" changed shape after the pre-expansion baseline was taken", nil
		}
	}
	return true, "", nil
}

// sameLegacyIndexShape compares two captured index records field by field.
// Parameters:
//   - left: baseline record.
//   - right: currently observed record.
//
// Return values:
//   - bool: true when uniqueness and the ordered column list are identical.
func sameLegacyIndexShape(left legacyIndexRecord, right legacyIndexRecord) bool {
	if left.Unique != right.Unique || len(left.Columns) != len(right.Columns) {
		return false
	}
	for index := range left.Columns {
		// Order matters: a reordered composite index serves different queries.
		if !equalFoldASCII(left.Columns[index], right.Columns[index]) {
			return false
		}
	}
	return true
}

// readLegacyIndexManifest loads one role's durable manifest.
// Parameters:
//   - ctx: context bounding the lookup.
//   - db: authoritative handle for the role.
//   - role: database role whose manifest is read.
//
// Return values:
//   - legacyIndexManifest: decoded manifest when present.
//   - bool: true when a manifest row exists.
//   - error: wrapped error when the row cannot be read or decoded.
func readLegacyIndexManifest(ctx context.Context, db *gorm.DB, role uuidDBRole) (legacyIndexManifest, bool, error) {
	manifest := legacyIndexManifest{}
	rows := []compactManifestRow{}
	err := db.WithContext(ctx).
		Model(&compactManifestRow{}).
		Where("manifest_key = ?", compactManifestKeyForRole(role)).
		Find(&rows).Error
	if err != nil {
		return manifest, false, errors.Wrapf(err, "read compact legacy index manifest for %s", role)
	}
	if len(rows) == 0 {
		return manifest, false, nil
	}
	if err := json.Unmarshal([]byte(rows[0].Payload), &manifest); err != nil {
		return manifest, false, errors.Wrapf(err, "decode compact legacy index manifest for %s", role)
	}
	return manifest, true, nil
}

// writeLegacyIndexManifest persists one role's manifest idempotently.
// Parameters:
//   - ctx: context bounding the write.
//   - db: authoritative handle for the role.
//   - role: database role whose manifest is written.
//   - manifest: manifest value to persist.
//
// Return values:
//   - error: wrapped error when the row cannot be encoded or written.
func writeLegacyIndexManifest(ctx context.Context, db *gorm.DB, role uuidDBRole, manifest legacyIndexManifest) error {
	payload, err := json.Marshal(manifest)
	if err != nil {
		return errors.Wrap(err, "encode compact legacy index manifest")
	}
	row := &compactManifestRow{
		ManifestKey: compactManifestKeyForRole(role),
		Payload:     string(payload),
	}
	if err := db.WithContext(ctx).Create(row).Error; err != nil {
		if !isDuplicateObjectError(err) {
			return errors.Wrapf(err, "write compact legacy index manifest for %s", role)
		}
		// Another worker captured the same baseline first. That is the expected race, and
		// the caller re-verifies against the stored row on its next cycle.
	}
	return nil
}

// compactManifestRow stores one role's versioned legacy-index manifest.
//
// It is a separate table from data_migrations because a manifest carries a payload rather than
// a completion timestamp. It is created by the compact worker, never by AutoMigrate, so a
// pinned old binary neither knows nor touches it.
type compactManifestRow struct {
	// ManifestKey is the versioned, role-scoped storage key.
	ManifestKey string `gorm:"primaryKey;size:191;column:manifest_key"`
	// Payload is the JSON-encoded legacyIndexManifest.
	Payload string `gorm:"type:text;column:payload"`
}

// TableName returns the table name used for compact index manifests.
// Parameters: none.
//
// Return values:
//   - string: the manifest table name.
func (compactManifestRow) TableName() string {
	return "compact_uuid_manifests"
}

// ensureCompactManifestTable creates the manifest table if it does not exist.
//
// This is the one piece of compact schema that is not a shadow column, and it must exist before
// the manifest can be captured, which in turn must happen before the first compact DDL.
// Parameters:
//   - ctx: context bounding the DDL.
//   - db: authoritative handle for the role.
//
// Return values:
//   - error: wrapped error when the table cannot be created.
func ensureCompactManifestTable(ctx context.Context, db *gorm.DB) error {
	migrator := db.WithContext(ctx).Migrator()
	if migrator.HasTable(&compactManifestRow{}) {
		return nil
	}
	if err := migrator.AutoMigrate(&compactManifestRow{}); err != nil {
		if !isDuplicateObjectError(err) {
			return errors.Wrap(err, "create compact uuid manifest table")
		}
	}
	return nil
}

// =============================================================================
// COMPACT INDEXES
// =============================================================================

// ensureCompactIndex creates and verifies one target's compact index.
//
// The index is created before historical fill, not after. Every supported engine permits
// multiple NULL entries in a unique index, so an empty shadow column cannot collide, and
// filling into an existing index avoids a second full pass over the table. The bounded NULL
// probe that finds work also needs the index to stay off a sequential scan.
// Parameters:
//   - ctx: context bounding the metadata reads and DDL.
//   - db: authoritative handle for the target table.
//   - target: registry target whose index is ensured.
//
// Return values:
//   - bool: true when this call created the index.
//   - error: wrapped error when the index cannot be created or verified.
func ensureCompactIndex(ctx context.Context, db *gorm.DB, target compactTarget) (bool, error) {
	name := target.indexName()
	usable, err := hasUsableIndexNamed(ctx, db, target.table, name)
	if err != nil {
		return false, err
	}
	if usable {
		return false, nil
	}

	kind := uuidIndexCandidate
	if target.unique() {
		kind = uuidIndexUnique
	}
	// createUUIDIndex already implements the dialect online-DDL policy the compact contract
	// requires: PostgreSQL CONCURRENTLY outside a transaction with invalid-build cleanup,
	// MySQL INPLACE/LOCK=NONE with no silent blocking fallback, and SQLite bounded busy retry.
	err = createUUIDIndex(ctx, db, target.table, name, []string{target.compactColumn}, kind)
	if err != nil && !isDuplicateObjectError(err) {
		return false, errors.Wrapf(err, "create compact index %s", name)
	}

	verified, verifyErr := verifyCompactIndex(ctx, db, target)
	if verifyErr != nil {
		return false, verifyErr
	}
	if !verified {
		return false, errors.Errorf("compact index %s is absent or invalid after creation", name)
	}
	return err == nil, nil
}

// verifyCompactIndex confirms one compact index exists with the expected shape and validity.
//
// Name, columns, uniqueness, and dialect validity are all read from metadata. A PostgreSQL
// index left INVALID by a failed concurrent build is never accepted: it stays in the catalog
// and answers HasIndex, but the planner never uses it, so accepting it would silently degrade
// every compact probe into a sequential scan forever.
// Parameters:
//   - ctx: context bounding the metadata reads.
//   - db: authoritative handle for the target table.
//   - target: registry target whose index is verified.
//
// Return values:
//   - bool: true when the index exists with the expected shape and is valid.
//   - error: wrapped error when metadata cannot be read.
func verifyCompactIndex(ctx context.Context, db *gorm.DB, target compactTarget) (bool, error) {
	ctx, cancel := withCompactMetadataDeadline(ctx)
	defer cancel()
	indexes, err := db.WithContext(ctx).Migrator().GetIndexes(target.model)
	if err != nil {
		return false, errors.Wrapf(err, "read index metadata for %s", target.table)
	}
	name := target.indexName()
	for _, index := range indexes {
		if index.Name() != name {
			continue
		}
		columns := index.Columns()
		if len(columns) != 1 || !equalFoldASCII(columns[0], target.compactColumn) {
			return false, nil
		}
		unique, known := index.Unique()
		if target.unique() && (!known || !unique) {
			return false, nil
		}
		if !target.unique() && known && unique {
			// A unique index where the contract requires a non-unique one would reject a
			// legitimate duplicate FK write and take down an ordinary application path.
			return false, nil
		}
		return isIndexValid(ctx, db, name)
	}
	return false, nil
}
