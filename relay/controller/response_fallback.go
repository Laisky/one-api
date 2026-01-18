package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/graceful"
	"github.com/songquanpeng/one-api/common/metrics"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/adaptor/openai_compatible"
	"github.com/songquanpeng/one-api/relay/apitype"
	"github.com/songquanpeng/one-api/relay/billing"
	"github.com/songquanpeng/one-api/relay/channeltype"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
	"github.com/songquanpeng/one-api/relay/relaymode"
	"github.com/songquanpeng/one-api/relay/tooling"
)

// relayResponseAPIThroughChat routes Response API requests through the Chat Completion fallback
func relayResponseAPIThroughChat(c *gin.Context, meta *metalib.Meta, responseAPIRequest *openai.ResponseAPIRequest) *relaymodel.ErrorWithStatusCode {
	lg := gmw.GetLogger(c)
	ctx := gmw.Ctx(c)

	chatRequest, err := openai.ConvertResponseAPIToChatCompletionRequest(responseAPIRequest)
	if err != nil {
		return openai.ErrorWrapper(err, "convert_response_api_request_failed", http.StatusBadRequest)
	}
	originalChatTools := append([]relaymodel.Tool(nil), chatRequest.Tools...)
	responseTools := responseToolsForMCP(responseAPIRequest)
	if len(responseTools) > 0 {
		chatRequest.Tools = append(chatRequest.Tools, responseTools...)
	}

	meta.Mode = relaymode.ChatCompletions
	meta.IsStream = chatRequest.Stream
	sanitizeChatCompletionRequest(chatRequest)
	meta.OriginModelName = chatRequest.Model
	chatRequest.Model = metalib.GetMappedModelName(meta.OriginModelName, meta.ModelMapping)
	meta.ActualModelName = chatRequest.Model
	if isDeepSeekModel(meta.ActualModelName) || isDeepSeekModel(meta.OriginModelName) {
		meta.APIType = apitype.DeepSeek
	}
	applyThinkingQueryToChatRequest(c, chatRequest, meta)
	meta.RequestURLPath = "/v1/chat/completions"
	meta.ResponseAPIFallback = true
	if c.Request != nil && c.Request.URL != nil {
		c.Request.URL.Path = "/v1/chat/completions"
		c.Request.URL.RawPath = "/v1/chat/completions"
	}
	metalib.Set2Context(c, meta)

	var channelRecord *model.Channel
	if channelModel, ok := c.Get(ctxkey.ChannelModel); ok {
		if channel, ok := channelModel.(*model.Channel); ok {
			channelRecord = channel
		}
	}

	requestAdaptor := relay.GetAdaptor(meta.APIType)
	if requestAdaptor == nil {
		return openai.ErrorWrapper(errors.New("invalid api type"), "invalid_api_type", http.StatusBadRequest)
	}

	registry, mcpToolNames, regErr := expandMCPBuiltinsInChatRequest(c, meta, channelRecord, requestAdaptor, chatRequest)
	if regErr != nil {
		return openai.ErrorWrapper(regErr, "mcp_tool_registry_failed", http.StatusBadRequest)
	}
	if registry == nil && len(responseTools) > 0 {
		chatRequest.Tools = originalChatTools
	}
	if registry != nil {
		responseAPIRequest.ToolChoice = normalizeMCPToolChoiceForResponse(responseAPIRequest.ToolChoice, mcpToolNames)
		chatRequest.ToolChoice = normalizeChatToolChoiceForMCP(chatRequest.ToolChoice, mcpToolNames)
		if chatRequest.Stream {
			lg.Warn("mcp tool execution forces non-streaming response")
			chatRequest.Stream = false
			meta.IsStream = false
			if responseAPIRequest.Stream != nil {
				stream := false
				responseAPIRequest.Stream = &stream
			}
		}
	}

	origWriter := c.Writer
	var capture *responseCaptureWriter
	if !meta.IsStream {
		capture = newResponseCaptureWriter(origWriter)
		c.Writer = capture
		defer func() {
			c.Writer = origWriter
		}()
	}

	c.Set(ctxkey.ResponseAPIRequestOriginal, responseAPIRequest)
	if chatRequest.Stream {
		c.Set(ctxkey.ResponseStreamRewriteHandler, newChatToResponseStreamBridge(c, meta, responseAPIRequest))
	} else {
		c.Set(ctxkey.ResponseRewriteHandler, func(gc *gin.Context, status int, textResp *openai_compatible.SlimTextResponse) error {
			if capture != nil {
				prevWriter := gc.Writer
				gc.Writer = origWriter
				defer func() {
					gc.Writer = prevWriter
				}()
			}
			return renderChatResponseAsResponseAPI(gc, status, textResp, responseAPIRequest, meta)
		})
	}

	if err := tooling.ValidateResponseBuiltinToolsWithExclusions(responseAPIRequest, meta, channelRecord, requestAdaptor, mcpToolNames); err != nil {
		return openai.ErrorWrapper(err, "tool_not_allowed", http.StatusBadRequest)
	}
	if err := tooling.ValidateChatBuiltinTools(c, chatRequest, meta, channelRecord, requestAdaptor); err != nil {
		return openai.ErrorWrapper(err, "tool_not_allowed", http.StatusBadRequest)
	}

	channelModelRatio, channelCompletionRatio := getChannelRatios(c)
	pricingAdaptor := relay.GetAdaptor(meta.ChannelType)
	modelRatio := pricing.GetModelRatioWithThreeLayers(chatRequest.Model, channelModelRatio, pricingAdaptor)
	groupRatio := c.GetFloat64(ctxkey.ChannelRatio)
	ratio := modelRatio * groupRatio

	promptTokens := getPromptTokens(gmw.Ctx(c), chatRequest, meta.Mode)
	meta.PromptTokens = promptTokens
	preConsumedQuota, bizErr := preConsumeQuota(c, chatRequest, promptTokens, ratio, meta)
	if bizErr != nil {
		lg.Warn("preConsumeQuota failed",
			zap.Error(bizErr.RawError),
			zap.String("err_msg", bizErr.Message),
			zap.Int("status_code", bizErr.StatusCode))
		return bizErr
	}

	requestAdaptor.Init(meta)
	if registry != nil {
		c.Set(ctxkey.ResponseRewriteHandler, nil)
		c.Set(ctxkey.ResponseStreamRewriteHandler, nil)
		response, usage, mcpSummary, incrementalCharged, execErr := executeChatMCPToolLoop(c, meta, chatRequest, registry, preConsumedQuota)
		if execErr != nil {
			billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, meta.TokenId)
			return execErr
		}
		tooling.ApplyBuiltinToolCharges(c, &usage, meta, channelRecord, requestAdaptor)
		if mcpSummary != nil && mcpSummary.summary != nil {
			var existing *model.ToolUsageSummary
			if raw, ok := c.Get(ctxkey.ToolInvocationSummary); ok {
				if summary, ok := raw.(*model.ToolUsageSummary); ok {
					existing = summary
				}
			}
			merged := mergeToolUsageSummaries(existing, mcpSummary.summary)
			c.Set(ctxkey.ToolInvocationSummary, merged)
		}

		choices := make([]openai_compatible.TextResponseChoice, 0, len(response.Choices))
		for _, choice := range response.Choices {
			choices = append(choices, openai_compatible.TextResponseChoice{
				Index:        choice.Index,
				Message:      choice.Message,
				FinishReason: choice.FinishReason,
			})
		}
		if capture != nil {
			prevWriter := c.Writer
			c.Writer = origWriter
			defer func() {
				c.Writer = prevWriter
			}()
		}
		if err := renderChatResponseAsResponseAPI(c, http.StatusOK, &openai_compatible.SlimTextResponse{Choices: choices, Usage: response.Usage}, responseAPIRequest, meta); err != nil {
			billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, meta.TokenId)
			return openai.ErrorWrapper(err, "response_rewrite_failed", http.StatusInternalServerError)
		}

		// refund pre-consumed quota immediately before final billing reconciliation
		billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, meta.TokenId)

		if usage != nil {
			userId := strconv.Itoa(meta.UserId)
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
				userId,
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
				userId,
				username,
				group,
				0,
				usage.PromptTokens,
				usage.CompletionTokens,
				userBalance,
			)

			metrics.GlobalRecorder.RecordModelUsage(meta.ActualModelName, channeltype.IdToName(meta.ChannelType), time.Since(meta.StartTime))
		}

		quotaId := c.GetInt(ctxkey.Id)
		requestId := c.GetString(ctxkey.RequestId)
		graceful.GoCritical(gmw.BackgroundCtx(c), "postBilling", func(ctx context.Context) {
			baseBillingTimeout := time.Duration(config.BillingTimeoutSec) * time.Second
			billingTimeout := baseBillingTimeout

			ctx, cancel := context.WithTimeout(gmw.BackgroundCtx(c), billingTimeout)
			defer cancel()

			done := make(chan bool, 1)
			var quota int64

			go func() {
				quota = postConsumeQuota(ctx, usage, meta, chatRequest, ratio, preConsumedQuota, incrementalCharged, modelRatio, groupRatio, false, channelCompletionRatio)
				if requestId != "" {
					if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, quota); err != nil {
						lg.Error("update user request cost failed", zap.Error(err), zap.String("request_id", requestId))
					}
				}
				done <- true
			}()

			select {
			case <-done:
			case <-ctx.Done():
				if ctx.Err() == context.DeadlineExceeded && usage != nil {
					estimatedQuota := float64(usage.PromptTokens+usage.CompletionTokens) * ratio
					elapsedTime := time.Since(meta.StartTime)
					lg.Error("CRITICAL BILLING TIMEOUT",
						zap.String("model", chatRequest.Model),
						zap.String("requestId", requestId),
						zap.Int("userId", meta.UserId),
						zap.Int64("estimatedQuota", int64(estimatedQuota)),
						zap.Duration("elapsedTime", elapsedTime))
					metrics.GlobalRecorder.RecordBillingTimeout(meta.UserId, meta.ChannelId, chatRequest.Model, estimatedQuota, elapsedTime)
				}
			}
		})
		return nil
	}

	convertedRequest, err := requestAdaptor.ConvertRequest(c, relaymode.ChatCompletions, chatRequest)
	if err != nil {
		billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, meta.TokenId)
		return openai.ErrorWrapper(err, "convert_request_failed", http.StatusInternalServerError)
	}
	c.Set(ctxkey.ConvertedRequest, convertedRequest)

	jsonData, err := json.Marshal(convertedRequest)
	if err != nil {
		billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, meta.TokenId)
		return openai.ErrorWrapper(err, "marshal_converted_request_failed", http.StatusInternalServerError)
	}
	requestBody := bytes.NewBuffer(jsonData)

	resp, err := requestAdaptor.DoRequest(c, meta, requestBody)
	if err != nil {
		billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, meta.TokenId)
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}
	upstreamCapture := wrapUpstreamResponse(resp)

	// Record provisional quota usage for reconciliation
	if requestId := c.GetString(ctxkey.RequestId); requestId != "" {
		quotaId := c.GetInt(ctxkey.Id)
		estimated := getPreConsumedQuota(chatRequest, promptTokens, ratio)
		if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, estimated); err != nil {
			lg.Warn("record provisional user request cost failed", zap.Error(err), zap.String("request_id", requestId))
		}
	}

	if isErrorHappened(meta, resp) {
		graceful.GoCritical(ctx, "returnPreConsumedQuota", func(cctx context.Context) {
			billing.ReturnPreConsumedQuota(cctx, preConsumedQuota, meta.TokenId)
		})
		return RelayErrorHandlerWithContext(c, resp)
	}

	usage, respErr := requestAdaptor.DoResponse(c, resp, meta)
	if upstreamCapture != nil {
		logUpstreamResponseFromCapture(lg, resp, upstreamCapture, "response_api_fallback")
	} else {
		logUpstreamResponseFromBytes(lg, resp, nil, "response_api_fallback")
	}
	if respErr != nil {
		if usage == nil {
			graceful.GoCritical(ctx, "returnPreConsumedQuota", func(cctx context.Context) {
				billing.ReturnPreConsumedQuota(cctx, preConsumedQuota, meta.TokenId)
			})
			return respErr
		}
	}

	tooling.ApplyBuiltinToolCharges(c, &usage, meta, channelRecord, requestAdaptor)

	if respErr == nil && capture != nil {
		c.Writer = origWriter
		if !c.GetBool(ctxkey.ResponseRewriteApplied) {
			body := capture.BodyBytes()
			statusCode := capture.StatusCode()
			if len(body) > 0 {
				var slim openai_compatible.SlimTextResponse
				if err := json.Unmarshal(body, &slim); err == nil && len(slim.Choices) > 0 {
					if err := renderChatResponseAsResponseAPI(c, statusCode, &slim, responseAPIRequest, meta); err != nil {
						billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, meta.TokenId)
						return openai.ErrorWrapper(err, "response_rewrite_failed", http.StatusInternalServerError)
					}
				} else {
					if statusCode > 0 {
						c.Writer.WriteHeader(statusCode)
					}
					if len(body) > 0 {
						if _, err := c.Writer.Write(body); err != nil {
							billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, meta.TokenId)
							return openai.ErrorWrapper(err, "write_response_body_failed", http.StatusInternalServerError)
						}
					}
					c.Set(ctxkey.ResponseRewriteApplied, true)
				}
			} else if capture.HeaderWritten() {
				if statusCode > 0 {
					c.Writer.WriteHeader(statusCode)
				}
				c.Set(ctxkey.ResponseRewriteApplied, true)
			}
		}
	}

	// Refund pre-consumed quota immediately before final billing reconciliation
	billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, meta.TokenId)

	if usage != nil {
		userId := strconv.Itoa(meta.UserId)
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
			userId,
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
			userId,
			username,
			group,
			0,
			usage.PromptTokens,
			usage.CompletionTokens,
			userBalance,
		)

		metrics.GlobalRecorder.RecordModelUsage(meta.ActualModelName, channeltype.IdToName(meta.ChannelType), time.Since(meta.StartTime))
	}

	quotaId := c.GetInt(ctxkey.Id)
	requestId := c.GetString(ctxkey.RequestId)

	graceful.GoCritical(gmw.BackgroundCtx(c), "postBilling", func(ctx context.Context) {
		baseBillingTimeout := time.Duration(config.BillingTimeoutSec) * time.Second
		billingTimeout := baseBillingTimeout

		ctx, cancel := context.WithTimeout(gmw.BackgroundCtx(c), billingTimeout)
		defer cancel()

		done := make(chan bool, 1)
		var quota int64

		go func() {
			quota = postConsumeQuota(ctx, usage, meta, chatRequest, ratio, preConsumedQuota, 0, modelRatio, groupRatio, false, channelCompletionRatio)
			if requestId != "" {
				if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, quota); err != nil {
					lg.Error("update user request cost failed", zap.Error(err), zap.String("request_id", requestId))
				}
			}
			done <- true
		}()

		select {
		case <-done:
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded && usage != nil {
				estimatedQuota := float64(usage.PromptTokens+usage.CompletionTokens) * ratio
				elapsedTime := time.Since(meta.StartTime)
				lg.Error("CRITICAL BILLING TIMEOUT",
					zap.String("model", chatRequest.Model),
					zap.String("requestId", requestId),
					zap.Int("userId", meta.UserId),
					zap.Int64("estimatedQuota", int64(estimatedQuota)),
					zap.Duration("elapsedTime", elapsedTime))
				metrics.GlobalRecorder.RecordBillingTimeout(meta.UserId, meta.ChannelId, chatRequest.Model, estimatedQuota, elapsedTime)
			}
		}
	})

	return nil
}
