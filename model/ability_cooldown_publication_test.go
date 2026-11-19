package model

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Reproductions for the PR #629 CodeRabbit review of cooldown publication:
//
//   - A committed cooldown must reach this process's cached routing even when
//     the read that follows the write fails (review comment on
//     model/ability_cooldown.go:50).
//   - A cooldown writer must not wait for database I/O issued for another
//     ability, for the cooldown mirror, or for a cache rebuild (review comment
//     on model/ability_cooldown.go:105), while a rebuild or mirror that read the
//     table before the write still must not drop the writer's publication.

// publicationWriterBudget is how long an unrelated cooldown writer may take
// while another statement is stalled. It only needs to absorb scheduling noise:
// a writer that waits for the stalled statement never finishes inside it.
const publicationWriterBudget = 2 * time.Second

// cooldownStallKey marks a context whose abilities statements stall.
type cooldownStallKey struct{}

// stallMarkedAbilityStatements makes every abilities statement issued with the
// returned context block after it executed, until release is called. The
// statement has already returned its connection to the pool, so the stall holds
// no database resource: anything that waits for it waits on a Go lock.
//
// Parameters:
//   - t: the test handle; cleanup releases the stall.
//   - db: the database whose callbacks are instrumented.
//
// Returns:
//   - context.Context: the marked context.
//   - <-chan struct{}: receives once per stalled statement.
//   - func(): the idempotent release.
func stallMarkedAbilityStatements(t *testing.T, db *gorm.DB) (context.Context, <-chan struct{}, func()) {
	t.Helper()
	gate := make(chan struct{})
	entered := make(chan struct{}, 64)
	var once sync.Once
	release := func() { once.Do(func() { close(gate) }) }
	t.Cleanup(release)
	stall := func(tx *gorm.DB) {
		ctx := tx.Statement.Context
		if ctx == nil || ctx.Value(cooldownStallKey{}) == nil || !strings.Contains(tx.Statement.SQL.String(), "abilities") {
			return
		}
		select {
		case entered <- struct{}{}:
		default:
		}
		<-gate
	}
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:stall_marked_query", stall))
	require.NoError(t, db.Callback().Update().After("gorm:update").Register("test:stall_marked_update", stall))
	return context.WithValue(context.Background(), cooldownStallKey{}, true), entered, release
}

// failAbilityReadsAfterCommit simulates a connection lost right after a
// cooldown write committed: once armed, the first abilities UPDATE that
// changes a row makes every later abilities read fail until disarmed.
//
// Parameters:
//   - t: the test handle.
//   - db: the database whose callbacks are instrumented.
//
// Returns:
//   - func(): arms the injection.
//   - func(): disarms it, so the test can inspect the committed state.
//   - *atomic.Bool: true once a write committed while armed.
func failAbilityReadsAfterCommit(t *testing.T, db *gorm.DB) (func(), func(), *atomic.Bool) {
	t.Helper()
	var armed, committed atomic.Bool
	require.NoError(t, db.Callback().Update().After("gorm:update").Register("test:observe_cooldown_commit", func(tx *gorm.DB) {
		if armed.Load() && tx.Error == nil && tx.RowsAffected > 0 && tx.Statement.Table == "abilities" {
			committed.Store(true)
		}
	}))
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:fail_read_after_commit", func(tx *gorm.DB) {
		if armed.Load() && committed.Load() && tx.Statement.Table == "abilities" {
			_ = tx.AddError(errors.New("injected: connection lost after the cooldown write committed"))
		}
	}))
	return func() { armed.Store(true) }, func() { armed.Store(false) }, &committed
}

// rejectAbilityWrites makes every abilities UPDATE fail before it executes
// while armed, so nothing is committed.
//
// Parameters:
//   - t: the test handle.
//   - db: the database whose callbacks are instrumented.
//
// Returns:
//   - *atomic.Bool: the arming switch.
func rejectAbilityWrites(t *testing.T, db *gorm.DB) *atomic.Bool {
	t.Helper()
	var armed atomic.Bool
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:reject_cooldown_write", func(tx *gorm.DB) {
		if armed.Load() && tx.Statement.Table == "abilities" {
			_ = tx.AddError(errors.New("injected: cooldown write rejected"))
		}
	}))
	return &armed
}

// persistedPublicationAbility reads the committed cooldown columns of one
// default/target ability.
//
// Parameters:
//   - t: the test handle.
//   - db: the database.
//   - channelID: the ability's channel.
//
// Returns:
//   - Ability: the persisted row.
func persistedPublicationAbility(t *testing.T, db *gorm.DB, channelID int) Ability {
	t.Helper()
	var row Ability
	require.NoError(t, exactCooldownAbility(db, cachedAbilityKey{"default", "target", channelID}).Take(&row).Error)
	return row
}

// requireCachedCooling asserts what this process's cached routing reports for
// one default/target ability.
//
// Parameters:
//   - t: the test handle.
//   - channelID: the ability's channel.
//   - want: whether the ability must be cooling.
//   - msg: the failure explanation.
func requireCachedCooling(t *testing.T, channelID int, want bool, msg string) {
	t.Helper()
	cooling, err := AbilityCoolingDown(context.Background(), "default", "target", channelID)
	require.NoError(t, err)
	require.Equal(t, want, cooling, msg)
}

// runWithinBudget runs fn on its own goroutine and fails the test if it does
// not return within publicationWriterBudget. wg tracks the goroutine so the
// caller can wait for it after releasing any stall.
//
// Parameters:
//   - t: the test handle.
//   - wg: tracks the goroutine.
//   - what: names the operation in the failure message.
//   - fn: the operation.
//
// Returns:
//   - error: the operation's error.
func runWithinBudget(t *testing.T, wg *sync.WaitGroup, what string, fn func() error) error {
	t.Helper()
	done := make(chan error, 1)
	wg.Go(func() { done <- fn() })
	select {
	case err := <-done:
		return err
	case <-time.After(publicationWriterBudget):
		require.FailNow(t, what+" waited for database I/O issued for something else")
		return nil
	}
}

// TestSuspendAbilityPublishesCommittedDeadlineWhenReadBackFails reproduces the
// first review comment on the fixed path: the suspension commits, the read
// that follows fails, and routing must still avoid the ability. The fixed path
// has no provisional overlay, so before the fix the ability stays routable on
// this process until the next mirror refresh or rebuild.
func TestSuspendAbilityPublishesCommittedDeadlineWhenReadBackFails(t *testing.T) {
	db := setupAbilityCacheTest(t, abilityCacheChannel(1, 10), abilityCacheChannel(2, 10))
	arm, disarm, committed := failAbilityReadsAfterCommit(t, db)

	arm()
	err := SuspendAbility(context.Background(), "default", "target", 1, time.Hour)
	disarm()

	require.True(t, committed.Load(), "the suspension write must have committed")
	row := persistedPublicationAbility(t, db, 1)
	require.NotNil(t, row.SuspendUntil)
	require.True(t, row.SuspendUntil.After(time.Now().UTC().Add(59*time.Minute)))
	requireCachedCooling(t, 1, true, "a committed suspension must reach cached routing even when the read after the write fails")
	require.NoError(t, err, "the suspension is durable and published, so the caller must not see a failure")
}

// TestRecordAbilityCooldownFailurePublishesCommittedDeadlineWhenReadBackFails
// reproduces the first review comment on the adaptive path: the CAS commits
// the first rung, the read that follows fails, and routing must still avoid
// the ability without relying on the controller's provisional overlay.
func TestRecordAbilityCooldownFailurePublishesCommittedDeadlineWhenReadBackFails(t *testing.T) {
	db := setupAbilityCacheTest(t, abilityCacheChannel(1, 10), abilityCacheChannel(2, 10))
	snapshotProvisionalAbilityCooldowns(t)
	arm, disarm, committed := failAbilityReadsAfterCommit(t, db)

	completed := time.Now().UTC()
	arm()
	duration, err := RecordAbilityCooldownFailure(context.Background(), "default", "target", 1, completed.Add(-time.Millisecond), completed)
	disarm()

	require.True(t, committed.Load(), "the cooldown write must have committed")
	require.Equal(t, abilityCooldownLadder[0], duration)
	row := persistedPublicationAbility(t, db, 1)
	require.NotNil(t, row.AdaptiveSuspendUntil)
	requireCachedCooling(t, 1, true, "a committed adaptive cooldown must reach cached routing even when the read after the write fails")
	require.NoError(t, err, "the cooldown is durable and published, so the caller must not see a failure")
}

// TestCooldownPublicationKeepsLongerFixedDeadline guards the fix for the first
// review comment: publishing the adaptive deadline a write committed must not
// shorten a longer fixed cooldown that is already in force.
func TestCooldownPublicationKeepsLongerFixedDeadline(t *testing.T) {
	db := setupAbilityCacheTest(t, abilityCacheChannel(1, 10), abilityCacheChannel(2, 10))
	snapshotProvisionalAbilityCooldowns(t)
	require.NoError(t, SuspendAbility(context.Background(), "default", "target", 1, time.Hour))
	arm, disarm, _ := failAbilityReadsAfterCommit(t, db)

	completed := time.Now().UTC()
	arm()
	_, _ = RecordAbilityCooldownFailure(context.Background(), "default", "target", 1, completed.Add(-time.Millisecond), completed)
	disarm()

	channelSyncLock.RLock()
	stillCooling := cachedAbilityCoolingLocked(cachedAbilityKey{"default", "target", 1}, time.Now().UTC().Add(30*time.Minute))
	channelSyncLock.RUnlock()
	require.True(t, stillCooling, "a 10s adaptive rung must not replace a one-hour fixed cooldown in the cache")
}

// TestCooldownPublicationSkipsRejectedWrites is the control for the first
// review comment: when the database rejects the write, nothing may be
// published, on either path.
func TestCooldownPublicationSkipsRejectedWrites(t *testing.T) {
	db := setupAbilityCacheTest(t, abilityCacheChannel(1, 10), abilityCacheChannel(2, 10))
	snapshotProvisionalAbilityCooldowns(t)
	reject := rejectAbilityWrites(t, db)

	reject.Store(true)
	require.Error(t, SuspendAbility(context.Background(), "default", "target", 1, time.Hour))
	completed := time.Now().UTC()
	_, err := RecordAbilityCooldownFailure(context.Background(), "default", "target", 1, completed.Add(-time.Millisecond), completed)
	require.Error(t, err)
	reject.Store(false)

	row := persistedPublicationAbility(t, db, 1)
	require.Nil(t, row.SuspendUntil)
	require.Nil(t, row.AdaptiveSuspendUntil)
	requireCachedCooling(t, 1, false, "the cache must never claim a cooldown the database rejected")
}

// TestCooldownStallHarnessHoldsNoConnection is the false-positive control for
// the second review comment: a statement stalled by the harness has released
// its connection, so a plain query on the single test connection still runs.
// Any writer that waits in the tests below therefore waits on a Go lock.
func TestCooldownStallHarnessHoldsNoConnection(t *testing.T) {
	db := setupAbilityCacheTest(t, abilityCacheChannel(1, 10), abilityCacheChannel(2, 10))
	stalled, entered, release := stallMarkedAbilityStatements(t, db)
	var wg sync.WaitGroup
	defer wg.Wait()
	defer release()

	completed := time.Now().UTC()
	wg.Go(func() {
		_, _ = RecordAbilityCooldownFailure(stalled, "default", "target", 1, completed.Add(-time.Millisecond), completed)
	})
	<-entered

	require.NoError(t, runWithinBudget(t, &wg, "a plain abilities query", func() error {
		var count int64
		return db.Model(&Ability{}).Count(&count).Error
	}))
}

// TestCooldownWritersDoNotWaitForOtherAbilities reproduces the second review
// comment for writers: while one ability's cooldown write is slow, cooldowns of
// other abilities, on either path, must persist and reach routing on time.
func TestCooldownWritersDoNotWaitForOtherAbilities(t *testing.T) {
	db := setupAbilityCacheTest(t, abilityCacheChannel(1, 10), abilityCacheChannel(2, 10), abilityCacheChannel(3, 10))
	snapshotProvisionalAbilityCooldowns(t)
	stalled, entered, release := stallMarkedAbilityStatements(t, db)
	var wg sync.WaitGroup
	defer wg.Wait()
	defer release()

	completed := time.Now().UTC()
	wg.Go(func() {
		_, _ = RecordAbilityCooldownFailure(stalled, "default", "target", 1, completed.Add(-time.Millisecond), completed)
	})
	<-entered

	require.NoError(t, runWithinBudget(t, &wg, "an adaptive cooldown of another ability", func() error {
		_, err := RecordAbilityCooldownFailure(context.Background(), "default", "target", 2, completed.Add(-time.Millisecond), completed)
		return err
	}))
	requireCachedCooling(t, 2, true, "the unrelated adaptive cooldown must be routable state at once")
	require.NoError(t, runWithinBudget(t, &wg, "a fixed suspension of another ability", func() error {
		return SuspendAbility(context.Background(), "default", "target", 3, time.Hour)
	}))
	requireCachedCooling(t, 3, true, "the unrelated fixed suspension must be routable state at once")
}

// TestCooldownRecoveryDoesNotWaitForOtherAbilities reproduces the second review
// comment for the recovery slow path: clearing a cooled ability must not wait
// for another ability's slow write.
func TestCooldownRecoveryDoesNotWaitForOtherAbilities(t *testing.T) {
	db := setupAbilityCacheTest(t, abilityCacheChannel(1, 10), abilityCacheChannel(2, 10))
	snapshotProvisionalAbilityCooldowns(t)
	failed := time.Now().UTC()
	_, err := RecordAbilityCooldownFailure(context.Background(), "default", "target", 2, failed.Add(-time.Millisecond), failed)
	require.NoError(t, err)
	requireCachedCooling(t, 2, true, "fixture: channel 2 must be cooling")

	stalled, entered, release := stallMarkedAbilityStatements(t, db)
	var wg sync.WaitGroup
	defer wg.Wait()
	defer release()
	wg.Go(func() {
		_, _ = RecordAbilityCooldownFailure(stalled, "default", "target", 1, failed.Add(-time.Millisecond), failed)
	})
	<-entered

	recovered := time.Now().UTC().Add(time.Millisecond)
	require.NoError(t, runWithinBudget(t, &wg, "the recovery of another ability", func() error {
		return ResetAbilityCooldown(context.Background(), "default", "target", 2, recovered.Add(-time.Millisecond), recovered)
	}))
	requireCachedCooling(t, 2, false, "the recovered ability must be routable at once")
}

// TestCooldownWritersDoNotWaitForSnapshotReads reproduces the second review
// comment for the background readers: a slow mirror refresh or cache rebuild
// must not hold cooldown writers back, and the snapshot they read before the
// write must not drop the writer's publication when it is swapped in.
func TestCooldownWritersDoNotWaitForSnapshotReads(t *testing.T) {
	for _, reader := range []struct {
		name string
		run  func(context.Context) error
	}{
		{"mirror", refreshCachedAbilitySuspensionsContext},
		{"rebuild", InitChannelCacheContext},
	} {
		t.Run(reader.name, func(t *testing.T) {
			db := setupAbilityCacheTest(t, abilityCacheChannel(1, 10), abilityCacheChannel(2, 10))
			snapshotProvisionalAbilityCooldowns(t)
			stalled, entered, release := stallMarkedAbilityStatements(t, db)
			var wg sync.WaitGroup
			defer wg.Wait()
			defer release()

			readerDone := make(chan error, 1)
			wg.Go(func() { readerDone <- reader.run(stalled) })
			<-entered // the snapshot has been read and is not yet published

			completed := time.Now().UTC()
			require.NoError(t, runWithinBudget(t, &wg, "an adaptive cooldown during a "+reader.name, func() error {
				_, err := RecordAbilityCooldownFailure(context.Background(), "default", "target", 1, completed.Add(-time.Millisecond), completed)
				return err
			}))
			require.NoError(t, runWithinBudget(t, &wg, "a fixed suspension during a "+reader.name, func() error {
				return SuspendAbility(context.Background(), "default", "target", 2, time.Hour)
			}))

			release()
			require.NoError(t, <-readerDone)
			requireCachedCooling(t, 1, true, "the "+reader.name+" must not drop an adaptive cooldown published after its read")
			requireCachedCooling(t, 2, true, "the "+reader.name+" must not drop a fixed suspension published after its read")
		})
	}
}

// TestCooldownPublicationsApplyInCommitOrder guards any lock-free fix of the
// second review comment: a failure whose publication is delayed past a later
// recovery of the same ability must not reopen the cooldown in the cache. The
// failure commits first and stalls before publishing; the recovery commits
// and publishes; then the failure publishes.
func TestCooldownPublicationsApplyInCommitOrder(t *testing.T) {
	db := setupAbilityCacheTest(t, abilityCacheChannel(1, 10), abilityCacheChannel(2, 10))
	snapshotProvisionalAbilityCooldowns(t)
	var stallAfterCommit atomic.Bool
	gate, committed := make(chan struct{}), make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(gate) }) }
	t.Cleanup(release)
	require.NoError(t, db.Callback().Update().After("gorm:update").Register("test:stall_failure_publication", func(tx *gorm.DB) {
		if tx.Error != nil || tx.RowsAffected == 0 || !strings.Contains(tx.Statement.SQL.String(), "abilities") {
			return
		}
		if tx.Statement.Context.Value(cooldownStallKey{}) == nil || !stallAfterCommit.CompareAndSwap(true, false) {
			return
		}
		close(committed)
		<-gate
	}))

	var wg sync.WaitGroup
	defer wg.Wait()
	defer release()
	failed := time.Now().UTC()
	stallAfterCommit.Store(true)
	wg.Go(func() {
		_, _ = RecordAbilityCooldownFailure(context.WithValue(context.Background(), cooldownStallKey{}, true),
			"default", "target", 1, failed.Add(-time.Millisecond), failed)
	})
	<-committed

	recovered := failed.Add(time.Millisecond)
	recoveryDone := make(chan error, 1)
	wg.Go(func() {
		recoveryDone <- ResetAbilityCooldown(context.Background(), "default", "target", 1, recovered.Add(-time.Millisecond), recovered)
	})
	// Let a lock-free recovery commit and publish before the failure resumes.
	select {
	case err := <-recoveryDone:
		recoveryDone <- err
	case <-time.After(200 * time.Millisecond):
	}
	release()
	require.NoError(t, <-recoveryDone)
	wg.Wait()

	row := persistedPublicationAbility(t, db, 1)
	require.Nil(t, row.AdaptiveSuspendUntil, "the recovery completed last and must win in the database")
	requireCachedCooling(t, 1, false, "a delayed publication of an older failure must not reopen a recovered ability")
}

// stallOnceKey marks a context for stallFirstMarkedStatement.
type stallOnceKey struct{ name string }

// stallFirstMarkedStatement blocks the first abilities statement of the given
// kind ("query" or "update") issued with a context marked by name, after it
// executed, until release is called.
//
// Parameters:
//   - t: the test handle; cleanup releases the stall.
//   - db: the database whose callbacks are instrumented.
//   - kind: "query" or "update".
//   - name: the marker name; it also names the callback.
//
// Returns:
//   - context.Context: the marked context.
//   - <-chan struct{}: closed once the statement stalls.
//   - func(): the idempotent release.
func stallFirstMarkedStatement(t *testing.T, db *gorm.DB, kind, name string) (context.Context, <-chan struct{}, func()) {
	t.Helper()
	marker := stallOnceKey{name}
	gate, entered := make(chan struct{}), make(chan struct{})
	var armed atomic.Bool
	armed.Store(true)
	var once sync.Once
	release := func() { once.Do(func() { close(gate) }) }
	t.Cleanup(release)
	stall := func(tx *gorm.DB) {
		ctx := tx.Statement.Context
		if tx.Error != nil || ctx == nil || ctx.Value(marker) == nil || !strings.Contains(tx.Statement.SQL.String(), "abilities") {
			return
		}
		if !armed.CompareAndSwap(true, false) {
			return
		}
		close(entered)
		<-gate
	}
	if kind == "query" {
		require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:stall_once_"+name, stall))
	} else {
		require.NoError(t, db.Callback().Update().After("gorm:update").Register("test:stall_once_"+name, stall))
	}
	return context.WithValue(context.Background(), marker, true), entered, release
}

// TestCooldownFixedSuspensionInsideAdaptiveWindowSurvives guards the revision
// fence the lock-free publication relies on: a fixed suspension committed
// between an adaptive failure's read and its write must make that write retry,
// so neither the database nor the cache loses the longer fixed cooldown, even
// when the adaptive failure publishes before the suspension does.
func TestCooldownFixedSuspensionInsideAdaptiveWindowSurvives(t *testing.T) {
	db := setupAbilityCacheTest(t, abilityCacheChannel(1, 10), abilityCacheChannel(2, 10))
	snapshotProvisionalAbilityCooldowns(t)
	adaptiveCtx, adaptiveRead, releaseAdaptive := stallFirstMarkedStatement(t, db, "query", "adaptive")
	fixedCtx, fixedWritten, releaseFixed := stallFirstMarkedStatement(t, db, "update", "fixed")
	var wg sync.WaitGroup
	defer wg.Wait()
	defer releaseAdaptive()
	defer releaseFixed()

	failed := time.Now().UTC()
	adaptiveDone := make(chan error, 1)
	wg.Go(func() {
		_, err := RecordAbilityCooldownFailure(adaptiveCtx, "default", "target", 1, failed.Add(-time.Millisecond), failed)
		adaptiveDone <- err
	})
	<-adaptiveRead // the failure read the row before any fixed suspension
	fixedDone := make(chan error, 1)
	wg.Go(func() { fixedDone <- SuspendAbility(fixedCtx, "default", "target", 1, time.Hour) })
	select {
	case <-fixedWritten: // the fixed suspension committed and has not published yet
	case <-time.After(publicationWriterBudget):
		require.FailNow(t, "a fixed suspension waited for another writer's database I/O")
	}
	releaseAdaptive()
	require.NoError(t, <-adaptiveDone)
	releaseFixed()
	require.NoError(t, <-fixedDone)

	row := persistedPublicationAbility(t, db, 1)
	require.NotNil(t, row.SuspendUntil)
	require.NotNil(t, row.AdaptiveSuspendUntil)
	channelSyncLock.RLock()
	stillCooling := cachedAbilityCoolingLocked(cachedAbilityKey{"default", "target", 1}, time.Now().UTC().Add(30*time.Minute))
	channelSyncLock.RUnlock()
	require.True(t, stillCooling, "the one-hour fixed suspension must stay in the cache after the 10s adaptive rung publishes")
}

// TestCooldownRebuildReplacesRecreatedAbilityState guards the snapshot merge
// against revision resets: an ability row deleted and recreated (a model
// removed from a channel and added back) restarts its cooldown revision at
// zero, and the next rebuild must publish the new row's state instead of
// keeping the deleted row's higher-revision cooldown.
func TestCooldownRebuildReplacesRecreatedAbilityState(t *testing.T) {
	db := setupAbilityCacheTest(t, abilityCacheChannel(1, 10), abilityCacheChannel(2, 10))
	snapshotProvisionalAbilityCooldowns(t)
	require.NoError(t, SuspendAbility(context.Background(), "default", "target", 1, time.Hour))
	requireCachedCooling(t, 1, true, "fixture: channel 1 must be cooling")

	row := persistedPublicationAbility(t, db, 1)
	require.Positive(t, row.CooldownRevision)
	require.NoError(t, exactCooldownAbility(db, cachedAbilityKey{"default", "target", 1}).Delete(&Ability{}).Error)
	priority := int64(10)
	require.NoError(t, db.Create(&Ability{Group: "default", Model: "target", ChannelId: 1, Enabled: true, Priority: &priority}).Error)

	require.NoError(t, InitChannelCacheContext(context.Background()))
	requireCachedCooling(t, 1, false, "the recreated ability has no cooldown; the deleted row's state must not survive the rebuild")
}
