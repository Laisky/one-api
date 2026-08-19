package model

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/env"
	"github.com/Laisky/one-api/common/random"
)

// uuidBackfillBatchSize bounds how many target rows one candidate query materializes.
// A catch-up cycle further reduces this to its remaining row budget.
const uuidBackfillBatchSize = 1000

// externalUUIDBackfillFinalizerEnabled controls whether this process may finalize the
// migration. It must be enabled only after every active and rollback-capable writer is
// UUID-aware and old processes and their open transactions are drained. IsMasterNode
// alone is not that barrier.
var externalUUIDBackfillFinalizerEnabled = env.Bool("EXTERNAL_UUID_BACKFILL_FINALIZER", false)

// uuidIntRow carries one target row id from an owned-UUID candidate query.
type uuidIntRow struct {
	Id int `gorm:"column:id"`
}

// uuidRefRow carries one target row id and its observed integer reference.
type uuidRefRow struct {
	Id    int `gorm:"column:id"`
	RefID int `gorm:"column:ref_id"`
}

// uuidNullableRefRow carries one target row id and its observed nullable integer reference.
type uuidNullableRefRow struct {
	Id    int  `gorm:"column:id"`
	RefID *int `gorm:"column:ref_id"`
}

// uuidLogTokenRow carries one log row id and its observed composite token reference.
type uuidLogTokenRow struct {
	Id        int    `gorm:"column:id"`
	UserID    int    `gorm:"column:user_id"`
	TokenName string `gorm:"column:token_name"`
}

// uuidTokenNameRow carries one aggregated token-name resolution result.
type uuidTokenNameRow struct {
	UserID int    `gorm:"column:user_id"`
	Name   string `gorm:"column:name"`
	UUID   string `gorm:"column:uuid"`
	Total  int    `gorm:"column:total"`
}

// uuidCatchUpBudget bounds one catch-up cycle by examined rows and wall time so historical
// reconciliation never sits on the readiness-critical path. The row budget is counted
// globally across every role, phase, table, and column in a cycle, and the remaining budget
// caps the next target query's LIMIT so a cycle cannot overshoot by up to a full batch.
type uuidCatchUpBudget struct {
	maxRows  int
	deadline time.Time
	examined int
	drained  bool
}

// newUUIDCatchUpBudget builds a per-cycle row and time budget.
// The same duration is also installed as a context deadline by the coordinator, so a single
// long-running statement cannot outlive the cycle; this value bounds inter-batch transitions.
// Parameters:
//   - maxRows: maximum target rows the cycle may examine across all phases.
//   - window: maximum wall-clock duration of the cycle.
//
// Return values:
//   - *uuidCatchUpBudget: initialized budget.
func newUUIDCatchUpBudget(maxRows int, window time.Duration) *uuidCatchUpBudget {
	return &uuidCatchUpBudget{maxRows: maxRows, deadline: time.Now().Add(window)}
}

// consume records examined rows and reports whether the cycle budget is now spent.
// Parameters:
//   - rows: number of target rows examined by the last batch.
//
// Return values:
//   - bool: true when the cycle must stop before the next batch.
func (budget *uuidCatchUpBudget) consume(rows int) bool {
	if budget == nil {
		return false
	}
	budget.examined += rows
	return budget.spent()
}

// spent reports whether the cycle budget is exhausted.
// Parameters: none.
//
// Return values:
//   - bool: true when no further batches may run in this cycle.
func (budget *uuidCatchUpBudget) spent() bool {
	if budget == nil {
		return false
	}
	if budget.drained || budget.examined >= budget.maxRows || !time.Now().Before(budget.deadline) {
		budget.drained = true
	}
	return budget.drained
}

// limit reduces a target query's row limit to the remaining cycle budget.
// Without this a cycle could examine up to one full batch beyond its configured ceiling.
// Parameters:
//   - want: the configured batch size the phase would otherwise request.
//
// Return values:
//   - int: rows the next target query may materialize, never above want.
func (budget *uuidCatchUpBudget) limit(want int) int {
	if budget == nil {
		return want
	}
	remaining := budget.maxRows - budget.examined
	if remaining < 1 {
		return 1
	}
	if remaining < want {
		return remaining
	}
	return want
}

// uuidMigrationRun carries the coordinator state shared by every phase of one invocation.
type uuidMigrationRun struct {
	topology *databaseTopology
	mode     uuidMigrationMode
	// budget bounds catch-up cycles; nil means unbounded, as finalizer mode requires.
	budget *uuidCatchUpBudget
	// cycleCtx is the reconciliation context carrying this cycle's time budget. It is nil in
	// finalizer mode. Comparing against this exact context is what distinguishes the cycle's
	// own budget expiring from an unrelated deadline, such as a DDL statement timeout.
	cycleCtx context.Context
	// updated counts rows written across all phases of this run.
	updated int
}

// cycleWindowExpired reports whether an error is just this catch-up cycle's time budget
// expiring rather than a real failure.
//
// The cycle deadline is a context deadline so it interrupts an in-flight statement, but
// reaching it is the normal end of a bounded cycle, not an error: the worker simply
// reschedules and resumes from the same durable state. A cancellation of the caller's own
// context is a different thing entirely and must still surface as an error, which is why the
// parent context is checked separately. Finalizer mode has no budget, so this never applies
// there and a cancelled finalizer always fails.
// Parameters:
//   - parentCtx: the caller's context, excluding this cycle's deadline.
//   - err: error returned by a phase.
//
// Return values:
//   - bool: true when the cycle should end gracefully with work remaining.
func (run *uuidMigrationRun) cycleWindowExpired(parentCtx context.Context, err error) bool {
	if run.budget == nil || run.cycleCtx == nil || parentCtx.Err() != nil {
		return false
	}
	// Match on THIS cycle's context, not on the DeadlineExceeded sentinel. A nested deadline
	// — most importantly a DDL statement timeout, which UUID-042 requires to surface as a
	// retryable failure — also reports DeadlineExceeded, and pattern-matching the sentinel
	// would silently convert that operator-configured bound into "the cycle ran out of time".
	if run.cycleCtx.Err() == nil || !errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	run.budget.drained = true
	return true
}

// uuidMigrationResult reports what one coordinator invocation observed.
type uuidMigrationResult struct {
	// completed is true when every applicable current-generation marker exists.
	completed bool
	// updated is the number of rows written by this run.
	updated int
	// budgetExhausted is true when a catch-up cycle stopped early with work remaining.
	budgetExhausted bool
}

// RunExternalUUIDMigrations reconciles external UUID data across the initialized topology.
// It is the supported completion-capable entry point: the primary-only compatibility
// wrapper used by InitDB cannot finalize split-database state. Finalizer mode is selected
// by EXTERNAL_UUID_BACKFILL_FINALIZER and requires the UUID-aware-writer barrier.
// Parameters:
//   - ctx: context controlling bounded migration reads and writes.
//
// Return values:
//   - error: wrapped migration error when topology, reconciliation, DDL, validation, or
//     marker writes fail.
func RunExternalUUIDMigrations(ctx context.Context) error {
	topology := databaseTopologySnapshot()
	if topology == nil {
		return errors.New("external uuid migration requires an initialized database topology")
	}
	mode := uuidMigrationModeCatchUp
	if externalUUIDBackfillFinalizerEnabled {
		mode = uuidMigrationModeFinalizer
	}
	_, err := runUUIDMigrationCoordinator(ctx, topology, mode)
	return err
}

// runUUIDMigrationCoordinator is the only component allowed to write completion markers.
// It validates inputs, short-circuits on complete markers, runs the topology-specific
// phase sequence, and finalizes only after global validation succeeds.
// Parameters:
//   - ctx: context controlling every bounded read, write, and DDL statement.
//   - topology: explicitly constructed database topology.
//   - mode: catch-up or finalizer coordinator mode.
//
// Return values:
//   - uuidMigrationResult: observed completion, write, and budget state.
//   - error: wrapped error when any phase, validation, or marker operation fails.
func runUUIDMigrationCoordinator(ctx context.Context, topology *databaseTopology, mode uuidMigrationMode) (uuidMigrationResult, error) {
	result := uuidMigrationResult{}
	if ctx == nil {
		ctx = context.Background()
	}
	// Step 1 validates mode, handles, and topology without issuing a metadata query, so a
	// completed invocation can prove it performed nothing but its marker lookups.
	if err := validateUUIDMigrationMode(mode); err != nil {
		return result, err
	}
	if err := topology.validate(); err != nil {
		return result, errors.Wrap(err, "validate database topology")
	}

	// Step 2 and 3: read one marker per applicable role and return immediately when every
	// applicable marker exists.
	markers, err := readUUIDMarkerState(ctx, topology)
	if err != nil {
		return result, err
	}
	if markers.allPresent() {
		result.completed = true
		return result, nil
	}

	// Step 4: only now, on the incomplete-marker path, is metadata inspection allowed.
	if err := topology.validateSchema(ctx); err != nil {
		return result, errors.Wrap(err, "validate database schema")
	}

	parentCtx := ctx
	run := &uuidMigrationRun{topology: topology, mode: mode}
	if mode == uuidMigrationModeCatchUp {
		// The row half of the cycle budget is counted globally across phases; the time half
		// is a context deadline so every query, update, and inter-batch transition observes
		// it. Finalizer mode deliberately gets neither: it must never stop early and then
		// write a marker.
		window := uuidCatchUpTimeBudget()
		run.budget = newUUIDCatchUpBudget(uuidCatchUpRowBudget(), window)
		cycleCtx, cancelCycle := context.WithTimeout(ctx, window)
		defer cancelCycle()
		run.cycleCtx = cycleCtx
		ctx = cycleCtx
	}

	started := time.Now()
	log := uuidMigrationLogger(ctx)
	log.Info("external uuid reconciliation started",
		zap.String("topology", string(topology.mode)),
		zap.String("mode", string(mode)))

	// Candidate indexes come before reconciliation so every NULL and empty-string pass is
	// served by an index instead of degrading into an unindexed historical scan.
	//
	// This phase deliberately runs on the PARENT context, not the cycle context. The cycle
	// budget bounds how many target ROWS one pass examines; index DDL examines none, and
	// building an index on a large table legitimately takes longer than one cycle window.
	// Sharing the cycle deadline would cap every build at the window (a context deadline can
	// only shorten, never extend), make EXTERNAL_UUID_BACKFILL_DDL_TIMEOUT unreachable, and
	// on PostgreSQL leave a cancelled concurrent build behind as an invalid index on every
	// attempt. The DDL has its own bounded lock and statement timeouts.
	if err := ensureUUIDCandidateIndexes(parentCtx, run); err != nil {
		// A failed index phase stops the cycle and records the error; it never falls through
		// to reconcile against a missing index.
		recordUUIDCycle(topology, mode, uuidResultFailure, time.Since(started))
		return result, err
	}
	if err := runUUIDReconciliationPhases(ctx, run); err != nil {
		if !run.cycleWindowExpired(parentCtx, err) {
			recordUUIDCycle(topology, mode, uuidResultFailure, time.Since(started))
			return result, err
		}
	}

	result.updated = run.updated
	result.budgetExhausted = run.budget.spent()

	if mode == uuidMigrationModeCatchUp {
		recordUUIDCycle(topology, mode, uuidResultSuccess, time.Since(started))
		recordUUIDCatchUpBacklog(topology, result)
		// One INFO event per cycle; per-batch detail stays at DEBUG.
		log.Info("external uuid catch-up cycle finished without markers",
			zap.String("topology", string(topology.mode)),
			zap.Int("updated_rows", result.updated),
			zap.Bool("budget_exhausted", result.budgetExhausted),
			zap.Duration("duration", time.Since(started)))
		return result, nil
	}

	for _, step := range []struct {
		name string
		run  func() error
	}{
		{name: "promote unique indexes", run: func() error { return promoteUUIDUniqueIndexes(ctx, topology) }},
		{name: "global validation", run: func() error { return validateExternalUUIDs(ctx, topology) }},
		{name: "write completion markers", run: func() error { return writeUUIDCompletionMarkers(ctx, topology, markers) }},
	} {
		if err := step.run(); err != nil {
			recordUUIDFinalizerResult(topology, false)
			recordUUIDCycle(topology, mode, uuidResultFailure, time.Since(started))
			return result, err
		}
		log.Info("external uuid finalizer phase completed",
			zap.String("topology", string(topology.mode)),
			zap.String("phase", step.name),
			zap.Duration("duration", time.Since(started)))
	}

	recordUUIDFinalizerResult(topology, true)
	recordUUIDCycle(topology, mode, uuidResultSuccess, time.Since(started))
	result.completed = true
	log.Info("external uuid finalization completed",
		zap.String("topology", string(topology.mode)),
		zap.Int("updated_rows", result.updated),
		zap.Duration("duration", time.Since(started)))
	return result, nil
}

// runUUIDReconciliationPhases executes the dependency-correct phase sequence for the topology.
// Owned UUIDs are always populated before any reference to them is resolved: primary owners,
// primary-local references, authoritative log owners, log references to primary owners, and
// finally primary references to authoritative log owners.
// Parameters:
//   - ctx: context controlling bounded reads and writes.
//   - run: coordinator state for this invocation.
//
// Return values:
//   - error: wrapped error when any phase fails.
func runUUIDReconciliationPhases(ctx context.Context, run *uuidMigrationRun) error {
	// Phases are looked up by name, so reordering the registry cannot silently pair a phase
	// with the wrong dependency step or make an error message blame the wrong phase.
	fkPhases, err := uuidFKPhasesByName()
	if err != nil {
		return err
	}

	steps := []struct {
		name string
		run  func() error
	}{
		{name: "primary owned uuids", run: func() error {
			return backfillOwnedUUIDsForRole(ctx, run, uuidRolePrimary)
		}},
		{name: uuidFKPhasePrimaryLocal, run: func() error {
			return backfillFKUUIDsForPhase(ctx, run, fkPhases[uuidFKPhasePrimaryLocal])
		}},
		{name: "authoritative log owned uuids", run: func() error {
			return backfillOwnedUUIDsForRole(ctx, run, uuidRoleLog)
		}},
		{name: uuidFKPhaseLogFromPrimary, run: func() error {
			return backfillFKUUIDsForPhase(ctx, run, fkPhases[uuidFKPhaseLogFromPrimary])
		}},
		{name: uuidFKPhasePrimaryFromLog, run: func() error {
			return backfillFKUUIDsForPhase(ctx, run, fkPhases[uuidFKPhasePrimaryFromLog])
		}},
	}
	for _, step := range steps {
		if err := step.run(); err != nil {
			return errors.Wrapf(err, "backfill %s", step.name)
		}
	}
	return nil
}

// uuidFKPhasesByName indexes the ordered FK phases by name and rejects an incomplete set.
// Parameters: none.
//
// Return values:
//   - map[string]uuidFKPhase: every FK phase keyed by name.
//   - error: wrapped error when a required phase is missing from the registry.
func uuidFKPhasesByName() (map[string]uuidFKPhase, error) {
	phases := map[string]uuidFKPhase{}
	for _, phase := range uuidFKPhaseOrder() {
		phases[phase.name] = phase
	}
	for _, required := range []string{uuidFKPhasePrimaryLocal, uuidFKPhaseLogFromPrimary, uuidFKPhasePrimaryFromLog} {
		if _, ok := phases[required]; !ok {
			return nil, errors.Errorf("fk phase %q is missing from the registry", required)
		}
	}
	return phases, nil
}

// backfillOwnedUUIDsForRole fills every missing owned UUID on one authoritative database.
// Parameters:
//   - ctx: context controlling bounded reads and writes.
//   - run: coordinator state for this invocation.
//   - role: database role whose owned tables are reconciled.
//
// Return values:
//   - error: wrapped error when a read or write fails.
func backfillOwnedUUIDsForRole(ctx context.Context, run *uuidMigrationRun, role uuidDBRole) error {
	db := run.topology.handle(role)
	for _, target := range ownedTargetsForRole(role) {
		if err := backfillOwnedUUIDs(ctx, run, db, target); err != nil {
			return errors.Wrapf(err, "backfill owned uuid for %s", target.table)
		}
	}
	return nil
}

// backfillOwnedUUIDs fills missing owned UUID values for a single table.
// Missing owned UUIDs are always fillable, so the UUID column itself is the durable work
// queue: a successful conditional update removes the row from the next candidate query.
// Parameters:
//   - ctx: context controlling bounded reads and writes.
//   - run: coordinator state for this invocation.
//   - db: authoritative database handle for the target table.
//   - target: owned UUID target metadata.
//
// Return values:
//   - error: wrapped database error when a read or write fails.
func backfillOwnedUUIDs(ctx context.Context, run *uuidMigrationRun, db *gorm.DB, target uuidOwnedTarget) error {
	if !db.Migrator().HasTable(target.model) || !db.Migrator().HasColumn(target.model, "uuid") {
		return nil
	}

	for _, missingPredicate := range missingStringPredicates(db, "uuid") {
		lastID := 0
		for {
			if run.budget.spent() {
				return nil
			}
			rows := []uuidIntRow{}
			err := db.WithContext(ctx).
				Table(target.table).
				Select("id").
				Where(quoteIdentifier(db, "id")+" > ? AND "+missingPredicate, lastID).
				Order(quoteIdentifier(db, "id") + " ASC").
				Limit(run.budget.limit(uuidBackfillBatchSize)).
				Find(&rows).Error
			if err != nil {
				return errors.Wrapf(err, "list missing uuid rows for %s", target.table)
			}
			if len(rows) == 0 {
				break
			}
			// Advance across every examined row so a row that loses the update race
			// cannot pin the batch.
			lastID = rows[len(rows)-1].Id

			values := make([]uuidConditionalValue, 0, len(rows))
			for _, row := range rows {
				values = append(values, uuidConditionalValue{id: row.Id, value: random.GetUUIDWithHyphens()})
			}
			updated, err := applyConditionalStringColumnRows(ctx, db, target.table, "uuid", values)
			if err != nil {
				return errors.Wrapf(err, "set uuid for %s", target.table)
			}
			run.updated += updated
			recordUUIDBatch(ctx, run, target.role, uuidPhaseOwned, target.table, "uuid", len(rows), updated, 0)
			run.budget.consume(len(rows))
		}
	}
	return nil
}

// recordUUIDBatch emits one bounded structured progress event and the row metrics for a batch.
// It reports aggregate counts only and never logs DSNs, credentials, token keys, UUID
// values, or row content. Per-batch detail is DEBUG; the coordinator emits one INFO per
// cycle or finalizer phase.
// Parameters:
//   - ctx: context supplying the scoped logger.
//   - run: coordinator state supplying topology and mode.
//   - role: authoritative database role for the target.
//   - phase: registry phase name.
//   - table: trusted target table name.
//   - column: trusted target column name.
//   - examined: rows examined by the candidate query.
//   - updated: rows written by the conditional update.
//   - ambiguous: ambiguous composite keys observed in the batch.
//
// Return values: none.
func recordUUIDBatch(ctx context.Context, run *uuidMigrationRun, role uuidDBRole, phase string, table string, column string, examined int, updated int, ambiguous int) {
	unresolved := examined - updated
	uuidMigrationLogger(ctx).Debug("external uuid batch reconciled",
		zap.String("topology", string(run.topology.mode)),
		zap.String("mode", string(run.mode)),
		zap.String("phase", phase),
		zap.String("table", table),
		zap.String("column", column),
		zap.Int("examined_rows", examined),
		zap.Int("updated_rows", updated),
		zap.Int("unresolved_rows", unresolved),
		zap.Int("ambiguous_keys", ambiguous))
	// The metric target label is the registry's table.column, never a row value.
	recordUUIDRows(role, phase, table+"."+column, updated, unresolved)
}

// missingStringPredicates returns separate indexed predicates for NULL and empty string values.
// Two passes keep each predicate sargable against the ordinary UUID index instead of forcing
// an OR that many planners refuse to index.
// Parameters:
//   - db: database handle whose dialect controls identifier quoting.
//   - column: trusted target string column name.
//
// Return values:
//   - []string: predicates for the NULL pass and the empty-string pass.
func missingStringPredicates(db *gorm.DB, column string) []string {
	quoted := quoteIdentifier(db, column)
	return []string{quoted + " IS NULL", quoted + " = ''"}
}
