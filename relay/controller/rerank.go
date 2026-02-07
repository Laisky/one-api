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

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/graceful"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/metrics"
	"github.com/songquanpeng/one-api/common/tracing"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/billing"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/controller/validator"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

// RelayRerankHelper handles POST /v1/rerank requests using the dedicated DTO pipeline.
func RelayRerankHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	lg := gmw.GetLogger(c)
	ctx := gmw.Ctx(c)
	meta := metalib.GetByContext(c)

	if err := logClientRequestPayload(c, "rerank"); err != nil {
		return openai.ErrorWrapper(err, "invalid_rerank_request", http.StatusBadRequest)
	}

	rerankRequest, err := getAndValidateRerankRequest(c)
	if err != nil {
		return openai.ErrorWrapper(err, "invalid_rerank_request", http.StatusBadRequest)
	}

	meta.IsStream = false
	meta.OriginModelName = rerankRequest.Model
	meta.ActualModelName = metalib.GetMappedModelName(rerankRequest.Model, meta.ModelMapping)
	rerankRequest.Model = meta.ActualModelName
	metalib.Set2Context(c, meta)

	channelModelRatio, _ := getChannelRatios(c)
	pricingAdaptor := resolvePricingAdaptor(meta)
	modelRatio := pricing.GetModelRatioWithThreeLayers(rerankRequest.Model, channelModelRatio, pricingAdaptor)
	groupRatio := c.GetFloat64(ctxkey.ChannelRatio)
	totalQuota := int64(math.Ceil(modelRatio * groupRatio))
	if modelRatio > 0 && totalQuota == 0 {
		totalQuota = 1
	}

	promptTokens := countRerankPromptTokens(ctx, rerankRequest)
	meta.PromptTokens = promptTokens

	preConsumedQuota, bizErr := preConsumeRerankQuota(c, totalQuota, meta)
	if bizErr != nil {
		lg.Warn("preConsumeRerankQuota failed",
			zap.Error(bizErr.RawError),
			zap.Int("status_code", bizErr.StatusCode),
			zap.String("err_msg", bizErr.Message))
		return bizErr
	}

	adaptorImpl := relay.GetAdaptor(meta.APIType)
	if adaptorImpl == nil {
		billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, meta.TokenId)
		preConsumedQuota = 0
		return openai.ErrorWrapper(errors.Errorf("invalid api type: %d", meta.APIType), "invalid_api_type", http.StatusBadRequest)
	}
	adaptorImpl.Init(meta)

	requestBody, err := prepareRerankRequestBody(c, meta, adaptorImpl, rerankRequest)
	if err != nil {
		billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, meta.TokenId)
		return openai.ErrorWrapper(err, "convert_request_failed", http.StatusInternalServerError)
	}

	requestBodyBytes, _ := io.ReadAll(requestBody)
	requestBody = bytes.NewBuffer(requestBodyBytes)

	resp, err := adaptorImpl.DoRequest(c, meta, requestBody)
	if err != nil {
		billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, meta.TokenId)
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
			billing.ReturnPreConsumedQuota(cctx, preConsumedQuota, meta.TokenId)
		})
		if requestId != "" {
			if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, 0); err != nil {
				lg.Warn("update user request cost to zero failed", zap.Error(err))
			}
		}
		return RelayErrorHandlerWithContext(c, resp)
	}

	c.Set(ctxkey.SkipAdaptorResponseBodyLog, true)
	usage, respErr := adaptorImpl.DoResponse(c, resp, meta)
	if upstreamCapture != nil {
		logUpstreamResponseFromCapture(lg, resp, upstreamCapture, "rerank")
	} else {
		logUpstreamResponseFromBytes(lg, resp, nil, "rerank")
	}
	if respErr != nil {
		if usage == nil {
			graceful.GoCritical(ctx, "returnPreConsumedQuota", func(cctx context.Context) {
				billing.ReturnPreConsumedQuota(cctx, preConsumedQuota, meta.TokenId)
			})
			if requestId != "" {
				if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, 0); err != nil {
					lg.Warn("update user request cost to zero failed", zap.Error(err))
				}
			}
			return respErr
		}
	}

	billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, meta.TokenId)
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

		userBalance := float64(c.GetInt64(ctxkey.UserQuota))
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

	graceful.GoCritical(gmw.BackgroundCtx(c), "postBillingRerank", func(bctx context.Context) {
		baseBillingTimeout := time.Duration(config.BillingTimeoutSec) * time.Second
		bctx, cancel := context.WithTimeout(gmw.BackgroundCtx(c), baseBillingTimeout)
		defer cancel()

		done := make(chan bool, 1)
		var quota int64

		go func() {
			quota = postConsumeRerankQuota(bctx, usage, meta, rerankRequest, preConsumedQuota, totalQuota, modelRatio, groupRatio)
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
			if bctx.Err() == context.DeadlineExceeded && usage != nil {
				estimatedQuota := float64(totalQuota)
				elapsedTime := time.Since(meta.StartTime)
				lg.Error("CRITICAL BILLING TIMEOUT",
					zap.String("model", rerankRequest.Model),
					zap.String("requestId", requestId),
					zap.Int("userId", meta.UserId),
					zap.Int64("estimatedQuota", int64(estimatedQuota)),
					zap.Duration("elapsedTime", elapsedTime))
				metrics.GlobalRecorder.RecordBillingTimeout(meta.UserId, meta.ChannelId, rerankRequest.Model, estimatedQuota, elapsedTime)
			}
		}
	})

	return nil
}

func getAndValidateRerankRequest(c *gin.Context) (*relaymodel.RerankRequest, error) {
	rawBody, err := common.GetRequestBody(c)
	if err != nil {
		return nil, errors.Wrap(err, "get request body")
	}

	if err := validator.ValidateUnknownParameters(rawBody); err != nil {
		return nil, errors.Wrap(err, "unknown parameter validation failed")
	}

	rerankRequest := &relaymodel.RerankRequest{}
	if err := common.UnmarshalBodyReusable(c, rerankRequest); err != nil {
		return nil, errors.Wrap(err, "unmarshal rerank request")
	}

	if err := rerankRequest.Normalize(); err != nil {
		return nil, err
	}

	if err := validator.ValidateRerankRequest(rerankRequest); err != nil {
		return nil, errors.Wrap(err, "rerank request validation failed")
	}

	return rerankRequest, nil
}

func prepareRerankRequestBody(c *gin.Context, meta *metalib.Meta, adaptorImpl adaptor.Adaptor, request *relaymodel.RerankRequest) (io.Reader, error) {
	if request == nil {
		return nil, errors.New("rerank request is nil")
	}

	if rerankAdaptor, ok := adaptorImpl.(adaptor.RerankAdaptor); ok {
		converted, err := rerankAdaptor.ConvertRerankRequest(c, request.Clone())
		if err != nil {
			return nil, errors.Wrap(err, "convert rerank request")
		}
		c.Set(ctxkey.ConvertedRequest, converted)

		payload, err := json.Marshal(converted)
		if err != nil {
			return nil, errors.Wrap(err, "marshal rerank request")
		}
		return bytes.NewBuffer(payload), nil
	}

	channelName := adaptorImpl.GetChannelName()
	if channelName == "" {
		channelName = "unknown"
	}
	return nil, errors.Errorf("rerank requests are not supported by adaptor %s", channelName)
}

func countRerankPromptTokens(ctx context.Context, request *relaymodel.RerankRequest) int {
	if request == nil {
		return 0
	}
	tokens := openai.CountTokenText(request.Query, request.Model)
	for _, doc := range request.Documents {
		tokens += openai.CountTokenText(doc, request.Model)
	}
	return tokens
}

func preConsumeRerankQuota(c *gin.Context, perCallQuota int64, meta *metalib.Meta) (int64, *relaymodel.ErrorWithStatusCode) {
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
	if err := model.CacheDecreaseUserQuota(ctx, meta.UserId, perCallQuota); err != nil {
		return perCallQuota, openai.ErrorWrapper(err, "decrease_user_quota_failed", http.StatusInternalServerError)
	}

	if userQuota > 100*perCallQuota && (tokenQuotaUnlimited || tokenQuota > 100*perCallQuota) {
		lg.Info("user has enough quota, trusted and no need to pre-consume", zap.Int("user_id", meta.UserId), zap.Int64("user_quota", userQuota))
		return 0, nil
	}

	if err := model.PreConsumeTokenQuota(ctx, meta.TokenId, perCallQuota); err != nil {
		return perCallQuota, openai.ErrorWrapper(err, "pre_consume_token_quota_failed", http.StatusForbidden)
	}

	return perCallQuota, nil
}

func postConsumeRerankQuota(ctx context.Context,
	usage *relaymodel.Usage,
	meta *metalib.Meta,
	request *relaymodel.RerankRequest,
	preConsumedQuota int64,
	totalQuota int64,
	modelRatio float64,
	groupRatio float64) (quota int64) {
	quota = max(totalQuota, 0)

	quotaDelta := quota - preConsumedQuota

	var requestId string
	if ginCtx, ok := gmw.GetGinCtxFromStdCtx(ctx); ok {
		requestId = ginCtx.GetString(ctxkey.RequestId)
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
			Content:          fmt.Sprintf("rerank per-call billing, base unit %.2f, group rate %.2f", modelRatio, groupRatio),
			IsStream:         false,
			ElapsedTime:      helper.CalcElapsedTime(meta.StartTime),
			RequestId:        requestId,
			TraceId:          traceId,
		}
		billing.PostConsumeQuotaWithLog(ctx, meta.TokenId, quotaDelta, quota, logEntry)
	} else {
		gmw.GetLogger(ctx).Error("meta information incomplete, cannot post consume rerank quota",
			zap.Int("token_id", meta.TokenId),
			zap.Int("user_id", meta.UserId),
			zap.Int("channel_id", meta.ChannelId),
			zap.String("request_id", requestId),
			zap.String("trace_id", traceId),
		)
	}

	return quota
}
