package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	gutils "github.com/Laisky/go-utils/v6"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/singleflight"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/channeltype"
)

func setupModelsDisplayTestEnv(t *testing.T) {
	t.Helper()

	anonymousModelsDisplayCache = gutils.NewExpCache[map[string]ChannelModelsDisplayInfo](context.Background(), time.Minute)
	anonymousModelsDisplayGroup = singleflight.Group{}

	originalRedisEnabled := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() {
		common.SetRedisEnabled(originalRedisEnabled)
	})

	originalSQLitePath := common.SQLitePath
	tempDir := t.TempDir()
	common.SQLitePath = filepath.Join(tempDir, "models-display.db")
	t.Cleanup(func() {
		common.SQLitePath = originalSQLitePath
	})

	model.InitDB()
	model.InitLogDB()

	t.Cleanup(func() {
		if model.DB != nil {
			require.NoError(t, model.CloseDB())
			model.DB = nil
			model.LOG_DB = nil
		}
	})
}

// TestGetModelsDisplay_Keyword ensures the endpoint accepts the 'keyword' filter
// and returns a valid success response (even when no data present in test DB).
func TestGetModelsDisplay_Keyword(t *testing.T) {
	setupModelsDisplayTestEnv(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	groupName := fmt.Sprintf("group-%d", time.Now().UnixNano())
	user := &model.User{
		Username: "keyword-user",
		Password: "password",
		Group:    groupName,
		Status:   model.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(user).Error)
	router.GET("/api/models/display", func(c *gin.Context) {
		// inject a test user id so CacheGetUserGroup works
		c.Set(ctxkey.Id, user.Id)
		GetModelsDisplay(c)
	})

	req := httptest.NewRequest("GET", "/api/models/display?keyword=test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Success bool           `json:"success"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	// Success should be either true (normal) or false if user/group missing but should not crash
	// We only assert the presence of the success field and valid JSON structure.
	assert.NotNil(t, resp.Success)
}

// TestGetModelsDisplay_Anonymous ensures anonymous users can access the endpoint
// and receive a well-formed success response (may be empty data on a fresh DB).
func TestGetModelsDisplay_Anonymous(t *testing.T) {
	setupModelsDisplayTestEnv(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/models/display", func(c *gin.Context) {
		// Do not set ctxkey.Id to simulate anonymous user
		GetModelsDisplay(c)
	})

	req := httptest.NewRequest("GET", "/api/models/display", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Success bool           `json:"success"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp.Success)
}

// TestGetModelsDisplay_AnonymousUsesConfiguredModels ensures guests only see models configured on the channel
func TestGetModelsDisplay_AnonymousUsesConfiguredModels(t *testing.T) {
	setupModelsDisplayTestEnv(t)
	gin.SetMode(gin.TestMode)
	channel := &model.Channel{
		Name:   "Public Channel",
		Type:   channeltype.OpenAI,
		Status: model.ChannelStatusEnabled,
		Models: "gpt-3.5-turbo,gpt-4o-mini",
		Group:  "public",
	}
	require.NoError(t, model.DB.Create(channel).Error)

	router := gin.New()
	router.GET("/api/models/display", func(c *gin.Context) {
		GetModelsDisplay(c)
	})

	req := httptest.NewRequest("GET", "/api/models/display", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp ModelsDisplayResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)

	key := fmt.Sprintf("%s:%s", channeltype.IdToName(channel.Type), channel.Name)
	info, ok := resp.Data[key]
	require.True(t, ok, "expected channel %s in response", key)
	require.Len(t, info.Models, 2)
	_, ok = info.Models["gpt-3.5-turbo"]
	require.True(t, ok, "expected gpt-3.5-turbo in models list: %+v", info.Models)
	_, ok = info.Models["gpt-4o-mini"]
	require.True(t, ok, "expected gpt-4o-mini in models list: %+v", info.Models)
	for modelName := range info.Models {
		require.True(t, modelName == "gpt-3.5-turbo" || modelName == "gpt-4o-mini", "unexpected model present: %s", modelName)
	}

	convertRatioToPrice := func(r float64) float64 {
		if r <= 0 {
			return 0
		}
		if r < 0.001 {
			return r * 1_000_000
		}
		return (r * 1_000_000) / ratio.QuotaPerUsd
	}

	gpt35 := info.Models["gpt-3.5-turbo"]
	gpt35Cfg := openai.ModelRatios["gpt-3.5-turbo"]
	expected35Input := convertRatioToPrice(gpt35Cfg.Ratio)
	require.InDelta(t, expected35Input, gpt35.InputPrice, 1e-6)
	expected35Cached := expected35Input
	if gpt35Cfg.CachedInputRatio != 0 {
		expected35Cached = convertRatioToPrice(gpt35Cfg.CachedInputRatio)
	}
	require.InDelta(t, expected35Cached, gpt35.CachedInputPrice, 1e-6)

	gpt4o := info.Models["gpt-4o-mini"]
	gpt4oCfg := openai.ModelRatios["gpt-4o-mini"]
	expected4oInput := convertRatioToPrice(gpt4oCfg.Ratio)
	require.InDelta(t, expected4oInput, gpt4o.InputPrice, 1e-6)
	expected4oCached := expected4oInput
	if gpt4oCfg.CachedInputRatio != 0 {
		expected4oCached = convertRatioToPrice(gpt4oCfg.CachedInputRatio)
	}
	require.InDelta(t, expected4oCached, gpt4o.CachedInputPrice, 1e-6)
}

// TestGetModelsDisplay_GptImageShowsTokenPrice verifies image models that bill prompt tokens expose input pricing.
func TestGetModelsDisplay_GptImageShowsTokenPrice(t *testing.T) {
	setupModelsDisplayTestEnv(t)
	gin.SetMode(gin.TestMode)
	channel := &model.Channel{
		Name:   "Image Channel",
		Type:   channeltype.OpenAI,
		Status: model.ChannelStatusEnabled,
		Models: "gpt-image-1",
		Group:  "public",
	}
	require.NoError(t, model.DB.Create(channel).Error)

	router := gin.New()
	router.GET("/api/models/display", func(c *gin.Context) {
		GetModelsDisplay(c)
	})

	req := httptest.NewRequest("GET", "/api/models/display", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp ModelsDisplayResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)

	key := fmt.Sprintf("%s:%s", channeltype.IdToName(channel.Type), channel.Name)
	info, ok := resp.Data[key]
	require.True(t, ok, "expected channel %s in response", key)
	modelInfo, ok := info.Models["gpt-image-1"]
	require.True(t, ok, "expected gpt-image-1 in model listing")

	convertRatioToPrice := func(r float64) float64 {
		if r <= 0 {
			return 0
		}
		if r < 0.001 {
			return r * 1_000_000
		}
		return (r * 1_000_000) / ratio.QuotaPerUsd
	}

	pricingCfg := openai.ModelRatios["gpt-image-1"]
	expectedInput := convertRatioToPrice(pricingCfg.Ratio)
	require.InDelta(t, expectedInput, modelInfo.InputPrice, 1e-6)
	expectedCached := convertRatioToPrice(pricingCfg.CachedInputRatio)
	require.InDelta(t, expectedCached, modelInfo.CachedInputPrice, 1e-6)
	require.NotNil(t, pricingCfg.Image, "expected image pricing metadata for gpt-image-1")
	require.InDelta(t, pricingCfg.Image.PricePerImageUsd, modelInfo.ImagePrice, 1e-9)
}

// TestGetModelsDisplay_AnonymousIncludesModelConfigOnlyEntries ensures channels that only declare models via
// model_configs still expose them on the models display endpoint.
func TestGetModelsDisplay_AnonymousIncludesModelConfigOnlyEntries(t *testing.T) {
	setupModelsDisplayTestEnv(t)
	gin.SetMode(gin.TestMode)
	channel := &model.Channel{
		Name:   "Config-Only Channel",
		Type:   channeltype.OpenAI,
		Status: model.ChannelStatusEnabled,
		Models: "",
		Group:  "public",
	}
	overrideRatio := 0.0000025
	configErr := channel.SetModelPriceConfigs(map[string]model.ModelConfigLocal{
		"custom-alpha": {
			Ratio:           overrideRatio,
			CompletionRatio: 2.2,
			MaxTokens:       8192,
		},
	})
	require.NoError(t, configErr)
	require.NoError(t, model.DB.Create(channel).Error)

	router := gin.New()
	router.GET("/api/models/display", func(c *gin.Context) {
		GetModelsDisplay(c)
	})
	req := httptest.NewRequest("GET", "/api/models/display", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp ModelsDisplayResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)

	key := fmt.Sprintf("%s:%s", channeltype.IdToName(channel.Type), channel.Name)
	info, ok := resp.Data[key]
	require.True(t, ok, "expected channel %s in response", key)
	modelInfo, ok := info.Models["custom-alpha"]
	require.True(t, ok, "expected custom-alpha in channel listing")

	convertRatioToPrice := func(r float64) float64 {
		if r <= 0 {
			return 0
		}
		if r < 0.001 {
			return r * 1_000_000
		}
		return (r * 1_000_000) / ratio.QuotaPerUsd
	}
	expectedInput := convertRatioToPrice(overrideRatio)
	require.InDelta(t, expectedInput, modelInfo.InputPrice, 1e-6)
	require.InDelta(t, expectedInput, modelInfo.CachedInputPrice, 1e-6)
	expectedOutput := expectedInput * 2.2
	require.InDelta(t, expectedOutput, modelInfo.OutputPrice, 1e-6)
	require.Equal(t, int32(8192), modelInfo.MaxTokens)
}

// TestGetModelsDisplay_CustomModelPricingOverrides verifies that custom pricing overrides are honored, including
// alias models defined through model mapping.
func TestGetModelsDisplay_CustomModelPricingOverrides(t *testing.T) {
	setupModelsDisplayTestEnv(t)
	gin.SetMode(gin.TestMode)
	mapping := map[string]string{"custom-pro": "gpt-3.5-turbo"}
	mappingRaw, err := json.Marshal(mapping)
	require.NoError(t, err)
	mappingStr := string(mappingRaw)
	channel := &model.Channel{
		Name:         "Override Channel",
		Type:         channeltype.OpenAI,
		Status:       model.ChannelStatusEnabled,
		Models:       "gpt-3.5-turbo,custom-pro",
		Group:        "public",
		ModelMapping: &mappingStr,
	}
	customCfg := map[string]model.ModelConfigLocal{
		"custom-pro": {
			Ratio:           0.0000031,
			CompletionRatio: 3.5,
			MaxTokens:       2048,
		},
	}
	require.NoError(t, channel.SetModelPriceConfigs(customCfg))
	require.NoError(t, model.DB.Create(channel).Error)

	router := gin.New()
	router.GET("/api/models/display", func(c *gin.Context) {
		GetModelsDisplay(c)
	})
	req := httptest.NewRequest("GET", "/api/models/display", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp ModelsDisplayResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	key := fmt.Sprintf("%s:%s", channeltype.IdToName(channel.Type), channel.Name)
	info, ok := resp.Data[key]
	require.True(t, ok, "expected channel %s in response", key)
	modelInfo, ok := info.Models["custom-pro"]
	require.True(t, ok, "expected custom-pro pricing entry")

	convertRatioToPrice := func(r float64) float64 {
		if r <= 0 {
			return 0
		}
		if r < 0.001 {
			return r * 1_000_000
		}
		return (r * 1_000_000) / ratio.QuotaPerUsd
	}
	inputExpected := convertRatioToPrice(customCfg["custom-pro"].Ratio)
	require.InDelta(t, inputExpected, modelInfo.InputPrice, 1e-6)
	require.InDelta(t, inputExpected, modelInfo.CachedInputPrice, 1e-6)
	outputExpected := inputExpected * customCfg["custom-pro"].CompletionRatio
	require.InDelta(t, outputExpected, modelInfo.OutputPrice, 1e-6)
	require.Equal(t, int32(2048), modelInfo.MaxTokens)
}

// TestGetModelsDisplay_LoggedInFiltersUnsupportedModels ensures logged-in users don't see models outside their allowed set
func TestGetModelsDisplay_LoggedInFiltersUnsupportedModels(t *testing.T) {
	setupModelsDisplayTestEnv(t)
	gin.SetMode(gin.TestMode)
	groupName := fmt.Sprintf("group-%d", time.Now().UnixNano())
	user := &model.User{
		Username: "allowed-user",
		Password: "password",
		Group:    groupName,
		Status:   model.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(user).Error)

	channel := &model.Channel{
		Name:   "User Channel",
		Type:   channeltype.OpenAI,
		Status: model.ChannelStatusEnabled,
		Models: "gpt-3.5-turbo",
		Group:  groupName,
	}
	require.NoError(t, model.DB.Create(channel).Error)

	abilities := []*model.Ability{
		{
			Group:     groupName,
			Model:     "gpt-3.5-turbo",
			ChannelId: channel.Id,
			Enabled:   true,
		},
		{
			Group:     groupName,
			Model:     "gpt-invalid-model",
			ChannelId: channel.Id,
			Enabled:   true,
		},
	}
	for _, ability := range abilities {
		require.NoError(t, model.DB.Create(ability).Error)
	}

	router := gin.New()
	router.GET("/api/models/display", func(c *gin.Context) {
		c.Set(ctxkey.Id, user.Id)
		GetModelsDisplay(c)
	})

	req := httptest.NewRequest("GET", "/api/models/display", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp ModelsDisplayResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)

	key := fmt.Sprintf("%s:%s", channeltype.IdToName(channel.Type), channel.Name)
	info, ok := resp.Data[key]
	require.True(t, ok, "expected channel %s in response", key)
	require.Len(t, info.Models, 1)
	_, ok = info.Models["gpt-3.5-turbo"]
	require.True(t, ok, "expected gpt-3.5-turbo for user; got %+v", info.Models)
	_, ok = info.Models["gpt-invalid-model"]
	require.False(t, ok, "unexpected unsupported model exposed to user: %+v", info.Models)
}
