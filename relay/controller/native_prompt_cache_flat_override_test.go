package controller

import (
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// nativeFlatOverride is a flat operator tariff deliberately distinct from every
// bundled native catalog rate: input 3, 30m write 6 and output 12 quota per
// token. A flat override has no tiers, so it applies at every input size.
var nativeFlatOverride = model.ModelConfigLocal{Ratio: 3, CompletionRatio: 4, CacheWrite5mRatio: 6}

// nativeFlatWriteModels are native OpenAI IDs whose 30m writes OpenAI creates
// implicitly and this gateway meters, including the GPT-6.2 entries added by
// the November 22 catalogue refresh.
var nativeFlatWriteModels = []string{"gpt-6.2-astra", "gpt-6.2-sol", "gpt-6.1-terra", "gpt-5.6-sol"}

// TestNativePromptCacheFlatOverrideReservation proves that a flat channel
// override on a native OpenAI GPT-5.6+ model funds OpenAI's default implicit
// 30m cache write before dispatch. Previously only tiered configs used complete
// admission, so the hold priced the prompt at the flat input rate while the
// receipt settled every prompt token at the higher configured write rate.
// Parameter t owns the SQLite ledger and the fake OpenAI upstream.
func TestNativePromptCacheFlatOverrideReservation(t *testing.T) {
	for _, modelName := range nativeFlatWriteModels {
		for _, protocol := range []string{"chat", "responses", "messages"} {
			t.Run(modelName+"/"+protocol, func(t *testing.T) {
				const balance = int64(100_000_000)
				const receiptInput, receiptOutput = 2_000, 128
				xaiVideoSetup(t, balance, false)
				var calls atomic.Int32
				upstream, observations := implicitCacheUpstream(t, modelName, receiptInput, receiptOutput, &calls)
				path, body := cyberCacheRequest(protocol, receiptInput, "")
				local := nativeFlatOverride
				result := relayNativeCacheRequest(t, modelName, protocol, path, body, upstream, balance, &local)
				require.False(t, result.failed, result.message)
				require.EqualValues(t, 1, calls.Load())
				observation := <-observations
				require.NoError(t, observation.err)
				_, pinned := observation.body["prompt_cache_options"]
				require.False(t, pinned, "a priced flat write must keep OpenAI's default caching enabled")
				reserved := balance - observation.userQuota
				highest := int64(result.promptTokens)*6 + 128*12
				require.GreaterOrEqual(t, reserved, highest,
					"the hold must cover implicit writes of the whole admitted prompt at the flat write rate")
				requireExactSettlement(t, result, receiptInput*6+receiptOutput*12)
			})
		}
	}
}

// TestNativePromptCacheFlatOverrideClientControls proves Responses clients
// keep their explicit cache controls under a flat override: explicit mode
// without breakpoints disables writes and is funded and settled at the flat
// input rate, while an explicit breakpoint re-enables writes that the hold must
// fund. Parameter t owns the SQLite ledger and the fake OpenAI upstream.
func TestNativePromptCacheFlatOverrideClientControls(t *testing.T) {
	for _, tc := range []struct {
		name, root, part string
		writes           bool
	}{
		{name: "explicit_opt_out", root: `"prompt_cache_options":{"mode":"explicit"},`},
		{name: "explicit_breakpoint", root: `"prompt_cache_options":{"mode":"explicit"},`, part: `,"prompt_cache_breakpoint":{"mode":"explicit"}`, writes: true},
		{name: "implicit_breakpoint", part: `,"prompt_cache_breakpoint":{"mode":"explicit"}`, writes: true},
	} {
		for _, modelName := range []string{"gpt-6.2-astra", "gpt-6.2-sol"} {
			t.Run(modelName+"/"+tc.name, func(t *testing.T) {
				const balance = int64(100_000_000)
				const receiptInput, receiptOutput = 2_000, 128
				xaiVideoSetup(t, balance, false)
				var calls atomic.Int32
				upstream, observations := implicitCacheUpstream(t, modelName, receiptInput, receiptOutput, &calls)
				_, body := cyberCacheRequest("responses", receiptInput, tc.part)
				body = strings.Replace(body, `{"model":"alias",`, `{"model":"alias",`+tc.root, 1)
				local := nativeFlatOverride
				result := relayNativeCacheRequest(t, modelName, "responses", "/v1/responses", body, upstream, balance, &local)
				require.False(t, result.failed, result.message)
				observation := <-observations
				require.NoError(t, observation.err)
				reserved := balance - observation.userQuota
				inputRate, exact := int64(3), int64(receiptInput*3+receiptOutput*12)
				if tc.writes {
					inputRate, exact = 6, receiptInput*6+receiptOutput*12
				}
				require.GreaterOrEqual(t, reserved, int64(result.promptTokens)*inputRate+128*12)
				if !tc.writes {
					require.Less(t, reserved, int64(result.promptTokens)*6,
						"an explicit opt-out must not be held as a cache write")
				}
				requireExactSettlement(t, result, exact)
			})
		}
	}
}

// TestNativePromptCacheFlatOverrideStaysFlat pins the exact admission quote of
// a flat native override: complete admission funds the configured write rate
// but never restores the bundled catalog's long-context tier above 272,000
// input tokens. Hosts without a verified native write contract keep their
// historical flat quote. Parameter t owns assertions.
func TestNativePromptCacheFlatOverrideStaysFlat(t *testing.T) {
	at := time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)
	for _, modelName := range nativeFlatWriteModels {
		local := map[string]model.ModelConfigLocal{modelName: nativeFlatOverride}
		request := &relaymodel.GeneralOpenAIRequest{Model: modelName, MaxTokens: 1,
			Messages: []relaymodel.Message{{Role: "user", Content: "fixture"}}}
		for _, host := range []int{channeltype.OpenAI, channeltype.Azure, channeltype.OpenAICompatible} {
			m := &metalib.Meta{ChannelType: host, APIType: channeltype.ToAPIType(host), ActualModelName: modelName,
				Mode: relaymode.ChatCompletions, StartTime: at}
			for _, prompt := range []int{100_000, 272_001} {
				quote, cache, err := estimatePreConsumedQuotaWithCachePolicy(request, &relaymodel.Usage{PromptTokens: prompt},
					3, 4, nil, 1, local, nil, m)
				require.NoError(t, err)
				inputRate := int64(3)
				if host == channeltype.OpenAI {
					inputRate = 6
				}
				require.Equal(t, int64(prompt)*inputRate+12+config.PreConsumedQuota*3, quote,
					"%s host %d must keep its flat tariff at %d tokens", modelName, host, prompt)
				require.Equal(t, host == channeltype.OpenAI, cache.write5m)
				require.False(t, cache.write1h)
				require.False(t, cache.pinExplicit)
			}
		}
	}
}

// TestNativePromptCacheFlatOverrideFollowUpQuotes covers the admission
// consumers that quote again after the first hold: every MCP Chat round and
// every WebSocket response.create frame must fund implicit writes at the flat
// write rate. Parameter t owns assertions.
func TestNativePromptCacheFlatOverrideFollowUpQuotes(t *testing.T) {
	const name = "gpt-5.6-sol"
	local := nativeFlatOverride
	c, _, _ := protocolContext(t, channeltype.OpenAI, name, "/v1/responses", "{}", "", 1_000_000, 1, false, &local)
	m := metalib.GetByContext(c)
	m.OriginModelName, m.ActualModelName = name, name
	round, aggregate, err := quoteMCPTierRoundQuota(c, m, name, 100_000, 1, 1,
		[]byte(`{"model":"gpt-5.6-sol","input":[{"role":"user","content":"hello"}]}`),
		&relaymodel.Usage{PromptTokens: 25, CompletionTokens: 7, ToolsCost: 11})
	require.NoError(t, err)
	require.EqualValues(t, 100_000*6+12, round)
	require.EqualValues(t, 100_025*6+8*12+11, aggregate)

	funding := newResponseWSTierFunding(c, m, 0)
	require.True(t, funding.enabled, "a native flat write contract must fund every WebSocket frame")
	frame := []byte(`{"type":"response.create","model":"gpt-5.6-sol","input":"hello","max_output_tokens":1}`)
	added, quoted, err := funding.additionalQuote(gmw.Ctx(c), frame)
	require.NoError(t, err)
	var request openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal(frame, &request))
	prompt := getResponseAPIPromptTokens(gmw.Ctx(c), &request)
	require.Equal(t, int64(prompt*6+12), quoted)
	require.Equal(t, quoted, added)
}
