package palm

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

type Adaptor struct {
	adaptor.DefaultPricingMethods
}

func (a *Adaptor) Init(meta *meta.Meta) {

}

func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	return fmt.Sprintf("%s/v1beta2/models/chat-bison-001:generateMessage", meta.BaseURL), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, meta)
	req.Header.Set("x-goog-api-key", meta.APIKey)
	return nil
}

func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return ConvertRequest(*request), nil
}

func (a *Adaptor) ConvertImageRequest(_ *gin.Context, request *model.ImageRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	// Convert Claude Messages API request to OpenAI format first
	openaiRequest := &model.GeneralOpenAIRequest{
		Model:       request.Model,
		MaxTokens:   request.MaxTokens,
		Temperature: request.Temperature,
		TopP:        request.TopP,
		Stream:      request.Stream != nil && *request.Stream,
		Stop:        request.StopSequences,
	}

	// Add system message if present
	// An absent system prompt adds no turn; block prompts become their text.
	if system := openai_compatible.ClaudeSystemText(request.System); system != "" {
		systemMessage := model.Message{
			Role:    "system",
			Content: system,
		}
		openaiRequest.Messages = append(openaiRequest.Messages, systemMessage)
	}

	// Convert messages - Palm has limited support, mainly text
	for _, msg := range request.Messages {
		openaiMessage := model.Message{
			Role: msg.Role,
		}

		// Convert content - Palm only supports text content
		switch content := msg.Content.(type) {
		case string:
			openaiMessage.Content = content
		case []any:
			// Extract text from structured content
			var textParts []string
			for _, block := range content {
				if blockMap, ok := block.(map[string]any); ok {
					if blockType, exists := blockMap["type"]; exists && blockType == "text" {
						if text, exists := blockMap["text"]; exists {
							if textStr, ok := text.(string); ok {
								textParts = append(textParts, textStr)
							}
						}
					}
				}
			}
			if len(textParts) > 0 {
				openaiMessage.Content = strings.Join(textParts, "\n")
			}
		default:
			// Fallback to string representation
			openaiMessage.Content = fmt.Sprintf("%v", content)
		}

		openaiRequest.Messages = append(openaiRequest.Messages, openaiMessage)
	}

	// Mark this as a Claude Messages conversion for response handling
	c.Set(ctxkey.ClaudeMessagesConversion, true)
	c.Set(ctxkey.OriginalClaudeRequest, request)

	// Now convert using Palm's existing logic
	return a.ConvertRequest(c, relaymode.ChatCompletions, openaiRequest)
}

func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	return adaptor.DoRequestHelper(a, c, meta, requestBody)
}

// DoResponse renders the provider answer for the client and returns the
// billing usage. When the client called the Claude Messages API, the chat
// output of doChatResponse is rendered as Claude Messages through the shared
// bridge, keeping the usage unchanged. Parameters: c is the request context,
// resp the upstream response, and meta the request meta. Returns: the billing
// usage and any error.
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (*model.Usage, *model.ErrorWithStatusCode) {
	return openai_compatible.RenderChatHandlerAsClaude(c, meta, func() (*model.Usage, *model.ErrorWithStatusCode) {
		return a.doChatResponse(c, resp, meta)
	})
}

// doChatResponse renders the provider answer as OpenAI chat output (or the
// mode-specific output) on c.Writer. Parameters: c is the request context,
// resp the upstream response, and meta the request meta. Returns: the billing
// usage and any error.
func (a *Adaptor) doChatResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	if meta.IsStream {
		var responseText string
		err, responseText = StreamHandler(c, resp)
		usage = openai.ResponseText2Usage(responseText, meta.ActualModelName, meta.PromptTokens)
	} else {
		err, usage = Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
	}
	return
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return "google palm"
}

// GetDefaultModelPricing returns the pricing table this channel advertises.
//
// Without this override the embedded DefaultPricingMethods answered 2.5 USD/1M,
// ignoring the rate recorded in models.go.
//
// Return values:
//   - map[string]adaptor.ModelConfig: the audited pricing keyed by model id.
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return ModelRatios
}

// GetModelRatio returns the input ratio for modelName, falling back to the
// framework default only for models this channel does not publish.
//
// Parameters:
//   - modelName: the requested model id.
//
// Return values:
//   - float64: quota per input token.
func (a *Adaptor) GetModelRatio(modelName string) float64 {
	if price, exists := ModelRatios[modelName]; exists {
		return price.Ratio
	}
	return a.DefaultPricingMethods.GetModelRatio(modelName)
}

// GetCompletionRatio returns the output multiplier for modelName, falling back to
// the framework default only for models this channel does not publish.
//
// Parameters:
//   - modelName: the requested model id.
//
// Return values:
//   - float64: output-to-input price multiplier.
func (a *Adaptor) GetCompletionRatio(modelName string) float64 {
	if price, exists := ModelRatios[modelName]; exists {
		return price.CompletionRatio
	}
	return a.DefaultPricingMethods.GetCompletionRatio(modelName)
}

// DefaultToolingConfig returns PaLM tooling defaults (tool pricing not published as of 2025-11-12).
func (a *Adaptor) DefaultToolingConfig() adaptor.ChannelToolConfig {
	return PalmToolingDefaults
}
