package aws_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/aws"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
)

// bedrockSKU is one model's on-demand Bedrock tariff in USD per 1M tokens.
// A zero cache read or cache write means Bedrock publishes no such usage type,
// so the only billable SKU for those tokens is the ordinary input token count.
type bedrockSKU struct {
	input, output, cacheRead, cacheWrite float64
}

// legacyBedrockClaudeSKUs is the independent oracle for the legacy Claude
// models: AWS Price List API, offer AmazonBedrockFoundationModels, regions
// us-east-1 and us-west-2 (publication 2026-11-09T22:43:12Z, fetched
// 2026-11-18T14:49Z), usage types InputTokenCount, OutputTokenCount,
// CacheReadInputTokenCount and CacheWriteInputTokenCount. None of these models
// has a CacheWrite1h usage type, and the first group has no cache usage type at
// all. It takes no arguments and returns the model-to-SKU map.
func legacyBedrockClaudeSKUs() map[string]bedrockSKU {
	return map[string]bedrockSKU{
		// No cache SKUs: "Claude Instant", "Claude 2.0" (Claude v2 and v2:1),
		// "Claude 3 Haiku", "Claude 3 Sonnet", "Claude 3 Opus", "Claude 3.5 Sonnet".
		"claude-instant-1.2":         {0.8, 2.4, 0, 0},
		"claude-2.0":                 {8, 24, 0, 0},
		"claude-2.1":                 {8, 24, 0, 0},
		"claude-3-haiku-20240307":    {0.25, 1.25, 0, 0},
		"claude-3-sonnet-20240229":   {3, 15, 0, 0},
		"claude-3-opus-20240229":     {15, 75, 0, 0},
		"claude-3-5-sonnet-20240620": {3, 15, 0, 0},
		// Five-minute cache only: read and write SKUs, no one-hour write SKU.
		"claude-3-5-haiku-20241022":  {0.8, 4, 0.08, 1},
		"claude-3-5-haiku-latest":    {0.8, 4, 0.08, 1},
		"claude-3-5-sonnet-20241022": {3, 15, 0.3, 3.75},
		"claude-3-5-sonnet-latest":   {3, 15, 0.3, 3.75},
		"claude-3-7-sonnet-20250219": {3, 15, 0.3, 3.75},
		"claude-3-7-sonnet-latest":   {3, 15, 0.3, 3.75},
		"claude-sonnet-4-20250514":   {3, 15, 0.3, 3.75},
		"claude-sonnet-4-0":          {3, 15, 0.3, 3.75},
		"claude-opus-4-20250514":     {15, 75, 1.5, 18.75},
		"claude-opus-4-0":            {15, 75, 1.5, 18.75},
		"claude-opus-4-1-20250805":   {15, 75, 1.5, 18.75},
		"claude-opus-4-1":            {15, 75, 1.5, 18.75},
	}
}

// TestBedrockLegacyClaudeCacheBillsOnlySoldSKUs settles a receipt carrying
// every Claude usage bucket through the real quota engine and compares it with
// the Bedrock SKU oracle. A cache bucket Bedrock does not sell can only be
// metered as ordinary input (or, for a one-hour write on a five-minute-only
// model, as the model's only cache-write SKU), so the catalog must neither
// discount it with an invented cache price nor mark it up with Anthropic's
// first-party one-hour rate. It accepts t and returns no values.
func TestBedrockLegacyClaudeCacheBillsOnlySoldSKUs(t *testing.T) {
	t.Parallel()
	a := &aws.Adaptor{}
	catalog := a.GetDefaultModelPricing()
	for name, sku := range legacyBedrockClaudeSKUs() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg, exists := catalog[name]
			require.True(t, exists)
			read, write := sku.cacheRead, sku.cacheWrite
			if read == 0 {
				require.Zero(t, cfg.CachedInputRatio, "Bedrock sells no cache read for %s", name)
				require.Zero(t, cfg.CacheWrite5mRatio, "Bedrock sells no cache write for %s", name)
				require.Zero(t, cfg.CacheWrite1hRatio, "Bedrock sells no one-hour cache write for %s", name)
				read, write = sku.input, sku.input
			} else {
				require.Equal(t, cfg.CacheWrite5mRatio, cfg.CacheWrite1hRatio,
					"Bedrock's only cache-write SKU for %s also prices any one-hour write", name)
			}
			usage := &model.Usage{PromptTokens: 100, CompletionTokens: 10,
				PromptTokensDetails: &model.UsagePromptTokensDetails{CachedTokens: 1000},
				CacheWrite5mTokens:  100, CacheWrite1hTokens: 200}
			got := quota.Compute(quota.ComputeInput{Usage: usage, ModelName: name, ModelRatio: cfg.Ratio,
				GroupRatio: 1, PricingAdaptor: a})
			want := math.Ceil(ratio.MilliTokensUsd * (100*sku.input + 10*sku.output + 1000*read + 100*write + 200*write))
			require.InDelta(t, want, float64(got.TotalQuota), 1, "%s must bill exactly the Bedrock SKUs", name)
			require.False(t, got.UnpricedUsage)
		})
	}
}

// TestBedrockEndOfLifeClaudeModelsAreMarked keeps the IDs that Bedrock retired
// in its main Regions (operators may still route them through another Region
// or a private arrangement) while making the catalog stop implying they are
// current. Source: https://docs.aws.amazon.com/bedrock/latest/userguide/model-lifecycle-legacy.html
// (fetched 2026-11-18): Claude 3 Haiku EOL 2026-09-10 (us-east-1, us-east-2,
// us-west-2, ap-northeast-1, ap-southeast-2, eu-central-1, eu-west-1/2/3) and
// 2026-11-13 (ap-northeast-2, ap-south-1, ap-southeast-1); Claude Sonnet 4 EOL
// 2026-10-14 (23 Regions including us-east-1/2 and us-west-1/2); Claude 3
// Sonnet, 3.5 Sonnet, 3.5 Sonnet v2 and 3.7 Sonnet EOL 2026-11-13 in their
// remaining listed Regions; Claude Sonnet 4.5 Legacy since 2026-10-08 with EOL
// 2027-04-08. It accepts t and returns no values.
func TestBedrockEndOfLifeClaudeModelsAreMarked(t *testing.T) {
	t.Parallel()
	a := &aws.Adaptor{}
	catalog := a.GetDefaultModelPricing()
	advertised := a.GetModelList()
	for name, marks := range map[string][]string{
		"claude-3-haiku-20240307":    {"end-of-life", "2026-09-10", "2026-11-13"},
		"claude-sonnet-4-20250514":   {"end-of-life", "2026-10-14"},
		"claude-sonnet-4-0":          {"end-of-life", "2026-10-14"},
		"claude-3-sonnet-20240229":   {"end-of-life", "2026-11-13"},
		"claude-3-5-sonnet-20240620": {"end-of-life", "2026-11-13"},
		"claude-3-5-sonnet-20241022": {"end-of-life", "2026-11-13"},
		"claude-3-5-sonnet-latest":   {"end-of-life", "2026-11-13"},
		"claude-3-7-sonnet-20250219": {"end-of-life", "2026-11-13"},
		"claude-3-7-sonnet-latest":   {"end-of-life", "2026-11-13"},
		"claude-sonnet-4-5":          {"Legacy", "2026-10-08", "2027-04-08"},
		"claude-sonnet-4-5-20250929": {"Legacy", "2026-10-08", "2027-04-08"},
	} {
		cfg, exists := catalog[name]
		require.True(t, exists, "%s stays configurable for other Regions", name)
		require.Contains(t, advertised, name)
		for _, mark := range marks {
			require.Contains(t, cfg.Description, mark, name)
		}
	}
}
