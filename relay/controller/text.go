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
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/graceful"
	"github.com/songquanpeng/one-api/common/metrics"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/common/structuredjson"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/apitype"
	"github.com/songquanpeng/one-api/relay/billing"
	"github.com/songquanpeng/one-api/relay/channeltype"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
	"github.com/songquanpeng/one-api/relay/relaymode"
	"github.com/songquanpeng/one-api/relay/streaming"
	"github.com/songquanpeng/one-api/relay/tooling"
)

func RelayTextHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	lg := gmw.GetLogger(c)
	ctx := gmw.Ctx(c)
	meta := metalib.GetByContext(c)
	if err := logClientRequestPayload(c, "chat_completions"); err != nil {
		return openai.ErrorWrapper(err, "invalid_text_request", http.StatusBadRequest)
	}

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
	applyThinkingQueryToChatRequest(c, textRequest, meta)
	// set system prompt if not empty
	systemPromptReset := setSystemPrompt(ctx, textRequest, meta.ForcedSystemPrompt)

	// get channel-specific pricing if available
	var channelRecord *model.Channel
	var channelModelRatio map[string]float64
	var channelCompletionRatio map[string]float64
	if channelModel, ok := c.Get(ctxkey.ChannelModel); ok {
		if channel, ok := channelModel.(*model.Channel); ok {
			channelRecord = channel
			// Get from unified ModelConfigs only (after migration)
			channelModelRatio = channel.GetModelRatioFromConfigs()
			channelCompletionRatio = channel.GetCompletionRatioFromConfigs()
		}
	}

	requestAdaptor := relay.GetAdaptor(meta.APIType)
	if requestAdaptor == nil {
		return openai.ErrorWrapper(errors.Errorf("invalid api type: %d", meta.APIType), "invalid_api_type", http.StatusBadRequest)
	}

	registry, mcpToolNames, regErr := expandMCPBuiltinsInChatRequest(c, meta, channelRecord, requestAdaptor, textRequest)
	if regErr != nil {
		return openai.ErrorWrapper(regErr, "mcp_tool_registry_failed", http.StatusBadRequest)
	}
	if registry != nil {
		textRequest.ToolChoice = normalizeChatToolChoiceForMCP(textRequest.ToolChoice, mcpToolNames)
		if textRequest.Stream {
			lg.Warn("mcp tool execution forces non-streaming response")
			textRequest.Stream = false
			meta.IsStream = false
		}
	}

	// get model ratio using three-layer pricing system
	pricingAdaptor := resolvePricingAdaptor(meta)
	modelRatio := pricing.GetModelRatioWithThreeLayers(textRequest.Model, channelModelRatio, pricingAdaptor)
	completionRatio := pricing.GetCompletionRatioWithThreeLayers(textRequest.Model, channelCompletionRatio, pricingAdaptor)
	// groupRatio := billingratio.GetGroupRatio(meta.Group)
	groupRatio := c.GetFloat64(ctxkey.ChannelRatio)

	ratio := modelRatio * groupRatio
	if err := tooling.ValidateChatBuiltinTools(c, textRequest, meta, channelRecord, requestAdaptor); err != nil {
		return openai.ErrorWrapper(err, "tool_not_allowed", http.StatusBadRequest)
	}

	// pre-consume quota
	promptTokens := getPromptTokens(gmw.Ctx(c), textRequest, meta.Mode)
	meta.PromptTokens = promptTokens
	preConsumedQuota, bizErr := preConsumeQuota(c, textRequest, promptTokens, ratio, completionRatio, meta)
	if bizErr != nil {
		lg.Warn("preConsumeQuota failed",
			zap.Error(bizErr.RawError),
			zap.Int("status_code", bizErr.StatusCode),
			zap.String("err_msg", bizErr.Message))
		return bizErr
	}

	var tracker *streaming.QuotaTracker
	if textRequest.Stream {
		tracker = streaming.NewQuotaTracker(streaming.QuotaTrackerParams{
			UserID:                 meta.UserId,
			TokenID:                meta.TokenId,
			ChannelID:              meta.ChannelId,
			ModelName:              textRequest.Model,
			PromptTokens:           promptTokens,
			ModelRatio:             modelRatio,
			GroupRatio:             groupRatio,
			PreConsumedQuota:       preConsumedQuota,
			ChannelCompletionRatio: channelCompletionRatio,
			PricingAdaptor:         pricingAdaptor,
			FlushInterval:          time.Duration(config.StreamingBillingIntervalSec) * time.Second,
			Ctx:                    gmw.Ctx(c),
		})
		streaming.StoreTracker(c, tracker)
	}

	requestAdaptor.Init(meta)
	if registry != nil {
		response, usage, mcpSummary, incrementalCharged, execErr := executeChatMCPToolLoop(c, meta, textRequest, registry, preConsumedQuota)
		if execErr != nil {
			billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, meta.TokenId)
			return execErr
		}
		applyOutputImageCharges(c, &usage, meta)
		applyOutputAudioCharges(c, &usage, meta)
		applyOutputVideoCharges(c, &usage, meta)
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

		c.JSON(http.StatusOK, response)

		// refund pre-consumed quota immediately
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
				quota = postConsumeQuota(ctx, usage, meta, textRequest, ratio, preConsumedQuota, incrementalCharged, modelRatio, groupRatio, systemPromptReset, channelCompletionRatio)
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
						zap.String("model", textRequest.Model),
						zap.String("requestId", requestId),
						zap.Int("userId", meta.UserId),
						zap.Int64("estimatedQuota", int64(estimatedQuota)),
						zap.Duration("elapsedTime", elapsedTime))

					metrics.GlobalRecorder.RecordBillingTimeout(meta.UserId, meta.ChannelId, textRequest.Model, estimatedQuota, elapsedTime)
				}
			}
		})
		return nil
	}

	// Downgrade structured JSON schema for providers that reject response_format
	if requiresJSONSchemaDowngrade(meta, textRequest) {
		structuredjson.EnsureInstruction(textRequest)
		textRequest.ResponseFormat = nil
	}

	// get request body
	requestBody, err := getRequestBody(c, meta, textRequest, requestAdaptor, systemPromptReset)
	if err != nil {
		return wrapConvertRequestError(err)
	}

	// for debug
	requestBodyBytes, _ := io.ReadAll(requestBody)
	requestBody = bytes.NewBuffer(requestBodyBytes)

	// do request
	resp, err := requestAdaptor.DoRequest(c, meta, requestBody)
	if err != nil {
		// ErrorWrapper will log the error, so we don't need to log it here
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}
	upstreamCapture := wrapUpstreamResponse(resp)
	// Immediately record a provisional request cost using the estimated base quota
	// even if we decided not to pre-consume physically (trusted user/token path).
	// This ensures user-cancelled requests are still tracked and later reconciled.
	{
		quotaId := c.GetInt(ctxkey.Id)
		requestId := c.GetString(ctxkey.RequestId)
		estimated := getPreConsumedQuota(textRequest, promptTokens, ratio, completionRatio)
		if requestId == "" {
			lg.Warn("request id missing when recording provisional user request cost",
				zap.Int("user_id", quotaId))
		} else if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, estimated); err != nil {
			lg.Warn("record provisional user request cost failed", zap.Error(err), zap.String("request_id", requestId))
		}
	}
	if isErrorHappened(meta, resp) {
		// refund pre-consumed quota under lifecycle management so shutdown waits for it
		graceful.GoCritical(ctx, "returnPreConsumedQuota", func(cctx context.Context) {
			billing.ReturnPreConsumedQuota(cctx, preConsumedQuota, meta.TokenId)
		})
		// Reconcile provisional record to 0 since upstream returned error
		quotaId := c.GetInt(ctxkey.Id)
		requestId := c.GetString(ctxkey.RequestId)
		if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, 0); err != nil {
			lg.Warn("update user request cost to zero failed", zap.Error(err))
		}
		return RelayErrorHandlerWithContext(c, resp)
	}

	// do response
	c.Set(ctxkey.SkipAdaptorResponseBodyLog, true)
	usage, respErr := requestAdaptor.DoResponse(c, resp, meta)
	if upstreamCapture != nil {
		logUpstreamResponseFromCapture(lg, resp, upstreamCapture, "chat_completions")
	} else {
		logUpstreamResponseFromBytes(lg, resp, nil, "chat_completions")
	}
	if respErr != nil {
		// If usage is available even though writing to client failed (e.g., client cancelled),
		// proceed to billing to ensure forwarded requests are charged; do not refund pre-consumed quota.
		// Otherwise, refund pre-consumed quota and return error.
		if usage == nil {
			billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, meta.TokenId)
			return respErr
		}
		// Fall through to billing with available usage
	}

	var incrementalCharged int64
	if tracker != nil {
		var trackerErr error
		usage, incrementalCharged, trackerErr = tracker.Finalize(usage)
		if trackerErr != nil {
			if errors.Is(trackerErr, streaming.ErrQuotaExceeded) {
				billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, meta.TokenId)
				return openai.ErrorWrapper(errors.New("user quota is not enough"), "insufficient_user_quota", http.StatusForbidden)
			}
			billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, meta.TokenId)
			return openai.ErrorWrapper(trackerErr, "streaming_billing_failed", http.StatusInternalServerError)
		}
	}

	applyOutputImageCharges(c, &usage, meta)
	applyOutputAudioCharges(c, &usage, meta)
	applyOutputVideoCharges(c, &usage, meta)
	tooling.ApplyBuiltinToolCharges(c, &usage, meta, channelRecord, requestAdaptor)

	// post-consume quota
	quotaId := c.GetInt(ctxkey.Id)
	// refund pre-consumed quota immediately
	billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, meta.TokenId)
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

	graceful.GoCritical(gmw.BackgroundCtx(c), "postBilling", func(ctx context.Context) {
		// Use configurable billing timeout with model-specific adjustments
		baseBillingTimeout := time.Duration(config.BillingTimeoutSec) * time.Second
		billingTimeout := baseBillingTimeout

		ctx, cancel := context.WithTimeout(gmw.BackgroundCtx(c), billingTimeout)
		defer cancel()

		// Monitor for timeout and log critical errors
		done := make(chan bool, 1)
		var quota int64

		requestId := c.GetString(ctxkey.RequestId)
		go func() {
			quota = postConsumeQuota(ctx, usage, meta, textRequest, ratio, preConsumedQuota, incrementalCharged, modelRatio, groupRatio, systemPromptReset, channelCompletionRatio)

			// Reconcile request cost with final quota (override provisional pre-consumed value)
			if requestId == "" {
				lg.Warn("request id missing when finalizing user request cost",
					zap.Int("user_id", quotaId))
			} else if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, quota); err != nil {
				lg.Error("update user request cost failed", zap.Error(err), zap.String("request_id", requestId))
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

				metrics.GlobalRecorder.RecordBillingTimeout(meta.UserId, meta.ChannelId, textRequest.Model, estimatedQuota, elapsedTime)
				// TODO: Implement dead letter queue or retry mechanism for failed billing
			}
		}
	})

	return nil
}

func getRequestBody(c *gin.Context, meta *metalib.Meta, textRequest *relaymodel.GeneralOpenAIRequest, adaptor adaptor.Adaptor, systemPromptReset bool) (io.Reader, error) {
	originalBody, err := common.GetRequestBody(c)
	if err != nil {
		return nil, errors.Wrap(err, "get raw request body")
	}

	if textRequest.ResponseFormat == nil &&
		!config.EnforceIncludeUsage &&
		meta.APIType == apitype.OpenAI &&
		meta.OriginModelName == meta.ActualModelName &&
		meta.ChannelType != channeltype.OpenAI &&
		meta.ChannelType != channeltype.Baichuan &&
		meta.ForcedSystemPrompt == "" {
		c.Set(ctxkey.ConvertedRequest, textRequest)
		return bytes.NewBuffer(originalBody), nil
	}

	convertedRequest, err := adaptor.ConvertRequest(c, meta.Mode, textRequest)
	if err != nil {
		return nil, errors.Wrap(err, "convert request failed")
	}
	c.Set(ctxkey.ConvertedRequest, convertedRequest)

	jsonData, err := json.Marshal(convertedRequest)
	if err != nil {
		return nil, errors.Wrap(err, "marshal converted request failed")
	}

	// When upstream expects the native OpenAI chat payload and we didn't rewrite the
	// system prompt, merge unknown user fields (e.g. encrypted extensions) back into
	// the converted JSON so we can preserve pass-through semantics.
	if _, isChatPayload := convertedRequest.(*relaymodel.GeneralOpenAIRequest); isChatPayload &&
		meta.APIType == apitype.OpenAI &&
		meta.ChannelType == channeltype.OpenAI &&
		!config.EnforceIncludeUsage &&
		!systemPromptReset {
		if merged, mergeErr := mergeJSONPreservingUnknown(originalBody, jsonData); mergeErr == nil {
			jsonData = merged
		} else {
			return nil, errors.Wrap(mergeErr, "merge original request fields")
		}
	}

	lg := gmw.GetLogger(c)
	lg.Debug("converted request", zap.ByteString("json", jsonData))
	return bytes.NewBuffer(jsonData), nil
}

func requiresJSONSchemaDowngrade(meta *metalib.Meta, request *relaymodel.GeneralOpenAIRequest) bool {
	if meta == nil || request == nil {
		return false
	}
	if request.ResponseFormat == nil || request.ResponseFormat.JsonSchema == nil {
		return false
	}

	modelName := strings.ToLower(strings.TrimSpace(request.Model))
	if strings.HasPrefix(modelName, "deepseek") {
		return true
	}

	baseURL := strings.ToLower(strings.TrimSpace(meta.BaseURL))
	if strings.Contains(baseURL, "deepseek.com") {
		return true
	}

	if meta.ChannelType == channeltype.DeepSeek {
		return true
	}

	return false
}

func mergeJSONPreservingUnknown(original, updated []byte) ([]byte, error) {
	if len(original) == 0 {
		return updated, nil
	}

	var originalMap map[string]json.RawMessage
	if err := json.Unmarshal(original, &originalMap); err != nil {
		return nil, err
	}

	var updatedMap map[string]json.RawMessage
	if err := json.Unmarshal(updated, &updatedMap); err != nil {
		return nil, err
	}

	for key, value := range originalMap {
		if _, exists := updatedMap[key]; !exists {
			updatedMap[key] = value
		}
	}

	merged, err := json.Marshal(updatedMap)
	if err != nil {
		return nil, err
	}
	return merged, nil
}
