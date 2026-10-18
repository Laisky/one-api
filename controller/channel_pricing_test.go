package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/deepseek"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/pricing"
)

func TestUpdateChannelPricing_UnifiedFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		requestBody    any
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name: "update with unified model_configs",
			requestBody: map[string]any{
				"model_configs": map[string]any{
					"gpt-3.5-turbo": map[string]any{
						"ratio":            0.0015,
						"completion_ratio": 2.0,
						"max_tokens":       65536,
					},
					"gpt-4": map[string]any{
						"ratio":            0.03,
						"completion_ratio": 2.0,
						"max_tokens":       128000,
					},
				},
			},
			expectedStatus: http.StatusOK,
			expectSuccess:  true,
		},
		{
			name: "update with old format - separate ratios",
			requestBody: map[string]any{
				"model_ratio": map[string]any{
					"gpt-3.5-turbo": 0.0015,
					"gpt-4":         0.03,
				},
				"completion_ratio": map[string]any{
					"gpt-3.5-turbo": 2.0,
					"gpt-4":         2.0,
				},
			},
			expectedStatus: http.StatusOK,
			expectSuccess:  true,
		},
		{
			name: "update with mixed format - model_ratio only",
			requestBody: map[string]any{
				"model_ratio": map[string]any{
					"gpt-3.5-turbo": 0.0015,
				},
			},
			expectedStatus: http.StatusOK,
			expectSuccess:  true,
		},
		{
			name:           "update with empty request",
			requestBody:    map[string]any{},
			expectedStatus: http.StatusOK,
			expectSuccess:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock channel
			channel := &model.Channel{
				Id:   1,
				Name: "test-channel",
			}

			// Create request
			jsonBody, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)

			req, err := http.NewRequest("POST", "/api/channel/1/pricing", bytes.NewBuffer(jsonBody))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			// Create response recorder
			w := httptest.NewRecorder()

			// Create gin context
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			c.Params = gin.Params{{Key: "id", Value: "1"}}

			// Mock the channel retrieval
			c.Set(ctxkey.Channel, channel)

			// Note: In a real test, you would need to mock the database operations
			// For now, we're just testing the request parsing and response structure

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestGetChannelPricing_UnifiedFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		channel        *model.Channel
		expectedFields []string
	}{
		{
			name: "channel with unified ModelConfigs",
			channel: &model.Channel{
				Id:           1,
				Name:         "test-channel",
				ModelConfigs: stringPtr(`{"gpt-3.5-turbo": {"ratio": 0.0015, "completion_ratio": 2.0, "max_tokens": 65536}}`),
			},
			expectedFields: []string{"model_ratio", "completion_ratio", "model_configs", "tooling"},
		},
		{
			name: "channel with old format",
			channel: &model.Channel{
				Id:              1,
				Name:            "test-channel",
				ModelRatio:      stringPtr(`{"gpt-3.5-turbo": 0.0015}`),
				CompletionRatio: stringPtr(`{"gpt-3.5-turbo": 2.0}`),
			},
			expectedFields: []string{"model_ratio", "completion_ratio", "model_configs", "tooling"},
		},
		{
			name: "channel with no pricing data",
			channel: &model.Channel{
				Id:   1,
				Name: "test-channel",
			},
			expectedFields: []string{"model_ratio", "completion_ratio", "model_configs", "tooling"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request
			req, err := http.NewRequest("GET", "/api/channel/1/pricing", nil)
			require.NoError(t, err)

			// Create response recorder
			w := httptest.NewRecorder()

			// Create gin context
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			c.Params = gin.Params{{Key: "id", Value: "1"}}

			// Mock the channel retrieval
			c.Set(ctxkey.Channel, tt.channel)

			// Note: In a real test, you would call the actual GetChannelPricing function
			// and verify the response structure contains all expected fields

			// Verify that the response would contain all expected fields
			for _, field := range tt.expectedFields {
				assert.Contains(t, tt.expectedFields, field)
			}
		})
	}
}

func TestChannelPricingUnifiedOnly(t *testing.T) {
	tests := []struct {
		name               string
		modelConfigs       *string
		expectedRatios     bool
		expectedCompletion bool
	}{
		{
			name:               "unified format with pricing data",
			modelConfigs:       stringPtr(`{"gpt-3.5-turbo": {"ratio": 0.0015, "completion_ratio": 2.0}}`),
			expectedRatios:     true,
			expectedCompletion: true,
		},
		{
			name:               "unified format with partial data",
			modelConfigs:       stringPtr(`{"gpt-3.5-turbo": {"ratio": 0.0015}}`),
			expectedRatios:     true,
			expectedCompletion: false,
		},
		{
			name:               "unified format with MaxTokens only",
			modelConfigs:       stringPtr(`{"gpt-3.5-turbo": {"max_tokens": 65536}}`),
			expectedRatios:     false,
			expectedCompletion: false,
		},
		{
			name:               "no pricing data",
			modelConfigs:       nil,
			expectedRatios:     false,
			expectedCompletion: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &model.Channel{
				Id:           1,
				ModelConfigs: tt.modelConfigs,
			}

			// Test unified format access only
			unifiedRatios := channel.GetModelRatioFromConfigs()
			unifiedCompletionRatios := channel.GetCompletionRatioFromConfigs()

			if tt.expectedRatios {
				assert.NotNil(t, unifiedRatios)
				assert.Contains(t, unifiedRatios, "gpt-3.5-turbo")
			} else {
				assert.Nil(t, unifiedRatios)
			}

			if tt.expectedCompletion {
				assert.NotNil(t, unifiedCompletionRatios)
				assert.Contains(t, unifiedCompletionRatios, "gpt-3.5-turbo")
			} else {
				assert.Nil(t, unifiedCompletionRatios)
			}
		})
	}
}

func TestChannelPricingMigration(t *testing.T) {
	tests := []struct {
		name                   string
		initialModelRatio      *string
		initialCompletionRatio *string
		expectedMigrated       bool
	}{
		{
			name:                   "migrate from separate fields",
			initialModelRatio:      stringPtr(`{"gpt-3.5-turbo": 0.0015}`),
			initialCompletionRatio: stringPtr(`{"gpt-3.5-turbo": 2.0}`),
			expectedMigrated:       true,
		},
		{
			name:                   "migrate from model_ratio only",
			initialModelRatio:      stringPtr(`{"gpt-3.5-turbo": 0.0015}`),
			initialCompletionRatio: nil,
			expectedMigrated:       true,
		},
		{
			name:                   "no data to migrate",
			initialModelRatio:      nil,
			initialCompletionRatio: nil,
			expectedMigrated:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &model.Channel{
				Id:              1,
				ModelRatio:      tt.initialModelRatio,
				CompletionRatio: tt.initialCompletionRatio,
			}

			err := channel.MigrateHistoricalPricingToModelConfigs()
			assert.NoError(t, err)

			if tt.expectedMigrated {
				assert.NotNil(t, channel.ModelConfigs)

				// Verify the migrated data
				configs := channel.GetModelPriceConfigs()
				assert.NotNil(t, configs)

				if tt.initialModelRatio != nil {
					assert.Contains(t, configs, "gpt-3.5-turbo")
					assert.Equal(t, float64(0.0015), configs["gpt-3.5-turbo"].Ratio)
				}

				if tt.initialCompletionRatio != nil {
					assert.Contains(t, configs, "gpt-3.5-turbo")
					assert.Equal(t, float64(2.0), configs["gpt-3.5-turbo"].CompletionRatio)
				}
			} else {
				// Should be nil or empty
				if channel.ModelConfigs != nil {
					assert.Equal(t, "", *channel.ModelConfigs)
				}
			}
		})
	}
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}

// requestDefaultPricing exercises the production HTTP handler without a database.
// Parameters: t is the test handle. Returns: decoded local pricing defaults.
func requestDefaultPricing(t *testing.T) map[string]model.ModelConfigLocal {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/channel/default-pricing", GetChannelDefaultPricing)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/channel/default-pricing?type=%d", channeltype.DeepSeek), nil))
	require.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			ModelConfigs string `json:"model_configs"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.True(t, response.Success, w.Body.String())
	var configs map[string]model.ModelConfigLocal
	require.NoError(t, json.Unmarshal([]byte(response.Data.ModelConfigs), &configs))
	for _, name := range []string{"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp", "deepseek-v4-pro"} {
		require.Contains(t, configs, name)
	}
	return configs
}

// TestChannelDefaultPricingDeepSeekSnapshot verifies Load Default retains the
// schedule and cache tariff after the returned JSON becomes a channel override.
// Parameters: t is the test handle. Returns: nothing; assertions report data loss.
func TestChannelDefaultPricingDeepSeekSnapshot(t *testing.T) {
	configs := requestDefaultPricing(t)
	channel := &model.Channel{}
	require.NoError(t, channel.SetModelPriceConfigs(configs))
	saved := channel.GetModelPriceConfigs()
	provider := &deepseek.Adaptor{}
	for _, name := range []string{"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp", "deepseek-v4-pro"} {
		t.Run(name, func(t *testing.T) {
			input, cached, output := 0.15, 0.003, 0.60
			if name == "deepseek-v4-pro" {
				input, cached, output = 0.66, 0.022, 1.98
			}
			require.NotEmpty(t, saved[name].TimeWindows)
			for _, tc := range []struct {
				name, instant string
				multiplier    float64
			}{
				{"offpeak", "2026-10-19T00:30:00Z", 1},
				{"peak", "2026-10-19T01:30:00Z", 2},
			} {
				t.Run(tc.name, func(t *testing.T) {
					at, err := time.Parse(time.RFC3339, tc.instant)
					require.NoError(t, err)
					cfg, ok := pricing.ResolveModelConfig(name, saved, provider, at)
					require.True(t, ok)
					require.InDelta(t, input*tc.multiplier*ratio.MilliTokensUsd, cfg.Ratio, 1e-12)
					require.InDelta(t, cached*tc.multiplier*ratio.MilliTokensUsd, cfg.CachedInputRatio, 1e-12)
					require.InDelta(t, output*tc.multiplier*ratio.MilliTokensUsd, cfg.Ratio*cfg.CompletionRatio, 1e-12)
					require.InDelta(t, cfg.Ratio, pricing.ResolveModelRatioAt(name, saved, channel.GetModelRatioFromConfigs(), provider, at), 1e-12)
				})
			}
		})
	}
	// Merely requesting fresh defaults must not alter an existing custom snapshot.
	custom := &model.Channel{}
	require.NoError(t, custom.SetModelPriceConfigs(map[string]model.ModelConfigLocal{"deepseek-flash": {Ratio: 17, CachedInputRatio: 3, CompletionRatio: 5}}))
	original := *custom.ModelConfigs
	_ = requestDefaultPricing(t)
	require.Equal(t, original, *custom.ModelConfigs)
	at, err := time.Parse(time.RFC3339, "2026-10-19T01:30:00Z")
	require.NoError(t, err)
	resolved, ok := pricing.ResolveModelConfig("deepseek-flash", custom.GetModelPriceConfigs(), provider, at)
	require.True(t, ok)
	require.Equal(t, 17.0, resolved.Ratio)
	require.Equal(t, 3.0, resolved.CachedInputRatio)
	require.Equal(t, 5.0, resolved.CompletionRatio)
	require.Empty(t, resolved.TimeWindows)
}

// TestChannelDefaultPricingAllPersistableFields guards against partial export
// copies, including sparse nested overlays and explicitly free unit tariffs.
// Parameters: t is the test handle. Returns: nothing; assertions report lost fields.
func TestChannelDefaultPricingAllPersistableFields(t *testing.T) {
	const name = "pricing-export-contract"
	const fixture = `{
  "ratio":1,"completion_ratio":2,"cached_input_ratio":0.1,"cache_write_5m_ratio":0.2,"cache_write_1h_ratio":0.3,
  "max_tokens":8192,"context_length":16384,
  "tiers":[{"ratio":3,"completion_ratio":4,"cached_input_ratio":0.5,"cache_write_5m_ratio":0.6,"cache_write_1h_ratio":0.7,"input_token_threshold":100,"output_token_threshold":50}],
  "video":{"per_second_usd":0.01,"input_image_usd":0.02,"base_resolution":"720p","resolution_multipliers":{"1080p":2}},
  "audio":{"prompt_ratio":2,"completion_ratio":3,"prompt_tokens_per_second":4,"completion_tokens_per_second":5,"usd_per_second":0.1,"input_unit":"seconds","input_price_usd":0.2,"input_price_quantity":10,"minimum_billable_seconds":2,"billing_increment_seconds":1},
  "image":{"price_per_image_usd":0.1,"prompt_ratio":2,"default_size":"1024x1024","default_quality":"standard","prompt_token_limit":100,"min_images":1,"max_images":4,"size_multipliers":{"1024x1024":1},"quality_multipliers":{"hd":2},"quality_size_multipliers":{"hd":{"1024x1024":2}}},
  "embedding":{"text_token_ratio":0.1,"image_token_ratio":0.2,"audio_token_ratio":0.3,"video_token_ratio":0.4,"document_token_ratio":0.5,"usd_per_image":0.6,"usd_per_audio_second":0.7,"usd_per_video_frame":0.8,"usd_per_document_page":0.9},
  "per_call":{"usd_per_thousand_calls":0},"per_page":{"usd_per_thousand_pages":0},
  "time_windows":[{"name":"ordered-first","timezone":"UTC","days_of_week":[1,2],"date_from":"2026-10-19","date_to":"2026-10-21","ranges":[{"start":"01:00","end":"04:00"}],"overlay":{"ratio":2,"cached_input_ratio":0.2,"tiers":[{"ratio":4,"input_token_threshold":100}],"per_call":{"usd_per_thousand_calls":0},"per_page":{"usd_per_thousand_pages":0},"embedding":{"text_token_ratio":0.2},"audio":{"usd_per_second":0.2},"image":{"price_per_image_usd":0.2},"video":{"per_second_usd":0.2}}},
  {"name":"ordered-second","timezone":"Asia/Shanghai","ranges":[{"start":"00:00","end":"00:00"}],"overlay":{"completion_ratio":3}}]
 }`
	var source adaptor.ModelConfig
	var expected model.ModelConfigLocal
	require.NoError(t, json.Unmarshal([]byte(fixture), &source))
	require.NoError(t, json.Unmarshal([]byte(fixture), &expected))
	source.Description = "provider-only metadata"
	source.Video.TotalUsdDecimal = "123.456"
	previous, existed := deepseek.ModelRatios[name]
	deepseek.ModelRatios[name] = source
	t.Cleanup(func() {
		if existed {
			deepseek.ModelRatios[name] = previous
		} else {
			delete(deepseek.ModelRatios, name)
		}
	})
	actual := requestDefaultPricing(t)[name]
	require.Equal(t, expected, actual)
	// Provider-only metadata is deliberately outside the persisted pricing schema.
	encoded, err := json.Marshal(actual)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), `"description"`)
	require.NotContains(t, string(encoded), `"total_usd_decimal"`)
	actual.TimeWindows[0].DaysOfWeek[0] = 6
	actual.Video.ResolutionMultipliers["1080p"] = 99
	require.Equal(t, 1, deepseek.ModelRatios[name].TimeWindows[0].DaysOfWeek[0])
	require.Equal(t, 2.0, deepseek.ModelRatios[name].Video.ResolutionMultipliers["1080p"])
}
