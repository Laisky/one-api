// Package qwen provides an adaptor for Qwen models in Vertex AI.
package qwen

import (
	"net/http"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	openai_compatible "github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// Adaptor is an implementation of the Qwen adaptor for Vertex AI.
type Adaptor struct{}

// ConvertRequest converts an OpenAI request to a Qwen-compatible request.
// Qwen models are fully OpenAI-compatible but require field mapping for token limits.
//
// This function handles the conversion by:
//  1. Creating a copy of the original request to avoid modification
//  2. Mapping max_completion_tokens to max_tokens if needed (Qwen only supports max_tokens)
//  3. Clearing max_completion_tokens to avoid conflicts
func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	// Qwen models are fully OpenAI-compatible
	if request == nil {
		return nil, errors.New("request is nil")
	}

	// Create a copy of the request to avoid modifying the original
	qwenRequest := *request

	// Qwen doesn't support max_completion_tokens, only max_tokens
	// If max_completion_tokens is set but max_tokens is not, use max_completion_tokens as max_tokens
	if qwenRequest.MaxCompletionTokens != nil && qwenRequest.MaxTokens == 0 {
		qwenRequest.MaxTokens = *qwenRequest.MaxCompletionTokens
	}

	// Clear max_completion_tokens to avoid conflicts
	qwenRequest.MaxCompletionTokens = nil

	return &qwenRequest, nil
}

// ConvertImageRequest handles image generation requests for Qwen models.
// Currently, Qwen models do not support image generation, so this function always returns an error.
func (a *Adaptor) ConvertImageRequest(c *gin.Context, request *model.ImageRequest) (any, error) {
	// Qwen doesn't support image generation currently
	return nil, errors.New("Vertex AI: Qwen does not support image generation")
}

// DoResponse handles the response from Qwen API and converts it to a standard format.
// Since Qwen models are fully OpenAI-compatible, it uses the standard OpenAI-compatible response handlers.
// For streaming responses, it uses the StreamHandler, otherwise the standard Handler.
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	// Use standard OpenAI-compatible response handling since Qwen is OpenAI-compatible
	if meta.IsStream {
		// TODO: This may need to return "openai_compatible.StreamHandlerWithThinking".
		// Note: Vertex AI uses Unicode-escaped thinking tags (e.g., \u003cthink\u003e...\u003c/think\u003e)
		// instead of raw XML-style tags like <think></think>.
		err, usage = openai_compatible.StreamHandler(c, resp, meta.PromptTokens, meta.ActualModelName)
	} else {
		err, usage = openai_compatible.Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
	}
	return usage, err
}
