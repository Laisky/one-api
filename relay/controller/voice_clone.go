package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
)

// RelayVoiceCloneHelper handles POST /v1/voice/clones requests using the
// dedicated voice-clone DTO pipeline (e.g. Zhipu /api/paas/v4/voice/clone).
func RelayVoiceCloneHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	lg := gmw.GetLogger(c)
	ctx := gmw.Ctx(c)
	meta := metalib.GetByContext(c)

	voiceCloneRequest, err := getAndValidateVoiceCloneRequest(c)
	if err != nil {
		return openai.ErrorWrapper(err, "invalid_voice_clone_request", http.StatusBadRequest)
	}

	meta.IsStream = false
	meta.OriginModelName = voiceCloneRequest.Model
	meta.ActualModelName = metalib.GetMappedModelName(voiceCloneRequest.Model, meta.ModelMapping)
	voiceCloneRequest.Model = meta.ActualModelName
	metalib.Set2Context(c, meta)

	channelModelRatio, _ := getChannelRatios(c)
	channelModelConfigs := getChannelModelConfigs(c)
	pricingAdaptor := resolvePricingAdaptor(meta)
	modelRatio := pricing.ResolveModelRatioAt(voiceCloneRequest.Model, channelModelConfigs, channelModelRatio, pricingAdaptor, meta.StartTime)
	groupRatio := c.GetFloat64(ctxkey.ChannelRatio)
	totalQuota := int64(math.Ceil(modelRatio * groupRatio))
	if modelRatio > 0 && totalQuota == 0 {
		totalQuota = 1
	}

	preConsumedQuota, bizErr := preConsumeVoiceCloneQuota(c, totalQuota, meta)
	if bizErr != nil {
		lg.Warn("preConsumeVoiceCloneQuota failed",
			zap.Error(bizErr.RawError),
			zap.Int("status_code", bizErr.StatusCode),
			zap.String("err_msg", bizErr.Message))
		return bizErr
	}
	markPreConsumed(c, preConsumedQuota)
	defer billingAuditSafetyNet(c)

	provisionalLogId := recordProvisionalLog(c, meta, voiceCloneRequest.Model, preConsumedQuota)
	c.Set(ctxkey.ProvisionalLogId, provisionalLogId)

	adaptorImpl := relay.GetAdaptor(meta.APIType)
	if adaptorImpl == nil {
		_ = returnPreConsumedQuotaConservative(ctx, c, preConsumedQuota, meta.TokenId, "invalid_api_type")
		preConsumedQuota = 0
		return openai.ErrorWrapper(errors.Errorf("invalid api type: %d", meta.APIType), "invalid_api_type", http.StatusBadRequest)
	}
	adaptorImpl.Init(meta)

	voiceCloneAdaptor, ok := adaptorImpl.(adaptor.VoiceCloneAdaptor)
	if !ok {
		_ = returnPreConsumedQuotaConservative(ctx, c, preConsumedQuota, meta.TokenId, "voice_clone_not_supported")
		preConsumedQuota = 0
		return openai.ErrorWrapper(errors.New("adaptor does not support voice clone"), "voice_clone_not_supported", http.StatusBadRequest)
	}

	convertedAny, err := voiceCloneAdaptor.ConvertVoiceCloneRequest(c, voiceCloneRequest)
	if err != nil {
		_ = returnPreConsumedQuotaConservative(ctx, c, preConsumedQuota, meta.TokenId, "convert_request_failed")
		return openai.ErrorWrapper(err, "convert_request_failed", http.StatusInternalServerError)
	}
	c.Set(ctxkey.ConvertedRequest, convertedAny)

	payload, err := json.Marshal(convertedAny)
	if err != nil {
		_ = returnPreConsumedQuotaConservative(ctx, c, preConsumedQuota, meta.TokenId, "marshal_request_failed")
		return openai.ErrorWrapper(err, "marshal_request_failed", http.StatusInternalServerError)
	}
	requestBody := bytes.NewBuffer(payload)
	c.Request.Body = io.NopCloser(bytes.NewReader(payload))
	c.Set(ctxkey.KeyRequestBody, payload)

	resp, err := adaptorImpl.DoRequest(c, meta, requestBody)
	if err != nil {
		_ = returnPreConsumedQuotaConservative(ctx, c, preConsumedQuota, meta.TokenId, "do_request_failed")
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}

	upstreamCapture := wrapUpstreamResponse(resp)

	quotaId := c.GetInt(ctxkey.Id)
	requestId := c.GetString(ctxkey.RequestId)
	provisionalQuota := preConsumedQuota
	if provisionalQuota == 0 && totalQuota > 0 {
		provisionalQuota = totalQuota
	}
	if requestId != "" {
		if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, provisionalQuota); err != nil {
			lg.Warn("record provisional user request cost failed", zap.Error(err), zap.String("request_id", requestId))
		}
	}

	if isErrorHappened(meta, resp) {
		scheduleConservativeRefund(c, preConsumedQuota, meta.TokenId, "upstream_http_error")
		if requestId != "" {
			if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, 0); err != nil {
				lg.Warn("update user request cost to zero failed", zap.Error(err))
			}
		}
		return RelayErrorHandlerWithContext(c, resp)
	}

	c.Set(ctxkey.SkipAdaptorResponseBodyLog, true)
	usage, respErr := voiceCloneAdaptor.DoVoiceCloneResponse(c, resp, meta)
	if upstreamCapture != nil {
		logUpstreamResponseFromCapture(lg, resp, upstreamCapture, "voice_clone")
	} else {
		logUpstreamResponseFromBytes(lg, resp, nil, "voice_clone")
	}
	if respErr != nil {
		if usage == nil {
			scheduleConservativeRefund(c, preConsumedQuota, meta.TokenId, "do_response_failed_without_usage")
			if requestId != "" {
				if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, 0); err != nil {
					lg.Warn("update user request cost to zero failed", zap.Error(err))
				}
			}
			return respErr
		}
	}

	_ = returnPreConsumedQuotaConservative(ctx, c, preConsumedQuota, meta.TokenId, "pre_billing_reconcile")
	markBillingReconciled(c)

	runPostBillingWithTimeout(detachForBilling(c), "postBillingVoiceClone", lg, postBillingTimeoutInfo{
		userID:          meta.UserId,
		channelID:       meta.ChannelId,
		model:           voiceCloneRequest.Model,
		requestID:       requestId,
		startTime:       meta.StartTime,
		estimatedQuota:  func() float64 { return float64(totalQuota) },
		guardTimeoutLog: func() bool { return true },
		logMessage:      "CRITICAL BILLING TIMEOUT",
	}, func(ctx context.Context) {
		quota := postConsumeVoiceCloneQuota(ctx, meta, voiceCloneRequest, preConsumedQuota, totalQuota, modelRatio, groupRatio)
		if requestId != "" {
			if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, quota); err != nil {
				lg.Error("update user request cost failed", zap.Error(err), zap.String("request_id", requestId))
			}
		}
	})

	return nil
}

// getAndValidateVoiceCloneRequest parses and validates the voice-clone payload.
func getAndValidateVoiceCloneRequest(c *gin.Context) (*relaymodel.VoiceCloneRequest, error) {
	voiceCloneRequest := &relaymodel.VoiceCloneRequest{}
	if err := common.UnmarshalBodyReusable(c, voiceCloneRequest); err != nil {
		return nil, errors.Wrap(err, "unmarshal voice clone request")
	}

	if err := voiceCloneRequest.Normalize(); err != nil {
		return nil, errors.Wrap(err, "normalize voice clone request")
	}

	return voiceCloneRequest, nil
}

// preConsumeVoiceCloneQuota reserves quota for a per-call voice-clone request,
// skipping pre-consumption for trusted users with ample balance.
func preConsumeVoiceCloneQuota(c *gin.Context, perCallQuota int64, meta *metalib.Meta) (int64, *relaymodel.ErrorWithStatusCode) {
	ctx := gmw.Ctx(c)
	lg := gmw.GetLogger(c)

	if perCallQuota < 0 {
		perCallQuota = 0
	}
	if perCallQuota == 0 {
		return 0, nil
	}

	tokenQuota := c.GetInt64(ctxkey.TokenQuota)
	tokenQuotaUnlimited := c.GetBool(ctxkey.TokenQuotaUnlimited)
	userQuota, err := model.CacheGetUserQuota(ctx, meta.UserId)
	if err != nil {
		return perCallQuota, openai.ErrorWrapper(err, "get_user_quota_failed", http.StatusInternalServerError)
	}
	if userQuota-perCallQuota < 0 {
		return perCallQuota, openai.ErrorWrapper(errors.New("user quota is not enough"), "insufficient_user_quota", http.StatusForbidden)
	}

	if userQuota > 100*perCallQuota && (tokenQuotaUnlimited || tokenQuota > 100*perCallQuota) {
		lg.Info("user has enough quota, trusted and no need to pre-consume", zap.Int64("user_quota", userQuota))
		return 0, nil
	}

	if err := model.PreConsumeTokenQuota(ctx, meta.TokenId, perCallQuota); err != nil {
		return perCallQuota, openai.ErrorWrapper(err, "pre_consume_token_quota_failed", http.StatusForbidden)
	}
	syncUserQuotaCacheAfterPreConsume(ctx, meta.UserId, perCallQuota, "voice_clone_preconsume")

	return perCallQuota, nil
}

// postConsumeVoiceCloneQuota settles the per-call charge after the upstream
// request completes, writing the billing log entry on a detached context.
func postConsumeVoiceCloneQuota(ctx context.Context,
	meta *metalib.Meta,
	request *relaymodel.VoiceCloneRequest,
	preConsumedQuota int64,
	totalQuota int64,
	modelRatio float64,
	groupRatio float64) (quota int64) {
	quota = max(totalQuota, 0)
	quotaDelta := quota - preConsumedQuota

	billingID := billingIdentityFromContext(ctx)
	requestId := billingID.requestID
	provLogID := billingID.provisionalLogID
	traceId := billingID.traceID

	if meta.TokenId > 0 && meta.UserId > 0 && meta.ChannelId > 0 {
		logEntry := &model.Log{
			UserId:      meta.UserId,
			ChannelId:   meta.ChannelId,
			ModelName:   request.Model,
			TokenName:   meta.TokenName,
			Content:     fmt.Sprintf("voice clone per-call billing, base unit %.2f, group rate %.2f", modelRatio, groupRatio),
			IsStream:    false,
			ElapsedTime: helper.CalcElapsedTime(meta.StartTime),
			RequestId:   requestId,
			TraceId:     traceId,
		}
		model.SetLogExternalUUIDs(logEntry, meta.UserUUID, meta.ChannelUUID, meta.TokenUUID)
		billing.PostConsumeQuotaWithLog(ctx, meta.TokenId, quotaDelta, quota, logEntry, provLogID)
	} else {
		gmw.GetLogger(ctx).Error("meta information incomplete, cannot post consume voice clone quota",
			zap.Int("meta_token_id", meta.TokenId),
			zap.Int("meta_user_id", meta.UserId),
			zap.Int("meta_channel_id", meta.ChannelId),
			zap.String("request_id", requestId),
			zap.String("trace_id", traceId),
		)
	}

	return quota
}
