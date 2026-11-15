package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

const responseExactBoundsSchema = `{"type":"object","properties":{"count":{"type":"integer","minimum":9007199254740993,"maximum":9007199254740995},"nested":{"type":"array","items":{"type":"number","minimum":9007199254740993.125}}}}`

// assertResponseSchemaPrecision checks exact numeric text after actual DTO decoding and wire encoding.
func assertResponseSchemaPrecision(t *testing.T, value any, schemaPath ...string) {
	t.Helper()
	wire, err := json.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, validateResponseBoundsTokens(wire, schemaPath...))
}

// TestResponseTextFormatExactSchemaNumbers retains nested schema numbers through Responses ingress decoding.
func TestResponseTextFormatExactSchemaNumbers(t *testing.T) {
	var request ResponseAPIRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"gemini-4-hydrogen","input":"extract","text":{"format":{"type":"json_schema","name":"bounds","schema":`+responseExactBoundsSchema+`}}}`), &request))
	assertResponseSchemaPrecision(t, request.Text.Format, "schema")
}

// TestResponseToolExactSchemaNumbers preserves flattened and nested function schemas without changing opaque approval data.
func TestResponseToolExactSchemaNumbers(t *testing.T) {
	for _, nested := range []bool{false, true} {
		raw := `{"type":"function","name":"bounds","parameters":` + responseExactBoundsSchema + `,"require_approval":{"count":1}}`
		if nested {
			raw = `{"type":"function","function":{"name":"bounds","parameters":` + responseExactBoundsSchema + `},"require_approval":{"count":1}}`
		}
		var tool ResponseAPITool
		require.NoError(t, json.Unmarshal([]byte(raw), &tool))
		assertResponseSchemaPrecision(t, tool, "parameters")
		assertResponseSchemaPrecision(t, tool, "function", "parameters")
		require.IsType(t, float64(0), tool.RequireApproval.(map[string]any)["count"])
	}
}
