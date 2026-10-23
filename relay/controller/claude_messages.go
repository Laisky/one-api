package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/relaymode"
)

// ClaudeMessagesRequest is an alias for the model.ClaudeRequest to follow DRY principle
type ClaudeMessagesRequest = relaymodel.ClaudeRequest

type claudeUpstreamErrorEnvelope struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// shouldRetryClaudeInvalidThinkingSignature reports whether the upstream failure matches Anthropic's invalid replayed thinking signature error.
// shouldRetryClaudeInvalidThinkingSignature returns true only for the specific invalid-request signature failure we can safely recover from.
func shouldRetryClaudeInvalidThinkingSignature(statusCode int, responseBody []byte) bool {
	if statusCode != http.StatusBadRequest || len(responseBody) == 0 {
		return false
	}

	var envelope claudeUpstreamErrorEnvelope
	if err := json.Unmarshal(responseBody, &envelope); err == nil {
		if strings.EqualFold(strings.TrimSpace(envelope.Error.Type), "invalid_request_error") &&
			(strings.Contains(envelope.Error.Message, "Invalid `signature` in `thinking` block") ||
				strings.Contains(envelope.Error.Message, ".thinking.signature: Field required") ||
				isClaudePreservedThinkingBindingError(envelope.Error.Message)) {
			return true
		}
	}

	return bytes.Contains(responseBody, []byte("Invalid `signature` in `thinking` block")) ||
		bytes.Contains(responseBody, []byte(".thinking.signature: Field required")) ||
		isClaudePreservedThinkingBindingError(string(responseBody))
}

// readAndRestoreResponseBody reads an HTTP response body and restores it for subsequent consumers.
// readAndRestoreResponseBody returns the full body bytes or an error if the response body cannot be read.
func readAndRestoreResponseBody(resp *http.Response) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read upstream response body")
	}
	if err := resp.Body.Close(); err != nil {
		return nil, errors.Wrap(err, "close upstream response body")
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

// RelayClaudeMessagesHelper validates, routes, and bills a Claude Messages
// request through either native pass-through or an adaptor conversion. Response
// errors with usage are returned after final settlement is scheduled.
func RelayClaudeMessagesHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	lg := gmw.GetLogger(c)
	// Passthrough is current-attempt authority. A retry reuses this gin context,
	// so only the selected adaptor may establish native capability again.
	if c.GetBool(ctxkey.ClaudeDirectPassthrough) {
		lg.Debug("reset native Claude passthrough capability for current attempt")
	}
	c.Set(ctxkey.ClaudeDirectPassthrough, false)
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

	// map model name
	meta.OriginModelName = claudeRequest.Model
	claudeRequest.Model = meta.ActualModelName
	meta.ActualModelName = claudeRequest.Model
	metalib.Set2Context(c, meta)

	claudeRequest.CompatibilityModel = anthropic.CompatibilityModel(c, claudeRequest.Model)
	if anthropic.IsClaudeSonnet55(claudeRequest.CompatibilityModel) {
		raw, bodyErr := common.GetRequestBody(c)
		if bodyErr != nil {
			return openai.ErrorWrapper(bodyErr, "invalid_claude_messages_request", http.StatusBadRequest)
		}
		var fields map[string]json.RawMessage
		if bodyErr := json.Unmarshal(raw, &fields); bodyErr != nil {
			return openai.ErrorWrapper(bodyErr, "invalid_claude_messages_request", http.StatusBadRequest)
		}
		if bodyErr := anthropic.NormalizeSonnet55Controls(claudeRequest.CompatibilityModel, fields); bodyErr != nil {
			return openai.ErrorWrapper(bodyErr, "invalid_claude_messages_request", http.StatusBadRequest)
		}
	}
	sanitizeClaudeMessagesRequest(claudeRequest)

	// Fold any mid-array role:"system" messages (Claude Code v2.1.154+ /
	// Anthropic mid-conversation system messages, issue #350) into an adjacent
	// turn for rebuilt upstream payloads, while preserving the cacheable top-level
	// system prefix. Direct HTTP passthrough adaptors still forward the raw body
	// verbatim; SDK/rebuilding paths use this normalized struct.
	mergeMidArraySystemMessages(claudeRequest)

	// get channel model ratio
	channelModelRatio, channelCompletionRatio := getChannelRatios(c)
	channelModelConfigs := getChannelModelConfigs(c)

	// get model ratio using three-layer pricing system
	pricingAdaptor := resolvePricingAdaptor(meta)
	modelRatio := pricing.ResolveModelRatioAt(claudeRequest.Model, channelModelConfigs, channelModelRatio, pricingAdaptor, meta.StartTime)
	completionRatio := pricing.ResolveCompletionRatioAt(claudeRequest.Model, channelModelConfigs, channelCompletionRatio, pricingAdaptor, meta.StartTime)
	groupRatio := c.GetFloat64(ctxkey.ChannelRatio)

	ratio := modelRatio * groupRatio

	// Prepare conversion once before admission so the quote follows the representation sent to the provider.
	adaptorInstance := relay.GetAdaptor(meta.APIType)
	if adaptorInstance == nil {
		return openai.ErrorWrapper(errors.New("invalid api type"), "invalid_api_type", http.StatusBadRequest)
	}
	adaptorInstance.Init(meta)
	convertedRequest, err := adaptorInstance.ConvertClaudeRequest(c, claudeRequest)
	if err != nil {
		return wrapConvertRequestError(err)
	}
	convertedRequest = sanitizeConvertedChatFields(convertedRequest)
	// Preserve the original request/converted quote for Claude-native Tool
	// Search. Injected deferred MCP tool definitions are a separate internal
	// round and are funded by executeClaudeToolSearchMCPLoop, not twice via
	// the initial client-facing reservation.
	originalToolSearchQuote := *claudeRequest
	originalToolSearchQuote.Tools = append([]relaymodel.ClaudeTool(nil), claudeRequest.Tools...)
	originalConvertedQuote := convertedRequest

	// Select the actual MCP dispatch route before tool admission or quota
	// reservation. A Claude client request may execute as Chat internally, and
	// those two modes can have different configured upstream endpoint URLs.
	mcpPlan, planErr := planClaudeMCPRoute(c, meta, claudeRequest, adaptorInstance)
	if planErr != nil {
		return planErr
	}
	if mcpPlan.toolSearch != nil {
		// Deferred MCP tools were injected by planning; quote and validate
		// the converted payload that will actually be used by the loop.
		convertedRequest, err = adaptorInstance.ConvertClaudeRequest(c, claudeRequest)
		if err != nil {
			return wrapConvertRequestError(err)
		}
		convertedRequest = sanitizeConvertedChatFields(convertedRequest)
	}
	admissionMeta := meta
	if mcpPlan.registry != nil {
		planned := *meta
		planned.Mode = relaymode.ChatCompletions
		planned.IsStream = false
		admissionMeta = &planned
	}
	if admissionErr := admitClaudeMessagesTools(c, admissionMeta); admissionErr != nil {
		return admissionErr
	}
	// Conversion can introduce additional built-ins, so check both the
	// provider-visible conversion and, when MCP is selected, the actual
	// Chat request used for its internal upstream rounds.
	if policyErr := admitConvertedClaudeBuiltins(c, admissionMeta, convertedRequest, adaptorInstance); policyErr != nil {
		return policyErr
	}
	if mcpPlan.registry != nil {
		if policyErr := admitMCPConvertedChatBuiltins(c, admissionMeta, mcpPlan.request, adaptorInstance); policyErr != nil {
			return policyErr
		}
	}
	quoteRequest := claudeRequest
	quoteConverted := convertedRequest
	if mcpPlan.toolSearch != nil {
		// Keep the pre-injection quote, matching the existing Claude MCP
		// reservation contract. The actual converted payload above still
		// receives tool admission, and every internal round funds its own
		// expanded schema before sending.
		quoteRequest = &originalToolSearchQuote
		quoteConverted = originalConvertedQuote
	}
	promptTokens, quoteErr := preparedClaudePromptTokens(ctx, quoteRequest, quoteConverted)
	if quoteErr != nil {
		return openai.ErrorWrapper(quoteErr, "invalid_claude_prompt_quote", http.StatusBadRequest)
	}
	meta.PromptTokens = promptTokens
	c.Set(ctxkey.ConvertedRequest, convertedRequest)
	preConsumedQuota, bizErr := preConsumeClaudeMessagesQuota(c, claudeRequest, promptTokens, ratio, completionRatio, meta)
	if bizErr != nil {
		lg.Warn("preConsumeClaudeMessagesQuota failed",
			zap.Int("status_code", bizErr.StatusCode),
			zap.Error(bizErr.RawError))
		return bizErr
	}
	markPreConsumed(c, preConsumedQuota)
	defer billingAuditSafetyNet(c)

	provisionalLogId := recordProvisionalLog(c, meta, claudeRequest.Model, preConsumedQuota)
	c.Set(ctxkey.ProvisionalLogId, provisionalLogId)

	// Declare response variables early so goto postConsume does not skip over them.
	var (
		usage                 *relaymodel.Usage
		respErr               *relaymodel.ErrorWithStatusCode
		mcpIncrementalCharged int64
		resp                  *http.Response
		origResp              *http.Response
		upstreamCapture       *loggingReadCloser
		requestBody           io.Reader
		passthroughBody       []byte
	)

	// Tool Search + MCP integration: when the request contains a tool_search_tool and
	// there are MCP tools in the catalog, inject them as deferred tools and run a
	// Claude-native MCP execution loop that handles tool discovery and execution.
	if toolSearchRegistry := mcpPlan.toolSearch; toolSearchRegistry != nil {
		lg.Debug("tool search with MCP tools detected, using Claude-native MCP loop")

		// Disable streaming for the MCP loop
		noStream := false
		claudeRequest.Stream = &noStream

		response, mcpUsage, mcpSummary, incrementalCharged, execErr := executeClaudeToolSearchMCPLoop(
			c, meta, claudeRequest, toolSearchRegistry, adaptorInstance, preConsumedQuota,
		)
		if execErr != nil && mcpUsage == nil {
			if refundClaudeAdmission(c, execErr, preConsumedQuota, c.GetInt(ctxkey.TokenId)) {
				return execErr
			}
			_ = returnPreConsumedQuotaConservative(ctx, c, preConsumedQuota, c.GetInt(ctxkey.TokenId), "tool_search_mcp_loop_failed")
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
		usage = mcpUsage
		mcpIncrementalCharged = incrementalCharged
		respErr = execErr
		if execErr == nil && response != nil {
			if errResp := writeClaudeResponse(c, response); errResp != nil {
				respErr = openai.ErrorWrapper(errResp, "write_tool_search_response_failed", http.StatusInternalServerError)
			}
		}
		// Preserve prior receipts and all physical reservation credits even when
		// continuation or downstream delivery fails after accepted upstream work.
		markResponseSettlement(c, usage, respErr)
		goto postConsume
	}

	// The registry and actual endpoint were selected before tool admission.
	if mcpRegistry := mcpPlan.registry; mcpRegistry != nil {
		mcpToolNames, mcpReq := mcpPlan.names, mcpPlan.request
		mcpReq.ToolChoice = normalizeChatToolChoiceForMCP(mcpReq.ToolChoice, mcpToolNames)
		if mcpReq.Stream {
			mcpReq.Stream = false
			meta.IsStream = false
		}
		// Internal rounds consume Chat responses; only the final result is
		// converted to Claude. Run before ordinary dispatch to avoid paid work
		// outside the loop and suppress intermediate Claude conversion.
		downstreamMode := meta.Mode
		downstreamNative := c.GetBool(ctxkey.ClaudeMessagesNative)
		downstreamConversion := c.GetBool(ctxkey.ClaudeMessagesConversion)
		meta.Mode = relaymode.ChatCompletions
		meta.IsStream = false
		c.Set(ctxkey.ClaudeMessagesNative, false)
		c.Set(ctxkey.ClaudeMessagesConversion, false)
		response, mcpUsage, mcpSummary, incrementalCharged, execErr := executeChatMCPToolLoop(c, meta, mcpReq, mcpRegistry, preConsumedQuota)
		meta.Mode = downstreamMode
		c.Set(ctxkey.ClaudeMessagesNative, downstreamNative)
		c.Set(ctxkey.ClaudeMessagesConversion, downstreamConversion)
		metalib.Set2Context(c, meta)
		if execErr != nil && mcpUsage == nil {
			if refundClaudeAdmission(c, execErr, preConsumedQuota, c.GetInt(ctxkey.TokenId)) {
				return execErr
			}
			_ = returnPreConsumedQuotaConservative(ctx, c, preConsumedQuota, c.GetInt(ctxkey.TokenId), "mcp_tool_loop_failed")
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
		usage = mcpUsage
		mcpIncrementalCharged = incrementalCharged
		respErr = execErr
		if execErr == nil {
			respErr = renderClaudeMessagesFromChatResponse(c, response)
		}
		markResponseSettlement(c, usage, respErr)
		goto postConsume
	}

	// Ordinary dispatch reuses the conversion that admission quoted. The MCP branch
	// above explicitly reconverts after it injects deferred tools and uses its own execution loop.

	// Determine request body:
	// - If adaptor marks direct pass-through, forward the Claude Messages payload
	//   but ensure the mapped model name is applied to the raw JSON
	// - Otherwise, marshal the converted request
	// requestBody declared above
	if passthrough, ok := c.Get(ctxkey.ClaudeDirectPassthrough); ok && passthrough.(bool) {
		rawBody, gerr := common.GetRequestBody(c)
		if gerr != nil {
			return openai.ErrorWrapper(gerr, "get_original_body_failed", http.StatusInternalServerError)
		}
		sanitizedBody, unsignedThinkingStats, serr := rewriteAndSanitizeClaudeRequestBody(rawBody, claudeRequest)
		if serr != nil {
			return openai.ErrorWrapper(serr, "rewrite_sanitize_claude_body_failed", http.StatusInternalServerError)
		}
		passthroughBody = sanitizedBody
		requestBody = bytes.NewReader(sanitizedBody)

		// Log signature-bearing thinking blocks count for diagnostics
		sigCount := countThinkingSignatures(rawBody)
		if sigCount > 0 || unsignedThinkingStats.RemovedThinkingBlocks > 0 {
			fields := []zap.Field{
				zap.Int("signature_count", sigCount),
				zap.Int("removed_unsigned_thinking_blocks", unsignedThinkingStats.RemovedThinkingBlocks),
				zap.Int("removed_empty_assistant_messages", unsignedThinkingStats.RemovedAssistantMessages),
				zap.Int("raw_body_len", len(rawBody)),
				zap.Int("sanitized_body_len", len(sanitizedBody)),
				zap.Bool("body_bytes_preserved", bytes.Contains(sanitizedBody, []byte(`"signature"`))),
			}
			if len(unsignedThinkingStats.Locations) > 0 {
				fields = append(fields, zap.Strings("removed_unsigned_thinking_locations", unsignedThinkingStats.Locations))
			}
			lg.Debug("analyzed Claude passthrough thinking blocks", fields...)
		}
	} else {
		c.Set(ctxkey.ConvertedRequest, convertedRequest)
		requestBytes, merr := json.Marshal(convertedRequest)
		if merr != nil {
			return openai.ErrorWrapper(merr, "marshal_request_failed", http.StatusInternalServerError)
		}
		requestBody = bytes.NewReader(requestBytes)
	}

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
		zap.String("outgoing_model", meta.ActualModelName),
	)

	// ST-022: when this Claude request is served by a Responses upstream and an exact
	// transcript checkpoint exists, continue from the bound upstream handle and send
	// only the delta. Fails open to the full body on any miss (pure optimization);
	// no-op on the passthrough branch (no converted *ResponseAPIRequest present).
	if newBody, matched := matchClaudeCheckpoint(c, meta, claudeRequest); matched {
		requestBody = bytes.NewReader(newBody)
	}

	// do request
	resp, err = adaptorInstance.DoRequest(c, meta, requestBody)
	if err != nil {
		if c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded) {
			_ = retainDeepSeekToolAttempt(c, preConsumedQuota, meta.TokenId, "transport_uncertain")
		}
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}
	origResp = resp
	upstreamCapture = wrapUpstreamResponse(resp)
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
		if passthrough, ok := c.Get(ctxkey.ClaudeDirectPassthrough); ok && passthrough.(bool) && len(passthroughBody) > 0 {
			responseBody, bodyErr := readAndRestoreResponseBody(resp)
			if bodyErr != nil {
				lg.Debug("failed to inspect Claude passthrough error response for signature retry",
					zap.Error(bodyErr),
					zap.Int("status_code", resp.StatusCode),
				)
			} else if shouldRetryClaudeThinkingReplay(resp.StatusCode, responseBody, claudeRequest) {
				logUpstreamResponseFromBytes(lg, resp, responseBody, "claude_messages_signature_rejected")

				retryBody, retryStats, retryBodyErr := stripClaudeThinkingFromAssistantHistory(passthroughBody)
				if retryBodyErr != nil {
					lg.Debug("failed to build Claude signature retry payload",
						zap.Error(retryBodyErr),
						zap.Int("status_code", resp.StatusCode),
					)
				} else if retryStats.RemovedThinkingBlocks > 0 {
					lg.Debug("retrying Claude passthrough after invalid thinking signature",
						zap.Int("status_code", resp.StatusCode),
						zap.Int("removed_thinking_blocks", retryStats.RemovedThinkingBlocks),
						zap.Int("removed_assistant_messages", retryStats.RemovedAssistantMessages),
					)

					resp, err = adaptorInstance.DoRequest(c, meta, bytes.NewReader(retryBody))
					if err != nil {
						return openai.ErrorWrapper(err, "retry_claude_request_failed", http.StatusInternalServerError)
					}
					origResp = resp
					upstreamCapture = wrapUpstreamResponse(resp)

					if resp != nil && resp.StatusCode != http.StatusOK {
						retryResponseBody, retryReadErr := readAndRestoreResponseBody(resp)
						if retryReadErr != nil {
							lg.Debug("failed to inspect Claude signature retry response",
								zap.Error(retryReadErr),
								zap.Int("status_code", resp.StatusCode),
							)
						} else {
							logUpstreamResponseFromBytes(lg, resp, retryResponseBody, "claude_messages_signature_retry_failed")
							lg.Debug("Claude passthrough signature retry still failed",
								zap.Int("status_code", resp.StatusCode),
								zap.Int("removed_thinking_blocks", retryStats.RemovedThinkingBlocks),
								zap.Int("removed_assistant_messages", retryStats.RemovedAssistantMessages),
							)
						}
					} else {
						lg.Debug("Claude passthrough signature retry succeeded",
							zap.Int("removed_thinking_blocks", retryStats.RemovedThinkingBlocks),
							zap.Int("removed_assistant_messages", retryStats.RemovedAssistantMessages),
						)
					}
				}
			}
		}

		if resp != nil && resp.StatusCode == http.StatusOK {
			goto handleResponse
		}

		if retainDeepSeekToolAttempt(c, preConsumedQuota, meta.TokenId, "upstream_http_error") {
			var nativeError *anthropic.NativeErrorResponse
			if c.GetBool(ctxkey.ClaudeDirectPassthrough) {
				nativeError = anthropic.PrepareNativeErrorResponse(resp)
			}
			apiErr := RelayErrorHandlerWithContext(c, resp)
			anthropic.RetainNativeErrorResponse(c, nativeError, apiErr)
			return apiErr
		}
		scheduleConservativeRefund(c, preConsumedQuota, c.GetInt(ctxkey.TokenId), "upstream_http_error")
		// Reconcile provisional record to 0 since upstream returned error
		quotaId := c.GetInt(ctxkey.Id)
		requestId := c.GetString(ctxkey.RequestId)
		if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, 0); err != nil {
			lg.Warn("update user request cost to zero failed", zap.Error(err))
		}
		var nativeError *anthropic.NativeErrorResponse
		if c.GetBool(ctxkey.ClaudeDirectPassthrough) {
			nativeError = anthropic.PrepareNativeErrorResponse(resp)
		}
		apiErr := RelayErrorHandlerWithContext(c, resp)
		anthropic.RetainNativeErrorResponse(c, nativeError, apiErr)
		return apiErr
	}

	// Set context flag to indicate Claude Messages native mode
	c.Set(ctxkey.ClaudeMessagesNative, true)

	// do response - for direct passthrough, forward upstream JSON verbatim; otherwise let adaptor convert

handleResponse:

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
			respErr, usage = anthropic.ClaudeNativeHandler(c, resp, promptTokens, meta.ActualModelName)
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

	if respErr == nil {
		// ST-022: record a stateless-client continuation checkpoint when this Claude
		// request was served by a Responses upstream. No-op otherwise; never fatal.
		recordClaudeCheckpoint(c, meta, claudeRequest)
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
							CacheWrite5mTokens: func() int {
								if claudeResp.Usage.CacheCreation != nil {
									return claudeResp.Usage.CacheCreation.Ephemeral5mInputTokens
								}
								return claudeResp.Usage.CacheCreationInputTokens
							}(),
							CacheWrite1hTokens: func() int {
								if claudeResp.Usage.CacheCreation != nil {
									return claudeResp.Usage.CacheCreation.Ephemeral1hInputTokens
								}
								return 0
							}(),
						}
						if claudeResp.Usage.CacheReadInputTokens > 0 {
							usage.PromptTokensDetails = &relaymodel.UsagePromptTokensDetails{CachedTokens: claudeResp.Usage.CacheReadInputTokens}
						}
						lg.Debug("using converted Claude usage for billing",
							zap.Int("input_tokens", usage.PromptTokens),
							zap.Int("output_tokens", usage.CompletionTokens),
							zap.Int("cache_read_input_tokens", claudeResp.Usage.CacheReadInputTokens),
							zap.Int("cache_write_5m_tokens", usage.CacheWrite5mTokens),
							zap.Int("cache_write_1h_tokens", usage.CacheWrite1hTokens),
						)
					} else {
						// No receipt provided: preserve the prepared admission quote and
						// mark inferred completion usage so settlement cannot refund uncertain work.
						accumulated := ""
						for _, part := range claudeResp.Content {
							if part.Type == "text" && part.Text != "" {
								accumulated += part.Text
							}
						}
						completion := openai.CountTokenText(accumulated, meta.ActualModelName)
						usage = &relaymodel.Usage{
							PromptTokens:          promptTokens,
							CompletionTokens:      completion,
							TotalTokens:           promptTokens + completion,
							BillingEstimateReason: "converted_claude_usage_missing_counters",
						}
					}
				} else {
					// 2) If not Claude JSON, it may be SSE (OpenAI-compatible). Detect and compute from stream text.
					ct := resp.Header.Get("Content-Type")
					if strings.Contains(strings.ToLower(ct), "text/event-stream") || bytes.HasPrefix(body, []byte("data:")) || bytes.Contains(body, []byte("\ndata:")) {
						usage = extractConvertedClaudeSSEUsage(body, promptTokens, meta.ActualModelName)
					} else {
						// 3) Fallback: estimate prompt only
						usage = &relaymodel.Usage{
							PromptTokens:          promptTokens,
							CompletionTokens:      0,
							TotalTokens:           promptTokens,
							BillingEstimateReason: "converted_claude_usage_unknown_payload",
						}
					}
				}
			}
		} else {
			// No converted response, use Claude native handlers for proper format
			if meta.IsStream {
				respErr, usage = anthropic.ClaudeNativeStreamHandler(c, resp)
			} else {
				// Native fallback also reuses the prepared admission quote for usage estimates.
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
			if refundClaudeAdmission(c, respErr, preConsumedQuota, c.GetInt(ctxkey.TokenId)) {
				return respErr
			}
			scheduleConservativeRefund(c, preConsumedQuota, c.GetInt(ctxkey.TokenId), "do_response_failed_without_usage")
			return respErr
		}
		// Fall through to billing with available usage
	}

postConsume:
	applyClaudeServerToolCharges(c, &usage, meta, adaptorInstance)

	// post-consume quota
	quotaId := c.GetInt(ctxkey.Id)
	requestId := c.GetString(ctxkey.RequestId)

	// Capture trace ID before launching goroutine
	traceId := tracing.GetTraceID(c)
	markBillingReconciled(c)
	runPostBillingWithTimeout(detachForBilling(c), "postBilling", lg, postBillingTimeoutInfo{
		userID:              meta.UserId,
		channelID:           meta.ChannelId,
		model:               claudeRequest.Model,
		requestID:           requestId,
		startTime:           meta.StartTime,
		estimatedQuota:      func() float64 { return float64(usage.PromptTokens+usage.CompletionTokens) * ratio },
		guardTimeoutLog:     func() bool { return true },
		logMessage:          "CRITICAL BILLING TIMEOUT",
		includeElapsedField: true,
	}, func(ctx context.Context) {
		quota := postConsumeClaudeMessagesQuotaWithTraceID(ctx, requestId, traceId, usage, meta, claudeRequest, ratio, preConsumedQuota, mcpIncrementalCharged, modelRatio, channelModelRatio, groupRatio, channelModelConfigs, channelCompletionRatio)
		// Reconcile request cost with final quota (override provisional value)
		if quota != 0 {
			if err := model.UpdateUserRequestCostQuotaByRequestID(quotaId, requestId, quota); err != nil {
				lg.Error("update user request cost failed", zap.Error(err))
			}
		}
	})

	markResponseSettlement(c, usage, respErr)
	return respErr
}
