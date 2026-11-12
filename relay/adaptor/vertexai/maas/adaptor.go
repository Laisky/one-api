package maas

import (
	"net/http"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	openaicompat "github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// Adaptor preserves the native OpenAI-compatible MaaS schema and response envelope.
// https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/maas/call-open-model-apis
// https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/maas/capabilities/thinking
// It does not translate Anthropic thinking controls into unrelated provider fields.
type Adaptor struct{}

// ConvertRequest maps the common completion limit to MaaS max_tokens, preserving
// messages, tools, JSON Schema, and native chat_template_kwargs extra-body controls.
func (*Adaptor) ConvertRequest(c *gin.Context, mode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("Vertex MaaS request is nil")
	}
	if mode != relaymode.ChatCompletions {
		return nil, errors.New("Vertex MaaS supports Chat Completions requests only")
	}
	name := request.Model
	if c != nil {
		if value, ok := c.Get(ctxkey.Meta); ok {
			if m, ok := value.(*meta.Meta); ok && m != nil && m.ActualModelName != "" {
				name = m.ActualModelName
			}
		}
	}
	if _, _, ok := EndpointConfig(name); !ok {
		return nil, errors.New("unknown Vertex managed open model")
	}
	if request.Thinking != nil {
		return nil, errors.New("Vertex MaaS does not accept Anthropic thinking; use the model's documented native chat_template_kwargs controls")
	}
	converted := *request
	converted.Model = name
	if converted.MaxTokens == 0 && converted.MaxCompletionTokens != nil {
		converted.MaxTokens = *converted.MaxCompletionTokens
	}
	converted.MaxCompletionTokens = nil
	return &converted, nil
}

// ConvertImageRequest rejects image generation; multimodal chat input is separate.
func (*Adaptor) ConvertImageRequest(*gin.Context, *model.ImageRequest) (any, error) {
	return nil, errors.New("Vertex MaaS models do not support image generation")
}

// DoResponse uses the common OpenAI handlers, preserving tool deltas, completion
// state, usage and reasoning through the existing downstream normalization contract.
func (*Adaptor) DoResponse(c *gin.Context, resp *http.Response, m *meta.Meta) (*model.Usage, *model.ErrorWithStatusCode) {
	return openaicompat.HandleClaudeMessagesResponse(c, resp, m, func(c *gin.Context, resp *http.Response, promptTokens int, name string) (*model.ErrorWithStatusCode, *model.Usage) {
		if m.IsStream {
			return openaicompat.StreamHandler(c, resp, promptTokens, name)
		}
		return openaicompat.Handler(c, resp, promptTokens, name)
	})
}
