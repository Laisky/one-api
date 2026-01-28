package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
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
	"github.com/songquanpeng/one-api/relay/billing"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
	"github.com/songquanpeng/one-api/relay/tooling"
)

// RelayResponseAPIHelper handles Response API requests with direct pass-through
func RelayResponseAPIHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	lg := gmw.GetLogger(c)
	ctx := gmw.Ctx(c)
	meta := metalib.GetByContext(c)
	if err := logClientRequestPayload(c, "response_api"); err != nil {
		return openai.ErrorWrapper(err, "invalid_response_api_request", http.StatusBadRequest)
	}

	var channelRecord *model.Channel
	if channelModel, ok := c.Get(ctxkey.ChannelModel); ok {
		if channel, ok := channelModel.(*model.Channel); ok {
			channelRecord = channel
		}
	}

	// get & validate Response API request
	responseAPIRequest, err := getAndValidateResponseAPIRequest(c)
	if err != nil {
		return openai.ErrorWrapper(err, "invalid_response_api_request", http.StatusBadRequest)
	}
	meta.OriginModelName = responseAPIRequest.Model
	meta.ActualModelName = metalib.GetMappedModelName(meta.OriginModelName, meta.ModelMapping)
	metalib.Set2Context(c, meta)
	meta.IsStream = responseAPIRequest.Stream != nil && *responseAPIRequest.Stream
	sanitizeResponseAPIRequest(responseAPIRequest, meta.ChannelType)
	applyThinkingQueryToResponseRequest(c, responseAPIRequest, meta)
	if normalized, changed := openai.NormalizeToolChoiceForResponse(responseAPIRequest.ToolChoice); changed {
		responseAPIRequest.ToolChoice = normalized
	}

	requestAdaptor := relay.GetAdaptor(meta.APIType)
	if requestAdaptor == nil {
		return openai.ErrorWrapper(errors.New("invalid api type"), "invalid_api_type", http.StatusBadRequest)
	}

	requestedBuiltins := make(map[string]struct{})
	for _, tool := range responseAPIRequest.Tools {
		if name := tooling.NormalizeBuiltinType(tool.Type); name != "" {
			requestedBuiltins[name] = struct{}{}
		}
	}

	if hasMCP, err := hasMCPBuiltinsInResponseRequest(c, meta, channelRecord, requestAdaptor, responseAPIRequest); err != nil {
		return openai.ErrorWrapper(err, "mcp_tool_registry_failed", http.StatusBadRequest)
	} else if hasMCP {
		lg.Debug("response api request routed through chat fallback for MCP tools",
			zap.String("origin_model", meta.OriginModelName),
			zap.String("actual_model", meta.ActualModelName),
		)
		return relayResponseAPIThroughChat(c, meta, responseAPIRequest)
	}

	// duplicated
	// if reqBody, ok := c.Get(ctxkey.KeyRequestBody); ok {
	// 	lg.Debug("get response api request", zap.ByteString("body", reqBody.([]byte)))
	// }

	// Route channels without native Response API support through the ChatCompletion fallback
	if !supportsNativeResponseAPI(meta) {
		lg.Debug("response api request routed through chat fallback",
			zap.String("origin_model", meta.OriginModelName),
			zap.String("actual_model", meta.ActualModelName),
			zap.Int("channel_id", meta.ChannelId),
			zap.Int("channel_type", meta.ChannelType),
		)
		return relayResponseAPIThroughChat(c, meta, responseAPIRequest)
	}

	// Map model name for pass-through: record origin and apply mapped model
	meta.OriginModelName = responseAPIRequest.Model
	responseAPIRequest.Model = metalib.GetMappedModelName(meta.OriginModelName, meta.ModelMapping)
	meta.ActualModelName = responseAPIRequest.Model
	metalib.Set2Context(c, meta)
	c.Set(ctxkey.ConvertedRequest, responseAPIRequest)

	if pruned := tooling.PruneDisallowedResponseBuiltins(responseAPIRequest, meta, channelRecord, requestAdaptor); len(pruned) > 0 {
		for _, name := range pruned {
			delete(requestedBuiltins, name)
		}
		lg.Debug("pruned disallowed response builtins", zap.Strings("tools", pruned), zap.String("model", responseAPIRequest.Model))
	}
	if err := tooling.ValidateRequestedBuiltins(responseAPIRequest.Model, meta, channelRecord, requestAdaptor, requestedBuiltins); err != nil {
		return openai.ErrorWrapper(err, "tool_not_allowed", http.StatusBadRequest)
	}

	// get channel model ratio
	channelModelRatio, channelCompletionRatio := getChannelRatios(c)

	// get model ratio using three-layer pricing system
	pricingAdaptor := relay.GetAdaptor(meta.ChannelType)
	modelRatio := pricing.GetModelRatioWithThreeLayers(responseAPIRequest.Model, channelModelRatio, pricingAdaptor)
	completionRatio := pricing.GetCompletionRatioWithThreeLayers(responseAPIRequest.Model, channelCompletionRatio, pricingAdaptor)
	groupRatio := c.GetFloat64(ctxkey.ChannelRatio)

	ratio := modelRatio * groupRatio
	outputRatio := ratio * completionRatio
	backgroundEnabled := responseAPIRequest.Background != nil && *responseAPIRequest.Background

	// pre-consume quota based on estimated input tokens
	promptTokens := getResponseAPIPromptTokens(gmw.Ctx(c), responseAPIRequest)
	meta.PromptTokens = promptTokens
	preConsumedQuota, bizErr := preConsumeResponseAPIQuota(c, responseAPIRequest, promptTokens, ratio, outputRatio, backgroundEnabled, meta)
	if bizErr != nil {
		lg.Warn("preConsumeResponseAPIQuota failed",
			zap.Error(bizErr.RawError),
			zap.String("err_msg", bizErr.Message),
			zap.Int("status_code", bizErr.StatusCode))
		return bizErr
	}

	requestAdaptor.Init(meta)

	// get request body - for Response API, we pass through directly without conversion,
	// but ensure mapped model is used in the outgoing JSON
	requestBody, err := getResponseAPIRequestBody(c, meta, responseAPIRequest, requestAdaptor)
	if err != nil {
		return openai.ErrorWrapper(err, "convert_request_failed", http.StatusInternalServerError)
	}

	// for debug
	requestBodyBytes, _ := io.ReadAll(requestBody)
	// Attempt to log outgoing model for diagnostics without printing the entire payload
	var outgoing struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(requestBodyBytes, &outgoing)
	lg.Debug("prepared Response API upstream request",
		zap.String("origin_model", meta.OriginModelName),
		zap.String("mapped_model", meta.ActualModelName),
		zap.String("outgoing_model", outgoing.Model),
	)
	requestBody = bytes.NewBuffer(requestBodyBytes)

	// do request
	resp, err := requestAdaptor.DoRequest(c, meta, requestBody)
	if err != nil {
		// ErrorWrapper will log the error, so we don't need to log it here
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}
	upstreamCapture := wrapUpstreamResponse(resp)
	// Immediately record a provisional request cost even if pre-consume was skipped (trusted path)
	// using the estimated base quota; reconcile when usage arrives.
	{
		quotaId := c.GetInt(ctxkey.Id)
		requestId := c.GetString(ctxkey.RequestId)
		promptQuota := float64(promptTokens) * ratio
		completionQuota := 0.0
		if responseAPIRequest.MaxOutputTokens != nil {
			completionQuota = float64(*responseAPIRequest.MaxOutputTokens) * outputRatio
		}
		estimated := int64(promptQuota + completionQuota)
		if estimated <= 0 {
			estimated = preConsumedQuota
		}
		if requestId == "" {
			lg.Warn("request id missing when recording provisional user request cost",
				zap.Int("user_id", quotaId))
		} else if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, estimated); err != nil {
			lg.Warn("record provisional user request cost failed", zap.Error(err), zap.String("request_id", requestId))
		}
	}

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		graceful.GoCritical(ctx, "returnPreConsumedQuota", func(cctx context.Context) {
			billing.ReturnPreConsumedQuota(cctx, preConsumedQuota, c.GetInt(ctxkey.TokenId))
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
		logUpstreamResponseFromCapture(lg, resp, upstreamCapture, "response_api")
	} else {
		logUpstreamResponseFromBytes(lg, resp, nil, "response_api")
	}
	if respErr != nil {
		// If usage is available even though writing to client failed (e.g., client cancelled),
		// proceed to billing to ensure forwarded requests are charged; do not refund pre-consumed quota.
		// Otherwise, refund pre-consumed quota and return error.
		if usage == nil {
			graceful.GoCritical(ctx, "returnPreConsumedQuota", func(cctx context.Context) {
				billing.ReturnPreConsumedQuota(cctx, preConsumedQuota, c.GetInt(ctxkey.TokenId))
			})
			return respErr
		}
		// Fall through to billing with available usage
	}

	tooling.ApplyBuiltinToolCharges(c, &usage, meta, channelRecord, requestAdaptor)

	// post-consume quota
	quotaId := c.GetInt(ctxkey.Id)
	requestId := c.GetString(ctxkey.RequestId)

	graceful.GoCritical(gmw.BackgroundCtx(c), "postBilling", func(ctx context.Context) {
		// Use configurable billing timeout with model-specific adjustments
		baseBillingTimeout := time.Duration(config.BillingTimeoutSec) * time.Second
		billingTimeout := baseBillingTimeout

		ctx, cancel := context.WithTimeout(gmw.BackgroundCtx(c), billingTimeout)
		defer cancel()

		// Monitor for timeout and log critical errors
		done := make(chan bool, 1)
		var quota int64

		go func() {
			// Attach IDs into context using a lightweight wrapper struct in meta if needed; for now,
			// we keep postConsumeResponseAPIQuota signature and rely on it to read IDs from outer scope.
			quota = postConsumeResponseAPIQuota(ctx, usage, meta, responseAPIRequest, preConsumedQuota, modelRatio, groupRatio, channelCompletionRatio)

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
					zap.String("model", responseAPIRequest.Model),
					zap.String("requestId", requestId),
					zap.Int("userId", meta.UserId),
					zap.Int64("estimatedQuota", int64(estimatedQuota)),
					zap.Duration("elapsedTime", elapsedTime))

				// Record billing timeout in metrics
				metrics.GlobalRecorder.RecordBillingTimeout(meta.UserId, meta.ChannelId, responseAPIRequest.Model, estimatedQuota, elapsedTime)

				// TODO: Implement dead letter queue or retry mechanism for failed billing
			}
		}
	})

	return nil
}
