package tooling

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/deepseek"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestDeepSeekToolReceiptPositiveQuotes keeps explicitly priced DeepSeek tools
// available under the conservative billing policy.
func TestDeepSeekToolReceiptPositiveQuotes(t *testing.T) {
	for _, tc := range []struct {
		name, base, actual string
		channel            int
		price              *model.ToolPricingLocal
		provider           adaptor.Adaptor
	}{
		{"dedicated_quota", "https://example.invalid", "deepseek-v4-pro", channeltype.DeepSeek, &model.ToolPricingLocal{QuotaPerCall: 17}, &deepseek.Adaptor{}},
		{"dedicated_overflow_usd", "https://example.invalid", "deepseek-v4-pro", channeltype.DeepSeek, &model.ToolPricingLocal{UsdPerCall: 1e100}, &deepseek.Adaptor{}},
		{"dedicated_usd", "https://example.invalid", "deepseek-v4-pro", channeltype.DeepSeek, &model.ToolPricingLocal{UsdPerCall: 0.01}, &deepseek.Adaptor{}},
		{"flash_ignored", "https://example.invalid", "deepseek-flash", channeltype.DeepSeek, &model.ToolPricingLocal{QuotaPerCall: 17}, &deepseek.Adaptor{}},
		{"flash_alias", "https://example.invalid", "deepseek-v4-flash-vision-exp", channeltype.DeepSeek, &model.ToolPricingLocal{QuotaPerCall: 17}, &deepseek.Adaptor{}},
		{"official_host_override", "https://api.deepseek.com", "deepseek-v4-pro", channeltype.OpenAI, &model.ToolPricingLocal{QuotaPerCall: 17}, &openai.Adaptor{}},
		{"official_host_provider_default", "https://api.deepseek.com", "deepseek-v4-pro", channeltype.OpenAI, nil, &openai.Adaptor{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			channel := &model.Channel{Type: tc.channel}
			if tc.price != nil {
				require.NoError(t, channel.SetToolingConfig(&model.ChannelToolingConfig{Pricing: map[string]model.ToolPricingLocal{BuiltinWebSearch: *tc.price}}))
			}
			meta := &metalib.Meta{ChannelType: tc.channel, BaseURL: tc.base, OriginModelName: "mapped-alias", ActualModelName: tc.actual}
			quotes, err := BuiltinToolQuotes(context.Background(), tc.actual, meta, channel, tc.provider, map[string]struct{}{BuiltinWebSearch: {}})
			require.NoError(t, err)
			require.Positive(t, quotes[BuiltinWebSearch])
		})
	}
}

// TestDeepSeekToolReceiptQuoteControls preserves intentional free overrides,
// third-party model hosting, ordinary OpenAI pricing and local tool definitions.
func TestDeepSeekToolReceiptQuoteControls(t *testing.T) {
	for _, tc := range []struct {
		name, base, actual string
		channel            int
		price              int64
	}{
		{"intentional_free_pro", "https://api.deepseek.com", "deepseek-v4-pro", channeltype.DeepSeek, 0},
		{"intentional_free_flash", "https://api.deepseek.com", "deepseek-flash", channeltype.DeepSeek, 0},
		{"third_party_deepseek_weights", "https://provider.example/v1", "deepseek-v4-pro", channeltype.OpenAI, 17},
		{"hostname_lookalike", "https://api.deepseek.com.example/v1", "deepseek-v4-pro", channeltype.OpenAI, 17},
		{"unrelated_openai", "https://api.openai.com/v1", "gpt-4o", channeltype.OpenAI, 17},
	} {
		t.Run(tc.name, func(t *testing.T) {
			channel := &model.Channel{Type: tc.channel}
			require.NoError(t, channel.SetToolingConfig(&model.ChannelToolingConfig{Pricing: map[string]model.ToolPricingLocal{BuiltinWebSearch: {QuotaPerCall: tc.price}}}))
			meta := &metalib.Meta{ChannelType: tc.channel, BaseURL: tc.base, ActualModelName: tc.actual}
			quotes, err := BuiltinToolQuotes(context.Background(), tc.actual, meta, channel, &openai.Adaptor{}, map[string]struct{}{BuiltinWebSearch: {}})
			require.NoError(t, err)
			require.Equal(t, tc.price, quotes[BuiltinWebSearch])
		})
	}
	meta := &metalib.Meta{ChannelType: channeltype.DeepSeek, ActualModelName: "deepseek-flash"}
	_, err := BuiltinToolQuotes(context.Background(), meta.ActualModelName, meta, nil, &deepseek.Adaptor{}, map[string]struct{}{BuiltinWebSearch: {}})
	require.ErrorContains(t, err, "not allowed", "empty DeepSeek defaults remain unchanged")
	for _, name := range []string{"search", "fetch", "web_search", "web_fetch"} {
		request := &relaymodel.GeneralOpenAIRequest{Model: "deepseek-flash", Tools: []relaymodel.Tool{{Type: "function", Function: &relaymodel.Function{Name: name}}}}
		require.NoError(t, ValidateChatBuiltinTools(nil, request, meta, nil, &deepseek.Adaptor{}), name)
	}
}

// TestDeepSeekToolReceiptFetchQuotes preserves separately configured Fetch
// tariffs while enabling positively priced calls.
func TestDeepSeekToolReceiptFetchQuotes(t *testing.T) {
	for _, tc := range []struct {
		name, base string
		channel    int
		price      int64
	}{
		{"dedicated_paid", "https://fixture.invalid", channeltype.DeepSeek, 23},
		{"official_claude_paid", "https://api.deepseek.com/anthropic", channeltype.Anthropic, 23},
		{"intentional_zero", "https://api.deepseek.com/anthropic", channeltype.Anthropic, 0},
		{"unrelated_claude", "https://api.anthropic.com", channeltype.Anthropic, 23},
	} {
		t.Run(tc.name, func(t *testing.T) {
			channel := &model.Channel{Type: tc.channel}
			require.NoError(t, channel.SetToolingConfig(&model.ChannelToolingConfig{Pricing: map[string]model.ToolPricingLocal{BuiltinWebFetch: {QuotaPerCall: tc.price}}}))
			meta := &metalib.Meta{ChannelType: tc.channel, BaseURL: tc.base, ActualModelName: "deepseek-v4-pro"}
			quotes, err := BuiltinToolQuotes(context.Background(), meta.ActualModelName, meta, channel, &deepseek.Adaptor{}, map[string]struct{}{BuiltinWebFetch: {}})
			require.NoError(t, err)
			require.Equal(t, tc.price, quotes[BuiltinWebFetch])
		})
	}
}

// TestDeepSeekToolReceiptNoAutomaticActivation retains explicit price
// configuration independent of the upstream grace-period boundary.
func TestDeepSeekToolReceiptNoAutomaticActivation(t *testing.T) {
	boundary := time.Date(2026, time.October, 31, 16, 0, 0, 0, time.UTC)
	for _, at := range []time.Time{boundary.Add(-time.Nanosecond), boundary, boundary.Add(time.Nanosecond)} {
		t.Run(at.Format(time.RFC3339Nano), func(t *testing.T) {
			metadata := &metalib.Meta{ChannelType: channeltype.DeepSeek, ActualModelName: "deepseek-v4-pro", StartTime: at}
			provider := &deepseek.Adaptor{}
			requested := map[string]struct{}{BuiltinWebSearch: {}}
			_, err := BuiltinToolQuotes(context.Background(), metadata.ActualModelName, metadata, nil, provider, requested)
			require.ErrorContains(t, err, "not allowed", "the calendar does not activate empty defaults")
			for _, price := range []int64{0, 17} {
				channel := &model.Channel{Type: channeltype.DeepSeek}
				require.NoError(t, channel.SetToolingConfig(&model.ChannelToolingConfig{Pricing: map[string]model.ToolPricingLocal{BuiltinWebSearch: {QuotaPerCall: price}}}))
				quotes, err := BuiltinToolQuotes(context.Background(), metadata.ActualModelName, metadata, channel, provider, requested)
				require.NoError(t, err)
				require.Equal(t, price, quotes[BuiltinWebSearch])
			}
		})
	}
}

// TestDeepSeekToolReceiptEndpointQuotes checks the planned full override rather
// than stale observed URLs, while preserving zero and unrelated host contracts.
func TestDeepSeekToolReceiptEndpointQuotes(t *testing.T) {
	for _, tc := range []struct {
		name, base, override, observed string
		channel                        int
		price                          int64
	}{
		{"planned_deepseek", "https://api.openai.com", "https://api.deepseek.com/v1/responses", "", channeltype.OpenAI, 17},
		{"planned_subdomain", "https://api.openai.com", "https://tools.deepseek.com/responses", "", channeltype.OpenAI, 17},
		{"planned_zero", "https://api.openai.com", "https://api.deepseek.com/v1/responses", "", channeltype.OpenAI, 0},
		{"reverse_base", "https://api.deepseek.com", "https://api.openai.com/v1/responses", "", channeltype.OpenAI, 17},
		{"reverse_channel", "https://api.deepseek.com", "https://api.openai.com/v1/responses", "", channeltype.DeepSeek, 17},
		{"stale_observed_deepseek", "https://api.openai.com", "", "https://api.deepseek.com/v1/responses", channeltype.OpenAI, 17},
		{"stale_observed_openai", "https://api.openai.com", "https://api.deepseek.com/v1/responses", "https://api.openai.com/v1/responses", channeltype.OpenAI, 17},
		{"hostname_lookalike", "https://api.openai.com", "https://api.deepseek.com.example/responses", "", channeltype.OpenAI, 17},
		{"query_not_host", "https://api.openai.com", "https://provider.example/responses?target=api.deepseek.com", "", channeltype.OpenAI, 17},
	} {
		t.Run(tc.name, func(t *testing.T) {
			channel := &model.Channel{Type: tc.channel}
			require.NoError(t, channel.SetToolingConfig(&model.ChannelToolingConfig{Pricing: map[string]model.ToolPricingLocal{BuiltinWebSearch: {QuotaPerCall: tc.price}}}))
			info := &metalib.Meta{ChannelType: tc.channel, BaseURL: tc.base, Mode: relaymode.ResponseAPI, ActualModelName: "deepseek-v4-pro", UpstreamRequestURL: tc.observed, Config: model.ChannelConfig{EndpointURLs: map[string]string{channeltype.RelayModeToEndpointName(relaymode.ResponseAPI): tc.override}}}
			quotes, err := BuiltinToolQuotes(context.Background(), info.ActualModelName, info, channel, &openai.Adaptor{}, map[string]struct{}{BuiltinWebSearch: {}})
			require.NoError(t, err)
			require.Equal(t, tc.price, quotes[BuiltinWebSearch])
		})
	}
}
