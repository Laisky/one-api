package quota_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
	"github.com/Laisky/one-api/relay/realtime"
)

// TestRealtimeCatalogModelsAreAllPriceable walks every Realtime model the OpenAI
// adaptor advertises and settles a receipt of the kind that model produces.
// Parameters: t is the test handle. Returns: none.
//
// An advertised-but-unpriced realtime model is worse than a wrong price: the
// resolver leaves every bucket at zero, the session settles at zero, and the
// reservation floor that would otherwise catch it is skipped for trusted
// accounts. Realtime audio output is the most expensive thing OpenAI sells, so
// a silent free session is a material loss. gpt-realtime-2025-08-28 and
// gpt-realtime-mini-2025-12-15 were exactly that until they were catalogued.
func TestRealtimeCatalogModelsAreAllPriceable(t *testing.T) {
	t.Parallel()
	provider := &openai.Adaptor{}
	pricing := provider.GetDefaultModelPricing()
	checked := 0
	for name, config := range pricing {
		if !strings.Contains(name, "realtime") {
			continue
		}
		checked++
		t.Run(name, func(t *testing.T) {
			record := realtimeTokenReceipt()
			if durationPriced(config) {
				record = realtime.Record{Duration: true, Seconds: 60, Model: name}
			}
			ledger := realtime.NewLedger()
			ledger.Records = []realtime.Record{record}
			ledger.InputTokens, ledger.OutputTokens = record.Tokens.Input, record.Tokens.Output
			result := quota.Compute(quota.ComputeInput{
				Usage:          &model.Usage{Realtime: ledger},
				ModelName:      name,
				ModelRatio:     config.Ratio,
				GroupRatio:     1,
				PricingAdaptor: provider,
			})
			require.False(t, result.UnpricedUsage,
				"realtime model %q resolves no price; a session on it would settle free (issues: %v)",
				name, result.BillingIssues)
			require.Empty(t, result.BillingIssues)
			require.Positive(t, result.TotalQuota, "realtime model %q settles at zero quota", name)
		})
	}
	require.GreaterOrEqual(t, checked, 10, "the realtime catalog should not have shrunk unnoticed")
}

// realtimeTokenReceipt returns a receipt shaped like a real gpt-realtime turn:
// mixed text/audio input with a cached subset, and mixed text/audio output.
// Parameters: none. Returns: one chargeable token record.
func realtimeTokenReceipt() realtime.Record {
	return realtime.Record{Tokens: realtime.Tokens{
		Input: 131, Text: 119, Audio: 12, CachedText: 64,
		Output: 177, OutputText: 43, OutputAudio: 134,
	}}
}

// durationPriced reports whether a model bills by audio duration instead of
// tokens. Parameters: config is the catalog entry. Returns: true for per-minute
// realtime models such as gpt-realtime-whisper and gpt-realtime-translate.
func durationPriced(config adaptor.ModelConfig) bool {
	return config.Audio != nil && config.Audio.UsdPerSecond > 0
}

// TestRealtimeDatedSnapshotsMatchTheirAlias pins snapshot pricing to its alias.
// Parameters: t is the test handle. Returns: none. OpenAI publishes one price
// for an alias and its dated snapshot, so they must settle identically.
func TestRealtimeDatedSnapshotsMatchTheirAlias(t *testing.T) {
	t.Parallel()
	for snapshot, alias := range map[string]string{
		"gpt-realtime-2025-08-28":      "gpt-realtime",
		"gpt-realtime-mini-2025-12-15": "gpt-realtime-mini",
	} {
		t.Run(snapshot, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, realtimeSessionQuota(t, alias), realtimeSessionQuota(t, snapshot))
		})
	}
}

// realtimeSessionQuota settles one representative receipt under modelName.
// Parameters: t is the test handle and modelName the billed model. Returns: the
// session quota, asserting the model is fully priced.
func realtimeSessionQuota(t *testing.T, modelName string) int64 {
	t.Helper()
	provider := &openai.Adaptor{}
	config, ok := provider.GetDefaultModelPricing()[modelName]
	require.True(t, ok, "model %q is absent from the catalog", modelName)
	record := realtimeTokenReceipt()
	ledger := realtime.NewLedger()
	ledger.Records = []realtime.Record{record}
	ledger.InputTokens, ledger.OutputTokens = record.Tokens.Input, record.Tokens.Output
	result := quota.Compute(quota.ComputeInput{
		Usage:          &model.Usage{Realtime: ledger},
		ModelName:      modelName,
		ModelRatio:     config.Ratio,
		GroupRatio:     1,
		PricingAdaptor: provider,
	})
	require.False(t, result.UnpricedUsage)
	require.Positive(t, result.TotalQuota)
	require.EqualValues(t, 500000, ratio.QuotaPerUsd, "quota scale assumption")
	return result.TotalQuota
}
