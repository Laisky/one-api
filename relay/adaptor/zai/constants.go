package zai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/zhipu"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// usdPrice is the flat per-1M-token USD price Z.AI publishes.
//
// Z.AI's international pricing has exactly one value per column and NO tiering
// by input length or output length -- unlike open.bigmodel.cn, which tiers the
// very same model ids. Never port BigModel's tier tables onto these entries, and
// never derive these numbers by dividing the CNY price by an exchange rate: Z.AI
// charges materially more than 1/7 of the CNY list for most models
// (glm-4.7 is $0.60 here versus CNY 2 = ~$0.29 on BigModel).
//
// Source: https://docs.z.ai/guides/overview/pricing (verified 2026-08-26).
type usdPrice struct {
	Input  float64 // USD per 1M input tokens
	Cached float64 // USD per 1M cached-input tokens (0 = no published cache rate)
	Output float64 // USD per 1M output tokens
}

// derive clones the *metadata* of the identically named BigModel entry -- the
// two brands serve the same model, so context length, modalities, features and
// sampling parameters are genuinely shared -- and then replaces every *pricing*
// field with Z.AI's flat USD numbers.
//
// It panics when the base id is absent so a rename on the BigModel side fails
// loudly at boot (and in `go test`) rather than silently yielding an entry with
// zero context length. Mirrors zhipu.mergeModelRatios' duplicate-key panic.
func derive(base string, p usdPrice, mutate ...func(*adaptor.ModelConfig)) adaptor.ModelConfig {
	src, ok := zhipu.ModelRatios[base]
	if !ok {
		panic("zai: no BigModel base entry named " + base + " to derive metadata from")
	}
	cfg := src.Clone()

	cfg.Ratio = p.Input * ratio.MilliTokensUsd
	cfg.CachedInputRatio = p.Cached * ratio.MilliTokensUsd
	if p.Input > 0 {
		cfg.CompletionRatio = p.Output / p.Input
	} else {
		// Free model: Ratio 0 already makes output free, so the multiplier is moot.
		cfg.CompletionRatio = 1
	}

	// Drop every CNY-shaped pricing artifact. Z.AI is flat and USD-only, and any
	// per-call/per-image/per-second block is re-attached explicitly below.
	cfg.Tiers, cfg.TimeWindows = nil, nil
	cfg.CacheWrite5mRatio, cfg.CacheWrite1hRatio = 0, 0
	cfg.Video, cfg.Audio, cfg.Image, cfg.Embedding, cfg.PerCall = nil, nil, nil, nil, nil

	for _, m := range mutate {
		m(&cfg)
	}
	return cfg
}

// describe replaces the inherited BigModel description with a Z.AI-specific one.
func describe(s string) func(*adaptor.ModelConfig) {
	return func(cfg *adaptor.ModelConfig) { cfg.Description = s }
}

// perImageUsd re-prices a derived entry as a flat per-rendered-image charge,
// which is how Z.AI bills image generation.
func perImageUsd(usd float64) func(*adaptor.ModelConfig) {
	return func(cfg *adaptor.ModelConfig) {
		cfg.Ratio = 0
		cfg.CompletionRatio = 1
		cfg.CachedInputRatio = 0
		cfg.Image = &adaptor.ImagePricingConfig{PricePerImageUsd: usd}
	}
}

// perCallUsd re-prices a derived entry as a flat per-invocation charge, which is
// how Z.AI bills video generation. Ratio carries the quota-per-call consumed by
// the billing pipeline; PerCall carries the USD price for the display layer.
func perCallUsd(usd float64) func(*adaptor.ModelConfig) {
	return func(cfg *adaptor.ModelConfig) {
		cfg.Ratio = usd * ratio.QuotaPerUsd
		cfg.CompletionRatio = 1
		cfg.CachedInputRatio = 0
		cfg.PerCall = &adaptor.PerCallPricingConfig{UsdPerThousandCalls: usd * 1000}
	}
}

// ModelRatios is Z.AI's catalog and flat USD price table.
//
// It is a strict subset of BigModel's catalog plus a handful of Z.AI-only ids.
// Deliberately ABSENT because Z.AI does not serve them (listing them would let
// requests route to a 404 and bill at the generic fallback rate): embedding-2,
// embedding-3, rerank, glm-tts, glm-tts-clone, glm-realtime-*, charglm-4,
// codegeex-4, emohaa, and the glm-4-*/glm-4v-*/glm-z1-* legacy families.
// autoglm-phone-multilingual is valid in Z.AI's model enum but has no published
// price anywhere, so it is omitted rather than billed at the fallback rate.
var ModelRatios = map[string]adaptor.ModelConfig{
	// ---- flagship text ------------------------------------------------------
	"glm-5.3": derive("glm-5.3", usdPrice{Input: 1.40, Cached: 0.26, Output: 4.40},
		describe("GLM-5.3 on Z.AI: 1M context, 128K max output, always-on thinking with reasoning_effort low/high/max. Flat $1.40/$4.40 per 1M input/output, $0.26 cached input.")),
	"glm-5.2": derive("glm-5.2", usdPrice{Input: 1.40, Cached: 0.26, Output: 4.40},
		describe("GLM-5.2 on Z.AI: 1M lossless context, 128K max output, model-decided thinking. Flat $1.40/$4.40 per 1M input/output, $0.26 cached input.")),
	// BigModel tiers glm-5.1 at CNY 6 -> 8 by input length; Z.AI is flat and
	// prices it at the same $1.40 as GLM-5.2.
	"glm-5.1": derive("glm-5.1", usdPrice{Input: 1.40, Cached: 0.26, Output: 4.40},
		describe("GLM-5.1 on Z.AI: 200K context, 128K max output. Flat $1.40/$4.40 per 1M input/output, $0.26 cached input -- no input-length tiers.")),
	"glm-5": derive("glm-5", usdPrice{Input: 1.00, Cached: 0.20, Output: 3.20},
		describe("GLM-5 on Z.AI: 754B agentic engineering flagship, 200K context, 128K max output. Flat $1.00/$3.20 per 1M input/output, $0.20 cached input.")),
	"glm-5-turbo": derive("glm-5-turbo", usdPrice{Input: 1.20, Cached: 0.24, Output: 4.00},
		describe("GLM-5-Turbo on Z.AI: 200K context, 128K max output. Flat $1.20/$4.00 per 1M input/output, $0.24 cached input.")),
	"glm-4.7": derive("glm-4.7", usdPrice{Input: 0.60, Cached: 0.11, Output: 2.20},
		describe("GLM-4.7 on Z.AI: 358B agentic-coding flagship with interleaved thinking, 200K context, 128K max output. Flat $0.60/$2.20 per 1M input/output, $0.11 cached input -- no input- or output-length tiers.")),
	"glm-4.7-flashx": derive("glm-4.7-flashx", usdPrice{Input: 0.07, Cached: 0.01, Output: 0.40},
		describe("GLM-4.7-FlashX on Z.AI: high-throughput sibling of GLM-4.7, 200K context. Flat $0.07/$0.40 per 1M input/output, $0.01 cached input.")),
	"glm-4.7-flash": derive("glm-4.7-flash", usdPrice{},
		describe("GLM-4.7-Flash on Z.AI: free in every token class (input, cached input, and output), 200K context, 128K max output.")),
	"glm-4.6": derive("glm-4.6", usdPrice{Input: 0.60, Cached: 0.11, Output: 2.20},
		describe("GLM-4.6 on Z.AI: 355B/32B-active MoE coding model, 200K context. Flat $0.60/$2.20 per 1M input/output, $0.11 cached input.")),
	"glm-4.5": derive("glm-4.5", usdPrice{Input: 0.60, Cached: 0.11, Output: 2.20},
		describe("GLM-4.5 on Z.AI: 355B/32B-active MoE agentic flagship, 128K context, 96K max output. Flat $0.60/$2.20 per 1M input/output, $0.11 cached input.")),
	"glm-4.5-x": derive("glm-4.5-x", usdPrice{Input: 2.20, Cached: 0.45, Output: 8.90},
		describe("GLM-4.5-X on Z.AI: speed-optimized GLM-4.5, 128K context, 96K max output. Flat $2.20/$8.90 per 1M input/output, $0.45 cached input.")),
	"glm-4.5-air": derive("glm-4.5-air", usdPrice{Input: 0.20, Cached: 0.03, Output: 1.10},
		describe("GLM-4.5-Air on Z.AI: 106B/12B-active cost-efficient MoE, 128K context, 96K max output. Flat $0.20/$1.10 per 1M input/output, $0.03 cached input.")),
	"glm-4.5-airx": derive("glm-4.5-airx", usdPrice{Input: 1.10, Cached: 0.22, Output: 4.50},
		describe("GLM-4.5-AirX on Z.AI: speed-optimized GLM-4.5-Air, 128K context, 96K max output. Flat $1.10/$4.50 per 1M input/output, $0.22 cached input.")),
	// Retired on BigModel (2026-01-30, auto-routed to glm-4.7-flash) but still
	// live and free on Z.AI, where it is documented with a 200K context window.
	"glm-4.5-flash": derive("glm-4.5-flash", usdPrice{}, func(cfg *adaptor.ModelConfig) {
		cfg.ContextLength = 200_000
		cfg.Description = "GLM-4.5-Flash on Z.AI: free in every token class, 200K context, 96K max output. Still served on Z.AI although BigModel retired it on 2026-01-30."
	}),

	// ---- vision / multimodal ------------------------------------------------
	// Z.AI runs a 50% launch promotion ($0.075/$0.015/$0.25) that ends 24:00 on
	// 2026-09-09 (UTC+8). Billed at the standard list price so quota does not
	// silently under-charge once the promotion lapses.
	"glm-5.3-flash": derive("glm-5.3-flash", usdPrice{Input: 0.15, Cached: 0.03, Output: 0.50},
		describe("GLM-5.3-Flash on Z.AI: native multimodal Flash sibling of GLM-5.3, 1M context, 128K max output, always-on thinking. List pricing $0.15/$0.50 per 1M input/output, $0.03 cached input (a 50% launch promotion runs until 2026-09-09 24:00 UTC+8).")),
	"glm-5v-turbo": derive("glm-5v-turbo", usdPrice{Input: 1.20, Cached: 0.24, Output: 4.00},
		describe("GLM-5V-Turbo on Z.AI: 744B MoE native multimodal agent for visual programming and GUI automation, 200K context. Flat $1.20/$4.00 per 1M input/output, $0.24 cached input.")),
	"glm-4.6v": derive("glm-4.6v", usdPrice{Input: 0.30, Cached: 0.05, Output: 0.90},
		describe("GLM-4.6V on Z.AI: 106B/12B-active vision-reasoning MoE, 128K context, 32K max output. Flat $0.30/$0.90 per 1M input/output, $0.05 cached input.")),
	"glm-4.6v-flashx": derive("glm-4.6v-flashx", usdPrice{Input: 0.04, Cached: 0.004, Output: 0.40},
		describe("GLM-4.6V-FlashX on Z.AI: 9B lightweight vision-reasoning model, 128K context, 32K max output. Flat $0.04/$0.40 per 1M input/output, $0.004 cached input.")),
	"glm-4.6v-flash": derive("glm-4.6v-flash", usdPrice{},
		describe("GLM-4.6V-Flash on Z.AI: free vision-reasoning model, 128K context, 32K max output.")),
	"glm-4.5v": derive("glm-4.5v", usdPrice{Input: 0.60, Cached: 0.11, Output: 1.80},
		describe("GLM-4.5V on Z.AI: open-weight visual reasoning model, 64K context, 16K max output. Flat $0.60/$1.80 per 1M input/output, $0.11 cached input.")),

	// ---- OCR ----------------------------------------------------------------
	"glm-ocr": derive("glm-ocr", usdPrice{Input: 0.03, Output: 0.03},
		describe("GLM-OCR on Z.AI: layout-aware OCR for images and PDFs via /api/paas/v4/layout_parsing, billed per token at a flat $0.03 per 1M for both input and output.")),

	// ---- image generation (billed per rendered image) -----------------------
	"glm-image": derive("glm-image", usdPrice{}, perImageUsd(0.015),
		describe("GLM-Image on Z.AI: flagship text-to-image model, $0.015 per image; default 1280x1280 at hd quality, custom sizes 1024-2048px in multiples of 32.")),
	// Z.AI's pricing page labels this "CogView-4" but its model enum requires the
	// dated id, so the BigModel id `cogview-4` is NOT valid here.
	"cogview-4-250304": derive("cogview-4", usdPrice{}, perImageUsd(0.01),
		describe("CogView-4 (250304) on Z.AI: text-to-image generation, $0.01 per image; default 1024x1024, custom sizes 512-2048px in multiples of 16.")),

	// ---- video generation (billed per call, flat regardless of duration) -----
	"cogvideox-3": derive("cogvideox-3", usdPrice{}, perCallUsd(0.20),
		describe("CogVideoX-3 on Z.AI: 5s or 10s clips up to 4K with optional AI sound effects, $0.20 per video (flat, independent of duration and resolution).")),
	"viduq1-text": derive("viduq1-text", usdPrice{}, perCallUsd(0.40),
		describe("Vidu Q1 (text-to-video) on Z.AI: fixed 5s 1080P clips from a text prompt, $0.40 per video.")),
	"viduq1-image": derive("viduq1-image", usdPrice{}, perCallUsd(0.40),
		describe("Vidu Q1 (image-to-video) on Z.AI: fixed 5s 1080P clips with BGM, $0.40 per video.")),
	"viduq1-start-end": derive("viduq1-start-end", usdPrice{}, perCallUsd(0.40),
		describe("Vidu Q1 (first/last frame) on Z.AI: 5s 1080P video from start and end frames, $0.40 per video.")),
	"vidu2-image": derive("vidu2-image", usdPrice{}, perCallUsd(0.20),
		describe("Vidu 2 (image-to-video) on Z.AI: fast 4s 720P clips with BGM, $0.20 per video.")),
	"vidu2-start-end": derive("vidu2-start-end", usdPrice{}, perCallUsd(0.20),
		describe("Vidu 2 (first/last frame) on Z.AI: 4s 720P video from start and end frames, $0.20 per video.")),
	"vidu2-reference": derive("vidu2-reference", usdPrice{}, perCallUsd(0.40),
		describe("Vidu 2 (reference-to-video) on Z.AI: 4s 720P clips from 1-3 reference images, $0.40 per video.")),

	// ---- audio --------------------------------------------------------------
	// Z.AI bills ASR per token, not per minute; the relay estimates audio tokens
	// as duration * PromptTokensPerSecond, so Ratio stays a per-token rate.
	// Z.AI has no text-to-speech and no realtime surface.
	"glm-asr-2512": derive("glm-asr-2512", usdPrice{Input: 0.03, Output: 0.03}, func(cfg *adaptor.ModelConfig) {
		cfg.Audio = &adaptor.AudioPricingConfig{PromptTokensPerSecond: 10}
		cfg.Description = "GLM-ASR-2512 on Z.AI: multilingual speech recognition via /api/paas/v4/audio/transcriptions; wav/mp3 up to 25MB and 30 seconds. Billed per token at a flat $0.03 per 1M (Z.AI quotes this as roughly $0.0024 per audio minute)."
	}),

	// ---- Z.AI-only ids (no BigModel counterpart to derive from) --------------
	"glm-4-32b-0414-128k": {
		Ratio:                       0.10 * ratio.MilliTokensUsd,
		CompletionRatio:             1, // $0.10 in, $0.10 out
		ContextLength:               128_000,
		MaxOutputTokens:             16_384,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "web_search"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "stop", "max_tokens"},
		Description:                 "GLM-4-32B-0414-128K on Z.AI: 128K context, 16K max output, no thinking mode. Flat $0.10 per 1M for both input and output; no cached-input rate published. Not offered on open.bigmodel.cn.",
	},
}

// ZaiToolingDefaults captures Z.AI's built-in tool pricing.
//
// Z.AI publishes a single flat $0.01-per-use web search backed by one engine
// (search-prime). BigModel's search_std / search_pro / search_pro_sogou /
// search_pro_quark tiers do not exist on this platform.
// Source: https://docs.z.ai/guides/overview/pricing (verified 2026-08-26).
var ZaiToolingDefaults = adaptor.ChannelToolConfig{
	Pricing: map[string]adaptor.ToolPricingConfig{
		"web_search":      {UsdPerCall: 0.01},
		"search-prime":    {UsdPerCall: 0.01},
		"search_pro_jina": {UsdPerCall: 0.01},
	},
}
