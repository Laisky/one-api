package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
	"github.com/songquanpeng/one-api/relay/adaptor/anthropic"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
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
	completionRatio := pricing.GetCompletionRatioWithThreeLayers(claudeRequest.Model, channelCompletionRatio, pricingAdaptor)
	groupRatio := c.GetFloat64(ctxkey.ChannelRatio)

	ratio := modelRatio * groupRatio

	// pre-consume quota based on estimated input tokens
	promptTokens := getClaudeMessagesPromptTokens(gmw.Ctx(c), claudeRequest)
	meta.PromptTokens = promptTokens
	preConsumedQuota, bizErr := preConsumeClaudeMessagesQuota(c, claudeRequest, promptTokens, ratio, completionRatio, meta)
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
		promptQuota := float64(promptTokens) * ratio
		completionQuota := 0.0
		if claudeRequest.MaxTokens > 0 {
			completionQuota = float64(claudeRequest.MaxTokens) * ratio * completionRatio
		}
		estimated := int64(promptQuota + completionQuota)
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
	var mcpIncrementalCharged int64

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
		response, mcpUsage, mcpSummary, incrementalCharged, execErr := executeChatMCPToolLoop(c, meta, mcpReq, mcpRegistry, preConsumedQuota)
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
		mcpIncrementalCharged = incrementalCharged
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
			quota = postConsumeClaudeMessagesQuotaWithTraceID(ctx, requestId, traceId, usage, meta, claudeRequest, ratio, preConsumedQuota, mcpIncrementalCharged, modelRatio, groupRatio, channelCompletionRatio)

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
