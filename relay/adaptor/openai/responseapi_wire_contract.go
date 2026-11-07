package openai

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/Laisky/errors/v2"
)

// The Responses wire contract: the gateway synthesizes Responses output for
// upstreams that do not speak the Responses API, and clients decode that output
// with strict schemas. The OpenAI OpenAPI spec (openai-openapi 2.3.0) requires
// fields that Go's omitempty drops when they hold their zero value, such as
// output_index 0, sequence_number 0, an empty function_call arguments string,
// or empty output_text annotations. The marshalers in this file restore exactly
// those fields, so every Responses DTO serializes spec-conformant whichever
// code path builds it. Fields that already carry a value are never touched.

// wireField is one JSON member appended to an encoded object.
type wireField struct {
	key   string
	value string
}

// responseStreamEventRequiredFields lists, per stream event type, the fields
// beyond type and sequence_number that the spec requires and that a zero value
// would otherwise omit. Events that are not listed only gain sequence_number.
var responseStreamEventRequiredFields = map[string][]string{
	"response.output_item.added":             {"output_index"},
	"response.output_item.done":              {"output_index"},
	"response.content_part.added":            {"item_id", "output_index", "content_index"},
	"response.content_part.done":             {"item_id", "output_index", "content_index"},
	"response.output_text.delta":             {"item_id", "output_index", "content_index", "delta", "logprobs"},
	"response.output_text.done":              {"item_id", "output_index", "content_index", "text", "logprobs"},
	"response.refusal.delta":                 {"item_id", "output_index", "content_index", "delta"},
	"response.reasoning_summary_part.added":  {"item_id", "output_index", "summary_index"},
	"response.reasoning_summary_part.done":   {"item_id", "output_index", "summary_index"},
	"response.reasoning_summary_text.delta":  {"item_id", "output_index", "summary_index", "delta"},
	"response.reasoning_summary_text.done":   {"item_id", "output_index", "summary_index", "text"},
	"response.reasoning_text.delta":          {"item_id", "output_index", "content_index", "delta"},
	"response.reasoning_text.done":           {"item_id", "output_index", "content_index", "text"},
	"response.function_call_arguments.delta": {"item_id", "output_index", "delta"},
	"response.function_call_arguments.done":  {"item_id", "output_index", "arguments"},
	"response.custom_tool_call_input.delta":  {"item_id", "output_index", "delta"},
	"response.custom_tool_call_input.done":   {"item_id", "output_index", "input"},
	"response.web_search_call.in_progress":   {"item_id", "output_index"},
	"response.web_search_call.searching":     {"item_id", "output_index"},
	"response.web_search_call.completed":     {"item_id", "output_index"},
}

// MarshalJSON serializes a stream event with every field its event type
// requires, including zero-valued indexes, empty strings and the logprobs
// array that the shared struct cannot carry. It returns the JSON object or a
// wrapped encoding error.
func (e ResponseAPIStreamEvent) MarshalJSON() ([]byte, error) {
	type wire ResponseAPIStreamEvent
	data, err := json.Marshal(wire(e))
	if err != nil {
		return nil, errors.Wrap(err, "marshal response API stream event")
	}
	var extras []wireField
	if e.Type != "" && e.SequenceNumber == 0 {
		extras = append(extras, wireField{"sequence_number", "0"})
	}
	for _, field := range responseStreamEventRequiredFields[e.Type] {
		if value, missing := e.zeroRequiredField(field); missing {
			extras = append(extras, wireField{field, value})
		}
	}
	return appendWireFields(data, extras), nil
}

// zeroRequiredField reports whether field was omitted from the encoded event
// because it holds its zero value, and the JSON zero value to restore.
func (e ResponseAPIStreamEvent) zeroRequiredField(field string) (string, bool) {
	switch field {
	case "output_index":
		return "0", e.OutputIndex == 0
	case "content_index":
		return "0", e.ContentIndex == 0
	case "summary_index":
		return "0", e.SummaryIndex == 0
	case "item_id":
		return `""`, e.ItemId == ""
	case "delta":
		return `""`, len(e.Delta) == 0
	case "text":
		return `""`, e.Text == ""
	case "arguments":
		return `""`, e.Arguments == ""
	case "input":
		return `""`, e.Input == ""
	case "logprobs":
		return "[]", true
	default:
		return "", false
	}
}

// MarshalJSON serializes a content part with the fields its type requires:
// output_text always carries text, annotations and logprobs; text-bearing
// summary, reasoning and input parts always carry text. It returns the JSON
// object or a wrapped encoding error.
func (c OutputContent) MarshalJSON() ([]byte, error) {
	type wire OutputContent
	data, err := json.Marshal(wire(c))
	if err != nil {
		return nil, errors.Wrap(err, "marshal response API output content")
	}
	var extras []wireField
	switch c.Type {
	case "output_text":
		if c.Text == "" {
			extras = append(extras, wireField{"text", `""`})
		}
		if len(c.Annotations) == 0 {
			extras = append(extras, wireField{"annotations", "[]"})
		}
		if len(c.Logprobs) == 0 {
			extras = append(extras, wireField{"logprobs", "[]"})
		}
	case "summary_text", "reasoning_text", "input_text", "text":
		if c.Text == "" {
			extras = append(extras, wireField{"text", `""`})
		}
	}
	return appendWireFields(data, extras), nil
}

// MarshalJSON serializes a Response with the members the spec requires even
// when unset: tools (ToolsEcho when set, else an array), tool_choice ("auto",
// the API default), and the
// nullable error, incomplete_details, instructions, metadata, temperature and
// top_p. It returns the JSON object or a wrapped encoding error.
func (r ResponseAPIResponse) MarshalJSON() ([]byte, error) {
	type wire ResponseAPIResponse
	data, err := json.Marshal(wire(r))
	if err != nil {
		return nil, errors.Wrap(err, "marshal response API response")
	}
	var extras []wireField
	switch {
	case len(r.Tools) > 0:
	case len(r.ToolsEcho) > 0:
		extras = append(extras, wireField{"tools", string(r.ToolsEcho)})
	default:
		extras = append(extras, wireField{"tools", "[]"})
	}
	if r.ToolChoice == nil {
		extras = append(extras, wireField{"tool_choice", `"auto"`})
	}
	nullable := []struct {
		key   string
		unset bool
	}{
		{"error", r.Error == nil},
		{"incomplete_details", r.IncompleteDetails == nil},
		{"instructions", r.Instructions == nil},
		{"metadata", r.Metadata == nil},
		{"temperature", r.Temperature == nil},
		{"top_p", r.TopP == nil},
	}
	for _, member := range nullable {
		if member.unset {
			extras = append(extras, wireField{member.key, "null"})
		}
	}
	return appendWireFields(data, extras), nil
}

// MarshalJSON serializes usage with both detail objects, whose counters the
// spec requires and which Codex decodes strictly once a detail object exists.
// It returns the JSON object or a wrapped encoding error.
func (u ResponseAPIUsage) MarshalJSON() ([]byte, error) {
	type wire ResponseAPIUsage
	encoded := wire(u)
	if encoded.InputTokensDetails == nil {
		encoded.InputTokensDetails = &ResponseAPIInputTokensDetails{}
	}
	if encoded.OutputTokensDetails == nil {
		encoded.OutputTokensDetails = &ResponseAPIOutputTokensDetails{}
	}
	data, err := json.Marshal(encoded)
	if err != nil {
		return nil, errors.Wrap(err, "marshal response API usage")
	}
	return data, nil
}

// appendWireFields appends fields to an encoded JSON object. Parameters: data
// is the object encoding and fields the members it lacks. It returns data
// unchanged when there is nothing to add or data is not an object.
func appendWireFields(data []byte, fields []wireField) []byte {
	if len(fields) == 0 {
		return data
	}
	trimmed := bytes.TrimRight(data, " \t\r\n")
	if len(trimmed) < 2 || trimmed[len(trimmed)-1] != '}' {
		return data
	}
	body := trimmed[:len(trimmed)-1]
	empty := len(bytes.TrimSpace(body)) == 1
	out := append(make([]byte, 0, len(trimmed)), body...)
	for _, field := range fields {
		if !empty {
			out = append(out, ',')
		}
		empty = false
		out = append(out, '"')
		out = append(out, field.key...)
		out = append(out, '"', ':')
		out = append(out, field.value...)
	}
	return append(out, '}')
}

// ResponseToolsEcho renders a request's tool definitions in the Response
// object's tools shape: function tools as the spec's FunctionTool (type, name,
// description, parameters and strict, the last two null when unset) and every
// other tool type as the request carried it. The gateway echoes it on
// responses it synthesizes from non-Responses upstreams. It returns nil for no
// tools, or a wrapped encoding error.
func ResponseToolsEcho(tools []ResponseAPITool) (json.RawMessage, error) {
	if len(tools) == 0 {
		return nil, nil
	}
	echoed := make([]json.RawMessage, 0, len(tools))
	for _, tool := range tools {
		var (
			encoded []byte
			err     error
		)
		if strings.EqualFold(strings.TrimSpace(tool.Type), "function") {
			encoded, err = json.Marshal(responseFunctionToolEcho(tool))
		} else {
			encoded, err = json.Marshal(tool)
		}
		if err != nil {
			return nil, errors.Wrap(err, "marshal response tool echo")
		}
		echoed = append(echoed, encoded)
	}
	data, err := json.Marshal(echoed)
	if err != nil {
		return nil, errors.Wrap(err, "marshal response tools echo")
	}
	return data, nil
}

// responseFunctionToolEchoShape is the spec FunctionTool shape of one function tool.
type responseFunctionToolEchoShape struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters"`
	Strict      *bool  `json:"strict"`
}

// responseFunctionToolEcho maps a function tool, flat or legacy nested, to its
// FunctionTool echo.
func responseFunctionToolEcho(tool ResponseAPITool) responseFunctionToolEchoShape {
	echo := responseFunctionToolEchoShape{Type: "function", Strict: tool.Strict}
	if fn := sanitizeFunctionForRequest(tool); fn != nil {
		echo.Name, echo.Description, echo.Parameters = fn.Name, fn.Description, fn.Parameters
		if echo.Strict == nil {
			echo.Strict = fn.Strict
		}
	}
	return echo
}
