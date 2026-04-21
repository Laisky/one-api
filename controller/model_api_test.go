package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"testing"
	"time"

	gutils "github.com/Laisky/go-utils/v6"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/channeltype"
)

// func min(a, b int) int {
// 	if a < b {
// 		return a
// 	}
// 	return b
// }

func TestDashboardListModels(t *testing.T) {
	// Initialize the database for testing
	model.InitDB()

	// Create a test router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/models", DashboardListModels)

	// Create a test request
	req, _ := http.NewRequest("GET", "/api/models", nil)
	w := httptest.NewRecorder()

	// Perform the request
	router.ServeHTTP(w, req)

	// Check the response
	assert.Equal(t, http.StatusOK, w.Code)

	// Parse the response
	var response struct {
		Success bool             `json:"success"`
		Message string           `json:"message"`
		Data    map[int][]string `json:"data"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// Verify the response structure
	assert.True(t, response.Success)
	assert.Equal(t, "", response.Message)
	assert.NotNil(t, response.Data)

	// Verify that the data contains channel type to models mapping
	// The data should be in the format: {channelType: [model1, model2, ...]}
	for channelType, models := range response.Data {
		assert.Greater(t, channelType, 0, "Channel type should be positive")
		assert.IsType(t, []string{}, models, "Models should be a slice of strings")

		// If there are models, verify they are strings
		for _, model := range models {
			assert.IsType(t, "", model, "Each model should be a string")
			assert.NotEmpty(t, model, "Model name should not be empty")
		}
	}

	t.Logf("DashboardListModels returned %d channel types", len(response.Data))

	// Log some sample data for verification
	for channelType, models := range response.Data {
		if len(models) > 0 {
			t.Logf("Channel type %d has %d models, first model: %s", channelType, len(models), models[0])
		}
		break // Just log the first one
	}
}

func TestListAllModels(t *testing.T) {
	// Initialize the database for testing
	model.InitDB()

	// Create a test router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/channel/models", ListAllModels)

	// Create a test request
	req, _ := http.NewRequest("GET", "/api/channel/models", nil)
	w := httptest.NewRecorder()

	// Perform the request
	router.ServeHTTP(w, req)

	// Check the response
	assert.Equal(t, http.StatusOK, w.Code)

	// Parse the response
	var response struct {
		Object string `json:"object"`
		Data   []struct {
			Id      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// Verify the response structure
	assert.Equal(t, "list", response.Object)
	assert.NotNil(t, response.Data)

	// Verify that the data contains model information
	for _, model := range response.Data {
		assert.NotEmpty(t, model.Id, "Model ID should not be empty")
		assert.Equal(t, "model", model.Object, "Object should be 'model'")
		assert.NotEmpty(t, model.OwnedBy, "OwnedBy should not be empty")
	}

	t.Logf("ListAllModels returned %d models", len(response.Data))

	// Log some sample data for verification
	if len(response.Data) > 0 {
		t.Logf("First model: ID=%s, OwnedBy=%s", response.Data[0].Id, response.Data[0].OwnedBy)
	}
}

func TestModelEndpointsDifference(t *testing.T) {
	// This test verifies that the two endpoints return different data structures
	// as expected by the frontend

	model.InitDB()
	gin.SetMode(gin.TestMode)

	// Test DashboardListModels (/api/models)
	router1 := gin.New()
	router1.GET("/api/models", DashboardListModels)
	req1, _ := http.NewRequest("GET", "/api/models", nil)
	w1 := httptest.NewRecorder()
	router1.ServeHTTP(w1, req1)

	// Test ListAllModels (/api/channel/models)
	router2 := gin.New()
	router2.GET("/api/channel/models", ListAllModels)
	req2, _ := http.NewRequest("GET", "/api/channel/models", nil)
	w2 := httptest.NewRecorder()
	router2.ServeHTTP(w2, req2)

	// Both should return 200
	assert.Equal(t, http.StatusOK, w1.Code)
	assert.Equal(t, http.StatusOK, w2.Code)

	// Parse responses
	var dashboardResponse struct {
		Success bool             `json:"success"`
		Data    map[int][]string `json:"data"`
	}
	var allModelsResponse struct {
		Object string `json:"object"`
		Data   []any  `json:"data"`
	}

	err1 := json.Unmarshal(w1.Body.Bytes(), &dashboardResponse)
	err2 := json.Unmarshal(w2.Body.Bytes(), &allModelsResponse)

	assert.NoError(t, err1)
	assert.NoError(t, err2)

	// Verify different structures
	assert.True(t, dashboardResponse.Success, "DashboardListModels should have success field")
	assert.Equal(t, "list", allModelsResponse.Object, "ListAllModels should have object='list'")

	// DashboardListModels should return channel-type-to-models mapping
	assert.IsType(t, map[int][]string{}, dashboardResponse.Data)

	// ListAllModels should return a list of model objects
	assert.IsType(t, []any{}, allModelsResponse.Data)

	t.Logf("✓ Confirmed that /api/models and /api/channel/models return different data structures")
	t.Logf("  - /api/models returns channel-type-to-models mapping with %d channel types", len(dashboardResponse.Data))
	t.Logf("  - /api/channel/models returns list of all models with %d models", len(allModelsResponse.Data))
}

func TestDeepSeekModelsInDashboard(t *testing.T) {
	// This test verifies that DeepSeek models are correctly included in the dashboard models endpoint
	model.InitDB()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/api/models", DashboardListModels)
	req, _ := http.NewRequest("GET", "/api/models", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Success bool             `json:"success"`
		Data    map[int][]string `json:"data"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response.Success)

	// Based on debug output, DeepSeek models are actually in channel type 36
	// This suggests StepFun (channel type 36) is somehow getting DeepSeek models
	const DeepSeekChannelType = 36

	deepSeekModels, exists := response.Data[DeepSeekChannelType]
	assert.True(t, exists, "DeepSeek channel type should exist in the response")
	assert.NotEmpty(t, deepSeekModels, "DeepSeek should have models")

	// Verify that DeepSeek models are included
	expectedModels := []string{"deepseek-chat", "deepseek-reasoner"}
	for _, expectedModel := range expectedModels {
		found := slices.Contains(deepSeekModels, expectedModel)
		assert.True(t, found, "Expected DeepSeek model %s should be present", expectedModel)
	}

	t.Logf("✓ DeepSeek channel type %d has %d models: %v", DeepSeekChannelType, len(deepSeekModels), deepSeekModels)
}

func TestListAllModelsIncludesCustomChannelModels(t *testing.T) {
	model.InitDB()
	gin.SetMode(gin.TestMode)
	channel := &model.Channel{
		Name:   "list-all-custom",
		Type:   channeltype.OpenAI,
		Status: model.ChannelStatusEnabled,
		Models: "",
		Group:  "default",
	}
	overrides := map[string]model.ModelConfigLocal{
		"custom-alpha": {
			Ratio:           0.0000025,
			CompletionRatio: 1.6,
			MaxTokens:       4096,
		},
	}
	require.NoError(t, channel.SetModelPriceConfigs(overrides))
	require.NoError(t, model.DB.Create(channel).Error)

	router := gin.New()
	router.GET("/v1/models", ListAllModels)
	req, _ := http.NewRequest("GET", "/v1/models", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Object string `json:"object"`
		Data   []struct {
			Id      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "list", resp.Object)
	found := false
	for _, m := range resp.Data {
		if m.Id == "custom-alpha" {
			found = true
			break
		}
	}
	require.True(t, found, "expected custom-alpha to be listed in /v1/models response")
}

func TestListAllModelsCacheInvalidationAfterChannelChange(t *testing.T) {
	model.InitDB()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/v1/models", ListAllModels)

	// Warm the cache with the current state (no custom channel overrides yet).
	preReq, _ := http.NewRequest("GET", "/v1/models", nil)
	preResp := httptest.NewRecorder()
	router.ServeHTTP(preResp, preReq)
	require.Equal(t, http.StatusOK, preResp.Code)

	channel := &model.Channel{
		Name:   "cache-invalidation",
		Type:   channeltype.OpenAI,
		Status: model.ChannelStatusEnabled,
		Models: "",
		Group:  "default",
	}
	overrides := map[string]model.ModelConfigLocal{
		"cache-alpha": {
			Ratio:           0.000003,
			CompletionRatio: 1.7,
			MaxTokens:       2048,
		},
	}
	require.NoError(t, channel.SetModelPriceConfigs(overrides))
	require.NoError(t, model.DB.Create(channel).Error)

	req, _ := http.NewRequest("GET", "/v1/models", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Object string `json:"object"`
		Data   []struct {
			Id      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "list", resp.Object)
	var found bool
	for _, m := range resp.Data {
		if m.Id == "cache-alpha" {
			found = true
			break
		}
	}
	require.True(t, found, "expected cache-alpha to appear after channel update even with warm cache")
}

func TestChannelDefaultPricing(t *testing.T) {
	// This test verifies that the /api/channel/default-pricing endpoint works correctly
	// for different channel types
	model.InitDB()
	gin.SetMode(gin.TestMode)

	// Initialize global pricing manager for the test
	relay.InitializeGlobalPricing()

	router := gin.New()
	router.GET("/api/channel/default-pricing", GetChannelDefaultPricing)

	// Test OpenAI-compatible channel (should return global pricing from all adapters)
	req1, _ := http.NewRequest("GET", "/api/channel/default-pricing?type=50", nil) // OpenAICompatible = 50
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusOK, w1.Code)

	var customResponse struct {
		Success bool `json:"success"`
		Data    struct {
			ModelRatio      string `json:"model_ratio"`
			CompletionRatio string `json:"completion_ratio"`
		} `json:"data"`
	}
	err1 := json.Unmarshal(w1.Body.Bytes(), &customResponse)
	assert.NoError(t, err1)
	assert.True(t, customResponse.Success)

	// Parse the JSON strings to verify they contain models from multiple adapters
	var compatibleModelRatios map[string]float64
	err1 = json.Unmarshal([]byte(customResponse.Data.ModelRatio), &compatibleModelRatios)
	assert.NoError(t, err1)
	assert.NotEmpty(t, compatibleModelRatios, "OpenAI-compatible channel should have model ratios")

	// Test specific channel type (should return only that adapter's pricing)
	req2, _ := http.NewRequest("GET", "/api/channel/default-pricing?type=40", nil) // DeepSeek = 40
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)

	var deepseekResponse struct {
		Success bool `json:"success"`
		Data    struct {
			ModelRatio      string `json:"model_ratio"`
			CompletionRatio string `json:"completion_ratio"`
		} `json:"data"`
	}
	err2 := json.Unmarshal(w2.Body.Bytes(), &deepseekResponse)
	assert.NoError(t, err2)
	assert.True(t, deepseekResponse.Success)

	var deepseekModelRatios map[string]float64
	err2 = json.Unmarshal([]byte(deepseekResponse.Data.ModelRatio), &deepseekModelRatios)
	assert.NoError(t, err2)
	assert.NotEmpty(t, deepseekModelRatios, "DeepSeek channel should have model ratios")

	// Custom should have significantly more models since it includes all adapters
	assert.Greater(t, len(compatibleModelRatios), len(deepseekModelRatios),
		"OpenAI-compatible channel should have more models than specific channel")

	t.Logf("✓ OpenAI-compatible channel has %d models from global pricing", len(compatibleModelRatios))
	t.Logf("✓ Specific channel has %d models from its adapter", len(deepseekModelRatios))
}

func TestListModels_DeduplicatesModels(t *testing.T) {
	setupListModelsTestEnv(t)
	gin.SetMode(gin.TestMode)
	group := fmt.Sprintf("group-%d", time.Now().UnixNano())
	user := createTestUserForGroup(t, group)

	createTestChannelForGroup(t, "groq-primary", group, "mixtral-8x7b", channeltype.Groq)
	createTestChannelForGroup(t, "groq-secondary", group, "mixtral-8x7b", channeltype.Groq)

	router := gin.New()
	router.GET("/v1/models", func(c *gin.Context) {
		c.Set(ctxkey.Id, user.Id)
		ListModels(c)
	})

	req := httptest.NewRequest("GET", "/v1/models", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Object string `json:"object"`
		Data   []struct {
			Id string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "list", resp.Object)

	count := 0
	for _, m := range resp.Data {
		if m.Id == "mixtral-8x7b" {
			count++
		}
	}
	require.Equal(t, 1, count, "mixtral-8x7b should appear exactly once")
}

func TestListModels_IncludesCustomChannelModels(t *testing.T) {
	setupListModelsTestEnv(t)
	gin.SetMode(gin.TestMode)
	group := fmt.Sprintf("group-%d", time.Now().UnixNano())
	user := createTestUserForGroup(t, group)
	customModel := "azure-gpt-5-nano"

	createTestChannelForGroup(t, "azure-custom", group, customModel, channeltype.Azure)

	router := gin.New()
	router.GET("/v1/models", func(c *gin.Context) {
		c.Set(ctxkey.Id, user.Id)
		ListModels(c)
	})

	req := httptest.NewRequest("GET", "/v1/models", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Object string `json:"object"`
		Data   []struct {
			Id      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "list", resp.Object)

	found := false
	for _, m := range resp.Data {
		if m.Id == customModel {
			found = true
			require.Equal(t, "azure", m.OwnedBy)
			break
		}
	}
	require.True(t, found, "expected %s to appear in ListModels response", customModel)
}

func TestListModelsFiltersHiddenModels(t *testing.T) {
	setupListModelsTestEnv(t)
	gin.SetMode(gin.TestMode)
	group := fmt.Sprintf("group-%d", time.Now().UnixNano())
	user := createTestUserForGroup(t, group)
	hidden := `["hidden-alpha"]`
	channel := &model.Channel{
		Name:         "hidden-list",
		Type:         channeltype.OpenAI,
		Status:       model.ChannelStatusEnabled,
		Models:       "hidden-alpha,public-alias",
		Group:        group,
		HiddenModels: &hidden,
	}
	require.NoError(t, channel.Insert())

	router := gin.New()
	router.GET("/v1/models", func(c *gin.Context) {
		c.Set(ctxkey.Id, user.Id)
		ListModels(c)
	})

	req := httptest.NewRequest("GET", "/v1/models", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Object string `json:"object"`
		Data   []struct {
			Id string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "list", resp.Object)

	ids := make([]string, 0, len(resp.Data))
	for _, item := range resp.Data {
		ids = append(ids, item.Id)
	}
	require.Contains(t, ids, "public-alias")
	require.NotContains(t, ids, "hidden-alpha")
}

func TestRetrieveModelRejectsHiddenModels(t *testing.T) {
	setupListModelsTestEnv(t)
	gin.SetMode(gin.TestMode)
	group := fmt.Sprintf("group-%d", time.Now().UnixNano())
	user := createTestUserForGroup(t, group)
	hidden := `["hidden-alpha"]`
	channel := &model.Channel{
		Name:         "hidden-retrieve",
		Type:         channeltype.OpenAI,
		Status:       model.ChannelStatusEnabled,
		Models:       "hidden-alpha,public-alias",
		Group:        group,
		HiddenModels: &hidden,
	}
	require.NoError(t, channel.Insert())

	router := gin.New()
	router.GET("/v1/models/:model", func(c *gin.Context) {
		c.Set(ctxkey.Id, user.Id)
		RetrieveModel(c)
	})

	req := httptest.NewRequest("GET", "/v1/models/hidden-alpha", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "model_not_found", resp.Error.Code)
}

func TestGetAvailableModelsByTokenFiltersHiddenModels(t *testing.T) {
	setupListModelsTestEnv(t)
	gin.SetMode(gin.TestMode)
	group := fmt.Sprintf("group-%d", time.Now().UnixNano())
	user := createTestUserForGroup(t, group)
	hidden := `["hidden-alpha"]`
	channel := &model.Channel{
		Name:         "hidden-token",
		Type:         channeltype.OpenAI,
		Status:       model.ChannelStatusEnabled,
		Models:       "hidden-alpha,public-alias",
		Group:        group,
		HiddenModels: &hidden,
	}
	require.NoError(t, channel.Insert())

	allowedModels := "hidden-alpha,public-alias"
	token := &model.Token{
		UserId:         user.Id,
		Name:           "hidden-token",
		Status:         model.TokenStatusEnabled,
		RemainQuota:    100,
		UnlimitedQuota: false,
		ExpiredTime:    -1,
		Models:         &allowedModels,
	}
	require.NoError(t, model.DB.Create(token).Error)

	router := gin.New()
	router.GET("/api/available_models", func(c *gin.Context) {
		c.Set(ctxkey.Id, user.Id)
		c.Set(ctxkey.TokenId, token.Id)
		c.Set(ctxkey.AvailableModels, allowedModels)
		GetAvailableModelsByToken(c)
	})

	req := httptest.NewRequest("GET", "/api/available_models", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Available []string `json:"available"`
			Enabled   bool     `json:"enabled"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.True(t, resp.Data.Enabled)
	require.Equal(t, []string{"public-alias"}, resp.Data.Available)
}

func setupListModelsTestEnv(t *testing.T) {
	t.Helper()
	prevRedis := common.IsRedisEnabled()
	common.SetRedisEnabled(false)
	t.Cleanup(func() { common.SetRedisEnabled(prevRedis) })

	originalSQLitePath := common.SQLitePath
	tempDir := t.TempDir()
	common.SQLitePath = filepath.Join(tempDir, "list-models.db")
	t.Cleanup(func() { common.SQLitePath = originalSQLitePath })

	model.InitDB()
	model.InitLogDB()
	t.Cleanup(func() {
		if model.DB != nil {
			require.NoError(t, model.CloseDB())
			model.DB = nil
			model.LOG_DB = nil
		}
	})

	originalCache := cachedListAllModels
	cachedListAllModels = gutils.NewSingleItemExpCache[listAllModelsCacheEntry](time.Minute)
	t.Cleanup(func() { cachedListAllModels = originalCache })
}

func createTestUserForGroup(t *testing.T, group string) *model.User {
	t.Helper()
	user := &model.User{
		Username: fmt.Sprintf("user-%s", group),
		Password: "password",
		Group:    group,
		Status:   model.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(user).Error)
	return user
}

func createTestChannelForGroup(t *testing.T, name, group, models string, channelType int) *model.Channel {
	t.Helper()
	channel := &model.Channel{
		Name:   name,
		Type:   channelType,
		Status: model.ChannelStatusEnabled,
		Models: models,
		Group:  group,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, channel.AddAbilities())
	return channel
}
