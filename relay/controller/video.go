package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
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
	"github.com/songquanpeng/one-api/common/graceful"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/tracing"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/billing"
	billingratio "github.com/songquanpeng/one-api/relay/billing/ratio"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

// RelayVideoHelper handles OpenAI /v1/videos requests, performing quota accounting
// based on per-second pricing while proxying the raw payload to the upstream channel.
func RelayVideoHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	if c.Request.Method != http.MethodPost {
		return RelayProxyHelper(c, relaymode.Videos)
	}

	ctx := gmw.Ctx(c)
	lg := gmw.GetLogger(c)
	meta := metalib.GetByContext(c)

	videoRequest := &relaymodel.VideoRequest{}
	if err := common.UnmarshalBodyReusable(c, videoRequest); err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "parse video request"), "invalid_video_request", http.StatusBadRequest)
	}

	originalRequestedModel := strings.TrimSpace(videoRequest.Model)
	if originalRequestedModel == "" {
		if raw := strings.TrimSpace(c.GetString(ctxkey.RequestModel)); raw != "" {
			videoRequest.Model = raw
			lg.Debug("video request missing model, reusing context model",
				zap.String("resolved_model", videoRequest.Model))
		} else {
			videoRequest.Model = "sora-2"
			lg.Debug("video request missing model, using default",
				zap.String("resolved_model", videoRequest.Model))
		}
		originalRequestedModel = videoRequest.Model
	}

	requestSnapshot := map[string]any{
		"model": originalRequestedModel,
	}
	if trimmedPrompt := strings.TrimSpace(videoRequest.Prompt); trimmedPrompt != "" {
		runes := []rune(trimmedPrompt)
		if len(runes) > 512 {
			trimmedPrompt = string(runes[:512])
		}
		requestSnapshot["prompt"] = trimmedPrompt
	}
	if seconds := videoRequest.RequestedDurationSeconds(); seconds > 0 {
		requestSnapshot["duration_seconds"] = seconds
	}
	if resolution := videoRequest.RequestedResolution(); resolution != "" {
		requestSnapshot["resolution"] = resolution
	}
	if remix := strings.TrimSpace(videoRequest.RemixID); remix != "" {
		requestSnapshot["remix_id"] = remix
	}
	if reference := strings.TrimSpace(videoRequest.ReferenceID); reference != "" {
		requestSnapshot["reference_id"] = reference
	}
	requestSnapshot["method"] = c.Request.Method
	requestSnapshot["path"] = c.Request.URL.Path
	c.Set(ctxkey.AsyncTaskRequestMetadata, requestSnapshot)

	meta.OriginModelName = videoRequest.Model
	meta.ActualModelName = metalib.GetMappedModelName(videoRequest.Model, meta.ModelMapping)
	meta.EnsureActualModelName(videoRequest.Model)
	videoRequest.Model = meta.ActualModelName
	metalib.Set2Context(c, meta)

	durationSeconds := videoRequest.RequestedDurationSeconds()
	if durationSeconds <= 0 {
		return openai.ErrorWrapper(errors.New("seconds must be positive for video generation"), "invalid_video_duration", http.StatusBadRequest)
	}
	resolutionKey := videoRequest.RequestedResolution()

	var channelVideoOverride *adaptor.VideoPricingConfig
	if channelModel, ok := c.Get(ctxkey.ChannelModel); ok {
		if channel, ok := channelModel.(*model.Channel); ok {
			if cfg := channel.GetModelPriceConfig(meta.ActualModelName); cfg != nil && cfg.Video != nil {
				channelVideoOverride = convertVideoLocalToAdaptor(cfg.Video)
			}
		}
	}

	pricingAdaptor := relay.GetAdaptor(meta.APIType)
	videoPricing := pricing.GetVideoPricingWithThreeLayers(meta.ActualModelName, channelVideoOverride, pricingAdaptor)
	if videoPricing == nil {
		return openai.ErrorWrapper(errors.Errorf("video pricing missing for model %s", meta.ActualModelName), "video_pricing_missing", http.StatusBadRequest)
	}

	multiplier := videoPricing.EffectiveMultiplier(resolutionKey)
	costUsd := videoPricing.PerSecondUsd * multiplier * durationSeconds
	groupRatio := c.GetFloat64(ctxkey.ChannelRatio)
	usedQuota := max(int64(math.Ceil(costUsd*billingratio.QuotaPerUsd*groupRatio)), 0)

	tokenId := c.GetInt(ctxkey.TokenId)
	userId := meta.UserId
	channelId := meta.ChannelId
	tokenName := meta.TokenName

	preConsumedQuota := int64(0)
	userQuota, err := model.CacheGetUserQuota(ctx, userId)
	if err != nil {
		return openai.ErrorWrapper(err, "get_user_quota_failed", http.StatusInternalServerError)
	}

	if usedQuota > 0 {
		if userQuota-usedQuota < 0 {
			return openai.ErrorWrapper(errors.New("user quota is not enough"), "insufficient_user_quota", http.StatusForbidden)
		}
		if err := model.CacheDecreaseUserQuota(ctx, userId, usedQuota); err != nil {
			return openai.ErrorWrapper(err, "decrease_user_quota_failed", http.StatusInternalServerError)
		}

		tokenQuota := c.GetInt64(ctxkey.TokenQuota)
		tokenQuotaUnlimited := c.GetBool(ctxkey.TokenQuotaUnlimited)
		preConsumedQuota = usedQuota
		if userQuota > 100*usedQuota && (tokenQuotaUnlimited || tokenQuota > 100*usedQuota) {
			preConsumedQuota = 0
		}
		if preConsumedQuota > 0 {
			if err := model.PreConsumeTokenQuota(ctx, tokenId, preConsumedQuota); err != nil {
				return openai.ErrorWrapper(err, "pre_consume_token_quota_failed", http.StatusForbidden)
			}

			// Billing audit safety net
			markPreConsumed(c, preConsumedQuota)
			defer billingAuditSafetyNet(c)

			provisionalLogId := recordProvisionalLog(c, meta, videoRequest.Model, preConsumedQuota)
			c.Set(ctxkey.ProvisionalLogId, provisionalLogId)
		}
	}

	succeed := false
	requestId := c.GetString(ctxkey.RequestId)
	traceId := tracing.GetTraceID(c)

	defer func() {
		if !succeed {
			markBillingReconciled(c)
			if preConsumedQuota > 0 {
				quotaToReturn := preConsumedQuota
				graceful.GoCritical(ctx, "videoRollbackPreConsumed", func(bgctx context.Context) {
					if err := model.PostConsumeTokenQuota(bgctx, tokenId, -quotaToReturn); err != nil {
						gmw.GetLogger(bgctx).Error("error rolling back pre-consumed quota", zap.Error(err))
					}
				})
			}
			if usedQuota > 0 {
				if err := model.UpdateUserRequestCostQuotaByRequestID(userId, requestId, 0); err != nil {
					lg.Warn("update user request cost failed", zap.Error(err))
				}
			}
			return
		}

		quotaDelta := usedQuota - preConsumedQuota
		logContent := fmt.Sprintf("video seconds %.2f, usd %.3f, multiplier %.2f, group rate %.2f", durationSeconds, videoPricing.PerSecondUsd, multiplier, groupRatio)
		entry := &model.Log{
			UserId:      userId,
			ChannelId:   channelId,
			ModelName:   meta.ActualModelName,
			TokenName:   tokenName,
			Quota:       int(usedQuota),
			Content:     logContent,
			RequestId:   requestId,
			TraceId:     traceId,
			ElapsedTime: helper.CalcElapsedTime(meta.StartTime),
		}

		bgctx, cancel := context.WithTimeout(gmw.BackgroundCtx(c), time.Minute)
		defer cancel()
		graceful.GoCritical(bgctx, "videoPostConsume", func(cctx context.Context) {
			billing.PostConsumeQuotaWithLog(cctx, tokenId, quotaDelta, usedQuota, entry)
		})

		if err := model.UpdateUserRequestCostQuotaByRequestID(userId, requestId, usedQuota); err != nil {
			lg.Error("update user request cost failed", zap.Error(err))
		}
	}()

	rawBody, err := common.GetRequestBody(c)
	if err != nil {
		return openai.ErrorWrapper(err, "get_request_body_failed", http.StatusInternalServerError)
	}

	bodyBytes := rawBody
	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	if meta.OriginModelName != meta.ActualModelName && strings.HasPrefix(contentType, "application/json") {
		var payload map[string]any
		if err := json.Unmarshal(rawBody, &payload); err != nil {
			return openai.ErrorWrapper(errors.Wrap(err, "unmarshal video request for model mapping"), "invalid_video_request", http.StatusBadRequest)
		}
		payload["model"] = meta.ActualModelName
		bodyBytes, err = json.Marshal(payload)
		if err != nil {
			return openai.ErrorWrapper(errors.Wrap(err, "marshal video request after mapping"), "invalid_video_request", http.StatusInternalServerError)
		}
		c.Set(ctxkey.KeyRequestBody, bodyBytes)
		rawBody = bodyBytes
	} else if meta.OriginModelName != meta.ActualModelName && !strings.HasPrefix(contentType, "application/json") {
		lg.Warn("model mapping for non-JSON video request not applied", zap.String("content_type", contentType))
	}

	ad := relay.GetAdaptor(meta.APIType)
	if ad == nil {
		return openai.ErrorWrapper(errors.Errorf("invalid api type: %d", meta.APIType), "invalid_api_type", http.StatusBadRequest)
	}
	ad.Init(meta)

	requestBody := bytes.NewBuffer(bodyBytes)
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	resp, err := ad.DoRequest(c, meta, requestBody)
	if err != nil {
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}

	usage, respErr := ad.DoResponse(c, resp, meta)
	_ = usage // video responses currently do not return usage metrics
	if respErr != nil {
		return respErr
	}

	succeed = true
	markBillingReconciled(c)
	return nil
}

func convertVideoLocalToAdaptor(local *model.VideoPricingLocal) *adaptor.VideoPricingConfig {
	if local == nil {
		return nil
	}
	cfg := &adaptor.VideoPricingConfig{
		PerSecondUsd:   local.PerSecondUsd,
		BaseResolution: local.BaseResolution,
	}
	if len(local.ResolutionMultipliers) > 0 {
		cfg.ResolutionMultipliers = make(map[string]float64, len(local.ResolutionMultipliers))
		maps.Copy(cfg.ResolutionMultipliers, local.ResolutionMultipliers)
	}
	return cfg
}
