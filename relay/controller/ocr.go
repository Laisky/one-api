package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/metrics"
	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/relaymode"
)

// RelayOCRHelper handles POST /v1/layout_parsing and /api/paas/v4/layout_parsing requests.
func RelayOCRHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	lg := gmw.GetLogger(c)
	ctx := gmw.Ctx(c)
	meta := metalib.GetByContext(c)

	if err := logClientRequestPayload(c, "ocr"); err != nil {
		return openai.ErrorWrapper(err, "invalid_ocr_request", http.StatusBadRequest)
	}

	ocrRequest, err := getAndValidateOCRRequest(c)
	if err != nil {
		return openai.ErrorWrapper(err, "invalid_ocr_request", http.StatusBadRequest)
	}

	meta.IsStream = false
	meta.OriginModelName = ocrRequest.Model
	meta.ActualModelName = metalib.GetMappedModelName(ocrRequest.Model, meta.ModelMapping)
	ocrRequest.Model = meta.ActualModelName
	metalib.Set2Context(c, meta)

	channelModelRatio, _ := getChannelRatios(c)
	pricingAdaptor := resolvePricingAdaptor(meta)
	modelRatio := pricing.GetModelRatioWithThreeLayers(ocrRequest.Model, channelModelRatio, pricingAdaptor)
	groupRatio := c.GetFloat64(ctxkey.ChannelRatio)
	totalQuota := int64(math.Ceil(modelRatio * groupRatio))
	if modelRatio > 0 && totalQuota == 0 {
		totalQuota = 1
	}

	meta.PromptTokens = 0

	preConsumedQuota, bizErr := preConsumeOCRQuota(c, totalQuota, meta)
	if bizErr != nil {
		lg.Warn("preConsumeOCRQuota failed",
			zap.Error(bizErr.RawError),
			zap.Int("status_code", bizErr.StatusCode),
			zap.String("err_msg", bizErr.Message))
		return bizErr
	}
	markPreConsumed(c, preConsumedQuota)
	defer billingAuditSafetyNet(c)

	provisionalLogId := recordProvisionalLog(c, meta, ocrRequest.Model, preConsumedQuota)
	c.Set(ctxkey.ProvisionalLogId, provisionalLogId)

	adaptorImpl := relay.GetAdaptor(meta.APIType)
	if adaptorImpl == nil {
		_ = returnPreConsumedQuotaConservative(ctx, c, preConsumedQuota, meta.TokenId, "invalid_api_type")
		return openai.ErrorWrapper(errors.Errorf("invalid api type: %d", meta.APIType), "invalid_api_type", http.StatusBadRequest)
	}
	adaptorImpl.Init(meta)

	requestBody, err := prepareOCRRequestBody(c, meta, adaptorImpl, ocrRequest)
	if err != nil {
		_ = returnPreConsumedQuotaConservative(ctx, c, preConsumedQuota, meta.TokenId, "convert_request_failed")
		return openai.ErrorWrapper(err, "convert_request_failed", http.StatusInternalServerError)
	}

	requestBodyBytes, _ := io.ReadAll(requestBody)
	requestBody = bytes.NewBuffer(requestBodyBytes)

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
		graceful.GoCritical(ctx, "returnPreConsumedQuota", func(cctx context.Context) {
			_ = returnPreConsumedQuotaConservative(cctx, c, preConsumedQuota, meta.TokenId, "upstream_http_error")
		})
		if requestId != "" {
			if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, 0); err != nil {
				lg.Warn("update user request cost to zero failed", zap.Error(err))
			}
		}
		return RelayErrorHandlerWithContext(c, resp)
	}

	ocrAdaptor, ok := adaptorImpl.(adaptor.OCRAdaptor)
	if !ok {
		_ = returnPreConsumedQuotaConservative(ctx, c, preConsumedQuota, meta.TokenId, "ocr_not_supported")
		return openai.ErrorWrapper(errors.New("adaptor does not support OCR"), "ocr_not_supported", http.StatusBadRequest)
	}

	c.Set(ctxkey.SkipAdaptorResponseBodyLog, true)
	usage, respErr := ocrAdaptor.DoOCRResponse(c, resp, meta)
	if upstreamCapture != nil {
		logUpstreamResponseFromCapture(lg, resp, upstreamCapture, "ocr")
	} else {
		logUpstreamResponseFromBytes(lg, resp, nil, "ocr")
	}
	if respErr != nil {
		if usage == nil {
			graceful.GoCritical(ctx, "returnPreConsumedQuota", func(cctx context.Context) {
				_ = returnPreConsumedQuotaConservative(cctx, c, preConsumedQuota, meta.TokenId, "do_response_failed_without_usage")
			})
			if requestId != "" {
				if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, 0); err != nil {
					lg.Warn("update user request cost to zero failed", zap.Error(err))
				}
			}
			return respErr
		}
	}

	_ = returnPreConsumedQuotaConservative(ctx, c, preConsumedQuota, meta.TokenId, "pre_billing_reconcile")
	preConsumedQuota = 0

	if usage != nil {
		userIdStr := strconv.Itoa(meta.UserId)
		username := c.GetString(ctxkey.Username)
		if username == "" {
			username = "unknown"
		}
		group := meta.Group
		if group == "" {
			group = "default"
		}

		apiFormat := c.GetString(ctxkey.APIFormat)
		if apiFormat == "" {
			apiFormat = "unknown"
		}
		apiType := relaymode.String(meta.Mode)
		tokenId := strconv.Itoa(meta.TokenId)

		metrics.GlobalRecorder.RecordRelayRequest(
			meta.StartTime,
			meta.ChannelId,
			channeltype.IdToName(meta.ChannelType),
			meta.ActualModelName,
			userIdStr,
			group,
			tokenId,
			apiFormat,
			apiType,
			true,
			usage.PromptTokens,
			usage.CompletionTokens,
			0,
		)

		userBalance := float64(getUserQuotaFromContext(c))
		metrics.GlobalRecorder.RecordUserMetrics(
			userIdStr,
			username,
			group,
			0,
			usage.PromptTokens,
			usage.CompletionTokens,
			userBalance,
		)

		metrics.GlobalRecorder.RecordModelUsage(meta.ActualModelName, channeltype.IdToName(meta.ChannelType), time.Since(meta.StartTime))
	}

	markBillingReconciled(c)
	graceful.GoCritical(gmw.BackgroundCtx(c), "postBillingOCR", func(bctx context.Context) {
		baseBillingTimeout := time.Duration(config.BillingTimeoutSec) * time.Second
		bctx, cancel := context.WithTimeout(gmw.BackgroundCtx(c), baseBillingTimeout)
		defer cancel()

		done := make(chan bool, 1)
		var quota int64

		go func() {
			quota = postConsumeOCRQuota(bctx, usage, meta, ocrRequest, preConsumedQuota, totalQuota, modelRatio, groupRatio)
			if requestId != "" {
				if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, quota); err != nil {
					lg.Error("update user request cost failed", zap.Error(err), zap.String("request_id", requestId))
				}
			}
			done <- true
		}()

		select {
		case <-done:
		case <-bctx.Done():
			if errors.Is(bctx.Err(), context.DeadlineExceeded) && usage != nil {
				estimatedQuota := float64(totalQuota)
				elapsedTime := time.Since(meta.StartTime)
				lg.Error("CRITICAL BILLING TIMEOUT",
					zap.String("model", ocrRequest.Model),
					zap.String("requestId", requestId),
					zap.Int("userId", meta.UserId),
					zap.Int64("estimatedQuota", int64(estimatedQuota)),
					zap.Duration("elapsedTime", elapsedTime))
				metrics.GlobalRecorder.RecordBillingTimeout(meta.UserId, meta.ChannelId, ocrRequest.Model, estimatedQuota, elapsedTime)
			}
		}
	})

	return nil
}

func getAndValidateOCRRequest(c *gin.Context) (*relaymodel.OCRRequest, error) {
	rawBody, err := common.GetRequestBody(c)
	if err != nil {
		return nil, errors.Wrap(err, "get request body")
	}
	_ = rawBody

	ocrRequest := &relaymodel.OCRRequest{}
	if err := common.UnmarshalBodyReusable(c, ocrRequest); err != nil {
		return nil, errors.Wrap(err, "unmarshal OCR request")
	}

	if ocrRequest.Model == "" {
		return nil, errors.New("field model is required")
	}
	if ocrRequest.File == "" {
		return nil, errors.New("field file is required")
	}

	return ocrRequest, nil
}

func prepareOCRRequestBody(c *gin.Context, meta *metalib.Meta, adaptorImpl adaptor.Adaptor, request *relaymodel.OCRRequest) (io.Reader, error) {
	if request == nil {
		return nil, errors.New("OCR request is nil")
	}

	if ocrAdaptor, ok := adaptorImpl.(adaptor.OCRAdaptor); ok {
		converted, err := ocrAdaptor.ConvertOCRRequest(c, request)
		if err != nil {
			return nil, errors.Wrap(err, "convert OCR request")
		}
		c.Set(ctxkey.ConvertedRequest, converted)

		payload, err := json.Marshal(converted)
		if err != nil {
			return nil, errors.Wrap(err, "marshal OCR request")
		}
		return bytes.NewBuffer(payload), nil
	}

	channelName := adaptorImpl.GetChannelName()
	if channelName == "" {
		channelName = "unknown"
	}
	return nil, errors.Errorf("OCR requests are not supported by adaptor %s", channelName)
}

func preConsumeOCRQuota(c *gin.Context, perCallQuota int64, meta *metalib.Meta) (int64, *relaymodel.ErrorWithStatusCode) {
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
		lg.Info("user has enough quota, trusted and no need to pre-consume", zap.Int("user_id", meta.UserId), zap.Int64("user_quota", userQuota))
		return 0, nil
	}

	if err := model.PreConsumeTokenQuota(ctx, meta.TokenId, perCallQuota); err != nil {
		return perCallQuota, openai.ErrorWrapper(err, "pre_consume_token_quota_failed", http.StatusForbidden)
	}
	syncUserQuotaCacheAfterPreConsume(ctx, meta.UserId, perCallQuota, "ocr_preconsume")

	return perCallQuota, nil
}

func postConsumeOCRQuota(ctx context.Context,
	usage *relaymodel.Usage,
	meta *metalib.Meta,
	request *relaymodel.OCRRequest,
	preConsumedQuota int64,
	totalQuota int64,
	modelRatio float64,
	groupRatio float64) (quota int64) {
	quota = max(totalQuota, 0)

	var requestId string
	var provLogID int
	if ginCtx, ok := gmw.GetGinCtxFromStdCtx(ctx); ok {
		requestId = ginCtx.GetString(ctxkey.RequestId)
		provLogID = ginCtx.GetInt(ctxkey.ProvisionalLogId)
	}
	traceId := tracing.GetTraceIDFromContext(ctx)

	var promptTokens, completionTokens int
	if usage != nil {
		promptTokens = usage.PromptTokens
		completionTokens = usage.CompletionTokens
	}

	if meta.TokenId > 0 && meta.UserId > 0 && meta.ChannelId > 0 {
		logEntry := &model.Log{
			UserId:           meta.UserId,
			ChannelId:        meta.ChannelId,
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			ModelName:        request.Model,
			TokenName:        meta.TokenName,
			Content:          fmt.Sprintf("OCR per-call billing, base unit %.2f, group rate %.2f", modelRatio, groupRatio),
			IsStream:         false,
			ElapsedTime:      helper.CalcElapsedTime(meta.StartTime),
			RequestId:        requestId,
			TraceId:          traceId,
		}
		billing.PostConsumeQuotaWithLog(ctx, meta.TokenId, quota-preConsumedQuota, quota, logEntry, provLogID)
	} else {
		gmw.GetLogger(ctx).Error("meta information incomplete, cannot post consume OCR quota",
			zap.Int("token_id", meta.TokenId),
			zap.Int("user_id", meta.UserId),
			zap.Int("channel_id", meta.ChannelId),
			zap.String("request_id", requestId),
			zap.String("trace_id", traceId),
		)
	}

	return quota
}
