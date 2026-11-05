package gemini

import (
	"encoding/json"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/model"
)

// ConvertClaudeToNativeChat shares the lossless content lowering between Gemini
// Developer API and Vertex. The source request remains owned by the caller.
func ConvertClaudeToNativeChat(c *gin.Context, request *model.ClaudeRequest) (*model.GeneralOpenAIRequest, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	converted, err := openai_compatible.ConvertClaudeRequestWithNativeMedia(c, request)
	if err != nil {
		return nil, err
	}
	chat, ok := converted.(*model.GeneralOpenAIRequest)
	if !ok || chat == nil {
		return nil, errors.New("invalid Claude conversion")
	}
	chat.ResponseFormat = claudeStructuredResponseFormat(request)
	if chat.ResponseFormat != nil {
		chat.Tools, chat.ToolChoice = nil, nil
	}
	if len(request.OutputConfig) > 0 && string(request.OutputConfig) != "null" {
		var controls struct {
			Effort *string `json:"effort"`
		}
		if err := json.Unmarshal(request.OutputConfig, &controls); err != nil {
			return nil, errors.New("validation failed: invalid Claude output_config effort")
		}
		chat.ReasoningEffort = controls.Effort
	}
	return chat, nil
}
