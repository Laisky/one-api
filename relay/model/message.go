package model

import (
	"encoding/json"
	"strings"

	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/logger"
)

// ReasoningFormat is the format of reasoning content,
// can be set by the reasoning_format parameter in the request url.
type ReasoningFormat string

const (
	ReasoningFormatUnspecified ReasoningFormat = ""
	// ReasoningFormatReasoningContent is the reasoning format used by deepseek official API
	ReasoningFormatReasoningContent ReasoningFormat = "reasoning_content"
	// ReasoningFormatReasoning is the reasoning format used by openrouter
	ReasoningFormatReasoning ReasoningFormat = "reasoning"

	// ReasoningFormatThinkTag is the reasoning format used by 3rd party deepseek-r1 providers.
	//
	// Deprecated: I believe <think> is a very poor format, especially in stream mode, it is difficult to extract and convert.
	// Considering that only a few deepseek-r1 third-party providers use this format, it has been decided to no longer support it.
	// ReasoningFormatThinkTag ReasoningFormat = "think-tag"

	// ReasoningFormatThinking is the reasoning format used by anthropic
	ReasoningFormatThinking ReasoningFormat = "thinking"
)

type Message struct {
	Role string `json:"role,omitempty"`
	// Content is a string or a list of objects
	Content    any              `json:"content,omitempty"`
	Name       *string          `json:"name,omitempty"`
	ToolCalls  []Tool           `json:"tool_calls,omitempty"`
	ToolCallId string           `json:"tool_call_id,omitempty"`
	Audio      *messageAudio    `json:"audio,omitempty"`
	Annotation []AnnotationItem `json:"annotation,omitempty"`

	// -------------------------------------
	// DeepSeek specific fields
	// https://api-docs.deepseek.com/api/create-chat-completion
	// -------------------------------------
	// Prefix forces the model to begin its answer with the supplied prefix in the assistant message.
	// To enable this feature, set base_url to "https://api.deepseek.com/beta".
	Prefix *bool `json:"prefix,omitempty"` // ReasoningContent is Used for the deepseek-reasoner model in the Chat
	// Prefix Completion feature as the input for the CoT in the last assistant message.
	// When using this feature, the prefix parameter must be set to true.
	ReasoningContent *string `json:"reasoning_content,omitempty"`

	// -------------------------------------
	// Openrouter
	// -------------------------------------
	Reasoning *string `json:"reasoning,omitempty"`
	Refusal   *bool   `json:"refusal,omitempty"`

	// -------------------------------------
	// Anthropic
	// -------------------------------------
	Thinking  *string `json:"thinking,omitempty"`
	Signature *string `json:"signature,omitempty"`
}

type AnnotationItem struct {
	Type        string      `json:"type" binding:"oneof=url_citation"`
	UrlCitation UrlCitation `json:"url_citation"`
}

// UrlCitation is a URL citation when using web search.
type UrlCitation struct {
	// Endpoint is the index of the last character of the URL citation in the message.
	EndIndex int `json:"end_index"`
	// StartIndex is the index of the first character of the URL citation in the message.
	StartIndex int `json:"start_index"`
	// Title is the title of the web resource.
	Title string `json:"title"`
	// Url is the URL of the web resource.
	Url string `json:"url"`
}

// SetReasoningContent sets the reasoning content based on the format
func (m *Message) SetReasoningContent(format string, reasoningContent string) {
	normalized := ReasoningFormat(strings.ToLower(strings.TrimSpace(format)))
	content := reasoningContent

	switch normalized {
	case ReasoningFormatReasoningContent:
		m.Reasoning = nil
		m.Thinking = nil
		m.ReasoningContent = &content
		// case ReasoningFormatThinkTag:
		// 	m.Content = fmt.Sprintf("<think>%s</think>%s", reasoningContent, m.Content)
	case ReasoningFormatThinking:
		m.Reasoning = nil
		m.ReasoningContent = nil
		m.Thinking = &content
	case ReasoningFormatReasoning,
		ReasoningFormatUnspecified:
		m.ReasoningContent = nil
		m.Thinking = nil
		m.Reasoning = &content
	default:
		logger.Logger.Warn("unknown reasoning format", zap.String("format", format))
	}
}

type messageAudio struct {
	Id         string `json:"id"`
	Data       string `json:"data,omitempty"`
	ExpiredAt  int    `json:"expired_at,omitempty"`
	Transcript string `json:"transcript,omitempty"`
}

func (m Message) IsStringContent() bool {
	_, ok := m.Content.(string)
	return ok
}

func (m Message) StringContent() string {
	content, ok := m.Content.(string)
	if ok {
		return content
	}
	contentList, ok := m.Content.([]any)
	if ok {
		var contentStr string
		for _, contentItem := range contentList {
			contentMap, ok := contentItem.(map[string]any)
			if !ok {
				continue
			}
			typeStr, _ := contentMap["type"].(string)
			switch strings.ToLower(typeStr) {
			case strings.ToLower(ContentTypeText):
				if subStr, ok := contentMap["text"].(string); ok {
					contentStr += subStr
				}
			case "output_json":
				if jsonText := extractJSONText(contentMap["json"]); jsonText != "" {
					contentStr += jsonText
				}
				if text, ok := contentMap["text"].(string); ok {
					contentStr += text
				}
			case "output_json_delta":
				if partial, ok := contentMap["partial_json"].(string); ok {
					contentStr += partial
				}
			}
		}
		return contentStr
	}

	return ""
}

func (m Message) ParseContent() []MessageContent {
	var contentList []MessageContent
	content, ok := m.Content.(string)
	if ok {
		contentList = append(contentList, MessageContent{
			Type: ContentTypeText,
			Text: &content,
		})
		return contentList
	}

	// Handle []MessageContent directly
	messageContentList, ok := m.Content.([]MessageContent)
	if ok {
		return messageContentList
	}

	anyList, ok := m.Content.([]any)
	if ok {
		var jsonAccumulator strings.Builder
		var outputJSONSeen bool
		for _, contentItem := range anyList {
			contentMap, ok := contentItem.(map[string]any)
			if !ok {
				continue
			}
			typeStr, _ := contentMap["type"].(string)
			switch strings.ToLower(typeStr) {
			case strings.ToLower(ContentTypeText):
				if subStr, ok := contentMap["text"].(string); ok {
					contentList = append(contentList, MessageContent{
						Type: ContentTypeText,
						Text: &subStr,
					})
				}
			case "output_json", "output_json_delta":
				if jsonText := extractJSONText(contentMap["json"]); jsonText != "" {
					jsonAccumulator.WriteString(jsonText)
					outputJSONSeen = true
				}
				if text, ok := contentMap["text"].(string); ok {
					jsonAccumulator.WriteString(text)
					outputJSONSeen = true
				}
				if partial, ok := contentMap["partial_json"].(string); ok {
					jsonAccumulator.WriteString(partial)
					outputJSONSeen = true
				}
			case ContentTypeImageURL:
				if subObj, ok := contentMap["image_url"].(map[string]any); ok {
					var urlStr string
					if u, ok := subObj["url"].(string); ok {
						urlStr = u
					}
					var detailStr string
					if d, ok := subObj["detail"].(string); ok {
						detailStr = d
					}
					contentList = append(contentList, MessageContent{
						Type: ContentTypeImageURL,
						ImageURL: &ImageURL{
							Url:    urlStr,
							Detail: detailStr,
						},
					})
				}
			case ContentTypeInputAudio:
				if subObj, ok := contentMap["input_audio"].(map[string]any); ok {
					contentList = append(contentList, MessageContent{
						Type: ContentTypeInputAudio,
						InputAudio: &InputAudio{
							Data:   subObj["data"].(string),
							Format: subObj["format"].(string),
						},
					})
				}
			default:
				logger.Logger.Warn("unknown content type", zap.Any("type", contentMap["type"]))
			}
		}
		if outputJSONSeen && jsonAccumulator.Len() > 0 {
			jsonText := jsonAccumulator.String()
			contentList = append(contentList, MessageContent{Type: ContentTypeText, Text: &jsonText})
		}

		return contentList
	}
	return nil
}

// extractJSONText normalizes JSON payloads that may appear as strings, bytes, or nested objects.
func extractJSONText(raw any) string {
	switch val := raw.(type) {
	case string:
		return val
	case json.RawMessage:
		return string(val)
	case []byte:
		return string(val)
	case map[string]any, []any:
		if encoded, err := json.Marshal(val); err == nil {
			return string(encoded)
		}
	}
	return ""
}

type ImageURL struct {
	Url    string `json:"url,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type MessageContent struct {
	// Type should be one of the following: text/input_audio
	Type       string      `json:"type,omitempty"`
	Text       *string     `json:"text,omitempty"`
	ImageURL   *ImageURL   `json:"image_url,omitempty"`
	InputAudio *InputAudio `json:"input_audio,omitempty"`
	// -------------------------------------
	// Anthropic
	// -------------------------------------
	Thinking  *string `json:"thinking,omitempty"`
	Signature *string `json:"signature,omitempty"`
}

type InputAudio struct {
	// Data is the base64 encoded audio data
	Data string `json:"data" binding:"required"`
	// Format is the audio format, should be one of the
	// following: mp3/mp4/mpeg/mpga/m4a/wav/webm/pcm16.
	// When stream=true, format should be pcm16
	Format string `json:"format"`
}
