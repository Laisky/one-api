package model

// Immediately-preceding production build qualification (AUTO-013, AUTO-T04/T05/T06/T21).
//
// Proposal section 2 requires TWO real artifacts in qualification: "the oldest supported rollback
// build containing the v3 external-UUID writer contract" AND "the immediately preceding
// production build". compact_uuid_oldbinary_test.go covers the former; this file covers the
// latter, and the difference between the two is the entire reason section 2 names both.
//
// The preceding build (ref ed15a144) predates the v3 external-UUID coordinator. Two consequences
// are what this suite measures, and neither is observable through the oldest artifact:
//
//  1. Its owned-UUID model tags still declare an ordinary index (`gorm:"...;index;column:uuid"`).
//     v3 removed that tag, promotes each owned column to an explicit unique index, then drops the
//     ordinary index it replaced — on the recorded assumption that "owned UUID model tags never
//     declare an ordinary index, so nothing recreates it on a later AutoMigrate". That holds for
//     the oldest artifact and HEAD, but not for the artifact section 2 also requires, whose
//     AutoMigrate re-creates exactly the index the compact manifest baselined as absent.
//  2. Its writers still reach the same BeforeCreate hooks, so whether it leaves a UUID-less row
//     behind is an empirical question, not an assumption. Section 2.2 fixes the answer either way:
//     a missing owned UUID blocks compact health, never violates compatibility, never mutates
//     legacy text, and never authorizes a marker.
//
// Every assertion is made against the REAL artifact and the REAL engine; nothing emulates a
// trigger, a catalog read, or an AutoMigrate. Gated on COMPACT_UUID_TEST_PREV_BINARY plus the
// PostgreSQL DSN; skips when either is absent so an ordinary `go test ./...` still passes on a
// workstation. CI's no-skip guard fails the run instead.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// compactPrevBinaryEnv names the environment variable carrying the preceding artifact's path.
const compactPrevBinaryEnv = "COMPACT_UUID_TEST_PREV_BINARY"

// compactPrevBinaryPort keeps this artifact off the port the oldest-artifact suite uses.
const compactPrevBinaryPort = "13997"

// compactPrevSettleFor is how long the artifact runs before it is stopped. It must comfortably
// exceed its whole startup path — open, AutoMigrate, the custom migrations, its own external-UUID
// migration, and the root-account write — because a premature kill would make this suite pass by
// never letting the artifact reach the code under test.
const compactPrevSettleFor = 25 * time.Second

// compactPrevLoadFirstID is the first channels primary key the legacy workload uses. It sits far
// above every fixture row so the workload cannot collide with a seeded UUID or primary key, which
// would turn a genuine compatibility failure into a test artifact.
const compactPrevLoadFirstID = 900000

// compactPrevRequiredOperations is the acknowledged-operation floor AUTO-T04 sets.
const compactPrevRequiredOperations = 1000

// compactPrevArtifact resolves the gated artifact path and PostgreSQL DSN for this suite.
// Parameters:
//   - t: test handle used for skips.
//
// Return values:
//   - string: absolute path to the pinned preceding artifact.
//   - compactLiveDialect: the PostgreSQL descriptor this suite runs against.
//   - string: primary DSN in the driver's key/value form.
//   - bool: false when the suite is not configured and must skip.
func compactPrevArtifact(t *testing.T) (string, compactLiveDialect, string, bool) {
	t.Helper()
	// PostgreSQL is the assigned engine: its native uuid shadow is the strictest type the contract
	// has to survive, and its catalog exposes trigger bodies and index definitions precisely enough
	// to prove "unchanged" rather than merely "still present".
	dialect := compactLiveDialects()[1]

	binary := strings.TrimSpace(os.Getenv(compactPrevBinaryEnv))
	if binary == "" {
		t.Skipf("%s is not configured; CI's no-skip guard enforces this suite", compactPrevBinaryEnv)
		return "", dialect, "", false
	}
	dsn := strings.TrimSpace(os.Getenv(dialect.primaryEnv))
	if dsn == "" {
		t.Skipf("%s is not configured; CI's no-skip guard enforces this suite", dialect.primaryEnv)
		return "", dialect, "", false
	}
	return binary, dialect, dsn, true
}

// compactPrevOpenLoadHandle opens a dedicated handle for the concurrent legacy workload. It must
// not share the suite's handle: the claim under test is that an ordinary client's connections keep
// working while the artifact migrates, and a shared pool would let one side's blocked connection
// mask the other's.
// Parameters:
//   - t: test handle used for assertions and cleanup.
//   - dialect: engine descriptor.
//   - dsn: primary DSN in the driver's key/value form.
//
// Return values:
//   - *gorm.DB: an independent handle that never resets the schema.
func compactPrevOpenLoadHandle(t *testing.T, dialect compactLiveDialect, dsn string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(dialect.open(dsn), &gorm.Config{})
	require.NoError(t, err, "open a dedicated legacy workload handle")
	t.Cleanup(func() {
		if pool, err := db.DB(); err == nil {
			_ = pool.Close()
		}
	})
	return db
}

// compactPrevLegacyLoad is an ordinary legacy CRUD workload running on its own connection.
type compactPrevLegacyLoad struct {
	// stopped is closed to ask the workload goroutine to finish.
	stopped chan struct{}
	// finished is closed once the goroutine has returned, which publishes operations and failure
	// to the test goroutine without a data race.
	finished chan struct{}
	// operations counts acknowledged legacy statements.
	operations int
	// failure holds the first statement error or wrong result observed.
	failure error
}

// compactPrevStartLegacyLoad begins ordinary legacy CRUD and returns its running handle. It
// deliberately targets `channels` rather than `users`: the artifact only writes its root account
// when no user exists, so a concurrent users insert would suppress the very write this suite
// exists to observe.
// Parameters:
//   - t: test handle used for cleanup registration.
//   - db: dedicated workload handle.
//
// Return values:
//   - *compactPrevLegacyLoad: running workload; call halt to collect its result.
func compactPrevStartLegacyLoad(t *testing.T, db *gorm.DB) *compactPrevLegacyLoad {
	t.Helper()
	load := &compactPrevLegacyLoad{
		stopped:  make(chan struct{}),
		finished: make(chan struct{}),
	}
	go func() {
		defer close(load.finished)
		for id := compactPrevLoadFirstID; ; id++ {
			select {
			case <-load.stopped:
				return
			default:
			}
			if !load.runCycle(db, id) {
				return
			}
			// Pacing keeps the workload comfortably above the required 10 requests/second
			// without turning the assertion into a benchmark of this machine's disk.
			time.Sleep(2 * time.Millisecond)
		}
	}()
	t.Cleanup(func() { load.halt() })
	return load
}

// runCycle performs one create/read/update/delete round through the legacy text contract.
// Parameters:
//   - db: dedicated workload handle.
//   - id: primary key for this round's row.
//
// Return values:
//   - bool: false when a statement failed or returned a wrong result.
func (load *compactPrevLegacyLoad) runCycle(db *gorm.DB, id int) bool {
	uuid := compactUUIDTextFor(id)
	name := fmt.Sprintf("prev-load-%d", id)

	for _, step := range []struct {
		label string
		exec  func() error
	}{
		{"insert", func() error {
			return db.Exec("INSERT INTO channels (id, uuid, name) VALUES (?, ?, ?)", id, uuid, name).Error
		}},
		{"exact read", func() error {
			var found int64
			if err := db.Raw("SELECT id FROM channels WHERE uuid = ?", uuid).Scan(&found).Error; err != nil {
				return err
			}
			if found != int64(id) {
				return errors.Errorf("returned id %d", found)
			}
			return nil
		}},
		{"update", func() error {
			return db.Exec("UPDATE channels SET name = ? WHERE id = ?", name+"-updated", id).Error
		}},
		{"delete", func() error { return db.Exec("DELETE FROM channels WHERE id = ?", id).Error }},
	} {
		if err := step.exec(); err != nil {
			load.failure = errors.Wrapf(err, "legacy %s of channel %d", step.label, id)
			return false
		}
		load.operations++
	}
	return true
}

// halt stops the workload and waits for its goroutine to publish its result.
// Parameters: none.
//
// Return values: none.
func (load *compactPrevLegacyLoad) halt() {
	select {
	case <-load.stopped:
	default:
		close(load.stopped)
	}
	<-load.finished
}

// compactPrevUserRow is one users row as the engine reports it after the artifact has written.
type compactPrevUserRow struct {
	// ID is the primary key.
	ID int64 `gorm:"column:id"`
	// UUID is the authoritative legacy text, exactly as the engine returns it.
	UUID string `gorm:"column:uuid"`
	// Shadow is the derived native uuid rendered as text, or nil when NULL.
	Shadow *string `gorm:"column:shadow"`
	// Agrees is the engine's own verdict on shadow == normalized text.
	Agrees *bool `gorm:"column:agrees"`
}

// blank reports whether this row's owned UUID is absent. Blankness rather than emptiness is the
// right test: the legacy column is CHAR(36), and PostgreSQL's bpchar pads a shorter stored value
// out to 36 characters on the wire, so an absent UUID arrives as spaces rather than as "".
// Parameters: none.
//
// Return values:
//   - bool: true when the owned UUID is NULL or blank.
func (row compactPrevUserRow) blank() bool {
	return strings.TrimSpace(row.UUID) == ""
}

// compactPrevReadUsers reads every users row with its shadow and the engine's equality verdict.
// Equality is asked of the engine, not recomputed in Go: a Go-side comparison would be this test's
// own emulation of the derivation, which the proposal refuses to accept as evidence.
// Parameters:
//   - t: test handle used for assertions.
//   - db: live PostgreSQL handle.
//
// Return values:
//   - []compactPrevUserRow: rows in ascending primary-key order.
func compactPrevReadUsers(t *testing.T, db *gorm.DB) []compactPrevUserRow {
	t.Helper()
	rows := []compactPrevUserRow{}
	require.NoError(t, db.Raw(
		`SELECT id,
		        uuid,
		        uuid_compact::text AS shadow,
		        (uuid_compact::text = lower(trim(uuid))) AS agrees
		   FROM users ORDER BY id`).Scan(&rows).Error)
	return rows
}

// compactPrevManifestChecksums returns every role's durable legacy-index manifest checksum. There
// is one manifest per registry role in BOTH topologies — a unified deployment carries a single
// marker but still owns the log role's tables through its primary handle. Checking only the primary
// role would miss every logs index, the blind spot that would let this suite report a clean
// rollback over a wedged one.
// Parameters:
//   - t: test handle used for assertions.
//   - ctx: context bounding the reads.
//   - topology: explicitly constructed database topology.
//
// Return values:
//   - map[uuidDBRole]string: stored checksum per owning role.
func compactPrevManifestChecksums(t *testing.T, ctx context.Context, topology *databaseTopology) map[uuidDBRole]string {
	t.Helper()
	checksums := map[uuidDBRole]string{}
	for _, role := range topology.targetRoles() {
		manifest, found, err := readLegacyIndexManifest(ctx, topology.handle(role), role)
		require.NoError(t, err)
		require.True(t, found, "a completed compact migration must have baselined the %s role's legacy indexes", role)
		checksums[role] = manifest.Checksum
	}
	return checksums
}
