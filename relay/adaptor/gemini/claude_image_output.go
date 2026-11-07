package gemini

import (
	"fmt"
)

// claudeGeminiImageText represents one image without exposing non-image data.
func claudeGeminiImageText(part Part) string {
	if part.FunctionCall != nil {
		return ""
	}
	if part.InlineData != nil && part.InlineData.Data != "" && part.InlineData.MimeType != "" && isGeminiImageMimeType(part.InlineData.MimeType) {
		return fmt.Sprintf("data:%s;base64,%s", part.InlineData.MimeType, part.InlineData.Data)
	}
	if part.FileData != nil && part.FileData.FileURI != "" && isGeminiImageMimeType(part.FileData.MimeType) {
		return part.FileData.FileURI
	}
	return ""
}

// claudeImageTextBlock is one rendered Claude text block; image marks a block
// that carries a generated image so its delivery boundary can be receipted.
type claudeImageTextBlock struct {
	text  string
	image bool
}

// mapGeminiFinishReason maps a Gemini finishReason to a Claude stop_reason:
// MAX_TOKENS is max_tokens, safety-style blocks are refusal, and STOP or any
// other reason is end_turn (callers promote function-call turns to tool_use).
func mapGeminiFinishReason(reason string) string {
	switch reason {
	case "MAX_TOKENS":
		return "max_tokens"
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII", "IMAGE_SAFETY":
		return "refusal"
	default:
		return "end_turn"
	}
}
