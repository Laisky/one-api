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
		for _, part := range message.ParseContent() {
			switch part.Type {
			case model.ContentTypeText:
			case model.ContentTypeImageURL:
				images++
			default:
				return errors.New("input content type is not supported by the native Gemini Nano Banana 2.1 adaptor")
			}
		}
		if parts, ok := message.Content.([]any); ok {
			for _, raw := range parts {
				if part, ok := raw.(map[string]any); ok {
					typ, _ := part["type"].(string)
					if typ != "" && !strings.EqualFold(typ, model.ContentTypeText) && !strings.EqualFold(typ, model.ContentTypeImageURL) {
						return errors.New("input content type is not supported by the native Gemini Nano Banana 2.1 adaptor")
					}
				}
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
	if effective := nativeEffectiveModel(c, request.Model); geminiOpenaiCompatible.IsGeminiNanoBanana21Model(effective) {
		request.Model = effective
	}
	return ConvertRequest(request)
}
