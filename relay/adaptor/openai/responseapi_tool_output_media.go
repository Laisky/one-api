package openai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/model"
)

// maxResponseFallbackToolOutputImages bounds how many images function and
// custom tool outputs may carry in one fallback request. It matches the
// per-request image ceiling Anthropic documents for the Messages API, so a
// request admitted here is never rejected upstream for its image count alone,
// and it bounds the image fetches a Claude conversion may perform.
const maxResponseFallbackToolOutputImages = 100

// responseToolOutputImagePlaceholder is the Chat tool message text used when
// an image-bearing tool output has no text of its own.
const responseToolOutputImagePlaceholder = "[image output attached in the next message]"

// responseToolOutputValue returns the tool output value the fallback converter
// lowers for a function_call_output item: output, or content when output is
// absent or empty. Parameters: item is the decoded input item. Returns: the
// raw JSON-decoded value, which may be nil.
func responseToolOutputValue(item map[string]any) any {
	switch output := item["output"].(type) {
	case nil:
		return item["content"]
	case string:
		if output == "" {
			return item["content"]
		}
	case []byte:
		if len(output) == 0 {
			return item["content"]
		}
	}
	return item["output"]
}

// validateResponseFallbackToolOutput admits one function or custom tool output
// value. Strings, objects and arrays without input_image parts keep their
// historical JSON stringification and are accepted unchanged. Arrays that do
// carry images are validated like user-message content: text parts must hold
// string text and every input_image part needs an image_url or file reference.
// Parameters: value is the output value from responseToolOutputValue.
// Returns: the number of images that will be forwarded as images, or a wrapped
// ErrResponseFallbackNotRepresentable when an image part is malformed.
func validateResponseFallbackToolOutput(value any) (int, error) {
	parts, ok := value.([]any)
	if !ok || !responseToolOutputHasImagePart(parts) {
		return 0, nil
	}
	images := 0
	representable := true
	for _, rawPart := range parts {
		part, ok := rawPart.(map[string]any)
		if !ok {
			// Non-object entries keep their in-place JSON text rendering.
			continue
		}
		kind, _ := part["type"].(string)
		switch kind {
		case "input_text", "text", "output_text":
			if text, present := part["text"]; present {
				if _, ok := text.(string); !ok {
					return 0, errors.Wrap(ErrResponseFallbackNotRepresentable, "invalid tool output text part")
				}
			}
		case "input_image":
			if url, _ := part["image_url"].(string); url != "" {
				if responseToolOutputImageURLRepresentable(url) {
					images++
				} else {
					representable = false
				}
				continue
			}
			if !responseFallbackFileRepresentable(part) {
				return 0, errors.Wrap(ErrResponseFallbackNotRepresentable, "unsupported tool output image part")
			}
			// File-backed images keep the legacy stringification of the output.
			representable = false
		}
	}
	if !representable {
		return 0, nil
	}
	return images, nil
}

// responseToolOutputHasImagePart reports whether parts holds an input_image object.
// Parameters: parts is a decoded tool output array. Returns: true when at least
// one entry is an object whose type is input_image.
func responseToolOutputHasImagePart(parts []any) bool {
	for _, rawPart := range parts {
		if part, ok := rawPart.(map[string]any); ok && part["type"] == "input_image" {
			return true
		}
	}
	return false
}

// responseToolOutputMediaParts lowers a tool output array that carries images
// into ordered Chat parts. Text parts become text, input_image parts become
// image_url parts, and any other entry keeps its JSON text in place, so the
// result holds the same information the legacy stringification did.
// Parameters: value is the output value from responseToolOutputValue.
// Returns: the parts and true, or nil and false when the output is not an
// array, carries no image, or carries an image that cannot be forwarded as an
// image (file references, raw file data, unsupported URL schemes); callers then
// keep the legacy JSON stringification.
func responseToolOutputMediaParts(value any) ([]model.MessageContent, bool) {
	rawParts, ok := value.([]any)
	if !ok || len(rawParts) == 0 {
		return nil, false
	}
	parts := make([]model.MessageContent, 0, len(rawParts))
	images := 0
	for _, rawPart := range rawParts {
		part, ok := rawPart.(map[string]any)
		if !ok {
			parts = appendResponseToolOutputJSONText(parts, rawPart)
			continue
		}
		kind, _ := part["type"].(string)
		switch kind {
		case "input_text", "text", "output_text":
			rawText, present := part["text"]
			if !present {
				continue
			}
			text, isString := rawText.(string)
			if !isString {
				parts = appendResponseToolOutputJSONText(parts, part)
				continue
			}
			if text != "" {
				parts = append(parts, model.MessageContent{Type: model.ContentTypeText, Text: &text})
			}
		case "input_image":
			imagePart, ok := responseToolOutputImagePart(part)
			if !ok {
				return nil, false
			}
			parts = append(parts, imagePart)
			images++
		default:
			parts = appendResponseToolOutputJSONText(parts, part)
		}
	}
	if images == 0 {
		return nil, false
	}
	return parts, true
}

// responseToolOutputImagePart converts one input_image tool output part.
// Parameters: part is the decoded input_image object. Returns: an image_url
// part and true when its image_url is a base64 image data URL or an http(s)
// URL; otherwise an empty part and false.
func responseToolOutputImagePart(part map[string]any) (model.MessageContent, bool) {
	url, _ := part["image_url"].(string)
	if !responseToolOutputImageURLRepresentable(url) {
		return model.MessageContent{}, false
	}
	return model.MessageContent{
		Type: model.ContentTypeImageURL,
		ImageURL: &model.ImageURL{
			Url:    url,
			Detail: normalizeResponseToolOutputImageDetail(part["detail"]),
		},
	}, true
}

// responseToolOutputImageURLRepresentable reports whether url can be forwarded
// as an image: a data:image/<subtype>;base64,<payload> URL in the shape the
// shared image decoder accepts, or an http(s) URL (scheme matched without
// regard to case). Parameters: url is the client-supplied image_url. Returns:
// true when the URL is usable. The check never copies the large payload.
func responseToolOutputImageURLRepresentable(url string) bool {
	for _, scheme := range []string{"https://", "http://"} {
		if hasASCIIPrefixFold(url, scheme) {
			return len(url) > len(scheme)
		}
	}
	// The shared decoder (image.GetImageFromUrl) matches the data URL header
	// case-sensitively, so only that exact spelling is forwarded as an image.
	if !strings.HasPrefix(url, "data:image/") {
		return false
	}
	comma := strings.IndexByte(url, ',')
	if comma < 0 || comma == len(url)-1 {
		return false
	}
	header := url[len("data:image/"):comma]
	subtype, ok := strings.CutSuffix(header, ";base64")
	return ok && subtype != "" && !strings.ContainsAny(subtype, "; ")
}

// hasASCIIPrefixFold reports whether s starts with prefix, ignoring ASCII case.
// Parameters: s is the inspected text and prefix an ASCII prefix. Returns:
// true on a case-insensitive prefix match.
func hasASCIIPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// normalizeResponseToolOutputImageDetail maps a Responses image detail onto the
// Chat Completions vocabulary. Parameters: raw is the decoded detail value.
// Returns: low, high or auto; "original" maps to high, the closest Chat level,
// and absent or unknown values return "" so upstream defaults apply.
func normalizeResponseToolOutputImageDetail(raw any) string {
	detail, _ := raw.(string)
	switch strings.ToLower(strings.TrimSpace(detail)) {
	case "low":
		return "low"
	case "high", "original":
		return "high"
	case "auto":
		return "auto"
	default:
		return ""
	}
}

// appendResponseToolOutputJSONText appends value's JSON encoding as a text part.
// Parameters: parts is the accumulator and value a decoded JSON value.
// Returns: the extended parts.
func appendResponseToolOutputJSONText(parts []model.MessageContent, value any) []model.MessageContent {
	text := ""
	if encoded, err := json.Marshal(value); err == nil {
		text = string(encoded)
	} else {
		// Values come from JSON decoding, so this is unreachable in practice;
		// still keep their printed form rather than dropping them.
		text = fmt.Sprintf("%v", value)
	}
	return append(parts, model.MessageContent{Type: model.ContentTypeText, Text: &text})
}

// responseToolOutputText joins the text parts of a lowered tool output for the
// text-only Chat tool message. Parameters: parts are the ordered output parts.
// Returns: the newline-joined text, or a placeholder when there is none.
func responseToolOutputText(parts []model.MessageContent) string {
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Type == model.ContentTypeText && part.Text != nil && *part.Text != "" {
			texts = append(texts, *part.Text)
		}
	}
	if len(texts) == 0 {
		return responseToolOutputImagePlaceholder
	}
	return strings.Join(texts, "\n")
}

// attachToolResultMediaCarriers inserts one user message carrying the images of
// each run of consecutive tool messages directly after that run. Chat
// Completions tool messages only carry text, and providers such as DeepSeek
// require every tool message of a turn to follow its assistant tool_calls
// message directly, so the images can neither stay in the tool messages nor
// sit between them. Parameters: messages is the lowered Chat history. Returns:
// messages itself when no tool message carries media, otherwise a new slice.
func attachToolResultMediaCarriers(messages []model.Message) []model.Message {
	carriers := 0
	for i := 0; i < len(messages); i++ {
		if messages[i].Role != "tool" || len(messages[i].ToolResultParts) == 0 {
			continue
		}
		carriers++
		for i+1 < len(messages) && messages[i+1].Role == "tool" {
			i++
		}
	}
	if carriers == 0 {
		return messages
	}

	out := make([]model.Message, 0, len(messages)+carriers)
	for i := 0; i < len(messages); {
		if messages[i].Role != "tool" {
			out = append(out, messages[i])
			i++
			continue
		}
		var carried []model.MessageContent
		for ; i < len(messages) && messages[i].Role == "tool"; i++ {
			out = append(out, messages[i])
			carried = appendToolResultCarrierParts(carried, messages[i])
		}
		if len(carried) > 0 {
			out = append(out, model.Message{Role: "user", Content: carried, ToolResultMediaCarrier: true})
		}
	}
	return out
}

// appendToolResultCarrierParts appends a label and copies of the image parts of
// one tool message to a carrier message's content. Parameters: carried is the
// accumulator and message a tool message. Returns: the extended parts; the
// input is returned unchanged when the message carries no image.
func appendToolResultCarrierParts(carried []model.MessageContent, message model.Message) []model.MessageContent {
	labeled := false
	for _, part := range message.ToolResultParts {
		if part.Type != model.ContentTypeImageURL || part.ImageURL == nil {
			continue
		}
		if !labeled {
			label := "[Image output from tool call " + message.ToolCallId + "]"
			if message.ToolCallId == "" {
				label = "[Image output from a tool call]"
			}
			carried = append(carried, model.MessageContent{Type: model.ContentTypeText, Text: &label})
			labeled = true
		}
		imageURL := *part.ImageURL
		carried = append(carried, model.MessageContent{Type: model.ContentTypeImageURL, ImageURL: &imageURL})
	}
	return carried
}
