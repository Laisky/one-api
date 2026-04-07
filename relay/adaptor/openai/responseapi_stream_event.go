package openai

import (
	"bytes"
	"encoding/json"
	"io"
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

type responseAPIStreamEnvelope struct {
	Id                 string                         `json:"id,omitempty"`
	Object             string                         `json:"object,omitempty"`
	CreatedAt          int64                          `json:"created_at,omitempty"`
	Status             string                         `json:"status,omitempty"`
	Model              string                         `json:"model,omitempty"`
	Instructions       *string                        `json:"instructions,omitempty"`
	MaxOutputTokens    *int                           `json:"max_output_tokens,omitempty"`
	Metadata           any                            `json:"metadata,omitempty"`
	ParallelToolCalls  bool                           `json:"parallel_tool_calls,omitempty"`
	PreviousResponseId *string                        `json:"previous_response_id,omitempty"`
	Reasoning          *model.OpenAIResponseReasoning `json:"reasoning,omitempty"`
	ServiceTier        *string                        `json:"service_tier,omitempty"`
	Temperature        *float64                       `json:"temperature,omitempty"`
	ToolChoice         any                            `json:"tool_choice,omitempty"`
	Tools              []model.Tool                   `json:"tools,omitempty"`
	RequiredAction     *ResponseAPIRequiredAction     `json:"required_action,omitempty"`
	TopP               *float64                       `json:"top_p,omitempty"`
	Truncation         *string                        `json:"truncation,omitempty"`
	User               *string                        `json:"user,omitempty"`
	Error              *model.Error                   `json:"error,omitempty"`
	IncompleteDetails  *IncompleteDetails             `json:"incomplete_details,omitempty"`
	UsageRaw           json.RawMessage                `json:"usage,omitempty"`
	OutputRaw          json.RawMessage                `json:"output,omitempty"`
	TextRaw            json.RawMessage                `json:"text,omitempty"`

	Type           string               `json:"type,omitempty"`
	SequenceNumber int                  `json:"sequence_number,omitempty"`
	Response       *ResponseAPIResponse `json:"response,omitempty"`
	OutputIndex    *int                 `json:"output_index,omitempty"`
	Item           *OutputItem          `json:"item,omitempty"`
	ItemId         string               `json:"item_id,omitempty"`
	ContentIndex   int                  `json:"content_index,omitempty"`
	Part           *OutputContent       `json:"part,omitempty"`
	Delta          json.RawMessage      `json:"delta,omitempty"`
	Arguments      string               `json:"arguments,omitempty"`
	JSONRaw        json.RawMessage      `json:"json,omitempty"`
}

// ParseResponseAPIStreamEvent attempts to parse a streaming event as either a full response
// or a streaming event, returning the appropriate data structure
func ParseResponseAPIStreamEvent(data []byte) (*ResponseAPIResponse, *ResponseAPIStreamEvent, error) {
	return ParseResponseAPIStreamEventFromReader(bytes.NewReader(data))
}

// ParseResponseAPIStreamEventFromReader decodes a Response API stream payload from an io.Reader.
// It accepts either a full response object or a streaming event payload and returns the decoded form.
func ParseResponseAPIStreamEventFromReader(reader io.Reader) (*ResponseAPIResponse, *ResponseAPIStreamEvent, error) {
	var envelope responseAPIStreamEnvelope
	if err := json.NewDecoder(reader).Decode(&envelope); err != nil {
		return nil, nil, errors.Wrap(err, "ParseResponseAPIStreamEventFromReader: decode payload")
	}

	if envelope.Id != "" && envelope.Type == "" {
		fullResponse, err := buildResponseAPIResponseFromEnvelope(&envelope)
		if err != nil {
			return nil, nil, err
		}

		return fullResponse, nil, nil
	}

	streamEvent, err := buildResponseAPIStreamEventFromEnvelope(&envelope)
	if err != nil {
		return nil, nil, errors.Wrap(err, "build response API stream event from envelope")
	}

	return nil, streamEvent, nil
}

// buildResponseAPIResponseFromEnvelope converts the decoded envelope into a full ResponseAPIResponse.
func buildResponseAPIResponseFromEnvelope(envelope *responseAPIStreamEnvelope) (*ResponseAPIResponse, error) {
	if envelope == nil {
		return nil, errors.New("buildResponseAPIResponseFromEnvelope: nil envelope")
	}

	fullResponse := &ResponseAPIResponse{
		Id:                 envelope.Id,
		Object:             envelope.Object,
		CreatedAt:          envelope.CreatedAt,
		Status:             envelope.Status,
		Model:              envelope.Model,
		Instructions:       envelope.Instructions,
		MaxOutputTokens:    envelope.MaxOutputTokens,
		Metadata:           envelope.Metadata,
		ParallelToolCalls:  envelope.ParallelToolCalls,
		PreviousResponseId: envelope.PreviousResponseId,
		Reasoning:          envelope.Reasoning,
		ServiceTier:        envelope.ServiceTier,
		Temperature:        envelope.Temperature,
		ToolChoice:         envelope.ToolChoice,
		Tools:              envelope.Tools,
		RequiredAction:     envelope.RequiredAction,
		TopP:               envelope.TopP,
		Truncation:         envelope.Truncation,
		User:               envelope.User,
		Error:              envelope.Error,
		IncompleteDetails:  envelope.IncompleteDetails,
	}

	if len(envelope.UsageRaw) > 0 {
		var usage ResponseAPIUsage
		if err := json.Unmarshal(envelope.UsageRaw, &usage); err != nil {
			return nil, errors.Wrap(err, "buildResponseAPIResponseFromEnvelope: decode usage")
		}
		fullResponse.Usage = &usage
	}

	if len(envelope.OutputRaw) > 0 {
		var output []OutputItem
		if err := json.Unmarshal(envelope.OutputRaw, &output); err != nil {
			return nil, errors.Wrap(err, "buildResponseAPIResponseFromEnvelope: decode output")
		}
		fullResponse.Output = output
	}

	if len(envelope.TextRaw) > 0 {
		var textConfig ResponseTextConfig
		if err := json.Unmarshal(envelope.TextRaw, &textConfig); err == nil {
			fullResponse.Text = &textConfig
		}
	}

	return fullResponse, nil
}

// buildResponseAPIStreamEventFromEnvelope converts the decoded envelope into a ResponseAPIStreamEvent.
func buildResponseAPIStreamEventFromEnvelope(envelope *responseAPIStreamEnvelope) (*ResponseAPIStreamEvent, error) {
	if envelope == nil {
		return nil, errors.New("buildResponseAPIStreamEventFromEnvelope: nil envelope")
	}

	streamEvent := &ResponseAPIStreamEvent{
		Type:           envelope.Type,
		SequenceNumber: envelope.SequenceNumber,
		Response:       envelope.Response,
		RequiredAction: envelope.RequiredAction,
		Item:           envelope.Item,
		ItemId:         envelope.ItemId,
		ContentIndex:   envelope.ContentIndex,
		Part:           envelope.Part,
		Delta:          cloneResponseAPIStreamRawMessage(envelope.Delta),
		Arguments:      envelope.Arguments,
		Output:         cloneResponseAPIStreamRawMessage(envelope.OutputRaw),
		JSON:           cloneResponseAPIStreamRawMessage(envelope.JSONRaw),
		Id:             envelope.Id,
		Status:         envelope.Status,
	}

	if envelope.OutputIndex != nil {
		streamEvent.OutputIndex = *envelope.OutputIndex
	}

	if len(envelope.UsageRaw) > 0 {
		var responseUsage ResponseAPIUsage
		if err := json.Unmarshal(envelope.UsageRaw, &responseUsage); err != nil {
			return nil, errors.Wrap(err, "buildResponseAPIStreamEventFromEnvelope: decode usage")
		}
		streamEvent.Usage = responseUsage.ToModelUsage()
	}

	if len(envelope.TextRaw) > 0 {
		var text string
		if err := json.Unmarshal(envelope.TextRaw, &text); err == nil {
			streamEvent.Text = text
		}
	}

	return streamEvent, nil
}

// cloneResponseAPIStreamRawMessage copies a json.RawMessage so callers can retain it safely.
func cloneResponseAPIStreamRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}

	dup := make([]byte, len(raw))
	copy(dup, raw)
	return json.RawMessage(dup)
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
