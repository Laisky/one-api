package gemini

import (
	"slices"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/model"
)

// NativePartsHeader explicitly opts Chat clients into the gateway's structured native-Part extension.
// Without its v1 value, text Chat content uses the standard scalar wire contract.
const NativePartsHeader = "X-OneAPI-Gemini-Parts"

// preserveNativeChatParts distinguishes internal conversion from the public Chat renderer.
// The private capture-writer marker also covers MCP rounds; a caller header never changes
// destination capabilities or signature validation, only its selected response representation.
func preserveNativeChatParts(c *gin.Context) bool {
	if c == nil {
		return false
	}
	if openai_compatible.StreamRewriterFromContext(c) != nil {
		return true
	}
	if writer, ok := c.Writer.(interface{ PreserveNativeChatParts() bool }); ok && writer.PreserveNativeChatParts() {
		return true
	}
	if c.Request == nil {
		return false
	}
	negotiated := strings.TrimSpace(c.Request.Header.Get(NativePartsHeader)) == "v1"
	if c.Writer != nil {
		headers := c.Writer.Header()
		if negotiated {
			headers.Set(NativePartsHeader, "v1")
		}
		found := false
		for _, value := range headers.Values("Vary") {
			for _, token := range strings.Split(value, ",") {
				found = found || strings.EqualFold(strings.TrimSpace(token), NativePartsHeader)
			}
		}
		if !found {
			headers.Add("Vary", NativePartsHeader)
		}
	}
	return negotiated
}

// ChatResponseForWire projects a native Gemini conversion for the final Chat renderer.
// Rich internal Parts and negotiated output are unchanged. Projection never mutates the
// source response, so internal retries, usage and protocol bridges retain their full fidelity.
func ChatResponseForWire(c *gin.Context, response *openai.TextResponse) *openai.TextResponse {
	if response == nil || preserveNativeChatParts(c) {
		return response
	}
	wire := *response
	wire.Choices = slices.Clone(response.Choices)
	for i := range wire.Choices {
		wire.Choices[i].Message = scalarChatMessage(wire.Choices[i].Message)
	}
	return &wire
}

func chatStreamResponseForWire(c *gin.Context, response *openai.ChatCompletionsStreamResponse) *openai.ChatCompletionsStreamResponse {
	if response == nil || preserveNativeChatParts(c) {
		return response
	}
	wire := *response
	wire.Choices = slices.Clone(response.Choices)
	for i := range wire.Choices {
		wire.Choices[i].Delta = scalarChatMessage(wire.Choices[i].Delta)
	}
	return &wire
}

// scalarChatMessage consumes text Part boundaries only at the public text Chat boundary.
// Existing multimodal image extensions retain their prior representation. Multiple native
// text signatures cannot be combined; lossless mixed-Part replay requires v1 negotiation
// or the Responses/Claude paths. Opaque function-call signatures remain on their own calls.
func scalarChatMessage(message model.Message) model.Message {
	parts, structured := chatTextParts(message)
	if structured {
		for _, part := range parts {
			if part.Type != model.ContentTypeText {
				return message
			}
		}
		var text strings.Builder
		for _, part := range parts {
			if part.Text != nil {
				text.WriteString(*part.Text)
			}
		}
		message.Content = text.String()
		if len(parts) == 1 && message.ExtraContent == nil {
			message.ExtraContent = parts[0].ExtraContent
		}
	}
	message.ExtraContent = withoutGeminiPartPosition(message.ExtraContent)
	var calls []model.Tool
	for i, call := range message.ToolCalls {
		scoped := withoutGeminiPartPosition(call.ExtraContent)
		if scoped == call.ExtraContent {
			continue
		}
		if calls == nil {
			calls = slices.Clone(message.ToolCalls)
		}
		calls[i].ExtraContent = scoped
	}
	if calls != nil {
		message.ToolCalls = calls
	}
	return message
}

// withoutGeminiPartPosition avoids publishing dangling positions after scalar projection,
// while retaining Google signatures and the presence bit on a sole metadata-only text Part.
func withoutGeminiPartPosition(extra *model.ToolExtraContent) *model.ToolExtraContent {
	if extra == nil || extra.OneAPI == nil || extra.OneAPI.GeminiPartIndex == nil {
		return extra
	}
	out, gateway := *extra, *extra.OneAPI
	gateway.GeminiPartIndex = nil
	out.OneAPI = &gateway
	if gateway.GeminiTextPresent == nil {
		out.OneAPI = nil
	}
	if out.OneAPI == nil && out.Google == nil {
		return nil
	}
	return &out
}

// chatTextParts also covers internally captured JSON (for example MCP rounds), whose
// content arrays decode as []any. Existing checked Part decoding supplies the same metadata
// interpretation without routing the entire typed request or response through JSON again.
func chatTextParts(message model.Message) ([]model.MessageContent, bool) {
	switch content := message.Content.(type) {
	case []model.MessageContent:
		return content, true
	case []any:
		for _, raw := range content {
			object, ok := raw.(map[string]any)
			if !ok || object["type"] != model.ContentTypeText {
				return nil, false
			}
		}
		parts, err := message.ParseContentChecked()
		return parts, err == nil
	default:
		return nil, false
	}
}
