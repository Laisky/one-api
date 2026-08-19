package controller

import (
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	relayadaptor "github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// getImageRequest parses and normalizes an image request from c. Parameters: c
// carries the reusable request body; the relay-mode argument is reserved.
// Returns: the normalized request or a wrapped parsing error.
func getImageRequest(c *gin.Context, _ int) (*relaymodel.ImageRequest, error) {
	imageRequest := &relaymodel.ImageRequest{}
	err := common.UnmarshalBodyReusable(c, imageRequest)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if imageRequest.N == 0 {
		imageRequest.N = 1
	}

	if imageRequest.Model == "" {
		imageRequest.Model = "dall-e-2"
	}

	return imageRequest, nil
}

// isOpenAIGPTImageModel reports whether model is an OpenAI GPT Image model whose
// request schema differs from the legacy DALL·E image schema. Parameters: model
// is the provider model name after any channel alias resolution. Returns: true
// when GPT Image-only validation and field filtering should be applied.
func isOpenAIGPTImageModel(model string) bool {
	return strings.HasPrefix(model, "gpt-image-") || strings.HasPrefix(model, "chatgpt-image-")
}

// imageResponseRequiresFailureReconciliation reports whether an upstream image
// response must enter failure billing. Parameters: resp is the upstream HTTP
// response and may be nil. Returns: true only for a present response outside
// the standard HTTP 2xx success range.
func imageResponseRequiresFailureReconciliation(resp *http.Response) bool {
	return resp != nil && (resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices)
}

// buildOpenAIImageRequest converts the shared image request into the OpenAI
// generation schema and removes fields owned by other providers or models.
// Parameters: req is the normalized request after model mapping and defaults.
// Returns: an OpenAI-specific request that is safe to serialize upstream.
func buildOpenAIImageRequest(req *relaymodel.ImageRequest) openai.ImageRequest {
	request := openai.ImageRequest{
		Model:   req.Model,
		Prompt:  req.Prompt,
		N:       req.N,
		Size:    req.Size,
		Quality: req.Quality,
		User:    req.User,
	}

	if isOpenAIGPTImageModel(req.Model) {
		return request
	}
	if req.ResponseFormat != nil {
		request.ResponseFormat = *req.ResponseFormat
	}
	if req.Model == "dall-e-3" {
		request.Style = req.Style
	}
	return request
}

// normalizeImageSizeKey canonicalizes an image size for pricing lookups.
// Parameters: value is a provider-facing size string. Returns: a lowercase,
// whitespace-free key using "x" as the dimension separator.
func normalizeImageSizeKey(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	trimmed = strings.ReplaceAll(trimmed, "×", "x")
	trimmed = strings.ReplaceAll(trimmed, "*", "x")
	trimmed = strings.ReplaceAll(trimmed, " ", "")
	return trimmed
}

// normalizeImageQualityKey canonicalizes an image quality for pricing lookups.
// Parameters: value is a provider-facing quality string. Returns: a trimmed,
// lowercase quality key.
func normalizeImageQualityKey(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}

// applyImageDefaults applies configured and model-specific request defaults.
// Parameters: req is mutated in place and cfg may provide channel pricing
// constraints. Returns: none.
func applyImageDefaults(req *relaymodel.ImageRequest, cfg *relayadaptor.ImagePricingConfig) {
	if cfg != nil {
		if req.Size == "" && cfg.DefaultSize != "" {
			req.Size = cfg.DefaultSize
		}
		if req.Quality == "" && cfg.DefaultQuality != "" {
			req.Quality = cfg.DefaultQuality
		}
		if cfg.MinImages > 0 && req.N < cfg.MinImages {
			req.N = cfg.MinImages
		}
		if cfg.MaxImages > 0 && cfg.MaxImages >= cfg.MinImages && req.N > cfg.MaxImages {
			req.N = cfg.MaxImages
		}
	}

	if req.Size == "" {
		switch req.Model {
		case "gpt-image-1", "gpt-image-1-mini", "chatgpt-image-latest", "gpt-image-1.5", "gpt-image-1.5-2025-12-16":
			req.Size = "1024x1536"
		case "dall-e-2", "dall-e-3", "grok-2-image", "grok-2-image-1212":
			req.Size = "1024x1024"
		default:
			req.Size = "1024x1024"
		}
	}

	if req.Quality == "" {
		switch req.Model {
		case "gpt-image-1", "gpt-image-1-mini", "chatgpt-image-latest", "gpt-image-1.5", "gpt-image-1.5-2025-12-16":
			req.Quality = "high"
		case "dall-e-2", "dall-e-3":
			req.Quality = "standard"
		default:
			req.Quality = "standard"
		}
	}
}

// isValidImageSize checks whether a requested size and quality combination is
// supported. Parameters: req contains normalized request values and cfg may
// override global rules. Returns: true when the combination is allowed.
func isValidImageSize(req *relaymodel.ImageRequest, cfg *relayadaptor.ImagePricingConfig) bool {
	sizeKey := normalizeImageSizeKey(req.Size)
	qualityKey := normalizeImageQualityKey(req.Quality)
	if qualityKey == "" {
		qualityKey = "default"
	}
	if cfg != nil {
		if len(cfg.QualitySizeMultipliers) > 0 {
			if table, ok := cfg.QualitySizeMultipliers[qualityKey]; ok {
				if _, exists := table[sizeKey]; exists {
					return true
				}
			}
			if qualityKey != "default" {
				return false
			}
			if table, ok := cfg.QualitySizeMultipliers["default"]; ok {
				if _, exists := table[sizeKey]; exists {
					return true
				}
			}
			return false
		}
		if len(cfg.SizeMultipliers) > 0 {
			_, exists := cfg.SizeMultipliers[sizeKey]
			return exists
		}
		return req.Size != ""
	}
	if req.Model == "cogview-3" || billingratio.ImageSizeRatios[req.Model] == nil {
		return true
	}
	_, ok := billingratio.ImageSizeRatios[req.Model][req.Size]
	return ok
}

// isValidImagePromptLength checks the configured or model-default prompt limit.
// Parameters: req contains the prompt and model and cfg may override the limit.
// Returns: true when the prompt is within the applicable limit.
func isValidImagePromptLength(req *relaymodel.ImageRequest, cfg *relayadaptor.ImagePricingConfig) bool {
	if cfg != nil && cfg.PromptTokenLimit > 0 {
		return len(req.Prompt) <= cfg.PromptTokenLimit
	}
	maxPromptLength, ok := billingratio.ImagePromptLengthLimitations[req.Model]
	return !ok || len(req.Prompt) <= maxPromptLength
}

// isWithinRange checks the requested image count against configured or global
// bounds. Parameters: req contains the count and model and cfg may override the
// bounds. Returns: true when the requested count is allowed.
func isWithinRange(req *relaymodel.ImageRequest, cfg *relayadaptor.ImagePricingConfig) bool {
	if cfg != nil {
		if cfg.MinImages > 0 && req.N < cfg.MinImages {
			return false
		}
		if cfg.MaxImages > 0 && req.N > cfg.MaxImages {
			return false
		}
		return true
	}
	amounts, ok := billingratio.ImageGenerationAmounts[req.Model]
	return !ok || (req.N >= amounts[0] && req.N <= amounts[1])
}

// getImageCostRatio resolves the size and quality price multiplier.
// Parameters: imageRequest contains normalized request values and cfg may
// provide channel pricing. Returns: the positive multiplier or a wrapped
// validation error.
func getImageCostRatio(imageRequest *relaymodel.ImageRequest, cfg *relayadaptor.ImagePricingConfig) (float64, error) {
	if cfg != nil {
		sizeKey := normalizeImageSizeKey(imageRequest.Size)
		qualityKey := normalizeImageQualityKey(imageRequest.Quality)
		if qualityKey == "" {
			qualityKey = "default"
		}
		if len(cfg.QualitySizeMultipliers) > 0 {
			if table, ok := cfg.QualitySizeMultipliers[qualityKey]; ok {
				if v, exists := table[sizeKey]; exists && v > 0 {
					return v, nil
				}
			}
			if qualityKey != "default" {
				return 0, errors.Errorf("quality %s not supported for model %s", imageRequest.Quality, imageRequest.Model)
			}
			if table, ok := cfg.QualitySizeMultipliers["default"]; ok {
				if v, exists := table[sizeKey]; exists && v > 0 {
					return v, nil
				}
			}
			return 0, errors.Errorf("size %s not supported for quality %s", imageRequest.Size, imageRequest.Quality)
		}
		multiplier := 1.0
		if len(cfg.SizeMultipliers) > 0 {
			if v, ok := cfg.SizeMultipliers[sizeKey]; ok && v > 0 {
				multiplier = v
			} else {
				return 0, errors.Errorf("size %s not supported for model %s", imageRequest.Size, imageRequest.Model)
			}
		}
		if len(cfg.QualityMultipliers) > 0 {
			if v, ok := cfg.QualityMultipliers[qualityKey]; ok && v > 0 {
				multiplier *= v
			} else if qualityKey != "default" {
				return 0, errors.Errorf("quality %s not supported for model %s", imageRequest.Quality, imageRequest.Model)
			}
		}
		if multiplier <= 0 {
			multiplier = 1
		}
		return multiplier, nil
	}

	imageCostRatio := getImageSizeRatioFallback(imageRequest.Model, imageRequest.Size)
	if imageRequest.Quality == "hd" && imageRequest.Model == "dall-e-3" {
		if imageRequest.Size == "1024x1024" {
			imageCostRatio *= 2
		} else {
			imageCostRatio *= 1.5
		}
	}
	if imageCostRatio <= 0 {
		imageCostRatio = 1
	}
	return imageCostRatio, nil
}

// getImageSizeRatioFallback resolves a legacy global size multiplier.
// Parameters: model and size identify the lookup entry. Returns: the configured
// multiplier, or one when no entry exists.
func getImageSizeRatioFallback(model string, size string) float64 {
	if ratio, ok := billingratio.ImageSizeRatios[model][size]; ok {
		return ratio
	}
	return 1
}

// validateImageRequest validates prompt, size, count, and quality constraints.
// Parameters: imageRequest is normalized, meta is reserved, and cfg supplies
// optional channel rules. Returns: a client-facing error or nil when valid.
func validateImageRequest(imageRequest *relaymodel.ImageRequest, _ *metalib.Meta, cfg *relayadaptor.ImagePricingConfig) *relaymodel.ErrorWithStatusCode {
	// check prompt length
	if imageRequest.Prompt == "" {
		return openai.ErrorWrapper(errors.New("prompt is required"), "prompt_missing", http.StatusBadRequest)
	}

	// model validation
	if !isValidImageSize(imageRequest, cfg) {
		return openai.ErrorWrapper(errors.New("size not supported for this image model"), "size_not_supported", http.StatusBadRequest)
	}

	if !isValidImagePromptLength(imageRequest, cfg) {
		return openai.ErrorWrapper(errors.New("prompt is too long"), "prompt_too_long", http.StatusBadRequest)
	}

	// Number of generated images validation
	if !isWithinRange(imageRequest, cfg) {
		return openai.ErrorWrapper(errors.New("invalid value of n"), "n_not_within_range", http.StatusBadRequest)
	}

	// Model-specific quality validation
	if cfg == nil && imageRequest.Model == "dall-e-3" && imageRequest.Quality != "" {
		q := strings.ToLower(imageRequest.Quality)
		if q != "standard" && q != "hd" {
			return openai.ErrorWrapper(
				errors.Errorf("Invalid value: '%s'. Supported values are: 'standard' and 'hd'.", imageRequest.Quality),
				"invalid_value",
				http.StatusBadRequest,
			)
		}
	}
	return nil
}

// getChannelImageTierOverride reads model tier overrides from channel model-configs map.
// Convention keys (in channel ModelConfigs Ratio map):
//
//	$image-tier:<model>|size=<WxH>|quality=<q>  (highest priority)
//	$image-tier:<model>|size=<WxH>
//	$image-tier:<model>|quality=<q>
func getChannelImageTierOverride(channelModelRatio map[string]float64, model, size, quality string) (float64, bool) {
	if channelModelRatio == nil {
		return 0, false
	}
	// Combined override
	key := "$image-tier:" + model + "|size=" + size + "|quality=" + quality
	if v, ok := channelModelRatio[key]; ok && v > 0 {
		return v, true
	}
	// Size-only override
	key = "$image-tier:" + model + "|size=" + size
	if v, ok := channelModelRatio[key]; ok && v > 0 {
		return v, true
	}
	// Quality-only override
	key = "$image-tier:" + model + "|quality=" + quality
	if v, ok := channelModelRatio[key]; ok && v > 0 {
		return v, true
	}
	return 0, false
}
