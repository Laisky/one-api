package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/metrics"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/apitype"
	"github.com/songquanpeng/one-api/relay/billing"
	"github.com/songquanpeng/one-api/relay/channeltype"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
)

func RelayTextHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	lg := gmw.GetLogger(c)
	ctx := gmw.Ctx(c)
	meta := metalib.GetByContext(c)

	// BUG: should not override meta.BaseURL and meta.ChannelId outside of metalib.GetByContext
	// meta.BaseURL = c.GetString(ctxkey.BaseURL)
	// meta.ChannelId = c.GetInt(ctxkey.ChannelId)

	// get & validate textRequest
	textRequest, err := getAndValidateTextRequest(c, meta.Mode)
	if err != nil {
		// ErrorWrapper will log the error, so we don't need to log it here
		return openai.ErrorWrapper(err, "invalid_text_request", http.StatusBadRequest)
	}
	meta.IsStream = textRequest.Stream

	// map model name
	meta.OriginModelName = textRequest.Model
	textRequest.Model = meta.ActualModelName
	meta.ActualModelName = textRequest.Model
	// set system prompt if not empty
	systemPromptReset := setSystemPrompt(ctx, textRequest, meta.ForcedSystemPrompt)

	// get channel-specific pricing if available
	var channelModelRatio map[string]float64
	var channelCompletionRatio map[string]float64
	if channelModel, ok := c.Get(ctxkey.ChannelModel); ok {
		if channel, ok := channelModel.(*model.Channel); ok {
			// Get from unified ModelConfigs only (after migration)
			channelModelRatio = channel.GetModelRatioFromConfigs()
			channelCompletionRatio = channel.GetCompletionRatioFromConfigs()
		}
	}

	// get model ratio using three-layer pricing system
	pricingAdaptor := relay.GetAdaptor(meta.ChannelType)
	modelRatio := pricing.GetModelRatioWithThreeLayers(textRequest.Model, channelModelRatio, pricingAdaptor)
	// groupRatio := billingratio.GetGroupRatio(meta.Group)
	groupRatio := c.GetFloat64(ctxkey.ChannelRatio)

	ratio := modelRatio * groupRatio
	// pre-consume quota
	promptTokens := getPromptTokens(gmw.Ctx(c), textRequest, meta.Mode)
	meta.PromptTokens = promptTokens
	preConsumedQuota, bizErr := preConsumeQuota(c, textRequest, promptTokens, ratio, meta)
	if bizErr != nil {
		lg.Warn("preConsumeQuota failed", zap.Any("error", *bizErr))
		return bizErr
	}

	adaptor := relay.GetAdaptor(meta.APIType)
	if adaptor == nil {
		return openai.ErrorWrapper(errors.Errorf("invalid api type: %d", meta.APIType), "invalid_api_type", http.StatusBadRequest)
	}
	adaptor.Init(meta)

	// get request body
	requestBody, err := getRequestBody(c, meta, textRequest, adaptor)
	if err != nil {
		// Check if this is a validation error and preserve the correct HTTP status code
		//
		// This is for AWS, which must be different from other providers that are based on proprietary systems such as OpenAI, etc.
		if strings.Contains(err.Error(), "validation failed") {
			return openai.ErrorWrapper(err, "invalid_request_error", http.StatusBadRequest)
		}
		return openai.ErrorWrapper(err, "convert_request_failed", http.StatusInternalServerError)
	}

	// for debug
	requestBodyBytes, _ := io.ReadAll(requestBody)
	requestBody = bytes.NewBuffer(requestBodyBytes)

	// do request
	resp, err := adaptor.DoRequest(c, meta, requestBody)
	if err != nil {
		// ErrorWrapper will log the error, so we don't need to log it here
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}
	if isErrorHappened(meta, resp) {
		billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, meta.TokenId)
		return RelayErrorHandler(resp)
	}

	// do response
	usage, respErr := adaptor.DoResponse(c, resp, meta)
	if respErr != nil {
		// DoResponse already logs errors internally, so we don't need to log it here
		billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, meta.TokenId)
		return respErr
	}

	// post-consume quota
	quotaId := c.GetInt(ctxkey.Id)
	requestId := c.GetString(ctxkey.RequestId)

	// Record detailed Prometheus metrics
	if usage != nil {
		// Get user information for metrics
		userId := strconv.Itoa(meta.UserId)
		username := c.GetString(ctxkey.Username)
		if username == "" {
			username = "unknown"
		}
		group := meta.Group
		if group == "" {
			group = "default"
		}

		// Record relay request metrics with actual usage
		metrics.GlobalRecorder.RecordRelayRequest(
			meta.StartTime,
			meta.ChannelId,
			channeltype.IdToName(meta.ChannelType),
			meta.ActualModelName,
			userId,
			true,
			usage.PromptTokens,
			usage.CompletionTokens,
			0, // Will be calculated in postConsumeQuota
		)

		// Record user metrics
		userBalance := float64(c.GetInt64(ctxkey.UserQuota))
		metrics.GlobalRecorder.RecordUserMetrics(
			userId,
			username,
			group,
			0, // Will be calculated in postConsumeQuota
			usage.PromptTokens,
			usage.CompletionTokens,
			userBalance,
		)

		// Record model usage metrics
		metrics.GlobalRecorder.RecordModelUsage(meta.ActualModelName, channeltype.IdToName(meta.ChannelType), time.Since(meta.StartTime))
	}

	go func() {
		// Use configurable billing timeout with model-specific adjustments
		baseBillingTimeout := time.Duration(config.BillingTimeoutSec) * time.Second
		billingTimeout := baseBillingTimeout

		ctx, cancel := context.WithTimeout(gmw.BackgroundCtx(c), billingTimeout)
		defer cancel()

		// Monitor for timeout and log critical errors
		done := make(chan bool, 1)
		var quota int64

		go func() {
			quota = postConsumeQuota(ctx, usage, meta, textRequest, ratio, preConsumedQuota, modelRatio, groupRatio, systemPromptReset, channelCompletionRatio)

			// also update user request cost
			if quota != 0 {
				docu := model.NewUserRequestCost(
					quotaId,
					requestId,
					quota,
				)
				if err = docu.Insert(); err != nil {
					lg.Error("insert user request cost failed", zap.Error(err))
				}
			}
			done <- true
		}()

		select {
		case <-done:
			// Billing completed successfully
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				estimatedQuota := float64(usage.PromptTokens+usage.CompletionTokens) * ratio
				elapsedTime := time.Since(meta.StartTime)

				lg.Error("CRITICAL BILLING TIMEOUT",
					zap.String("model", textRequest.Model),
					zap.String("requestId", requestId),
					zap.Int("userId", meta.UserId),
					zap.Int64("estimatedQuota", int64(estimatedQuota)),
					zap.Duration("elapsedTime", elapsedTime))

				// Record billing timeout in metrics
				metrics.GlobalRecorder.RecordBillingTimeout(meta.UserId, meta.ChannelId, textRequest.Model, estimatedQuota, elapsedTime)

				// TODO: Implement dead letter queue or retry mechanism for failed billing
			}
		}
	}()

	return nil
}

func getRequestBody(c *gin.Context, meta *metalib.Meta, textRequest *relaymodel.GeneralOpenAIRequest, adaptor adaptor.Adaptor) (io.Reader, error) {
	if !config.EnforceIncludeUsage &&
		meta.APIType == apitype.OpenAI &&
		meta.OriginModelName == meta.ActualModelName &&
		meta.ChannelType != channeltype.OpenAI && // openai also need to convert request
		meta.ChannelType != channeltype.Baichuan &&
		meta.ForcedSystemPrompt == "" {
		return c.Request.Body, nil
	}

	// get request body
	var requestBody io.Reader
	convertedRequest, err := adaptor.ConvertRequest(c, meta.Mode, textRequest)
	if err != nil {
		gmw.GetLogger(c).Debug("converted request failed", zap.Error(err))
		return nil, err
	}
	c.Set(ctxkey.ConvertedRequest, convertedRequest)

	jsonData, err := json.Marshal(convertedRequest)
	if err != nil {
		gmw.GetLogger(c).Debug("converted request json_marshal_failed", zap.Error(err))
		return nil, err
	}
	gmw.GetLogger(c).Debug("converted request", zap.ByteString("json", jsonData))
	requestBody = bytes.NewBuffer(jsonData)
	return requestBody, nil
}
