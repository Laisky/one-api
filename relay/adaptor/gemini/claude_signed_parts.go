package gemini

import (
	"net/http"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/model"
)

// appendClaudeNativeBlocks projects text and function-call Parts without combining signed boundaries.
// Parameters: blocks is the ordered output and part is one native provider Part. Returns the appended blocks.
func appendClaudeNativeBlocks(blocks []model.ClaudeContent, part Part) []model.ClaudeContent {
	metadata := geminiPartExtraContent(part, 0, false)
	if part.FunctionCall != nil {
		return append(blocks, model.ClaudeContent{Type: "tool_use", ID: claudeGeminiToolID(), Name: part.FunctionCall.FunctionName, Input: openai_compatible.ClaudeToolInputFromArguments(part.FunctionCall.Arguments), ExtraContent: metadata})
	}
	if part.Text != "" || (metadata != nil && part.InlineData == nil && part.FileData == nil) {
		blocks = append(blocks, model.ClaudeContent{Type: "text", Text: part.Text, ExtraContent: metadata})
	}
	return blocks
}

// emitClaudeNativeBlocks writes exact text/function block boundaries to a buffered Claude emitter.
// Parameters: emitter owns the stream and blocks are ordered projections. It returns no values; emitter.Err reports failures.
func emitClaudeNativeBlocks(emitter *openai_compatible.ClaudeStreamEmitter, blocks []model.ClaudeContent) {
	for _, block := range blocks {
		index := emitter.StartBlock(block)
		if block.Type == "tool_use" {
			emitter.InputJSONDelta(index, string(block.Input))
		} else {
			emitter.TextDelta(index, block.Text)
		}
		emitter.StopBlock(index)
	}
}

// rejectClaudeGeminiFunctionFragments rejects the opt-in protocol this complete-call bridge cannot render.
// Parameters: c retains a received native usage receipt and response is the decoded provider payload.
// Returns a controlled gateway error for incremental arguments, or nil for complete-call responses.
func rejectClaudeGeminiFunctionFragments(c *gin.Context, response *ChatResponse) *model.ErrorWithStatusCode {
	if !unsupportedGeminiFunctionFragments(response) {
		return nil
	}
	if response.UsageMetadata != nil {
		c.Set(NativeUsageReceiptKey, geminiUsageMetadataToOpenAIUsage(response.UsageMetadata))
	}
	return openai.ErrorWrapper(errors.New("incremental Gemini function arguments are unsupported on this relay"), "unsupported_gemini_function_arguments", http.StatusBadGateway)
}
