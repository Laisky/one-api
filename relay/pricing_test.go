package relay

import (
	"testing"

	"github.com/songquanpeng/one-api/relay/adaptor/openrouter"
	"github.com/songquanpeng/one-api/relay/adaptor/xai"
	"github.com/songquanpeng/one-api/relay/apitype"
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
		{"Gemini", apitype.Gemini, "gemini-pro", false},
		{"Xunfei", apitype.Xunfei, "Spark-Lite", false},
		{"VertexAI", apitype.VertexAI, "gemini-pro", false},
		{"xAI", apitype.XAI, "grok-beta", false}, // Prefer primary constant; model alias still tested
		{"AWS Bedrock/Mistral AI", apitype.AwsClaude, "mistral-pixtral-large-2502", false},
		// Adapters that still use DefaultPricingMethods (expected to have empty pricing)
		{"Ollama", apitype.Ollama, "llama2", true},
		{"Cohere", apitype.Cohere, "command", false},
		{"Coze", apitype.Coze, "coze-chat", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			adaptor := GetAdaptor(tc.apiType)
			if adaptor == nil {
				t.Fatalf("No adaptor found for %s (type: %d)", tc.name, tc.apiType)
			}

			// Test GetDefaultModelPricing
			defaultPricing := adaptor.GetDefaultModelPricing()

			if tc.expectEmpty {
				if len(defaultPricing) != 0 {
					t.Errorf("%s: Expected empty pricing map, got %d models", tc.name, len(defaultPricing))
				}
				return // Skip further tests for adapters expected to have empty pricing
			}

			if len(defaultPricing) == 0 {
				t.Errorf("%s: GetDefaultModelPricing returned empty map", tc.name)
				return
			}

			t.Logf("%s: Found %d models with pricing", tc.name, len(defaultPricing))

			// Test GetModelRatio
			ratio := adaptor.GetModelRatio(tc.sampleModel)
			if ratio <= 0 {
				t.Errorf("%s: GetModelRatio for %s returned invalid ratio: %f", tc.name, tc.sampleModel, ratio)
			}

			// Test GetCompletionRatio
			completionRatio := adaptor.GetCompletionRatio(tc.sampleModel)
			if completionRatio <= 0 {
				t.Errorf("%s: GetCompletionRatio for %s returned invalid ratio: %f", tc.name, tc.sampleModel, completionRatio)
			}

			t.Logf("%s: Model %s - ratio=%.6f, completion_ratio=%.2f", tc.name, tc.sampleModel, ratio, completionRatio)
		})
	}
}

// TestSpecificAdapterPricing tests specific pricing for individual adapters
func TestSpecificAdapterPricing(t *testing.T) {
	t.Run("Ali_Pricing", func(t *testing.T) {
		adaptor := GetAdaptor(apitype.Ali)
		if adaptor == nil {
			t.Fatal("Ali adaptor not found")
		}

		// Ali uses RMB pricing with ratio.MilliTokensRmb = 3.5
		testModels := map[string]struct {
			expectedRatio           float64
			expectedCompletionRatio float64
		}{
			"qwen-turbo": {0.3 * 3.5, 1.0}, // 0.3 * ratio.MilliTokensRmb
			"qwen-plus":  {0.8 * 3.5, 1.0}, // 0.8 * ratio.MilliTokensRmb
			"qwen-max":   {2.4 * 3.5, 1.0}, // 2.4 * ratio.MilliTokensRmb
		}

		for model, expected := range testModels {
			ratio := adaptor.GetModelRatio(model)
			completionRatio := adaptor.GetCompletionRatio(model)

			if ratio != expected.expectedRatio {
				t.Errorf("Ali %s: expected ratio %.6f, got %.6f", model, expected.expectedRatio, ratio)
			}
			if completionRatio != expected.expectedCompletionRatio {
				t.Errorf("Ali %s: expected completion ratio %.2f, got %.2f", model, expected.expectedCompletionRatio, completionRatio)
			}
		}
	})

	// H0llyW00dzZ: I'm writing this test myself now because this codebase is too complex.
	t.Run("OpenRouter_Pricing", func(t *testing.T) {
		adaptor := &openrouter.Adaptor{}

		// Test GetDefaultModelPricing
		defaultPricing := adaptor.GetDefaultModelPricing()
		if len(defaultPricing) == 0 {
			t.Fatal("OpenRouter: GetDefaultModelPricing returned empty map")
		}

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

			if expected.expectValidRatio && ratio <= 0 {
				t.Errorf("OpenRouter %s: expected valid ratio, got %.6f", model, ratio)
			}
			if expected.expectValidCompletion && completionRatio <= 0 {
				t.Errorf("OpenRouter %s: expected valid completion ratio, got %.2f", model, completionRatio)
			}

			t.Logf("OpenRouter %s: ratio=%.6f, completion_ratio=%.2f", model, ratio, completionRatio)
		}
	})

	t.Run("xAI_Pricing", func(t *testing.T) {
		adaptor := GetAdaptor(apitype.XAI)
		if adaptor == nil {
			t.Fatal("xAI_Pricing not found")
		}

		// Use values directly from xai.ModelRatios for test expectations
		testModels := map[string]struct {
			expectedRatio           float64
			expectedCompletionRatio float64
			description             string
		}{
			"grok-4-0709":      {xai.ModelRatios["grok-4-0709"].Ratio, xai.ModelRatios["grok-4-0709"].CompletionRatio, "$3.00 input, $15.00 output"},
			"grok-3":           {xai.ModelRatios["grok-3"].Ratio, xai.ModelRatios["grok-3"].CompletionRatio, "$3.00 input, $15.00 output"},
			"grok-3-mini":      {xai.ModelRatios["grok-3-mini"].Ratio, xai.ModelRatios["grok-3-mini"].CompletionRatio, "$0.30 input, $0.50 output"},
			"grok-3-fast":      {xai.ModelRatios["grok-3-fast"].Ratio, xai.ModelRatios["grok-3-fast"].CompletionRatio, "$5.00 input, $25.00 output"},
			"grok-3-mini-fast": {xai.ModelRatios["grok-3-mini-fast"].Ratio, xai.ModelRatios["grok-3-mini-fast"].CompletionRatio, "$0.60 input, $4.00 output"},
			"grok-2-1212":      {xai.ModelRatios["grok-2-1212"].Ratio, xai.ModelRatios["grok-2-1212"].CompletionRatio, "$2.00 input, $10.00 output"},
			// Test legacy aliases
			"grok-beta": {xai.ModelRatios["grok-beta"].Ratio, xai.ModelRatios["grok-beta"].CompletionRatio, "Legacy alias for grok-2-1212"},
		}

		for model, expected := range testModels {
			ratio := adaptor.GetModelRatio(model)
			completionRatio := adaptor.GetCompletionRatio(model)

			if ratio != expected.expectedRatio {
				t.Errorf("xAI %s: expected ratio %.6f, got %.6f (%s)",
					model, expected.expectedRatio, ratio, expected.description)
			}
			if completionRatio != expected.expectedCompletionRatio {
				t.Errorf("xAI %s: expected completion ratio %.2f, got %.2f (%s)",
					model, expected.expectedCompletionRatio, completionRatio, expected.description)
			}

			t.Logf("xAI %s: ratio=%.6f (expected %.6f), completion_ratio=%.2f (expected %.2f) - %s",
				model, ratio, expected.expectedRatio, completionRatio, expected.expectedCompletionRatio,
				expected.description)
		}

		// Special test for image model which uses ImageUsdPerPic (1000)
		imageModel := "grok-2-image-1212"
		expectedImageRatio := xai.ModelRatios[imageModel].Ratio // $0.07 per image
		expectedImageCompletionRatio := xai.ModelRatios[imageModel].CompletionRatio

		imageModelRatio := adaptor.GetModelRatio(imageModel)
		imageModelCompletionRatio := adaptor.GetCompletionRatio(imageModel)

		if imageModelRatio != expectedImageRatio {
			t.Errorf("xAI %s: expected ratio %.6f, got %.6f (Image model: $0.07 per image)",
				imageModel, expectedImageRatio, imageModelRatio)
		}
		if imageModelCompletionRatio != expectedImageCompletionRatio {
			t.Errorf("xAI %s: expected completion ratio %.2f, got %.2f",
				imageModel, expectedImageCompletionRatio, imageModelCompletionRatio)
		}

		t.Logf("xAI %s: ratio=%.6f (expected %.6f), completion_ratio=%.2f (expected %.2f) - %s",
			imageModel, imageModelRatio, expectedImageRatio, imageModelCompletionRatio, expectedImageCompletionRatio,
			"$0.07 per image using ImageUsdPerPic")
	})

	t.Run("Gemini_Pricing", func(t *testing.T) {
		adaptor := GetAdaptor(apitype.Gemini)
		if adaptor == nil {
			t.Fatal("Gemini adaptor not found")
		}

		// Gemini uses USD pricing with ratio.MilliTokensUsd = 0.5
		testModels := map[string]struct {
			expectedRatio           float64
			expectedCompletionRatio float64
		}{
			"gemini-pro":       {0.5 * 0.5, 3.0},   // 0.5 * ratio.MilliTokensUsd
			"gemini-1.5-flash": {0.075 * 0.5, 4.0}, // 0.075 * ratio.MilliTokensUsd
			"gemini-1.5-pro":   {1.25 * 0.5, 4.0},  // 1.25 * ratio.MilliTokensUsd
		}

		for model, expected := range testModels {
			ratio := adaptor.GetModelRatio(model)
			completionRatio := adaptor.GetCompletionRatio(model)

			if ratio != expected.expectedRatio {
				t.Errorf("Gemini %s: expected ratio %.9f, got %.9f", model, expected.expectedRatio, ratio)
			}
			if completionRatio != expected.expectedCompletionRatio {
				t.Errorf("Gemini %s: expected completion ratio %.2f, got %.2f", model, expected.expectedCompletionRatio, completionRatio)
			}
		}
	})

	t.Run("VertexAI_Pricing", func(t *testing.T) {
		adaptor := GetAdaptor(apitype.VertexAI)
		if adaptor == nil {
			t.Fatal("VertexAI adaptor not found")
		}

		// VertexAI should have the same pricing as Gemini for shared models
		testModels := []string{"gemini-pro", "gemini-1.5-flash", "gemini-1.5-pro"}

		for _, model := range testModels {
			ratio := adaptor.GetModelRatio(model)
			completionRatio := adaptor.GetCompletionRatio(model)

			if ratio <= 0 {
				t.Errorf("VertexAI %s: invalid ratio %.9f", model, ratio)
			}
			if completionRatio <= 0 {
				t.Errorf("VertexAI %s: invalid completion ratio %.2f", model, completionRatio)
			}

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
			if adaptor == nil {
				t.Fatalf("%s adaptor not found", adapter.name)
			}

			defaultPricing := adaptor.GetDefaultModelPricing()
			if len(defaultPricing) == 0 {
				t.Fatalf("%s: No default pricing found", adapter.name)
			}

			// Test that GetModelRatio and GetCompletionRatio return consistent values
			// with what's in GetDefaultModelPricing
			for model, expectedPrice := range defaultPricing {
				actualRatio := adaptor.GetModelRatio(model)
				actualCompletionRatio := adaptor.GetCompletionRatio(model)

				if actualRatio != expectedPrice.Ratio {
					t.Errorf("%s %s: GetModelRatio (%.9f) != DefaultModelPricing.Ratio (%.9f)",
						adapter.name, model, actualRatio, expectedPrice.Ratio)
				}

				if actualCompletionRatio != expectedPrice.CompletionRatio {
					t.Errorf("%s %s: GetCompletionRatio (%.2f) != DefaultModelPricing.CompletionRatio (%.2f)",
						adapter.name, model, actualCompletionRatio, expectedPrice.CompletionRatio)
				}
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
			if adaptor == nil {
				t.Fatalf("%s adaptor not found", adapter.name)
			}

			// Test fallback pricing for unknown model
			ratio := adaptor.GetModelRatio(unknownModel)
			completionRatio := adaptor.GetCompletionRatio(unknownModel)

			if ratio <= 0 {
				t.Errorf("%s: Fallback ratio for unknown model should be > 0, got %.9f", adapter.name, ratio)
			}

			if completionRatio <= 0 {
				t.Errorf("%s: Fallback completion ratio for unknown model should be > 0, got %.2f", adapter.name, completionRatio)
			}

			t.Logf("%s fallback pricing: ratio=%.9f, completion_ratio=%.2f", adapter.name, ratio, completionRatio)
		})
	}
}
