// Package controller is a package for handling the relay controller
package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/tracing"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// RelayProxyHelper is a helper function to proxy the request to the upstream service
func RelayProxyHelper(c *gin.Context, relayMode int) *relaymodel.ErrorWithStatusCode {
	meta := metalib.GetByContext(c)
	if err := logClientRequestPayload(c, "proxy"); err != nil {
		return openai.ErrorWrapper(err, "invalid_proxy_request", http.StatusBadRequest)
	}

	adaptor := relay.GetAdaptor(meta.APIType)
	if adaptor == nil {
		return openai.ErrorWrapper(errors.Errorf("invalid api type: %d", meta.APIType), "invalid_api_type", http.StatusBadRequest)
	}
	adaptor.Init(meta)

	resp, err := adaptor.DoRequest(c, meta, c.Request.Body)
	if err != nil {
		// ErrorWrapper already logs the error, so we don't need to log it here
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}

	// do response
	usage, respErr := adaptor.DoResponse(c, resp, meta)
	if respErr != nil {
		// respErr is already a structured error, no need to log it here
		return respErr
	}

	// log proxy request with zero quota
	quotaId := c.GetInt(ctxkey.Id)
	requestId := c.GetString(ctxkey.RequestId)
	// Capture trace ID before launching goroutine
	traceId := tracing.GetTraceID(c)
	promptTokens, completionTokens := proxyTokenSummary(c, meta, usage)
	userId := meta.UserId
	channelId := meta.ChannelId
	tokenName := meta.TokenName
	isStream := meta.IsStream
	modelName := "proxy"
	elapsed := helper.CalcElapsedTime(meta.StartTime)
	go func() {
		ctx, cancel := context.WithTimeout(gmw.BackgroundCtx(c), 30*time.Second)
		defer cancel()

		// Log the proxy request with zero quota
		model.RecordConsumeLog(ctx, &model.Log{
			UserId:           userId,
			ChannelId:        channelId,
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			ModelName:        modelName,
			TokenName:        tokenName,
			Quota:            0,
			Content:          "proxy request, no quota consumption",
			IsStream:         isStream,
			ElapsedTime:      elapsed,
			TraceId:          traceId,
			RequestId:        requestId,
		})
		model.UpdateUserUsedQuotaAndRequestCount(userId, 0)
		model.UpdateChannelUsedQuota(channelId, 0)

		// Reconcile user request cost (proxy does not consume quota)
		if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, 0); err != nil {
			gmw.GetLogger(ctx).Error("update user request cost failed", zap.Error(err))
		}
	}()

	return nil
}

func proxyTokenSummary(c *gin.Context, meta *metalib.Meta, usage *relaymodel.Usage) (promptTokens int, completionTokens int) {
	if usage == nil {
		if lg := gmw.GetLogger(c); lg != nil {
			lg.Debug("proxy adaptor returned no usage payload; defaulting to zero tokens",
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path),
				zap.Int("channel_id", meta.ChannelId))
		}
		return 0, 0
	}
	return usage.PromptTokens, usage.CompletionTokens
}
