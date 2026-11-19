package siliconflow

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// mossImageModels returns the independently owned MOSS text-to-image tariff.
// It takes no parameters. The published CNY image price uses the repository's
// fixed exchange-rate convention, not a live market conversion. Sources checked
// on 2026-11-16:
// https://docs.siliconflow.cn/docs/release-notes/overview
// https://docs.siliconflow.cn/docs/api/images-generations-post
// https://www.siliconflow.cn/news/yj02gsluu233nit05mtf8m9m
func mossImageModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"OpenMOSS-Team/MOSS-Image": {
			CompletionRatio: 1,
			InputModalities: []string{"text"}, OutputModalities: []string{"image"},
			SupportedFeatures: []string{"gateway_siliconflow_image"},
			Image: &adaptor.ImagePricingConfig{
				PricePerImageUsd: 0.30 / ratio.ExchangeRateRmb,
				MinImages:        1, MaxImages: 1,
			},
			Description: "OpenMOSS MOSS-Image on SiliconFlow, launched 2026-11-03 at CNY 0.30 per image. Uses the existing JSON /v1/images/generations contract with one output per gateway call. The USD image tariff uses the repository's fixed CNY/USD conversion. No unpublished model-specific output size, token-priced chat contract or image-editing capability is inferred.",
		},
	}
}
