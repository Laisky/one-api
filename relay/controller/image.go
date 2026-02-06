package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/tracing"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay"
	relayadaptor "github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/adaptor/replicate"
	billingratio "github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/channeltype"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

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

	if strings.HasPrefix(imageRequest.Model, "gpt-image-") {
		imageRequest.ResponseFormat = nil
	}

	return imageRequest, nil
}

func normalizeImageSizeKey(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	trimmed = strings.ReplaceAll(trimmed, "×", "x")
	trimmed = strings.ReplaceAll(trimmed, "*", "x")
	trimmed = strings.ReplaceAll(trimmed, " ", "")
	return trimmed
}

func normalizeImageQualityKey(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}

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

func isValidImagePromptLength(req *relaymodel.ImageRequest, cfg *relayadaptor.ImagePricingConfig) bool {
	if cfg != nil && cfg.PromptTokenLimit > 0 {
		return len(req.Prompt) <= cfg.PromptTokenLimit
	}
	maxPromptLength, ok := billingratio.ImagePromptLengthLimitations[req.Model]
	return !ok || len(req.Prompt) <= maxPromptLength
}

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

func getImageSizeRatioFallback(model string, size string) float64 {
	if ratio, ok := billingratio.ImageSizeRatios[model][size]; ok {
		return ratio
	}
	return 1
}

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

func RelayImageHelper(c *gin.Context, relayMode int) *relaymodel.ErrorWithStatusCode {
	lg := gmw.GetLogger(c)
	ctx := gmw.Ctx(c)
	meta := metalib.GetByContext(c)
	imageRequest, err := getImageRequest(c, meta.Mode)
	if err != nil {
		// Let ErrorWrapper handle the logging to avoid duplicate logging
		return openai.ErrorWrapper(err, "invalid_image_request", http.StatusBadRequest)
	}

	// map model name
	var isModelMapped bool
	meta.OriginModelName = imageRequest.Model
	imageRequest.Model = meta.ActualModelName
	isModelMapped = meta.OriginModelName != meta.ActualModelName
	meta.ActualModelName = imageRequest.Model
	metalib.Set2Context(c, meta)

	var channelModelRatio map[string]float64
	var channelModelConfigs map[string]model.ModelConfigLocal
	if channelModel, ok := c.Get(ctxkey.ChannelModel); ok {
		if channel, ok := channelModel.(*model.Channel); ok {
			channelModelRatio = channel.GetModelRatioFromConfigs()
			channelModelConfigs = channel.GetModelPriceConfigs()
		}
	}

	adaptor := relay.GetAdaptor(meta.APIType)
	if adaptor == nil {
		return openai.ErrorWrapper(errors.Errorf("invalid api type: %d", meta.APIType), "invalid_api_type", http.StatusBadRequest)
	}

	resolvedConfig, _ := pricing.ResolveModelConfig(imageRequest.Model, channelModelConfigs, adaptor)
	imagePricingCfg := resolvedConfig.Image
	applyImageDefaults(imageRequest, imagePricingCfg)

	bizErr := validateImageRequest(imageRequest, meta, imagePricingCfg)
	if bizErr != nil {
		return bizErr
	}

	imageCostRatio, err := getImageCostRatio(imageRequest, imagePricingCfg)
	if err != nil {
		return openai.ErrorWrapper(err, "get_image_cost_ratio_failed", http.StatusInternalServerError)
	}

	imageModel := imageRequest.Model
	// Convert the original image model
	imageRequest.Model = metalib.GetMappedModelName(imageRequest.Model, billingratio.ImageOriginModelName)
	c.Set(ctxkey.ResponseFormat, imageRequest.ResponseFormat)

	var requestBody io.Reader
	if strings.ToLower(c.GetString(ctxkey.ContentType)) == "application/json" &&
		isModelMapped || meta.ChannelType == channeltype.Azure { // make Azure channel request body
		jsonStr, err := json.Marshal(imageRequest)
		if err != nil {
			return openai.ErrorWrapper(err, "marshal_image_request_failed", http.StatusInternalServerError)
		}
		requestBody = bytes.NewBuffer(jsonStr)
	} else {
		requestBody = c.Request.Body
	}

	adaptor.Init(meta)

	// these adaptors need to convert the request
	switch meta.ChannelType {
	case channeltype.Zhipu,
		channeltype.Ali,
		channeltype.VertextAI,
		channeltype.Baidu,
		channeltype.XAI:
		finalRequest, err := adaptor.ConvertImageRequest(c, imageRequest)
		if err != nil {
			// Check if this is a validation error and preserve the correct HTTP status code for AWS Bedrock
			if strings.Contains(err.Error(), "does not support image generation") {
				return openai.ErrorWrapper(err, "invalid_request_error", http.StatusBadRequest)
			}

			return openai.ErrorWrapper(err, "convert_image_request_failed", http.StatusInternalServerError)
		}

		jsonStr, err := json.Marshal(finalRequest)
		if err != nil {
			return openai.ErrorWrapper(err, "marshal_image_request_failed", http.StatusInternalServerError)
		}
		requestBody = bytes.NewBuffer(jsonStr)
	case channeltype.Replicate:
		finalRequest, err := replicate.ConvertImageRequest(c, imageRequest)
		if err != nil {
			return openai.ErrorWrapper(err, "convert_image_request_failed", http.StatusInternalServerError)
		}
		jsonStr, err := json.Marshal(finalRequest)
		if err != nil {
			return openai.ErrorWrapper(err, "marshal_image_request_failed", http.StatusInternalServerError)
		}
		requestBody = bytes.NewBuffer(jsonStr)
	case channeltype.OpenAI:
		if meta.Mode != relaymode.ImagesEdits {
			jsonStr, err := json.Marshal(imageRequest)
			if err != nil {
				return openai.ErrorWrapper(err, "marshal_image_request_failed", http.StatusInternalServerError)
			}

			requestBody = bytes.NewBuffer(jsonStr)
		}
	}

	// Resolve model ratio using unified three-layer pricing (channel overrides → adapter defaults → global fallback)
	// IMPORTANT: Use APIType here (adaptor family), not ChannelType. ChannelType IDs do not map to adaptor switch.
	pricingAdaptor := adaptor
	modelRatio := pricing.GetModelRatioWithThreeLayers(imageModel, channelModelRatio, pricingAdaptor)
	// groupRatio := billingratio.GetGroupRatio(meta.Group)
	groupRatio := c.GetFloat64(ctxkey.ChannelRatio)

	// Channel override for size/quality tier multiplier (optional)
	if override, ok := getChannelImageTierOverride(channelModelRatio, imageModel, imageRequest.Size, imageRequest.Quality); ok {
		imageCostRatio = override
	}

	// Determine if this model is billed per image (Image.PricePerImageUsd) or per token (Ratio)
	imagePriceUsd := 0.0
	if resolvedConfig.Image != nil {
		imagePriceUsd = resolvedConfig.Image.PricePerImageUsd
	}
	if imagePriceUsd == 0 {
		if pm, ok := pricing.GetGlobalModelConfig(imageModel); ok && pm.Image != nil {
			imagePriceUsd = pm.Image.PricePerImageUsd
		}
	}

	ratio := modelRatio * groupRatio
	requestedCount := imageRequest.N
	if requestedCount <= 0 {
		requestedCount = 1
	}
	billedCount := requestedCount
	if meta.ChannelType == channeltype.Replicate && billedCount > 1 {
		billedCount = 1
	}
	perImageBilling := imagePriceUsd > 0
	baseQuota := calculateImageBaseQuota(imagePriceUsd, ratio, imageCostRatio, groupRatio, billedCount)
	usedQuota := baseQuota
	tokenQuota := int64(0)
	tokenQuotaFloat := 0.0

	userQuota, err := model.CacheGetUserQuota(ctx, meta.UserId)
	if err != nil {
		return openai.ErrorWrapper(err, "get_user_quota_failed", http.StatusInternalServerError)
	}

	var preConsumedQuota int64
	if userQuota < usedQuota {
		return openai.ErrorWrapper(errors.New("user quota is not enough"), "insufficient_user_quota", http.StatusForbidden)
	}

	// If using per-image billing, pre-consume the estimated quota now
	if perImageBilling && usedQuota > 0 {
		preConsumedQuota = usedQuota
		if err := model.PreConsumeTokenQuota(ctx, meta.TokenId, preConsumedQuota); err != nil {
			return openai.ErrorWrapper(err, "pre_consume_failed", http.StatusInternalServerError)
		}
		// Record provisional request cost so user-cancel before upstream usage still gets tracked
		quotaId := c.GetInt(ctxkey.Id)
		requestId := c.GetString(ctxkey.RequestId)
		if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, preConsumedQuota); err != nil {
			lg.Warn("record provisional user request cost failed", zap.Error(err))
		}
	}

	// do request
	resp, err := adaptor.DoRequest(c, meta, requestBody)
	if err != nil {
		// ErrorWrapper will log the error, so we don't need to log it here
		// Refund any pre-consumed quota if request failed
		if preConsumedQuota > 0 {
			_ = model.PostConsumeTokenQuota(ctx, meta.TokenId, -preConsumedQuota)
		}
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}

	var promptTokens, completionTokens int
	// Capture IDs from gin context before switching to a background context in defer
	requestId := c.GetString(ctxkey.RequestId)
	traceId := tracing.GetTraceID(c)
	defer func() {
		bgCtx, cancel := context.WithTimeout(gmw.BackgroundCtx(c), time.Minute)
		defer cancel()

		if resp != nil &&
			resp.StatusCode != http.StatusCreated && // replicate returns 201
			resp.StatusCode != http.StatusOK {
			// Refund pre-consumed quota when upstream not successful
			if preConsumedQuota > 0 {
				_ = model.PostConsumeTokenQuota(bgCtx, meta.TokenId, -preConsumedQuota)
			}
			// Reconcile provisional record to 0
			if err := model.UpdateUserRequestCostQuotaByRequestID(
				c.GetInt(ctxkey.Id),
				c.GetString(ctxkey.RequestId),
				0,
			); err != nil {
				lg.Warn("update user request cost to zero failed", zap.Error(err))
			}
			return
		}

		// Apply delta if we pre-consumed; otherwise apply full usage
		quotaDelta := usedQuota
		if preConsumedQuota > 0 {
			quotaDelta = usedQuota - preConsumedQuota
		}
		if quotaDelta < 0 {
			quotaDelta = 0
		}
		err := model.PostConsumeTokenQuota(bgCtx, meta.TokenId, quotaDelta)
		if err != nil {
			lg.Error("error consuming token remain quota", zap.Error(err))
		}
		err = model.CacheUpdateUserQuota(bgCtx, meta.UserId)
		if err != nil {
			lg.Error("error update user quota cache", zap.Error(err))
		}
		if usedQuota >= 0 {
			tokenName := c.GetString(ctxkey.TokenName)
			logContent := formatImageBillingLog(imageBillingLogParams{
				OriginModel:     meta.OriginModelName,
				Model:           imageModel,
				Size:            imageRequest.Size,
				Quality:         imageRequest.Quality,
				RequestCount:    requestedCount,
				BilledCount:     billedCount,
				ImagePriceUsd:   imagePriceUsd,
				ImageTier:       imageCostRatio,
				BaseQuota:       baseQuota,
				TokenQuota:      tokenQuota,
				TokenQuotaFloat: tokenQuotaFloat,
				TotalQuota:      usedQuota,
				GroupRatio:      groupRatio,
				ModelRatio:      modelRatio,
			})
			// Record log with RequestId/TraceId set directly on the log
			model.RecordConsumeLog(bgCtx, &model.Log{
				UserId:           meta.UserId,
				ChannelId:        meta.ChannelId,
				PromptTokens:     promptTokens,
				CompletionTokens: completionTokens,
				ModelName:        imageRequest.Model,
				TokenName:        tokenName,
				Quota:            int(usedQuota),
				Content:          logContent,
				ElapsedTime:      helper.CalcElapsedTime(meta.StartTime),
				RequestId:        requestId,
				TraceId:          traceId,
			})
			model.UpdateUserUsedQuotaAndRequestCount(meta.UserId, usedQuota)
			channelId := c.GetInt(ctxkey.ChannelId)
			model.UpdateChannelUsedQuota(channelId, usedQuota)

			// Reconcile request cost with final usedQuota (override provisional value if any)
			if err := model.UpdateUserRequestCostQuotaByRequestID(
				c.GetInt(ctxkey.Id),
				c.GetString(ctxkey.RequestId),
				usedQuota,
			); err != nil {
				lg.Error("update user request cost failed", zap.Error(err))
			}
		}
	}()

	// do response
	usage, respErr := adaptor.DoResponse(c, resp, meta)
	if respErr != nil {
		// If upstream already responded and usage is available but the client canceled (write failed),
		// compute usedQuota here so the logging goroutine can record requestId and cost.
		if usage != nil {
			promptTokens = usage.PromptTokens
			completionTokens = usage.CompletionTokens
			summary := finalizeImageQuota(baseQuota, perImageBilling, imageModel, meta.ActualModelName, usage, groupRatio)
			tokenQuota = summary.TokenQuota
			tokenQuotaFloat = summary.TokenQuotaFloat
			usedQuota = summary.TotalQuota
		}
		return respErr
	}

	if usage != nil {
		promptTokens = usage.PromptTokens
		completionTokens = usage.CompletionTokens

		// Universal reconciliation: if we have reliable usage, compute token quota and add it to per-image base.
		summary := finalizeImageQuota(baseQuota, perImageBilling, imageModel, meta.ActualModelName, usage, groupRatio)
		tokenQuota = summary.TokenQuota
		tokenQuotaFloat = summary.TokenQuotaFloat
		usedQuota = summary.TotalQuota
	}

	return nil
}

// gptImageTokenBucketPricing stores USD per 1M token prices for GPT Image models.
type gptImageTokenBucketPricing struct {
	inputTextUSD        float64
	cachedInputTextUSD  float64
	inputImageUSD       float64
	cachedInputImageUSD float64
	outputImageUSD      float64
}

var gptImageTokenBucketPrices = map[string]gptImageTokenBucketPricing{
	// https://platform.openai.com/docs/models/gpt-image-1
	"gpt-image-1": {
		inputTextUSD:        5.0,
		cachedInputTextUSD:  1.25,
		inputImageUSD:       10.0,
		cachedInputImageUSD: 2.5,
		outputImageUSD:      40.0,
	},
	// https://platform.openai.com/docs/models/gpt-image-1-mini
	"gpt-image-1-mini": {
		inputTextUSD:        2.0,
		cachedInputTextUSD:  0.20,
		inputImageUSD:       2.5,
		cachedInputImageUSD: 0.25,
		outputImageUSD:      8.0,
	},
	// https://platform.openai.com/docs/models/chatgpt-image-latest
	"chatgpt-image-latest": {
		inputTextUSD:        5.0,
		cachedInputTextUSD:  1.25,
		inputImageUSD:       8.0,
		cachedInputImageUSD: 2.0,
		outputImageUSD:      32.0,
	},
	// https://platform.openai.com/docs/models/gpt-image-1.5
	"gpt-image-1.5": {
		inputTextUSD:        5.0,
		cachedInputTextUSD:  1.25,
		inputImageUSD:       8.0,
		cachedInputImageUSD: 2.0,
		outputImageUSD:      32.0,
	},
	"gpt-image-1.5-2025-12-16": {
		inputTextUSD:        5.0,
		cachedInputTextUSD:  1.25,
		inputImageUSD:       8.0,
		cachedInputImageUSD: 2.0,
		outputImageUSD:      32.0,
	},
}

// computeGptImageTokenQuota calculates quota for gpt-image-1 family models using five billing buckets:
// input text, cached input text, input image, cached input image, and output image tokens.
// Prices are expressed in USD per 1M tokens and multiplied by the groupRatio (quota multiplier) before returning quota units.
func computeGptImageTokenQuota(modelName string, usage *relaymodel.Usage, groupRatio float64) float64 {
	if usage == nil {
		return 0
	}
	pricing, ok := gptImageTokenBucketPrices[modelName]
	if !ok {
		return 0
	}

	var textIn, imageIn, cachedIn int
	if usage.PromptTokensDetails != nil {
		textIn = usage.PromptTokensDetails.TextTokens
		imageIn = usage.PromptTokensDetails.ImageTokens
		cachedIn = usage.PromptTokensDetails.CachedTokens
	}
	if textIn < 0 {
		textIn = 0
	}
	if imageIn < 0 {
		imageIn = 0
	}
	if cachedIn < 0 {
		cachedIn = 0
	}
	totalIn := textIn + imageIn
	if cachedIn > totalIn {
		cachedIn = totalIn
	}
	cachedText := 0
	cachedImage := 0
	if cachedIn > 0 && totalIn > 0 {
		cachedText = min(max(int(math.Round(float64(cachedIn)*(float64(textIn)/float64(totalIn)))), 0), cachedIn)
		cachedImage = cachedIn - cachedText
	}
	normalText := max(textIn-cachedText, 0)
	normalImage := max(imageIn-cachedImage, 0)
	outTokens := max(usage.CompletionTokens, 0)

	quota := 0.0
	quota += float64(normalText) * pricing.inputTextUSD * billingratio.MilliTokensUsd
	quota += float64(cachedText) * pricing.cachedInputTextUSD * billingratio.MilliTokensUsd
	quota += float64(normalImage) * pricing.inputImageUSD * billingratio.MilliTokensUsd
	quota += float64(cachedImage) * pricing.cachedInputImageUSD * billingratio.MilliTokensUsd
	quota += float64(outTokens) * pricing.outputImageUSD * billingratio.MilliTokensUsd

	if groupRatio > 0 {
		quota *= groupRatio
	}
	return quota
}

// computeImageUsageQuota routes to the correct usage-based cost function per model.
// Returns 0 when usage is missing or the model has no token pricing rule.
func computeImageUsageQuota(modelName string, usage *relaymodel.Usage, groupRatio float64) float64 {
	if usage == nil {
		return 0
	}
	// Basic reliability check: some providers may omit usage entirely
	if usage.PromptTokens == 0 && usage.CompletionTokens == 0 && (usage.PromptTokensDetails == nil) {
		return 0
	}
	switch modelName {
	case "gpt-image-1", "gpt-image-1-mini", "chatgpt-image-latest", "gpt-image-1.5", "gpt-image-1.5-2025-12-16":
		return computeGptImageTokenQuota(modelName, usage, groupRatio)
	default:
		// Add more models here as they publish token pricing for image buckets
		return 0
	}
}

// imageQuotaSummary tracks the breakdown of image billing across fixed per-image components and token-based usage.
type imageQuotaSummary struct {
	BaseQuota       int64
	TokenQuota      int64
	TokenQuotaFloat float64
	TotalQuota      int64
}

// calculateImageBaseQuota derives the upfront quota reservation for an image request.
// When per-image billing is enabled, the quota scales with the billed image count and tier multiplier.
// For token-only models, the base quota falls back to the model ratio estimation.
func calculateImageBaseQuota(imagePriceUsd, ratio, imageCostRatio, groupRatio float64, count int) int64 {
	if count <= 0 {
		return 0
	}
	if imagePriceUsd > 0 {
		perImageQuota := math.Ceil(imagePriceUsd * billingratio.QuotaPerUsd * imageCostRatio * groupRatio)
		if perImageQuota <= 0 {
			return 0
		}
		return int64(perImageQuota) * int64(count)
	}
	if ratio <= 0 {
		return 0
	}
	perImageQuota := math.Ceil(ratio * imageCostRatio)
	if perImageQuota <= 0 {
		return 0
	}
	return int64(perImageQuota) * int64(count)
}

// finalizeImageQuota merges token usage data with the reserved base quota to produce the final billed amount.
// Token usage augments per-image pricing, ensuring prompt and output buckets are not skipped.
func finalizeImageQuota(baseQuota int64, perImageBilling bool, imageModel string, actualModel string, usage *relaymodel.Usage, groupRatio float64) imageQuotaSummary {
	summary := imageQuotaSummary{
		BaseQuota:  baseQuota,
		TotalQuota: baseQuota,
	}
	if usage == nil {
		return summary
	}

	tokenQuotaFloat := computeImageUsageQuota(imageModel, usage, groupRatio)
	if tokenQuotaFloat < 0 {
		tokenQuotaFloat = 0
	}
	tokenQuota := int64(math.Ceil(tokenQuotaFloat))
	if tokenQuota < 0 {
		tokenQuota = 0
	}
	summary.TokenQuotaFloat = tokenQuotaFloat
	summary.TokenQuota = tokenQuota

	if perImageBilling {
		if tokenQuota > 0 {
			summary.TotalQuota += tokenQuota
		}
		return summary
	}

	if tokenQuota > 0 {
		summary.TotalQuota = tokenQuota
		return summary
	}

	fallbackFloat := computeLegacyImageTokenQuota(actualModel, usage, groupRatio)
	if fallbackFloat > 0 {
		fallbackQuota := int64(math.Ceil(fallbackFloat))
		if fallbackQuota < 0 {
			fallbackQuota = 0
		}
		summary.TokenQuotaFloat = fallbackFloat
		summary.TokenQuota = fallbackQuota
		summary.TotalQuota = baseQuota + fallbackQuota
	}

	return summary
}

// computeLegacyImageTokenQuota handles legacy token billing paths for image models lacking detailed bucket pricing.
func computeLegacyImageTokenQuota(modelName string, usage *relaymodel.Usage, groupRatio float64) float64 {
	if usage == nil || usage.PromptTokensDetails == nil {
		return 0
	}
	switch modelName {
	case "gpt-image-1", "gpt-image-1-mini":
		textTokens := usage.PromptTokensDetails.TextTokens
		if textTokens < 0 {
			textTokens = 0
		}
		imageTokens := usage.PromptTokensDetails.ImageTokens
		if imageTokens < 0 {
			imageTokens = 0
		}
		quota := float64(textTokens)*5*billingratio.MilliTokensUsd + float64(imageTokens)*10*billingratio.MilliTokensUsd
		if groupRatio > 0 {
			quota *= groupRatio
		}
		return quota
	case "chatgpt-image-latest", "gpt-image-1.5", "gpt-image-1.5-2025-12-16":
		textTokens := usage.PromptTokensDetails.TextTokens
		if textTokens < 0 {
			textTokens = 0
		}
		imageTokens := usage.PromptTokensDetails.ImageTokens
		if imageTokens < 0 {
			imageTokens = 0
		}
		quota := float64(textTokens)*5*billingratio.MilliTokensUsd + float64(imageTokens)*8*billingratio.MilliTokensUsd
		if groupRatio > 0 {
			quota *= groupRatio
		}
		return quota
	default:
		return 0
	}
}

// imageBillingLogParams captures the attributes required to build a user-facing billing log entry for image requests.
type imageBillingLogParams struct {
	OriginModel     string
	Model           string
	Size            string
	Quality         string
	RequestCount    int
	BilledCount     int
	ImagePriceUsd   float64
	ImageTier       float64
	BaseQuota       int64
	TokenQuota      int64
	TokenQuotaFloat float64
	TotalQuota      int64
	GroupRatio      float64
	ModelRatio      float64
}

// formatImageBillingLog renders a concise billing summary including size, quality, pricing tiers, and token costs.
func formatImageBillingLog(params imageBillingLogParams) string {
	var builder strings.Builder
	builder.Grow(256)
	builder.WriteString("image")

	modelName := params.Model
	if modelName == "" {
		modelName = "unknown"
	}
	builder.WriteString(" model=")
	builder.WriteString(modelName)
	if params.OriginModel != "" && params.OriginModel != modelName {
		builder.WriteString(" origin_model=")
		builder.WriteString(params.OriginModel)
	}
	if params.Size != "" {
		builder.WriteString(" size=")
		builder.WriteString(params.Size)
	}
	if params.Quality != "" {
		builder.WriteString(" quality=")
		builder.WriteString(params.Quality)
	}
	fmt.Fprintf(&builder, " requested_n=%d billed_n=%d", params.RequestCount, params.BilledCount)

	totalUsd := float64(params.TotalQuota) / billingratio.QuotaPerUsd
	fmt.Fprintf(&builder, " total_usd=%.4f", totalUsd)
	fmt.Fprintf(&builder, " group_rate=%.2f", params.GroupRatio)

	if params.ImagePriceUsd > 0 {
		unitUsd := params.ImagePriceUsd * params.ImageTier
		baseUsd := float64(params.BaseQuota) / billingratio.QuotaPerUsd
		fmt.Fprintf(&builder, " unit_usd=%.4f tier=%.2f base_usd=%.4f", unitUsd, params.ImageTier, baseUsd)
	} else if params.ModelRatio > 0 {
		fmt.Fprintf(&builder, " model_ratio=%.4f", params.ModelRatio)
	}

	if params.TokenQuota > 0 {
		tokenUsd := float64(params.TokenQuota) / billingratio.QuotaPerUsd
		fmt.Fprintf(&builder, " token_usd=%.4f", tokenUsd)
	} else if params.TokenQuotaFloat > 0 {
		tokenUsd := params.TokenQuotaFloat / billingratio.QuotaPerUsd
		fmt.Fprintf(&builder, " token_usd=%.4f", tokenUsd)
	}

	return builder.String()
}
