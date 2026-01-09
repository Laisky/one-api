package replicate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

type Adaptor struct {
	meta *meta.Meta
}

// ConvertImageRequest implements adaptor.Adaptor.
func (a *Adaptor) ConvertImageRequest(_ *gin.Context, request *model.ImageRequest) (any, error) {
	return nil, errors.New("should call replicate.ConvertImageRequest instead")
}

func ConvertImageRequest(c *gin.Context, request *model.ImageRequest) (any, error) {
	meta := meta.GetByContext(c)

	if request.ResponseFormat == nil || *request.ResponseFormat != "b64_json" {
		return nil, errors.New("only support b64_json response format")
	}
	if request.N != 1 && request.N != 0 {
		return nil, errors.New("only support N=1")
	}

	switch meta.Mode {
	case relaymode.ImagesGenerations:
		return convertImageCreateRequest(request)
	case relaymode.ImagesEdits:
		return convertImageRemixRequest(c)
	default:
		return nil, errors.New("not implemented")
	}
}

func convertImageCreateRequest(request *model.ImageRequest) (any, error) {
	convertedReq := DrawImageRequest{
		Input: ImageInput{
			Steps:           25,
			Prompt:          request.Prompt,
			Guidance:        3,
			Seed:            int(time.Now().UnixNano()),
			SafetyTolerance: 5,
			NImages:         1, // replicate will always return 1 image
			Width:           1440,
			Height:          1440,
			AspectRatio:     "1:1",
		},
	}

	if strings.Contains(request.Model, "flux-kontext") {
		convertedReq.Input.InputImage = request.ImagePrompt
	} else {
		convertedReq.Input.ImagePrompt = request.ImagePrompt
	}

	return convertedReq, nil
}

func convertImageRemixRequest(c *gin.Context) (any, error) {
	// recover request body
	requestBody, err := common.GetRequestBody(c)
	if err != nil {
		return nil, errors.Wrap(err, "get request body")
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))

	rawReq := new(model.OpenaiImageEditRequest)
	if err := c.ShouldBind(rawReq); err != nil {
		return nil, errors.Wrap(err, "parse image edit form")
	}

	return Convert2FluxRemixRequest(rawReq)
}

// ConvertRequest converts the request to the format that the target API expects.
func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	gmw.GetLogger(c).Debug("replicate.ConvertRequest",
		zap.String("model", request.Model),
		zap.Bool("stream", request.Stream),
	)

	// Build the prompt from OpenAI messages
	var promptBuilder strings.Builder
	for _, message := range request.Messages {
		switch msgCnt := message.Content.(type) {
		case string:
			promptBuilder.WriteString(message.Role)
			promptBuilder.WriteString(": ")
			promptBuilder.WriteString(msgCnt)
			promptBuilder.WriteString("\n")
		default:
		}
	}

	// Image models are not supported via chat API
	// if pricing, ok := ModelRatios[request.Model]; ok && pricing.Image != nil {
	// 	return nil, errors.Errorf("model %s is an image model, please use image API", request.Model)
	// }

	replicateRequest := ReplicateChatRequest{
		Input: ChatInput{
			Prompt:           promptBuilder.String(),
			MaxTokens:        request.MaxTokens,
			Temperature:      1.0,
			TopP:             1.0,
			PresencePenalty:  0.0,
			FrequencyPenalty: 0.0,
		},
	}

	// Map optional fields
	if request.Temperature != nil {
		replicateRequest.Input.Temperature = *request.Temperature
	}
	if request.TopP != nil {
		replicateRequest.Input.TopP = *request.TopP
	}
	if request.PresencePenalty != nil {
		replicateRequest.Input.PresencePenalty = *request.PresencePenalty
	}
	if request.FrequencyPenalty != nil {
		replicateRequest.Input.FrequencyPenalty = *request.FrequencyPenalty
	}
	if request.MaxTokens > 0 {
		replicateRequest.Input.MaxTokens = request.MaxTokens
	} else if request.MaxTokens == 0 {
		replicateRequest.Input.MaxTokens = config.DefaultMaxToken
	}

	return replicateRequest, nil
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

	// Convert system prompt
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
			// For structured system content, extract text parts
			var systemParts []string
			for _, block := range system {
				if blockMap, ok := block.(map[string]any); ok {
					if text, exists := blockMap["text"]; exists {
						if textStr, ok := text.(string); ok {
							systemParts = append(systemParts, textStr)
						}
					}
				}
			}
			if len(systemParts) > 0 {
				systemText := strings.Join(systemParts, "\n")
				openaiRequest.Messages = append(openaiRequest.Messages, model.Message{
					Role:    "system",
					Content: systemText,
				})
			}
		}
	}

	// Convert messages
	for _, msg := range request.Messages {
		openaiMessage := model.Message{
			Role: msg.Role,
		}

		// Convert content based on type
		switch content := msg.Content.(type) {
		case string:
			// Simple string content
			openaiMessage.Content = content
		case []any:
			// Structured content blocks - convert to OpenAI format
			var contentParts []model.MessageContent
			for _, block := range content {
				if blockMap, ok := block.(map[string]any); ok {
					if blockType, exists := blockMap["type"]; exists {
						switch blockType {
						case "text":
							if text, exists := blockMap["text"]; exists {
								if textStr, ok := text.(string); ok {
									contentParts = append(contentParts, model.MessageContent{
										Type: "text",
										Text: &textStr,
									})
								}
							}
						case "image":
							if source, exists := blockMap["source"]; exists {
								if sourceMap, ok := source.(map[string]any); ok {
									imageURL := model.ImageURL{}
									if mediaType, exists := sourceMap["media_type"]; exists {
										if data, exists := sourceMap["data"]; exists {
											if dataStr, ok := data.(string); ok {
												// Convert to data URL format
												imageURL.Url = fmt.Sprintf("data:%s;base64,%s", mediaType, dataStr)
											}
										}
									}
									contentParts = append(contentParts, model.MessageContent{
										Type:     "image_url",
										ImageURL: &imageURL,
									})
								}
							}
						}
					}
				}
			}
			if len(contentParts) > 0 {
				openaiMessage.Content = contentParts
			}
		default:
			// Fallback: convert to string
			if contentBytes, err := json.Marshal(content); err == nil {
				openaiMessage.Content = string(contentBytes)
			}
		}

		openaiRequest.Messages = append(openaiRequest.Messages, openaiMessage)
	}

	// Convert tools
	for _, tool := range request.Tools {
		openaiTool := model.Tool{
			Type: "function",
			Function: &model.Function{
				Name:        tool.Name,
				Description: tool.Description,
			},
		}

		// Convert input schema
		if tool.InputSchema != nil {
			if schemaMap, ok := tool.InputSchema.(map[string]any); ok {
				openaiTool.Function.Parameters = schemaMap
			}
		}

		openaiRequest.Tools = append(openaiRequest.Tools, openaiTool)
	}

	// Convert tool choice
	if request.ToolChoice != nil {
		openaiRequest.ToolChoice = request.ToolChoice
	}

	// Mark this as a Claude Messages conversion for response handling
	c.Set(ctxkey.ClaudeMessagesConversion, true)
	c.Set(ctxkey.OriginalClaudeRequest, request)

	// Now convert using Replicate's existing logic
	return a.ConvertRequest(c, relaymode.ChatCompletions, openaiRequest)
}

func (a *Adaptor) Init(meta *meta.Meta) {
	a.meta = meta
}

func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	if !slices.Contains(ModelList, meta.OriginModelName) {
		return "", errors.Errorf("model %s not supported", meta.OriginModelName)
	}

	return fmt.Sprintf("https://api.replicate.com/v1/models/%s/predictions", meta.OriginModelName), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, meta)
	req.Header.Set("Authorization", "Bearer "+meta.APIKey)
	return nil
}

func (a *Adaptor) DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error) {
	gmw.GetLogger(c).Info("send request to replicate",
		zap.String("model", meta.OriginModelName),
		zap.Int("mode", meta.Mode),
	)
	return adaptor.DoRequestHelper(a, c, meta, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode) {
	switch meta.Mode {
	case relaymode.ImagesGenerations,
		relaymode.ImagesEdits:
		err, usage = ImageHandler(c, resp)
	case relaymode.ChatCompletions:
		err, usage = ChatHandler(c, resp)
	default:
		err = openai.ErrorWrapper(errors.New("not implemented"), "not_implemented", http.StatusInternalServerError)
	}

	return
}

func (a *Adaptor) GetModelList() []string {
	return adaptor.GetModelListFromPricing(ModelRatios)
}

func (a *Adaptor) GetChannelName() string {
	return "replicate"
}

// Pricing methods - Replicate adapter manages its own model pricing
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	// Use the constants.go ModelRatios which already use ratio.MilliTokensUsd correctly
	return ModelRatios
}

func (a *Adaptor) GetModelRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.Ratio
	}
	// Default Replicate pricing (image generation) - no per-token price by default
	return 0
}

func (a *Adaptor) GetCompletionRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.CompletionRatio
	}
	// Default completion ratio for Replicate
	return 1.0
}

// DefaultToolingConfig returns Replicate tooling defaults (no separate tooling fees documented).
func (a *Adaptor) DefaultToolingConfig() adaptor.ChannelToolConfig {
	return ReplicateToolingDefaults
}
