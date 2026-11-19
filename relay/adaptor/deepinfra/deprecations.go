package deepinfra

import (
	"maps"

	"github.com/Laisky/one-api/relay/adaptor"
)

// deprecationsVerifiedAt is the UTC date the table below was checked against
// https://api.deepinfra.com/models/<model> (fetched 2026-11-18T14:45Z-14:46Z).
const deprecationsVerifiedAt = "2026-11-18"

// deprecation records one model the DeepInfra model API marks `deprecated`.
// Date is the UTC date of that timestamp and ReplacedBy is its `replaced_by`.
type deprecation struct {
	Date       string
	ReplacedBy string
}

// deprecatedModels lists every catalog model whose DeepInfra model API record
// carries a `deprecated` timestamp. DeepInfra keeps such models callable and
// "inference requests are automatically forwarded to a recommended replacement
// model" (https://docs.deepinfra.com/models.md, "Model deprecation", fetched
// 2026-11-18), so the IDs stay for channels that still name them. The family
// files keep each model's own published tariff; applyDeprecations marks the
// model and floors its token tariff at the replacement's (see there).
var deprecatedModels = map[string]deprecation{
	"Audio8/Audio8-TTS-Preview-0.6b":                    {"2026-11-16", "hexgrad/Kokoro-82M"},
	"Bria/fibo":                                         {"2026-10-27", "Bria/fibo-2.0"},
	"Gryphe/MythoMax-L2-13b":                            {"2026-10-29", "Sao10K/L3-8B-Lunaris-v1-Turbo"},
	"MiniMaxAI/MiniMax-M2.7":                            {"2026-09-10", "MiniMaxAI/MiniMax-M3"},
	"MiniMaxAI/MiniMax-M2.7-Turbo":                      {"2026-09-30", "MiniMaxAI/MiniMax-M3"},
	"NousResearch/Hermes-3-Llama-3.1-405B":              {"2026-11-02", "deepseek-ai/DeepSeek-V4.1-Flash-1017"},
	"NousResearch/Hermes-3-Llama-3.1-70B":               {"2026-11-02", "deepseek-ai/DeepSeek-V4.1-Flash-1017"},
	"Qwen/Qwen3-Max":                                    {"2026-10-10", "Qwen/Qwen3.8-Max"},
	"Qwen/Qwen3-Max-Thinking":                           {"2026-10-10", "Qwen/Qwen3.8-Max"},
	"Qwen/Qwen3.5-122B-A10B":                            {"2026-10-01", "Qwen/Qwen3.5-397B-A17B"},
	"Qwen/Qwen3.8-2.4T-A95B":                            {"2026-10-12", "zai-org/GLM-5.3"},
	"XiaomiMiMo/MiMo-V2.5":                              {"2026-09-29", "XiaomiMiMo/MiMo-V2.6-Flash"},
	"XiaomiMiMo/MiMo-V2.5-Pro":                          {"2026-09-29", "XiaomiMiMo/MiMo-V2.6-Pro"},
	"deepseek-ai/DeepSeek-V3-0324":                      {"2026-09-29", "deepseek-ai/DeepSeek-V4.1-Flash"},
	"google/embeddinggemma-300m":                        {"2026-11-23", "google/embeddinggemma-2"},
	"google/gemma-4-E4B-it":                             {"2026-10-01", "google/gemma-4-31B-it-turbo"},
	"ibm-granite/granite-4.2-30b":                       {"2026-10-18", "ibm-granite/granite-4.2-8b"},
	"ibm-granite/granite-4.2-3b":                        {"2026-10-18", "ibm-granite/granite-4.2-8b"},
	"inclusionAI/Ling-3.0-flash-Fin":                    {"2026-10-09", "inclusionAI/Ling-3.0-flash-VL"},
	"meta-llama/Llama-4-Maverick-17B-128E-Instruct-FP8": {"2026-10-01", "google/gemma-4-31B-it-turbo"},
	"meta-llama/Meta-Llama-3.1-70B-Instruct-Turbo":      {"2026-11-03", "meta-llama/Llama-3.3-70B-Instruct-Turbo"},
	"moonshotai/Kimi-K2.5":                              {"2026-09-07", "moonshotai/Kimi-K2.6"},
	"moonshotai/Kimi-K2.7-Code":                         {"2026-09-29", "moonshotai/Kimi-K3"},
	"nvidia/Nemotron-3-Nano-30B-A3B":                    {"2026-10-08", "nvidia/NVIDIA-Nemotron-3.5-Lightning"},
	"openai/gpt-oss-120b-Turbo":                         {"2026-10-20", "openai/gpt-oss-120b"},
	"stepfun-ai/Step-3.7-Flash":                         {"2026-10-01", "XiaomiMiMo/MiMo-V2.6-Flash"},
	"tencent/Hy3":                                       {"2026-10-08", "tencent/Hy4-preview"},
	"thinkingmachines/Inkling":                          {"2026-10-22", "thinkingmachines/Inkling-Small"},
	"zai-org/GLM-4.7-Flash":                             {"2026-09-10", "zai-org/GLM-5.3-Flash"},
	"zai-org/GLM-5":                                     {"2026-09-10", "zai-org/GLM-5.2"},
	"zai-org/GLM-5.1":                                   {"2026-10-01", "zai-org/GLM-5.3"},
}

// applyDeprecations marks every deprecated model in catalog and, for plain
// token tariffs, raises each price dimension to the replacement's when the
// replacement is dearer. DeepInfra does not document whether a forwarded
// request bills the deprecated model's tariff or the replacement's, so the
// higher of the two is charged and a forwarded request is never under-billed.
// Deprecations dated after deprecationsVerifiedAt, and media or embedding
// tariffs, are not lifted here. As checked, Audio8 TTS ($5 vs $0.99 per 1M
// characters) and Bria/fibo ($0.04 per image on both) already bill at least
// their replacement, and google/embeddinggemma-300m carries its own dated
// overlay in models_google.go. It mutates catalog in place and returns nothing.
func applyDeprecations(catalog map[string]adaptor.ModelConfig) {
	// Replacements are read from the published family tariffs, so the result
	// never depends on map iteration order.
	published := maps.Clone(catalog)
	for id, dep := range deprecatedModels {
		cfg, exists := catalog[id]
		if !exists {
			continue
		}
		note := "Deprecated: DeepInfra deprecation date " + dep.Date + " (model API, checked " +
			deprecationsVerifiedAt + "); requests from that date are forwarded to " + dep.ReplacedBy + "."
		replacement, known := published[dep.ReplacedBy]
		if known && dep.Date <= deprecationsVerifiedAt && liftToReplacementTariff(&cfg, replacement) {
			note += " Billed at the higher of its own and the replacement's published price per token dimension, because DeepInfra does not document which applies to a forwarded request."
		}
		if cfg.Description != "" {
			note = cfg.Description + " " + note
		}
		cfg.Description = note
		catalog[id] = cfg
	}
}

// liftToReplacementTariff raises cfg's input, output, cache-read and
// cache-write prices to replacement's where replacement is dearer. Both must
// be plain positive token tariffs without tiers, time windows or media
// pricing; otherwise nothing changes. A zero cache ratio keeps meaning "bill
// at input", so it is only compared, never invented. It accepts the deprecated
// model's config to mutate and the replacement's config, and returns whether
// any price changed.
func liftToReplacementTariff(cfg *adaptor.ModelConfig, replacement adaptor.ModelConfig) bool {
	if !plainTokenTariff(*cfg) || !plainTokenTariff(replacement) {
		return false
	}
	input := max(cfg.Ratio, replacement.Ratio)
	output := max(cfg.Ratio*cfg.CompletionRatio, replacement.Ratio*replacement.CompletionRatio)
	cached := liftCacheRatio(cfg.CachedInputRatio, replacement.CachedInputRatio, replacement.Ratio, input)
	write5m := liftCacheRatio(cfg.CacheWrite5mRatio, replacement.CacheWrite5mRatio, replacement.Ratio, input)
	write1h := liftCacheRatio(cfg.CacheWrite1hRatio, replacement.CacheWrite1hRatio, replacement.Ratio, input)
	if input == cfg.Ratio && output == cfg.Ratio*cfg.CompletionRatio && cached == cfg.CachedInputRatio &&
		write5m == cfg.CacheWrite5mRatio && write1h == cfg.CacheWrite1hRatio {
		return false
	}
	cfg.Ratio, cfg.CompletionRatio = input, output/input
	cfg.CachedInputRatio, cfg.CacheWrite5mRatio, cfg.CacheWrite1hRatio = cached, write5m, write1h
	return true
}

// liftCacheRatio returns a deprecated model's cache ratio raised to the
// replacement's effective cache price. A zero ratio means "bill at input" on
// either side, and a negative one means free. The own zero is kept while the
// lifted input already covers the replacement's price (always true for cache
// reads, which never exceed input); otherwise the replacement's price is set
// explicitly. It accepts the own and replacement ratios, the replacement's
// input ratio and the lifted input ratio, and returns the lifted ratio.
func liftCacheRatio(own, replacement, replacementInput, liftedInput float64) float64 {
	effectiveReplacement := max(replacement, 0)
	if replacement == 0 {
		effectiveReplacement = replacementInput
	}
	effectiveOwn := max(own, 0)
	if own == 0 {
		effectiveOwn = liftedInput
	}
	if effectiveReplacement <= effectiveOwn {
		return own
	}
	return effectiveReplacement
}

// plainTokenTariff reports whether cfg is a positive per-token tariff that a
// per-dimension comparison can lift safely: no tiers, time windows, media,
// embedding or per-call pricing. It accepts a model config and returns the
// result.
func plainTokenTariff(cfg adaptor.ModelConfig) bool {
	return cfg.Ratio > 0 && cfg.CompletionRatio > 0 && len(cfg.Tiers) == 0 && len(cfg.TimeWindows) == 0 &&
		cfg.Image == nil && cfg.Audio == nil && cfg.Video == nil && cfg.Embedding == nil &&
		cfg.PerCall == nil && cfg.PerPage == nil
}
