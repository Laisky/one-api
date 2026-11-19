package aws_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/aws"
	awsclaude "github.com/Laisky/one-api/relay/adaptor/aws/claude"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestBedrockClaude35HaikuLatestOwnsBedrockTariff pins the alias to Bedrock's
// own on-demand prices for the model it invokes. Source: AWS Price List API,
// offer AmazonBedrockFoundationModels, region us-east-1, publication
// 2026-11-09T22:43:12Z, fetched 2026-11-18T14:32Z (usage types
// USE1-MP:USE1_InputTokenCount / OutputTokenCount / CacheWriteInputTokenCount /
// CacheReadInputTokenCount = $0.80 / $4.00 / $1.00 / $0.08 per 1M tokens).
// Before this entry existed the alias resolved through the global layer, i.e.
// Anthropic's first-party price. It accepts t and returns no values.
func TestBedrockClaude35HaikuLatestOwnsBedrockTariff(t *testing.T) {
	t.Parallel()
	catalog := (&aws.Adaptor{}).GetDefaultModelPricing()
	alias, exists := catalog["claude-3-5-haiku-latest"]
	require.True(t, exists, "the advertised alias must carry the AWS adaptor's own tariff")
	require.InDelta(t, 0.80, alias.Ratio/ratio.MilliTokensUsd, 1e-12)
	require.InDelta(t, 4.00, alias.Ratio*alias.CompletionRatio/ratio.MilliTokensUsd, 1e-12)
	require.InDelta(t, 1.00, alias.CacheWrite5mRatio/ratio.MilliTokensUsd, 1e-12)
	require.InDelta(t, 0.08, alias.CachedInputRatio/ratio.MilliTokensUsd, 1e-12)
	require.GreaterOrEqual(t, alias.CacheWrite1hRatio, alias.CacheWrite5mRatio)

	// The alias and the dated ID invoke the same Bedrock model, so they bill alike.
	require.Equal(t, awsclaude.AwsModelIDMap["claude-3-5-haiku-20241022"], awsclaude.AwsModelIDMap["claude-3-5-haiku-latest"])
	dated := catalog["claude-3-5-haiku-20241022"]
	require.Equal(t, dated.Ratio, alias.Ratio)
	require.Equal(t, dated.CompletionRatio, alias.CompletionRatio)
	require.Equal(t, dated.CachedInputRatio, alias.CachedInputRatio)
	require.Equal(t, dated.CacheWrite5mRatio, alias.CacheWrite5mRatio)
	require.Equal(t, dated.CacheWrite1hRatio, alias.CacheWrite1hRatio)
}

// TestEveryAdvertisedBedrockModelHasOwnTariff keeps every model the AWS adaptor
// advertises priced by the AWS catalog itself, so billing never falls through
// to the global layer (another host's price) for a Bedrock model. It accepts t
// and returns no values.
func TestEveryAdvertisedBedrockModelHasOwnTariff(t *testing.T) {
	t.Parallel()
	a := &aws.Adaptor{}
	catalog := a.GetDefaultModelPricing()
	for _, name := range a.GetModelList() {
		_, exists := catalog[name]
		require.True(t, exists, "%s is advertised by the AWS adaptor without an AWS tariff", name)
	}
}
