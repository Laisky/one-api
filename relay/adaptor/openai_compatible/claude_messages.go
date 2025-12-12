package openai_compatible

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

// ConvertClaudeRequest converts Claude Messages API request to OpenAI format for OpenAI-compatible adapters
func ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	// Convert Claude Messages API request to OpenAI format first
	openaiRequest := &model.GeneralOpenAIRequest{
		Model:               request.Model,
		MaxCompletionTokens: &request.MaxTokens,
		Temperature:         request.Temperature,
		TopP:                request.TopP,
		Stream:              request.Stream != nil && *request.Stream,
		Stop:                request.StopSequences,
	}

	schemaName, schemaPayload, schemaDescription, promoteStructured := detectStructuredToolSchema(request)
	var metaInfo *meta.Meta
	if c != nil {
		if cached, exists := c.Get(ctxkey.Meta); exists {
			if stored, ok := cached.(*meta.Meta); ok {
				metaInfo = stored
			}
		} else if c.Request != nil && c.Request.URL != nil {
			metaInfo = meta.GetByContext(c)
		}
	}
	if promoteStructured && structuredPromotionDisabled(metaInfo) {
		promoteStructured = false
	}
	if promoteStructured {
		strict := true
		openaiRequest.ResponseFormat = &model.ResponseFormat{
			Type: "json_schema",
			JsonSchema: &model.JSONSchema{
				Name:        schemaName,
				Description: schemaDescription,
				Schema:      schemaPayload,
				Strict:      &strict,
			},
		}
		openaiRequest.ToolChoice = nil
	}

	// Convert system message if present
	if request.System != nil {
		switch system := request.System.(type) {
		case string:
			if system != "" {
				openaiRequest.Messages = append(openaiRequest.Messages, model.Message{
					Role:    "system",
					Content: system,
				})
			}
		case []any:
			// Extract text parts and join; ignore non-text
			var parts []string
			for _, block := range system {
				if blockMap, ok := block.(map[string]any); ok {
					if t, ok := blockMap["type"].(string); ok && t == "text" {
						if text, exists := blockMap["text"]; exists {
							if textStr, ok := text.(string); ok && textStr != "" {
								parts = append(parts, textStr)
							}
						}
					}
				}
			}
			if len(parts) > 0 {
				openaiRequest.Messages = append(openaiRequest.Messages, model.Message{
					Role:    "system",
					Content: strings.Join(parts, "\n"),
				})
			}
		}
	}

	// Convert messages
	for _, msg := range request.Messages {
		converted := convertClaudeMessageToOpenAI(msg)
		openaiRequest.Messages = append(openaiRequest.Messages, converted...)
	}

	// Convert tools if present
	if !promoteStructured && len(request.Tools) > 0 {
		var tools []model.Tool
		for _, claudeTool := range request.Tools {
			tool := model.Tool{
				Type: "function",
				Function: &model.Function{
					Name:        claudeTool.Name,
					Description: claudeTool.Description,
					Parameters:  claudeTool.InputSchema.(map[string]any),
				},
			}
			tools = append(tools, tool)
		}
		openaiRequest.Tools = tools
	}

	// Convert tool choice if present
	if request.ToolChoice != nil {
		openaiRequest.ToolChoice = normalizeClaudeToolChoice(request.ToolChoice)
		if promoteStructured {
			openaiRequest.ToolChoice = nil
		}
	}

	// Mark this as a Claude Messages conversion for response handling
	c.Set(ctxkey.ClaudeMessagesConversion, true)
	c.Set(ctxkey.OriginalClaudeRequest, request)

	return openaiRequest, nil
}

func structuredPromotionDisabled(metaInfo *meta.Meta) bool {
	if metaInfo == nil {
		return false
	}

	lowerModel := strings.ToLower(strings.TrimSpace(metaInfo.ActualModelName))
	switch metaInfo.ChannelType {
	case channeltype.DeepSeek:
		return true
	case channeltype.OpenAICompatible:
		if strings.Contains(lowerModel, "deepseek") {
			return true
		}
	}

	return false
}

type pendingOpenAIMessage struct {
	message      model.Message
	contentParts []model.MessageContent
}

func convertClaudeMessageToOpenAI(msg model.ClaudeMessage) []model.Message {
	switch content := msg.Content.(type) {
	case string:
		return []model.Message{{Role: msg.Role, Content: content}}
	case []any:
		return convertClaudeBlocks(msg.Role, content)
	default:
		if b, err := json.Marshal(content); err == nil {
			return []model.Message{{Role: msg.Role, Content: string(b)}}
		}
		return []model.Message{{Role: msg.Role, Content: ""}}
	}
}

func convertClaudeBlocks(role string, blocks []any) []model.Message {
	var (
		result  []model.Message
		pending *pendingOpenAIMessage
	)

	flush := func() {
		if pending == nil {
			return
		}
		msg := pending.message
		if len(pending.contentParts) > 0 {
			snapshot := make([]model.MessageContent, len(pending.contentParts))
			copy(snapshot, pending.contentParts)
			msg.Content = snapshot
		}
		if msg.Content == nil && len(msg.ToolCalls) > 0 {
			msg.Content = ""
		}
		if msg.Content != nil || len(msg.ToolCalls) > 0 || msg.ToolCallId != "" {
			result = append(result, msg)
		}
		pending = nil
	}

	ensurePending := func() *pendingOpenAIMessage {
		if pending == nil {
			pending = &pendingOpenAIMessage{message: model.Message{Role: role}}
		}
		return pending
	}

	for _, raw := range blocks {
		blockMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		bt, _ := blockMap["type"].(string)
		switch bt {
		case "text":
			if text, ok := blockMap["text"].(string); ok {
				msg := ensurePending()
				textCopy := text
				msg.contentParts = append(msg.contentParts, model.MessageContent{Type: model.ContentTypeText, Text: &textCopy})
			}
		case "image":
			if source, exists := blockMap["source"].(map[string]any); exists {
				msg := ensurePending()
				sourceType, _ := source["type"].(string)
				switch sourceType {
				case "base64":
					if mt, ok := source["media_type"].(string); ok {
						if data, ok := source["data"].(string); ok {
							url := fmt.Sprintf("data:%s;base64,%s", mt, data)
							imageURL := model.ImageURL{Url: url}
							msg.contentParts = append(msg.contentParts, model.MessageContent{Type: model.ContentTypeImageURL, ImageURL: &imageURL})
						}
					}
				case "url":
					if urlStr, ok := source["url"].(string); ok {
						imageURL := model.ImageURL{Url: urlStr}
						if detail, ok := source["detail"].(string); ok {
							imageURL.Detail = detail
						}
						msg.contentParts = append(msg.contentParts, model.MessageContent{Type: model.ContentTypeImageURL, ImageURL: &imageURL})
					}
				}
			}
		case "tool_use":
			id, _ := blockMap["id"].(string)
			name, _ := blockMap["name"].(string)
			msg := ensurePending()
			var argsStr string
			if input := blockMap["input"]; input != nil {
				if inputBytes, err := json.Marshal(input); err == nil {
					argsStr = string(inputBytes)
				}
			}
			msg.message.ToolCalls = append(msg.message.ToolCalls, model.Tool{
				Id:   id,
				Type: "function",
				Function: &model.Function{
					Name:      name,
					Arguments: argsStr,
				},
			})
		case "tool_result":
			flush()
			if toolMsg := convertClaudeToolResultBlock(blockMap); toolMsg != nil {
				result = append(result, *toolMsg)
			}
		default:
			// Preserve unexpected blocks as JSON text to avoid silent data loss
			if len(blockMap) > 0 {
				msg := ensurePending()
				if encoded, err := json.Marshal(blockMap); err == nil {
					text := string(encoded)
					msg.contentParts = append(msg.contentParts, model.MessageContent{Type: model.ContentTypeText, Text: &text})
				}
			}
		}
	}

	flush()
	return result
}

func convertClaudeToolResultBlock(block map[string]any) *model.Message {
	if block == nil {
		return nil
	}
	toolCallID, _ := block["tool_call_id"].(string)
	if toolCallID == "" {
		toolCallID, _ = block["tool_use_id"].(string)
	}
	if toolCallID == "" {
		toolCallID, _ = block["id"].(string)
	}

	contentStr := extractClaudeToolResultContent(block["content"])
	if contentStr == "" {
		if text, ok := block["text"].(string); ok {
			contentStr = text
		}
	}

	toolMsg := model.Message{
		Role:       "tool",
		ToolCallId: toolCallID,
	}
	if contentStr != "" {
		toolMsg.Content = contentStr
	} else {
		toolMsg.Content = ""
	}
	if name, ok := block["name"].(string); ok && name != "" {
		toolMsg.Name = &name
	}
	return &toolMsg
}

func extractClaudeToolResultContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var builder strings.Builder
		for _, raw := range v {
			itemMap, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			typeStr, _ := itemMap["type"].(string)
			if strings.EqualFold(typeStr, "text") {
				if txt, ok := itemMap["text"].(string); ok {
					builder.WriteString(txt)
				}
				continue
			}
			if encoded, err := json.Marshal(itemMap); err == nil {
				builder.WriteString(string(encoded))
			}
		}
		return builder.String()
	case map[string]any:
		if encoded, err := json.Marshal(v); err == nil {
			return string(encoded)
		}
	}
	if content == nil {
		return ""
	}
	if encoded, err := json.Marshal(content); err == nil {
		return string(encoded)
	}
	return fmt.Sprintf("%v", content)
}

// normalizeClaudeToolChoice adapts Claude tool_choice payloads to the OpenAI ChatCompletion schema.
// Claude clients often set type=tool with a top-level name; OpenAI-compatible upstreams expect
// type=function with the name nested under the function field.
func normalizeClaudeToolChoice(choice any) any {
	switch src := choice.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(src))
		maps.Copy(cloned, src)

		typeVal, _ := cloned["type"].(string)
		switch strings.ToLower(typeVal) {
		case "tool":
			name, _ := cloned["name"].(string)
			var fn map[string]any
			if existing, ok := cloned["function"].(map[string]any); ok {
				fn = cloneMap(existing)
			} else {
				fn = map[string]any{}
			}
			if name != "" && fn["name"] == nil {
				fn["name"] = name
			}
			if len(fn) > 0 {
				cloned["function"] = fn
			}
			cloned["type"] = "function"
			delete(cloned, "name")
		case "function":
			if name, ok := cloned["name"].(string); ok && name != "" {
				fn, _ := cloned["function"].(map[string]any)
				if fn == nil {
					fn = map[string]any{}
				}
				if fn["name"] == nil {
					fn["name"] = name
				}
				cloned["function"] = fn
				delete(cloned, "name")
			}
		}
		return cloned
	default:
		return choice
	}
}

// cloneMap returns a shallow copy of a map[string]any. It avoids mutating caller state when normalizing payloads.
func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned := make(map[string]any, len(input))
	maps.Copy(cloned, input)
	return cloned
}

// detectStructuredToolSchema inspects the Claude request for the single-tool structured output pattern.
// It returns the promoted schema name, schema payload, optional description, and a boolean indicating detection success.
func detectStructuredToolSchema(request *model.ClaudeRequest) (string, map[string]any, string, bool) {
	if request == nil {
		return "", nil, "", false
	}
	if len(request.Tools) != 1 {
		return "", nil, "", false
	}
	if containsClaudeToolUsage(request.Messages) {
		return "", nil, "", false
	}
	tool := request.Tools[0]
	choiceName, hasChoice := extractToolChoiceName(request.ToolChoice)
	if !hasChoice || !strings.EqualFold(choiceName, tool.Name) {
		return "", nil, "", false
	}
	schemaMap, ok := tool.InputSchema.(map[string]any)
	if !ok || len(schemaMap) == 0 {
		return "", nil, "", false
	}
	if !schemaIndicatesStructured(schemaMap) {
		return "", nil, "", false
	}
	if !hasStructuredIntent(request, tool.Description) {
		return "", nil, "", false
	}
	return tool.Name, deepCopyMapAny(schemaMap), strings.TrimSpace(tool.Description), true
}

// extractToolChoiceName extracts the tool name from a Claude tool_choice payload and reports whether a name was found.
func extractToolChoiceName(toolChoice any) (string, bool) {
	switch v := toolChoice.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return "", false
		}
		return v, true
	case map[string]any:
		if name, ok := v["name"].(string); ok && strings.TrimSpace(name) != "" {
			return name, true
		}
		if fn, ok := v["function"].(map[string]any); ok {
			if name, ok := fn["name"].(string); ok && strings.TrimSpace(name) != "" {
				return name, true
			}
		}
	}
	return "", false
}

// containsClaudeToolUsage reports whether the Claude message list already embeds tool_use or tool_result blocks.
// Such requests represent real tool invocation flows and should not be promoted to structured output.
func containsClaudeToolUsage(messages []model.ClaudeMessage) bool {
	for _, msg := range messages {
		switch content := msg.Content.(type) {
		case []any:
			for _, entry := range content {
				block, ok := entry.(map[string]any)
				if !ok {
					continue
				}
				typeStr, _ := block["type"].(string)
				if strings.EqualFold(typeStr, "tool_use") || strings.EqualFold(typeStr, "tool_result") {
					return true
				}
			}
		}
	}
	return false
}

// deepCopyMapAny creates a deep copy of a map[string]any to prevent accidental mutation of the caller-provided schema.
func deepCopyMapAny(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	dup := make(map[string]any, len(input))
	for k, v := range input {
		dup[k] = deepCopyValue(v)
	}
	return dup
}

// deepCopySliceAny creates a deep copy of a []any to support deepCopyMapAny recursion.
func deepCopySliceAny(input []any) []any {
	if input == nil {
		return nil
	}
	dup := make([]any, len(input))
	for idx, v := range input {
		dup[idx] = deepCopyValue(v)
	}
	return dup
}

// deepCopyValue performs a recursive deep copy for arbitrary JSON-like structures.
func deepCopyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return deepCopyMapAny(typed)
	case []any:
		return deepCopySliceAny(typed)
	default:
		return typed
	}
}

// schemaIndicatesStructured ensures the schema includes explicit structured-output hints such as additionalProperties=false.
func schemaIndicatesStructured(schema map[string]any) bool {
	if schema == nil {
		return false
	}
	apRaw, exists := schema["additionalProperties"]
	if !exists {
		return false
	}
	ap, ok := apRaw.(bool)
	if !ok || ap {
		return false
	}
	return true
}

// hasStructuredIntent inspects request messages and tool metadata for keywords suggesting structured JSON output.
func hasStructuredIntent(request *model.ClaudeRequest, toolDescription string) bool {
	keywords := []string{"json", "structured", "schema", "fields"}
	if containsKeyword(toolDescription, keywords) {
		return true
	}
	if request == nil {
		return false
	}
	if request.System != nil {
		if containsKeyword(extractClaudeContentText(request.System), keywords) {
			return true
		}
	}
	for _, msg := range request.Messages {
		if containsKeyword(extractClaudeContentText(msg.Content), keywords) {
			return true
		}
	}
	return false
}

// containsKeyword reports whether the provided text contains any of the keywords (case-insensitive).
func containsKeyword(text string, keywords []string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	lower := strings.ToLower(text)
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// extractClaudeContentText flattens Claude content payloads to a single text blob for keyword matching.
func extractClaudeContentText(content any) string {
	var parts []string
	collectClaudeText(content, &parts)
	return strings.Join(parts, "\n")
}

// collectClaudeText recursively gathers text fields from Claude content blocks.
func collectClaudeText(content any, parts *[]string) {
	switch val := content.(type) {
	case string:
		if strings.TrimSpace(val) != "" {
			*parts = append(*parts, val)
		}
	case []any:
		for _, entry := range val {
			collectClaudeText(entry, parts)
		}
	case map[string]any:
		if text, ok := val["text"].(string); ok && strings.TrimSpace(text) != "" {
			*parts = append(*parts, text)
		}
		if content, ok := val["content"].(any); ok {
			collectClaudeText(content, parts)
		}
	}
}

// HandleClaudeMessagesResponse handles Claude Messages response conversion for OpenAI-compatible adapters
// This should be called in the adapter's DoResponse method when ClaudeMessagesConversion flag is set
func HandleClaudeMessagesResponse(c *gin.Context, resp *http.Response, meta *meta.Meta, handler func(*gin.Context, *http.Response, int, string) (*model.ErrorWithStatusCode, *model.Usage)) (*model.Usage, *model.ErrorWithStatusCode) {
	// Check if this is a Claude Messages conversion
	if isClaudeConversion, exists := c.Get(ctxkey.ClaudeMessagesConversion); !exists || !isClaudeConversion.(bool) {
		// Not a Claude Messages conversion, proceed normally
		err, usage := handler(c, resp, meta.PromptTokens, meta.ActualModelName)
		return usage, err
	}

	// Claude Messages conversion path
	if meta.IsStream {
		// Convert OpenAI-compatible SSE to Claude-native SSE, write to client, return usage
		usage, convErr := ConvertOpenAIStreamToClaudeSSE(c, resp, meta.PromptTokens, meta.ActualModelName)
		if convErr != nil {
			return nil, convErr
		}
		return usage, nil
	}

	// Non-stream: convert to Claude JSON and let controller forward it
	claudeResp, convErr := ConvertOpenAIResponseToClaudeResponse(c, resp)
	if convErr != nil {
		return nil, convErr
	}
	c.Set(ctxkey.ConvertedResponse, claudeResp)
	return nil, nil
}
