package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
)

// probedModels extracts the "model" field of every recorded probe body.
// Parameters: t is the test handle and recorder holds the upstream requests.
// Returns: the requested model names in order.
func probedModels(t *testing.T, recorder *upstreamRecorder) []string {
	t.Helper()
	names := make([]string, 0)
	for _, body := range recorder.requestedBodies() {
		var request struct {
			Model string `json:"model"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &request))
		names = append(names, request.Model)
	}
	return names
}

// TestSweepUsesCurrentConfigurationForLaterChannels reproduces the review's P1: an
// administrator changes a channel's models and testing model after the sweep loaded its
// channel list. The sweep must judge and probe that channel with its current
// configuration: keep the new selection and never probe the obsolete model.
// Parameters: t is the test handle. Returns: no values.
func TestSweepUsesCurrentConfigurationForLaterChannels(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	later := newUpstreamRecorder(t, http.StatusOK)
	reconfigured := insertProbeChannel(t, "reconfigured", "gpt-4o-mini", later.server.URL, nil)

	_, url := hookedUpstream(t, http.StatusOK, func() {
		selection := "gpt-4o"
		update := &model.Channel{Id: reconfigured.Id, Name: reconfigured.Name, Models: "gpt-4o", TestingModel: &selection}
		require.NoError(t, update.UpdateWithContext(context.Background()))
	})
	first := insertProbeChannel(t, "probed-first", "gpt-4o-mini", url, nil)
	require.Greater(t, first.Id, reconfigured.Id, "the hooked channel must be swept first")

	runChannelSweep(t)

	stored, _ := readChannelTestState(t, reconfigured.Id)
	requireTestingModel(t, testingModelPtr("gpt-4o"), stored, "the new selection must survive the sweep")
	require.Equal(t, []string{"gpt-4o"}, probedModels(t, later),
		"the sweep must probe the channel's current testing model, not the obsolete one")
}

// TestSweepDiscardsResultWhenConfigurationChangesDuringProbe reproduces the review's
// configuration-freshness gap: the administrator rotates a bad key while the probe that
// used the old key is in flight. That failed probe no longer describes the channel, so
// it must not auto-disable it or record a test time.
// Parameters: t is the test handle. Returns: no values.
func TestSweepDiscardsResultWhenConfigurationChangesDuringProbe(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	var channelID atomic.Int64
	url, requests := invalidKeyUpstream(t, func() {
		require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channelID.Load()).
			Update("key", "sk-rotated").Error)
	})
	channel := insertProbeChannel(t, "key-rotated-mid-probe", "gpt-4o-mini", url, nil)
	channelID.Store(int64(channel.Id))

	runChannelSweep(t)

	require.EqualValues(t, 1, requests.Load())
	require.Equal(t, model.ChannelStatusEnabled, channelStatus(t, channel.Id),
		"a probe of a superseded configuration must not disable the channel")
	_, testTime := readChannelTestState(t, channel.Id)
	require.Zero(t, testTime)
}

// TestManualChannelTestKeepsSelectionValidatedByNewConfiguration reproduces the stale
// cleanup decision: the manual test judged the stored selection against a model list
// that an administrator extended right after the test loaded the channel. The cleanup
// must not erase a selection the current configuration serves.
// Parameters: t is the test handle. Returns: no values.
func TestManualChannelTestKeepsSelectionValidatedByNewConfiguration(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	upstream := newUpstreamRecorder(t, http.StatusOK)
	selection := "gpt-4o"
	// Legacy data: the selection is not served by the stored model list yet.
	channel := insertProbeChannel(t, "extended-models", "gpt-4o-mini", upstream.server.URL, &selection)

	var fired atomic.Bool
	const callback = "test:extend_models_after_load"
	require.NoError(t, model.DB.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		loaded, ok := tx.Statement.Dest.(*model.Channel)
		if !ok || loaded.Id != channel.Id || !fired.CompareAndSwap(false, true) {
			return
		}
		require.NoError(t, tx.Session(&gorm.Session{NewDB: true}).
			Exec("UPDATE channels SET models = ? WHERE id = ?", "gpt-4o-mini,gpt-4o", channel.Id).Error)
	}))
	t.Cleanup(func() { require.NoError(t, model.DB.Callback().Query().Remove(callback)) })

	router := gin.New()
	router.POST("/api/channel/test/:id", TestChannel)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/channel/test/"+channel.UUID, nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Eventually(t, func() bool {
		_, testTime := readChannelTestState(t, channel.Id)
		return testTime != 0
	}, 5*time.Second, 10*time.Millisecond)

	require.True(t, fired.Load())
	stored, _ := readChannelTestState(t, channel.Id)
	requireTestingModel(t, &selection, stored, "a selection the current models serve must not be cleared")
}

// TestChannelListOffersStoredTestingModel reproduces the review's P2 at the API
// boundary: a persisted selection the channel serves (a mapping alias, or a custom model
// on an unrestricted channel) must be among the offered test_models, so the selector can
// display it instead of falling back to Auto.
// Parameters: t is the test handle. Returns: no values.
func TestChannelListOffersStoredTestingModel(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	router := gin.New()
	router.GET("/api/channel/", GetAllChannels)
	mapping := `{"house-alias":"gpt-4o"}`
	cases := []struct {
		channel *model.Channel
		stored  string
	}{
		{channel: &model.Channel{Name: "alias", Type: 1, Models: "gpt-4o", ModelMapping: &mapping}, stored: "house-alias"},
		{channel: &model.Channel{Name: "unrestricted", Type: 1, Models: ""}, stored: "vendor/custom-chat"},
	}
	for _, tc := range cases {
		t.Run(tc.channel.Name, func(t *testing.T) {
			tc.channel.Key, tc.channel.Group, tc.channel.Status = "sk-test", "default", model.ChannelStatusEnabled
			tc.channel.TestingModel = &tc.stored
			require.NoError(t, tc.channel.Insert())
			require.Equal(t, &tc.stored, tc.channel.TestingModel, "the stored selection is valid for this channel")

			listed := serveTestingModelJSON(t, router, http.MethodGet,
				fmt.Sprintf("/api/channel/?p=0&size=%d", config.MaxItemsPerPage), nil)
			var rows []struct {
				UUID         string   `json:"uuid"`
				TestingModel *string  `json:"testing_model"`
				TestModels   []string `json:"test_models"`
			}
			require.NoError(t, json.Unmarshal(listed.Data, &rows))
			for _, row := range rows {
				if row.UUID == tc.channel.UUID {
					requireTestingModel(t, &tc.stored, row.TestingModel, "listed testing_model")
					require.Contains(t, row.TestModels, tc.stored, "the stored selection must be offered")
					return
				}
			}
			require.FailNow(t, "channel missing from list")
		})
	}
}

// TestSweepDoesNotReenableManuallyDisabledChannel reproduces the review's P2: automatic
// recovery must only undo automatic disables. A channel an administrator disabled stays
// disabled even when its probe succeeds.
// Parameters: t is the test handle. Returns: no values.
func TestSweepDoesNotReenableManuallyDisabledChannel(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	originalAutoEnable := config.AutomaticEnableChannelEnabled
	config.AutomaticEnableChannelEnabled = true
	t.Cleanup(func() { config.AutomaticEnableChannelEnabled = originalAutoEnable })
	upstream := newUpstreamRecorder(t, http.StatusOK)
	channel := insertProbeChannel(t, "manually-disabled", "gpt-4o-mini", upstream.server.URL, nil)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).
		Update("status", model.ChannelStatusManuallyDisabled).Error)

	runChannelSweep(t)

	require.Equal(t, model.ChannelStatusManuallyDisabled, channelStatus(t, channel.Id))
}

// TestSweepDoesNotReenableSlowRecovery reproduces the review's flapping concern: an
// auto-disabled channel whose probe succeeds but exceeds the response-time threshold
// would be disabled again on the next sweep, so it must stay disabled.
// Parameters: t is the test handle. Returns: no values.
func TestSweepDoesNotReenableSlowRecovery(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	originalAutoEnable := config.AutomaticEnableChannelEnabled
	config.AutomaticEnableChannelEnabled = true
	config.ChannelDisableThreshold = 0.05
	t.Cleanup(func() { config.AutomaticEnableChannelEnabled = originalAutoEnable })
	inner := newUpstreamRecorder(t, http.StatusOK)
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		inner.server.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(slow.Close)
	channel := insertProbeChannel(t, "slow-recovery", "gpt-4o-mini", slow.URL, nil)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).
		Update("status", model.ChannelStatusAutoDisabled).Error)

	runChannelSweep(t)

	require.Len(t, inner.requestedPaths(), 1)
	require.Equal(t, model.ChannelStatusAutoDisabled, channelStatus(t, channel.Id),
		"a recovery slower than the disable threshold must not re-enable the channel")
}

// TestLegacyStatusWriterKeepsAbilitiesConsistent reproduces the review's P2 for the
// legacy status helper used by relay errors and balance checks: it wrote abilities and
// the channel status in separate statements, so a concurrent status change could leave
// the channel disabled while its abilities still route traffic to it.
// Parameters: t is the test handle. Returns: no values.
func TestLegacyStatusWriterKeepsAbilitiesConsistent(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	channel := &model.Channel{Name: "legacy-status", Type: 1, Key: "sk-test", Group: "default",
		Models: "gpt-4o-mini", Status: model.ChannelStatusEnabled}
	require.NoError(t, channel.Insert())
	_, done := interleaveCompetingWriteBeforeStatusUpdate(t, model.ChannelStatusAutoDisabled, func() error {
		return model.SetChannelStatusWithContext(context.Background(), channel.Id, model.ChannelStatusEnabled)
	})

	model.UpdateChannelStatusById(channel.Id, model.ChannelStatusAutoDisabled)
	<-done

	status := channelStatus(t, channel.Id)
	var flags []bool
	require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", channel.Id).Pluck("enabled", &flags).Error)
	require.NotEmpty(t, flags)
	for _, enabled := range flags {
		require.Equal(t, status == model.ChannelStatusEnabled, enabled,
			fmt.Sprintf("ability routing (%v) must match channel status %d", enabled, status))
	}
	require.False(t, strings.Contains(fmt.Sprint(flags), "true") && status != model.ChannelStatusEnabled)
}
