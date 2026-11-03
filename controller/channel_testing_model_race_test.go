package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// hookedUpstream wraps an upstreamRecorder so a test can act at the moment the
// probe reaches the upstream, i.e. while the health check is in flight.
// Parameters: t is the test handle, status is the recorder's answer, and onRequest
// runs before the recorder handles each request.
// Returns: the inner recorder (for assertions) and the hooked server URL.
func hookedUpstream(t *testing.T, status int, onRequest func()) (*upstreamRecorder, string) {
	t.Helper()
	inner := newUpstreamRecorder(t, status)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		onRequest()
		inner.server.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return inner, server.URL
}

// invalidKeyUpstream answers every probe with an OpenAI invalid_api_key error, which
// the sweep treats as grounds for auto-disabling the channel.
// Parameters: t is the test handle and onRequest runs when a probe arrives.
// Returns: the server URL and a counter of received probes.
func invalidKeyUpstream(t *testing.T, onRequest func()) (string, *atomic.Int64) {
	t.Helper()
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		onRequest()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Incorrect API key provided",` +
			`"type":"invalid_request_error","code":"invalid_api_key"}}`))
	}))
	t.Cleanup(server.Close)
	return server.URL, &requests
}

// insertProbeChannel stores an OpenAI-compatible channel pointing at baseURL.
// Parameters: t is the test handle, name labels the channel, models is its model list,
// baseURL is the upstream, and testingModel is the stored selection (nil for auto).
// Returns: the inserted channel.
func insertProbeChannel(t *testing.T, name, models, baseURL string, testingModel *string) *model.Channel {
	t.Helper()
	channel := &model.Channel{
		Name:    name,
		Type:    channeltype.OpenAICompatible,
		Status:  model.ChannelStatusEnabled,
		Key:     "sk-test",
		BaseURL: &baseURL,
		Models:  models,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	if testingModel != nil {
		require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).
			Update("testing_model", *testingModel).Error)
	}
	return channel
}

// storeTestingModelSkip simulates an administrator saving SKIP through another request.
// Parameters: db is the session to write with and id identifies the channel.
// Returns: the write error, if any.
func storeTestingModelSkip(db *gorm.DB, id int) error {
	return db.Exec("UPDATE channels SET testing_model = ? WHERE id = ?", model.ChannelTestingModelSkip, id).Error
}

// readChannelTestState returns the persisted testing_model and test_time of a channel.
// Parameters: t is the test handle and id identifies the channel.
// Returns: the stored testing_model (nil when NULL) and test_time.
func readChannelTestState(t *testing.T, id int) (*string, int64) {
	t.Helper()
	var stored model.Channel
	require.NoError(t, model.DB.Select("id", "testing_model", "test_time").First(&stored, "id = ?", id).Error)
	return stored.TestingModel, stored.TestTime
}

// TestUpdateChannelPricingDoesNotReassertStaleTestingModel reproduces the review's
// lost-update scenario for the pricing endpoint: it reloads the whole row, so the
// testing_model it read must not overwrite a SKIP saved before the pricing write.
// Parameters: t is the test handle. Returns: no values.
func TestUpdateChannelPricingDoesNotReassertStaleTestingModel(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	named := "gpt-4o"
	channel := insertProbeChannel(t, "pricing-race", "gpt-4o-mini,gpt-4o", "http://127.0.0.1:1", &named)

	// Save SKIP right after the pricing handler has loaded the row.
	var fired atomic.Bool
	const callback = "test:pricing_reload_interleave"
	require.NoError(t, model.DB.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		loaded, ok := tx.Statement.Dest.(*model.Channel)
		if !ok || loaded.Id != channel.Id || !fired.CompareAndSwap(false, true) {
			return
		}
		require.NoError(t, storeTestingModelSkip(tx.Session(&gorm.Session{NewDB: true}), channel.Id))
	}))
	t.Cleanup(func() { require.NoError(t, model.DB.Callback().Query().Remove(callback)) })

	router := gin.New()
	router.PUT("/api/channel/pricing/:id", UpdateChannelPricing)
	serveTestingModelJSON(t, router, http.MethodPut, "/api/channel/pricing/"+channel.UUID, map[string]any{
		"model_configs": map[string]any{"gpt-4o": map[string]any{"ratio": 2}},
	})

	require.True(t, fired.Load(), "the interleaving hook must have run")
	stored, _ := readChannelTestState(t, channel.Id)
	requireTestingModel(t, testingModelPtr(model.ChannelTestingModelSkip), stored, "database row")
}

// TestManualChannelTestKeepsSkipSavedDuringStaleCleanup reproduces the review's P2
// scenario for the manual test: the probe read a stale (non-chat) testing model, and an
// administrator saved SKIP before the cleanup ran; the cleanup must not erase SKIP.
// Parameters: t is the test handle. Returns: no values.
func TestManualChannelTestKeepsSkipSavedDuringStaleCleanup(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	upstream := newUpstreamRecorder(t, http.StatusOK)
	stale := "sora-2"
	channel := insertProbeChannel(t, "manual-cleanup-race", "sora-2,gpt-4o-mini", upstream.server.URL, &stale)

	var fired atomic.Bool
	const callback = "test:stale_cleanup_interleave"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		updates, ok := tx.Statement.Dest.(map[string]any)
		if !ok || tx.Statement.Table != "channels" {
			return
		}
		if value, present := updates["testing_model"]; !present || value != nil || !fired.CompareAndSwap(false, true) {
			return
		}
		require.NoError(t, storeTestingModelSkip(tx.Session(&gorm.Session{NewDB: true}), channel.Id))
	}))
	t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove(callback)) })

	router := gin.New()
	router.POST("/api/channel/test/:id", TestChannel)
	req := httptest.NewRequest(http.MethodPost, "/api/channel/test/"+channel.UUID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	require.True(t, fired.Load(), "the stale testing model must have triggered the cleanup")
	require.Eventually(t, func() bool {
		_, testTime := readChannelTestState(t, channel.Id)
		return testTime != 0
	}, 5*time.Second, 10*time.Millisecond, "the manual test must persist its response time")
	stored, _ := readChannelTestState(t, channel.Id)
	requireTestingModel(t, testingModelPtr(model.ChannelTestingModelSkip), stored, "database row")
}

// TestManualChannelTestClearsStaleTestingModel pins the existing repair behavior that
// the conditional cleanup must keep: a stale non-chat selection reverts to auto-select.
// Parameters: t is the test handle. Returns: no values.
func TestManualChannelTestClearsStaleTestingModel(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	upstream := newUpstreamRecorder(t, http.StatusOK)
	stale := "sora-2"
	channel := insertProbeChannel(t, "manual-cleanup", "sora-2,gpt-4o-mini", upstream.server.URL, &stale)

	router := gin.New()
	router.POST("/api/channel/test/:id", TestChannel)
	req := httptest.NewRequest(http.MethodPost, "/api/channel/test/"+channel.UUID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	require.Eventually(t, func() bool {
		_, testTime := readChannelTestState(t, channel.Id)
		return testTime != 0
	}, 5*time.Second, 10*time.Millisecond)
	stored, _ := readChannelTestState(t, channel.Id)
	requireTestingModel(t, nil, stored, "database row")
	require.Equal(t, []string{"/v1/chat/completions"}, upstream.requestedPaths())
}

// TestSweepIgnoresResultForChannelSkippedDuringProbe reproduces the review's P2 sweep
// scenario: SKIP saved while the probe is in flight must stop the failed probe from
// auto-disabling the channel or recording a test time.
// Parameters: t is the test handle. Returns: no values.
func TestSweepIgnoresResultForChannelSkippedDuringProbe(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	var channelID atomic.Int64
	url, requests := invalidKeyUpstream(t, func() {
		require.NoError(t, storeTestingModelSkip(model.DB, int(channelID.Load())))
	})
	channel := insertProbeChannel(t, "skip-mid-probe", "gpt-4o-mini", url, nil)
	channelID.Store(int64(channel.Id))

	runChannelSweep(t)

	require.EqualValues(t, 1, requests.Load(), "the probe was already in flight")
	require.Equal(t, model.ChannelStatusEnabled, channelStatus(t, channel.Id),
		"a channel opted out mid-probe must not be auto-disabled by that probe")
	stored, testTime := readChannelTestState(t, channel.Id)
	requireTestingModel(t, testingModelPtr(model.ChannelTestingModelSkip), stored, "database row")
	require.Zero(t, testTime, "a skipped channel never records a test time")
}

// TestSweepAutoDisablesOnInvalidKey is the control for the mid-probe scenario: the same
// upstream failure does auto-disable a channel that stays opted in, so the scenario
// above exercises the real disable path rather than passing vacuously.
// Parameters: t is the test handle. Returns: no values.
func TestSweepAutoDisablesOnInvalidKey(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	url, requests := invalidKeyUpstream(t, func() {})
	channel := insertProbeChannel(t, "invalid-key", "gpt-4o-mini", url, nil)

	runChannelSweep(t)

	require.EqualValues(t, 1, requests.Load())
	require.Equal(t, model.ChannelStatusAutoDisabled, channelStatus(t, channel.Id))
}

// TestSweepHonoursSkipSavedAfterSnapshot verifies the sweep re-reads each channel's
// testing selection before probing it, so SKIP saved after the sweep loaded its channel
// list still prevents the probe. Channels are swept in descending id order.
// Parameters: t is the test handle. Returns: no values.
func TestSweepHonoursSkipSavedAfterSnapshot(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	later := newUpstreamRecorder(t, http.StatusInternalServerError)
	optedOut := insertProbeChannel(t, "skip-after-snapshot", "gpt-4o-mini", later.server.URL, nil)

	_, url := hookedUpstream(t, http.StatusOK, func() {
		require.NoError(t, storeTestingModelSkip(model.DB, optedOut.Id))
	})
	first := insertProbeChannel(t, "probed-first", "gpt-4o-mini", url, nil)
	require.Greater(t, first.Id, optedOut.Id, "the hooked channel must be swept first")

	runChannelSweep(t)

	require.Empty(t, later.requestedPaths(),
		fmt.Sprintf("channel %d was set to SKIP before its turn and must not be probed", optedOut.Id))
	require.Equal(t, model.ChannelStatusEnabled, channelStatus(t, optedOut.Id))
	_, testTime := readChannelTestState(t, optedOut.Id)
	require.Zero(t, testTime)
}
