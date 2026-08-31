package mcp

import (
	"bytes"
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

// ToolDescriptor describes a tool returned by MCP servers and preserves extension fields.
type ToolDescriptor struct {
	Name             string           `json:"-"`
	Title            string           `json:"-"`
	Description      string           `json:"-"`
	InputSchema      map[string]any   `json:"-"`
	OutputSchema     map[string]any   `json:"-"`
	Annotations      map[string]any   `json:"-"`
	Icons            []map[string]any `json:"-"`
	Meta             map[string]any   `json:"-"`
	AdditionalFields map[string]any   `json:"-"`
}

// MarshalJSON encodes a ToolDescriptor with current MCP field names and preserved extensions.
//
// Parameters: none.
//
// Return values:
//   - []byte: the encoded MCP tool descriptor.
//   - error: a wrapped encoding error when a field cannot be represented as JSON.
func (t ToolDescriptor) MarshalJSON() ([]byte, error) {
	payload := make(map[string]any, len(t.AdditionalFields)+8)
	for key, value := range t.AdditionalFields {
		payload[key] = value
	}
	payload["name"] = t.Name
	if t.Title != "" {
		payload["title"] = t.Title
	}
	if t.Description != "" {
		payload["description"] = t.Description
	}
	if t.InputSchema != nil {
		payload["inputSchema"] = t.InputSchema
	}
	if t.OutputSchema != nil {
		payload["outputSchema"] = t.OutputSchema
	}
	if t.Annotations != nil {
		payload["annotations"] = t.Annotations
	}
	if t.Icons != nil {
		payload["icons"] = t.Icons
	}
	if t.Meta != nil {
		payload["_meta"] = t.Meta
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.Wrap(err, "marshal mcp tool descriptor")
	}
	return encoded, nil
}

// UnmarshalJSON decodes an MCP tool descriptor, accepts legacy aliases, and preserves extensions.
//
// Parameters:
//   - data: a complete JSON object containing one MCP tool descriptor.
//
// Return values:
//   - error: a wrapped decoding error when required field types or schemas are malformed.
func (t *ToolDescriptor) UnmarshalJSON(data []byte) error {
	if t == nil {
		return errors.New("mcp tool descriptor is nil")
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return errors.Wrap(err, "unmarshal mcp tool descriptor")
	}

	var decoded ToolDescriptor
	if err := decodeOptionalString(raw, "name", &decoded.Name); err != nil {
		return errors.Wrap(err, "decode mcp tool name")
	}
	if err := decodeOptionalString(raw, "title", &decoded.Title); err != nil {
		return errors.Wrap(err, "decode mcp tool title")
	}
	if err := decodeOptionalString(raw, "description", &decoded.Description); err != nil {
		return errors.Wrap(err, "decode mcp tool description")
	}
	var err error
	decoded.InputSchema, err = decodeOptionalObject(raw, "inputSchema", "input_schema")
	if err != nil {
		return errors.Wrap(err, "decode mcp tool input schema")
	}
	decoded.OutputSchema, err = decodeOptionalObject(raw, "outputSchema", "output_schema")
	if err != nil {
		return errors.Wrap(err, "decode mcp tool output schema")
	}
	decoded.Annotations, err = decodeOptionalObject(raw, "annotations")
	if err != nil {
		return errors.Wrap(err, "decode mcp tool annotations")
	}
	decoded.Meta, err = decodeOptionalObject(raw, "_meta")
	if err != nil {
		return errors.Wrap(err, "decode mcp tool metadata")
	}
	decoded.Icons, err = decodeOptionalObjectSlice(raw, "icons")
	if err != nil {
		return errors.Wrap(err, "decode mcp tool icons")
	}

	known := map[string]struct{}{
		"name": {}, "title": {}, "description": {},
		"inputSchema": {}, "input_schema": {},
		"outputSchema": {}, "output_schema": {},
		"annotations": {}, "icons": {}, "_meta": {},
	}
	for key, value := range raw {
		if _, exists := known[key]; exists {
			continue
		}
		var extension any
		if err := json.Unmarshal(value, &extension); err != nil {
			return errors.Wrapf(err, "decode mcp tool extension %s", key)
		}
		if decoded.AdditionalFields == nil {
			decoded.AdditionalFields = make(map[string]any)
		}
		decoded.AdditionalFields[key] = extension
	}

	*t = decoded
	return nil
}

// ListToolsResult represents a tools/list response across current and legacy MCP versions.
type ListToolsResult struct {
	ResultType string           `json:"resultType,omitempty"`
	Tools      []ToolDescriptor `json:"tools"`
	NextCursor string           `json:"nextCursor,omitempty"`
	TTLMS      int64            `json:"ttlMs,omitempty"`
	CacheScope string           `json:"cacheScope,omitempty"`
	Meta       map[string]any   `json:"_meta,omitempty"`
}

// CallToolRequestOptions carries MCP 2026-07-28 multi-round-trip retry fields.
type CallToolRequestOptions struct {
	InputResponses map[string]any `json:"inputResponses,omitempty"`
	RequestState   string         `json:"requestState,omitempty"`
}

// CallToolResult represents an MCP tool result while preserving extension fields.
type CallToolResult struct {
	ResultType        string
	Content           any
	StructuredContent any
	IsError           bool
	InputRequests     map[string]any
	RequestState      string
	Meta              map[string]any
	AdditionalFields  map[string]any
	Raw               json.RawMessage
}

// MarshalJSON encodes a CallToolResult with current camelCase field names and preserved extensions.
//
// Parameters: none.
//
// Return values:
//   - []byte: the encoded MCP tool result.
//   - error: a wrapped encoding error when a field cannot be represented as JSON.
func (c CallToolResult) MarshalJSON() ([]byte, error) {
	payload := make(map[string]any, len(c.AdditionalFields)+7)
	for key, value := range c.AdditionalFields {
		payload[key] = value
	}
	if c.ResultType != "" {
		payload["resultType"] = c.ResultType
	}
	if c.Content != nil {
		payload["content"] = c.Content
	}
	if c.StructuredContent != nil {
		payload["structuredContent"] = c.StructuredContent
	}
	if c.IsError {
		payload["isError"] = true
	}
	if c.InputRequests != nil {
		payload["inputRequests"] = c.InputRequests
	}
	if c.RequestState != "" {
		payload["requestState"] = c.RequestState
	}
	if c.Meta != nil {
		payload["_meta"] = c.Meta
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.Wrap(err, "marshal mcp tool result")
	}
	return encoded, nil
}

// UnmarshalJSON decodes a tool result, accepts legacy aliases, and preserves extension fields.
//
// Parameters:
//   - data: a complete JSON object containing one MCP tool result.
//
// Return values:
//   - error: a wrapped decoding error when a known result field has an invalid type.
func (c *CallToolResult) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("mcp tool result is nil")
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return errors.Wrap(err, "unmarshal mcp tool result")
	}

	var decoded CallToolResult
	decoded.Raw = append(decoded.Raw, data...)
	if err := decodeOptionalString(raw, "resultType", &decoded.ResultType); err != nil {
		return errors.Wrap(err, "decode mcp result type")
	}
	if encoded, exists := raw["content"]; exists {
		if err := json.Unmarshal(encoded, &decoded.Content); err != nil {
			return errors.Wrap(err, "decode mcp result content")
		}
	}
	if encoded, exists := firstRawMessage(raw, "structuredContent", "structured_content"); exists {
		if err := json.Unmarshal(encoded, &decoded.StructuredContent); err != nil {
			return errors.Wrap(err, "decode mcp structured content")
		}
	}
	if encoded, exists := firstRawMessage(raw, "isError", "is_error"); exists {
		if err := json.Unmarshal(encoded, &decoded.IsError); err != nil {
			return errors.Wrap(err, "decode mcp result error flag")
		}
	}
	var err error
	decoded.InputRequests, err = decodeOptionalObject(raw, "inputRequests", "input_requests")
	if err != nil {
		return errors.Wrap(err, "decode mcp input requests")
	}
	if encoded, exists := firstRawMessage(raw, "requestState", "request_state"); exists {
		if err := json.Unmarshal(encoded, &decoded.RequestState); err != nil {
			return errors.Wrap(err, "decode mcp request state")
		}
	}
	decoded.Meta, err = decodeOptionalObject(raw, "_meta")
	if err != nil {
		return errors.Wrap(err, "decode mcp result metadata")
	}

	known := map[string]struct{}{
		"resultType": {}, "content": {},
		"structuredContent": {}, "structured_content": {},
		"isError": {}, "is_error": {},
		"inputRequests": {}, "input_requests": {},
		"requestState": {}, "request_state": {}, "_meta": {},
	}
	for key, value := range raw {
		if _, exists := known[key]; exists {
			continue
		}
		var extension any
		if err := json.Unmarshal(value, &extension); err != nil {
			return errors.Wrapf(err, "decode mcp result extension %s", key)
		}
		if decoded.AdditionalFields == nil {
			decoded.AdditionalFields = make(map[string]any)
		}
		decoded.AdditionalFields[key] = extension
	}

	*c = decoded
	return nil
}

// NormalizeCallToolResult fills protocol-required defaults without changing tool payloads.
//
// Parameters:
//   - result: the tool result returned by an upstream MCP server; nil is allowed.
//
// Return values:
//   - *CallToolResult: a non-nil result whose resultType is populated.
func NormalizeCallToolResult(result *CallToolResult) *CallToolResult {
	if result == nil {
		return &CallToolResult{ResultType: ResultTypeComplete}
	}
	if result.ResultType == "" {
		result.ResultType = ResultTypeComplete
	}
	return result
}

// decodeOptionalString decodes a string field when it is present in raw.
//
// Parameters:
//   - raw: the source JSON object.
//   - name: the field name to inspect.
//   - destination: the string receiving a present value.
//
// Return values:
//   - error: a wrapped type error for a present non-string value.
func decodeOptionalString(raw map[string]json.RawMessage, name string, destination *string) error {
	encoded, exists := raw[name]
	if !exists {
		return nil
	}
	if err := json.Unmarshal(encoded, destination); err != nil {
		return errors.Wrapf(err, "decode string field %s", name)
	}
	return nil
}

// decodeOptionalObject decodes the first present object alias and rejects null or non-object values.
//
// Parameters:
//   - raw: the source JSON object.
//   - names: modern and legacy aliases in precedence order.
//
// Return values:
//   - map[string]any: the decoded object, or nil when every alias is absent.
//   - error: a wrapped type error for a present null or non-object value.
func decodeOptionalObject(raw map[string]json.RawMessage, names ...string) (map[string]any, error) {
	encoded, exists := firstRawMessage(raw, names...)
	if !exists {
		return nil, nil
	}
	if bytes.Equal(bytes.TrimSpace(encoded), []byte("null")) {
		return nil, errors.Errorf("field %s must be an object", names[0])
	}
	var object map[string]any
	if err := json.Unmarshal(encoded, &object); err != nil {
		return nil, errors.Wrapf(err, "decode object field %s", names[0])
	}
	if object == nil {
		return nil, errors.Errorf("field %s must be an object", names[0])
	}
	return object, nil
}

// decodeOptionalObjectSlice decodes an optional array whose elements must be JSON objects.
//
// Parameters:
//   - raw: the source JSON object.
//   - name: the array field name.
//
// Return values:
//   - []map[string]any: the decoded object array, or nil when the field is absent.
//   - error: a wrapped type error for null, non-array, or non-object elements.
func decodeOptionalObjectSlice(raw map[string]json.RawMessage, name string) ([]map[string]any, error) {
	encoded, exists := raw[name]
	if !exists {
		return nil, nil
	}
	if bytes.Equal(bytes.TrimSpace(encoded), []byte("null")) {
		return nil, errors.Errorf("field %s must be an array", name)
	}
	var values []json.RawMessage
	if err := json.Unmarshal(encoded, &values); err != nil {
		return nil, errors.Wrapf(err, "decode object array field %s", name)
	}
	objects := make([]map[string]any, 0, len(values))
	for index, value := range values {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, errors.Errorf("field %s element %d must be an object", name, index)
		}
		var object map[string]any
		if err := json.Unmarshal(value, &object); err != nil {
			return nil, errors.Wrapf(err, "decode object array field %s element %d", name, index)
		}
		if object == nil {
			return nil, errors.Errorf("field %s element %d must be an object", name, index)
		}
		objects = append(objects, object)
	}
	return objects, nil
}

// firstRawMessage returns the first present alias from a JSON object.
//
// Parameters:
//   - raw: the source JSON object.
//   - names: aliases in precedence order.
//
// Return values:
//   - json.RawMessage: the encoded value for the first present alias.
//   - bool: true when an alias was present, including an explicit null value.
func firstRawMessage(raw map[string]json.RawMessage, names ...string) (json.RawMessage, bool) {
	for _, name := range names {
		if encoded, exists := raw[name]; exists {
			return encoded, true
		}
	}
	return nil, false
}
