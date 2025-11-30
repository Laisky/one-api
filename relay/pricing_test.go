package relay

import (
	"testing"

	"github.com/songquanpeng/one-api/relay/adaptor/ali"
	"github.com/songquanpeng/one-api/relay/adaptor/openrouter"
	"github.com/songquanpeng/one-api/relay/adaptor/xai"
	"github.com/songquanpeng/one-api/relay/apitype"
	"github.com/stretchr/testify/require"
)

// TestAdapterPricingImplementations tests that all major adapters have proper pricing implementations
func TestAdapterPricingImplementations(t *testing.T) {
	testCases := []struct {
		name        string
		apiType     int
		sampleModel string
		expectEmpty bool // true if we expect empty pricing (uses DefaultPricingMethods)
	}{
		{"OpenAI", apitype.OpenAI, "gpt-4", false},
		{"Anthropic", apitype.Anthropic, "claude-3-sonnet-20240229", false},
		{"Zhipu", apitype.Zhipu, "glm-4", false},
		{"Ali", apitype.Ali, "qwen-turbo", false},
		{"Baidu", apitype.Baidu, "ERNIE-4.0-8K", false},
		{"Tencent", apitype.Tencent, "hunyuan-lite", false},
		{"Gemini", apitype.Gemini, "gemini-2.5-flash", false},
		{"Xunfei", apitype.Xunfei, "Spark-Lite", false},
		{"VertexAI", apitype.VertexAI, "gemini-2.5-flash", false},
		{"xAI", apitype.XAI, "grok-3", false},
		{"AWS Bedrock/Mistral AI", apitype.AwsClaude, "mistral-pixtral-large-2502", false},
		// Adapters that still use DefaultPricingMethods (expected to have empty pricing)
		{"Ollama", apitype.Ollama, "llama2", true},
		{"Cohere", apitype.Cohere, "command", false},
		{"Coze", apitype.Coze, "coze-chat", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			adaptor := GetAdaptor(tc.apiType)
			require.NotNil(t, adaptor, "No adaptor found for %s (type: %d)", tc.name, tc.apiType)

			// Test GetDefaultModelPricing
			defaultPricing := adaptor.GetDefaultModelPricing()

			if tc.expectEmpty {
				require.Empty(t, defaultPricing, "%s: Expected empty pricing map, got %d models", tc.name, len(defaultPricing))
				return // Skip further tests for adapters expected to have empty pricing
			}

			require.NotEmpty(t, defaultPricing, "%s: GetDefaultModelPricing returned empty map", tc.name)

			t.Logf("%s: Found %d models with pricing", tc.name, len(defaultPricing))

			// Test GetModelRatio
			ratio := adaptor.GetModelRatio(tc.sampleModel)
			require.Greater(t, ratio, 0.0, "%s: GetModelRatio for %s returned invalid ratio: %f", tc.name, tc.sampleModel, ratio)

			// Test GetCompletionRatio
			completionRatio := adaptor.GetCompletionRatio(tc.sampleModel)
			require.Greater(t, completionRatio, 0.0, "%s: GetCompletionRatio for %s returned invalid ratio: %f", tc.name, tc.sampleModel, completionRatio)

			t.Logf("%s: Model %s - ratio=%.6f, completion_ratio=%.2f", tc.name, tc.sampleModel, ratio, completionRatio)
		})
	}
}

// TestSpecificAdapterPricing tests specific pricing for individual adapters
func TestSpecificAdapterPricing(t *testing.T) {
	t.Run("Ali_Pricing", func(t *testing.T) {
		adaptor := GetAdaptor(apitype.Ali)
		require.NotNil(t, adaptor, "Ali adaptor not found")

		// Verify adapter pricing matches the authoritative ModelRatios table.
		testModels := []string{"qwen-turbo", "qwen-plus", "qwen-max"}

		for _, model := range testModels {
			expectedConfig, ok := ali.ModelRatios[model]
			require.True(t, ok, "Ali model %s missing from ModelRatios", model)

			ratio := adaptor.GetModelRatio(model)
			completionRatio := adaptor.GetCompletionRatio(model)

			require.Equal(t, expectedConfig.Ratio, ratio, "Ali %s: expected ratio %.6f, got %.6f", model, expectedConfig.Ratio, ratio)
			require.Equal(t, expectedConfig.CompletionRatio, completionRatio, "Ali %s: expected completion ratio %.2f, got %.2f", model, expectedConfig.CompletionRatio, completionRatio)
		}
	})

	// H0llyW00dzZ: I'm writing this test myself now because this codebase is too complex.
	t.Run("OpenRouter_Pricing", func(t *testing.T) {
		adaptor := &openrouter.Adaptor{}

		// Test GetDefaultModelPricing
		defaultPricing := adaptor.GetDefaultModelPricing()
		require.NotEmpty(t, defaultPricing, "OpenRouter: GetDefaultModelPricing returned empty map")

		t.Logf("OpenRouter: Found %d models with pricing", len(defaultPricing))

		// Test specific OpenRouter models to ensure they have proper pricing
		testModels := map[string]struct {
			expectValidRatio      bool
			expectValidCompletion bool
		}{
			"openai/gpt-4o":                    {true, true},
			"anthropic/claude-3-sonnet":        {true, true},
			"meta-llama/llama-3.1-8b-instruct": {true, true},
		}

		for model, expected := range testModels {
			ratio := adaptor.GetModelRatio(model)
			completionRatio := adaptor.GetCompletionRatio(model)

			if expected.expectValidRatio {
				require.Greater(t, ratio, 0.0, "OpenRouter %s: expected valid ratio, got %.6f", model, ratio)
			}
			if expected.expectValidCompletion {
				require.Greater(t, completionRatio, 0.0, "OpenRouter %s: expected valid completion ratio, got %.2f", model, completionRatio)
			}

			t.Logf("OpenRouter %s: ratio=%.6f, completion_ratio=%.2f", model, ratio, completionRatio)
		}
	})

	t.Run("xAI_Pricing", func(t *testing.T) {
		adaptor := GetAdaptor(apitype.XAI)
		require.NotNil(t, adaptor, "xAI_Pricing not found")

		testModels := map[string]string{
			"grok-code-fast-1":          "$0.20 input, $0.02 cached input, $1.50 output",
			"grok-4-0709":               "$3.00 input, $15.00 output",
			"grok-4-fast-reasoning":     "$0.20 input, $0.05 cached input, $0.50 output",
			"grok-4-fast":               "$0.20 input, $0.05 cached input, $0.50 output",
			"grok-4-fast-non-reasoning": "$0.20 input, $0.05 cached input, $0.50 output",
			"grok-3":                    "$3.00 input, $15.00 output",
			"grok-3-mini":               "$0.30 input, $0.50 output",
			"grok-2-vision-1212":        "$2.00 input, $10.00 output",
		}

		for model, description := range testModels {
			expectedConfig, ok := xai.ModelRatios[model]
			require.True(t, ok, "xAI model %s missing from ModelRatios", model)

			ratio := adaptor.GetModelRatio(model)
			completionRatio := adaptor.GetCompletionRatio(model)

			require.Equal(t, expectedConfig.Ratio, ratio, "xAI %s: expected ratio %.6f, got %.6f (%s)",
				model, expectedConfig.Ratio, ratio, description)
			require.Equal(t, expectedConfig.CompletionRatio, completionRatio, "xAI %s: expected completion ratio %.2f, got %.2f (%s)",
				model, expectedConfig.CompletionRatio, completionRatio, description)

			t.Logf("xAI %s: ratio=%.6f (expected %.6f), completion_ratio=%.2f (expected %.2f) - %s",
				model, ratio, expectedConfig.Ratio, completionRatio, expectedConfig.CompletionRatio,
				description)
		}

		imageModels := []string{"grok-2-image-1212", "grok-2-image"}
		for _, imageModel := range imageModels {
			expectedImageConfig, ok := xai.ModelRatios[imageModel]
			require.True(t, ok, "xAI image model %s missing from ModelRatios", imageModel)

			imageModelRatio := adaptor.GetModelRatio(imageModel)
			imageModelCompletionRatio := adaptor.GetCompletionRatio(imageModel)

			require.Equal(t, expectedImageConfig.Ratio, imageModelRatio, "xAI %s: expected ratio %.6f, got %.6f (Image model: $%.2f per image)",
				imageModel, expectedImageConfig.Ratio, imageModelRatio, expectedImageConfig.Image.PricePerImageUsd)
			require.Equal(t, expectedImageConfig.CompletionRatio, imageModelCompletionRatio, "xAI %s: expected completion ratio %.2f, got %.2f",
				imageModel, expectedImageConfig.CompletionRatio, imageModelCompletionRatio)

			t.Logf("xAI %s: ratio=%.6f (expected %.6f), completion_ratio=%.2f (expected %.2f) - $%.2f per image",
				imageModel, imageModelRatio, expectedImageConfig.Ratio, imageModelCompletionRatio, expectedImageConfig.CompletionRatio,
				expectedImageConfig.Image.PricePerImageUsd)
		}
	})

	t.Run("Gemini_Pricing", func(t *testing.T) {
		adaptor := GetAdaptor(apitype.Gemini)
		require.NotNil(t, adaptor, "Gemini adaptor not found")

		// Gemini uses USD pricing with ratio.MilliTokensUsd = 0.5
		testModels := map[string]struct {
			expectedRatio           float64
			expectedCompletionRatio float64
		}{
			"gemini-2.5-pro":   {1.25 * 0.5, 10.0 / 1.25},
			"gemini-2.5-flash": {0.30 * 0.5, 2.5 / 0.30},
			"gemini-2.0-flash": {0.10 * 0.5, 0.40 / 0.10},
		}

		for model, expected := range testModels {
			ratio := adaptor.GetModelRatio(model)
			completionRatio := adaptor.GetCompletionRatio(model)

			require.Equal(t, expected.expectedRatio, ratio, "Gemini %s: expected ratio %.9f, got %.9f", model, expected.expectedRatio, ratio)
			require.Equal(t, expected.expectedCompletionRatio, completionRatio, "Gemini %s: expected completion ratio %.2f, got %.2f", model, expected.expectedCompletionRatio, completionRatio)
		}
	})

	t.Run("VertexAI_Pricing", func(t *testing.T) {
		adaptor := GetAdaptor(apitype.VertexAI)
		require.NotNil(t, adaptor, "VertexAI adaptor not found")

		// VertexAI should have the same pricing as Gemini for shared models
		testModels := []string{"gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.0-flash"}

		for _, model := range testModels {
			ratio := adaptor.GetModelRatio(model)
			completionRatio := adaptor.GetCompletionRatio(model)

			require.Greater(t, ratio, 0.0, "VertexAI %s: invalid ratio %.9f", model, ratio)
			require.Greater(t, completionRatio, 0.0, "VertexAI %s: invalid completion ratio %.2f", model, completionRatio)

			t.Logf("VertexAI %s: ratio=%.9f, completion_ratio=%.2f", model, ratio, completionRatio)
		}
	})
}

// TestPricingConsistency tests that pricing methods are consistent
func TestPricingConsistency(t *testing.T) {
	adapters := []struct {
		name    string
		apiType int
	}{
		{"Ali", apitype.Ali},
		{"Baidu", apitype.Baidu},
		{"Tencent", apitype.Tencent},
		{"Gemini", apitype.Gemini},
		{"Xunfei", apitype.Xunfei},
		{"VertexAI", apitype.VertexAI},
		{"xAI", apitype.XAI},
	}

	for _, adapter := range adapters {
		t.Run(adapter.name+"_Consistency", func(t *testing.T) {
			adaptor := GetAdaptor(adapter.apiType)
			require.NotNil(t, adaptor, "%s adaptor not found", adapter.name)

			defaultPricing := adaptor.GetDefaultModelPricing()
			require.NotEmpty(t, defaultPricing, "%s: No default pricing found", adapter.name)

			// Test that GetModelRatio and GetCompletionRatio return consistent values
			// with what's in GetDefaultModelPricing
			for model, expectedPrice := range defaultPricing {
				actualRatio := adaptor.GetModelRatio(model)
				actualCompletionRatio := adaptor.GetCompletionRatio(model)

				require.Equal(t, expectedPrice.Ratio, actualRatio, "%s %s: GetModelRatio (%.9f) != DefaultModelPricing.Ratio (%.9f)",
					adapter.name, model, actualRatio, expectedPrice.Ratio)

				require.Equal(t, expectedPrice.CompletionRatio, actualCompletionRatio, "%s %s: GetCompletionRatio (%.2f) != DefaultModelPricing.CompletionRatio (%.2f)",
					adapter.name, model, actualCompletionRatio, expectedPrice.CompletionRatio)
			}
		})
	}
}

// TestFallbackPricing tests that adapters return reasonable fallback pricing for unknown models
func TestFallbackPricing(t *testing.T) {
	adapters := []struct {
		name    string
		apiType int
	}{
		{"Ali", apitype.Ali},
		{"Baidu", apitype.Baidu},
		{"Tencent", apitype.Tencent},
		{"Gemini", apitype.Gemini},
		{"Xunfei", apitype.Xunfei},
		{"VertexAI", apitype.VertexAI},
		{"xAI", apitype.XAI},
		{"AWS Bedrock", apitype.AwsClaude},
	}

	unknownModel := "unknown-test-model-12345"

	for _, adapter := range adapters {
		t.Run(adapter.name+"_Fallback", func(t *testing.T) {
			adaptor := GetAdaptor(adapter.apiType)
			require.NotNil(t, adaptor, "%s adaptor not found", adapter.name)

			// Test fallback pricing for unknown model
			ratio := adaptor.GetModelRatio(unknownModel)
			completionRatio := adaptor.GetCompletionRatio(unknownModel)

			require.Greater(t, ratio, 0.0, "%s: Fallback ratio for unknown model should be > 0, got %.9f", adapter.name, ratio)

			require.Greater(t, completionRatio, 0.0, "%s: Fallback completion ratio for unknown model should be > 0, got %.2f", adapter.name, completionRatio)

			t.Logf("%s fallback pricing: ratio=%.9f, completion_ratio=%.2f", adapter.name, ratio, completionRatio)
		})
	}
}
