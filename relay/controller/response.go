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
	"github.com/songquanpeng/one-api/common/tracing"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/adaptor/openai_compatible"
	"github.com/songquanpeng/one-api/relay/apitype"
	"github.com/songquanpeng/one-api/relay/billing"
	"github.com/songquanpeng/one-api/relay/channeltype"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
	quotautil "github.com/songquanpeng/one-api/relay/quota"
	"github.com/songquanpeng/one-api/relay/relaymode"
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

	requestedBuiltins := make(map[string]struct{})
	for _, tool := range responseAPIRequest.Tools {
		if name := tooling.NormalizeBuiltinType(tool.Type); name != "" {
			requestedBuiltins[name] = struct{}{}
		}
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

	requestAdaptor := relay.GetAdaptor(meta.APIType)
	if requestAdaptor == nil {
		return openai.ErrorWrapper(errors.New("invalid api type"), "invalid_api_type", http.StatusBadRequest)
	}
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
		estimatedTokens := int64(promptTokens)
		if responseAPIRequest.MaxOutputTokens != nil {
			estimatedTokens += int64(*responseAPIRequest.MaxOutputTokens)
		}
		estimated := int64(float64(estimatedTokens) * ratio)
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

func relayResponseAPIThroughChat(c *gin.Context, meta *metalib.Meta, responseAPIRequest *openai.ResponseAPIRequest) *relaymodel.ErrorWithStatusCode {
	lg := gmw.GetLogger(c)
	ctx := gmw.Ctx(c)

	chatRequest, err := openai.ConvertResponseAPIToChatCompletionRequest(responseAPIRequest)
	if err != nil {
		return openai.ErrorWrapper(err, "convert_response_api_request_failed", http.StatusBadRequest)
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
	if err := tooling.ValidateResponseBuiltinTools(responseAPIRequest, meta, channelRecord, requestAdaptor); err != nil {
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

		metrics.GlobalRecorder.RecordRelayRequest(
			meta.StartTime,
			meta.ChannelId,
			channeltype.IdToName(meta.ChannelType),
			meta.ActualModelName,
			userId,
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

func renderChatResponseAsResponseAPI(c *gin.Context, status int, textResp *openai_compatible.SlimTextResponse, originalReq *openai.ResponseAPIRequest, meta *metalib.Meta) error {
	c.Set(ctxkey.ResponseRewriteApplied, true)
	responseID := generateResponseAPIID(c, textResp)
	statusText, incomplete := deriveResponseStatus(textResp.Choices)
	usage := (&openai.ResponseAPIUsage{}).FromModelUsage(&textResp.Usage)
	output := buildResponseOutput(textResp.Choices)
	toolCalls := buildRequiredActionToolCalls(textResp.Choices)

	response := openai.ResponseAPIResponse{
		Id:                 responseID,
		Object:             "response",
		CreatedAt:          time.Now().Unix(),
		Status:             statusText,
		Model:              meta.ActualModelName,
		Output:             output,
		Usage:              usage,
		Instructions:       originalReq.Instructions,
		MaxOutputTokens:    originalReq.MaxOutputTokens,
		Metadata:           originalReq.Metadata,
		ParallelToolCalls:  originalReq.ParallelToolCalls != nil && *originalReq.ParallelToolCalls,
		PreviousResponseId: originalReq.PreviousResponseId,
		Reasoning:          originalReq.Reasoning,
		ServiceTier:        originalReq.ServiceTier,
		Temperature:        originalReq.Temperature,
		Text:               originalReq.Text,
		ToolChoice:         originalReq.ToolChoice,
		Tools:              convertResponseAPITools(originalReq.Tools),
		TopP:               originalReq.TopP,
		Truncation:         originalReq.Truncation,
		User:               originalReq.User,
	}

	if len(toolCalls) > 0 {
		response.RequiredAction = &openai.ResponseAPIRequiredAction{
			Type: "submit_tool_outputs",
			SubmitToolOutputs: &openai.ResponseAPISubmitToolOutputs{
				ToolCalls: toolCalls,
			},
		}
	}

	if incomplete != nil {
		response.IncompleteDetails = incomplete
	}

	data, err := json.Marshal(response)
	if err != nil {
		return err
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(status)
	_, err = c.Writer.Write(data)
	return err
}

type responseCaptureWriter struct {
	gin.ResponseWriter
	body        bytes.Buffer
	status      int
	wroteHeader bool
}

func newResponseCaptureWriter(w gin.ResponseWriter) *responseCaptureWriter {
	return &responseCaptureWriter{ResponseWriter: w}
}

func (w *responseCaptureWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.body.Write(data)
}

func (w *responseCaptureWriter) WriteString(s string) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.body.WriteString(s)
}

func (w *responseCaptureWriter) WriteHeader(code int) {
	w.status = code
	w.wroteHeader = true
}

func (w *responseCaptureWriter) WriteHeaderNow() {
	if !w.wroteHeader {
		w.status = w.ResponseWriter.Status()
		if w.status == 0 {
			w.status = http.StatusOK
		}
		w.wroteHeader = true
	}
}

func (w *responseCaptureWriter) StatusCode() int {
	if w.status > 0 {
		return w.status
	}
	if code := w.ResponseWriter.Status(); code > 0 {
		return code
	}
	return http.StatusOK
}

func (w *responseCaptureWriter) BodyBytes() []byte {
	return w.body.Bytes()
}

func (w *responseCaptureWriter) HeaderWritten() bool {
	return w.wroteHeader
}

func (w *responseCaptureWriter) Written() bool {
	if w.wroteHeader || w.body.Len() > 0 {
		return true
	}
	return w.ResponseWriter.Written()
}

func (w *responseCaptureWriter) Size() int {
	if w.body.Len() > 0 {
		return w.body.Len()
	}
	return w.ResponseWriter.Size()
}

func generateResponseAPIID(c *gin.Context, _ *openai_compatible.SlimTextResponse) string {
	if reqID := c.GetString(ctxkey.RequestId); reqID != "" {
		return fmt.Sprintf("resp-%s", reqID)
	}
	return fmt.Sprintf("resp-%d", time.Now().UnixNano())
}

func deriveResponseStatus(choices []openai_compatible.TextResponseChoice) (string, *openai.IncompleteDetails) {
	status := "completed"
	for _, choice := range choices {
		switch choice.FinishReason {
		case "length":
			return "incomplete", &openai.IncompleteDetails{Reason: "max_output_tokens"}
		case "content_filter":
			return "incomplete", &openai.IncompleteDetails{Reason: "content_filter"}
		case "cancelled":
			status = "cancelled"
		}
	}
	return status, nil
}

func buildResponseOutput(choices []openai_compatible.TextResponseChoice) []openai.OutputItem {
	var output []openai.OutputItem
	for _, choice := range choices {
		msg := choice.Message
		contents := convertMessageContent(msg)
		if len(contents) > 0 {
			output = append(output, openai.OutputItem{
				Type:    "message",
				Role:    "assistant",
				Status:  "completed",
				Content: contents,
			})
		}

		if reasoning := extractReasoning(msg); reasoning != "" {
			output = append(output, openai.OutputItem{
				Type:   "reasoning",
				Status: "completed",
				Summary: []openai.OutputContent{
					{Type: "summary_text", Text: reasoning},
				},
			})
		}

		for _, tool := range msg.ToolCalls {
			arguments := ""
			if tool.Function != nil && tool.Function.Arguments != nil {
				switch v := tool.Function.Arguments.(type) {
				case string:
					arguments = v
				default:
					if b, err := json.Marshal(v); err == nil {
						arguments = string(b)
					}
				}
			}
			output = append(output, openai.OutputItem{
				Type:   "function_call",
				Status: "completed",
				CallId: tool.Id,
				Name: func() string {
					if tool.Function != nil {
						return tool.Function.Name
					}
					return ""
				}(),
				Arguments: arguments,
			})
		}
	}
	return output
}

func buildRequiredActionToolCalls(choices []openai_compatible.TextResponseChoice) []openai.ResponseAPIToolCall {
	toolCalls := make([]openai.ResponseAPIToolCall, 0)
	for _, choice := range choices {
		for _, tool := range choice.Message.ToolCalls {
			if tool.Function == nil {
				continue
			}
			callID := ensureResponseAPICallID(tool.Id)
			if callID == "" {
				continue
			}
			arguments := ""
			if tool.Function.Arguments != nil {
				switch v := tool.Function.Arguments.(type) {
				case string:
					arguments = v
				default:
					if b, err := json.Marshal(v); err == nil {
						arguments = string(b)
					}
				}
			}
			toolCalls = append(toolCalls, openai.ResponseAPIToolCall{
				Id:   callID,
				Type: "function",
				Function: &openai.ResponseAPIFunctionCall{
					Name:      tool.Function.Name,
					Arguments: arguments,
				},
			})
		}
	}
	return toolCalls
}

func ensureResponseAPICallID(originalID string) string {
	trimmed := strings.TrimSpace(originalID)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "call_") {
		return trimmed
	}
	if strings.HasPrefix(trimmed, "fc_") {
		return strings.Replace(trimmed, "fc_", "call_", 1)
	}
	return "call_" + trimmed
}

func convertMessageContent(msg relaymodel.Message) []openai.OutputContent {
	var contents []openai.OutputContent
	if msg.IsStringContent() {
		if text := strings.TrimSpace(msg.StringContent()); text != "" {
			contents = append(contents, openai.OutputContent{Type: "output_text", Text: text})
		}
		return contents
	}

	for _, part := range msg.ParseContent() {
		switch part.Type {
		case relaymodel.ContentTypeText:
			if part.Text != nil && *part.Text != "" {
				contents = append(contents, openai.OutputContent{Type: "output_text", Text: *part.Text})
			}
		case relaymodel.ContentTypeImageURL:
			if part.ImageURL != nil && part.ImageURL.Url != "" {
				contents = append(contents, openai.OutputContent{Type: "output_text", Text: part.ImageURL.Url})
			}
		case relaymodel.ContentTypeInputAudio:
			if part.InputAudio != nil && part.InputAudio.Data != "" {
				contents = append(contents, openai.OutputContent{Type: "output_text", Text: part.InputAudio.Data})
			}
		}
	}
	return contents
}

func extractReasoning(msg relaymodel.Message) string {
	if msg.Reasoning != nil {
		return *msg.Reasoning
	}
	if msg.ReasoningContent != nil {
		return *msg.ReasoningContent
	}
	if msg.Thinking != nil {
		return *msg.Thinking
	}
	return ""
}

func convertResponseAPITools(tools []openai.ResponseAPITool) []relaymodel.Tool {
	if len(tools) == 0 {
		return nil
	}
	converted := make([]relaymodel.Tool, 0, len(tools))
	for _, tool := range tools {
		switch strings.ToLower(tool.Type) {
		case "function":
			fn := tool.Function
			if fn == nil {
				fn = &relaymodel.Function{
					Name:        tool.Name,
					Description: tool.Description,
					Parameters:  tool.Parameters,
				}
			}
			converted = append(converted, relaymodel.Tool{
				Type: "function",
				Function: &relaymodel.Function{
					Name:        fn.Name,
					Description: fn.Description,
					Parameters:  fn.Parameters,
					Required:    fn.Required,
					Strict:      fn.Strict,
				},
			})
		case "web_search":
			converted = append(converted, relaymodel.Tool{
				Type:              "web_search",
				SearchContextSize: tool.SearchContextSize,
				Filters:           tool.Filters,
				UserLocation:      tool.UserLocation,
			})
		case "mcp":
			converted = append(converted, relaymodel.Tool{
				Type:            "mcp",
				ServerLabel:     tool.ServerLabel,
				ServerUrl:       tool.ServerUrl,
				RequireApproval: tool.RequireApproval,
				AllowedTools:    tool.AllowedTools,
				Headers:         tool.Headers,
			})
		default:
			converted = append(converted, relaymodel.Tool{Type: tool.Type})
		}
	}
	return converted
}

// getChannelRatios gets channel model and completion ratios from unified ModelConfigs
func getChannelRatios(c *gin.Context) (map[string]float64, map[string]float64) {
	channel := c.MustGet(ctxkey.ChannelModel).(*model.Channel)

	// Only use unified ModelConfigs after migration
	modelRatios := channel.GetModelRatioFromConfigs()
	completionRatios := channel.GetCompletionRatioFromConfigs()

	return modelRatios, completionRatios
}

// getAndValidateResponseAPIRequest gets and validates Response API request
func getAndValidateResponseAPIRequest(c *gin.Context) (*openai.ResponseAPIRequest, error) {
	responseAPIRequest := &openai.ResponseAPIRequest{}
	err := common.UnmarshalBodyReusable(c, responseAPIRequest)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal Response API request")
	}

	// Basic validation
	if responseAPIRequest.Model == "" {
		return nil, errors.New("model is required")
	}

	// Either input or prompt is required, but not both
	hasInput := len(responseAPIRequest.Input) > 0
	hasPrompt := responseAPIRequest.Prompt != nil

	if !hasInput && !hasPrompt {
		return nil, errors.New("either input or prompt is required")
	}
	if hasInput && hasPrompt {
		return nil, errors.New("input and prompt are mutually exclusive - provide only one")
	}

	return responseAPIRequest, nil
}

// getResponseAPIPromptTokens estimates prompt tokens for Response API requests
func getResponseAPIPromptTokens(ctx context.Context, responseAPIRequest *openai.ResponseAPIRequest) int {
	// For now, use a simple estimation based on input content
	// This will be improved with proper token counting
	totalTokens := 0

	// Count tokens from input array (if present)
	for _, input := range responseAPIRequest.Input {
		switch v := input.(type) {
		case map[string]any:
			if content, ok := v["content"].(string); ok {
				// Simple estimation: ~4 characters per token
				totalTokens += len(content) / 4
			}
		case string:
			totalTokens += len(v) / 4
		}
	}

	// Count tokens from prompt template (if present)
	if responseAPIRequest.Prompt != nil {
		// Estimate tokens for prompt template ID (small fixed cost)
		totalTokens += 10

		// Count tokens from prompt variables
		for _, value := range responseAPIRequest.Prompt.Variables {
			switch v := value.(type) {
			case string:
				totalTokens += len(v) / 4
			case map[string]any:
				// For complex variables like input_file, add a fixed cost
				totalTokens += 20
			}
		}
	}

	// Add instruction tokens if present
	if responseAPIRequest.Instructions != nil {
		totalTokens += len(*responseAPIRequest.Instructions) / 4
	}

	// Minimum token count
	if totalTokens < 10 {
		totalTokens = 10
	}

	return totalTokens
}

func sanitizeResponseAPIRequest(request *openai.ResponseAPIRequest, channelType int) {
	if request == nil {
		return
	}
	modelName := strings.TrimSpace(strings.ToLower(request.Model))

	if isReasoningModel(modelName) {
		request.Temperature = nil
		request.TopP = nil
	}

	if request.Text != nil && request.Text.Format != nil && request.Text.Format.Schema != nil {
		if normalized, changed := openai.NormalizeStructuredJSONSchema(request.Text.Format.Schema, channelType); changed {
			request.Text.Format.Schema = normalized
		}
	}

	for idx := range request.Tools {
		tool := &request.Tools[idx]
		toolType := strings.ToLower(strings.TrimSpace(tool.Type))
		if (toolType == "web_search_preview" || toolType == "web_search_preview_reasoning" || toolType == "web_search_preview_non_reasoning") && channelType != channeltype.OpenAI {
			tool.Type = "web_search"
		}
		if tool.Parameters != nil {
			tool.Parameters, _ = openai.NormalizeStructuredJSONSchema(tool.Parameters, channelType)
		}
		if tool.Function != nil {
			if params, ok := tool.Function.Parameters.(map[string]any); ok && params != nil {
				if normalized, changed := openai.NormalizeStructuredJSONSchema(params, channelType); changed {
					tool.Function.Parameters = normalized
				}
			}
		}
	}
}

func sanitizeChatCompletionRequest(request *relaymodel.GeneralOpenAIRequest) {
	if request == nil {
		return
	}
	modelName := strings.TrimSpace(strings.ToLower(request.Model))

	if isReasoningModel(modelName) {
		request.Temperature = nil
		request.TopP = nil
	}
}

func supportsNativeResponseAPI(meta *metalib.Meta) bool {
	if meta == nil {
		return false
	}

	modelName := strings.TrimSpace(strings.ToLower(meta.ActualModelName))
	if modelName == "" {
		modelName = strings.TrimSpace(strings.ToLower(meta.OriginModelName))
	}
	if modelName != "" && openai.IsModelsOnlySupportedByChatCompletionAPI(modelName) {
		return false
	}

	if isDeepSeekModel(meta.ActualModelName) || isDeepSeekModel(meta.OriginModelName) {
		return false
	}

	switch meta.ChannelType {
	case channeltype.OpenAI:
		base := strings.TrimSpace(strings.ToLower(meta.BaseURL))
		if base == "" {
			return true
		}
		return strings.Contains(base, "api.openai.com")
	case channeltype.Azure:
		return openai.AzureRequiresResponseAPI(meta.ActualModelName)
	case channeltype.XAI:
		// XAI supports Response API natively
		return true
	case channeltype.OpenAICompatible:
		return channeltype.UseOpenAICompatibleResponseAPI(meta.Config.APIFormat)
	default:
		return false
	}
}

func isDeepSeekModel(modelName string) bool {
	normalized := strings.TrimSpace(strings.ToLower(modelName))
	if normalized == "" {
		return false
	}
	return strings.HasPrefix(normalized, "deepseek")
}

func isReasoningModel(modelName string) bool {
	if modelName == "" {
		return false
	}
	// Check for reasoning model prefixes (direct model names)
	if strings.HasPrefix(modelName, "gpt-5") ||
		strings.HasPrefix(modelName, "o1") ||
		strings.HasPrefix(modelName, "o3") ||
		strings.HasPrefix(modelName, "o4") ||
		strings.HasPrefix(modelName, "o-") {
		return true
	}
	// Also check for prefixed model names (e.g., "azure-gpt-5-nano", "vertex-o1-mini")
	// These are user-facing aliases that map to reasoning models.
	return strings.Contains(modelName, "-gpt-5") ||
		strings.Contains(modelName, "-o1-") ||
		strings.Contains(modelName, "-o3-") ||
		strings.Contains(modelName, "-o4-") ||
		strings.HasSuffix(modelName, "-o1") ||
		strings.HasSuffix(modelName, "-o3") ||
		strings.HasSuffix(modelName, "-o4")
}

// preConsumeResponseAPIQuota pre-consumes quota for Response API requests
func preConsumeResponseAPIQuota(
	c *gin.Context,
	responseAPIRequest *openai.ResponseAPIRequest,
	promptTokens int,
	inputRatio float64,
	outputRatio float64,
	background bool,
	meta *metalib.Meta,
) (int64, *relaymodel.ErrorWithStatusCode) {
	ctx := gmw.Ctx(c)
	baseQuota := calculateResponseAPIPreconsumeQuota(promptTokens, responseAPIRequest.MaxOutputTokens, inputRatio, outputRatio, background)

	tokenQuota := c.GetInt64(ctxkey.TokenQuota)
	tokenQuotaUnlimited := c.GetBool(ctxkey.TokenQuotaUnlimited)
	userQuota, err := model.CacheGetUserQuota(ctx, meta.UserId)
	if err != nil {
		return baseQuota, openai.ErrorWrapper(err, "get_user_quota_failed", http.StatusInternalServerError)
	}
	if userQuota-baseQuota < 0 {
		return baseQuota, openai.ErrorWrapper(errors.New("user quota is not enough"), "insufficient_user_quota", http.StatusForbidden)
	}

	if !tokenQuotaUnlimited && tokenQuota > 0 && tokenQuota-baseQuota < 0 {
		return baseQuota, openai.ErrorWrapper(errors.New("token quota is not enough"), "insufficient_token_quota", http.StatusForbidden)
	}

	err = model.PreConsumeTokenQuota(ctx, c.GetInt(ctxkey.TokenId), baseQuota)
	if err != nil {
		return baseQuota, openai.ErrorWrapper(err, "pre_consume_token_quota_failed", http.StatusForbidden)
	}

	return baseQuota, nil
}

func calculateResponseAPIPreconsumeQuota(promptTokens int, maxOutputTokens *int, inputRatio float64, outputRatio float64, background bool) int64 {
	preConsumedTokens := int64(promptTokens)
	if maxOutputTokens != nil {
		preConsumedTokens += int64(*maxOutputTokens)
	}

	baseQuota := int64(float64(preConsumedTokens) * inputRatio)
	if inputRatio != 0 && baseQuota <= 0 {
		baseQuota = 1
	}

	if background && outputRatio > 0 {
		backgroundQuota := int64(math.Ceil(float64(config.PreconsumeTokenForBackgroundRequest) * outputRatio))
		if backgroundQuota <= 0 {
			backgroundQuota = 1
		}
		if baseQuota < backgroundQuota {
			baseQuota = backgroundQuota
		}
	}

	return baseQuota
}

// postConsumeResponseAPIQuota calculates final quota consumption for Response API requests
// Following DRY principle by reusing the centralized billing.PostConsumeQuota function
func postConsumeResponseAPIQuota(ctx context.Context,
	usage *relaymodel.Usage,
	meta *metalib.Meta,
	responseAPIRequest *openai.ResponseAPIRequest,
	preConsumedQuota int64,
	modelRatio float64,
	groupRatio float64,
	channelCompletionRatio map[string]float64) (quota int64) {

	if usage == nil {
		// No gin context here; cannot use request-scoped logger
		// Keep silent here to avoid global logger; caller should ensure usage
		return
	}

	pricingAdaptor := relay.GetAdaptor(meta.ChannelType)
	computeResult := quotautil.Compute(quotautil.ComputeInput{
		Usage:                  usage,
		ModelName:              responseAPIRequest.Model,
		ModelRatio:             modelRatio,
		GroupRatio:             groupRatio,
		ChannelCompletionRatio: channelCompletionRatio,
		PricingAdaptor:         pricingAdaptor,
	})

	quota = computeResult.TotalQuota
	totalTokens := computeResult.PromptTokens + computeResult.CompletionTokens
	if totalTokens == 0 {
		quota = 0
	}

	// Use centralized detailed billing function to follow DRY principle
	quotaDelta := quota - preConsumedQuota
	cachedPrompt := computeResult.CachedPromptTokens
	promptTokens := computeResult.PromptTokens
	completionTokens := computeResult.CompletionTokens
	usedModelRatio := computeResult.UsedModelRatio
	if usedModelRatio == 0 {
		usedModelRatio = modelRatio
	}
	usedCompletionRatio := computeResult.UsedCompletionRatio
	if usedCompletionRatio == 0 {
		usedCompletionRatio = pricing.GetCompletionRatioWithThreeLayers(responseAPIRequest.Model, channelCompletionRatio, pricingAdaptor)
	}

	// Derive RequestId/TraceId from std context if possible
	var requestId string
	if ginCtx, ok := gmw.GetGinCtxFromStdCtx(ctx); ok {
		requestId = ginCtx.GetString(ctxkey.RequestId)
	}
	traceId := tracing.GetTraceIDFromContext(ctx)
	if meta.TokenId > 0 && meta.UserId > 0 && meta.ChannelId > 0 {
		var toolSummary *model.ToolUsageSummary
		if ginCtx, ok := gmw.GetGinCtxFromStdCtx(ctx); ok {
			if raw, exists := ginCtx.Get(ctxkey.ToolInvocationSummary); exists {
				if summary, ok := raw.(*model.ToolUsageSummary); ok {
					toolSummary = summary
				}
			}
		}
		metadata := model.AppendToolUsageMetadata(nil, toolSummary)
		metadata = model.AppendCacheWriteTokensMetadata(metadata, usage.CacheWrite5mTokens, usage.CacheWrite1hTokens)

		billing.PostConsumeQuotaDetailed(billing.QuotaConsumeDetail{
			Ctx:                    ctx,
			TokenId:                meta.TokenId,
			QuotaDelta:             quotaDelta,
			TotalQuota:             quota,
			UserId:                 meta.UserId,
			ChannelId:              meta.ChannelId,
			PromptTokens:           promptTokens,
			CompletionTokens:       completionTokens,
			ModelRatio:             usedModelRatio,
			GroupRatio:             groupRatio,
			ModelName:              responseAPIRequest.Model,
			TokenName:              meta.TokenName,
			IsStream:               meta.IsStream,
			StartTime:              meta.StartTime,
			SystemPromptReset:      false,
			CompletionRatio:        usedCompletionRatio,
			ToolsCost:              usage.ToolsCost,
			CachedPromptTokens:     cachedPrompt,
			CachedCompletionTokens: 0,
			CacheWrite5mTokens:     usage.CacheWrite5mTokens,
			CacheWrite1hTokens:     usage.CacheWrite1hTokens,
			Metadata:               metadata,
			RequestId:              requestId,
			TraceId:                traceId,
		})
	} else {
		// Should not happen; log for investigation
		lg := gmw.GetLogger(ctx)
		lg.Error("postConsumeResponseAPIQuota missing essential meta information",
			zap.Int("token_id", meta.TokenId),
			zap.Int("user_id", meta.UserId),
			zap.Int("channel_id", meta.ChannelId),
			zap.String("request_id", requestId),
			zap.String("trace_id", traceId),
		)
	}

	return quota
}

// getResponseAPIRequestBody gets the request body for Response API requests
func getResponseAPIRequestBody(c *gin.Context, meta *metalib.Meta, responseAPIRequest *openai.ResponseAPIRequest, adaptor adaptor.Adaptor) (io.Reader, error) {
	// Prefer forwarding the exact user payload to avoid mutating vendor-specific fields
	rawBody, err := common.GetRequestBody(c)
	if err != nil {
		return nil, errors.Wrap(err, "get raw Response API request body")
	}

	patched, err := normalizeResponseAPIRawBody(rawBody, responseAPIRequest)
	if err != nil {
		return nil, errors.Wrap(err, "normalize Response API request body")
	}

	return bytes.NewReader(patched), nil
}

func normalizeResponseAPIRawBody(rawBody []byte, request *openai.ResponseAPIRequest) ([]byte, error) {
	if request == nil {
		return rawBody, nil
	}

	if len(rawBody) == 0 {
		return json.Marshal(request)
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &root); err != nil {
		return json.Marshal(request)
	}

	changed := false

	if request.Model != "" {
		modelBytes, err := json.Marshal(request.Model)
		if err != nil {
			return nil, errors.Wrap(err, "marshal mapped model value")
		}
		if existing, ok := root["model"]; !ok || !bytes.Equal(existing, modelBytes) {
			root["model"] = modelBytes
			changed = true
		}
	}

	if request.ToolChoice == nil {
		if _, ok := root["tool_choice"]; ok {
			delete(root, "tool_choice")
			changed = true
		}
	} else {
		choiceBytes, err := json.Marshal(request.ToolChoice)
		if err != nil {
			return nil, errors.Wrap(err, "marshal request tool_choice")
		}
		if existing, ok := root["tool_choice"]; !ok || !bytes.Equal(existing, choiceBytes) {
			root["tool_choice"] = choiceBytes
			changed = true
		}
	}

	if request.Temperature == nil {
		if _, ok := root["temperature"]; ok {
			delete(root, "temperature")
			changed = true
		}
	}

	if request.TopP == nil {
		if _, ok := root["top_p"]; ok {
			delete(root, "top_p")
			changed = true
		}
	}

	if request.Text == nil {
		if _, ok := root["text"]; ok {
			delete(root, "text")
			changed = true
		}
	} else {
		textBytes, err := json.Marshal(request.Text)
		if err != nil {
			return nil, errors.Wrap(err, "marshal sanitized response text config")
		}
		if existing, ok := root["text"]; !ok || !bytes.Equal(existing, textBytes) {
			root["text"] = textBytes
			changed = true
		}
	}

	if len(request.Tools) == 0 {
		if _, ok := root["tools"]; ok {
			delete(root, "tools")
			changed = true
		}
	} else {
		toolsBytes, err := json.Marshal(request.Tools)
		if err != nil {
			return nil, errors.Wrap(err, "marshal sanitized response tools")
		}
		if existing, ok := root["tools"]; !ok || !bytes.Equal(existing, toolsBytes) {
			root["tools"] = toolsBytes
			changed = true
		}
	}

	if !changed {
		return rawBody, nil
	}

	patched, err := json.Marshal(root)
	if err != nil {
		return nil, errors.Wrap(err, "marshal patched Response API request")
	}
	return patched, nil
}

func applyResponseAPIStreamParams(c *gin.Context, meta *metalib.Meta) error {
	streamParam := c.Query("stream")
	if streamParam == "" {
		meta.IsStream = false
		return nil
	}

	stream, err := strconv.ParseBool(streamParam)
	if err != nil {
		return errors.Wrap(err, "parse stream query parameter")
	}
	meta.IsStream = stream
	return nil
}

// RelayResponseAPIGetHelper handles GET /v1/responses/:response_id requests
func RelayResponseAPIGetHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	meta := metalib.GetByContext(c)

	if meta.ChannelType != channeltype.OpenAI {
		return openai.ErrorWrapper(errors.New("Response API is only supported for OpenAI channels"), "unsupported_channel", http.StatusBadRequest)
	}

	if err := applyResponseAPIStreamParams(c, meta); err != nil {
		return openai.ErrorWrapper(err, "invalid_query_parameter", http.StatusBadRequest)
	}
	metalib.Set2Context(c, meta)

	adaptor := relay.GetAdaptor(meta.APIType)
	if adaptor == nil {
		return openai.ErrorWrapper(errors.New("invalid api type"), "invalid_api_type", http.StatusBadRequest)
	}
	adaptor.Init(meta)

	resp, err := adaptor.DoRequest(c, meta, nil)
	if err != nil {
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}

	if resp.StatusCode != http.StatusOK {
		return RelayErrorHandlerWithContext(c, resp)
	}

	_, respErr := adaptor.DoResponse(c, resp, meta)
	if respErr != nil {
		return respErr
	}

	return nil
}

// RelayResponseAPIDeleteHelper handles DELETE /v1/responses/:response_id requests
func RelayResponseAPIDeleteHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	meta := metalib.GetByContext(c)
	meta.IsStream = false
	metalib.Set2Context(c, meta)

	if meta.ChannelType != channeltype.OpenAI {
		return openai.ErrorWrapper(errors.New("Response API is only supported for OpenAI channels"), "unsupported_channel", http.StatusBadRequest)
	}

	adaptor := relay.GetAdaptor(meta.APIType)
	if adaptor == nil {
		return openai.ErrorWrapper(errors.New("invalid api type"), "invalid_api_type", http.StatusBadRequest)
	}
	adaptor.Init(meta)

	resp, err := adaptor.DoRequest(c, meta, nil)
	if err != nil {
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}

	if resp.StatusCode != http.StatusOK {
		return RelayErrorHandlerWithContext(c, resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	if err = resp.Body.Close(); err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError)
	}

	for key, values := range resp.Header {
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}
	if resp.Header.Get("Content-Type") == "" {
		c.Writer.Header().Set("Content-Type", "application/json")
	}
	c.Writer.WriteHeader(resp.StatusCode)
	if _, err = c.Writer.Write(body); err != nil {
		return openai.ErrorWrapper(err, "write_response_body_failed", http.StatusInternalServerError)
	}

	return nil
}

// RelayResponseAPICancelHelper handles POST /v1/responses/:response_id/cancel requests
func RelayResponseAPICancelHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	meta := metalib.GetByContext(c)
	meta.IsStream = false
	metalib.Set2Context(c, meta)

	if meta.ChannelType != channeltype.OpenAI {
		return openai.ErrorWrapper(errors.New("Response API is only supported for OpenAI channels"), "unsupported_channel", http.StatusBadRequest)
	}

	adaptor := relay.GetAdaptor(meta.APIType)
	if adaptor == nil {
		return openai.ErrorWrapper(errors.New("invalid api type"), "invalid_api_type", http.StatusBadRequest)
	}
	adaptor.Init(meta)

	resp, err := adaptor.DoRequest(c, meta, nil)
	if err != nil {
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}

	if resp.StatusCode != http.StatusOK {
		return RelayErrorHandlerWithContext(c, resp)
	}

	_, respErr := adaptor.DoResponse(c, resp, meta)
	if respErr != nil {
		return respErr
	}

	return nil
}
