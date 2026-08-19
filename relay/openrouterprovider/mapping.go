// Package openrouterprovider mapping helpers translate one-api's adaptor.ModelConfig
// into OpenRouter's provider-listing Model schema. The functions in this file are
// pure (no I/O, no globals) so they can be unit-tested cheaply and re-used from
// controllers, tests, and tooling alike.
package openrouterprovider

import (
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/one-api/relay/adaptor"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
)

// defaultContextLength is the fallback total token context returned to OpenRouter
// when an adaptor does not advertise an explicit ContextLength.
const defaultContextLength int32 = 8192

// defaultMaxOutputTokens is the fallback per-response output cap returned to
// OpenRouter when an adaptor does not advertise an explicit MaxOutputTokens.
const defaultMaxOutputTokens int32 = 4096

// defaultQuantization is the fallback numeric precision label used when an
// adaptor does not advertise a specific quantization. fp16 is chosen because it
// is the most common precision for hosted production models.
const defaultQuantization = "fp16"

// defaultInputModality is the single-element fallback input modality list.
var defaultInputModality = []string{"text"}

// defaultOutputModality is the single-element fallback output modality list.
var defaultOutputModality = []string{"text"}

// defaultSupportedFeatures advertises capabilities every chat-completions
// adaptor in this project supports out of the box: tool calling.
var defaultSupportedFeatures = []string{"tools"}

// defaultSupportedSamplingParameters mirrors the sampling parameters one-api
// already accepts on POST /v1/chat/completions for any OpenAI-compatible model.
var defaultSupportedSamplingParameters = []string{
	"temperature",
	"top_p",
	"frequency_penalty",
	"presence_penalty",
	"stop",
	"seed",
	"max_tokens",
}

// ModelInput aggregates the inputs required to build a single OpenRouter Model
// entry. It is used by the bulk builder so callers can pass a flat list of
// (model, config, owner) tuples.
type ModelInput struct {
	// Name is the canonical model identifier (e.g., "gpt-4o-mini").
	Name string
	// Config is the pricing/metadata snapshot from the adaptor's
	// GetDefaultModelPricing(). The zero value is allowed and triggers defaults.
	Config adaptor.ModelConfig
	// Owner is the human-friendly channel/organization label (e.g., "openai").
	// Empty owner is allowed; the resulting Name will fall back to the bare model id.
	Owner string
	// Created overrides the Created timestamp for deterministic output. When zero
	// the helper substitutes time.Now().Unix() at call time.
	Created int64
}

// MapModel converts one (modelName, ModelConfig, owner, created) tuple into an
// OpenRouter Model entry. All defaults documented at the package level are
// applied transparently. The created argument may be zero to use the current
// Unix timestamp.
func MapModel(modelName string, cfg adaptor.ModelConfig, owner string, created int64) Model {
	trimmedName := strings.TrimSpace(modelName)
	trimmedOwner := strings.TrimSpace(owner)

	displayName := trimmedName
	if trimmedOwner != "" {
		displayName = trimmedOwner + ": " + trimmedName
	}

	if created == 0 {
		created = time.Now().Unix()
	}

	contextLength := cfg.ContextLength
	if contextLength <= 0 {
		contextLength = defaultContextLength
	}

	maxOutput := cfg.MaxOutputTokens
	if maxOutput <= 0 {
		// Prefer ContextLength when explicitly provided but smaller than the
		// 4096 fallback; otherwise use 4096.
		if cfg.ContextLength > 0 && cfg.ContextLength < defaultMaxOutputTokens {
			maxOutput = cfg.ContextLength
		} else {
			maxOutput = defaultMaxOutputTokens
		}
	}

	quantization := cfg.Quantization
	if strings.TrimSpace(quantization) == "" {
		quantization = defaultQuantization
	}

	inputModalities := chooseModalities(cfg.InputModalities, deriveInputModalities(cfg))
	outputModalities := chooseModalities(cfg.OutputModalities, defaultOutputModality)

	supportedFeatures := cfg.SupportedFeatures
	if len(supportedFeatures) == 0 {
		supportedFeatures = append([]string(nil), defaultSupportedFeatures...)
	} else {
		supportedFeatures = append([]string(nil), supportedFeatures...)
	}

	supportedSamplingParameters := cfg.SupportedSamplingParameters
	if len(supportedSamplingParameters) == 0 {
		supportedSamplingParameters = append([]string(nil), defaultSupportedSamplingParameters...)
	} else {
		supportedSamplingParameters = append([]string(nil), supportedSamplingParameters...)
	}

	return Model{
		ID:                          trimmedName,
		HuggingFaceID:               cfg.HuggingFaceID,
		Name:                        displayName,
		Created:                     created,
		Description:                 cfg.Description,
		InputModalities:             inputModalities,
		OutputModalities:            outputModalities,
		Quantization:                quantization,
		ContextLength:               contextLength,
		MaxOutputLength:             maxOutput,
		Pricing:                     buildPricing(cfg),
		SupportedSamplingParameters: supportedSamplingParameters,
		SupportedFeatures:           supportedFeatures,
	}
}

// BuildModelListResponse converts a slice of ModelInput tuples into the
// OpenRouter ModelListResponse envelope. Inputs with empty model names are
// skipped so callers can pass best-effort lists without pre-filtering.
func BuildModelListResponse(inputs []ModelInput) ModelListResponse {
	now := time.Now().Unix()
	models := make([]Model, 0, len(inputs))
	for _, in := range inputs {
		name := strings.TrimSpace(in.Name)
		if name == "" {
			continue
		}
		created := in.Created
		if created == 0 {
			created = now
		}
		models = append(models, MapModel(name, in.Config, in.Owner, created))
	}
	return ModelListResponse{Data: models}
}

// chooseModalities returns explicit when non-empty, otherwise a defensive copy
// of fallback so callers cannot mutate package-level defaults.
func chooseModalities(explicit, fallback []string) []string {
	if len(explicit) > 0 {
		out := make([]string, len(explicit))
		copy(out, explicit)
		return out
	}
	out := make([]string, len(fallback))
	copy(out, fallback)
	return out
}

// deriveInputModalities inspects the ModelConfig's modality-specific sub-configs
// to derive a sensible default input modality set when InputModalities is unset.
// Only modalities supported by OpenRouter ("text", "image", "file") are emitted;
// audio and video sub-configs imply specialized endpoints, not chat input.
func deriveInputModalities(cfg adaptor.ModelConfig) []string {
	out := []string{"text"}
	if cfg.Image != nil {
		out = append(out, "image")
	}
	return out
}

// buildPricing computes the OpenRouter Pricing struct from the adaptor pricing
// metadata. Conversion follows: usdPerToken = Ratio / MilliTokensUsd / 1e6.
// CompletionRatio defaults to 1.0 when zero (matching adaptor convention).
func buildPricing(cfg adaptor.ModelConfig) Pricing {
	pricing := Pricing{
		Prompt:     formatUsdPerToken(cfg.Ratio),
		Completion: formatUsdPerToken(completionRatePerToken(cfg)),
	}
	if cfg.CachedInputRatio > 0 {
		pricing.InputCacheRead = formatUsdPerToken(cfg.CachedInputRatio)
	}
	if cfg.Image != nil && cfg.Image.PricePerImageUsd > 0 {
		pricing.Image = strconv.FormatFloat(cfg.Image.PricePerImageUsd, 'f', -1, 64)
	}
	return pricing
}

// completionRatePerToken returns the effective per-input-token Ratio used when
// computing the completion (output) USD price. CompletionRatio defaults to 1.0
// to match the adaptor billing convention.
func completionRatePerToken(cfg adaptor.ModelConfig) float64 {
	completionRatio := cfg.CompletionRatio
	if completionRatio == 0 {
		completionRatio = 1.0
	}
	return cfg.Ratio * completionRatio
}

// formatUsdPerToken converts a billing ratio (quota per milli-token) into the
// OpenRouter USD-per-token string representation. A zero/negative ratio returns
// "0" so the response always carries a parseable numeric string.
func formatUsdPerToken(ratio float64) string {
	if ratio <= 0 {
		return "0"
	}
	usd := ratio / billingratio.MilliTokensUsd / 1e6
	return strconv.FormatFloat(usd, 'f', -1, 64)
}
