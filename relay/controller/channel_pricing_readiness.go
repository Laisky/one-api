package controller

import (
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/model"
	relayadaptor "github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/quota"
)

const (
	// PricingRequirementImageFallback marks a GPT Image 2.5/2.6 model that has
	// neither a positive image.price_per_image_usd nor an image-output token
	// price to derive the default reservation from, so image admission answers
	// 503 image_billing_not_configured.
	PricingRequirementImageFallback = "image_price_per_image_usd"
	// PricingRequirementTokenTariff marks a model the provider declares
	// operator-priced (adaptor.OperatorTariffPolicy) whose channel entry lacks a
	// complete positive ratio and completion_ratio, tiers included, so dispatch
	// answers 503 model_pricing_not_configured.
	PricingRequirementTokenTariff = "token_tariff"
	// PricingRequirementTariffSupplement marks an operator-priced model whose
	// provider also requires explicit cache prices that the channel lacks
	// (pricing.OperatorTariffSupplementValidator).
	PricingRequirementTariffSupplement = "tariff_supplement"
	// PricingRequirementLiveTariff marks a Live-only Google model whose
	// provider requires explicit realtime prices the channel lacks, so the Live
	// session is refused before any provider work.
	PricingRequirementLiveTariff = "live_tariff"
)

// ChannelPricingGap is one supported model that request admission would
// refuse until the channel's model_configs prices it. Model is the
// client-facing supported model, UpstreamModel the model after the channel's
// model mapping (the name admission prices), Requirement one of the
// PricingRequirement* codes, and Message the refusal text admission returns.
type ChannelPricingGap struct {
	Model         string `json:"model"`
	UpstreamModel string `json:"upstream_model"`
	Requirement   string `json:"requirement"`
	Message       string `json:"message"`
}

// ResolveChannelPricingAdaptor returns the pricing adaptor request admission
// uses for a channel of channelType, including the channel-type bind that
// OpenAI-compatible channels need. Parameters: channelType is a channel type
// ID. Returns: the adaptor, or nil when channelType is outside the registry or
// maps to no adaptor.
func ResolveChannelPricingAdaptor(channelType int) relayadaptor.Adaptor {
	if channelType <= channeltype.Unknown || channelType >= channeltype.Dummy {
		return nil
	}
	return resolvePricingAdaptor(&metalib.Meta{ChannelType: channelType, APIType: channeltype.ToAPIType(channelType)})
}

// EvaluateChannelPricingReadiness reports, without a request, which supported
// models admission would refuse for missing operator pricing on a channel with
// the given configuration. It runs the admission resolvers and validators
// themselves: the strict GPT Image reservation check (at size/quality tier 1,
// group ratio 1, one image), the Live realtime price check, and the
// operator-tariff dispatch check with any provider supplement. It never
// charges quota or contacts a provider. Parameters: provider is the channel
// type's pricing adaptor (ResolveChannelPricingAdaptor) and may be nil; models
// are the supported model names; mapping is the channel's model mapping and
// configs its parsed model_configs, both possibly nil; at is the evaluation
// time for time-window pricing. Returns: the gaps in model order, possibly
// several for one model, and an empty non-nil slice when every model is ready.
func EvaluateChannelPricingReadiness(provider relayadaptor.Adaptor, models []string, mapping map[string]string, configs map[string]model.ModelConfigLocal, at time.Time) []ChannelPricingGap {
	gaps := make([]ChannelPricingGap, 0)
	for _, name := range models {
		upstream := metalib.GetMappedModelName(name, mapping)
		add := func(requirement string, err error) {
			if err != nil {
				gaps = append(gaps, ChannelPricingGap{Model: name, UpstreamModel: upstream, Requirement: requirement, Message: err.Error()})
			}
		}
		if usesStrictGPTImageTokenBilling(upstream) {
			add(PricingRequirementImageFallback, strictGPTImageReadiness(upstream, configs, provider, at))
		}
		if relayadaptor.IsLiveOnlyGoogleModel(upstream) {
			// A Live-only model is served only through a Live session, which
			// validates realtime prices; REST dispatch never reaches it.
			add(PricingRequirementLiveTariff, quota.ValidateRealtimeModelPricing(upstream, configs, provider, at))
			continue
		}
		if !relayadaptor.RequiresOperatorTariff(provider, upstream) {
			continue
		}
		if err := pricing.ValidateExplicitTokenTariffConfigs(upstream, configs, at); err != nil {
			add(PricingRequirementTokenTariff, err)
			continue
		}
		if supplement, ok := provider.(pricing.OperatorTariffSupplementValidator); ok {
			add(PricingRequirementTariffSupplement, supplement.ValidateOperatorTariffSupplement(upstream, configs, at))
		}
	}
	return gaps
}

// strictGPTImageReadiness applies image admission's strict GPT Image tariff
// check to a channel configuration: it resolves the image tariff and catalog
// defaults exactly as RelayImageHelper does, derives the default reservation
// when no per-image fallback is configured, and validates the reservation at
// tier 1, group ratio 1, and one image. Parameters: name is a strict GPT Image
// model, configs the channel's model_configs, provider the pricing adaptor,
// and at the evaluation time. Returns: nil when admission would reserve the
// request, otherwise admission's configuration error.
func strictGPTImageReadiness(name string, configs map[string]model.ModelConfigLocal, provider relayadaptor.Adaptor, at time.Time) error {
	cfg, _ := pricing.ResolveImagePricing(name, configs, provider, at)
	cfg = completeStrictGPTImageDefaults(name, cfg)
	fallbackUSD := 0.0
	if cfg != nil {
		if cfg.Megapixel != nil {
			// Admission replaces the flat fallback with the request's megapixel
			// quote, which is positive whenever the megapixel tariff is valid.
			if err := pricing.ValidateImageMegapixelPricing(cfg.Megapixel); err != nil {
				return errors.Wrap(err, "validate image megapixel tariff")
			}
			return nil
		}
		fallbackUSD = cfg.PricePerImageUsd
	}
	if fallbackUSD <= 0 && strictGPTImageDefaultReservationAllowed(name, configs, provider, at) {
		// Admission derives the default from the request-start token tariff;
		// readiness checks its most expensive case (auto size and quality).
		usage := resolveStrictGPTImageUsageConfig(name, configs, nil, provider, at)
		fallbackUSD = strictGPTImageDefaultReservationUSD(name, "auto", "auto", 0, 0, 1, usage)
	}
	return validateStrictGPTImageBilling(name, fallbackUSD, 1, 1, 1)
}
