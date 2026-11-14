package vertexai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// vertexTypedSchema models typed Schema fields exercised by the offline contract.
type vertexTypedSchema struct {
	Type       string                       `json:"type"`
	Properties map[string]vertexTypedSchema `json:"properties"`
	Items      *vertexTypedSchema           `json:"items"`
	Enum       []any                        `json:"enum"`
	Required   []string                     `json:"required"`
}

// vertexSchemaWire converts a source request through the production Vertex adapter.
func vertexSchemaWire(t *testing.T, request *model.GeneralOpenAIRequest) []byte {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	converted, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, request)
	require.NoError(t, err)
	raw, err := json.Marshal(converted)
	require.NoError(t, err)
	return raw
}

// vertexSchemaRequest constructs a text request with an optional function schema.
func vertexSchemaRequest(schema any) *model.GeneralOpenAIRequest {
	request := &model.GeneralOpenAIRequest{Model: "gemini-3.8-flash", Messages: []model.Message{{Role: "user", Content: "take a note"}}}
	if schema != nil {
		request.Tools = []model.Tool{{Type: "function", Function: &model.Function{Name: "record", Description: "Record a note", Parameters: schema}}}
	}
	return request
}

// TestVertexSchemaContractTransport verifies serialized adapter payloads against an offline schema-envelope mock.
// JSON Value fields are opaque: acceptance does not prove Google's model supports the enclosed keywords.
func TestVertexSchemaContractTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var wire struct {
			Tools []struct {
				Functions []struct {
					Parameters json.RawMessage `json:"parameters"`
					JSON       json.RawMessage `json:"parametersJsonSchema"`
				} `json:"function_declarations"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		for _, group := range wire.Tools {
			for _, fn := range group.Functions {
				if len(fn.Parameters) > 0 && len(fn.JSON) > 0 {
					http.Error(w, "mutually exclusive fields", 400)
					return
				}
				if len(fn.Parameters) > 0 {
					decoder := json.NewDecoder(bytes.NewReader(fn.Parameters))
					decoder.DisallowUnknownFields()
					if err := decoder.Decode(new(vertexTypedSchema)); err != nil {
						http.Error(w, err.Error(), 400)
						return
					}
				}
			}
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	for _, tc := range []struct{ name, schema string }{
		{"no_tools", ""},
		{"enum_control", `{"type":"object","properties":{"notes":{"type":"array","items":{"type":"object","properties":{"kind":{"type":"string","enum":["note"]}}}}}}`},
		{"nested_const", `{"type":"object","properties":{"notes":{"type":"array","items":{"type":"object","properties":{"kind":{"type":"string","const":"note"}}}}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var schema any
			if tc.schema != "" {
				require.NoError(t, json.Unmarshal([]byte(tc.schema), &schema))
			}
			response, err := server.Client().Post(server.URL, "application/json", bytes.NewReader(vertexSchemaWire(t, vertexSchemaRequest(schema))))
			require.NoError(t, err)
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.Equal(t, 200, response.StatusCode, "offline envelope rejected: %s", body)
		})
	}
	response, err := server.Client().Post(server.URL, "application/json", bytes.NewBufferString(`{"tools":[{"function_declarations":[{"parameters":{"type":"object","properties":{"kind":{"const":"note"}}}}]}]}`))
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, 400, response.StatusCode)
	bothResponse, err := server.Client().Post(server.URL, "application/json", bytes.NewBufferString(`{"tools":[{"function_declarations":[{"parameters":{"type":"object"},"parametersJsonSchema":{"type":"object"}}]}]}`))
	require.NoError(t, err)
	defer bothResponse.Body.Close()
	require.Equal(t, http.StatusBadRequest, bothResponse.StatusCode)
}

// TestVertexSchemaContractPreservesJSON checks nested schemas and literal data survive without source mutation.
func TestVertexSchemaContractPreservesJSON(t *testing.T) {
	var schema map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","description":"description","additionalProperties":false,"$defs":{"Note":{"type":"object","properties":{"kind":{"const":"note"},"text":{"type":["string","null"]}}}},"properties":{"items":{"type":"array","items":{"$ref":"#/$defs/Note"}},"const":{"type":"string","const":"keyword property"},"literal":{"enum":[{"const":"literal","additionalProperties":false}],"default":{"const":"literal","format":"date"}},"choice":{"anyOf":[{"type":"integer"},{"type":"null"}]},"intersection":{"allOf":[{"type":"string"},{"minLength":1}]},"exclusive":{"oneOf":[{"const":"a"},{"const":"b"}]}},"required":["items"]}`), &schema))
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "tools", true: "legacy_functions"}[legacy], func(t *testing.T) {
			request := vertexSchemaRequest(schema)
			if legacy {
				request.Functions = []model.Function{*request.Tools[0].Function}
				request.Tools = nil
			}
			before, err := json.Marshal(request)
			require.NoError(t, err)
			var wire struct {
				Tools []struct {
					Functions []map[string]json.RawMessage `json:"function_declarations"`
				} `json:"tools"`
			}
			require.NoError(t, json.Unmarshal(vertexSchemaWire(t, request), &wire))
			require.Len(t, wire.Tools, 1)
			require.Len(t, wire.Tools[0].Functions, 1)
			fn := wire.Tools[0].Functions[0]
			require.NotContains(t, fn, "parameters")
			expected, err := json.Marshal(schema)
			require.NoError(t, err)
			require.JSONEq(t, string(expected), string(fn["parametersJsonSchema"]))
			after, err := json.Marshal(request)
			require.NoError(t, err)
			require.JSONEq(t, string(before), string(after))
		})
	}
}

// TestVertexSchemaContractStructuredOutput checks original response schemas use the JSON Value field.
func TestVertexSchemaContractStructuredOutput(t *testing.T) {
	schema := map[string]any{"$ref": "#/$defs/Note", "$defs": map[string]any{"Note": map[string]any{"type": "object", "properties": map[string]any{"kind": map[string]any{"const": "note"}}}}}
	request := vertexSchemaRequest(nil)
	request.ResponseFormat = &model.ResponseFormat{Type: "json_schema", JsonSchema: &model.JSONSchema{Name: "note", Schema: schema}}
	before, err := json.Marshal(request)
	require.NoError(t, err)
	var wire struct {
		Config map[string]json.RawMessage `json:"generation_config"`
	}
	require.NoError(t, json.Unmarshal(vertexSchemaWire(t, request), &wire))
	require.NotContains(t, wire.Config, "responseSchema")
	require.JSONEq(t, `"application/json"`, string(wire.Config["responseMimeType"]))
	expected, err := json.Marshal(schema)
	require.NoError(t, err)
	require.JSONEq(t, string(expected), string(wire.Config["responseJsonSchema"]))
	after, err := json.Marshal(request)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
}

// TestVertexSchemaContractEmptyTools checks nil and empty catalogs omit function declarations.
func TestVertexSchemaContractEmptyTools(t *testing.T) {
	for _, tools := range [][]model.Tool{nil, {}, {{Type: "function"}}} {
		request := vertexSchemaRequest(nil)
		request.Tools = tools
		var wire map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(vertexSchemaWire(t, request), &wire))
		require.NotContains(t, wire, "tools")
	}
}

// TestVertexSchemaContractRequired lowers the legacy required extension into JSON Schema without mutating caller data.
func TestVertexSchemaContractRequired(t *testing.T) {
	for _, tc := range []struct {
		name     string
		schema   any
		required []string
		want     string
	}{
		{"invalid_null", map[string]any{"type": "object", "required": nil}, []string{"a"}, `{"type":"object","required":null}`},
		{"invalid_list", map[string]any{"type": "object", "required": []any{1}}, []string{"a"}, `{"type":"object","required":[1]}`},
		{"nil_map", map[string]any(nil), []string{"a"}, `null`},
		{"missing", map[string]any{"type": "object"}, []string{"a"}, `{"type":"object","required":["a"]}`},
		{"union", map[string]any{"type": "object", "required": []any{"a"}}, []string{"b", "a"}, `{"type":"object","required":["a","b"]}`},
		{"string_slice", map[string]any{"type": "object", "required": []string{"a"}}, []string{"b"}, `{"type":"object","required":["a","b"]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := vertexSchemaRequest(tc.schema)
			request.Tools[0].Function.Required = tc.required
			before, err := json.Marshal(request)
			require.NoError(t, err)
			var wire struct {
				Tools []struct {
					Functions []map[string]json.RawMessage `json:"function_declarations"`
				} `json:"tools"`
			}
			require.NoError(t, json.Unmarshal(vertexSchemaWire(t, request), &wire))
			fn := wire.Tools[0].Functions[0]
			require.NotContains(t, fn, "required")
			require.JSONEq(t, tc.want, string(fn["parametersJsonSchema"]))
			after, err := json.Marshal(request)
			require.NoError(t, err)
			require.JSONEq(t, string(before), string(after))
		})
	}
}

// TestVertexSchemaContractNoParameters checks parameterless declarations omit both schema fields.
func TestVertexSchemaContractNoParameters(t *testing.T) {
	request := vertexSchemaRequest(map[string]any{"type": "object"})
	request.Tools[0].Function.Parameters = nil
	var wire struct {
		Tools []struct {
			Functions []map[string]json.RawMessage `json:"function_declarations"`
		} `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(vertexSchemaWire(t, request), &wire))
	fn := wire.Tools[0].Functions[0]
	require.NotContains(t, fn, "parameters")
	require.NotContains(t, fn, "parametersJsonSchema")
}

// TestVertexSchemaContractToolsPrecedence preserves tools precedence over the legacy function catalog.
func TestVertexSchemaContractToolsPrecedence(t *testing.T) {
	request := vertexSchemaRequest(map[string]any{"type": "object"})
	request.Functions = []model.Function{{Name: "ignored", Parameters: map[string]any{"type": "object"}}}
	var wire struct {
		Tools []struct {
			Functions []map[string]json.RawMessage `json:"function_declarations"`
		} `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(vertexSchemaWire(t, request), &wire))
	require.Len(t, wire.Tools, 1)
	require.Len(t, wire.Tools[0].Functions, 1)
	require.JSONEq(t, `"record"`, string(wire.Tools[0].Functions[0]["name"]))
}
