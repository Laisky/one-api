package controller

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/adaptor/gemini/tts"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/quota"
)

// TestGeminiSpeechPublishedPriceBoundary verifies absolute bills across the UTC promotion transition.
func TestGeminiSpeechPublishedPriceBoundary(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"gemini-3.8-flash-tts", "gemini-3.8-flash-lite-tts"} {
		for _, at := range []time.Time{time.Date(2026, 12, 31, 23, 59, 59, 999999999, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)} {
			input := quota.ComputeInput{ModelName: name, GroupRatio: 1, PricingAdaptor: &gemini.Adaptor{}, RequestTime: at}
			input.ModelRatio = pricing.ResolveModelRatioAt(name, nil, nil, input.PricingAdaptor, at)
			prices, err := newGeminiSpeechPrices(input)
			require.NoError(t, err)
			receipt := tts.Receipt{PromptTokens: 1000, CachedTokens: 200, OutputTokens: 100, Accepted: true, UsageComplete: true}
			total, err := prices.charge(receipt)
			require.NoError(t, err)
			// 800 * $0.50/1M + 200 * $0.125/1M + 100 * $9 (or $6)/1M,
			// converted at 500,000 quota/USD and rounded once.
			want := int64(663)
			if name == "gemini-3.8-flash-lite-tts" {
				want = 513
			}
			if at.Year() == 2027 {
				if name == "gemini-3.8-flash-tts" {
					want = 1325
				} else {
					want = 1025
				}
			}
			require.Equal(t, want, total)
			reserve, err := prices.reserve(100)
			require.NoError(t, err)
			require.GreaterOrEqual(t, reserve, total)
		}
	}
}

// TestGeminiSpeechDecimalRoundingAndOverrides prevents rounding drift and preserves configured prices.
func TestGeminiSpeechDecimalRoundingAndOverrides(t *testing.T) {
	t.Parallel()
	result, err := sumGeminiSpeechTerms([]float64{10, 0.1}, []float64{20, 0.05})
	require.NoError(t, err)
	require.Equal(t, int64(2), result)
	for _, invalid := range []float64{-1, math.NaN(), math.Inf(1), math.MaxFloat64} {
		_, err := sumGeminiSpeechTerms([]float64{invalid})
		require.Error(t, err)
	}
	const name = "gemini-3.8-flash-tts"
	cfg := dbmodel.ModelConfigLocal{Ratio: 2, CompletionRatio: 3, CachedInputRatio: -1}
	input := quota.ComputeInput{ModelName: name, ModelRatio: 2, GroupRatio: 1, PricingAdaptor: &gemini.Adaptor{}, ChannelModelConfigs: map[string]dbmodel.ModelConfigLocal{name: cfg}}
	prices, err := newGeminiSpeechPrices(input)
	require.NoError(t, err)
	value, err := prices.charge(tts.Receipt{PromptTokens: 12, CachedTokens: 4, OutputTokens: 2})
	require.NoError(t, err)
	require.Equal(t, int64(28), value)
	input.GroupRatio = 0
	prices, err = newGeminiSpeechPrices(input)
	require.NoError(t, err)
	value, err = prices.reserve(16384)
	require.NoError(t, err)
	require.Zero(t, value)
}

// TestVertexSpeechRequiresOwnedTariff rejects Developer API billing fallback before ADC or dispatch.
func TestVertexSpeechRequiresOwnedTariff(t *testing.T) {
	const balance = int64(10000)
	xaiVideoSetup(t, balance, false)
	c, _, _ := protocolContext(t, channeltype.VertextAI, "gemini-3.8-flash-tts", "/v1/audio/speech", `{"model":"alias","input":"hello","voice":"Kore","response_format":"pcm"}`, "https://example.test", balance, 1, false, nil)
	_, err := loadGeminiSpeechPrices(c, meta.GetByContext(c))
	require.ErrorContains(t, err, "explicit channel")
}
