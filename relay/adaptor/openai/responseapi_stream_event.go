package openai

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/songquanpeng/one-api/relay/model"
)

// ResponseAPIStreamEvent represents a flexible structure for Response API streaming events
// This handles different event types that have varying schemas
type ResponseAPIStreamEvent struct {
	// Common fields for all events
	Type           string `json:"type,omitempty"`            // Event type (e.g., "response.output_text.done")
	SequenceNumber int    `json:"sequence_number,omitempty"` // Sequence number for ordering

	// Response-level events (type starts with "response.")
	Response *ResponseAPIResponse `json:"response,omitempty"` // Full response object for response-level events
	// Required action events (response.required_action.*)
	RequiredAction *ResponseAPIRequiredAction `json:"required_action,omitempty"`

	// Output item events (type contains "output_item")
	OutputIndex int         `json:"output_index,omitempty"` // Index of the output item
	Item        *OutputItem `json:"item,omitempty"`         // Output item for item-level events

	// Content events (type contains "content" or "output_text")
	ItemId       string          `json:"item_id,omitempty"`       // ID of the item containing the content
	ContentIndex int             `json:"content_index,omitempty"` // Index of the content within the item
	Part         *OutputContent  `json:"part,omitempty"`          // Content part for part-level events
	Delta        json.RawMessage `json:"delta,omitempty"`         // Delta payload (string or object)
	Text         string          `json:"text,omitempty"`          // Full text content (for done events)

	// Function call events (type contains "function_call")
	Arguments string          `json:"arguments,omitempty"` // Complete function arguments (for done events)
	Output    json.RawMessage `json:"output,omitempty"`    // Output payload for structured data events
	JSON      json.RawMessage `json:"json,omitempty"`      // Direct JSON payload field when provided

	// General fields that might be in any event
	Id     string       `json:"id,omitempty"`     // Event ID
	Status string       `json:"status,omitempty"` // Event status
	Usage  *model.Usage `json:"usage,omitempty"`  // Usage information
}

// ParseResponseAPIStreamEvent attempts to parse a streaming event as either a full response
// or a streaming event, returning the appropriate data structure
func ParseResponseAPIStreamEvent(data []byte) (*ResponseAPIResponse, *ResponseAPIStreamEvent, error) {
	// First try to parse as a full ResponseAPIResponse (for response-level events)
	var fullResponse ResponseAPIResponse
	if err := json.Unmarshal(data, &fullResponse); err == nil && fullResponse.Id != "" {
		return &fullResponse, nil, nil
	}

	// If that fails, try to parse as a streaming event
	var streamEvent ResponseAPIStreamEvent
	if err := json.Unmarshal(data, &streamEvent); err != nil {
		return nil, nil, errors.Wrap(err, "ParseResponseAPIStreamEvent: failed to unmarshal as stream event")
	}

	return nil, &streamEvent, nil
}

// ConvertStreamEventToResponse converts a streaming event to a ResponseAPIResponse structure
// This allows us to use the existing conversion logic for different event types
func ConvertStreamEventToResponse(event *ResponseAPIStreamEvent) ResponseAPIResponse {
	// Convert model.Usage to ResponseAPIUsage if present
	var responseUsage *ResponseAPIUsage
	if event.Usage != nil {
		responseUsage = (&ResponseAPIUsage{}).FromModelUsage(event.Usage)
	}

	response := ResponseAPIResponse{
		Id:        event.Id,
		Object:    "response",
		Status:    "in_progress", // Default status for streaming events
		Usage:     responseUsage,
		CreatedAt: 0, // Will be filled by the conversion logic if needed
	}

	// If the event already has a specific status, use it
	if event.Status != "" {
		response.Status = event.Status
	}

	// Handle different event types
	switch {
	case event.Response != nil:
		// Handle events that contain a full response object (response.created, response.completed, etc.)
		return *event.Response

	case strings.HasPrefix(event.Type, "response.reasoning_summary_text.delta"):
		// Handle reasoning summary text delta events
		if delta := extractStringFromRaw(event.Delta, "text", "delta"); delta != "" {
			outputItem := OutputItem{
				Type: "reasoning",
				Summary: []OutputContent{
					{
						Type: "summary_text",
						Text: delta,
					},
				},
			}
			response.Output = []OutputItem{outputItem}
		}

	case strings.HasPrefix(event.Type, "response.reasoning_summary_text.done"):
		// Handle reasoning summary text completion events
		if event.Text != "" {
			outputItem := OutputItem{
				Type: "reasoning",
				Summary: []OutputContent{
					{
						Type: "summary_text",
						Text: event.Text,
					},
				},
			}
			response.Output = []OutputItem{outputItem}
		}

	case strings.HasPrefix(event.Type, "response.output_text.delta"):
		// Handle text delta events
		if delta := extractStringFromRaw(event.Delta, "text", "delta"); delta != "" {
			outputItem := OutputItem{
				Type: "message",
				Role: "assistant",
				Content: []OutputContent{
					{
						Type: "output_text",
						Text: delta,
					},
				},
			}
			response.Output = []OutputItem{outputItem}
		}

	case strings.HasPrefix(event.Type, "response.output_json.delta"):
		// Handle structured JSON delta events
		if partial := extractStringFromRaw(event.Delta, "partial_json", "json", "text"); partial != "" {
			outputItem := OutputItem{
				Type: "message",
				Role: "assistant",
				Content: []OutputContent{
					{
						Type: "output_json",
						Text: partial,
					},
				},
			}
			response.Output = []OutputItem{outputItem}
		}

	case strings.HasPrefix(event.Type, "response.output_text.done"):
		// Handle text completion events
		if event.Text != "" {
			outputItem := OutputItem{
				Type: "message",
				Role: "assistant",
				Content: []OutputContent{
					{
						Type: "output_text",
						Text: event.Text,
					},
				},
			}
			response.Output = []OutputItem{outputItem}
		}

	case strings.HasPrefix(event.Type, "response.output_json.done"):
		// Handle structured JSON completion events
		if jsonPayload := extractJSONPayloadFromEvent(event); len(jsonPayload) > 0 {
			outputItem := OutputItem{
				Type: "message",
				Role: "assistant",
				Content: []OutputContent{
					{
						Type: "output_json",
						JSON: jsonPayload,
					},
				},
			}
			response.Output = []OutputItem{outputItem}
			if response.Status == "in_progress" {
				response.Status = "completed"
			}
		}

	case strings.HasPrefix(event.Type, "response.output_item"):
		// Handle output item events (added, done)
		if event.Item != nil {
			response.Output = []OutputItem{*event.Item}
		}

	case strings.HasPrefix(event.Type, "response.function_call_arguments.delta"):
		// Handle function call arguments delta events
		if delta := extractStringFromRaw(event.Delta, "partial_json", "text", "arguments", "delta"); delta != "" {
			outputItem := OutputItem{
				Type:      "function_call",
				Arguments: delta, // This is a delta, not complete arguments
			}
			response.Output = []OutputItem{outputItem}
		}

	case strings.HasPrefix(event.Type, "response.function_call_arguments.done"):
		// Handle function call arguments completion events
		if event.Arguments != "" {
			outputItem := OutputItem{
				Type:      "function_call",
				Arguments: event.Arguments, // Complete arguments
			}
			response.Output = []OutputItem{outputItem}
		}

	case strings.HasPrefix(event.Type, "response.content_part"):
		// Handle content part events (added, done)
		if event.Part != nil {
			outputItem := OutputItem{
				Type:    "message",
				Role:    "assistant",
				Content: []OutputContent{*event.Part},
			}
			response.Output = []OutputItem{outputItem}
		}

	case strings.HasPrefix(event.Type, "response.reasoning_summary_part"):
		// Handle reasoning summary part events (added, done)
		if event.Part != nil {
			outputItem := OutputItem{
				Type:    "reasoning",
				Summary: []OutputContent{*event.Part},
			}
			response.Output = []OutputItem{outputItem}
		}

	case strings.HasPrefix(event.Type, "response."):
		// Handle other response-level events (in_progress, etc.)
		// These typically don't have content but may have metadata
		// The response structure is already set up above with basic fields

	default:
		// Unknown event type - log but don't fail
		// The response structure is already set up above with basic fields
	}

	return response
}

func extractStringFromRaw(raw json.RawMessage, keys ...string) string {
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

func extractJSONPayloadFromEvent(event *ResponseAPIStreamEvent) json.RawMessage {
	if event == nil {
		return nil
	}
	if len(event.JSON) > 0 {
		return cloneRawMessage(event.JSON)
	}
	if event.Part != nil {
		if len(event.Part.JSON) > 0 {
			return cloneRawMessage(event.Part.JSON)
		}
		if event.Part.Text != "" {
			return normalizeJSONRaw(event.Part.Text)
		}
	}
	if len(event.Output) > 0 {
		if payload := decodeJSONBlob(event.Output); len(payload) > 0 {
			return payload
		}
	}
	if event.Text != "" {
		return normalizeJSONRaw(event.Text)
	}
	if len(event.Delta) > 0 {
		if partial := extractStringFromRaw(event.Delta, "json", "partial_json", "text"); partial != "" {
			return normalizeJSONRaw(partial)
		}
	}
	return nil
}

func decodeJSONBlob(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var node any
	if err := json.Unmarshal(raw, &node); err != nil {
		return cloneRawMessage(raw)
	}
	if payload := extractJSONValue(node); len(payload) > 0 {
		return payload
	}
	return cloneRawMessage(raw)
}

func extractJSONValue(node any) json.RawMessage {
	switch v := node.(type) {
	case map[string]any:
		if val, ok := v["json"]; ok {
			if payload := extractJSONValue(val); len(payload) > 0 {
				return payload
			}
		}
		if val, ok := v["text"]; ok {
			if payload := extractJSONValue(val); len(payload) > 0 {
				return payload
			}
		}
		if val, ok := v["content"]; ok {
			if payload := extractJSONValue(val); len(payload) > 0 {
				return payload
			}
		}
		if b, err := json.Marshal(v); err == nil {
			return b
		}
	case []any:
		for _, child := range v {
			if payload := extractJSONValue(child); len(payload) > 0 {
				return payload
			}
		}
		if b, err := json.Marshal(v); err == nil {
			return b
		}
	case string:
		return normalizeJSONRaw(v)
	case json.RawMessage:
		return cloneRawMessage(v)
	}
	return nil
}

func normalizeJSONRaw(text string) json.RawMessage {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	if len(trimmed) >= 2 && ((trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"') || (trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'')) {
		if unquoted, err := strconv.Unquote(trimmed); err == nil {
			trimmed = unquoted
		}
	}
	bytes := []byte(trimmed)
	cloned := make([]byte, len(bytes))
	copy(cloned, bytes)
	return json.RawMessage(cloned)
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	cloned := make([]byte, len(raw))
	copy(cloned, raw)
	return json.RawMessage(cloned)
}
