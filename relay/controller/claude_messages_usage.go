package controller

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// extractConvertedClaudeSSEUsage parses converted streaming response bodies for billing usage.
// It accepts Claude-style SSE events and OpenAI-compatible chat stream chunks, uses explicit
// usage metadata when present, and falls back to token counting accumulated output text.
// The promptTokens parameter supplies a fallback input token count, modelName selects the
// tokenizer, and the return value contains prompt, completion, and total token usage.
func extractConvertedClaudeSSEUsage(body []byte, promptTokens int, modelName string) *relaymodel.Usage {
	accumulated := ""
	extractedPromptTokens := 0
	extractedCompletionTokens := 0
	extractedTotalTokens := 0

	for line := range bytes.SplitSeq(body, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}

		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}

		text, usage := parseConvertedClaudeSSEPayload(payload)
		accumulated += text
		if usage != nil {
			if usage.InputTokens > 0 {
				extractedPromptTokens = usage.InputTokens
			}
			if usage.OutputTokens > 0 {
				extractedCompletionTokens = usage.OutputTokens
			}
			if usage.TotalTokens > 0 {
				extractedTotalTokens = usage.TotalTokens
			}
		}
	}

	if extractedPromptTokens > 0 {
		promptTokens = extractedPromptTokens
	}
	completionTokens := extractedCompletionTokens
	if completionTokens == 0 {
		completionTokens = openai.CountTokenText(accumulated, modelName)
	}
	totalTokens := extractedTotalTokens
	if totalTokens == 0 {
		totalTokens = promptTokens + completionTokens
	}

	return &relaymodel.Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}
}

// parseConvertedClaudeSSEPayload extracts output text and optional usage from one SSE data payload.
// It recognizes Claude content_block_delta and message_delta events as well as OpenAI chat stream
// chunks; the payload parameter is a JSON event body, and the return values are accumulated output
// text plus usage metadata when present.
func parseConvertedClaudeSSEPayload(payload []byte) (string, *convertedClaudeSSEUsage) {
	var event struct {
		Type  string `json:"type"`
		Delta struct {
			Type        string `json:"type"`
			Text        string `json:"text"`
			PartialJSON string `json:"partial_json"`
		} `json:"delta"`
		Usage *convertedClaudeSSEUsage `json:"usage"`
	}
	if err := json.Unmarshal(payload, &event); err == nil {
		text := ""
		if event.Type == "content_block_delta" {
			switch event.Delta.Type {
			case "text_delta":
				text = event.Delta.Text
			case "input_json_delta":
				text = event.Delta.PartialJSON
			}
		}
		if text != "" || event.Usage != nil {
			return text, event.Usage
		}
	}

	return parseOpenAIChatStreamPayload(payload), nil
}

// parseOpenAIChatStreamPayload extracts output text from an OpenAI-compatible chat stream chunk.
// The payload parameter is a JSON chunk body, and the return value is the textual content found in
// choices[].delta.content, including text entries in array-form content.
func parseOpenAIChatStreamPayload(payload []byte) string {
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content any `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(payload, &chunk); err != nil {
		return ""
	}

	var accumulated strings.Builder
	for _, ch := range chunk.Choices {
		switch v := ch.Delta.Content.(type) {
		case string:
			accumulated.WriteString(v)
		case []any:
			for _, p := range v {
				if m, ok := p.(map[string]any); ok {
					if t, _ := m["type"].(string); t == "text" {
						if s, ok := m["text"].(string); ok {
							accumulated.WriteString(s)
						}
					}
				}
			}
		}
	}

	return accumulated.String()
}

// convertedClaudeSSEUsage stores token usage parsed from converted Claude SSE message_delta events.
// InputTokens and OutputTokens map to Claude usage fields, while TotalTokens captures optional
// converter metadata when it is available.
type convertedClaudeSSEUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}
