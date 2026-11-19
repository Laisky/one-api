package controller

import (
	"math"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestNativePromptCacheAdmissionBoundary pins the Cyber 272,000/272,001 input
// boundary at admission. At 272,000 tokens the short-context tier prices
// OpenAI's implicit 30m writes, so the hold covers the whole prompt at the
// $25/M write rate and caching stays enabled. At 272,001 tokens the
// long-context write price is unpublished: caching is pinned off and the hold
// covers the long-context input/output rates. An explicit operator tier with a
// write rate re-qualifies caching at the operator rate.
func TestNativePromptCacheAdmissionBoundary(t *testing.T) {
	const maxOutput = 1_000
	buffer := float64(config.PreConsumedQuota)
	operator := map[string]model.ModelConfigLocal{"gpt-6-cyber": {Tiers: []model.ModelRatioTierLocal{{
		Ratio: 20, CompletionRatio: 3.75, CachedInputRatio: 2, CacheWrite5mRatio: 25, InputTokenThreshold: 272_001,
	}}}}
	for _, tc := range []struct {
		name    string
		prompt  int
		configs map[string]model.ModelConfigLocal
		write5m bool
		pin     bool
		quote   float64
	}{
		{name: "short_context_272000", prompt: 272_000, write5m: true, quote: 272_000*12.5 + maxOutput*50 + buffer*10},
		{name: "long_context_272001", prompt: 272_001, pin: true, quote: 272_001*20 + maxOutput*75 + buffer*20},
		{name: "operator_long_context_272001", prompt: 272_001, configs: operator, write5m: true, quote: 272_001*25 + maxOutput*75 + buffer*20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &metalib.Meta{ChannelType: channeltype.OpenAI, ChannelId: 7, APIType: channeltype.ToAPIType(channeltype.OpenAI),
				ActualModelName: "gpt-6-cyber", Mode: relaymode.ChatCompletions, StartTime: time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)}
			request := &relaymodel.GeneralOpenAIRequest{Model: "gpt-6-cyber", MaxTokens: maxOutput,
				Messages: []relaymodel.Message{{Role: "user", Content: "fixture"}}}
			quote, cache, err := estimatePreConsumedQuotaWithCachePolicy(request, &relaymodel.Usage{PromptTokens: tc.prompt},
				10, 5, nil, 1, tc.configs, nil, m)
			require.NoError(t, err)
			require.Equal(t, tc.write5m, cache.write5m)
			require.Equal(t, tc.pin, cache.pinExplicit)
			require.Equal(t, int64(math.Ceil(tc.quote)), quote, "the hold must equal the highest permitted charge plus buffer")
		})
	}
}

// TestNativePromptCacheAdmissionScope proves the implicit-write contract
// applies only to native OpenAI GPT-5.6+ models that price cache writes:
// hosted and compatible channels keep their operator semantics, and older
// OpenAI models (no write tariff) keep caching unfunded.
func TestNativePromptCacheAdmissionScope(t *testing.T) {
	for _, tc := range []struct {
		channel int
		model   string
		implied bool
	}{
		{channeltype.OpenAI, "gpt-6-cyber", true},
		{channeltype.OpenAI, "gpt-6.1-terra", true},
		{channeltype.OpenAI, " GPT-6-SOL ", true},
		{channeltype.OpenAI, "gpt-4o-mini", false},
		{channeltype.Azure, "gpt-6.1-terra", false},
		{channeltype.OpenAICompatible, "gpt-6-cyber", false},
	} {
		require.Equal(t, tc.implied, openai.ImplicitPromptCacheWrites(tc.channel, tc.model), "%d/%s", tc.channel, tc.model)
		m := &metalib.Meta{ChannelType: tc.channel, ActualModelName: tc.model, StartTime: time.Now()}
		decision, err := decideTierAdmissionCacheWrites(m, strings.TrimSpace(strings.ToLower(tc.model)), map[string]any{"input": "hi"}, 1_000, nil, nil)
		require.NoError(t, err)
		require.Equal(t, tc.implied, decision.write5m)
		require.False(t, decision.pinExplicit)
	}

	// A WebSocket frame (no request context) is forwarded unchanged, so a
	// required pin cannot be applied and the frame must be refused instead.
	cyber := &metalib.Meta{ChannelType: channeltype.OpenAI, ActualModelName: "gpt-6-cyber", StartTime: time.Now()}
	_, _, err := tierAdmissionCacheWritesForDispatch(nil, cyber, "gpt-6-cyber", map[string]any{"input": "hi"}, 272_001, nil, nil)
	require.True(t, adaptor.IsUnbillableRequestControl(err), "an unpinnable unpriced write must be refused: %v", err)
	_, _, err = tierAdmissionCacheWritesForDispatch(nil, cyber, "gpt-6-cyber", map[string]any{"input": "hi"}, 272_000, nil, nil)
	require.NoError(t, err)
}

// TestNativeRequestControlRefusalsAreClientErrors proves client controls the
// gateway cannot bill (a non-Standard Cyber service tier, an unmetered 12h
// cache lifetime) are answered as HTTP 400 before dispatch on every client
// format. They previously surfaced as 500 convert_request_failed, which reads
// as a gateway fault and pages operators for a client mistake.
func TestNativeRequestControlRefusalsAreClientErrors(t *testing.T) {
	for _, tc := range []struct {
		name, protocol, body string
	}{
		{name: "chat_priority_tier", protocol: "chat", body: `{"model":"alias","service_tier":"priority","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`},
		{name: "responses_priority_tier", protocol: "responses", body: `{"model":"alias","service_tier":"priority","max_output_tokens":16,"input":"hi"}`},
		{name: "responses_12h_options", protocol: "responses", body: `{"model":"alias","prompt_cache_options":{"ttl":"12h"},"max_output_tokens":16,"input":"hi"}`},
		{name: "responses_12h_breakpoint", protocol: "responses", body: `{"model":"alias","max_output_tokens":16,"input":[{"role":"user","content":[{"type":"input_text","text":"hi","prompt_cache_breakpoint":{"ttl":"12h"}}]}]}`},
		{name: "responses_12h_tool_output", protocol: "responses", body: `{"model":"alias","max_output_tokens":16,"input":[{"type":"function_call_output","call_id":"call_1","output":[{"type":"input_text","text":"result","prompt_cache_breakpoint":{"ttl":"12h"}}]}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(10_000_000)
			xaiVideoSetup(t, balance, false)
			var calls atomic.Int32
			upstream, _ := implicitCacheUpstream(t, "gpt-6-cyber", 10, 1, &calls)
			path := "/v1/chat/completions"
			if tc.protocol == "responses" {
				path = "/v1/responses"
			}
			result := relayNativeCacheRequest(t, "gpt-6-cyber", tc.protocol, path, tc.body, upstream, balance, nil)
			require.True(t, result.failed)
			require.Equal(t, http.StatusBadRequest, result.status, result.message)
			require.NotContains(t, result.message, "prepare tiered Responses provider body", "clients see the refusal, not internal wrap context")
			require.NotContains(t, result.message, "convert request failed")
			require.Zero(t, calls.Load(), "a refused control must never reach the provider")
			require.Zero(t, result.charged)
			require.Zero(t, result.tokenCharged)
		})
	}
}
