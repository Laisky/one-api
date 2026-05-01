package openrouterprovider

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
)

// TestMapModel_PricingConversion verifies the documented quota -> USD/token
// formula: usdPerToken = Ratio / MilliTokensUsd / 1e6 with MilliTokensUsd=0.5,
// so Ratio=1.25 yields prompt = "0.0000025" (i.e., $2.50 per 1M tokens).
func TestMapModel_PricingConversion(t *testing.T) {
	cfg := adaptor.ModelConfig{
		Ratio:           1.25,
		CompletionRatio: 2.0,
	}
	got := MapModel("gpt-test", cfg, "openai", 1700000000)
	require.Equal(t, "0.0000025", got.Pricing.Prompt)
	// Completion = 1.25 * 2.0 / 0.5 / 1e6 = 5e-6.
	require.Equal(t, "0.000005", got.Pricing.Completion)
	require.Empty(t, got.Pricing.InputCacheRead)
	require.Empty(t, got.Pricing.Image)
	require.Empty(t, got.Pricing.Request)
}

// TestMapModel_DefaultsAppliedWhenSparse confirms every default documented in
// the package comments fires when the ModelConfig carries no explicit metadata.
func TestMapModel_DefaultsAppliedWhenSparse(t *testing.T) {
	got := MapModel("anonymous-model", adaptor.ModelConfig{}, "", 1700000000)
	require.Equal(t, "anonymous-model", got.ID)
	require.Equal(t, "anonymous-model", got.Name)
	require.Equal(t, int32(8192), got.ContextLength)
	require.Equal(t, int32(4096), got.MaxOutputLength)
	require.Equal(t, "fp16", got.Quantization)
	require.Equal(t, []string{"text"}, got.InputModalities)
	require.Equal(t, []string{"text"}, got.OutputModalities)
	require.Equal(t, []string{"tools"}, got.SupportedFeatures)
	require.NotEmpty(t, got.SupportedSamplingParameters)
	require.Contains(t, got.SupportedSamplingParameters, "temperature")
	require.Equal(t, "0", got.Pricing.Prompt)
	require.Equal(t, "0", got.Pricing.Completion)
}

// TestMapModel_ExplicitOverridesWin ensures explicit ModelConfig values are
// emitted verbatim and not shadowed by defaults.
func TestMapModel_ExplicitOverridesWin(t *testing.T) {
	cfg := adaptor.ModelConfig{
		ContextLength:               200000,
		MaxOutputTokens:             16384,
		Quantization:                "bf16",
		InputModalities:             []string{"text", "image", "file"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
		SupportedSamplingParameters: []string{"temperature", "top_p"},
		HuggingFaceID:               "owner/model",
		Description:                 "test description",
	}
	got := MapModel("gpt-explicit", cfg, "openai", 1700000000)
	require.Equal(t, "openai: gpt-explicit", got.Name)
	require.Equal(t, int32(200000), got.ContextLength)
	require.Equal(t, int32(16384), got.MaxOutputLength)
	require.Equal(t, "bf16", got.Quantization)
	require.Equal(t, []string{"text", "image", "file"}, got.InputModalities)
	require.Equal(t, []string{"text"}, got.OutputModalities)
	require.Equal(t, []string{"tools", "json_mode", "structured_outputs"}, got.SupportedFeatures)
	require.Equal(t, []string{"temperature", "top_p"}, got.SupportedSamplingParameters)
	require.Equal(t, "owner/model", got.HuggingFaceID)
	require.Equal(t, "test description", got.Description)
}

// TestMapModel_ImageModalityDerived checks that when InputModalities is unset
// but the ModelConfig advertises an Image sub-config, "image" is added to the
// derived input modalities (alongside "text"). Audio/Video are *not* added.
func TestMapModel_ImageModalityDerived(t *testing.T) {
	cfg := adaptor.ModelConfig{
		Image: &adaptor.ImagePricingConfig{PricePerImageUsd: 0.04},
		Audio: &adaptor.AudioPricingConfig{UsdPerSecond: 0.001},
	}
	got := MapModel("multimodal", cfg, "vendor", 0)
	require.Equal(t, []string{"text", "image"}, got.InputModalities)
}

// TestMapModel_ImagePricingEmittedWhenPositive verifies the optional Image
// pricing string is emitted with the raw USD-per-image value when set, and
// suppressed otherwise.
func TestMapModel_ImagePricingEmittedWhenPositive(t *testing.T) {
	cfg := adaptor.ModelConfig{
		Ratio: 1.0,
		Image: &adaptor.ImagePricingConfig{PricePerImageUsd: 0.04},
	}
	got := MapModel("image-gen", cfg, "openai", 0)
	require.Equal(t, "0.04", got.Pricing.Image)

	// Zero or missing Image pricing must not emit the field.
	cfg2 := adaptor.ModelConfig{Ratio: 1.0}
	got2 := MapModel("text-only", cfg2, "openai", 0)
	require.Empty(t, got2.Pricing.Image)
}

// TestMapModel_CachedInputRatioEmits ensures InputCacheRead is included when
// CachedInputRatio is positive and absent otherwise.
func TestMapModel_CachedInputRatioEmits(t *testing.T) {
	cfg := adaptor.ModelConfig{
		Ratio:            1.0,
		CompletionRatio:  1.0,
		CachedInputRatio: 0.5,
	}
	got := MapModel("cached-model", cfg, "openai", 0)
	require.Equal(t, "0.000002", got.Pricing.Prompt)
	// 0.5 / 0.5 / 1e6 = 1e-6
	require.Equal(t, "0.000001", got.Pricing.InputCacheRead)
}

// TestMapModel_CompletionRatioApplied ensures the completion price scales by
// CompletionRatio relative to the prompt ratio.
func TestMapModel_CompletionRatioApplied(t *testing.T) {
	cfg := adaptor.ModelConfig{
		Ratio:           2.0,
		CompletionRatio: 4.0,
	}
	got := MapModel("ratio-model", cfg, "owner", 0)
	// Prompt: 2 / 0.5 / 1e6 = 4e-6
	require.Equal(t, "0.000004", got.Pricing.Prompt)
	// Completion: 2*4 / 0.5 / 1e6 = 1.6e-5
	require.Equal(t, "0.000016", got.Pricing.Completion)
}

// TestMapModel_CompletionRatioDefaultOne checks that an unset CompletionRatio
// defaults to 1.0 so completion price equals prompt price.
func TestMapModel_CompletionRatioDefaultOne(t *testing.T) {
	cfg := adaptor.ModelConfig{Ratio: 1.0}
	got := MapModel("no-completion-ratio", cfg, "owner", 0)
	require.Equal(t, got.Pricing.Prompt, got.Pricing.Completion)
}

// TestBuildModelListResponse_Structure verifies the bulk builder produces the
// expected envelope and skips empty model names.
func TestBuildModelListResponse_Structure(t *testing.T) {
	inputs := []ModelInput{
		{Name: "alpha", Owner: "openai", Config: adaptor.ModelConfig{Ratio: 1.0}, Created: 100},
		{Name: "  ", Owner: "skip-me"},
		{Name: "beta", Owner: "anthropic", Config: adaptor.ModelConfig{Ratio: 0.5, CompletionRatio: 3.0}, Created: 200},
	}
	resp := BuildModelListResponse(inputs)
	require.Len(t, resp.Data, 2)
	require.Equal(t, "alpha", resp.Data[0].ID)
	require.Equal(t, "openai: alpha", resp.Data[0].Name)
	require.Equal(t, int64(100), resp.Data[0].Created)
	require.Equal(t, "beta", resp.Data[1].ID)
	require.Equal(t, "anthropic: beta", resp.Data[1].Name)
	require.Equal(t, int64(200), resp.Data[1].Created)
	// Sanity-check beta completion price: 0.5*3.0 / 0.5 / 1e6 = 3e-6
	require.Equal(t, "0.000003", resp.Data[1].Pricing.Completion)
}

// TestMapModel_CreatedFallsBackToNow confirms a zero Created argument is
// replaced with a positive Unix timestamp.
func TestMapModel_CreatedFallsBackToNow(t *testing.T) {
	got := MapModel("now-model", adaptor.ModelConfig{}, "owner", 0)
	require.Greater(t, got.Created, int64(0))
}

// TestMapModel_ContextLengthBelowMaxOutputDefault verifies the documented
// edge case: when ContextLength is explicitly set below the 4096 default
// MaxOutputTokens, the cap is clamped to ContextLength.
func TestMapModel_ContextLengthBelowMaxOutputDefault(t *testing.T) {
	cfg := adaptor.ModelConfig{ContextLength: 1024}
	got := MapModel("small-ctx", cfg, "", 0)
	require.Equal(t, int32(1024), got.ContextLength)
	require.Equal(t, int32(1024), got.MaxOutputLength)
}

// TestMapModel_FormatPrecisionStable spot-checks that strconv.FormatFloat with
// precision -1 emits the shortest round-trippable representation, which
// matches OpenRouter's expectation of decimal strings.
func TestMapModel_FormatPrecisionStable(t *testing.T) {
	cfg := adaptor.ModelConfig{Ratio: 0.075}
	got := MapModel("precision", cfg, "", 0)
	parsed, err := strconv.ParseFloat(got.Pricing.Prompt, 64)
	require.NoError(t, err)
	require.InDelta(t, 1.5e-7, parsed, 1e-12)
}
