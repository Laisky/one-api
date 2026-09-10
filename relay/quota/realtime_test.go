package quota_test

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	modelcfg "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	rmodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
	"github.com/Laisky/one-api/relay/realtime"
)

// receiptInput constructs a session with consistent aggregate counters and the
// real OpenAI pricing adaptor, rather than a mock mirroring the billing formula.
func receiptInput(name string, records ...realtime.Record) quota.ComputeInput {
	ledger := realtime.NewLedger()
	ledger.Records = records
	for _, record := range records {
		ledger.InputTokens += record.Tokens.Input
		ledger.OutputTokens += record.Tokens.Output
	}
	return quota.ComputeInput{
		Usage: &rmodel.Usage{Realtime: ledger}, ModelName: name,
		ModelRatio: openai.ModelRatios[name].Ratio, GroupRatio: 1,
		PricingAdaptor: &openai.Adaptor{}, RequestTime: time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC),
	}
}

// TestRealtimeReceiptPublishedRates tests each modality independently against
// published USD prices. Values are not derived from implementation multipliers.
func TestRealtimeReceiptPublishedRates(t *testing.T) {
	t.Parallel()
	// Order: text in/cache/out, audio in/cache/out, image in/cache ($/1M).
	models := map[string][8]float64{
		"gpt-realtime":                            {4, .4, 16, 32, .4, 64, 5, .5},
		"gpt-realtime-1.5":                        {4, .4, 16, 32, .4, 64, 5, .5},
		"gpt-realtime-2":                          {4, .4, 24, 32, .4, 64, 5, .5},
		"gpt-realtime-2.1":                        {4, .4, 24, 32, .4, 64, 5, .5},
		"gpt-realtime-mini":                       {.6, .06, 2.4, 10, .3, 20, .8, .08},
		"gpt-realtime-2.1-mini":                   {.6, .06, 2.4, 10, .3, 20, .8, .08},
		"gpt-4o-realtime-preview":                 {5, 2.5, 20, 40, 2.5, 80, 0, 0},
		"gpt-4o-realtime-preview-2025-06-03":      {5, 2.5, 20, 40, 2.5, 80, 0, 0},
		"gpt-4o-mini-realtime-preview":            {.6, .3, 2.4, 10, .3, 20, 0, 0},
		"gpt-4o-mini-realtime-preview-2024-12-17": {.6, .3, 2.4, 10, .3, 20, 0, 0},
	}
	const n = 1_000_000
	buckets := []struct {
		name   string
		tokens realtime.Tokens
	}{
		{"text_input", realtime.Tokens{Input: n, Text: n}},
		{"cached_text", realtime.Tokens{Input: n, Text: n, CachedText: n}},
		{"text_output", realtime.Tokens{Output: n, OutputText: n}},
		{"audio_input", realtime.Tokens{Input: n, Audio: n}},
		{"cached_audio", realtime.Tokens{Input: n, Audio: n, CachedAudio: n}},
		{"audio_output", realtime.Tokens{Output: n, OutputAudio: n}},
		{"image_input", realtime.Tokens{Input: n, Image: n}},
		{"cached_image", realtime.Tokens{Input: n, Image: n, CachedImage: n}},
	}
	for name, prices := range models {
		for i, bucket := range buckets {
			if prices[i] == 0 {
				continue
			} // These previews do not support images.
			t.Run(name+"/"+bucket.name, func(t *testing.T) {
				result := quota.Compute(receiptInput(name, realtime.Record{Tokens: bucket.tokens}))
				require.Empty(t, result.BillingIssues)
				require.Equal(t, int64(math.Round(prices[i]*ratio.QuotaPerUsd)), result.TotalQuota)
			})
		}
	}
}

// TestRealtimeReceiptMixedAndTranscription checks additive ASR charges, cached
// subsets, text associated with speech, group scaling and session-level rounding.
func TestRealtimeReceiptMixedAndTranscription(t *testing.T) {
	t.Parallel()
	conversation := realtime.Record{Tokens: realtime.Tokens{
		Input: 6000, Text: 1000, Audio: 2000, Image: 3000,
		CachedText: 100, CachedAudio: 200, CachedImage: 300,
		Output: 2000, OutputText: 500, OutputAudio: 1500,
	}}
	// Hand calculation in USD: .0036+.00004+.0576+.00008+
	// .0135+.00015+.012+.096 = .18297 => 91485 quota.
	main := receiptInput("gpt-realtime-2.1", conversation)
	require.Equal(t, int64(91485), quota.Compute(main).TotalQuota)
	asr := realtime.Tokens{Input: 3000, Text: 1000, Audio: 2000, Output: 300, OutputText: 300}
	for _, tc := range []struct {
		model string
		want  int64
	}{
		{"gpt-4o-transcribe", 8750}, {"gpt-4o-mini-transcribe", 4375},
	} {
		t.Run(tc.model, func(t *testing.T) {
			input := receiptInput("gpt-realtime-2.1", conversation, realtime.Record{Model: tc.model, Tokens: asr})
			input.GroupRatio = 1.25
			input.Usage.ToolsCost = 7
			result := quota.Compute(input)
			require.Empty(t, result.BillingIssues)
			require.Equal(t, int64(math.Ceil(float64(91485+tc.want)*1.25))+7, result.TotalQuota)
			require.Equal(t, int64(7), input.Usage.ToolsCost, "calculation must not mutate usage")
			require.Equal(t, result, quota.Compute(input), "repeat computation must be idempotent")
		})
	}
}

// TestRealtimeReceiptDurationAndFreeSessions covers the zero-token paths that
// previously either retained the reservation or erased a duration-based charge.
func TestRealtimeReceiptDurationAndFreeSessions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		model   string
		seconds float64
		want    int64
	}{
		{"whisper-1", 12.5, 625}, {"whisper-1", 0, 0},
		{"gpt-realtime-whisper", 60, 8500},
	} {
		input := receiptInput("gpt-realtime", realtime.Record{Model: tc.model, Duration: true, Seconds: tc.seconds})
		result := quota.Compute(input)
		require.Empty(t, result.BillingIssues)
		require.Equal(t, tc.want, result.TotalQuota)
		require.Zero(t, result.PromptTokens+result.CompletionTokens)
	}
	require.Zero(t, quota.Compute(receiptInput("gpt-realtime")).TotalQuota)
	input := receiptInput("gpt-realtime", realtime.Record{Tokens: realtime.Tokens{Input: 100, Audio: 100}})
	input.GroupRatio = 0
	require.Zero(t, quota.Compute(input).TotalQuota)
	input.GroupRatio = 1
	input.ModelRatio = 0
	input.ChannelModelRatio = map[string]float64{"gpt-realtime": 0}
	require.Zero(t, quota.Compute(input).TotalQuota)
	// Making the conversation free must not accidentally make independent ASR free.
	input.Usage.Realtime.Records = append(input.Usage.Realtime.Records,
		realtime.Record{Model: "whisper-1", Duration: true, Seconds: 12.5})
	require.Equal(t, int64(625), quota.Compute(input).TotalQuota)
}

// TestRealtimeReceiptChannelOverrides verifies the existing JSON configuration
// and flat completion override contracts, including supplementary cache discounts.
func TestRealtimeReceiptChannelOverrides(t *testing.T) {
	t.Parallel()
	var cfg modelcfg.ModelConfigLocal
	require.NoError(t, json.Unmarshal([]byte(`{"ratio":2,"completion_ratio":3,"audio":{"prompt_ratio":4,"completion_ratio":5}}`), &cfg))
	input := receiptInput("gpt-realtime", realtime.Record{Tokens: realtime.Tokens{
		Input: 200, Text: 100, Audio: 100, Output: 30, OutputText: 10, OutputAudio: 20,
	}})
	input.ChannelModelConfigs = map[string]modelcfg.ModelConfigLocal{"gpt-realtime": cfg}
	input.ChannelModelRatio = map[string]float64{"gpt-realtime": 2}
	input.ChannelCompletionRatio = map[string]float64{"gpt-realtime": 3}
	input.ModelRatio, input.GroupRatio, input.Usage.ToolsCost = 2, 1.5, 7
	result := quota.Compute(input)
	require.Empty(t, result.BillingIssues)
	require.Equal(t, int64(2797), result.TotalQuota) // (200+800+60+800)*1.5+7
	input.ChannelCompletionRatio["gpt-realtime"] = 7
	require.Equal(t, int64(2917), quota.Compute(input).TotalQuota) // output text override only
}

// TestRealtimeReceiptRoundingAndUnknownUsage ensures no per-event minimum charge,
// no guessed model fallback, and no silent "complete" result for bad evidence.
func TestRealtimeReceiptRoundingAndUnknownUsage(t *testing.T) {
	t.Parallel()
	one := realtime.Record{Tokens: realtime.Tokens{Input: 1, Text: 1, CachedText: 1}}
	input := receiptInput("gpt-realtime-mini", one, one, one)
	require.Equal(t, int64(1), quota.Compute(input).TotalQuota) // ceil(.03*3), not 3
	input.Usage.Realtime.Issues = []string{"missing final usage"}
	require.Equal(t, []string{"missing final usage"}, quota.Compute(input).BillingIssues)
	unknown := receiptInput("gpt-realtime", realtime.Record{Model: "unknown-asr", Tokens: realtime.Tokens{Input: 1, Audio: 1}})
	result := quota.Compute(unknown)
	require.Zero(t, result.TotalQuota)
	require.NotEmpty(t, result.BillingIssues)
	bad := receiptInput("gpt-realtime", realtime.Record{Tokens: realtime.Tokens{Input: 1, Audio: 2}})
	require.NotEmpty(t, quota.Compute(bad).BillingIssues)
}

// TestRealtimeUsageCannotBeInjected asserts that client JSON cannot turn regular
// completion billing into the internal receipt-based accounting path.
func TestRealtimeUsageCannotBeInjected(t *testing.T) {
	t.Parallel()
	var usage rmodel.Usage
	require.NoError(t, json.Unmarshal([]byte(`{"prompt_tokens":100,"realtime":{"records":[]},"Realtime":{}}`), &usage))
	require.Nil(t, usage.Realtime)
	require.Equal(t, 100, usage.PromptTokens)
}
