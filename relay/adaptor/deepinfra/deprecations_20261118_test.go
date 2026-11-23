package deepinfra_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/deepinfra"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/quota"
)

// usd3 is a token tariff in USD per 1M tokens: input, output and cache read.
// A zero cache read means the model publishes no cache price.
type usd3 struct{ in, out, cached float64 }

// deprecatedOracle is one model that https://api.deepinfra.com/models/<model>
// (fetched 2026-11-18T14:45Z-14:46Z) marks `deprecated`, with the UTC date of
// that timestamp, its `replaced_by` model, and both models' published token
// tariffs (list price times 1 - discount; cache = input times
// rate_per_input_token_cached). Non-token models carry zero tariffs.
type deprecatedOracle struct {
	id, date, replacedBy string
	own, replacement     usd3
}

// deepInfraDeprecatedOracle returns the API snapshot described above. It takes
// no arguments and returns one row per deprecated catalog model.
func deepInfraDeprecatedOracle() []deprecatedOracle {
	return []deprecatedOracle{
		{"Audio8/Audio8-TTS-Preview-0.6b", "2026-11-16", "hexgrad/Kokoro-82M", usd3{}, usd3{}},
		{"Bria/fibo", "2026-10-27", "Bria/fibo-2.0", usd3{}, usd3{}},
		{"Gryphe/MythoMax-L2-13b", "2026-10-29", "Sao10K/L3-8B-Lunaris-v1-Turbo", usd3{0.4, 0.4, 0}, usd3{0.04, 0.05, 0}},
		{"MiniMaxAI/MiniMax-M2.7", "2026-09-10", "MiniMaxAI/MiniMax-M3", usd3{0.25, 1, 0.05}, usd3{0.28, 1.1, 0.056}},
		{"MiniMaxAI/MiniMax-M2.7-Turbo", "2026-09-30", "MiniMaxAI/MiniMax-M3", usd3{0.38, 1.7, 0.07}, usd3{0.28, 1.1, 0.056}},
		{"NousResearch/Hermes-3-Llama-3.1-405B", "2026-11-02", "deepseek-ai/DeepSeek-V4.1-Flash-1017", usd3{1, 1, 0}, usd3{0.098, 0.294, 0.00294}},
		{"NousResearch/Hermes-3-Llama-3.1-70B", "2026-11-02", "deepseek-ai/DeepSeek-V4.1-Flash-1017", usd3{0.7, 0.7, 0}, usd3{0.098, 0.294, 0.00294}},
		{"Qwen/Qwen3-Max", "2026-10-10", "Qwen/Qwen3.8-Max", usd3{1.2, 6, 0.24}, usd3{1.65, 4.951, 0.206}},
		{"Qwen/Qwen3-Max-Thinking", "2026-10-10", "Qwen/Qwen3.8-Max", usd3{1.2, 6, 0.24}, usd3{1.65, 4.951, 0.206}},
		{"Qwen/Qwen3.5-122B-A10B", "2026-10-01", "Qwen/Qwen3.5-397B-A17B", usd3{0.29, 2.4, 0}, usd3{0.45, 3, 0.22}},
		{"Qwen/Qwen3.8-2.4T-A95B", "2026-10-12", "zai-org/GLM-5.3", usd3{2, 6, 0.2}, usd3{0.5625, 2.5, 0.125}},
		{"XiaomiMiMo/MiMo-V2.5", "2026-09-29", "XiaomiMiMo/MiMo-V2.6-Flash", usd3{0.133, 0.266, 0.00266}, usd3{0.14, 0.28, 0.0028}},
		{"XiaomiMiMo/MiMo-V2.5-Pro", "2026-09-29", "XiaomiMiMo/MiMo-V2.6-Pro", usd3{0.39, 1.17, 0.078}, usd3{0.387, 0.783, 0.00324}},
		{"deepseek-ai/DeepSeek-V3-0324", "2026-09-29", "deepseek-ai/DeepSeek-V4.1-Flash", usd3{0.24, 0.9, 0.135}, usd3{0.14, 0.42, 0.0042}},
		{"google/embeddinggemma-300m", "2026-11-23", "google/embeddinggemma-2", usd3{}, usd3{}},
		{"google/gemma-4-E4B-it", "2026-10-01", "google/gemma-4-31B-it-turbo", usd3{0.02, 0.1, 0}, usd3{0.09, 0.34, 0.05}},
		{"ibm-granite/granite-4.2-30b", "2026-10-18", "ibm-granite/granite-4.2-8b", usd3{0.16, 0.65, 0.04}, usd3{0.06, 0.25, 0.015}},
		{"ibm-granite/granite-4.2-3b", "2026-10-18", "ibm-granite/granite-4.2-8b", usd3{0.03, 0.12, 0.0075}, usd3{0.06, 0.25, 0.015}},
		{"inclusionAI/Ling-3.0-flash-Fin", "2026-10-09", "inclusionAI/Ling-3.0-flash-VL", usd3{0.06, 0.18, 0.012}, usd3{0.06, 0.18, 0.012}},
		{"meta-llama/Llama-4-Maverick-17B-128E-Instruct-FP8", "2026-10-01", "google/gemma-4-31B-it-turbo", usd3{0.2, 0.8, 0}, usd3{0.09, 0.34, 0.05}},
		{"meta-llama/Meta-Llama-3.1-70B-Instruct-Turbo", "2026-11-03", "meta-llama/Llama-3.3-70B-Instruct-Turbo", usd3{0.4, 0.4, 0}, usd3{0.1, 0.32, 0}},
		{"moonshotai/Kimi-K2.5", "2026-09-07", "moonshotai/Kimi-K2.6", usd3{0.45, 2.25, 0.07}, usd3{0.75, 3.5, 0.15}},
		{"moonshotai/Kimi-K2.7-Code", "2026-09-29", "moonshotai/Kimi-K3", usd3{0.68, 3.4, 0.136}, usd3{2.85, 14.25, 0.285}},
		{"nvidia/Nemotron-3-Nano-30B-A3B", "2026-10-08", "nvidia/NVIDIA-Nemotron-3.5-Lightning", usd3{0.05, 0.2, 0.025}, usd3{0.06, 0.16, 0.03}},
		{"openai/gpt-oss-120b-Turbo", "2026-10-20", "openai/gpt-oss-120b", usd3{0.15, 0.6, 0}, usd3{0.037, 0.17, 0.0185}},
		{"stepfun-ai/Step-3.7-Flash", "2026-10-01", "XiaomiMiMo/MiMo-V2.6-Flash", usd3{0.16, 0.92, 0.032}, usd3{0.14, 0.28, 0.0028}},
		{"tencent/Hy3", "2026-10-08", "tencent/Hy4-preview", usd3{0.13, 0.53, 0.033}, usd3{0.834, 2.501, 0.042}},
		{"thinkingmachines/Inkling", "2026-10-22", "thinkingmachines/Inkling-Small", usd3{0.95, 4.05, 0.16}, usd3{0.45, 1.2, 0.1}},
		{"zai-org/GLM-4.7-Flash", "2026-09-10", "zai-org/GLM-5.3-Flash", usd3{0.06, 0.4, 0.01}, usd3{0.1005, 0.335, 0.0201}},
		{"zai-org/GLM-5", "2026-09-10", "zai-org/GLM-5.2", usd3{0.6, 2.08, 0.12}, usd3{0.5625, 1.8, 0.105}},
		{"zai-org/GLM-5.1", "2026-10-01", "zai-org/GLM-5.3", usd3{1.05, 3.5, 0.205}, usd3{0.5625, 2.5, 0.125}},
	}
}

// TestDeepInfraDeprecatedModelsAreMarkedButKept keeps every model the DeepInfra
// API marks deprecated (operators may still name it on a channel, and DeepInfra
// forwards such requests to the replacement) while making the catalog say so.
// It accepts t and returns no values.
func TestDeepInfraDeprecatedModelsAreMarkedButKept(t *testing.T) {
	t.Parallel()
	a := &deepinfra.Adaptor{}
	catalog := a.GetDefaultModelPricing()
	advertised := a.GetModelList()
	for _, row := range deepInfraDeprecatedOracle() {
		cfg, exists := catalog[row.id]
		require.True(t, exists, "%s stays configurable", row.id)
		require.Contains(t, advertised, row.id)
		require.Contains(t, cfg.Description, "Deprecated", row.id)
		require.Contains(t, cfg.Description, row.date, row.id)
		require.Contains(t, cfg.Description, "forwarded to "+row.replacedBy, row.id)
	}
}

// TestDeepInfraDeprecatedModelsNeverBillBelowForwardedTariff pins the tariff of
// each deprecated token model to the higher of its own published price and its
// replacement's, per dimension. DeepInfra documents that requests to a
// deprecated model are forwarded to the replacement
// (https://docs.deepinfra.com/models.md, "Model deprecation", fetched
// 2026-11-18) but not which of the two tariffs a forwarded request bills, so
// the ambiguity resolves toward never under-billing. A model without a
// published cache price keeps no cache ratio (cache reads bill at input). It
// accepts t and returns no values.
func TestDeepInfraDeprecatedModelsNeverBillBelowForwardedTariff(t *testing.T) {
	t.Parallel()
	catalog := (&deepinfra.Adaptor{}).GetDefaultModelPricing()
	for _, row := range deepInfraDeprecatedOracle() {
		if row.own.in == 0 {
			continue
		}
		cfg := catalog[row.id]
		wantIn := max(row.own.in, row.replacement.in)
		wantOut := max(row.own.out, row.replacement.out)
		require.InDelta(t, wantIn, cfg.Ratio/ratio.MilliTokensUsd, 1e-9, row.id)
		require.InDelta(t, wantOut, cfg.Ratio*cfg.CompletionRatio/ratio.MilliTokensUsd, 1e-9, row.id)
		if row.own.cached == 0 {
			require.Zero(t, cfg.CachedInputRatio, "%s publishes no cache price", row.id)
			continue
		}
		replacementCache := row.replacement.cached
		if replacementCache == 0 {
			replacementCache = row.replacement.in
		}
		require.InDelta(t, max(row.own.cached, replacementCache), cfg.CachedInputRatio/ratio.MilliTokensUsd, 1e-9, row.id)
	}
}

// TestDeepInfraEmbeddingGemmaForwardFloorStartsAtDeprecation checks the one
// deprecation that is still scheduled: google/embeddinggemma-300m is published
// at $0.002 per 1M input tokens until 2026-11-23T17:09:54Z, after which
// DeepInfra forwards it to google/embeddinggemma-2 at $0.02 (model API,
// fetched 2026-11-18). The higher tariff starts on that UTC date, so the
// current price is not overstated and a forwarded request is never
// under-billed. It accepts t and returns no values.
func TestDeepInfraEmbeddingGemmaForwardFloorStartsAtDeprecation(t *testing.T) {
	t.Parallel()
	a := &deepinfra.Adaptor{}
	cfg := a.GetDefaultModelPricing()["google/embeddinggemma-300m"]
	for _, tc := range []struct {
		at   time.Time
		want float64
	}{
		{time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC), 0.002},
		{time.Date(2026, 11, 22, 23, 59, 0, 0, time.UTC), 0.002},
		{time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC), 0.02},
		{time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), 0.02},
	} {
		resolved := pricing.ApplyTimeWindow(cfg, tc.at)
		require.InDelta(t, tc.want, resolved.Ratio/ratio.MilliTokensUsd, 1e-12, tc.at)
		require.NotNil(t, resolved.Embedding)
		require.InDelta(t, tc.want, resolved.Embedding.TextTokenRatio/ratio.MilliTokensUsd, 1e-12, tc.at)
		got := quota.Compute(quota.ComputeInput{Usage: &model.Usage{PromptTokens: 1_000_000},
			ModelName: "google/embeddinggemma-300m", ModelRatio: cfg.Ratio, GroupRatio: 1,
			PricingAdaptor: a, RequestTime: tc.at})
		require.EqualValues(t, int64(tc.want*float64(ratio.QuotaPerUsd)), got.TotalQuota, tc.at)
	}
}

// TestDeepInfraRemovedJanusModelsAreMarkedButKept covers the two catalog models
// DeepInfra removed outright: https://api.deepinfra.com/models/deepseek-ai/Janus-Pro-1B
// and .../Janus-Pro-7B return 404 {"detail":{"error":"model not found"}} and
// neither appears in https://api.deepinfra.com/models/list (fetched
// 2026-11-18). Requests fail upstream, so keeping the IDs cannot mis-bill; they
// stay for configured channels but must not read as current. It accepts t and
// returns no values.
func TestDeepInfraRemovedJanusModelsAreMarkedButKept(t *testing.T) {
	t.Parallel()
	catalog := (&deepinfra.Adaptor{}).GetDefaultModelPricing()
	for _, id := range []string{"deepseek-ai/Janus-Pro-1B", "deepseek-ai/Janus-Pro-7B"} {
		cfg, exists := catalog[id]
		require.True(t, exists, id)
		require.Contains(t, cfg.Description, "Removed by DeepInfra", id)
		require.Contains(t, cfg.Description, "404", id)
	}
}

// TestDeepInfraKokoroCharacterTariff pins hexgrad/Kokoro-82M to DeepInfra's
// published speech price: $0.99 per 1M input characters (model API
// cents_per_input_chars 9.9e-05 and model page "$0.99 / 1M characters", both
// fetched 2026-11-18). Speech is billed per character through the legacy unit
// ratio, which previously encoded $0.62 and under-billed every request by 37%.
// It accepts t and returns no values.
func TestDeepInfraKokoroCharacterTariff(t *testing.T) {
	t.Parallel()
	cfg := (&deepinfra.Adaptor{}).GetDefaultModelPricing()["hexgrad/Kokoro-82M"]
	require.InDelta(t, 0.99, cfg.Ratio/ratio.MilliTokensUsd, 1e-12)
	require.Contains(t, cfg.Description, "$0.99 per 1M input characters")
}
