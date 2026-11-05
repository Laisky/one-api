package controller

import (
	"context"
	"maps"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/metrics"
	"github.com/Laisky/one-api/model"
)

// healthCheckRecorder captures sweep outcomes; every other metric is a no-op.
type healthCheckRecorder struct {
	metrics.NoOpRecorder
	mu       sync.Mutex
	outcomes map[string]int
}

// RecordChannelHealthCheck tallies one sweep outcome.
// Parameters: outcome is a ChannelHealthCheck* constant. Returns: no values.
func (r *healthCheckRecorder) RecordChannelHealthCheck(outcome string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.outcomes[outcome]++
}

// snapshot returns a copy of the recorded outcome counts.
// Returns: the outcome counts recorded so far.
func (r *healthCheckRecorder) snapshot() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return maps.Clone(r.outcomes)
}

// recordHealthChecks installs a healthCheckRecorder for one test.
// Parameters: t is the test handle. Returns: the recorder; the previous one is restored on cleanup.
func recordHealthChecks(t *testing.T) *healthCheckRecorder {
	t.Helper()
	recorder := &healthCheckRecorder{outcomes: make(map[string]int)}
	previous := metrics.Recorder()
	metrics.SetRecorder(recorder)
	t.Cleanup(func() { metrics.SetRecorder(previous) })
	return recorder
}

// saveTestingModelAfterReload saves selection right after the sweep reloads the channel,
// i.e. after the sweep judged the stored selection but before it cleans it up.
// Parameters: t is the test handle, id identifies the channel, and selection is saved.
// Returns: a flag reporting whether the interleaved save ran.
func saveTestingModelAfterReload(t *testing.T, id int, selection string) *atomic.Bool {
	t.Helper()
	var fired atomic.Bool
	const callback = "test:save_testing_model_after_reload"
	require.NoError(t, model.DB.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		loaded, ok := tx.Statement.Dest.(*model.Channel)
		if !ok || loaded.Id != id || !fired.CompareAndSwap(false, true) {
			return
		}
		require.NoError(t, tx.Session(&gorm.Session{NewDB: true}).
			Exec("UPDATE channels SET testing_model = ? WHERE id = ?", selection, id).Error)
	}))
	t.Cleanup(func() { require.NoError(t, model.DB.Callback().Query().Remove(callback)) })
	return &fired
}

// TestSweepPersistsProbeAfterSuccessfulStaleCleanup reproduces the review's P2: the sweep
// captured the probe revision before clearing a stale testing model, so its own cleanup
// changed the stored revision and the atomic apply discarded the probe as
// config_changed. A failing channel was never disabled and no latency was recorded.
// Parameters: t is the test handle. Returns: no values.
func TestSweepPersistsProbeAfterSuccessfulStaleCleanup(t *testing.T) {
	stale := "sora-2" // a model the channel serves, but not through a chat API format

	t.Run("a failing probe disables the channel", func(t *testing.T) {
		setupChannelSweepTestEnvironment(t)
		outcomes := recordHealthChecks(t)
		url, requests := invalidKeyUpstream(t, func() {})
		channel := insertProbeChannel(t, "stale-cleanup-failing", "sora-2,gpt-4o-mini", url, &stale)

		runChannelSweep(t)

		require.EqualValues(t, 1, requests.Load())
		stored, testTime := readChannelTestState(t, channel.Id)
		requireTestingModel(t, nil, stored, "the stale selection must be cleared")
		require.Equal(t, model.ChannelStatusAutoDisabled, channelStatus(t, channel.Id),
			"the probe that followed the sweep's own cleanup must be applied")
		require.NotZero(t, testTime, "the probe latency must be recorded")
		require.Equal(t, map[string]int{metrics.ChannelHealthCheckStatusChanged: 1}, outcomes.snapshot())
	})

	t.Run("a healthy probe records its latency", func(t *testing.T) {
		setupChannelSweepTestEnvironment(t)
		outcomes := recordHealthChecks(t)
		upstream := newUpstreamRecorder(t, http.StatusOK)
		channel := insertProbeChannel(t, "stale-cleanup-healthy", "sora-2,gpt-4o-mini", upstream.server.URL, &stale)

		runChannelSweep(t)

		require.Equal(t, []string{"gpt-4o-mini"}, probedModels(t, upstream))
		stored, testTime := readChannelTestState(t, channel.Id)
		requireTestingModel(t, nil, stored, "the stale selection must be cleared")
		require.NotZero(t, testTime, "the probe latency must be recorded")
		require.Equal(t, map[string]int{metrics.ChannelHealthCheckApplied: 1}, outcomes.snapshot())
	})

	t.Run("a selection saved before the cleanup discards the probe", func(t *testing.T) {
		setupChannelSweepTestEnvironment(t)
		outcomes := recordHealthChecks(t)
		url, requests := invalidKeyUpstream(t, func() {})
		channel := insertProbeChannel(t, "stale-cleanup-raced", "sora-2,gpt-4o-mini", url, &stale)
		fired := saveTestingModelAfterReload(t, channel.Id, "gpt-4o-mini")

		runChannelSweep(t)

		require.True(t, fired.Load(), "the interleaved save must have run")
		require.EqualValues(t, 1, requests.Load())
		stored, testTime := readChannelTestState(t, channel.Id)
		requireTestingModel(t, testingModelPtr("gpt-4o-mini"), stored, "the new selection must survive")
		require.Equal(t, model.ChannelStatusEnabled, channelStatus(t, channel.Id),
			"a probe judged against a superseded selection must not disable the channel")
		require.Zero(t, testTime)
		require.Equal(t, map[string]int{metrics.ChannelHealthCheckConfigChanged: 1}, outcomes.snapshot())
	})
}

// TestSweepCountsEveryChannelItFinishes reproduces the review's P3: channels the sweep
// skips before probing (SKIP, or no chat-format model) were not counted, so
// oneapi_channel_health_checks_total under-reported the sweep and opted_out only counted
// mid-probe opt-outs.
// Parameters: t is the test handle. Returns: no values.
func TestSweepCountsEveryChannelItFinishes(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	outcomes := recordHealthChecks(t)
	upstream := newUpstreamRecorder(t, http.StatusOK)
	skip := model.ChannelTestingModelSkip
	insertProbeChannel(t, "counted-skip", "gpt-4o-mini", upstream.server.URL, &skip)
	insertProbeChannel(t, "counted-embeddings", "text-embedding-3-small", upstream.server.URL, nil)
	insertProbeChannel(t, "counted-probed", "gpt-4o-mini", upstream.server.URL, nil)

	runChannelSweep(t)

	require.Len(t, upstream.requestedPaths(), 1, "only the opted-in chat channel is probed")
	require.Equal(t, map[string]int{
		metrics.ChannelHealthCheckOptedOut:      1,
		metrics.ChannelHealthCheckNotApplicable: 1,
		metrics.ChannelHealthCheckApplied:       1,
	}, outcomes.snapshot())
}

// TestSavingTestingModelKeepsAbilitySuspension reproduces the review's P1 at the API
// boundary the Modern selector uses: saving SKIP through PUT /api/channel/ used to
// rebuild every ability and lift an active relay-time suspension.
// Parameters: t is the test handle. Returns: no values.
func TestSavingTestingModelKeepsAbilitySuspension(t *testing.T) {
	router := newTestingModelPersistenceRouter(t)
	channel := insertTestingModelChannel(t, testingModelPersistenceModels, nil)
	require.NoError(t, model.SuspendAbility(context.Background(), channel.Group, "gpt-4o", channel.Id, time.Hour))

	serveTestingModelJSON(t, router, http.MethodPut, "/api/channel/", map[string]any{
		"uuid": channel.UUID, "name": channel.Name, "testing_model": model.ChannelTestingModelSkip,
	})

	requireTestingModel(t, testingModelPtr(model.ChannelTestingModelSkip), storedTestingModel(t, channel.Id),
		"database row")
	var ability model.Ability
	require.NoError(t, model.DB.Where("channel_id = ? AND model = ?", channel.Id, "gpt-4o").First(&ability).Error)
	require.NotNil(t, ability.SuspendUntil, "saving SKIP must not lift the suspension")
	require.True(t, ability.SuspendUntil.After(time.Now()), "the ability must still be suspended")
}
