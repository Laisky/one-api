package controller

import (
	"math"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
)

// strictImageDefaultFallbackReason flags a strict GPT Image charge settled at
// the calculator-derived default reservation because the receipt carried no
// usable token totals and the channel configured no per-image fallback.
const strictImageDefaultFallbackReason = "gpt_image_receipt_unusable_default_reservation"

// strictGPTImagePartialImageTokens is OpenAI's documented output-token cost of
// one streamed partial image; partial_images accepts 0 to 3.
const (
	strictGPTImagePartialImageTokens = 100
	strictGPTImageMaxPartialImages   = 3
)

// strictGPTImageClientValues are the size, quality, and image count exactly as
// the client sent them, before catalog or channel defaults and clamping.
type strictGPTImageClientValues struct {
	Size    string
	Quality string
	N       int
}

// strictGPTImageRendered describes what the provider will render for a strict
// GPT Image request, the basis of the derived default reservation.
type strictGPTImageRendered struct {
	Size          string
	Quality       string
	Count         int
	PartialImages int
}

// strictGPTImageRenderedRequest resolves what the provider renders. A
// re-encoded body carries the normalized request, so its size, quality, and
// clamped count are what the provider receives. A body forwarded verbatim
// (edits, OpenAI-compatible channel types) carries the client's values
// instead: an omitted size or quality is the provider's own default, auto; and
// the client's own positive n is rendered, whether the gateway's copy raised it
// to a channel min_images or clamped it to max_images. Both bodies carry the
// client's streamed partial_images, which add output tokens. Parameters: req is
// the normalized request, client the pre-default client values, billedCount
// the normalized billed count, and reencoded whether the body was re-encoded.
// Returns: the rendered request.
func strictGPTImageRenderedRequest(req *relaymodel.ImageRequest, client strictGPTImageClientValues, billedCount int, reencoded bool) strictGPTImageRendered {
	rendered := strictGPTImageRendered{
		Size:          normalizeImageSizeKey(req.Size),
		Quality:       normalizeImageQualityKey(req.Quality),
		Count:         billedCount,
		PartialImages: requestedPartialImages(req),
	}
	if reencoded {
		return rendered
	}
	if normalizeImageSizeKey(client.Size) == "" {
		rendered.Size = "auto"
	}
	if normalizeImageQualityKey(client.Quality) == "" {
		rendered.Quality = "auto"
	}
	if client.N > 0 {
		// The provider receives the client's n, not the normalized copy. A
		// non-positive n keeps the normalized count; the provider rejects it
		// or renders its own default of at least one image.
		rendered.Count = client.N
	}
	return rendered
}

// requestedPartialImages returns the streamed partial images a GPT Image
// request asks for per image, clamped to the documented 0 to 3, since the
// provider rejects any other value. Parameters: req is the decoded request.
// Returns: the partial image count, zero when omitted.
func requestedPartialImages(req *relaymodel.ImageRequest) int {
	if req == nil || req.PartialImages == nil {
		return 0
	}
	return min(max(*req.PartialImages, 0), strictGPTImageMaxPartialImages)
}

// strictGPTImageDefaultReservationUSD derives the per-image reservation and
// fallback price a strict GPT Image request uses when the channel configures no
// image.price_per_image_usd. It prices OpenAI's calculator estimate of the
// request's image-output tokens (an "auto" or unpriced size/quality resolves to
// its most expensive case) plus any streamed partial images at the effective
// output-token rate, and the prompt's UTF-8 byte count (an upper bound on its
// text tokens) at the effective text-input rate, spread across the billed
// images. It does not price reference-image input, which no published rule
// bounds, so RelayImageHelper never uses it for an edit (see
// strictGPTImageEditFallbackError). Parameters: name is the mapped model, size
// and quality the normalized rendered values, promptBytes the prompt length in
// bytes, partialImages the streamed partial images per image, count the billed
// image count, and cfg the request-start pricing from
// resolveStrictGPTImageUsageConfig. Returns: the per-image USD price, or zero
// when the model has no calculator coverage or no positive derived price (for
// example an operator-priced host whose channel lacks token prices).
func strictGPTImageDefaultReservationUSD(name, size, quality string, promptBytes, partialImages, count int, cfg adaptor.ModelConfig) float64 {
	if !usesStrictGPTImageTokenBilling(name) || count <= 0 || promptBytes < 0 || partialImages < 0 {
		return 0
	}
	tokens, ok := openai.EstimateGPTImageOutputTokens(name, size, quality)
	if !ok || tokens <= 0 {
		return 0
	}
	tokens += partialImages * strictGPTImagePartialImageTokens
	// Price at the tier the estimated request reaches, exactly as settlement
	// selects it from a receipt.
	estimate := &relaymodel.Usage{PromptTokens: promptBytes, CompletionTokens: tokens * count}
	prices := strictGPTImageConfiguredPrices(estimate, cfg)
	if prices.outputImageUSD <= 0 {
		return 0
	}
	perImage := float64(tokens)*prices.outputImageUSD/1e6 +
		float64(promptBytes)*max(prices.inputTextUSD, 0)/1e6/float64(count)
	if math.IsNaN(perImage) || math.IsInf(perImage, 0) || perImage <= 0 {
		return 0
	}
	return perImage
}

// strictGPTImageEditFallbackError explains why a strict GPT Image edit without
// an explicit per-image reservation/fallback is refused before reservation.
// OpenAI bills every reference image (and mask) of an edit as image input
// tokens and publishes no formula or per-request cap that bounds them (the
// vision patch rules explicitly exclude GPT Image), so the calculator-derived
// default cannot cover an edit whose receipt turns out unusable. Returns: the
// client-facing configuration error.
func strictGPTImageEditFallbackError() error {
	return errors.New("GPT Image 2.5/2.6 image edits require a positive image.price_per_image_usd reservation/fallback in channel model_configs: " +
		"OpenAI bills reference images and masks as image input tokens and publishes no rule that bounds them, " +
		"so the default derived from OpenAI's token calculator cannot cover an edit")
}

// strictGPTImageDefaultReservationAllowed reports whether a channel may use the
// derived default reservation. A host that declares the model operator-priced
// (adaptor.OperatorTariffPolicy) has no qualified token tariff of its own, so
// its default may only be derived from a complete explicit channel tariff, the
// same one dispatch requires; otherwise the resolvers' generic fallback ratio
// would price the hold. Parameters: name is the mapped model, configs the
// channel's model_configs, provider the pricing adaptor, and at the request
// start. Returns: whether strictGPTImageDefaultReservationUSD may be used.
func strictGPTImageDefaultReservationAllowed(name string, configs map[string]model.ModelConfigLocal, provider adaptor.Adaptor, at time.Time) bool {
	if !adaptor.RequiresOperatorTariff(provider, name) {
		return true
	}
	return pricing.ValidateExplicitTokenTariffConfigs(name, configs, at) == nil
}

// labelStrictImageFallback renames the configured-fallback estimate reason
// when the fallback is the derived default rather than an operator price.
// Parameters: reason is the settlement's estimate reason and derived reports
// whether the reservation came from strictGPTImageDefaultReservationUSD.
// Returns: the reason to record.
func labelStrictImageFallback(reason string, derived bool) string {
	if derived && reason == strictImageConfiguredFallbackReason {
		return strictImageDefaultFallbackReason
	}
	return reason
}
