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

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/replicate"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/relaymode"
)

// RelayImageHelper relays one image request and reconciles its billing state.
// Parameters: c carries request, routing, and quota metadata; relayMode selects
// the image operation. Returns: a client-facing relay error or nil on success.
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
			channelModelRatio = channel.GetModelRatioFromConfigsWithContext(ctx)
			channelModelConfigs = channel.GetModelPriceConfigsWithContext(ctx)
		}
	}

	adaptor := relay.GetAdaptor(meta.APIType)
	if adaptor == nil {
		return openai.ErrorWrapper(errors.Errorf("invalid api type: %d", meta.APIType), "invalid_api_type", http.StatusBadRequest)
	}

	imagePricingCfg, _ := pricing.ResolveImagePricing(imageRequest.Model, channelModelConfigs, adaptor, meta.StartTime)
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
	visibleModelName := userVisibleModelName(meta, imageRequest.Model)
	c.Set(ctxkey.ResponseFormat, imageRequest.ResponseFormat)

	var requestBody io.Reader
	if strings.ToLower(c.GetString(ctxkey.ContentType)) == "application/json" &&
		isModelMapped || meta.ChannelType == channeltype.Azure { // make Azure channel request body
		requestToMarshal := any(imageRequest)
		if meta.Mode != relaymode.ImagesEdits &&
			(meta.ChannelType == channeltype.OpenAI || meta.ChannelType == channeltype.Azure) {
			requestToMarshal = buildOpenAIImageRequest(imageRequest)
		}
		jsonStr, err := json.Marshal(requestToMarshal)
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
		channeltype.Zai,
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
			jsonStr, err := json.Marshal(buildOpenAIImageRequest(imageRequest))
			if err != nil {
				return openai.ErrorWrapper(err, "marshal_image_request_failed", http.StatusInternalServerError)
			}

			requestBody = bytes.NewBuffer(jsonStr)
		}
	}

	// Resolve model ratio using unified three-layer pricing (channel overrides → adapter defaults → global fallback)
	// IMPORTANT: Use APIType here (adaptor family), not ChannelType. ChannelType IDs do not map to adaptor switch.
	pricingAdaptor := adaptor
	modelRatio := pricing.ResolveModelRatioAt(imageModel, channelModelConfigs, channelModelRatio, pricingAdaptor, meta.StartTime)
	// groupRatio := billingratio.GetGroupRatio(meta.Group)
	groupRatio := c.GetFloat64(ctxkey.ChannelRatio)

	// Channel override for size/quality tier multiplier (optional)
	if override, ok := getChannelImageTierOverride(channelModelRatio, imageModel, imageRequest.Size, imageRequest.Quality); ok {
		imageCostRatio = override
	}

	// Determine if this model is billed per image (Image.PricePerImageUsd) or per token (Ratio)
	imagePriceUsd := 0.0
	if imagePricingCfg != nil {
		imagePriceUsd = imagePricingCfg.PricePerImageUsd
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

		// Billing audit safety net: track pre-consumed quota for audit reconciliation
		markPreConsumed(c, preConsumedQuota)
		defer billingAuditSafetyNet(c)

		// Record provisional consume log immediately so that every pre-consume
		// has an audit trail in the logs table.
		provisionalLogId := recordProvisionalLog(c, meta, visibleModelName, preConsumedQuota)
		c.Set(ctxkey.ProvisionalLogId, provisionalLogId)

		// Record provisional request cost
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
		bgCtx, cancel := context.WithTimeout(gmw.BackgroundCtx(c), time.Minute)
		reconcileImageFailureBilling(c, bgCtx, meta.TokenId, preConsumedQuota,
			c.GetInt(ctxkey.ProvisionalLogId), "do_request_failed")
		cancel()
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}

	var promptTokens, completionTokens int
	// Capture IDs from gin context before switching to a background context in defer
	requestId := c.GetString(ctxkey.RequestId)
	traceId := tracing.GetTraceID(c)
	provLogID := c.GetInt(ctxkey.ProvisionalLogId)
	// NOTE: This image post-billing/refund runs in a SYNCHRONOUS defer — it executes on the
	// request goroutine, inside the handler call stack, BEFORE ServeHTTP returns and gin
	// recycles c via sync.Pool. It is therefore NOT the async-goroutine race class the
	// proposal addresses (docs/proposals/20260608_relay-billing-async-sync-race-fixes.md):
	// reading c here is safe. gmw.BackgroundCtx(c) is used only to DETACH the DB writes from
	// request-context cancellation (a client disconnect must not abort the refund), not to
	// hand c to a goroutine. Do NOT copy this pattern into a `go func`/GoCritical — there it
	// would be a use-after-return; use detachForBilling(c)/goDetachedBillingWork instead.
	defer func() {
		bgCtx, cancel := context.WithTimeout(gmw.BackgroundCtx(c), time.Minute)
		defer cancel()

		if imageResponseRequiresFailureReconciliation(resp) {
			reconcileImageFailureBilling(c, bgCtx, meta.TokenId, preConsumedQuota, provLogID, "upstream_http_error")
			return
		}

		// Post-billing: reconcile pre-consumed quota with actual usage
		markBillingReconciled(c)
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
				OriginModel:     visibleModelName,
				Model:           visibleModelName,
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
			// Reconcile provisional log if one exists, otherwise create a new log entry.
			elapsedTime := helper.CalcElapsedTime(meta.StartTime)
			if provLogID > 0 {
				if err := model.ReconcileConsumeLog(bgCtx, provLogID, usedQuota,
					logContent, promptTokens, completionTokens,
					elapsedTime, nil); err != nil {
					lg.Error("failed to reconcile provisional log, falling back to new log entry",
						zap.Error(err), zap.Int("provisional_log_id", provLogID))
					model.RecordConsumeLog(bgCtx, &model.Log{
						UserId:           meta.UserId,
						UserUUID:         model.StringPtrIfNotEmpty(meta.UserUUID),
						ChannelId:        meta.ChannelId,
						ChannelUUID:      model.StringPtrIfNotEmpty(meta.ChannelUUID),
						PromptTokens:     promptTokens,
						CompletionTokens: completionTokens,
						ModelName:        visibleModelName,
						TokenName:        tokenName,
						TokenUUID:        model.StringPtrIfNotEmpty(meta.TokenUUID),
						Quota:            int(usedQuota),
						Content:          logContent,
						ElapsedTime:      elapsedTime,
						RequestId:        requestId,
						TraceId:          traceId,
					})
				}
			} else {
				model.RecordConsumeLog(bgCtx, &model.Log{
					UserId:           meta.UserId,
					UserUUID:         model.StringPtrIfNotEmpty(meta.UserUUID),
					ChannelId:        meta.ChannelId,
					ChannelUUID:      model.StringPtrIfNotEmpty(meta.ChannelUUID),
					PromptTokens:     promptTokens,
					CompletionTokens: completionTokens,
					ModelName:        visibleModelName,
					TokenName:        tokenName,
					TokenUUID:        model.StringPtrIfNotEmpty(meta.TokenUUID),
					Quota:            int(usedQuota),
					Content:          logContent,
					ElapsedTime:      elapsedTime,
					RequestId:        requestId,
					TraceId:          traceId,
				})
			}
			model.UpdateUserUsedQuotaAndRequestCountWithContext(bgCtx, meta.UserId, usedQuota)
			channelId := c.GetInt(ctxkey.ChannelId)
			model.UpdateChannelUsedQuotaWithContext(bgCtx, channelId, usedQuota)

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

// reconcileImageFailureBilling settles image billing after an upstream or
// transport failure. Parameters: c is the live request context, bgCtx is a
// detached database context, tokenID identifies the charged token,
// preConsumedQuota is the provisional debit, provisionalLogID identifies its
// audit row, and reason labels the failure. Returns: none; persistence errors are
// logged for manual reconciliation.
func reconcileImageFailureBilling(
	c *gin.Context,
	bgCtx context.Context,
	tokenID int,
	preConsumedQuota int64,
	provisionalLogID int,
	reason string,
) {
	lg := gmw.GetLogger(c)
	markBillingReconciled(c)

	finalQuota := int64(0)
	requestCost := int64(0)
	logContent := "request failed, refunded"
	if reason == "upstream_http_error" {
		logContent = "upstream error, refunded"
	}
	if preConsumedQuota > 0 {
		if shouldSkipPreConsumedRefund(c) {
			finalQuota = preConsumedQuota
			requestCost = preConsumedQuota
			logContent = "request failed, charge retained after forwarding"
			if reason == "upstream_http_error" {
				logContent = "upstream error, charge retained after forwarding"
			}
			lg.Warn("skip pre-consumed refund to prevent underbilling",
				zap.Int64("pre_consumed_quota", preConsumedQuota),
				zap.String("reason", reason),
			)
		} else if err := model.PostConsumeTokenQuota(bgCtx, tokenID, -preConsumedQuota); err != nil {
			finalQuota = preConsumedQuota
			requestCost = preConsumedQuota
			logContent = "request failed, refund failed; charge retained"
			if reason == "upstream_http_error" {
				logContent = "upstream error, refund failed; charge retained"
			}
			lg.Error("CRITICAL BILLING AUDIT: image upstream error refund failed",
				zap.Error(err),
				zap.Int64("pre_consumed_quota", preConsumedQuota),
				zap.String("reason", reason),
			)
		}
	}

	if provisionalLogID > 0 {
		if err := model.ReconcileConsumeLog(bgCtx, provisionalLogID, finalQuota,
			logContent, 0, 0, 0, nil); err != nil {
			lg.Warn("failed to reconcile provisional image log on upstream error",
				zap.Error(err), zap.Int("provisional_log_id", provisionalLogID))
		}
	}
	if err := model.UpdateUserRequestCostQuotaByRequestID(
		c.GetInt(ctxkey.Id),
		c.GetString(ctxkey.RequestId),
		requestCost,
	); err != nil {
		lg.Warn("failed to reconcile image request cost on upstream error", zap.Error(err))
	}
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
	// https://platform.openai.com/docs/models/gpt-image-2
	"gpt-image-2": {
		inputTextUSD:        5.0,
		cachedInputTextUSD:  1.25,
		inputImageUSD:       8.0,
		cachedInputImageUSD: 2.0,
		outputImageUSD:      30.0,
	},
	"gpt-image-2-2026-04-21": {
		inputTextUSD:        5.0,
		cachedInputTextUSD:  1.25,
		inputImageUSD:       8.0,
		cachedInputImageUSD: 2.0,
		outputImageUSD:      30.0,
	},
}

// computeGptImageTokenQuota calculates quota for GPT image family models using five billing buckets:
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
	case "gpt-image-1", "gpt-image-1-mini", "chatgpt-image-latest", "gpt-image-1.5", "gpt-image-1.5-2025-12-16", "gpt-image-2", "gpt-image-2-2026-04-21":
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
	case "chatgpt-image-latest", "gpt-image-1.5", "gpt-image-1.5-2025-12-16", "gpt-image-2", "gpt-image-2-2026-04-21":
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
