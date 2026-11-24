package quota_test

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	model "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/apitype"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	quotautil "github.com/Laisky/one-api/relay/quota"
)

// channelTierAt is the request start time shared by these fixtures.
var channelTierAt = time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

// channelComputeInput builds the ComputeInput the relay controllers build for
// a channel whose model_configs holds local for name: the scalar ratio maps are
// derived from model_configs exactly as in production. Parameters: t owns
// assertions, provider is the pricing adaptor and usage the receipt. Returns:
// the input, at group ratio 1.
func channelComputeInput(t *testing.T, name string, local model.ModelConfigLocal, provider adaptor.Adaptor, usage *relaymodel.Usage) quotautil.ComputeInput {
	t.Helper()
	ch := &model.Channel{}
	require.NoError(t, ch.SetModelPriceConfigs(map[string]model.ModelConfigLocal{name: local}))
	configs, ratios := ch.GetModelPriceConfigs(), ch.GetModelRatioFromConfigs()
	return quotautil.ComputeInput{
		Usage: usage, ModelName: name,
		ModelRatio:        pricing.ResolveModelRatioAt(name, configs, ratios, provider, channelTierAt),
		ChannelModelRatio: ratios, ChannelCompletionRatio: ch.GetCompletionRatioFromConfigs(),
		GroupRatio: 1, ChannelModelConfigs: configs, PricingAdaptor: provider, RequestTime: channelTierAt,
	}
}

// TestComputeChannelRatioWithTiersAppliesTierInput verifies a channel entry
// with a base ratio and tiers but no time windows bills the tier's input
// ratio: the scalar ratio map copies the entry's own base ratio, which is not
// a legacy flat override. Before the fix a 2,000-token prompt billed input 2.
func TestComputeChannelRatioWithTiersAppliesTierInput(t *testing.T) {
	t.Parallel()
	const name = "channel-tiered-model"
	provider := &stubQuotaAdaptor{pricing: map[string]adaptor.ModelConfig{}}
	local := model.ModelConfigLocal{Ratio: 2, CompletionRatio: 3,
		Tiers: []model.ModelRatioTierLocal{{InputTokenThreshold: 1000, Ratio: 3}}}

	tiered := quotautil.Compute(channelComputeInput(t, name, local, provider, &relaymodel.Usage{PromptTokens: 2000, CompletionTokens: 100}))
	require.InDelta(t, 3.0, tiered.UsedModelRatio, 1e-12, "the tier's input ratio applies at 2,000 tokens")
	require.InDelta(t, 3.0, tiered.UsedCompletionRatio, 1e-12)
	require.Equal(t, int64(2000*3+100*3*3), tiered.TotalQuota)

	base := quotautil.Compute(channelComputeInput(t, name, local, provider, &relaymodel.Usage{PromptTokens: 500, CompletionTokens: 100}))
	require.InDelta(t, 2.0, base.UsedModelRatio, 1e-12, "below the threshold the base ratio applies")
	require.Equal(t, int64(500*2+100*2*3), base.TotalQuota)

	// Admission prices the same tier as settlement.
	quote, applies, err := quotautil.EstimateTierAdmission(channelComputeInput(t, name, local, provider,
		&relaymodel.Usage{PromptTokens: 2000}), 100, 0, quotautil.AdmissionOptions{})
	require.NoError(t, err)
	require.True(t, applies)
	require.Equal(t, tiered.TotalQuota, quote)
}

// TestComputeChannelLegacyFlatOverrideKeepsPrecedenceOverTiers verifies a
// scalar channel ratio that differs from the entry's own ratio remains a
// legacy flat override: it keeps precedence over the entry's tier input ratio.
func TestComputeChannelLegacyFlatOverrideKeepsPrecedenceOverTiers(t *testing.T) {
	t.Parallel()
	const name = "legacy-flat-tiered-model"
	provider := &stubQuotaAdaptor{pricing: map[string]adaptor.ModelConfig{}}
	input := channelComputeInput(t, name, model.ModelConfigLocal{Ratio: 2, CompletionRatio: 3,
		Tiers: []model.ModelRatioTierLocal{{InputTokenThreshold: 1000, Ratio: 3}}}, provider,
		&relaymodel.Usage{PromptTokens: 2000, CompletionTokens: 100})
	input.ChannelModelRatio = map[string]float64{name: 5}
	input.ModelRatio = pricing.ResolveModelRatioAt(name, input.ChannelModelConfigs, input.ChannelModelRatio, provider, channelTierAt)
	require.InDelta(t, 5.0, input.ModelRatio, 1e-12)

	result := quotautil.Compute(input)
	require.InDelta(t, 5.0, result.UsedModelRatio, 1e-12)
	require.Equal(t, int64(2000*5+100*5*3), result.TotalQuota)
}

// TestComputeChannelEntryInheritsProviderTiers verifies a channel entry for a
// tiered provider model that inherits the provider's base ratio also inherits
// its long-context tiers (gpt-5.5-pro: $30/$180 base, $60/$270 above 272K
// input). Before the fix a metadata-only entry billed a 300K prompt at input
// $30. An explicit operator ratio keeps its own flat schedule.
func TestComputeChannelEntryInheritsProviderTiers(t *testing.T) {
	t.Parallel()
	const name = "gpt-5.5-pro"
	provider := relay.GetAdaptor(apitype.OpenAI)
	require.NotNil(t, provider)
	m := billingratio.MilliTokensUsd
	usage := func() *relaymodel.Usage { return &relaymodel.Usage{PromptTokens: 300_000, CompletionTokens: 1000} }
	longContext := int64(math.Ceil(300_000*60*m + 1000*270*m))

	for _, tc := range []struct {
		name  string
		local model.ModelConfigLocal
		want  int64
	}{
		{name: "metadata_only", local: model.ModelConfigLocal{MaxTokens: 4096}, want: longContext},
		{name: "cache_price_only", local: model.ModelConfigLocal{CachedInputRatio: 3 * m}, want: longContext},
		// An explicit completion ratio applies inside the inherited tier,
		// never below the provider tier's own multiplier (4.5).
		{name: "explicit_completion_only", local: model.ModelConfigLocal{CompletionRatio: 8}, want: int64(math.Ceil(300_000*60*m + 1000*60*8*m))},
		// An explicit operator ratio is the operator's flat schedule.
		{name: "explicit_ratio", local: model.ModelConfigLocal{Ratio: 30 * m}, want: int64(math.Ceil(300_000*30*m + 1000*180*m))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := channelComputeInput(t, name, tc.local, provider, usage())
			require.Equal(t, tc.want, quotautil.Compute(input).TotalQuota)

			// Admission resolves the same schedule as settlement.
			input.Usage = &relaymodel.Usage{PromptTokens: 300_000}
			quote, applies, err := quotautil.EstimateTierAdmission(input, 1000, 0, quotautil.AdmissionOptions{})
			require.NoError(t, err)
			if tc.local.Ratio != 0 {
				require.False(t, applies, "an explicit flat operator ratio has no tiers")
				return
			}
			require.True(t, applies)
			require.Equal(t, tc.want, quote)
		})
	}

	// The 272K threshold still selects the base price below it.
	below := quotautil.Compute(channelComputeInput(t, name, model.ModelConfigLocal{MaxTokens: 4096}, provider,
		&relaymodel.Usage{PromptTokens: 200_000, CompletionTokens: 1000}))
	require.Equal(t, int64(math.Ceil(200_000*30*m+1000*180*m)), below.TotalQuota)
}

// TestComputeInheritedTierNeverDiscountsUngrantedCache verifies an inherited
// provider tier does not grant a cached-input discount the channel entry never
// set: gpt-5.5 publishes $1/M cached input above 272K, but an entry without a
// cache price bills cached tokens at the input price on both sides of the
// threshold, so the longer prompt can never cost less.
func TestComputeInheritedTierNeverDiscountsUngrantedCache(t *testing.T) {
	t.Parallel()
	const name = "gpt-5.5"
	provider := relay.GetAdaptor(apitype.OpenAI)
	require.NotNil(t, provider)
	m := billingratio.MilliTokensUsd
	usage := &relaymodel.Usage{PromptTokens: 300_000, CompletionTokens: 1000,
		PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{CachedTokens: 200_000}}

	result := quotautil.Compute(channelComputeInput(t, name, model.ModelConfigLocal{MaxTokens: 4096}, provider, usage))
	require.Equal(t, int64(math.Ceil(300_000*10*m+1000*45*m)), result.TotalQuota)
}

// TestComputeChannelEntryInheritsProviderTimeWindows verifies a channel entry
// that inherits the provider's base ratio and defines no windows of its own
// inherits the provider's time windows: Ali's deepseek-v4.1-flash-1017 bills
// CNY 2/8 per million input/output from 08:00 to 22:00 Asia/Shanghai and 1/4
// otherwise. Before the fix a metadata-only entry billed the peak at the
// unwindowed base input price. An entry without a cache price still bills
// cached tokens at the window's input price, never the window's cache discount.
func TestComputeChannelEntryInheritsProviderTimeWindows(t *testing.T) {
	t.Parallel()
	const name = "deepseek-v4.1-flash-1017"
	provider := relay.GetAdaptor(apitype.Ali)
	require.NotNil(t, provider)
	r := billingratio.MilliTokensRmb
	peak := time.Date(2026, 11, 24, 4, 0, 0, 0, time.UTC)     // 12:00 Asia/Shanghai
	offPeak := time.Date(2026, 11, 24, 16, 0, 0, 0, time.UTC) // 00:00 Asia/Shanghai
	usage := func() *relaymodel.Usage {
		return &relaymodel.Usage{PromptTokens: 10_000, CompletionTokens: 1_000,
			PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{CachedTokens: 4_000}}
	}
	for _, tc := range []struct {
		name  string
		local model.ModelConfigLocal
		at    time.Time
		want  int64
	}{
		// Peak: every input token at CNY 2 (no granted cache price), output at 2*4.
		{name: "metadata_only_peak", local: model.ModelConfigLocal{MaxTokens: 4096}, at: peak, want: int64(math.Ceil(10_000*2*r + 1_000*8*r))},
		{name: "metadata_only_off_peak", local: model.ModelConfigLocal{MaxTokens: 4096}, at: offPeak, want: int64(math.Ceil(10_000*1*r + 1_000*4*r))},
		// A stated cache price is never undercut by the window's (CNY 0.04).
		{name: "stated_cache_price_peak", local: model.ModelConfigLocal{CachedInputRatio: 0.03 * r}, at: peak,
			want: int64(math.Ceil(6_000*2*r + 4_000*0.04*r + 1_000*8*r))},
		// A stated completion ratio above the window's (4) is kept.
		{name: "stated_completion_peak", local: model.ModelConfigLocal{CompletionRatio: 5}, at: peak, want: int64(math.Ceil(10_000*2*r + 1_000*10*r))},
		// An explicit operator ratio keeps its own flat schedule.
		{name: "explicit_ratio_peak", local: model.ModelConfigLocal{Ratio: 1 * r}, at: peak, want: int64(math.Ceil(10_000*1*r + 1_000*4*r))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := channelComputeInput(t, name, tc.local, provider, usage())
			input.RequestTime = tc.at
			input.ModelRatio = pricing.ResolveModelRatioAt(name, input.ChannelModelConfigs, input.ChannelModelRatio, provider, tc.at)
			require.Equal(t, tc.want, quotautil.Compute(input).TotalQuota)
		})
	}
}
