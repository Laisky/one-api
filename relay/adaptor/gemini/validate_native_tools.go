package gemini

import (
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/common/builtinname"
	"github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// ValidateNativeToolSupport fails closed when Gemini's native REST adaptor
// cannot represent a requested tool. Parameters: c may provide the mapped
// upstream model, and request contains the client tool requirements. Returns
// a descriptive error before provider dispatch or quota reservation.
func ValidateNativeToolSupport(c *gin.Context, request *model.GeneralOpenAIRequest) error {
	if request == nil {
		return errors.New("gemini request is nil")
	}
	modelName := nativeEffectiveModel(c, request.Model)
	// Other Gemini models retain their existing transport contracts.
	if !geminiOpenaiCompatible.IsGeminiNanoBanana21Model(modelName) {
		return nil
	}
	if request.N != nil && *request.N > 1 {
		return errors.New("native Gemini Nano Banana 2.1 supports at most one response candidate")
	}
	if request.ReasoningEffort != nil {
		switch strings.ToLower(*request.ReasoningEffort) {
		case "", "default", "minimal", "medium", "high":
		default:
			return errors.New("reasoning effort is not supported by Gemini Nano Banana 2.1")
		}
	}
	if request.ToolChoice != nil {
		choice, ok := request.ToolChoice.(string)
		if !ok || (choice != "" && !strings.EqualFold(choice, "auto") && !strings.EqualFold(choice, "none")) {
			return errors.New("tool choice is not supported by Gemini Nano Banana 2.1")
		}
	}
	images := 0
	for _, message := range request.Messages {
		if len(message.ToolCalls) > 0 || message.ToolCallId != "" || strings.EqualFold(message.Role, "tool") {
			return errors.New("function calling history is not supported by Gemini Nano Banana 2.1")
		}
		// Check the raw type tags before ParseContent, because the shared
		// decoder intentionally omits unknown part types. Validation must not
		// silently accept discarded input or parse malformed audio first.
		switch message.Content.(type) {
		case nil, string, []any, []model.MessageContent:
		default:
			return errors.New("input content type is not supported by the native Gemini Nano Banana 2.1 adaptor")
		}
		if parts, ok := message.Content.([]any); ok {
			for _, raw := range parts {
				part, ok := raw.(map[string]any)
				if !ok {
					return errors.New("input content type is not supported by the native Gemini Nano Banana 2.1 adaptor")
				}
				typ, _ := part["type"].(string)
				switch {
				case strings.EqualFold(typ, model.ContentTypeText):
					if _, ok := part["text"].(string); !ok {
						return errors.New("input text content is invalid for Gemini Nano Banana 2.1")
					}
				case typ == model.ContentTypeImageURL:
					image, ok := part["image_url"].(map[string]any)
					if !ok {
						return errors.New("input image content is invalid for Gemini Nano Banana 2.1")
					}
					url, ok := image["url"].(string)
					if !ok || strings.TrimSpace(url) == "" {
						return errors.New("input image content is invalid for Gemini Nano Banana 2.1")
					}
				default:
					return errors.New("input content type is not supported by the native Gemini Nano Banana 2.1 adaptor")
				}
			}
		}
		for _, part := range message.ParseContent() {
			switch part.Type {
			case model.ContentTypeText:
			case model.ContentTypeImageURL:
				if part.ImageURL == nil || strings.TrimSpace(part.ImageURL.Url) == "" {
					return errors.New("input image content is invalid for Gemini Nano Banana 2.1")
				}
				images++
			default:
				return errors.New("input content type is not supported by the native Gemini Nano Banana 2.1 adaptor")
			}
		}
	}
	if images > 14 {
		return errors.New("Gemini Nano Banana 2.1 supports at most 14 reference images per request")
	}
	if request.WebSearchOptions != nil {
		return errors.New("web_search is not supported by the native Gemini Nano Banana 2.1 adaptor")
	}
	for _, tool := range request.Tools {
		if builtinname.Normalize(tool.Type) == "web_search" ||
			strings.EqualFold(strings.TrimSpace(tool.Type), "web_search") ||
			strings.EqualFold(strings.TrimSpace(tool.Type), "web_search_preview") {
			return errors.New("web_search is not supported by the native Gemini Nano Banana 2.1 adaptor")
		}
		// This upstream model does not offer function calling. Do not emit a
		// function declaration that would be rejected or silently lost.
		if tool.Function != nil || strings.EqualFold(tool.Type, "function") {
			return errors.New("function calling is not supported by Gemini Nano Banana 2.1")
		}
		return errors.New("tool type is not supported by the native Gemini Nano Banana 2.1 adaptor")
	}
	if request.ResponseFormat != nil && (request.ResponseFormat.JsonSchema != nil || (request.ResponseFormat.Type != "" && !strings.EqualFold(request.ResponseFormat.Type, "text"))) {
		return errors.New("structured output is not supported by Gemini Nano Banana 2.1")
	}
	if len(request.Functions) > 0 {
		return errors.New("function calling is not supported by Gemini Nano Banana 2.1")
	}
	if request.FunctionCall != nil {
		choice, ok := request.FunctionCall.(string)
		if !ok || !strings.EqualFold(choice, "none") {
			return errors.New("function calling is not supported by Gemini Nano Banana 2.1")
		}
	}
	return nil
}

// configureNanoBananaThinking maps a supported explicit reasoning level to native configuration.
func configureNanoBananaThinking(out *ChatRequest, request model.GeneralOpenAIRequest) {
	if geminiOpenaiCompatible.IsGeminiNanoBanana21Model(request.Model) && request.ReasoningEffort != nil {
		level := strings.ToLower(*request.ReasoningEffort)
		if level != "" && level != "default" {
			out.GenerationConfig.ThinkingConfig = &ThinkingConfig{ThinkingLevel: level}
		}
	}
}

// nativeEffectiveModel resolves the mapped upstream model without modifying the request.
func nativeEffectiveModel(c *gin.Context, modelName string) string {
	if c != nil {
		if raw, ok := c.Get(ctxkey.Meta); ok {
			if m, ok := raw.(*meta.Meta); ok && m.ActualModelName != "" {
				return m.ActualModelName
			}
		}
	}
	return modelName
}

// ConvertNativeRequest converts a request using the effective Nano Banana model when mapped.
func ConvertNativeRequest(c *gin.Context, request model.GeneralOpenAIRequest) *ChatRequest {
	isNanoBanana := false
	if effective := nativeEffectiveModel(c, request.Model); geminiOpenaiCompatible.IsGeminiNanoBanana21Model(effective) {
		request.Model = effective
		isNanoBanana = true
	}
	converted := ConvertRequest(request)
	if isNanoBanana {
		// "tool_choice": "none" is a valid no-tools client constraint,
		// but upstream Nano Banana does not support a function-calling
		// toolConfig even when its mode is NONE.
		converted.ToolConfig = nil
	}
	return converted
}

// ConvertNativeRequestChecked converts native requests and validates Nano Banana image inputs.
// Parameters: c supplies the mapped model and request is the validated source request.
// Returns: the converted payload or a controlled error without exposing the image URL.
func ConvertNativeRequestChecked(c *gin.Context, request model.GeneralOpenAIRequest) (*ChatRequest, error) {
	effective := nativeEffectiveModel(c, request.Model)
	if !geminiOpenaiCompatible.IsGeminiNanoBanana21Model(effective) {
		return ConvertNativeRequest(c, request), nil
	}
	request.Model = effective
	converted, err := convertRequest(request, true)
	if err != nil {
		return nil, errors.Wrap(err, "convert native Gemini request")
	}
	converted.ToolConfig = nil
	return converted, nil
}
