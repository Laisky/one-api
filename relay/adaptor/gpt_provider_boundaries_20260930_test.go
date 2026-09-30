package adaptor_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/ai360"
	"github.com/Laisky/one-api/relay/adaptor/aiproxy"
	awsopenai "github.com/Laisky/one-api/relay/adaptor/aws/openai"
	"github.com/Laisky/one-api/relay/adaptor/azure"
	"github.com/Laisky/one-api/relay/adaptor/cerebras"
	"github.com/Laisky/one-api/relay/adaptor/cloudflare"
	"github.com/Laisky/one-api/relay/adaptor/copilot"
	"github.com/Laisky/one-api/relay/adaptor/deepinfra"
	"github.com/Laisky/one-api/relay/adaptor/fireworks"
	"github.com/Laisky/one-api/relay/adaptor/groq"
	"github.com/Laisky/one-api/relay/adaptor/novita"
	"github.com/Laisky/one-api/relay/adaptor/nvidia"
	"github.com/Laisky/one-api/relay/adaptor/ollama"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/siliconflow"
	"github.com/Laisky/one-api/relay/adaptor/togetherai"
	vertexopenai "github.com/Laisky/one-api/relay/adaptor/vertexai/openai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/pricing"
)

// gptCatalogProvider exposes public listing and price lookups for inheritance
// checks without requiring transport setup, credentials, or network access.
type gptCatalogProvider interface {
	GetModelList() []string
	GetDefaultModelPricing() map[string]adaptor.ModelConfig
	GetModelRatio(string) float64
	GetCompletionRatio(string) float64
}

// TestGPT61InheritedCatalogDefaults verifies existing Azure and AIProxy fallback
// catalogs inherit native metadata without duplicating IDs or inventing tariffs.
// This is a compatibility test, not proof of provider availability or invoicing.
func TestGPT61InheritedCatalogDefaults(t *testing.T) {
	t.Parallel()
	for name, provider := range map[string]gptCatalogProvider{
		"openai":  &openai.Adaptor{ChannelType: channeltype.OpenAI},
		"azure":   &azure.Adaptor{Adaptor: openai.Adaptor{ChannelType: channeltype.Azure}},
		"aiproxy": &aiproxy.Adaptor{},
	} {
		t.Run(name, func(t *testing.T) {
			const modelName = "gpt-6.1-sol"
			require.Contains(t, provider.GetModelList(), modelName)
			cfg, exists := provider.GetDefaultModelPricing()[modelName]
			require.True(t, exists)
			require.Equal(t, openai.ModelRatios[modelName], cfg)
			require.InDelta(t, 2.0, provider.GetModelRatio(modelName)/ratio.MilliTokensUsd, 1e-9)
			require.InDelta(t, 10.0, provider.GetModelRatio(modelName)*provider.GetCompletionRatio(modelName)/ratio.MilliTokensUsd, 1e-9)
			for _, alias := range []string{"gpt-6.1-sol-pro", "openai/gpt-6.1-sol-pro", "openai/gpt-6.1-sol:batch"} {
				require.NotContains(t, provider.GetModelList(), alias)
			}
			before := pricing.ResolveEffectivePricingFromConfig(272_000, cfg)
			after := pricing.ResolveEffectivePricingFromConfig(272_001, cfg)
			require.InDelta(t, 2.0, before.InputRatio/ratio.MilliTokensUsd, 1e-9)
			require.InDelta(t, 4.0, after.InputRatio/ratio.MilliTokensUsd, 1e-9)
			require.InDelta(t, 0.1, before.CachedInputRatio/ratio.MilliTokensUsd, 1e-9)
			require.InDelta(t, 0.2, after.CachedInputRatio/ratio.MilliTokensUsd, 1e-9)
		})
	}
}

// TestGPTProviderCatalogBoundaries20260930 verifies the audited GPT-OSS catalogs
// do not acquire a closed GPT-6.1 model or an OpenRouter-specific alias merely
// because their names include GPT. Future native support needs explicit changes.
func TestGPTProviderCatalogBoundaries20260930(t *testing.T) {
	t.Parallel()
	for name, catalog := range map[string]map[string]adaptor.ModelConfig{
		"ai360": ai360.ModelRatios, "cerebras": cerebras.ModelRatios,
		"cloudflare": cloudflare.ModelRatios, "deepinfra": deepinfra.ModelRatios,
		"fireworks": fireworks.ModelRatios, "groq": groq.ModelRatios,
		"novita": novita.ModelRatios, "nvidia": nvidia.ModelRatios,
		"ollama": ollama.ModelRatios, "siliconflow": siliconflow.ModelRatios,
		"togetherai": togetherai.ModelRatios, "vertexai": vertexopenai.ModelRatios,
	} {
		t.Run(name, func(t *testing.T) {
			require.NotEmpty(t, catalog)
			for _, id := range []string{"gpt-6.1-sol", "openai/gpt-6.1-sol", "openai/gpt-6.1-sol-pro"} {
				require.NotContains(t, catalog, id)
			}
		})
	}
	// Bedrock upstream supports closed GPT models, but this repository's current
	// Converse mapping implements only GPT-OSS. Do not advertise unsupported I/O.
	require.Contains(t, awsopenai.AwsModelIDMap, "gpt-oss-120b")
	require.NotContains(t, awsopenai.AwsModelIDMap, "gpt-6.1-sol")
	// Copilot's account-dependent catalog must not be replaced by OpenAI's list.
	require.Empty(t, (&copilot.Adaptor{}).GetModelList())
}

// TestGPTOSSProviderPrices20260930 verifies independently published Standard
// token prices through the production pricing resolver. These fixtures protect
// provider separation; they do not test live account entitlements or invoices.
func TestGPTOSSProviderPrices20260930(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		catalog       map[string]adaptor.ModelConfig
		id            string
		input, output float64
	}{
		{"groq", groq.ModelRatios, "openai/gpt-oss-120b", 0.15, 0.60},
		{"cerebras", cerebras.ModelRatios, "gpt-oss-120b", 0.35, 0.75},
		{"fireworks", fireworks.ModelRatios, "accounts/fireworks/models/gpt-oss-120b", 0.15, 0.60},
		{"deepinfra", deepinfra.ModelRatios, "openai/gpt-oss-120b", 0.037, 0.17},
		{"togetherai", togetherai.ModelRatios, "openai/gpt-oss-120b", 0.15, 0.60},
		{"novita", novita.ModelRatios, "openai/gpt-oss-120b", 0.05, 0.25},
		{"vertexai", vertexopenai.ModelRatios, "openai/gpt-oss-120b-maas", 0.09, 0.36},
		{"cloudflare", cloudflare.ModelRatios, "@cf/openai/gpt-oss-120b", 0.35, 0.75},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, exists := tc.catalog[tc.id]
			require.True(t, exists)
			effective := pricing.ResolveEffectivePricingFromConfig(1, cfg)
			require.InDelta(t, tc.input, effective.InputRatio/ratio.MilliTokensUsd, 1e-9)
			require.InDelta(t, tc.output, effective.OutputRatio/ratio.MilliTokensUsd, 1e-9)
		})
	}
}
