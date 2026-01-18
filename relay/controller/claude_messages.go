package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
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
	"github.com/songquanpeng/one-api/relay/adaptor/anthropic"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/adaptor/openai_compatible"
	"github.com/songquanpeng/one-api/relay/billing"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/pricing"
)

// ClaudeMessagesRequest is an alias for the model.ClaudeRequest to follow DRY principle
type ClaudeMessagesRequest = relaymodel.ClaudeRequest

// RelayClaudeMessagesHelper handles Claude Messages API requests with direct pass-through
func RelayClaudeMessagesHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	lg := gmw.GetLogger(c)
	ctx := gmw.Ctx(c)
	meta := metalib.GetByContext(c)
	if err := logClientRequestPayload(c, "claude_messages"); err != nil {
		return openai.ErrorWrapper(err, "invalid_claude_messages_request", http.StatusBadRequest)
	}

	// get & validate Claude Messages API request
	claudeRequest, err := getAndValidateClaudeMessagesRequest(c)
	if err != nil {
		return openai.ErrorWrapper(err, "invalid_claude_messages_request", http.StatusBadRequest)
	}
	meta.IsStream = claudeRequest.Stream != nil && *claudeRequest.Stream

	if reqBody, ok := c.Get(ctxkey.KeyRequestBody); ok {
		lg.Debug("get claude messages request", zap.ByteString("body", reqBody.([]byte)))
	}

	// map model name
	meta.OriginModelName = claudeRequest.Model
	claudeRequest.Model = meta.ActualModelName
	meta.ActualModelName = claudeRequest.Model
	metalib.Set2Context(c, meta)

	sanitizeClaudeMessagesRequest(claudeRequest)

	// get channel model ratio
	channelModelRatio, channelCompletionRatio := getChannelRatios(c)

	// get model ratio using three-layer pricing system
	pricingAdaptor := relay.GetAdaptor(meta.ChannelType)
	modelRatio := pricing.GetModelRatioWithThreeLayers(claudeRequest.Model, channelModelRatio, pricingAdaptor)
	groupRatio := c.GetFloat64(ctxkey.ChannelRatio)

	ratio := modelRatio * groupRatio

	// pre-consume quota based on estimated input tokens
	promptTokens := getClaudeMessagesPromptTokens(gmw.Ctx(c), claudeRequest)
	meta.PromptTokens = promptTokens
	preConsumedQuota, bizErr := preConsumeClaudeMessagesQuota(c, claudeRequest, promptTokens, ratio, meta)
	if bizErr != nil {
		lg.Warn("preConsumeClaudeMessagesQuota failed",
			zap.Int("status_code", bizErr.StatusCode),
			zap.Error(bizErr.RawError))
		return bizErr
	}

	adaptorInstance := relay.GetAdaptor(meta.APIType)
	if adaptorInstance == nil {
		return openai.ErrorWrapper(errors.New("invalid api type"), "invalid_api_type", http.StatusBadRequest)
	}
	adaptorInstance.Init(meta)

	// convert request using adaptor's ConvertClaudeRequest method
	convertedRequest, err := adaptorInstance.ConvertClaudeRequest(c, claudeRequest)
	if err != nil {
		// Check if this is a validation error and preserve the correct HTTP status code
		//
		// This is for AWS, which must be different from other providers that are
		// based on proprietary systems such as OpenAI, etc.
		switch {
		case strings.Contains(err.Error(), "does not support the v1/messages endpoint"):
			return openai.ErrorWrapper(err, "invalid_request_error", http.StatusBadRequest)
		default:
			return openai.ErrorWrapper(err, "convert_request_failed", http.StatusInternalServerError)
		}
	}

	// Determine request body:
	// - If adaptor marks direct pass-through, forward the Claude Messages payload
	//   but ensure the mapped model name is applied to the raw JSON
	// - Otherwise, marshal the converted request
	var requestBody io.Reader
	if passthrough, ok := c.Get(ctxkey.ClaudeDirectPassthrough); ok && passthrough.(bool) {
		rawBody, gerr := common.GetRequestBody(c)
		if gerr != nil {
			return openai.ErrorWrapper(gerr, "get_original_body_failed", http.StatusInternalServerError)
		}
		rewritten, rerr := rewriteClaudeRequestBody(rawBody, claudeRequest)
		if rerr != nil {
			return openai.ErrorWrapper(rerr, "rewrite_claude_body_failed", http.StatusInternalServerError)
		}
		requestBody = bytes.NewReader(rewritten)
	} else {
		requestBytes, merr := json.Marshal(convertedRequest)
		if merr != nil {
			return openai.ErrorWrapper(merr, "marshal_request_failed", http.StatusInternalServerError)
		}
		requestBody = bytes.NewReader(requestBytes)
	}

	// for debug
	requestBodyBytes, _ := io.ReadAll(requestBody)
	// Attempt to log outgoing model for diagnostics without printing the entire payload
	var outgoing struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(requestBodyBytes, &outgoing)
	lg.Debug("prepared Claude upstream request",
		zap.Bool("passthrough", func() bool {
			if v, ok := c.Get(ctxkey.ClaudeDirectPassthrough); ok {
				b, _ := v.(bool)
				return b
			}
			return false
		}()),
		zap.String("origin_model", meta.OriginModelName),
		zap.String("mapped_model", meta.ActualModelName),
		zap.String("outgoing_model", outgoing.Model),
	)
	requestBody = bytes.NewReader(requestBodyBytes)

	// do request
	resp, err := adaptorInstance.DoRequest(c, meta, requestBody)
	if err != nil {
		// ErrorWrapper will log the error, so we don't need to log it here
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}
	origResp := resp
	upstreamCapture := wrapUpstreamResponse(resp)
	// Immediately record a provisional request cost using estimated base quota
	// even if the trusted path skipped physical pre-consume.
	{
		quotaId := c.GetInt(ctxkey.Id)
		requestId := c.GetString(ctxkey.RequestId)
		estimatedTokens := int64(promptTokens)
		if claudeRequest.MaxTokens > 0 {
			estimatedTokens += int64(claudeRequest.MaxTokens)
		}
		estimated := int64(float64(estimatedTokens) * ratio)
		if estimated <= 0 {
			estimated = preConsumedQuota
		}
		if estimated > 0 {
			if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, estimated); err != nil {
				lg.Warn("record provisional user request cost failed", zap.Error(err))
			}
		}
	}

	// Check for HTTP errors when an HTTP response is returned by the adaptor
	if resp != nil && resp.StatusCode != http.StatusOK {
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

	// Set context flag to indicate Claude Messages native mode
	c.Set(ctxkey.ClaudeMessagesNative, true)

	// do response - for direct passthrough, forward upstream JSON verbatim; otherwise let adaptor convert
	var usage *relaymodel.Usage
	var respErr *relaymodel.ErrorWithStatusCode

	// MCP tool loop handling for Claude Messages requests.
	if mcpRegistry, mcpToolNames, mcpReq, mcpErr := detectClaudeMCPTools(c, meta, claudeRequest, adaptorInstance); mcpRegistry != nil || mcpErr != nil {
		if mcpErr != nil {
			billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, c.GetInt(ctxkey.TokenId))
			return openai.ErrorWrapper(mcpErr, "mcp_tool_registry_failed", http.StatusBadRequest)
		}
		mcpReq.ToolChoice = normalizeChatToolChoiceForMCP(mcpReq.ToolChoice, mcpToolNames)
		if mcpReq.Stream {
			mcpReq.Stream = false
			meta.IsStream = false
		}
		response, mcpUsage, mcpSummary, execErr := executeChatMCPToolLoop(c, meta, mcpReq, mcpRegistry)
		if execErr != nil {
			billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, c.GetInt(ctxkey.TokenId))
			return execErr
		}
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
		if errResp := renderClaudeMessagesFromChatResponse(c, response); errResp != nil {
			billing.ReturnPreConsumedQuota(ctx, preConsumedQuota, c.GetInt(ctxkey.TokenId))
			return errResp
		}
		usage = mcpUsage
		goto postConsume
	}

	if passthrough, ok := c.Get(ctxkey.ClaudeDirectPassthrough); ok && passthrough.(bool) && meta.IsStream {
		// Streaming direct passthrough: forward Claude SSE events verbatim
		// For AWS Bedrock, resp might be nil since it uses SDK calls
		if resp != nil {
			respErr, usage = anthropic.ClaudeNativeStreamHandler(c, resp)
		} else {
			// For AWS Bedrock streaming, delegate to adapter's DoResponse
			c.Set(ctxkey.SkipAdaptorResponseBodyLog, true)
			usage, respErr = adaptorInstance.DoResponse(c, resp, meta)
		}
	} else if passthrough, ok := c.Get(ctxkey.ClaudeDirectPassthrough); ok && passthrough.(bool) && !meta.IsStream {
		// Non-streaming direct passthrough: copy headers/body exactly as upstream returned
		// and extract usage for billing from the Claude response
		// For AWS Bedrock, resp might be nil since it uses SDK calls
		if resp != nil {
			body, rerr := io.ReadAll(resp.Body)
			if rerr != nil {
				respErr = openai.ErrorWrapper(rerr, "read_upstream_response_failed", http.StatusInternalServerError)
			} else {
				// Close upstream body
				_ = resp.Body.Close()

				// Forward headers
				for k, v := range resp.Header {
					if len(v) > 0 {
						c.Header(k, v[0])
					}
				}
				c.Status(resp.StatusCode)
				c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)

				// Parse usage from Claude native response for billing
				var claudeResp anthropic.Response
				if perr := json.Unmarshal(body, &claudeResp); perr == nil {
					usage = &relaymodel.Usage{
						PromptTokens:     claudeResp.Usage.InputTokens,
						CompletionTokens: claudeResp.Usage.OutputTokens,
						TotalTokens:      claudeResp.Usage.InputTokens + claudeResp.Usage.OutputTokens,
						ServiceTier:      claudeResp.Usage.ServiceTier,
					}
					// Map cached prompt token details
					if claudeResp.Usage.CacheReadInputTokens > 0 {
						usage.PromptTokensDetails = &relaymodel.UsagePromptTokensDetails{CachedTokens: claudeResp.Usage.CacheReadInputTokens}
					}
					if claudeResp.Usage.CacheCreation != nil {
						usage.CacheWrite5mTokens = claudeResp.Usage.CacheCreation.Ephemeral5mInputTokens
						usage.CacheWrite1hTokens = claudeResp.Usage.CacheCreation.Ephemeral1hInputTokens
					} else if claudeResp.Usage.CacheCreationInputTokens > 0 {
						// Legacy field: treat as 5m cache write
						usage.CacheWrite5mTokens = claudeResp.Usage.CacheCreationInputTokens
					}
				} else {
					// Fallback usage on parse error
					promptTokens := getClaudeMessagesPromptTokens(ctx, claudeRequest)
					usage = &relaymodel.Usage{
						PromptTokens:     promptTokens,
						CompletionTokens: 0,
						TotalTokens:      promptTokens,
					}
				}
			}
		} else {
			// For AWS Bedrock non-streaming, delegate to adapter's DoResponse
			c.Set(ctxkey.SkipAdaptorResponseBodyLog, true)
			usage, respErr = adaptorInstance.DoResponse(c, resp, meta)
		}
	} else {
		// Call the adapter's DoResponse method to handle response conversion
		c.Set(ctxkey.SkipAdaptorResponseBodyLog, true)
		usage, respErr = adaptorInstance.DoResponse(c, resp, meta)
	}
	if upstreamCapture != nil {
		logUpstreamResponseFromCapture(lg, origResp, upstreamCapture, "claude_messages")
	} else {
		logUpstreamResponseFromBytes(lg, origResp, nil, "claude_messages")
	}

	// If the adapter didn't handle the conversion (e.g., for native Anthropic),
	// fall back to Claude native handlers
	if respErr == nil && usage == nil {
		// Check if there's a converted response from the adapter
		if convertedResp, exists := c.Get(ctxkey.ConvertedResponse); exists {
			// The adapter has already converted the response to Claude format
			// We can use it directly without calling Claude native handlers
			resp = convertedResp.(*http.Response)

			// Copy the response directly to the client
			for k, v := range resp.Header {
				c.Header(k, v[0])
			}
			c.Status(resp.StatusCode)

			// Copy the response body and extract usage information
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				respErr = openai.ErrorWrapper(err, "read_converted_response_failed", http.StatusInternalServerError)
			} else {
				c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)

				// Extract usage information from the response body for billing
				// 1) Try Claude JSON body with usage
				var claudeResp relaymodel.ClaudeResponse
				if parseErr := json.Unmarshal(body, &claudeResp); parseErr == nil {
					if claudeResp.Usage.InputTokens > 0 || claudeResp.Usage.OutputTokens > 0 {
						usage = &relaymodel.Usage{
							PromptTokens:     claudeResp.Usage.InputTokens,
							CompletionTokens: claudeResp.Usage.OutputTokens,
							TotalTokens:      claudeResp.Usage.InputTokens + claudeResp.Usage.OutputTokens,
						}
					} else {
						// No usage provided: compute completion tokens from content text
						accumulated := ""
						for _, part := range claudeResp.Content {
							if part.Type == "text" && part.Text != "" {
								accumulated += part.Text
							}
						}
						promptTokens := getClaudeMessagesPromptTokens(ctx, claudeRequest)
						completion := openai.CountTokenText(accumulated, meta.ActualModelName)
						usage = &relaymodel.Usage{
							PromptTokens:     promptTokens,
							CompletionTokens: completion,
							TotalTokens:      promptTokens + completion,
						}
					}
				} else {
					// 2) If not Claude JSON, it may be SSE (OpenAI-compatible). Detect and compute from stream text.
					ct := resp.Header.Get("Content-Type")
					if strings.Contains(strings.ToLower(ct), "text/event-stream") || bytes.HasPrefix(body, []byte("data:")) || bytes.Contains(body, []byte("\ndata:")) {
						accumulated := ""
						for line := range bytes.SplitSeq(body, []byte("\n")) {
							line = bytes.TrimSpace(line)
							if !bytes.HasPrefix(line, []byte("data:")) {
								continue
							}
							payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
							if bytes.Equal(payload, []byte("[DONE]")) {
								continue
							}
							// Minimal parse of OpenAI chat stream chunk
							var chunk struct {
								Choices []struct {
									Delta struct {
										Content any `json:"content"`
									} `json:"delta"`
								} `json:"choices"`
							}
							if err := json.Unmarshal(payload, &chunk); err == nil {
								for _, ch := range chunk.Choices {
									switch v := ch.Delta.Content.(type) {
									case string:
										accumulated += v
									case []any:
										for _, p := range v {
											if m, ok := p.(map[string]any); ok {
												if t, _ := m["type"].(string); t == "text" {
													if s, ok := m["text"].(string); ok {
														accumulated += s
													}
												}
											}
										}
									}
								}
							}
						}
						promptTokens := getClaudeMessagesPromptTokens(ctx, claudeRequest)
						completion := openai.CountTokenText(accumulated, meta.ActualModelName)
						usage = &relaymodel.Usage{
							PromptTokens:     promptTokens,
							CompletionTokens: completion,
							TotalTokens:      promptTokens + completion,
						}
					} else {
						// 3) Fallback: estimate prompt only
						promptTokens := getClaudeMessagesPromptTokens(ctx, claudeRequest)
						usage = &relaymodel.Usage{
							PromptTokens:     promptTokens,
							CompletionTokens: 0,
							TotalTokens:      promptTokens,
						}
					}
				}
			}
		} else {
			// No converted response, use Claude native handlers for proper format
			if meta.IsStream {
				respErr, usage = anthropic.ClaudeNativeStreamHandler(c, resp)
			} else {
				// For non-streaming, we need the prompt tokens count for usage calculation
				promptTokens := getClaudeMessagesPromptTokens(ctx, claudeRequest)
				respErr, usage = anthropic.ClaudeNativeHandler(c, resp, promptTokens, meta.ActualModelName)
			}
		}
	}

	if respErr != nil {
		lg.Error("Claude native response handler failed",
			zap.Int("status_code", respErr.StatusCode),
			zap.Error(respErr.RawError))
		// If usage is available (e.g., client disconnected after upstream response),
		// proceed with billing; otherwise, refund pre-consumed quota and return error.
		if usage == nil {
			graceful.GoCritical(ctx, "returnPreConsumedQuota", func(cctx context.Context) {
				billing.ReturnPreConsumedQuota(cctx, preConsumedQuota, c.GetInt(ctxkey.TokenId))
			})
			return respErr
		}
		// Fall through to billing with available usage
	}

postConsume:

	// post-consume quota
	quotaId := c.GetInt(ctxkey.Id)
	requestId := c.GetString(ctxkey.RequestId)

	// Capture trace ID before launching goroutine
	traceId := tracing.GetTraceID(c)
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
			quota = postConsumeClaudeMessagesQuotaWithTraceID(ctx, requestId, traceId, usage, meta, claudeRequest, ratio, preConsumedQuota, modelRatio, groupRatio, channelCompletionRatio)

			// Reconcile request cost with final quota (override provisional value)
			if quota != 0 {
				if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, quota); err != nil {
					lg.Error("update user request cost failed", zap.Error(err))
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
					zap.String("model", claudeRequest.Model),
					zap.String("requestId", requestId),
					zap.Int("userId", meta.UserId),
					zap.Int64("estimatedQuota", int64(estimatedQuota)),
					zap.Duration("elapsedTime", elapsedTime))

				// Record billing timeout in metrics
				metrics.GlobalRecorder.RecordBillingTimeout(meta.UserId, meta.ChannelId, claudeRequest.Model, estimatedQuota, elapsedTime)

				// TODO: Implement dead letter queue or retry mechanism for failed billing
			}
		}
	})

	return nil
}

// Removed redundant getChannelRatiosForClaude; use getChannelRatios from response.go to keep DRY.

// sanitizeClaudeMessagesRequest enforces parameter constraints required by upstream providers.
func sanitizeClaudeMessagesRequest(request *ClaudeMessagesRequest) {
	if request == nil {
		return
	}
	if request.Temperature != nil && request.TopP != nil {
		request.TopP = nil
	}
}

// buildClaudeToolsForMCP converts Claude tool definitions into OpenAI-style tools for MCP matching.
func buildClaudeToolsForMCP(request *ClaudeMessagesRequest) []relaymodel.Tool {
	if request == nil || len(request.Tools) == 0 {
		return nil
	}
	tools := make([]relaymodel.Tool, 0, len(request.Tools))
	for _, claudeTool := range request.Tools {
		toolType := strings.TrimSpace(claudeTool.Type)
		if toolType != "" && claudeTool.InputSchema == nil {
			tools = append(tools, relaymodel.Tool{Type: toolType})
			continue
		}
		parameters, ok := claudeTool.InputSchema.(map[string]any)
		if !ok {
			parameters = map[string]any{}
		}
		tools = append(tools, relaymodel.Tool{
			Type: "function",
			Function: &relaymodel.Function{
				Name:        claudeTool.Name,
				Description: claudeTool.Description,
				Parameters:  parameters,
			},
		})
	}
	return tools
}

// detectClaudeMCPTools returns a converted request and MCP registry when Claude tools match MCP tools.
func detectClaudeMCPTools(c *gin.Context, meta *metalib.Meta, request *ClaudeMessagesRequest, adaptorInstance adaptor.Adaptor) (*mcpToolRegistry, map[string]struct{}, *relaymodel.GeneralOpenAIRequest, error) {
	if request == nil {
		return nil, nil, nil, nil
	}
	channelRecord := func() *model.Channel {
		if channelModel, ok := c.Get(ctxkey.ChannelModel); ok {
			if channel, ok := channelModel.(*model.Channel); ok {
				return channel
			}
		}
		return nil
	}()

	probe := &relaymodel.GeneralOpenAIRequest{Model: request.Model, Tools: buildClaudeToolsForMCP(request)}
	registry, mcpToolNames, regErr := expandMCPBuiltinsInChatRequest(c, meta, channelRecord, adaptorInstance, probe)
	if regErr != nil || registry == nil {
		return nil, nil, nil, regErr
	}

	convertedAny, err := openai_compatible.ConvertClaudeRequest(c, request)
	if err != nil {
		return nil, nil, nil, err
	}
	converted, ok := convertedAny.(*relaymodel.GeneralOpenAIRequest)
	if !ok {
		return nil, nil, nil, errors.New("converted Claude request is not OpenAI request")
	}
	registry, mcpToolNames, regErr = expandMCPBuiltinsInChatRequest(c, meta, channelRecord, adaptorInstance, converted)
	if regErr != nil || registry == nil {
		return nil, nil, nil, regErr
	}
	return registry, mcpToolNames, converted, nil
}

// renderClaudeMessagesFromChatResponse converts an OpenAI chat response into Claude Messages JSON output.
func renderClaudeMessagesFromChatResponse(c *gin.Context, response *openai.TextResponse) *relaymodel.ErrorWithStatusCode {
	if response == nil {
		return openai.ErrorWrapper(errors.New("chat response is nil"), "convert_response_failed", http.StatusInternalServerError)
	}
	payload, err := json.Marshal(response)
	if err != nil {
		return openai.ErrorWrapper(err, "marshal_chat_response_failed", http.StatusInternalServerError)
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(payload)),
	}
	converted, errResp := openai_compatible.ConvertOpenAIResponseToClaudeResponse(c, resp)
	if errResp != nil {
		return errResp
	}
	defer converted.Body.Close()
	body, err := io.ReadAll(converted.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "read_converted_response_failed", http.StatusInternalServerError)
	}
	contentType := converted.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Writer.Header().Set("Content-Type", contentType)
	c.Writer.WriteHeader(converted.StatusCode)
	if _, err = c.Writer.Write(body); err != nil {
		return openai.ErrorWrapper(err, "write_converted_response_failed", http.StatusInternalServerError)
	}
	return nil
}

// rewriteClaudeRequestBody updates the raw JSON payload to reflect sanitized request fields.
func rewriteClaudeRequestBody(raw []byte, request *ClaudeMessagesRequest) ([]byte, error) {
	if len(raw) == 0 || request == nil {
		return raw, nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, errors.Wrap(err, "unmarshal raw claude body for rewrite")
	}
	if request.Model != "" {
		obj["model"] = request.Model
	}
	if request.TopP == nil {
		delete(obj, "top_p")
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, errors.Wrap(err, "marshal rewritten claude body")
	}
	return out, nil
}

// getAndValidateClaudeMessagesRequest gets and validates Claude Messages API request
func getAndValidateClaudeMessagesRequest(c *gin.Context) (*ClaudeMessagesRequest, error) {
	claudeRequest := &ClaudeMessagesRequest{}
	err := common.UnmarshalBodyReusable(c, claudeRequest)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal Claude messages request")
	}

	// Basic validation
	if claudeRequest.Model == "" {
		return nil, errors.New("model is required")
	}
	if claudeRequest.MaxTokens <= 0 {
		return nil, errors.New("max_tokens must be greater than 0")
	}
	if len(claudeRequest.Messages) == 0 {
		return nil, errors.New("messages array cannot be empty")
	}

	// Validate messages
	for i, message := range claudeRequest.Messages {
		if message.Role == "" {
			return nil, errors.Errorf("message[%d].role is required", i)
		}
		if message.Role != "user" && message.Role != "assistant" {
			return nil, errors.Errorf("message[%d].role must be 'user' or 'assistant'", i)
		}
		if message.Content == nil {
			return nil, errors.Errorf("message[%d].content is required", i)
		}
		// Additional validation for content based on type
		switch content := message.Content.(type) {
		case string:
			if content == "" {
				return nil, errors.Errorf("message[%d].content cannot be empty string", i)
			}
		case []any:
			if len(content) == 0 {
				return nil, errors.Errorf("message[%d].content array cannot be empty", i)
			}
		default:
			// Allow other content types (like structured content blocks)
		}
	}

	return claudeRequest, nil
}

// getClaudeMessagesPromptTokens estimates the number of prompt tokens for Claude Messages API
func getClaudeMessagesPromptTokens(ctx context.Context, request *ClaudeMessagesRequest) int {
	logger := gmw.GetLogger(ctx)

	// Convert Claude Messages to OpenAI format for accurate token counting
	openaiRequest := convertClaudeToOpenAIForTokenCounting(request)

	// Use simple character-based estimation for now to avoid tiktoken issues
	// This can be improved later with proper tokenization
	promptTokens := estimateTokensFromMessages(openaiRequest.Messages)

	// Add tokens for tools if present
	toolsTokens := 0
	if len(request.Tools) > 0 {
		toolsTokens = countClaudeToolsTokens(ctx, request.Tools, "gpt-3.5-turbo")
		promptTokens += toolsTokens
	}

	// Add tokens for images using Claude-specific calculation
	imageTokens := calculateClaudeImageTokens(ctx, request)
	promptTokens += imageTokens

	textTokens := promptTokens - imageTokens - toolsTokens

	logger.Debug("estimated prompt tokens for Claude Messages",
		zap.Int("total", promptTokens),
		zap.Int("text", textTokens),
		zap.Int("tools", toolsTokens),
		zap.Int("images", imageTokens),
	)
	return promptTokens
}

// countClaudeToolsTokens estimates tokens for Claude tools
func countClaudeToolsTokens(ctx context.Context, tools []relaymodel.ClaudeTool, model string) int {
	totalTokens := 0

	for _, tool := range tools {
		// Count tokens for tool name and description
		totalTokens += openai.CountTokenText(tool.Name, model)
		totalTokens += openai.CountTokenText(tool.Description, model)

		// Count tokens for input schema (convert to JSON string for counting)
		if tool.InputSchema != nil {
			if schemaBytes, err := json.Marshal(tool.InputSchema); err == nil {
				totalTokens += openai.CountTokenText(string(schemaBytes), model)
			}
		}
	}

	return totalTokens
}

// convertClaudeToOpenAIForTokenCounting converts Claude Messages format to OpenAI format for token counting
func convertClaudeToOpenAIForTokenCounting(request *ClaudeMessagesRequest) *relaymodel.GeneralOpenAIRequest {
	openaiRequest := &relaymodel.GeneralOpenAIRequest{
		Model:    request.Model,
		Messages: []relaymodel.Message{},
	}

	// Convert system prompt
	if request.System != nil {
		switch system := request.System.(type) {
		case string:
			if system != "" {
				openaiRequest.Messages = append(openaiRequest.Messages, relaymodel.Message{
					Role:    "system",
					Content: system,
				})
			}
		case []any:
			// For structured system content, extract text parts
			var systemParts []string
			for _, block := range system {
				if blockMap, ok := block.(map[string]any); ok {
					if text, exists := blockMap["text"]; exists {
						if textStr, ok := text.(string); ok {
							systemParts = append(systemParts, textStr)
						}
					}
				}
			}
			if len(systemParts) > 0 {
				systemText := strings.Join(systemParts, "\n")
				openaiRequest.Messages = append(openaiRequest.Messages, relaymodel.Message{
					Role:    "system",
					Content: systemText,
				})
			}
		}
	}

	// Convert messages
	for _, msg := range request.Messages {
		openaiMessage := relaymodel.Message{
			Role: msg.Role,
		}

		// Convert content based on type
		switch content := msg.Content.(type) {
		case string:
			// Simple string content
			openaiMessage.Content = content
		case []any:
			// Structured content blocks - convert to OpenAI format
			var contentParts []relaymodel.MessageContent
			for _, block := range content {
				if blockMap, ok := block.(map[string]any); ok {
					if blockType, exists := blockMap["type"]; exists {
						switch blockType {
						case "text":
							if text, exists := blockMap["text"]; exists {
								if textStr, ok := text.(string); ok {
									contentParts = append(contentParts, relaymodel.MessageContent{
										Type: "text",
										Text: &textStr,
									})
								}
							}
						case "image":
							if source, exists := blockMap["source"]; exists {
								if sourceMap, ok := source.(map[string]any); ok {
									imageURL := relaymodel.ImageURL{}
									if mediaType, exists := sourceMap["media_type"]; exists {
										if data, exists := sourceMap["data"]; exists {
											if dataStr, ok := data.(string); ok {
												// Convert to data URL format for token counting
												imageURL.Url = fmt.Sprintf("data:%s;base64,%s", mediaType, dataStr)
											}
										}
									} else if url, exists := sourceMap["url"]; exists {
										if urlStr, ok := url.(string); ok {
											imageURL.Url = urlStr
										}
									}
									contentParts = append(contentParts, relaymodel.MessageContent{
										Type:     "image_url",
										ImageURL: &imageURL,
									})
								}
							}
						}
					}
				}
			}
			if len(contentParts) > 0 {
				openaiMessage.Content = contentParts
			}
		default:
			// Fallback: convert to string
			if contentBytes, err := json.Marshal(content); err == nil {
				openaiMessage.Content = string(contentBytes)
			}
		}

		openaiRequest.Messages = append(openaiRequest.Messages, openaiMessage)
	}

	return openaiRequest
}

// convertClaudeToolsToOpenAI converts Claude tools to OpenAI format for token counting
func convertClaudeToolsToOpenAI(claudeTools []relaymodel.ClaudeTool) []relaymodel.Tool {
	var openaiTools []relaymodel.Tool

	for _, tool := range claudeTools {
		openaiTool := relaymodel.Tool{
			Type: "function",
			Function: &relaymodel.Function{
				Name:        tool.Name,
				Description: tool.Description,
			},
		}

		// Convert input schema
		if tool.InputSchema != nil {
			if schemaMap, ok := tool.InputSchema.(map[string]any); ok {
				openaiTool.Function.Parameters = schemaMap
			}
		}

		openaiTools = append(openaiTools, openaiTool)
	}

	return openaiTools
}

// calculateClaudeStructuredOutputCost calculates additional cost for structured output in Claude Messages API
func calculateClaudeStructuredOutputCost(_ *ClaudeMessagesRequest, _ int, _ float64, _ float64) int64 {
	// No surcharge for structured outputs
	return 0
}

// calculateClaudeImageTokens calculates tokens for images in Claude Messages API
// According to Claude documentation: tokens = (width px * height px) / 750
func calculateClaudeImageTokens(ctx context.Context, request *ClaudeMessagesRequest) int {
	logger := gmw.GetLogger(ctx)
	totalImageTokens := 0

	// Process messages for images
	for _, message := range request.Messages {
		switch content := message.Content.(type) {
		case []any:
			// Handle content blocks (text, image, etc.)
			for _, block := range content {
				if blockMap, ok := block.(map[string]any); ok {
					if blockType, exists := blockMap["type"]; exists && blockType == "image" {
						imageTokens := calculateSingleImageTokens(ctx, blockMap)
						totalImageTokens += imageTokens
					}
				}
			}
		}
	}

	// Process system prompt for images (if it contains structured content)
	if request.System != nil {
		if systemBlocks, ok := request.System.([]any); ok {
			for _, block := range systemBlocks {
				if blockMap, ok := block.(map[string]any); ok {
					if blockType, exists := blockMap["type"]; exists && blockType == "image" {
						imageTokens := calculateSingleImageTokens(ctx, blockMap)
						totalImageTokens += imageTokens
					}
				}
			}
		}
	}

	logger.Debug("calculated image tokens for Claude Messages", zap.Int("image_tokens", totalImageTokens))
	return totalImageTokens
}

// calculateSingleImageTokens calculates tokens for a single image block
func calculateSingleImageTokens(ctx context.Context, imageBlock map[string]any) int {
	logger := gmw.GetLogger(ctx)

	source, exists := imageBlock["source"]
	if !exists {
		return 0
	}

	sourceMap, ok := source.(map[string]any)
	if !ok {
		return 0
	}

	sourceType, exists := sourceMap["type"]
	if !exists {
		return 0
	}

	switch sourceType {
	case "base64":
		if data, exists := sourceMap["data"]; exists {
			if dataStr, ok := data.(string); ok {
				estimatedTokens := min(max(len(dataStr)/1000, 50), 2000)
				logger.Debug("estimated tokens for base64 image",
					zap.Int("tokens", estimatedTokens),
					zap.Int("data_length", len(dataStr)),
				)
				return estimatedTokens
			}
		}

	case "url":
		estimatedTokens := 853
		logger.Debug("estimated tokens for URL image", zap.Int("tokens", estimatedTokens))
		return estimatedTokens

	case "file":
		estimatedTokens := 853
		logger.Debug("estimated tokens for file image", zap.Int("tokens", estimatedTokens))
		return estimatedTokens
	}

	return 0
}

// estimateTokensFromMessages provides a simple character-based token estimation
// This is a fallback when proper tokenization is not available
func estimateTokensFromMessages(messages []relaymodel.Message) int {
	totalChars := 0

	for _, message := range messages {
		// Count role characters
		totalChars += len(message.Role)

		// Count content characters
		switch content := message.Content.(type) {
		case string:
			totalChars += len(content)
		case []relaymodel.MessageContent:
			for _, part := range content {
				if part.Type == "text" && part.Text != nil {
					totalChars += len(*part.Text)
				}
				// Images are counted separately in calculateClaudeImageTokens
			}
		default:
			// Fallback: convert to string and count
			if contentBytes, err := json.Marshal(content); err == nil {
				totalChars += len(contentBytes)
			}
		}
	}

	// Rough estimation: 4 characters per token (this is a simplification)
	estimatedTokens := max(totalChars/4, 1)
	return estimatedTokens
}

// preConsumeClaudeMessagesQuota pre-consumes quota for Claude Messages API requests
func preConsumeClaudeMessagesQuota(c *gin.Context, request *ClaudeMessagesRequest, promptTokens int, ratio float64, meta *metalib.Meta) (int64, *relaymodel.ErrorWithStatusCode) {
	// Use similar logic to ChatCompletion pre-consumption
	ctx := gmw.Ctx(c)
	preConsumedTokens := int64(promptTokens)
	if request.MaxTokens > 0 {
		preConsumedTokens += int64(request.MaxTokens)
	}

	baseQuota := int64(float64(preConsumedTokens) * ratio)
	if ratio != 0 && baseQuota <= 0 {
		baseQuota = 1
	}

	// Check user quota first
	tokenQuota := c.GetInt64(ctxkey.TokenQuota)
	tokenQuotaUnlimited := c.GetBool(ctxkey.TokenQuotaUnlimited)
	userQuota, err := model.CacheGetUserQuota(ctx, meta.UserId)
	if err != nil {
		return baseQuota, openai.ErrorWrapper(err, "get_user_quota_failed", http.StatusInternalServerError)
	}
	if userQuota-baseQuota < 0 {
		return baseQuota, openai.ErrorWrapper(errors.New("user quota is not enough"), "insufficient_user_quota", http.StatusForbidden)
	}
	err = model.CacheDecreaseUserQuota(ctx, meta.UserId, baseQuota)
	if err != nil {
		return baseQuota, openai.ErrorWrapper(err, "decrease_user_quota_failed", http.StatusInternalServerError)
	}
	if userQuota > 100*baseQuota &&
		(tokenQuotaUnlimited || tokenQuota > 100*baseQuota) {
		// in this case, we do not pre-consume quota
		// because the user and token have enough quota
		baseQuota = 0
		gmw.GetLogger(c).Info(fmt.Sprintf("user %d has enough quota %d, trusted and no need to pre-consume", meta.UserId, userQuota))
	}
	if baseQuota > 0 {
		err := model.PreConsumeTokenQuota(ctx, meta.TokenId, baseQuota)
		if err != nil {
			return baseQuota, openai.ErrorWrapper(err, "pre_consume_token_quota_failed", http.StatusForbidden)
		}
	}

	gmw.GetLogger(c).Debug("pre-consumed quota for Claude Messages",
		zap.Int64("quota", baseQuota),
		zap.Int("tokens", int(preConsumedTokens)),
		zap.Float64("ratio", ratio))
	return baseQuota, nil
}

// postConsumeClaudeMessagesQuotaWithTraceID calculates and applies final quota consumption for Claude Messages API with explicit trace ID
func postConsumeClaudeMessagesQuotaWithTraceID(ctx context.Context, requestId string, traceId string, usage *relaymodel.Usage, meta *metalib.Meta, request *ClaudeMessagesRequest, ratio float64, preConsumedQuota int64, modelRatio float64, groupRatio float64, channelCompletionRatio map[string]float64) int64 {
	if usage == nil {
		// Context may be detached; log with context if available
		gmw.GetLogger(ctx).Warn("usage is nil for Claude Messages API")
		return 0
	}

	// Use three-layer pricing system for completion ratio
	pricingAdaptor := relay.GetAdaptor(meta.ChannelType)
	completionRatio := pricing.GetCompletionRatioWithThreeLayers(request.Model, channelCompletionRatio, pricingAdaptor)
	promptTokens := usage.PromptTokens
	completionTokens := usage.CompletionTokens

	// Calculate base quota
	baseQuota := int64(math.Ceil((float64(promptTokens) + float64(completionTokens)*completionRatio) * ratio))

	// No structured output surcharge
	quota := baseQuota + usage.ToolsCost
	if ratio != 0 && quota <= 0 {
		quota = 1
	}

	totalTokens := promptTokens + completionTokens
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
	}

	// Extract cache token counts from usage details
	cachedPromptTokens := 0
	if usage.PromptTokensDetails != nil {
		cachedPromptTokens = usage.PromptTokensDetails.CachedTokens
	}
	cachedCompletionTokens := 0
	if usage.CompletionTokensDetails != nil {
		cachedCompletionTokens = usage.CompletionTokensDetails.CachedTokens
	}

	cacheWrite5mTokens := usage.CacheWrite5mTokens
	cacheWrite1hTokens := usage.CacheWrite1hTokens
	metadata := model.AppendCacheWriteTokensMetadata(nil, cacheWrite5mTokens, cacheWrite1hTokens)

	// Use centralized detailed billing function with explicit trace ID
	quotaDelta := quota - preConsumedQuota
	// If requestId somehow empty, try derive from ctx (best-effort)
	if requestId == "" {
		if ginCtx, ok := gmw.GetGinCtxFromStdCtx(ctx); ok {
			requestId = ginCtx.GetString(ctxkey.RequestId)
		}
	}
	billing.PostConsumeQuotaDetailed(billing.QuotaConsumeDetail{
		Ctx:                    ctx,
		TokenId:                meta.TokenId,
		QuotaDelta:             quotaDelta,
		TotalQuota:             quota,
		UserId:                 meta.UserId,
		ChannelId:              meta.ChannelId,
		PromptTokens:           promptTokens,
		CompletionTokens:       completionTokens,
		ModelRatio:             modelRatio,
		GroupRatio:             groupRatio,
		ModelName:              request.Model,
		TokenName:              meta.TokenName,
		IsStream:               meta.IsStream,
		StartTime:              meta.StartTime,
		SystemPromptReset:      false,
		CompletionRatio:        completionRatio,
		ToolsCost:              usage.ToolsCost,
		CachedPromptTokens:     cachedPromptTokens,
		CachedCompletionTokens: cachedCompletionTokens,
		CacheWrite5mTokens:     cacheWrite5mTokens,
		CacheWrite1hTokens:     cacheWrite1hTokens,
		Metadata:               metadata,
		RequestId:              requestId,
		TraceId:                traceId,
	})

	// Log with context if available
	gmw.GetLogger(ctx).Debug("Claude Messages quota with trace ID",
		zap.Int64("pre_consumed", preConsumedQuota),
		zap.Int64("actual", quota),
		zap.Int64("difference", quotaDelta),
	)
	return quota
}
