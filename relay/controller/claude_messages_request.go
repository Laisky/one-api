package controller

import (
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
