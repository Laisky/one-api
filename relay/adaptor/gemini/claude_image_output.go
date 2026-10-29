package gemini

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/Laisky/errors/v2"
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

type claudeImageTextBlock struct {
	text  string
	image bool
}

func writeClaudeSSEEvent(buf *bytes.Buffer, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "marshal Claude SSE payload")
	}
	if event != "" {
		buf.WriteString("event: ")
		buf.WriteString(event)
		buf.WriteString("\n")
	}
	buf.WriteString("data: ")
	buf.Write(data)
	buf.WriteString("\n\n")
	return nil
}

func mapGeminiFinishReason(reason string) string {
	switch reason {
	case "MAX_TOKENS":
		return "max_tokens"
	case "SAFETY", "RECITATION":
		return "stop_sequence"
	case "STOP", "":
		return "end_turn"
	default:
		return "end_turn"
	}
}
