package controller

import (
	"bytes"
	"encoding/json"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
)

// sanitizeClaudeMessagesRequest enforces parameter constraints required by upstream providers.
func sanitizeClaudeMessagesRequest(request *ClaudeMessagesRequest) {
	if request == nil {
		return
	}
	if request.Temperature != nil && request.TopP != nil {
		request.TopP = nil
	}
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
// extra_body, top_p) are re-encoded; all other fields pass through byte-for-byte.
func rewriteClaudeRequestBody(raw []byte, request *ClaudeMessagesRequest) ([]byte, error) {
	if len(raw) == 0 || request == nil {
		return raw, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, errors.Wrap(err, "unmarshal raw claude body for rewrite")
	}
	if request.Model != "" {
		modelBytes, merr := json.Marshal(request.Model)
		if merr != nil {
			return nil, errors.Wrap(merr, "marshal model name")
		}
		obj["model"] = json.RawMessage(modelBytes)
	}
	delete(obj, "extra_body")
	if request.TopP == nil {
		delete(obj, "top_p")
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
