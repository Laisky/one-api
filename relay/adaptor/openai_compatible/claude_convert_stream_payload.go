package openai_compatible

import (
	"encoding/json"
	"strconv"
	"strings"

	relaymodel "github.com/Laisky/one-api/relay/model"
)

// responseAPIStreamEvent is the decoded envelope of one Responses stream event.
type responseAPIStreamEvent struct {
	Type        string               `json:"type,omitempty"`
	Response    *responseAPIResponse `json:"response,omitempty"`
	OutputIndex *int                 `json:"output_index,omitempty"`
	Item        *responseAPIOutput   `json:"item,omitempty"`
	Part        *responseAPIContent  `json:"part,omitempty"`
	Delta       json.RawMessage      `json:"delta,omitempty"`
	Text        string               `json:"text,omitempty"`
	Arguments   string               `json:"arguments,omitempty"`
	Output      json.RawMessage      `json:"output,omitempty"`
	JSON        json.RawMessage      `json:"json,omitempty"`
	Usage       *responseAPIUsage    `json:"usage,omitempty"`
	Status      string               `json:"status,omitempty"`
	Id          string               `json:"id,omitempty"`
	// ItemId, ContentIndex, and SummaryIndex locate a delta inside its output
	// item so the Claude renderer can de-duplicate the later .done snapshots.
	ItemId       string `json:"item_id,omitempty"`
	ContentIndex *int   `json:"content_index,omitempty"`
	SummaryIndex *int   `json:"summary_index,omitempty"`
	// Code and Message carry the payload of a Responses "error" event.
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// extractStringFromRawMessage extracts delta protocol strings in caller-supplied order.
func extractStringFromRawMessage(raw json.RawMessage, keys ...string) string {
	if len(raw) == 0 {
		return ""
	}

	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return str
	}

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		for _, key := range keys {
			if key == "" {
				continue
			}
			if val, ok := obj[key]; ok {
				switch v := val.(type) {
				case string:
					return v
				case []byte:
					return string(v)
				default:
					if b, err := json.Marshal(v); err == nil {
						return string(b)
					}
				}
			}
		}
	}

	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ""
	}
	if unquoted, err := strconv.Unquote(trimmed); err == nil {
		trimmed = unquoted
	}
	return trimmed
}

// extractJSONFromStreamEvent supplies the same location-specific done-event contract used by billing.
func extractJSONFromStreamEvent(event *responseAPIStreamEvent) json.RawMessage {
	if event == nil {
		return nil
	}
	fields := relaymodel.ResponseOutputJSONDoneFields{JSON: event.JSON, Output: event.Output, Text: event.Text, Delta: event.Delta}
	if event.Part != nil {
		fields.PartJSON = event.Part.JSON
		fields.PartText = event.Part.Text
	}
	return relaymodel.ExtractResponseOutputJSONDone(fields)
}

// --- Minimal local response types to avoid import cycles ---

// responseAPIResponse is the minimal Responses object used by the Claude converters.
type responseAPIResponse struct {
	Id        string              `json:"id"`
	Object    string              `json:"object"`
	Model     string              `json:"model"`
	Output    []responseAPIOutput `json:"output"`
	Usage     *responseAPIUsage   `json:"usage,omitempty"`
	CreatedAt int64               `json:"created_at"`
	Status    string              `json:"status"`
	// IncompleteDetails explains an incomplete status (e.g. max_output_tokens).
	IncompleteDetails *responseAPIIncompleteDetails `json:"incomplete_details,omitempty"`
	// Error describes a failed response.
	Error *responseAPIError `json:"error,omitempty"`
}

// responseAPIIncompleteDetails is the Responses incomplete_details object.
type responseAPIIncompleteDetails struct {
	Reason string `json:"reason,omitempty"`
}

// responseAPIError is the Responses error object of a failed response.
type responseAPIError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// responseAPIUsage is the Responses usage receipt.
type responseAPIUsage struct {
	InputTokens        int                            `json:"input_tokens"`
	OutputTokens       int                            `json:"output_tokens"`
	TotalTokens        int                            `json:"total_tokens"`
	InputTokensDetails *responseAPIInputTokensDetails `json:"input_tokens_details,omitempty"`
	CacheWrite5mTokens int                            `json:"cache_write_5m_tokens,omitempty"`
	CacheWrite1hTokens int                            `json:"cache_write_1h_tokens,omitempty"`
}

// responseAPIInputTokensDetails carries Responses cached input tokens.
type responseAPIInputTokensDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
}

// responseAPIOutput is one Responses output item.
type responseAPIOutput struct {
	Id        string               `json:"id,omitempty"`
	Type      string               `json:"type"`
	Role      string               `json:"role,omitempty"`
	Content   []responseAPIContent `json:"content,omitempty"`
	Summary   []responseAPIContent `json:"summary,omitempty"`
	CallId    string               `json:"call_id,omitempty"`
	Name      string               `json:"name,omitempty"`
	Arguments string               `json:"arguments,omitempty"`
	Tools     []relaymodel.Tool    `json:"tools,omitempty"`
	Metadata  map[string]any       `json:"metadata,omitempty"`
}

// responseAPIContent is one part of a Responses output item.
type responseAPIContent struct {
	Type string          `json:"type"`
	Text string          `json:"text,omitempty"`
	JSON json.RawMessage `json:"json,omitempty"`
}

// chatTextResponse is a non-streaming Chat Completions response.
type chatTextResponse struct {
	Id      string           `json:"id"`
	Model   string           `json:"model"`
	Object  string           `json:"object"`
	Created int64            `json:"created"`
	Choices []chatTextChoice `json:"choices"`
	Usage   relaymodel.Usage `json:"usage"`
}

// chatTextChoice is one Chat Completions choice.
type chatTextChoice struct {
	Index        int                `json:"index"`
	Message      relaymodel.Message `json:"message"`
	FinishReason string             `json:"finish_reason"`
}
