package controller

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openrouter"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestOpenRouterGPT62ImplicitWriteAdmission checks that a long prompt and tiny
// output allowance fund automatic cache creation before dispatch, including the
// 272K tier boundary and channel overrides. Parameter t reports failures.
func TestOpenRouterGPT62ImplicitWriteAdmission(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		input, write, output float64
	}{
		{"openai/gpt-6.2-astra", 4, 5, 20},
		{"openai/gpt-6.2-astra-pro", 4, 5, 20},
		{"openai/gpt-6.2-sol", 1, 1.25, 5},
		{"openai/gpt-6.2-sol-pro", 1, 1.25, 5},
	} {
		for _, prompt := range []int{100_000, 272_000, 272_001} {
			m := &metalib.Meta{ChannelType: channeltype.OpenRouter, APIType: channeltype.ToAPIType(channeltype.OpenRouter),
				ActualModelName: tc.name, Mode: relaymode.ChatCompletions, StartTime: time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)}
			request := &model.GeneralOpenAIRequest{Model: tc.name, MaxTokens: 1,
				Messages: []model.Message{{Role: "user", Content: "fixture"}}}
			quote, cache, err := estimatePreConsumedQuotaWithCachePolicy(request, &model.Usage{PromptTokens: prompt},
				tc.input, tc.output/tc.input, nil, 1, nil, nil, m)
			require.NoError(t, err)
			require.True(t, cache.write5m, "%s/%d must fund default 30m writes", tc.name, prompt)
			require.False(t, cache.write1h)
			require.False(t, cache.pinExplicit, "OpenRouter must not inherit a native request rewrite")
			input, write, output := tc.input, tc.write, tc.output
			if prompt > 272_000 {
				input, write, output = input*2, write*2, output*1.5
			}
			want := int64(math.Ceil(float64(prompt)*write + output + float64(config.PreConsumedQuota)*input))
			require.Equal(t, want, quote, "%s/%d", tc.name, prompt)
			if prompt == 272_001 {
				local := map[string]dbmodel.ModelConfigLocal{tc.name: {Tiers: []dbmodel.ModelRatioTierLocal{{
					Ratio: 20, CompletionRatio: 2, CacheWrite5mRatio: 30, InputTokenThreshold: 272_001,
				}}}}
				quote, cache, err = estimatePreConsumedQuotaWithCachePolicy(request, &model.Usage{PromptTokens: prompt},
					tc.input, tc.output/tc.input, nil, 1, local, nil, m)
				require.NoError(t, err)
				require.True(t, cache.write5m)
				require.Equal(t, int64(272_001*30+40)+config.PreConsumedQuota*20, quote,
					"channel write tariffs keep their existing precedence")
			}
		}
	}
}

// TestOpenRouterPromptCacheControls checks exact prepared payloads: opt-outs,
// Chat and Responses breakpoints, translated controls, unsupported lifetimes,
// and caller data that must not be interpreted as controls. Parameter t asserts.
func TestOpenRouterPromptCacheControls(t *testing.T) {
	m := &metalib.Meta{ChannelType: channeltype.OpenRouter, ActualModelName: "openai/gpt-6.2-astra", StartTime: time.Now()}
	for _, tc := range []struct {
		name, body string
		writes     bool
		refused    bool
	}{
		{name: "automatic", body: `{"messages":[{"role":"user","content":"hello"}]}`, writes: true},
		{name: "explicit_opt_out", body: `{"prompt_cache_options":{"mode":"explicit"},"messages":[{"role":"user","content":"hello"}]}`},
		{name: "chat_breakpoint", body: `{"prompt_cache_options":{"mode":"explicit"},"messages":[{"role":"user","content":[{"type":"text","text":"hi","prompt_cache_breakpoint":{"ttl":"30m"}}]}]}`, writes: true},
		{name: "responses_breakpoint", body: `{"prompt_cache_options":{"mode":"explicit"},"input":[{"role":"user","content":[{"type":"input_text","text":"hi","prompt_cache_breakpoint":{"ttl":"30m"}}]}]}`, writes: true},
		{name: "translated_1h_marker", body: `{"prompt_cache_options":{"mode":"explicit"},"messages":[{"role":"user","content":[{"type":"text","text":"hi","cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`, writes: true},
		{name: "responses_translated_1h_marker", body: `{"prompt_cache_options":{"mode":"explicit"},"input":[{"role":"user","content":[{"type":"input_text","text":"hi","cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`, writes: true},
		{name: "root_12h", body: `{"prompt_cache_options":{"ttl":"12h"}}`, refused: true},
		{name: "chat_12h", body: `{"messages":[{"role":"user","content":[{"type":"text","text":"hi","prompt_cache_breakpoint":{"ttl":"12h"}}]}]}`, refused: true},
		{name: "responses_12h", body: `{"input":[{"role":"user","content":[{"type":"input_text","text":"hi","prompt_cache_breakpoint":{"ttl":"12h"}}]}]}`, refused: true},
		{name: "malformed_options", body: `{"prompt_cache_options":"explicit"}`, refused: true},
		{name: "caller_data", body: `{"prompt_cache_options":{"mode":"explicit"},"tools":[{"type":"function","function":{"name":"f","parameters":{"prompt_cache_breakpoint":{"ttl":"12h"}}}}],"messages":[{"role":"user","content":"{\"prompt_cache_breakpoint\":{\"ttl\":\"12h\"}}"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := json.RawMessage(tc.body)
			before := string(payload)
			got, err := decideTierAdmissionCacheWrites(m, m.ActualModelName, payload, 100_000, nil, &openrouter.Adaptor{})
			if tc.refused {
				require.Error(t, err)
				require.True(t, adaptor.IsUnbillableRequestControl(err))
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.writes, got.write5m)
				require.False(t, got.write1h, "OpenRouter drops Anthropic TTLs when translating to OpenAI")
				require.False(t, got.pinExplicit)
			}
			require.Equal(t, before, string(payload), "admission must not change provider bytes")
		})
	}
	for _, channel := range []int{channeltype.Azure, channeltype.OpenAICompatible, channeltype.Groq} {
		other := *m
		other.ChannelType = channel
		got, err := decideTierAdmissionCacheWrites(&other, m.ActualModelName, map[string]any{"input": "hi"}, 100_000, nil, nil)
		require.NoError(t, err)
		require.False(t, got.write5m, "an OpenRouter tariff cannot qualify another host")
	}
}
