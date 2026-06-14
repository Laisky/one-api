package ali

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// qwenClosedModelRatios captures pricing and metadata for Alibaba's closed-weight
// Qwen tiers exposed via Model Studio (turbo / plus / max / long / flash) plus the
// closed-weight Qwen-VL, Qwen-Audio, Qwen-Math, Qwen-Coder, and Qwen-MT variants.
// HuggingFaceID is intentionally empty for these models because their weights are
// not published. Quantization is likewise unspecified (Alibaba does not document
// the served numeric format for managed models).
//
// All chat/coder/math token prices are expressed via ratio.MilliTokensRmb so that
// the project's RMB→USD exchange rate (8 RMB per USD, see
// relay/billing/ratio/model.go) is applied consistently. Aliyun publishes prices
// in CNY per 1M tokens; the encoding here is
//
//	(CNY per 1k tokens) * 1000 * ratio.MilliTokensRmb
//	= CNY/1M * ratio.MilliTokensRmb
//
// Tiered prices (Qwen3, qwen-plus, qwen-flash) record the base/lowest tier in
// Ratio with the higher-tier price range noted in the Description. Operators that
// need tier-accurate billing can layer ModelRatioTier entries on top.
//
// Context-length, max-output and pricing figures verified 2026-05-18 against
//   - https://www.alibabacloud.com/help/en/model-studio/model-pricing
//   - https://help.aliyun.com/zh/model-studio/getting-started/models
//   - https://help.aliyun.com/zh/model-studio/model-pricing
var qwenClosedModelRatios = map[string]adaptor.ModelConfig{
	// ----- Qwen Turbo (closed) -------------------------------------------------
	// qwen-turbo is documented as deprecated in favor of qwen-flash but remains
	// callable. Pricing: 0.31 CNY/1M input, 0.62 CNY/1M output.
	"qwen-turbo": {
		Ratio:                       0.00031 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2,
		CachedInputRatio:            0.2 * 0.00031 * 1000 * ratio.MilliTokensRmb,
		ContextLength:               1000000,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen Turbo: closed-weight cost-optimized chat tier with up to 1M context (deprecated; use qwen-flash).",
	},
	"qwen-turbo-latest": {
		Ratio:                       0.00031 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2,
		CachedInputRatio:            0.2 * 0.00031 * 1000 * ratio.MilliTokensRmb,
		ContextLength:               1000000,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen Turbo (latest snapshot alias); deprecated alongside qwen-turbo.",
	},

	// ----- Qwen Flash (closed) -------------------------------------------------
	// qwen-flash supersedes qwen-turbo; tiered pricing 0.16-1.24 CNY/1M input,
	// 1.55-12.37 CNY/1M output across the 0-128K, 128K-256K, 256K-1M tiers.
	"qwen-flash": {
		Ratio:                       0.00016 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             9.6875, // 1.55 / 0.16
		CachedInputRatio:            0.2 * 0.00016 * 1000 * ratio.MilliTokensRmb,
		ContextLength:               1000000,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen Flash: closed-weight cost-optimized successor to qwen-turbo with tiered 1M context pricing (0-128K base tier billed here).",
	},
	"qwen-flash-latest": {
		Ratio:                       0.00016 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             9.6875,
		CachedInputRatio:            0.2 * 0.00016 * 1000 * ratio.MilliTokensRmb,
		ContextLength:               1000000,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen Flash (latest snapshot alias).",
	},

	// ----- Qwen Plus (closed) --------------------------------------------------
	// Tiered: 0.82/2.06 CNY (0-128K), 4.94/65.86 CNY (256K-1M).
	"qwen-plus": {
		Ratio:                       0.00082 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2.51, // 2.06 / 0.82
		CachedInputRatio:            0.2 * 0.00082 * 1000 * ratio.MilliTokensRmb,
		ContextLength:               1000000,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen Plus: closed-weight balanced tier with tiered 1M context pricing (base 0-128K tier billed here).",
	},
	"qwen-plus-latest": {
		Ratio:                       0.00082 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2.51,
		CachedInputRatio:            0.2 * 0.00082 * 1000 * ratio.MilliTokensRmb,
		ContextLength:               1000000,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen Plus (latest snapshot alias).",
	},

	// ----- Qwen Long (closed, document QA) -------------------------------------
	// qwen-long is optimized for long-context document QA at flat pricing.
	"qwen-long": {
		Ratio:                       0.0005 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             4, // 2.0 / 0.5 CNY per 1M
		CachedInputRatio:            0.2 * 0.0005 * 1000 * ratio.MilliTokensRmb,
		ContextLength:               10000000,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen Long: closed-weight long-context tier (up to 10M tokens) tuned for document QA.",
	},

	// ----- Qwen Max (closed) ---------------------------------------------------
	// Flat 2.47 / 9.88 CNY per 1M (no tiered pricing on managed -max).
	"qwen-max": {
		Ratio:                       0.00247 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             4, // 9.88 / 2.47
		CachedInputRatio:            0.2 * 0.00247 * 1000 * ratio.MilliTokensRmb,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen Max: closed-weight flagship chat tier (32K context, 2.47/9.88 CNY per 1M tokens).",
	},
	"qwen-max-latest": {
		Ratio:                       0.00247 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             4,
		CachedInputRatio:            0.2 * 0.00247 * 1000 * ratio.MilliTokensRmb,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen Max (latest snapshot alias).",
	},
	"qwen-max-longcontext": {
		Ratio:                       0.00247 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             4,
		CachedInputRatio:            0.2 * 0.00247 * 1000 * ratio.MilliTokensRmb,
		ContextLength:               30720,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen Max long-context legacy variant; superseded by tiered qwen-max.",
	},

	// ----- Qwen3 / Qwen3.5 / Qwen3.6 (closed) ----------------------------------
	// Aliyun pricing (verified 2026-05-19, Beijing CNY/1M):
	//   qwen3-max:           0-32K 2.5/10, 32K-128K 4/16, 128K-256K 7/28
	//   qwen3.5-plus:        0-128K 0.8/4.8, 128K-256K 2/12, 256K-1M 4/24
	//   qwen3.5-flash:       0-128K 0.2/2, 128K-256K 0.8/8, 256K-1M 1.2/12
	//   qwen3.6-plus:        0-256K 2/12, 256K-1M 8/48 (released 2026-04-02)
	//   qwen3.6-flash:       0-256K 1.2/7.2, 256K-1M 4.8/28.8 (released 2026-04-16)
	//   qwen3.6-max-preview: 0-128K 9/54, 128K-256K 15/90 (released 2026-04-20)
	"qwen3-max": {
		Ratio:                       0.0025 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             4, // 10 / 2.5
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen3 Max: closed-weight flagship with 256K context and tiered pricing (0-32K tier billed here).",
	},
	"qwen3-max-preview": {
		Ratio:                       0.0025 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             4,
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen3 Max Preview alias (matches qwen3-max pricing).",
	},
	"qwen3.5-plus": {
		Ratio:                       0.0008 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             6, // 4.8 / 0.8
		ContextLength:               1000000,
		MaxOutputTokens:             32768,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen3.5 Plus: closed-weight balanced tier with 1M context (0-128K base tier billed here).",
	},
	"qwen3.5-flash": {
		Ratio:                       0.0002 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             10, // 2 / 0.2
		ContextLength:               1000000,
		MaxOutputTokens:             32768,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen3.5 Flash: closed-weight cost-optimized tier with 1M context (0-128K base tier billed here).",
	},
	"qwen3.6-plus": {
		Ratio:                       0.002 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             6, // 12 / 2
		ContextLength:               1000000,
		MaxOutputTokens:             65536,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen3.6 Plus: closed-weight flagship released 2026-04-02 with 1M context (0-256K base tier billed here).",
	},
	"qwen3.6-flash": {
		Ratio:                       0.0012 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             6, // 7.2 / 1.2
		ContextLength:               1000000,
		MaxOutputTokens:             65536,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen3.6 Flash: closed-weight cost-optimized tier released 2026-04-16 with 1M context (0-256K base tier billed here).",
	},
	"qwen3.6-max-preview": {
		Ratio:                       0.009 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             6, // 54 / 9
		ContextLength:               262144,
		MaxOutputTokens:             65536,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenReasoningFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		MaxReasoningTokens:          65536,
		Description:                 "Qwen3.6 Max Preview: closed-weight reasoning flagship released 2026-04-20 (256K context, 0-128K base tier billed here).",
	},

	// ----- Qwen3.7 (closed) ----------------------------------------------------
	// Aliyun pricing (verified 2026-06-13, Beijing CNY/1M):
	//   qwen3.7-max:  12 input / 36 output, cache 1.2; reasoning, text-only, 1M context
	//   qwen3.7-plus: 2.88 input / 11.52 output, cache 0.288; vision+GUI, 1M context
	"qwen3.7-max": {
		Ratio:                       12 * ratio.MilliTokensRmb,
		CompletionRatio:             36.0 / 12.0,
		CachedInputRatio:            1.2 * ratio.MilliTokensRmb,
		ContextLength:               1000000,
		MaxOutputTokens:             65536,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenReasoningFeatures(),
		SupportedSamplingParameters: qwenReasoningSamplingParameters(),
		Description:                 "Qwen3.7 Max: closed-weight reasoning flagship (2026-05-20) with 1M context and native extended-thinking; text-only.",
	},
	"qwen3.7-max-latest": {
		Ratio:                       12 * ratio.MilliTokensRmb,
		CompletionRatio:             36.0 / 12.0,
		CachedInputRatio:            1.2 * ratio.MilliTokensRmb,
		ContextLength:               1000000,
		MaxOutputTokens:             65536,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenReasoningFeatures(),
		SupportedSamplingParameters: qwenReasoningSamplingParameters(),
		Description:                 "Qwen3.7 Max (latest alias).",
	},
	"qwen3.7-plus": {
		Ratio:                       2.88 * ratio.MilliTokensRmb,
		CompletionRatio:             11.52 / 2.88,
		CachedInputRatio:            0.288 * ratio.MilliTokensRmb,
		ContextLength:               1000000,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen3.7 Plus: closed-weight multimodal vision+GUI agent (GA 2026-06-01) with 1M context; supports image input and computer-use tasks.",
	},
	"qwen3.7-plus-latest": {
		Ratio:                       2.88 * ratio.MilliTokensRmb,
		CompletionRatio:             11.52 / 2.88,
		CachedInputRatio:            0.288 * ratio.MilliTokensRmb,
		ContextLength:               1000000,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen3.7 Plus (latest alias).",
	},

	// ----- Qwen-VL (closed vision) --------------------------------------------
	// qwen-vl-max: 1.65 / 4.11 CNY per 1M; qwen-vl-plus: 0.82 / 2.06 CNY per 1M.
	"qwen-vl-max": {
		Ratio:                       0.00165 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2.49, // 4.11 / 1.65
		ContextLength:               32000,
		MaxOutputTokens:             2000,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen-VL Max: closed-weight high-end multimodal model (text+image input).",
	},
	"qwen-vl-max-latest": {
		Ratio:                       0.00165 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2.49,
		ContextLength:               32000,
		MaxOutputTokens:             2000,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen-VL Max (latest snapshot alias).",
	},
	"qwen-vl-plus": {
		Ratio:                       0.00082 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2.51, // 2.06 / 0.82
		ContextLength:               32000,
		MaxOutputTokens:             2000,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen-VL Plus: closed-weight balanced multimodal tier.",
	},
	"qwen-vl-plus-latest": {
		Ratio:                       0.00082 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2.51,
		ContextLength:               32000,
		MaxOutputTokens:             2000,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen-VL Plus (latest snapshot alias).",
	},
	"qwen-vl-ocr": {
		Ratio:                       0.00515 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               34096,
		MaxOutputTokens:             4096,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen-VL OCR: closed-weight model tuned for document/scene OCR (5.15 CNY per 1M tokens).",
	},
	"qwen-vl-ocr-latest": {
		Ratio:                       0.00515 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               34096,
		MaxOutputTokens:             4096,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen-VL OCR (latest snapshot alias).",
	},

	// ----- Qwen3-VL (closed vision, tiered) ------------------------------------
	// qwen3-vl-plus: 1.03-3.09 CNY in / 10.30-30.87 CNY out; base tier 0-32K.
	"qwen3-vl-plus": {
		Ratio:                       0.00103 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             10, // 10.30 / 1.03
		ContextLength:               262144,
		MaxOutputTokens:             16384,
		InputModalities:             []string{"text", "image", "file"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen3-VL Plus: closed-weight multimodal (text/image/video) with tiered 256K context (0-32K base tier billed here).",
	},
	"qwen3-vl-flash": {
		Ratio:                       0.00015 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             10, // 1.5 / 0.15
		ContextLength:               262144,
		MaxOutputTokens:             16384,
		InputModalities:             []string{"text", "image", "file"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen3-VL Flash: closed-weight cost-optimized multimodal tier (0-32K base tier billed here).",
	},

	// ----- Qwen Omni (closed, multimodal) --------------------------------------
	// Aliyun pricing (verified 2026-05-19, Beijing CNY/1M, text-only billing
	// shown here; audio/image inputs are billed at separate rates documented in
	// the Aliyun model-pricing page):
	//   qwen3-omni-flash:  text-in 6.9 / text+audio-out 62.6
	//   qwen-omni-turbo:   text-in 1.6 / text+audio-out 50
	// The relay records the text input/output rate; audio billing must be
	// resolved upstream via the multimodal billing path.
	"qwen3-omni-flash": {
		Ratio:                       0.0069 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2.29, // 15.8 / 6.9 (text-only audio-disabled output)
		ContextLength:               262144,
		MaxOutputTokens:             16384,
		InputModalities:             []string{"text", "image", "file"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen3-Omni Flash: closed-weight multimodal (text/image/audio/video) tier (text I/O rate billed here; audio billing is per-modality upstream).",
	},
	"qwen-omni-turbo": {
		Ratio:                       0.0016 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             15.625, // 25 / 1.6 (text-only audio-disabled output)
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text", "image", "file"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen-Omni Turbo: closed-weight multimodal tier (text I/O rate billed here; audio billing is per-modality upstream).",
	},
	// Aliyun pricing (verified 2026-06-13, Beijing CNY/1M):
	//   qwen3.5-omni-plus: 2.88 input / 34.56 output (audio output is expensive)
	"qwen3.5-omni-plus": {
		Ratio:                       2.88 * ratio.MilliTokensRmb,
		CompletionRatio:             34.56 / 2.88,
		ContextLength:               262144,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text", "image", "audio", "video"},
		OutputModalities:            []string{"text", "audio"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen3.5 Omni Plus: closed-weight multimodal model accepting text/image/audio/video inputs with text and audio outputs; 262K context.",
	},

	// ----- Qwen-Audio (closed) -------------------------------------------------
	"qwen-audio-turbo": {
		Ratio:                       0,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2000,
		InputModalities:             []string{"text", "file"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{},
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen-Audio Turbo: closed-weight audio-understanding model (currently free trial).",
	},

	// ----- Qwen Math (closed) --------------------------------------------------
	// qwen-math-plus: 4.11 / 12.37 CNY per 1M.
	"qwen-math-plus": {
		Ratio:                       0.00411 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             3.01, // 12.37 / 4.11
		ContextLength:               4096,
		MaxOutputTokens:             3072,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen-Math Plus: closed-weight model tuned for mathematical reasoning.",
	},
	"qwen-math-plus-latest": {
		Ratio:                       0.00411 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             3.01,
		ContextLength:               4096,
		MaxOutputTokens:             3072,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen-Math Plus (latest snapshot alias).",
	},
	"qwen-math-turbo": {
		Ratio:                       0.00206 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2.995, // 6.17 / 2.06
		ContextLength:               4096,
		MaxOutputTokens:             3072,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen-Math Turbo: cost-optimized closed-weight math model.",
	},
	"qwen-math-turbo-latest": {
		Ratio:                       0.00206 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2.995,
		ContextLength:               4096,
		MaxOutputTokens:             3072,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen-Math Turbo (latest snapshot alias).",
	},

	// ----- Qwen Coder (closed) -------------------------------------------------
	// qwen-coder-plus: 3.60 / 7.21 CNY per 1M; turbo: 2.06 / 6.17 CNY per 1M.
	"qwen-coder-plus": {
		Ratio:                       0.0036 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2.0, // 7.21 / 3.60
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen-Coder Plus: closed-weight model tuned for code generation/completion.",
	},
	"qwen-coder-plus-latest": {
		Ratio:                       0.0036 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2.0,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen-Coder Plus (latest snapshot alias).",
	},
	"qwen-coder-turbo": {
		Ratio:                       0.00206 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2.995, // 6.17 / 2.06
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen-Coder Turbo: cost-optimized closed-weight coder model.",
	},
	"qwen-coder-turbo-latest": {
		Ratio:                       0.00206 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2.995,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen-Coder Turbo (latest snapshot alias).",
	},

	// ----- Qwen3 Coder (closed, tiered) ----------------------------------------
	// Aliyun pricing (verified 2026-05-19, Beijing CNY/1M, 0-32K base tier):
	//   qwen3-coder-plus:  4 / 16   (256K-1M: 20 / 200)
	//   qwen3-coder-flash: 1 / 4    (256K-1M: 5 / 25)
	"qwen3-coder-plus": {
		Ratio:                       0.004 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             4, // 16 / 4
		ContextLength:               1000000,
		MaxOutputTokens:             65536,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen3-Coder Plus: closed-weight code flagship with tiered 1M context (0-32K base tier billed here).",
	},
	"qwen3-coder-flash": {
		Ratio:                       0.001 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             4, // 4 / 1
		ContextLength:               1000000,
		MaxOutputTokens:             65536,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           qwenChatFeatures(),
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen3-Coder Flash: closed-weight cost-optimized code tier (0-32K base tier billed here).",
	},

	// ----- Qwen MT (machine translation, closed) -------------------------------
	// qwen-mt-plus: 1.86 / 5.57 CNY per 1M; turbo: 0.72 / 2.01 CNY per 1M.
	"qwen-mt-plus": {
		Ratio:                       0.00186 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2.995, // 5.57 / 1.86
		ContextLength:               4096,
		MaxOutputTokens:             2048,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{},
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen-MT Plus: closed-weight machine-translation tier.",
	},
	"qwen-mt-turbo": {
		Ratio:                       0.00072 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             2.79, // 2.01 / 0.72
		ContextLength:               4096,
		MaxOutputTokens:             2048,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{},
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen-MT Turbo: cost-optimized closed-weight translation model.",
	},

	// ----- Legacy closed-weight Qwen 1.x chat ---------------------------------
	"qwen-72b-chat": {
		Ratio:                       0.004 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen 72B Chat (Qwen 1 generation, hosted only).",
	},
	"qwen-14b-chat": {
		Ratio:                       0.001 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen 14B Chat (Qwen 1 generation, hosted only).",
	},
	"qwen-7b-chat": {
		Ratio:                       0.0005 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen 7B Chat (Qwen 1 generation, hosted only).",
	},
	"qwen-1.8b-chat": {
		Ratio:                       0.0003 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen 1.8B Chat (Qwen 1 generation, hosted only).",
	},
	"qwen-1.8b-longcontext-chat": {
		Ratio:                       0.0003 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             2048,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen 1.8B Long-Context Chat (Qwen 1 generation, hosted only).",
	},

	// ----- Legacy closed Qwen-VL / Qwen-Audio v1 ------------------------------
	"qwen-vl-v1": {
		Ratio:                       0.001 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               2048,
		MaxOutputTokens:             1500,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{},
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen-VL v1: legacy closed-weight multimodal base model.",
	},
	"qwen-vl-chat-v1": {
		Ratio:                       0.001 * 1000 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               2048,
		MaxOutputTokens:             1500,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{},
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen-VL Chat v1: legacy closed-weight multimodal chat model.",
	},
	"qwen-audio-chat": {
		Ratio:                       0,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2000,
		InputModalities:             []string{"text", "file"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{},
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Description:                 "Qwen-Audio Chat: legacy closed-weight audio chat model (free trial).",
	},
}
