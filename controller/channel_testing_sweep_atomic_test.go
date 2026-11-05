package controller

import (
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// sweepWriterGrace bounds how long a status-transition scenario waits for a competing
// administrator write that the sweep's transaction should hold back.
const sweepWriterGrace = 300 * time.Millisecond

// errInjectedChannelRead is the failure injected into channel reads.
var errInjectedChannelRead = errors.New("injected channel read failure")

// failChannelReadsWhenArmed makes every later query against the channels table fail
// once armed is set, simulating a database outage or cancelled context. Writes are
// unaffected, so code that mutates without reading is not shielded by the injection.
// Parameters: t is the test handle and armed switches the failure on.
// Returns: no values; the callback is removed when the test ends.
func failChannelReadsWhenArmed(t *testing.T, armed *atomic.Bool) {
	t.Helper()
	const name = "test:fail_channel_reads"
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if armed.Load() && tx.Statement.Table == "channels" {
			_ = tx.AddError(errInjectedChannelRead)
		}
	}))
	t.Cleanup(func() { require.NoError(t, model.DB.Callback().Query().Remove(name)) })
}

// TestSweepDiscardsResultWhenSelectionCannotBeVerified reproduces the review's P1: SKIP
// is saved while the probe is in flight and the post-probe read of the selection then
// fails. Unable to confirm the channel is still opted in, the sweep must fail closed:
// no auto-disable and no recorded test time.
// Parameters: t is the test handle. Returns: no values.
func TestSweepDiscardsResultWhenSelectionCannotBeVerified(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	var channelID atomic.Int64
	var readsFail atomic.Bool
	failChannelReadsWhenArmed(t, &readsFail)
	url, requests := invalidKeyUpstream(t, func() {
		require.NoError(t, storeTestingModelSkip(model.DB, int(channelID.Load())))
		readsFail.Store(true)
	})
	channel := insertProbeChannel(t, "skip-unverifiable", "gpt-4o-mini", url, nil)
	channelID.Store(int64(channel.Id))

	runChannelSweep(t)
	readsFail.Store(false)

	require.EqualValues(t, 1, requests.Load())
	require.Equal(t, model.ChannelStatusEnabled, channelStatus(t, channel.Id),
		"an unverifiable opt-out must not let the probe auto-disable the channel")
	_, testTime := readChannelTestState(t, channel.Id)
	require.Zero(t, testTime, "an unverifiable result must not be recorded")
}

// TestSweepSkipsChannelWhenSelectionCannotBeRefreshed reproduces the review's P2: when
// the pre-probe read of the selection fails, the sweep must not fall back to its stale
// snapshot and spend an upstream request on a channel that may have opted out.
// Parameters: t is the test handle. Returns: no values.
func TestSweepSkipsChannelWhenSelectionCannotBeRefreshed(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	var readsFail atomic.Bool
	failChannelReadsWhenArmed(t, &readsFail)
	// Arm the failure once the sweep has loaded its channel list.
	const armAfterList = "test:arm_after_channel_list"
	require.NoError(t, model.DB.Callback().Query().After("gorm:query").Register(armAfterList, func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*[]*model.Channel); ok {
			readsFail.Store(true)
		}
	}))
	t.Cleanup(func() { require.NoError(t, model.DB.Callback().Query().Remove(armAfterList)) })
	upstream := newUpstreamRecorder(t, http.StatusOK)
	channel := insertProbeChannel(t, "refresh-unavailable", "gpt-4o-mini", upstream.server.URL, nil)

	runChannelSweep(t)
	readsFail.Store(false)

	require.Empty(t, upstream.requestedPaths(),
		"a channel whose current selection cannot be read must not be probed")
	_, testTime := readChannelTestState(t, channel.Id)
	require.Zero(t, testTime)
}

// interleaveCompetingWriteBeforeStatusUpdate runs write from a separate goroutine just
// before the sweep persists the given channel status, and reports whether that write
// completed before the status update executed. A sweep that applies its transition
// atomically holds the competing write back until it commits.
// Parameters: t is the test handle, status is the status update to intercept, and write
// is the competing administrator action.
// Returns: a flag set when the write finished first, and a channel closed when it ends.
func interleaveCompetingWriteBeforeStatusUpdate(t *testing.T, status int, write func() error) (*atomic.Bool, <-chan struct{}) {
	t.Helper()
	var finishedFirst, fired atomic.Bool
	done := make(chan struct{})
	const name = "test:competing_write_before_status"
	// Intercept before GORM opens the statement's implicit transaction: on this
	// single-connection fixture that transaction would otherwise hold the write back
	// even for a sweep that applies the status change without protection.
	require.NoError(t, model.DB.Callback().Update().Before("gorm:begin_transaction").Register(name, func(tx *gorm.DB) {
		updates, ok := tx.Statement.Dest.(map[string]any)
		if !ok || tx.Statement.Table != "channels" || updates["status"] != status || !fired.CompareAndSwap(false, true) {
			return
		}
		writeDone := make(chan struct{})
		go func() {
			defer close(done)
			require.NoError(t, write())
			close(writeDone)
		}()
		select {
		case <-writeDone:
			finishedFirst.Store(true)
		case <-time.After(sweepWriterGrace):
		}
	}))
	t.Cleanup(func() {
		require.NoError(t, model.DB.Callback().Update().Remove(name))
		if fired.Load() {
			<-done
		}
	})
	return &finishedFirst, done
}

// TestSweepDisableIsLinearizableWithSkip reproduces the review's P2 time-of-check/
// time-of-use race: SKIP saved after the sweep verified the selection but before it
// disabled the channel. The disable must either take effect before the SKIP write (the
// write waits for it) or not at all; it must never be applied over a saved SKIP.
// Parameters: t is the test handle. Returns: no values.
func TestSweepDisableIsLinearizableWithSkip(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	url, _ := invalidKeyUpstream(t, func() {})
	channel := insertProbeChannel(t, "skip-before-disable", "gpt-4o-mini", url, nil)
	skipFirst, done := interleaveCompetingWriteBeforeStatusUpdate(t, model.ChannelStatusAutoDisabled, func() error {
		return storeTestingModelSkip(model.DB, channel.Id)
	})

	runChannelSweep(t)
	<-done

	stored, _ := readChannelTestState(t, channel.Id)
	requireTestingModel(t, testingModelPtr(model.ChannelTestingModelSkip), stored, "database row")
	require.False(t, skipFirst.Load() && channelStatus(t, channel.Id) == model.ChannelStatusAutoDisabled,
		"the sweep disabled the channel after the administrator had already saved SKIP")
}

// TestSweepDoesNotOverrideStatusChangedDuringProbe verifies the sweep's status change is
// a compare-and-swap against the status it probed: an administrator who manually
// disables an auto-disabled channel while its recovery probe is in flight must not see
// the sweep re-enable it.
// Parameters: t is the test handle. Returns: no values.
func TestSweepDoesNotOverrideStatusChangedDuringProbe(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	originalAutoEnable := config.AutomaticEnableChannelEnabled
	config.AutomaticEnableChannelEnabled = true
	t.Cleanup(func() { config.AutomaticEnableChannelEnabled = originalAutoEnable })

	var channelID atomic.Int64
	_, url := hookedUpstream(t, http.StatusOK, func() {
		require.NoError(t, model.DB.Exec("UPDATE channels SET status = ? WHERE id = ?",
			model.ChannelStatusManuallyDisabled, channelID.Load()).Error)
	})
	channel := insertProbeChannel(t, "recovering", "gpt-4o-mini", url, nil)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).
		Update("status", model.ChannelStatusAutoDisabled).Error)
	channelID.Store(int64(channel.Id))

	runChannelSweep(t)

	require.Equal(t, model.ChannelStatusManuallyDisabled, channelStatus(t, channel.Id),
		"a manual status change made during the probe must win over the sweep's re-enable")
}

// TestSweepReenablesRecoveredChannel is the control for the compare-and-swap above: with
// no concurrent change, a recovered auto-disabled channel is still re-enabled.
// Parameters: t is the test handle. Returns: no values.
func TestSweepReenablesRecoveredChannel(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	originalAutoEnable := config.AutomaticEnableChannelEnabled
	config.AutomaticEnableChannelEnabled = true
	t.Cleanup(func() { config.AutomaticEnableChannelEnabled = originalAutoEnable })

	upstream := newUpstreamRecorder(t, http.StatusOK)
	channel := insertProbeChannel(t, "recovered", "gpt-4o-mini", upstream.server.URL, nil)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).
		Update("status", model.ChannelStatusAutoDisabled).Error)

	runChannelSweep(t)

	require.Equal(t, model.ChannelStatusEnabled, channelStatus(t, channel.Id))
	_, testTime := readChannelTestState(t, channel.Id)
	require.NotZero(t, testTime)
}

// sweepNotificationRecorder captures the announcements a sweep makes.
type sweepNotificationRecorder struct {
	mu       sync.Mutex
	disabled []int
	enabled  []int
	notices  []string
}

// recordSweepNotifications swaps the sweep notifier for a recorder for one test.
// Parameters: t is the test handle. Returns: the recorder; the notifier is restored on cleanup.
func recordSweepNotifications(t *testing.T) *sweepNotificationRecorder {
	t.Helper()
	recorder := &sweepNotificationRecorder{}
	original := channelSweepNotifier
	channelSweepNotifier.disabled = func(channelID int, _, _ string) {
		recorder.mu.Lock()
		defer recorder.mu.Unlock()
		recorder.disabled = append(recorder.disabled, channelID)
	}
	channelSweepNotifier.enabled = func(channelID int, _ string) {
		recorder.mu.Lock()
		defer recorder.mu.Unlock()
		recorder.enabled = append(recorder.enabled, channelID)
	}
	channelSweepNotifier.notice = func(title, _ string) error {
		recorder.mu.Lock()
		defer recorder.mu.Unlock()
		recorder.notices = append(recorder.notices, title)
		return nil
	}
	t.Cleanup(func() { channelSweepNotifier = original })
	return recorder
}

// snapshot returns copies of the recorded announcements.
// Returns: disabled channel ids, enabled channel ids, and notice titles.
func (r *sweepNotificationRecorder) snapshot() ([]int, []int, []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]int{}, r.disabled...), append([]int{}, r.enabled...), append([]string{}, r.notices...)
}

// TestSweepAnnouncesOnlyPersistedTransitions verifies notifications follow what the
// sweep's transaction actually persisted: one disable announcement for a real
// transition, and none when SKIP or a concurrent status change made the sweep keep the
// channel as it was.
// Parameters: t is the test handle. Returns: no values.
func TestSweepAnnouncesOnlyPersistedTransitions(t *testing.T) {
	t.Run("a real disable is announced once", func(t *testing.T) {
		setupChannelSweepTestEnvironment(t)
		recorder := recordSweepNotifications(t)
		url, _ := invalidKeyUpstream(t, func() {})
		channel := insertProbeChannel(t, "announce-disable", "gpt-4o-mini", url, nil)

		runChannelSweep(t)

		disabled, enabled, notices := recorder.snapshot()
		require.Equal(t, []int{channel.Id}, disabled)
		require.Empty(t, enabled)
		require.Empty(t, notices)
	})

	t.Run("SKIP saved mid-probe announces nothing", func(t *testing.T) {
		setupChannelSweepTestEnvironment(t)
		recorder := recordSweepNotifications(t)
		var channelID atomic.Int64
		url, _ := invalidKeyUpstream(t, func() {
			require.NoError(t, storeTestingModelSkip(model.DB, int(channelID.Load())))
		})
		channel := insertProbeChannel(t, "announce-skip", "gpt-4o-mini", url, nil)
		channelID.Store(int64(channel.Id))

		runChannelSweep(t)

		disabled, enabled, notices := recorder.snapshot()
		require.Empty(t, disabled)
		require.Empty(t, enabled)
		require.Empty(t, notices)
	})

	t.Run("a refused re-enable announces nothing", func(t *testing.T) {
		setupChannelSweepTestEnvironment(t)
		recorder := recordSweepNotifications(t)
		originalAutoEnable := config.AutomaticEnableChannelEnabled
		config.AutomaticEnableChannelEnabled = true
		t.Cleanup(func() { config.AutomaticEnableChannelEnabled = originalAutoEnable })
		var channelID atomic.Int64
		_, url := hookedUpstream(t, http.StatusOK, func() {
			require.NoError(t, model.DB.Exec("UPDATE channels SET status = ? WHERE id = ?",
				model.ChannelStatusManuallyDisabled, channelID.Load()).Error)
		})
		channel := insertProbeChannel(t, "announce-refused", "gpt-4o-mini", url, nil)
		require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).
			Update("status", model.ChannelStatusAutoDisabled).Error)
		channelID.Store(int64(channel.Id))

		runChannelSweep(t)

		_, enabled, _ := recorder.snapshot()
		require.Empty(t, enabled)
	})
}

// TestPlanChannelSweepResult pins the sweep's decision policy, which the atomic apply
// then re-validates: slow or failing enabled channels are disabled (or reported when
// auto-disable is off); only automatically disabled channels that recover within the
// latency threshold are re-enabled, and only when auto-enable is on.
// Parameters: t is the test handle. Returns: no values.
func TestPlanChannelSweepResult(t *testing.T) {
	originalDisable, originalEnable := config.AutomaticDisableChannelEnabled, config.AutomaticEnableChannelEnabled
	t.Cleanup(func() {
		config.AutomaticDisableChannelEnabled, config.AutomaticEnableChannelEnabled = originalDisable, originalEnable
	})
	withStatus := func(status int) *model.Channel { return &model.Channel{Id: 7, Name: "plan", Status: status} }
	enabled := withStatus(model.ChannelStatusEnabled)
	invalidKey := &relaymodel.Error{Message: "bad key", Code: "invalid_api_key"}
	probeErr := errors.New("connection refused")

	config.AutomaticDisableChannelEnabled, config.AutomaticEnableChannelEnabled = true, true
	slow := planChannelSweepResult(enabled, 5000, 1000, nil, nil)
	require.Equal(t, model.ChannelStatusAutoDisabled, slow.targetStatus)
	require.Contains(t, slow.reason, "exceeds threshold")
	require.Empty(t, slow.notices)

	failed := planChannelSweepResult(enabled, 10, 1000, nil, invalidKey)
	require.Equal(t, model.ChannelStatusAutoDisabled, failed.targetStatus)
	require.Equal(t, "bad key", failed.reason)

	require.Zero(t, planChannelSweepResult(enabled, 10, 1000, nil, nil).targetStatus)

	autoDisabled := withStatus(model.ChannelStatusAutoDisabled)
	require.Equal(t, model.ChannelStatusEnabled, planChannelSweepResult(autoDisabled, 10, 1000, nil, nil).targetStatus)
	require.Zero(t, planChannelSweepResult(autoDisabled, 10, 1000, probeErr, nil).targetStatus)
	require.Zero(t, planChannelSweepResult(autoDisabled, 5000, 1000, nil, nil).targetStatus,
		"a recovery slower than the threshold would flap, so it is not re-enabled")
	require.Zero(t, planChannelSweepResult(withStatus(model.ChannelStatusManuallyDisabled), 10, 1000, nil, nil).targetStatus,
		"a manual disable is never undone by the sweep")

	config.AutomaticEnableChannelEnabled = false
	require.Zero(t, planChannelSweepResult(autoDisabled, 10, 1000, nil, nil).targetStatus)

	config.AutomaticDisableChannelEnabled = false
	reported := planChannelSweepResult(enabled, 5000, 1000, probeErr, nil)
	require.Zero(t, reported.targetStatus, "without auto-disable the sweep only reports")
	require.Len(t, reported.notices, 2)
	require.Contains(t, reported.notices[0].title, "Channel test timed out")
	require.Contains(t, reported.notices[1].title, "Channel test failed")
}
