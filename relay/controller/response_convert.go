package controller

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/random"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// renderChatResponseAsResponseAPI renders a Chat Completion response as a Response API response
func renderChatResponseAsResponseAPI(c *gin.Context, status int, textResp *openai_compatible.SlimTextResponse, originalReq *openai.ResponseAPIRequest, meta *metalib.Meta) error {
	c.Set(ctxkey.ResponseRewriteApplied, true)
	responseID := generateResponseAPIID(c, textResp)
	statusText, incomplete := deriveResponseStatus(textResp.Choices)
	usage := (&openai.ResponseAPIUsage{}).FromModelUsage(&textResp.Usage)
	lg := gmw.GetLogger(c)
	output := buildResponseOutput(lg, textResp.Choices, responseFallbackCatalogForRender(c, originalReq))
	toolCalls := buildRequiredActionToolCalls(textResp.Choices)

	// Commit a gateway response node so the fallback ID is resolvable, retrievable,
	// and deletable (closes B10). No-op when the feature is inactive; on any store
	// failure the response still renders with a synthetic ID.
	var storePtr *bool
	var conversationPtr *openai.ResponseAPIConversation
	if commit := pendingCommitFromContext(c); commit != nil {
		res := commitFallbackResponseNode(c, commit, output, usage, statusText)
		output = res.output
		if res.committed {
			responseID = res.gatewayID
		}
		storeMode := res.storeMode
		storePtr = &storeMode
		if res.conversation != "" {
			conversationPtr = &openai.ResponseAPIConversation{Id: res.conversation}
		}
	}
	// Persisted state keeps gateway thinking envelopes for previous_response_id;
	// the client sees them only when it opted in through include.
	output = clientVisibleResponseOutput(output, responseIncludesReasoningEncryptedContent(originalReq))

	response := openai.ResponseAPIResponse{
		Id:                 responseID,
		Object:             "response",
		CreatedAt:          time.Now().Unix(),
		Status:             statusText,
		Model:              userVisibleModelName(meta, originalReq.Model),
		Output:             output,
		Usage:              usage,
		Store:              storePtr,
		Conversation:       conversationPtr,
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
		ToolsEcho:          responseToolsEcho(lg, originalReq.Tools),
		TopP:               originalReq.TopP,
		Truncation:         originalReq.Truncation,
		User:               originalReq.User,
	}
	if response.Model == "" && meta != nil {
		response.Model = meta.ActualModelName
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
		return errors.Wrap(err, "marshal response API response")
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(status)
	_, err = c.Writer.Write(data)
	return errors.Wrap(err, "write response API response")
}

// renderChatResponseAsResponseAPIStream renders a completed Chat Completion response as a terminal Responses API SSE sequence.
func renderChatResponseAsResponseAPIStream(c *gin.Context, status int, textResp *openai_compatible.SlimTextResponse, originalReq *openai.ResponseAPIRequest, meta *metalib.Meta) error {
	c.Set(ctxkey.ResponseRewriteApplied, true)
	c.Status(status)
	common.SetEventStreamHeaders(c)

	bridge := newChatToResponseStreamBridge(c, meta, originalReq)
	choices := make([]openai_compatible.ChatCompletionsStreamResponseChoice, 0, len(textResp.Choices))
	for _, choice := range textResp.Choices {
		finishReason := choice.FinishReason
		choices = append(choices, openai_compatible.ChatCompletionsStreamResponseChoice{
			Index:        choice.Index,
			Delta:        choice.Message,
			FinishReason: &finishReason,
		})
	}

	bridge.HandleChunk(c, &openai_compatible.ChatCompletionsStreamResponse{
		Object:  "chat.completion.chunk",
		Model:   meta.ActualModelName,
		Choices: choices,
		Usage:   &textResp.Usage,
	})
	bridge.FinalizeUsage(&textResp.Usage)
	bridge.HandleDone(c)
	return nil
}

// generateResponseAPIID generates a unique ID for a Response API response
func generateResponseAPIID(c *gin.Context, _ *openai_compatible.SlimTextResponse) string {
	if reqID := c.GetString(ctxkey.RequestId); reqID != "" {
		return fmt.Sprintf("resp-%s", reqID)
	}
	return fmt.Sprintf("resp-%d", time.Now().UnixNano())
}

// deriveResponseStatus derives the status of a Response API response from Chat Completion choices
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

// buildResponseOutput builds the output items for a Response API response from Chat Completion choices.
// The returned slice is always non-nil so that JSON serialization yields "output": [] instead of null,
// which strict OpenAI SDK clients reject as a violation of the Responses API schema. Parameters: lg
// reports envelope failures, choices are the Chat choices and catalog restores fallback tool identities.
// Claude thinking blocks become one reasoning item each, placed before the text or function_call they
// preceded, mirroring the streaming bridge's output_index order: reasoning, searches, message, calls.
func buildResponseOutput(lg glog.Logger, choices []openai_compatible.TextResponseChoice, catalog *openai.ResponseFallbackToolCatalog) []openai.OutputItem {
	output := make([]openai.OutputItem, 0)
	for _, choice := range choices {
		msg := choice.Message
		blocks := msg.ClaudeThinkingBlocks
		nextBlock := 0
		appendBlocks := func(ready func(relaymodel.ClaudeThinkingBlock) bool) {
			for nextBlock < len(blocks) && ready(blocks[nextBlock]) {
				block := blocks[nextBlock]
				output = append(output, newClaudeThinkingReasoningItem(lg, newClaudeThinkingReasoningID(), block, strings.TrimSpace(block.Thinking)))
				nextBlock++
			}
		}
		// Items follow the order the model produced them, as the streaming
		// bridge emits them: reasoning, then provider-executed searches, then
		// the answer text, then caller tool calls with the thinking that
		// preceded each of them. Verbatim Claude blocks supersede the joined
		// reasoning text.
		if reasoning := extractReasoning(msg); len(blocks) == 0 && reasoning != "" {
			output = append(output, openai.OutputItem{
				Id:     "rs_" + random.GetRandomString(16),
				Type:   "reasoning",
				Status: "completed",
				Content: []openai.OutputContent{
					{Type: "reasoning_text", Text: reasoning},
				},
				Summary: []openai.OutputContent{
					{Type: "summary_text", Text: reasoning},
				},
			})
		}
		appendBlocks(func(block relaymodel.ClaudeThinkingBlock) bool { return block.ToolCallsBefore == 0 && !block.AfterText })
		output = append(output, webSearchCallOutputItems(msg.WebSearchCalls)...)
		if contents := convertMessageContent(msg); len(contents) > 0 {
			// Every output item carries an id (clients key items on it); gateway
			// state, when active, replaces it with a stable gateway item ID.
			output = append(output, openai.OutputItem{
				Id:      "msg_" + random.GetRandomString(16),
				Type:    "message",
				Role:    "assistant",
				Status:  "completed",
				Content: contents,
			})
		}

		for toolIndex, tool := range msg.ToolCalls {
			appendBlocks(func(block relaymodel.ClaudeThinkingBlock) bool { return block.ToolCallsBefore <= toolIndex })
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
			// Mirror the streaming bridge's function_call item exactly: both id and
			// call_id must be present and carry the same normalized call ID. A client
			// that keys the tool call on either field has to see the same value from
			// the streamed and the non-streamed fallback of the same channel.
			callID := ensureResponseAPICallID(tool.Id)
			name := ""
			if tool.Function != nil {
				name = tool.Function.Name
			}
			// The catalog restores namespaced, custom and tool_search identities
			// that fallback flattened into ordinary Chat function tools.
			item := catalog.ResponseFallbackToolCallItem(callID, name, arguments)
			item.ExtraContent = tool.ExtraContent
			output = append(output, item)
		}
		appendBlocks(func(relaymodel.ClaudeThinkingBlock) bool { return true })
	}
	return output
}

// buildRequiredActionToolCalls builds the required action tool calls for a Response API response from Chat Completion choices
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
				ExtraContent: tool.ExtraContent,
				Id:           callID,
				Type:         "function",
				Function: &openai.ResponseAPIFunctionCall{
					Name:      tool.Function.Name,
					Arguments: arguments,
				},
			})
		}
	}
	return toolCalls
}

// ensureResponseAPICallID ensures that a tool call ID is in the format expected by the Response API
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

// convertMessageContent converts a Chat Completion message content to Response API output content
func convertMessageContent(msg relaymodel.Message) []openai.OutputContent {
	var contents []openai.OutputContent
	if msg.IsStringContent() {
		text := strings.TrimSpace(msg.StringContent())
		if msg.ExtraContent != nil {
			text = msg.StringContent()
		}
		if text != "" || msg.ExtraContent != nil {
			contents = append(contents, openai.OutputContent{Type: "output_text", Text: text, ExtraContent: msg.ExtraContent})
		}
		return contents
	}

	for _, part := range responseGoogleTextParts(msg) {
		switch part.Type {
		case relaymodel.ContentTypeText:
			if part.Text != nil && (*part.Text != "" || part.ExtraContent != nil) {
				contents = append(contents, openai.OutputContent{Type: "output_text", Text: *part.Text, ExtraContent: part.ExtraContent})
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

// extractReasoning extracts reasoning content from a Chat Completion message
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

// responseToolsEcho returns the request's tools in the Response object's spec
// shape for a synthesized response. Parameters: lg reports an encoding failure
// and tools are the request's definitions. It returns nil (rendered as an
// empty tools array) when there are none or they cannot be encoded.
func responseToolsEcho(lg glog.Logger, tools []openai.ResponseAPITool) json.RawMessage {
	echo, err := openai.ResponseToolsEcho(tools)
	if err != nil {
		lg.Warn("encode response tools echo", zap.Error(err))
		return nil
	}
	return echo
}
