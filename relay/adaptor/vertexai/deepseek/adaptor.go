// Package deepseek provides an adaptor for the DeepSeek AI models in Vertex AI.
package deepseek

import (
	"net/http"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	openai_compatible "github.com/songquanpeng/one-api/relay/adaptor/openai_compatible"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

// Adaptor is an implementation of the DeepSeek AI adaptor for Vertex AI.
type Adaptor struct{}

// ConvertRequest converts an OpenAI request to a DeepSeek-compatible request.
// DeepSeek is OpenAI-compatible but requires field mapping, particularly for token limits.
// This function handles the conversion by:
//
//  1. Creating a copy of the original request to avoid modification
//  2. Mapping max_completion_tokens to max_tokens if needed (DeepSeek only supports max_tokens)
//  3. Clearing max_completion_tokens to avoid conflicts
func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	// DeepSeek is OpenAI-compatible but requires field mapping
	if request == nil {
		return nil, errors.New("request is nil")
	}

	// Create a copy of the request to avoid modifying the original
	deepseekRequest := *request

	// DeepSeek doesn't support max_completion_tokens, only max_tokens
	// If max_completion_tokens is set but max_tokens is not, use max_completion_tokens as max_tokens
	if deepseekRequest.MaxCompletionTokens != nil && deepseekRequest.MaxTokens == 0 {
		deepseekRequest.MaxTokens = *deepseekRequest.MaxCompletionTokens
	}

	// Clear max_completion_tokens to avoid conflicts
	deepseekRequest.MaxCompletionTokens = nil

	return &deepseekRequest, nil
}

// ConvertImageRequest handles image generation requests for DeepSeek.
// Currently, DeepSeek does not support image generation, so this function always returns an error.
func (a *Adaptor) ConvertImageRequest(c *gin.Context, request *model.ImageRequest) (any, error) {
	// DeepSeek doesn't support image generation currently
	return nil, errors.New("Vertex AI: deepseek does not support image generation")
}

// DoResponse handles the response from DeepSeek API and converts it to a standard format.
// Since DeepSeek is OpenAI-compatible, it uses the OpenAI-compatible response handlers.
// For streaming responses, it uses the StreamHandler, otherwise the standard Handler.
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	// Use OpenAI-compatible response handling since DeepSeek is OpenAI-compatible
	return openai_compatible.HandleClaudeMessagesResponse(c, resp, meta, func(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
		if meta.IsStream {
			// TODO: This may need to return "openai_compatible.StreamHandlerWithThinking".
			// Note: Vertex AI uses Unicode-escaped thinking tags (e.g., \u003cthink\u003e...\u003c/think\u003e)
			// instead of raw XML-style tags like <think></think>.
			return openai_compatible.StreamHandler(c, resp, promptTokens, modelName)
		}
		return openai_compatible.Handler(c, resp, promptTokens, modelName)
	})
}
