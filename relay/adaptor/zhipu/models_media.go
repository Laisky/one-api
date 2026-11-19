package zhipu

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// imageGenerationModels enumerates Zhipu's image and video generation models.
// BigModel prices every model in this table per call (元/次), never per token, so
// none of them carry a token Ratio:
//
//   - Image models set Ratio=0 and carry Image.PricePerImageUsd (published RMB
//     price / ratio.ExchangeRateRmb), matching the OpenAI/xAI image convention
//     consumed by calculateImageBaseQuota.
//   - Video models encode the per-call price as quota (Ratio = price in RMB *
//     ratio.QuotaPerRMB) and surface it through PerCall.UsdPerThousandCalls,
//     which relay/controller/video.go bills from.
//
// Pricing verified 2026-08-26 against https://bigmodel.cn/pricing.
var imageGenerationModels = map[string]adaptor.ModelConfig{
	"cogview-4": {
		Ratio:           0,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.06 / ratio.ExchangeRateRmb,
		},
		InputModalities:  textInput(),
		OutputModalities: []string{"image"},
		Description:      "CogView-4: high-quality text-to-image generation model with multi-resolution support; ¥0.06 per image.",
	},
	"glm-image": {
		Ratio:           0,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.1 / ratio.ExchangeRateRmb,
		},
		InputModalities:  textInput(),
		OutputModalities: []string{"image"},
		Description:      "GLM-Image: flagship text-to-image generation model (max 1000-char prompt), multi-resolution 512px-2048px in multiples of 32; ¥0.1 per image.",
	},
	"cogview-3-plus": {
		Ratio:           0,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.08 / ratio.ExchangeRateRmb,
		},
		InputModalities:  textInput(),
		OutputModalities: []string{"image"},
		Description:      "CogView-3-Plus: enhanced CogView-3 image generator. (retired/superseded; no longer listed among current image-generation models, which now comprise GLM-Image, CogView-4, and CogView-3-Flash)",
	},
	"cogview-3": {
		Ratio:           0,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.04 / ratio.ExchangeRateRmb,
		},
		InputModalities:  textInput(),
		OutputModalities: []string{"image"},
		Description:      "CogView-3: text-to-image generation model. (retired/superseded; no longer listed among current image-generation models, which now comprise GLM-Image, CogView-4, and CogView-3-Flash)",
	},
	"cogview-3-flash": {
		Ratio:            0,
		CompletionRatio:  1,
		CachedInputRatio: 0,
		InputModalities:  textInput(),
		OutputModalities: []string{"image"},
		Description:      "CogView-3-Flash: free fast text-to-image generation model.",
	},
	// CogVideoX-3: ¥1 per call, 5s/10s clips up to 4K.
	// Source: https://docs.bigmodel.cn/cn/guide/models/video-generation/cogvideox-3
	"cogvideox-3": {
		Ratio:            1 * ratio.QuotaPerRMB,
		CompletionRatio:  1,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"video"},
		PerCall:          &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 1.0 / 7 * 1000},
		Description:      "CogVideoX-3: flagship text/image-to-video generation model supporting first/last-frame control, up to 4K resolution at ¥1/call (supersedes CogVideoX/CogVideoX-2).",
	},
	// CogVideoX-2: ¥0.5 per call (Batch API ¥0.25).
	"cogvideox-2": {
		Ratio:            0.5 * ratio.QuotaPerRMB,
		CompletionRatio:  1,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"video"},
		PerCall:          &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 0.5 / 7 * 1000},
		Description:      "CogVideoX-2: previous-generation text/image-to-video model at ¥0.5/call (superseded by CogVideoX-3).",
	},
	// CogVideoX (legacy id) is priced like the generation that superseded it,
	// CogVideoX-2 at ¥0.5 per call. It previously carried a per-TOKEN Ratio, which
	// contradicts this file's stated convention and, because relay/controller/video.go
	// needs either a PerCall price or a Video block, made every request for it fail
	// with HTTP 400 video_pricing_missing — unbillable rather than mis-billed.
	"cogviewx": {
		Ratio:            0.5 * ratio.QuotaPerRMB,
		CompletionRatio:  1,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"video"},
		PerCall:          &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 0.5 / 7 * 1000},
		Description:      "CogVideoX: text-and-image-to-video generation model at ¥0.5/call. (legacy id; no live Zhipu model currently uses this id -- current video-generation ids are cogvideox-3 and cogvideox-flash)",
	},
	"cogvideox-flash": {
		Ratio:            0,
		PerCall:          &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 0},
		CompletionRatio:  1,
		CachedInputRatio: 0,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"video"},
		Description:      "CogVideoX-Flash: free fast text-to-video generator with 4K and 60fps support.",
	},
	"cogviewx-flash": {
		Ratio:            0,
		CompletionRatio:  1,
		CachedInputRatio: 0,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"video"},
		Description:      "Legacy misspelled catalog ID; configure model mapping to cogvideox-flash. Not a provider model ID.",
	},
	// Vidu Q1: high-quality 5s / 1080P video generation, ¥2.5 per call.
	// Source: https://docs.bigmodel.cn/cn/guide/models/video-generation/viduq1
	"viduq1-image": {
		Ratio:            2.5 * ratio.QuotaPerRMB,
		CompletionRatio:  1,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"video"},
		PerCall:          &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 2.5 / 7 * 1000},
		Description:      "Vidu Q1 (image-to-video): fixed 5s 1080P clips at ¥2.5/call; high-fidelity video generation for cinematic scenes.",
	},
	"viduq1-start-end": {
		Ratio:            2.5 * ratio.QuotaPerRMB,
		CompletionRatio:  1,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"video"},
		PerCall:          &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 2.5 / 7 * 1000},
		Description:      "Vidu Q1 (first/last frame): generates 5s 1080P video from start and end frames at ¥2.5/call.",
	},
	"viduq1-text": {
		Ratio:            2.5 * ratio.QuotaPerRMB,
		CompletionRatio:  1,
		InputModalities:  []string{"text"},
		OutputModalities: []string{"video"},
		PerCall:          &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 2.5 / 7 * 1000},
		Description:      "Vidu Q1 (text-to-video): fixed 5s 1080P clips from text prompts at ¥2.5/call.",
	},
	// Vidu 2: fast 4s / 720P video generation, ¥1.25 per call (reference mode ¥2.5).
	// Source: https://docs.bigmodel.cn/cn/guide/models/video-generation/vidu2
	"vidu2-image": {
		Ratio:            1.25 * ratio.QuotaPerRMB,
		CompletionRatio:  1,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"video"},
		PerCall:          &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 1.25 / 7 * 1000},
		Description:      "Vidu 2 (image-to-video): fast 4s 720P clips at ¥1.25/call; stable and color-accurate for e-commerce scenes.",
	},
	"vidu2-start-end": {
		Ratio:            1.25 * ratio.QuotaPerRMB,
		CompletionRatio:  1,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"video"},
		PerCall:          &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 1.25 / 7 * 1000},
		Description:      "Vidu 2 (first/last frame): fast 4s 720P video from start and end frames at ¥1.25/call.",
	},
	"vidu2-reference": {
		Ratio:            2.5 * ratio.QuotaPerRMB,
		CompletionRatio:  1,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"video"},
		PerCall:          &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 2.5 / 7 * 1000},
		Description:      "Vidu 2 (reference-to-video): 4s 720P clips from reference images of people or objects at ¥2.5/call.",
	},
}
