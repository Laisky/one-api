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
	modelName := request.Model
	if c != nil {
		if raw, ok := c.Get(ctxkey.Meta); ok {
			if m, ok := raw.(*meta.Meta); ok && m.ActualModelName != "" {
				modelName = m.ActualModelName
			}
		}
	}
	// Other Gemini models retain their existing transport contracts.
	if !geminiOpenaiCompatible.IsGeminiNanoBanana21Model(modelName) {
		return nil
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
	}
	if len(request.Functions) > 0 {
		return errors.New("function calling is not supported by Gemini Nano Banana 2.1")
	}
	return nil
}
