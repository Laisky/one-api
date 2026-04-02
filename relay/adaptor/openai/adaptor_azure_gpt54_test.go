package openai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

// ---------------------------------------------------------------------------
// 1. Azure URL construction for gpt-5.4 model variants
// ---------------------------------------------------------------------------

func TestAzureGPT54ModelsUseResponseAPI(t *testing.T) {
	t.Parallel()

	gpt54Models := []string{
		"gpt-5.4",
		"gpt-5.4-mini",
		"gpt-5.4-nano",
		"gpt-5.4-pro",
	}

	for _, modelName := range gpt54Models {
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()
			cfg := model.ChannelConfig{APIVersion: "2024-10-21"}
			m := &meta.Meta{
				Mode:            relaymode.ChatCompletions,
				ChannelType:     channeltype.Azure,
				BaseURL:         "https://myresource.openai.azure.com",
				ActualModelName: modelName,
				RequestURLPath:  "/v1/chat/completions",
				Config:          cfg,
			}

			a := &Adaptor{}
			a.Init(m)

			url, err := a.GetRequestURL(m)
			require.NoError(t, err)
			assert.Contains(t, url, "/openai/v1/responses", "gpt-5.4 model %q must use Response API path", modelName)
			assert.Contains(t, url, "api-version=v1", "gpt-5.4 model %q must use api-version=v1", modelName)
			assert.NotContains(t, url, "/deployments/", "Response API URL must NOT contain /deployments/")
		})
	}
}

func TestAzureGPT54MiniURLExact(t *testing.T) {
	t.Parallel()
	cfg := model.ChannelConfig{APIVersion: "2024-10-21"}
	m := &meta.Meta{
		Mode:            relaymode.ChatCompletions,
		ChannelType:     channeltype.Azure,
		BaseURL:         "https://myresource.openai.azure.com",
		ActualModelName: "gpt-5.4-mini",
		RequestURLPath:  "/v1/chat/completions",
		Config:          cfg,
	}

	a := &Adaptor{}
	a.Init(m)

	url, err := a.GetRequestURL(m)
	require.NoError(t, err)
	assert.Equal(t, "https://myresource.openai.azure.com/openai/v1/responses?api-version=v1", url)
}

func TestAzureGPT54OverridesUserAPIVersion(t *testing.T) {
	t.Parallel()

	// Even if the user configured an older API version, gpt-5.4 should override to v1.
	cfg := model.ChannelConfig{APIVersion: "2024-06-01"}
	m := &meta.Meta{
		Mode:            relaymode.ChatCompletions,
		ChannelType:     channeltype.Azure,
		BaseURL:         "https://myresource.openai.azure.com",
		ActualModelName: "gpt-5.4-mini",
		RequestURLPath:  "/v1/chat/completions",
		Config:          cfg,
	}

	a := &Adaptor{}
	a.Init(m)

	url, err := a.GetRequestURL(m)
	require.NoError(t, err)
	assert.Contains(t, url, "api-version=v1", "gpt-5.4-mini must override api-version to v1 regardless of channel config")
}

func TestAzureGPT54ResponseAPIModeAlsoWorks(t *testing.T) {
	t.Parallel()

	// If the user sends directly via /v1/responses, it should also work.
	cfg := model.ChannelConfig{APIVersion: "2024-10-21"}
	m := &meta.Meta{
		Mode:            relaymode.ResponseAPI,
		ChannelType:     channeltype.Azure,
		BaseURL:         "https://myresource.openai.azure.com",
		ActualModelName: "gpt-5.4-mini",
		RequestURLPath:  "/v1/responses",
		Config:          cfg,
	}

	a := &Adaptor{}
	a.Init(m)

	url, err := a.GetRequestURL(m)
	require.NoError(t, err)
	assert.Contains(t, url, "/openai/v1/responses")
	assert.Contains(t, url, "api-version=v1")
}

func TestAzureGPT54ImageGenerationURL(t *testing.T) {
	t.Parallel()

	// Image generation should still use deployment-based URL even for gpt-5.4 models.
	cfg := model.ChannelConfig{APIVersion: "2024-10-21"}
	m := &meta.Meta{
		Mode:            relaymode.ImagesGenerations,
		ChannelType:     channeltype.Azure,
		BaseURL:         "https://myresource.openai.azure.com",
		ActualModelName: "gpt-5.4",
		RequestURLPath:  "/v1/images/generations",
		Config:          cfg,
	}

	a := &Adaptor{}
	a.Init(m)

	url, err := a.GetRequestURL(m)
	require.NoError(t, err)
	assert.Contains(t, url, "/openai/deployments/gpt-5.4/images/generations")
}

func TestAzureGPT54CaseInsensitive(t *testing.T) {
	t.Parallel()

	// Model names with different casing should still route to Response API.
	cases := []string{"GPT-5.4-mini", "Gpt-5.4-Mini", "GPT-5.4-MINI"}
	for _, modelName := range cases {
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()
			require.True(t, azureRequiresResponseAPI(modelName),
				"azureRequiresResponseAPI should be case-insensitive for %q", modelName)
		})
	}
}

// ---------------------------------------------------------------------------
// 2. Contrast with non-gpt-5 models on Azure (deployment-based URL)
// ---------------------------------------------------------------------------

func TestAzureNonGPT5ModelsUseDeploymentURL(t *testing.T) {
	t.Parallel()

	nonGPT5Models := []struct {
		name               string
		expectedAPIVersion string
	}{
		{"gpt-4o-mini", "2024-10-21"},
		{"gpt-4.1-mini", "2024-10-21"},
		{"gpt-4.1-nano", "2024-10-21"},
	}

	for _, tc := range nonGPT5Models {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := model.ChannelConfig{APIVersion: "2024-10-21"}
			m := &meta.Meta{
				Mode:            relaymode.ChatCompletions,
				ChannelType:     channeltype.Azure,
				BaseURL:         "https://myresource.openai.azure.com",
				ActualModelName: tc.name,
				RequestURLPath:  "/v1/chat/completions",
				Config:          cfg,
			}

			a := &Adaptor{}
			a.Init(m)

			url, err := a.GetRequestURL(m)
			require.NoError(t, err)
			assert.Contains(t, url, "/openai/deployments/"+tc.name+"/chat/completions")
			assert.Contains(t, url, "api-version="+tc.expectedAPIVersion)
			assert.NotContains(t, url, "/v1/responses")
		})
	}
}

func TestAzureReasoningModelsUsePreviewAPIVersion(t *testing.T) {
	t.Parallel()

	reasoningModels := []string{"o3", "o3-mini", "o4-mini", "o1"}
	for _, modelName := range reasoningModels {
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()
			cfg := model.ChannelConfig{APIVersion: "2024-10-21"}
			m := &meta.Meta{
				Mode:            relaymode.ChatCompletions,
				ChannelType:     channeltype.Azure,
				BaseURL:         "https://myresource.openai.azure.com",
				ActualModelName: modelName,
				RequestURLPath:  "/v1/chat/completions",
				Config:          cfg,
			}

			a := &Adaptor{}
			a.Init(m)

			url, err := a.GetRequestURL(m)
			require.NoError(t, err)
			assert.Contains(t, url, "api-version=2025-04-01-preview",
				"reasoning model %q should use 2025-04-01-preview", modelName)
			assert.Contains(t, url, "/openai/deployments/"+modelName+"/")
		})
	}
}

// ---------------------------------------------------------------------------
// 3. azureRequiresResponseAPI coverage
// ---------------------------------------------------------------------------

func TestAzureRequiresResponseAPI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		model    string
		expected bool
	}{
		// All gpt-5.x models should require Response API
		{"gpt-5", true},
		{"gpt-5-mini", true},
		{"gpt-5-nano", true},
		{"gpt-5-pro", true},
		{"gpt-5.1", true},
		{"gpt-5.1-codex", true},
		{"gpt-5.2", true},
		{"gpt-5.2-codex", true},
		{"gpt-5.3-chat-latest", true},
		{"gpt-5.4", true},
		{"gpt-5.4-mini", true},
		{"gpt-5.4-nano", true},
		{"gpt-5.4-pro", true},

		// Non-gpt-5 models should NOT require Response API
		{"gpt-4o", false},
		{"gpt-4o-mini", false},
		{"gpt-4.1", false},
		{"gpt-4.1-mini", false},
		{"gpt-4.1-nano", false},
		{"gpt-3.5-turbo", false},
		{"o3", false},
		{"o4-mini", false},
		{"o1", false},
		{"text-embedding-3-small", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.model, func(t *testing.T) {
			t.Parallel()
			result := azureRequiresResponseAPI(tc.model)
			assert.Equal(t, tc.expected, result,
				"azureRequiresResponseAPI(%q) = %v, want %v", tc.model, result, tc.expected)
		})
	}
}

// ---------------------------------------------------------------------------
// 4. shouldForceResponseAPI for Azure with gpt-5.4 models
// ---------------------------------------------------------------------------

func TestShouldForceResponseAPIForAzureGPT54(t *testing.T) {
	t.Parallel()

	gpt54Models := []string{"gpt-5.4", "gpt-5.4-mini", "gpt-5.4-nano", "gpt-5.4-pro"}
	for _, modelName := range gpt54Models {
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()
			m := &meta.Meta{
				ChannelType:     channeltype.Azure,
				ActualModelName: modelName,
			}
			assert.True(t, shouldForceResponseAPI(m),
				"shouldForceResponseAPI must be true for Azure + %q", modelName)
		})
	}
}

func TestShouldNotForceResponseAPIForAzureNonGPT5(t *testing.T) {
	t.Parallel()

	models := []string{"gpt-4o", "gpt-4o-mini", "gpt-4.1-mini", "o3", "o4-mini"}
	for _, modelName := range models {
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()
			m := &meta.Meta{
				ChannelType:     channeltype.Azure,
				ActualModelName: modelName,
			}
			assert.False(t, shouldForceResponseAPI(m),
				"shouldForceResponseAPI must be false for Azure + %q", modelName)
		})
	}
}

// ---------------------------------------------------------------------------
// 5. Pricing / billing for gpt-5.4 models
// ---------------------------------------------------------------------------

func TestGPT54ModelsPricingDefined(t *testing.T) {
	t.Parallel()

	expected := map[string]struct {
		inputPerMillion     float64
		completionRatio     float64
		hasCachedInputRatio bool
		cachedInputPerMil   float64
	}{
		"gpt-5.4":      {2.5, 15 / 2.5, true, 0.25},
		"gpt-5.4-mini": {0.75, 4.5 / 0.75, true, 0.075},
		"gpt-5.4-nano": {0.2, 1.25 / 0.2, true, 0.02},
		"gpt-5.4-pro":  {30, 180 / 30.0, false, 0},
	}

	a := &Adaptor{}

	for modelName, exp := range expected {
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()

			config, exists := ModelRatios[modelName]
			require.True(t, exists, "ModelRatios must contain %q", modelName)

			// Verify input ratio
			expectedRatio := exp.inputPerMillion * ratio.MilliTokensUsd
			assert.InDelta(t, expectedRatio, config.Ratio, 1e-12,
				"input ratio mismatch for %q", modelName)

			// Verify completion ratio
			assert.InDelta(t, exp.completionRatio, config.CompletionRatio, 1e-12,
				"completion ratio mismatch for %q", modelName)

			// Verify cached input ratio
			if exp.hasCachedInputRatio {
				expectedCached := exp.cachedInputPerMil * ratio.MilliTokensUsd
				assert.InDelta(t, expectedCached, config.CachedInputRatio, 1e-12,
					"cached input ratio mismatch for %q", modelName)
			}

			// Verify adaptor methods return correct values
			assert.InDelta(t, config.Ratio, a.GetModelRatio(modelName), 1e-12)
			assert.InDelta(t, config.CompletionRatio, a.GetCompletionRatio(modelName), 1e-12)
		})
	}
}

func TestGPT54ModelsInModelList(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}
	modelList := a.GetModelList()

	gpt54Models := []string{"gpt-5.4", "gpt-5.4-mini", "gpt-5.4-nano", "gpt-5.4-pro"}
	for _, modelName := range gpt54Models {
		found := false
		for _, m := range modelList {
			if m == modelName {
				found = true
				break
			}
		}
		assert.True(t, found, "model list must contain %q", modelName)
	}
}

func TestGPT54MiniPricingConsistency(t *testing.T) {
	t.Parallel()

	// Verify gpt-5.4-mini: $0.75/1M input, $4.50/1M output
	// So completion ratio = 4.5 / 0.75 = 6.0
	config := ModelRatios["gpt-5.4-mini"]

	inputCostPerToken := config.Ratio
	outputCostPerToken := config.Ratio * config.CompletionRatio

	// For 1M tokens
	inputCostPerMillion := inputCostPerToken / ratio.MilliTokensUsd
	outputCostPerMillion := outputCostPerToken / ratio.MilliTokensUsd

	assert.InDelta(t, 0.75, inputCostPerMillion, 1e-9,
		"gpt-5.4-mini input cost should be $0.75/1M tokens")
	assert.InDelta(t, 4.5, outputCostPerMillion, 1e-9,
		"gpt-5.4-mini output cost should be $4.50/1M tokens")
}

func TestGPT54PricingConsistency(t *testing.T) {
	t.Parallel()

	// gpt-5.4: $2.50/1M input, $15.00/1M output
	config := ModelRatios["gpt-5.4"]

	inputCostPerMillion := config.Ratio / ratio.MilliTokensUsd
	outputCostPerMillion := (config.Ratio * config.CompletionRatio) / ratio.MilliTokensUsd

	assert.InDelta(t, 2.5, inputCostPerMillion, 1e-9)
	assert.InDelta(t, 15.0, outputCostPerMillion, 1e-9)
}

func TestGPT54NanoPricingConsistency(t *testing.T) {
	t.Parallel()

	// gpt-5.4-nano: $0.20/1M input, $1.25/1M output
	config := ModelRatios["gpt-5.4-nano"]

	inputCostPerMillion := config.Ratio / ratio.MilliTokensUsd
	outputCostPerMillion := (config.Ratio * config.CompletionRatio) / ratio.MilliTokensUsd

	assert.InDelta(t, 0.2, inputCostPerMillion, 1e-9)
	assert.InDelta(t, 1.25, outputCostPerMillion, 1e-9)
}

func TestGPT54ProPricingConsistency(t *testing.T) {
	t.Parallel()

	// gpt-5.4-pro: $30/1M input, $180/1M output
	config := ModelRatios["gpt-5.4-pro"]

	inputCostPerMillion := config.Ratio / ratio.MilliTokensUsd
	outputCostPerMillion := (config.Ratio * config.CompletionRatio) / ratio.MilliTokensUsd

	assert.InDelta(t, 30.0, inputCostPerMillion, 1e-9)
	assert.InDelta(t, 180.0, outputCostPerMillion, 1e-9)
}

// ---------------------------------------------------------------------------
// 6. Reasoning model detection for gpt-5.4 models
// ---------------------------------------------------------------------------

func TestGPT54IsReasoningModel(t *testing.T) {
	t.Parallel()

	gpt54Models := []string{"gpt-5.4", "gpt-5.4-mini", "gpt-5.4-nano", "gpt-5.4-pro"}
	for _, modelName := range gpt54Models {
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()
			assert.True(t, isModelSupportedReasoning(modelName),
				"%q should be detected as reasoning model", modelName)
		})
	}
}

func TestGPT54VersionExtraction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		model           string
		expectedVersion float64
		ok              bool
	}{
		{"gpt-5.4", 5.4, true},
		{"gpt-5.4-mini", 5.4, true},
		{"gpt-5.4-nano", 5.4, true},
		{"gpt-5.4-pro", 5.4, true},
		{"gpt-4.1-mini", 4.1, true},
		{"gpt-4o-mini", 4, true}, // extracts "4" before 'o'
		{"o3-mini", 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.model, func(t *testing.T) {
			t.Parallel()
			version, ok := extractGptVersion(tc.model)
			assert.Equal(t, tc.ok, ok, "extractGptVersion(%q) ok", tc.model)
			if ok {
				assert.InDelta(t, tc.expectedVersion, version, 1e-9,
					"extractGptVersion(%q) version", tc.model)
			}
		})
	}
}

func TestGPT54ReasoningEffort(t *testing.T) {
	t.Parallel()

	// gpt-5.4-mini is NOT a medium-only reasoning model (no "-chat" suffix),
	// so it should allow all reasoning effort levels.
	gpt54Models := []string{"gpt-5.4", "gpt-5.4-mini", "gpt-5.4-nano", "gpt-5.4-pro"}
	for _, modelName := range gpt54Models {
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()
			assert.False(t, isMediumOnlyReasoningModel(modelName),
				"%q should not be medium-only reasoning model", modelName)

			for _, effort := range []string{"low", "medium", "high"} {
				assert.True(t, isReasoningEffortAllowedForModel(modelName, effort),
					"%q should allow reasoning effort %q", modelName, effort)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 7. IsModelsOnlySupportedByChatCompletionAPI for gpt-5.4 models
// ---------------------------------------------------------------------------

func TestGPT54SupportsResponseAPI(t *testing.T) {
	t.Parallel()

	gpt54Models := []string{"gpt-5.4", "gpt-5.4-mini", "gpt-5.4-nano", "gpt-5.4-pro"}
	for _, modelName := range gpt54Models {
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()
			assert.False(t, IsModelsOnlySupportedByChatCompletionAPI(modelName),
				"%q should NOT be chat-completion-only (must support Response API)", modelName)
		})
	}
}

// ---------------------------------------------------------------------------
// 8. Azure authentication header for gpt-5.4 models
// ---------------------------------------------------------------------------

func TestAzureAuthHeaderForGPT54(t *testing.T) {
	t.Parallel()

	// Azure uses api-key header, not Bearer token.
	m := &meta.Meta{
		ChannelType:     channeltype.Azure,
		ActualModelName: "gpt-5.4-mini",
		APIKey:          "test-azure-key-123",
	}

	a := &Adaptor{}
	a.Init(m)

	// We can't easily test SetupRequestHeader without a full gin context + http.Request,
	// but we verify the channel type is set correctly.
	assert.Equal(t, channeltype.Azure, a.ChannelType)
}

// ---------------------------------------------------------------------------
// 9. DefaultToolingConfig is accessible
// ---------------------------------------------------------------------------

func TestGPT54DefaultToolingConfig(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}
	config := a.DefaultToolingConfig()
	// Just verify it returns a valid config without panic
	_ = config
}

// ---------------------------------------------------------------------------
// 10. GetDefaultModelPricing includes gpt-5.4 models
// ---------------------------------------------------------------------------

func TestDefaultModelPricingIncludesGPT54(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}
	pricing := a.GetDefaultModelPricing()

	gpt54Models := []string{"gpt-5.4", "gpt-5.4-mini", "gpt-5.4-nano", "gpt-5.4-pro"}
	for _, modelName := range gpt54Models {
		config, exists := pricing[modelName]
		assert.True(t, exists, "default pricing must contain %q", modelName)
		assert.Greater(t, config.Ratio, 0.0, "ratio for %q must be positive", modelName)
		assert.Greater(t, config.CompletionRatio, 0.0, "completion ratio for %q must be positive", modelName)
	}
}

// ---------------------------------------------------------------------------
// 11. Broader GPT-5 family coverage on Azure
// ---------------------------------------------------------------------------

func TestAzureAllGPT5ModelsRouteToResponseAPI(t *testing.T) {
	t.Parallel()

	// Every model in ModelRatios that starts with "gpt-5" should use Response API on Azure
	for modelName := range ModelRatios {
		if !isGPT5Model(modelName) {
			continue
		}
		t.Run(modelName, func(t *testing.T) {
			t.Parallel()
			assert.True(t, azureRequiresResponseAPI(modelName),
				"gpt-5 model %q must require Response API on Azure", modelName)

			cfg := model.ChannelConfig{APIVersion: "2024-10-21"}
			m := &meta.Meta{
				Mode:            relaymode.ChatCompletions,
				ChannelType:     channeltype.Azure,
				BaseURL:         "https://myresource.openai.azure.com",
				ActualModelName: modelName,
				RequestURLPath:  "/v1/chat/completions",
				Config:          cfg,
			}

			a := &Adaptor{}
			a.Init(m)

			url, err := a.GetRequestURL(m)
			require.NoError(t, err, "GetRequestURL for %q", modelName)
			assert.Contains(t, url, "/openai/v1/responses",
				"Azure URL for %q must use Response API", modelName)
			assert.Contains(t, url, "api-version=v1",
				"Azure URL for %q must use api-version=v1", modelName)
		})
	}
}

// isGPT5Model checks if a model name starts with "gpt-5" (helper for tests).
func isGPT5Model(modelName string) bool {
	normalized := normalizedModelName(modelName)
	return len(normalized) >= 5 && normalized[:5] == "gpt-5"
}

// ---------------------------------------------------------------------------
// 12. Billing calculation end-to-end for gpt-5.4-mini
// ---------------------------------------------------------------------------

func TestGPT54MiniBillingCalculation(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}

	modelRatio := a.GetModelRatio("gpt-5.4-mini")
	completionRatio := a.GetCompletionRatio("gpt-5.4-mini")

	// Simulate 1000 prompt tokens and 500 completion tokens
	promptTokens := 1000
	completionTokens := 500

	inputCost := float64(promptTokens) * modelRatio
	outputCost := float64(completionTokens) * modelRatio * completionRatio
	totalCost := inputCost + outputCost

	assert.Greater(t, totalCost, 0.0, "total cost must be positive")
	assert.Greater(t, outputCost, inputCost,
		"output cost should be greater than input cost for gpt-5.4-mini (completion ratio > 1)")

	// Verify proportions: output ratio is 6x input
	expectedOutputPerToken := modelRatio * completionRatio
	expectedInputPerToken := modelRatio
	assert.InDelta(t, 6.0, expectedOutputPerToken/expectedInputPerToken, 1e-9,
		"output-to-input cost ratio should be 6.0 for gpt-5.4-mini")
}

// ---------------------------------------------------------------------------
// 13. Azure endpoint support includes ResponseAPI
// ---------------------------------------------------------------------------

func TestAzureEndpointsIncludeResponseAPI(t *testing.T) {
	t.Parallel()

	defaults := channeltype.DefaultEndpointNamesForChannelType(channeltype.Azure)
	found := false
	for _, name := range defaults {
		if name == "response_api" {
			found = true
			break
		}
	}
	assert.True(t, found, "Azure default endpoints must include 'response_api'")
}

func TestAzureEndpointsIncludeChatCompletions(t *testing.T) {
	t.Parallel()

	defaults := channeltype.DefaultEndpointNamesForChannelType(channeltype.Azure)
	found := false
	for _, name := range defaults {
		if name == "chat_completions" {
			found = true
			break
		}
	}
	assert.True(t, found, "Azure default endpoints must include 'chat_completions'")
}

// ---------------------------------------------------------------------------
// 14. GetModelListFromPricing returns gpt-5.4 models
// ---------------------------------------------------------------------------

func TestGetModelListFromPricingIncludesGPT54(t *testing.T) {
	t.Parallel()

	modelList := adaptor.GetModelListFromPricing(ModelRatios)

	gpt54Models := []string{"gpt-5.4", "gpt-5.4-mini", "gpt-5.4-nano", "gpt-5.4-pro"}
	modelSet := make(map[string]bool, len(modelList))
	for _, m := range modelList {
		modelSet[m] = true
	}

	for _, modelName := range gpt54Models {
		assert.True(t, modelSet[modelName], "GetModelListFromPricing must include %q", modelName)
	}
}
