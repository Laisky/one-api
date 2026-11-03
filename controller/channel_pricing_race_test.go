package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// pricingRaceModels is the model list every pricing interleaving scenario starts from.
const pricingRaceModels = "gpt-4o-mini,gpt-4o"

// insertPricingChannel stores an enabled OpenAI channel in the default group through
// the production insert path, so its abilities exist exactly as they would in service.
// Parameters: t is the test handle, name labels the channel, and channelConfig is the
// initial config JSON blob ("" for none).
// Returns: the inserted channel with its generated id and UUID.
func insertPricingChannel(t *testing.T, name, channelConfig string) *model.Channel {
	t.Helper()
	baseURL := "https://pricing-race.example.com"
	channel := &model.Channel{
		Name:    name,
		Type:    channeltype.OpenAI,
		Status:  model.ChannelStatusEnabled,
		Key:     "sk-test",
		Group:   "default",
		BaseURL: &baseURL,
		Models:  pricingRaceModels,
		Config:  channelConfig,
	}
	require.NoError(t, channel.Insert())
	return channel
}

// loadPricingChannel reads the full persisted channel row, bypassing every cache.
// Parameters: t is the test handle and id identifies the channel.
// Returns: the stored row.
func loadPricingChannel(t *testing.T, id int) *model.Channel {
	t.Helper()
	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, "id = ?", id).Error)
	return &stored
}

// newPricingRouter serves only the pricing endpoint under its production route.
// Parameters: none. Returns: the gin engine.
func newPricingRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/api/channel/pricing/:id", UpdateChannelPricing)
	return router
}

// requireModelRatio asserts the stored unified pricing for one model.
// Parameters: t is the test handle, channel is the stored row, modelName selects the
// entry, and want is the expected ratio.
// Returns: no values.
func requireModelRatio(t *testing.T, channel *model.Channel, modelName string, want float64) {
	t.Helper()
	configs := channel.GetModelPriceConfigs()
	require.Contains(t, configs, modelName, "model_configs: %v", configs)
	require.InDelta(t, want, configs[modelName].Ratio, 1e-12)
}

// TestUpdateChannelPricingKeepsConcurrentChannelEdit reproduces the pricing endpoint's
// lost update: an administrator's edit of other columns (priority and models here),
// committed right after the pricing handler read the row, must survive the pricing save.
// Before the fix the handler wrote its whole stale row back and reverted the edit.
// Parameters: t is the test handle. Returns: no values.
func TestUpdateChannelPricingKeepsConcurrentChannelEdit(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	channel := insertPricingChannel(t, "pricing-concurrent-edit", "")
	const editedModels = pricingRaceModels + ",o3-mini"
	const editedPriority = int64(7)

	// Commit the competing edit right after the handler has read the row.
	var fired atomic.Bool
	const callback = "test:pricing_concurrent_edit"
	require.NoError(t, model.DB.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		loaded, ok := tx.Statement.Dest.(*model.Channel)
		if !ok || loaded.Id != channel.Id || !fired.CompareAndSwap(false, true) {
			return
		}
		require.NoError(t, tx.Session(&gorm.Session{NewDB: true}).
			Exec("UPDATE channels SET priority = ?, models = ? WHERE id = ?", editedPriority, editedModels, channel.Id).Error)
	}))
	t.Cleanup(func() { require.NoError(t, model.DB.Callback().Query().Remove(callback)) })

	serveTestingModelJSON(t, newPricingRouter(), http.MethodPut, "/api/channel/pricing/"+channel.UUID, map[string]any{
		"model_configs": map[string]any{"gpt-4o": map[string]any{"ratio": 2}},
	})

	require.True(t, fired.Load(), "the interleaving hook must have run")
	stored := loadPricingChannel(t, channel.Id)
	require.Equal(t, editedPriority, stored.GetPriority(), "the pricing save must not revert a concurrent priority edit")
	require.Equal(t, editedModels, stored.Models, "the pricing save must not revert a concurrent models edit")
	requireModelRatio(t, stored, "gpt-4o", 2)
}

// TestUpdateChannelPricingMergesToolingIntoLatestConfig verifies the tooling change is
// applied to the config blob as it stands when the pricing write takes the row lock: a
// config edit (custom headers here) that landed after the old handler's read is kept,
// and the tooling entry is added on top of it.
// Parameters: t is the test handle. Returns: no values.
func TestUpdateChannelPricingMergesToolingIntoLatestConfig(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	channel := insertPricingChannel(t, "pricing-config-merge", `{"custom_headers":{"X-Before":"1"}}`)
	const editedConfig = `{"custom_headers":{"X-Edited":"1"}}`

	// Commit the competing config edit immediately before the first channel UPDATE of
	// the pricing request, which is the row lock that guards the pricing write.
	var fired atomic.Bool
	const callback = "test:pricing_config_interleave"
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table != "channels" || !fired.CompareAndSwap(false, true) {
			return
		}
		require.NoError(t, tx.Session(&gorm.Session{NewDB: true}).
			Exec("UPDATE channels SET config = ? WHERE id = ?", editedConfig, channel.Id).Error)
	}))
	t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove(callback)) })

	serveTestingModelJSON(t, newPricingRouter(), http.MethodPut, "/api/channel/pricing/"+channel.UUID, map[string]any{
		"tooling": map[string]any{"whitelist": []string{"web_search"}},
	})

	require.True(t, fired.Load(), "the interleaving hook must have run")
	stored := loadPricingChannel(t, channel.Id)
	cfg, err := stored.LoadConfig()
	require.NoError(t, err)
	require.Equal(t, map[string]string{"X-Edited": "1"}, cfg.CustomHeaders,
		"the pricing save must keep the config edit committed before its write")
	require.NotNil(t, cfg.Tooling)
	require.Equal(t, []string{"web_search"}, cfg.Tooling.Whitelist)
}

// TestUpdateChannelPricingLeavesAbilitiesUntouched verifies a pricing save does not
// rebuild the channel's abilities. Abilities derive from models, groups, hidden models,
// status, and priority, none of which pricing changes; rebuilding them used to discard
// per-ability state such as an active suspension.
// Parameters: t is the test handle. Returns: no values.
func TestUpdateChannelPricingLeavesAbilitiesUntouched(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	channel := insertPricingChannel(t, "pricing-abilities", "")
	suspendUntil := time.Now().UTC().Add(time.Hour)
	require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", channel.Id).
		Update("suspend_until", suspendUntil).Error)

	serveTestingModelJSON(t, newPricingRouter(), http.MethodPut, "/api/channel/pricing/"+channel.UUID, map[string]any{
		"model_configs": map[string]any{"gpt-4o": map[string]any{"ratio": 3}},
	})

	var abilities []model.Ability
	require.NoError(t, model.DB.Where("channel_id = ?", channel.Id).Order("model").Find(&abilities).Error)
	require.Len(t, abilities, 2)
	for _, ability := range abilities {
		require.NotNil(t, ability.SuspendUntil, "ability %s/%s must keep its suspension", ability.Group, ability.Model)
		require.WithinDuration(t, suspendUntil, *ability.SuspendUntil, time.Second)
	}
	requireModelRatio(t, loadPricingChannel(t, channel.Id), "gpt-4o", 3)
}

// TestUpdateChannelPricingClearsTooling verifies an explicit null tooling payload clears
// the tooling entry even when it is the only entry in the config blob. The whole-row
// save skipped the then-empty config column, so the old tooling stayed in force.
// Parameters: t is the test handle. Returns: no values.
func TestUpdateChannelPricingClearsTooling(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	cases := []struct {
		name       string
		config     string
		wantConfig string
	}{
		{name: "tooling is the only entry", config: `{"tooling":{"whitelist":["web_search"]}}`, wantConfig: ""},
		{
			name:       "other entries are kept",
			config:     `{"api_version":"2024-06-01","tooling":{"whitelist":["web_search"]}}`,
			wantConfig: `{"api_version":"2024-06-01"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			channel := insertPricingChannel(t, "pricing-clear-tooling-"+tc.name, tc.config)
			require.NotNil(t, loadPricingChannel(t, channel.Id).GetToolingConfig())

			serveTestingModelJSON(t, newPricingRouter(), http.MethodPut, "/api/channel/pricing/"+channel.UUID,
				map[string]any{"tooling": nil})

			stored := loadPricingChannel(t, channel.Id)
			require.Nil(t, stored.GetToolingConfig())
			require.Equal(t, tc.wantConfig, stored.Config)
		})
	}
}

// TestUpdateChannelPricingKeepsPricingWithoutNonZeroLegacyRatios pins existing
// behaviour: legacy ratios that carry no non-zero price leave model_configs as stored,
// and a request that supplies neither format changes nothing.
// Parameters: t is the test handle. Returns: no values.
func TestUpdateChannelPricingKeepsPricingWithoutNonZeroLegacyRatios(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	channel := insertPricingChannel(t, "pricing-zero-legacy", "")
	router := newPricingRouter()
	serveTestingModelJSON(t, router, http.MethodPut, "/api/channel/pricing/"+channel.UUID, map[string]any{
		"model_configs": map[string]any{"gpt-4o": map[string]any{"ratio": 4, "max_tokens": 4096}},
	})

	for name, payload := range map[string]map[string]any{
		"zero legacy ratios": {"model_ratio": map[string]any{"gpt-4o": 0}, "completion_ratio": map[string]any{"gpt-4o": 0}},
		"empty request":      {},
	} {
		t.Run(name, func(t *testing.T) {
			serveTestingModelJSON(t, router, http.MethodPut, "/api/channel/pricing/"+channel.UUID, payload)
			stored := loadPricingChannel(t, channel.Id)
			requireModelRatio(t, stored, "gpt-4o", 4)
			require.EqualValues(t, 4096, stored.GetModelPriceConfigs()["gpt-4o"].MaxTokens)
		})
	}
}

// TestUpdateChannelPricingLegacyRatiosReplacePricing pins the legacy format: separate
// model and completion ratios replace the unified pricing table.
// Parameters: t is the test handle. Returns: no values.
func TestUpdateChannelPricingLegacyRatiosReplacePricing(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	channel := insertPricingChannel(t, "pricing-legacy", "")
	router := newPricingRouter()
	serveTestingModelJSON(t, router, http.MethodPut, "/api/channel/pricing/"+channel.UUID, map[string]any{
		"model_configs": map[string]any{"gpt-4o-mini": map[string]any{"ratio": 9}},
	})

	serveTestingModelJSON(t, router, http.MethodPut, "/api/channel/pricing/"+channel.UUID, map[string]any{
		"model_ratio":      map[string]any{"gpt-4o": 1.5},
		"completion_ratio": map[string]any{"gpt-4o": 4},
	})

	configs := loadPricingChannel(t, channel.Id).GetModelPriceConfigs()
	require.Equal(t, map[string]model.ModelConfigLocal{"gpt-4o": {Ratio: 1.5, CompletionRatio: 4}}, configs)
}

// TestUpdateChannelPricingRejectsInvalidPricing verifies validation failures are
// reported to the caller and leave the stored pricing unchanged.
// Parameters: t is the test handle. Returns: no values.
func TestUpdateChannelPricingRejectsInvalidPricing(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	channel := insertPricingChannel(t, "pricing-invalid", "")
	router := newPricingRouter()
	serveTestingModelJSON(t, router, http.MethodPut, "/api/channel/pricing/"+channel.UUID, map[string]any{
		"model_configs": map[string]any{"gpt-4o": map[string]any{"ratio": 5}},
	})

	for name, payload := range map[string]map[string]any{
		"negative ratio": {"model_configs": map[string]any{"gpt-4o": map[string]any{"ratio": -1}}},
		"empty model":    {"model_configs": map[string]any{" ": map[string]any{"ratio": 1}}},
		"bad tooling":    {"tooling": "{not json"},
	} {
		t.Run(name, func(t *testing.T) {
			envelope := servePricingFailure(t, router, channel.UUID, payload)
			require.NotEmpty(t, envelope.Message)
			requireModelRatio(t, loadPricingChannel(t, channel.Id), "gpt-4o", 5)
		})
	}
}

// servePricingFailure sends a pricing request that the handler must refuse.
// Parameters: t is the test handle, router serves the request, channelUUID selects the
// channel, and payload is the JSON body.
// Returns: the decoded failure envelope; HTTP 200 with success=false is asserted.
func servePricingFailure(t *testing.T, router *gin.Engine, channelUUID string, payload any) testingModelEnvelope {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/api/channel/pricing/"+channelUUID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var envelope testingModelEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	require.False(t, envelope.Success, "the invalid pricing request must be refused")
	return envelope
}

// TestUpdateChannelPricingRefreshesChannelCache verifies routing sees the new pricing
// right away: the in-memory channel cache serves the updated model_configs.
// Parameters: t is the test handle. Returns: no values.
func TestUpdateChannelPricingRefreshesChannelCache(t *testing.T) {
	setupChannelSweepTestEnvironment(t)
	originalMemoryCache := config.MemoryCacheEnabled
	config.MemoryCacheEnabled = true
	t.Cleanup(func() { config.MemoryCacheEnabled = originalMemoryCache })
	channel := insertPricingChannel(t, "pricing-cache", "")

	serveTestingModelJSON(t, newPricingRouter(), http.MethodPut, "/api/channel/pricing/"+channel.UUID, map[string]any{
		"model_configs": map[string]any{"gpt-4o": map[string]any{"ratio": 6}},
	})

	cached, err := model.GetChannelsFromCache("default", "gpt-4o")
	require.NoError(t, err)
	require.Len(t, cached, 1)
	require.Equal(t, channel.Id, cached[0].Id)
	requireModelRatio(t, cached[0], "gpt-4o", 6)
}
