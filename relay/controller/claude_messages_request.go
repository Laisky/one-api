package controller

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/relay/adaptor/anthropic"
)

// sanitizeClaudeMessagesRequest enforces parameter constraints required by upstream providers.
func sanitizeClaudeMessagesRequest(request *ClaudeMessagesRequest) {
	if request == nil {
		return
	}
	anthropic.NormalizeModelCompatibility(request.Model, &request.Temperature, &request.TopP, &request.TopK, &request.Thinking)
}

// applyClaudeRequestRewriteFields rewrites sanitized top-level Claude request fields in obj.
// It updates only the top-level fields that one-api intentionally normalizes and returns any rewrite error.
func applyClaudeRequestRewriteFields(obj map[string]json.RawMessage, request *ClaudeMessagesRequest) error {
	if request.Model != "" {
		modelBytes, merr := json.Marshal(request.Model)
		if merr != nil {
			return errors.Wrap(merr, "marshal model name")
		}
		obj["model"] = json.RawMessage(modelBytes)
	}

	delete(obj, "extra_body")
	if request.Temperature == nil {
		delete(obj, "temperature")
	}
	if request.TopP == nil {
		delete(obj, "top_p")
	}
	if request.TopK == nil {
		delete(obj, "top_k")
	}

	if anthropic.IsClaudeOpus47Model(request.Model) {
		rewrittenThinking, changed, err := rewriteClaudeOpus47Thinking(obj["thinking"])
		if err != nil {
			return errors.Wrap(err, "rewrite Claude Opus 4.7 thinking")
		}
		if changed {
			if len(rewrittenThinking) == 0 {
				delete(obj, "thinking")
			} else {
				obj["thinking"] = rewrittenThinking
			}
		}
	}

	return nil
}

// rewriteClaudeOpus47Thinking normalizes a raw thinking object for Claude Opus 4.7.
// It preserves valid adaptive settings where possible, rewrites legacy manual thinking to adaptive, and returns whether the raw field changed.
func rewriteClaudeOpus47Thinking(rawThinking json.RawMessage) (json.RawMessage, bool, error) {
	if len(rawThinking) == 0 {
		return nil, false, nil
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(rawThinking, &obj); err != nil {
		rewritten, merr := json.Marshal(map[string]string{"type": "adaptive"})
		if merr != nil {
			return nil, false, errors.Wrap(merr, "marshal adaptive thinking")
		}
		return json.RawMessage(rewritten), true, nil
	}

	var thinkingType string
	if rawType, ok := obj["type"]; ok {
		if err := json.Unmarshal(rawType, &thinkingType); err != nil {
			return nil, false, errors.Wrap(err, "unmarshal thinking type")
		}
	}

	if strings.EqualFold(strings.TrimSpace(thinkingType), "adaptive") {
		if _, ok := obj["budget_tokens"]; !ok {
			return rawThinking, false, nil
		}
		delete(obj, "budget_tokens")
		rewritten, err := encodeClaudeRawJSONObject(obj)
		if err != nil {
			return nil, false, errors.Wrap(err, "marshal adaptive thinking")
		}
		return json.RawMessage(rewritten), true, nil
	}

	rewritten, err := json.Marshal(map[string]string{"type": "adaptive"})
	if err != nil {
		return nil, false, errors.Wrap(err, "marshal adaptive thinking")
	}
	return json.RawMessage(rewritten), true, nil
}

// rewriteClaudeRequestBody updates the raw JSON payload to reflect sanitized request fields.
//
// IMPORTANT: This function uses map[string]json.RawMessage to preserve the exact byte
// representation of nested values (especially the "messages" array). This is critical
// because Claude's extended thinking feature includes cryptographic "signature" fields
// in thinking blocks. A full json.Unmarshal -> map[string]any -> json.Marshal round-trip
// would corrupt these signatures due to:
//   - Go's default HTML escaping of <, >, & characters in strings
//   - Potential floating-point precision loss for numbers
//   - Key reordering within nested objects
//
// By using json.RawMessage, only the top-level fields we explicitly modify (model,
// extra_body, temperature, top_p, top_k, and Opus 4.7 thinking) are re-encoded;
// all other fields pass through byte-for-byte.
func rewriteClaudeRequestBody(raw []byte, request *ClaudeMessagesRequest) ([]byte, error) {
	if len(raw) == 0 || request == nil {
		return raw, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, errors.Wrap(err, "unmarshal raw claude body for rewrite")
	}
	if err := applyClaudeRequestRewriteFields(obj, request); err != nil {
		return nil, errors.Wrap(err, "apply Claude rewrite fields")
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(obj); err != nil {
		return nil, errors.Wrap(err, "marshal rewritten claude body")
	}
	// json.Encoder.Encode appends a trailing newline; trim it
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// rewriteAndSanitizeClaudeRequestBody combines rewriteClaudeRequestBody and
// stripClaudeUnsignedThinkingFromAssistantHistory into a single JSON parse/encode pass.
// This avoids parsing the (potentially very large) request body twice.
func rewriteAndSanitizeClaudeRequestBody(raw []byte, request *ClaudeMessagesRequest) ([]byte, claudeUnsignedThinkingStats, error) {
	var stats claudeUnsignedThinkingStats
	if len(raw) == 0 || request == nil {
		return raw, stats, nil
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, stats, errors.Wrap(err, "unmarshal raw claude body for rewrite+sanitize")
	}

	// --- rewrite phase (model, extra_body, sampling, thinking) ---
	if err := applyClaudeRequestRewriteFields(obj, request); err != nil {
		return nil, stats, errors.Wrap(err, "apply Claude rewrite fields")
	}

	// --- sanitize phase (strip unsigned thinking blocks) ---
	rawMessages, hasMessages := obj["messages"]
	if hasMessages {
		var messages []json.RawMessage
		if err := json.Unmarshal(rawMessages, &messages); err != nil {
			return nil, stats, errors.Wrap(err, "unmarshal messages for unsigned thinking sanitization")
		}

		keptMessages := make([]json.RawMessage, 0, len(messages))
		for messageIndex, messageRaw := range messages {
			sanitizedMessage, messageStats, keepMessage, err := stripClaudeUnsignedThinkingFromAssistantMessage(messageRaw, messageIndex)
			if err != nil {
				return nil, stats, errors.Wrap(err, "sanitize Claude assistant message for unsigned thinking")
			}
			stats.RemovedThinkingBlocks += messageStats.RemovedThinkingBlocks
			stats.RemovedAssistantMessages += messageStats.RemovedAssistantMessages
			stats.Locations = append(stats.Locations, messageStats.Locations...)
			if keepMessage {
				keptMessages = append(keptMessages, sanitizedMessage)
			}
		}

		if stats.RemovedThinkingBlocks > 0 {
			encodedMessages, err := encodeClaudeRawJSONArray(keptMessages)
			if err != nil {
				return nil, stats, errors.Wrap(err, "marshal sanitized messages")
			}
			obj["messages"] = json.RawMessage(encodedMessages)
		}
	}

	// --- encode final result ---
	result, err := encodeClaudeRawJSONObject(obj)
	if err != nil {
		return nil, stats, errors.Wrap(err, "marshal rewritten+sanitized claude body")
	}
	return result, stats, nil
}

// countThinkingSignatures counts the number of "signature" fields co-located
// with "thinking" type blocks in the raw JSON body. Used for debug logging only.
func countThinkingSignatures(raw []byte) int {
	count := 0
	// Simple heuristic: count occurrences of "signature" near "thinking" type blocks
	idx := 0
	signatureKey := []byte(`"signature"`)
	for {
		pos := bytes.Index(raw[idx:], signatureKey)
		if pos < 0 {
			break
		}
		count++
		idx += pos + len(signatureKey)
	}
	return count
}

type claudeSignatureRetryStats struct {
	RemovedThinkingBlocks    int
	RemovedAssistantMessages int
}

type claudeUnsignedThinkingStats struct {
	RemovedThinkingBlocks    int
	RemovedAssistantMessages int
	Locations                []string
}

// encodeClaudeRawJSONArray marshals an array of raw JSON values without altering retained elements.
// encodeClaudeRawJSONArray returns a valid JSON array containing each raw element in order.
func encodeClaudeRawJSONArray(items []json.RawMessage) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, item := range items {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(item)
	}
	buf.WriteByte(']')
	return buf.Bytes(), nil
}

// encodeClaudeRawJSONObject marshals a map of raw JSON fields while preserving nested raw payloads.
// encodeClaudeRawJSONObject returns a valid JSON object with HTML escaping disabled.
func encodeClaudeRawJSONObject(obj map[string]json.RawMessage) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(obj); err != nil {
		return nil, errors.Wrap(err, "marshal Claude raw object")
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// stripClaudeThinkingFromAssistantMessage removes replayed thinking blocks from an assistant message.
// stripClaudeThinkingFromAssistantMessage returns the rewritten message, removal stats, whether the message should be kept, and any error.
func stripClaudeThinkingFromAssistantMessage(messageRaw json.RawMessage) ([]byte, claudeSignatureRetryStats, bool, error) {
	var stats claudeSignatureRetryStats
	var message map[string]json.RawMessage
	if err := json.Unmarshal(messageRaw, &message); err != nil {
		return nil, stats, false, errors.Wrap(err, "unmarshal Claude message")
	}

	var role string
	if rawRole, ok := message["role"]; ok {
		if err := json.Unmarshal(rawRole, &role); err != nil {
			return nil, stats, false, errors.Wrap(err, "unmarshal Claude message role")
		}
	}
	if role != "assistant" {
		return messageRaw, stats, true, nil
	}

	rawContent, ok := message["content"]
	if !ok || len(rawContent) == 0 || rawContent[0] != '[' {
		return messageRaw, stats, true, nil
	}

	var contentBlocks []json.RawMessage
	if err := json.Unmarshal(rawContent, &contentBlocks); err != nil {
		return nil, stats, false, errors.Wrap(err, "unmarshal Claude message content blocks")
	}

	keptBlocks := make([]json.RawMessage, 0, len(contentBlocks))
	for _, blockRaw := range contentBlocks {
		var block map[string]json.RawMessage
		if err := json.Unmarshal(blockRaw, &block); err != nil {
			return nil, stats, false, errors.Wrap(err, "unmarshal Claude content block")
		}

		var blockType string
		if rawType, exists := block["type"]; exists {
			if err := json.Unmarshal(rawType, &blockType); err != nil {
				return nil, stats, false, errors.Wrap(err, "unmarshal Claude content block type")
			}
		}

		normalizedType := strings.ToLower(strings.TrimSpace(blockType))
		if normalizedType == "thinking" || normalizedType == "redacted_thinking" {
			stats.RemovedThinkingBlocks++
			continue
		}

		keptBlocks = append(keptBlocks, blockRaw)
	}

	if stats.RemovedThinkingBlocks == 0 {
		return messageRaw, stats, true, nil
	}
	if len(keptBlocks) == 0 {
		stats.RemovedAssistantMessages++
		return nil, stats, false, nil
	}

	encodedBlocks, err := encodeClaudeRawJSONArray(keptBlocks)
	if err != nil {
		return nil, stats, false, errors.Wrap(err, "marshal sanitized Claude content blocks")
	}
	message["content"] = json.RawMessage(encodedBlocks)

	encodedMessage, err := encodeClaudeRawJSONObject(message)
	if err != nil {
		return nil, stats, false, errors.Wrap(err, "marshal sanitized Claude message")
	}

	return encodedMessage, stats, true, nil
}

// stripClaudeThinkingFromAssistantHistory removes replayed thinking blocks from assistant history in a raw Claude Messages request.
// stripClaudeThinkingFromAssistantHistory preserves untouched fields where possible and returns the rewritten payload plus removal stats.
func stripClaudeThinkingFromAssistantHistory(raw []byte) ([]byte, claudeSignatureRetryStats, error) {
	var stats claudeSignatureRetryStats
	if len(raw) == 0 {
		return raw, stats, nil
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, stats, errors.Wrap(err, "unmarshal Claude request for signature retry")
	}

	rawMessages, ok := obj["messages"]
	if !ok {
		return raw, stats, nil
	}

	var messages []json.RawMessage
	if err := json.Unmarshal(rawMessages, &messages); err != nil {
		return nil, stats, errors.Wrap(err, "unmarshal Claude request messages for signature retry")
	}

	keptMessages := make([]json.RawMessage, 0, len(messages))
	for _, messageRaw := range messages {
		sanitizedMessage, messageStats, keepMessage, err := stripClaudeThinkingFromAssistantMessage(messageRaw)
		if err != nil {
			return nil, stats, errors.Wrap(err, "sanitize Claude assistant message for signature retry")
		}
		stats.RemovedThinkingBlocks += messageStats.RemovedThinkingBlocks
		stats.RemovedAssistantMessages += messageStats.RemovedAssistantMessages
		if keepMessage {
			keptMessages = append(keptMessages, sanitizedMessage)
		}
	}

	if stats.RemovedThinkingBlocks == 0 {
		return raw, stats, nil
	}

	encodedMessages, err := encodeClaudeRawJSONArray(keptMessages)
	if err != nil {
		return nil, stats, errors.Wrap(err, "marshal sanitized Claude request messages")
	}
	obj["messages"] = json.RawMessage(encodedMessages)

	encodedRequest, err := encodeClaudeRawJSONObject(obj)
	if err != nil {
		return nil, stats, errors.Wrap(err, "marshal sanitized Claude request")
	}

	return encodedRequest, stats, nil
}

// stripClaudeUnsignedThinkingFromAssistantMessage removes assistant thinking blocks that cannot be replayed because they lack signatures.
// stripClaudeUnsignedThinkingFromAssistantMessage returns the rewritten message, removal stats, whether the message should be kept, and any error.
func stripClaudeUnsignedThinkingFromAssistantMessage(messageRaw json.RawMessage, messageIndex int) ([]byte, claudeUnsignedThinkingStats, bool, error) {
	var stats claudeUnsignedThinkingStats
	var message map[string]json.RawMessage
	if err := json.Unmarshal(messageRaw, &message); err != nil {
		return nil, stats, false, errors.Wrap(err, "unmarshal Claude message")
	}

	var role string
	if rawRole, ok := message["role"]; ok {
		if err := json.Unmarshal(rawRole, &role); err != nil {
			return nil, stats, false, errors.Wrap(err, "unmarshal Claude message role")
		}
	}
	if role != "assistant" {
		return messageRaw, stats, true, nil
	}

	rawContent, ok := message["content"]
	if !ok || len(rawContent) == 0 || rawContent[0] != '[' {
		return messageRaw, stats, true, nil
	}

	var contentBlocks []json.RawMessage
	if err := json.Unmarshal(rawContent, &contentBlocks); err != nil {
		return nil, stats, false, errors.Wrap(err, "unmarshal Claude message content blocks")
	}

	keptBlocks := make([]json.RawMessage, 0, len(contentBlocks))
	for blockIndex, blockRaw := range contentBlocks {
		var block map[string]json.RawMessage
		if err := json.Unmarshal(blockRaw, &block); err != nil {
			return nil, stats, false, errors.Wrap(err, "unmarshal Claude content block")
		}

		var blockType string
		if rawType, exists := block["type"]; exists {
			if err := json.Unmarshal(rawType, &blockType); err != nil {
				return nil, stats, false, errors.Wrap(err, "unmarshal Claude content block type")
			}
		}

		normalizedType := strings.ToLower(strings.TrimSpace(blockType))
		if normalizedType != "thinking" && normalizedType != "redacted_thinking" {
			keptBlocks = append(keptBlocks, blockRaw)
			continue
		}

		var signature string
		if rawSignature, exists := block["signature"]; exists {
			if err := json.Unmarshal(rawSignature, &signature); err != nil {
				return nil, stats, false, errors.Wrap(err, "unmarshal Claude thinking signature")
			}
		}

		if strings.TrimSpace(signature) == "" {
			stats.RemovedThinkingBlocks++
			stats.Locations = append(stats.Locations, messageLocation(messageIndex, blockIndex))
			continue
		}

		keptBlocks = append(keptBlocks, blockRaw)
	}

	if stats.RemovedThinkingBlocks == 0 {
		return messageRaw, stats, true, nil
	}
	if len(keptBlocks) == 0 {
		stats.RemovedAssistantMessages++
		return nil, stats, false, nil
	}

	encodedBlocks, err := encodeClaudeRawJSONArray(keptBlocks)
	if err != nil {
		return nil, stats, false, errors.Wrap(err, "marshal sanitized Claude content blocks")
	}
	message["content"] = json.RawMessage(encodedBlocks)

	encodedMessage, err := encodeClaudeRawJSONObject(message)
	if err != nil {
		return nil, stats, false, errors.Wrap(err, "marshal sanitized Claude message")
	}

	return encodedMessage, stats, true, nil
}

// stripClaudeUnsignedThinkingFromAssistantHistory removes replayed assistant thinking blocks that are missing required signatures.
// stripClaudeUnsignedThinkingFromAssistantHistory preserves untouched fields where possible and returns the rewritten payload plus removal stats.
func stripClaudeUnsignedThinkingFromAssistantHistory(raw []byte) ([]byte, claudeUnsignedThinkingStats, error) {
	var stats claudeUnsignedThinkingStats
	if len(raw) == 0 {
		return raw, stats, nil
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, stats, errors.Wrap(err, "unmarshal Claude request for unsigned thinking sanitization")
	}

	rawMessages, ok := obj["messages"]
	if !ok {
		return raw, stats, nil
	}

	var messages []json.RawMessage
	if err := json.Unmarshal(rawMessages, &messages); err != nil {
		return nil, stats, errors.Wrap(err, "unmarshal Claude request messages for unsigned thinking sanitization")
	}

	keptMessages := make([]json.RawMessage, 0, len(messages))
	for messageIndex, messageRaw := range messages {
		sanitizedMessage, messageStats, keepMessage, err := stripClaudeUnsignedThinkingFromAssistantMessage(messageRaw, messageIndex)
		if err != nil {
			return nil, stats, errors.Wrap(err, "sanitize Claude assistant message for unsigned thinking")
		}
		stats.RemovedThinkingBlocks += messageStats.RemovedThinkingBlocks
		stats.RemovedAssistantMessages += messageStats.RemovedAssistantMessages
		stats.Locations = append(stats.Locations, messageStats.Locations...)
		if keepMessage {
			keptMessages = append(keptMessages, sanitizedMessage)
		}
	}

	if stats.RemovedThinkingBlocks == 0 {
		return raw, stats, nil
	}

	encodedMessages, err := encodeClaudeRawJSONArray(keptMessages)
	if err != nil {
		return nil, stats, errors.Wrap(err, "marshal sanitized Claude request messages")
	}
	obj["messages"] = json.RawMessage(encodedMessages)

	encodedRequest, err := encodeClaudeRawJSONObject(obj)
	if err != nil {
		return nil, stats, errors.Wrap(err, "marshal sanitized Claude request")
	}

	return encodedRequest, stats, nil
}

func messageLocation(messageIndex, blockIndex int) string {
	return "messages[" + strconv.Itoa(messageIndex) + "].content[" + strconv.Itoa(blockIndex) + "]"
}

// getAndValidateClaudeMessagesRequest gets and validates Claude Messages API request.
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
