package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

const exactBoundsSchema = `{"type":"object","properties":{"count":{"type":"integer","minimum":9007199254740993,"maximum":9007199254740995},"nested":{"type":"array","items":{"type":"number","minimum":9007199254740993.125}}}}`

// assertExactBoundsWire checks integer and fractional bounds in a serialized schema carrier.
func assertExactBoundsWire(t *testing.T, value any, schemaPath ...string) {
	t.Helper()
	wire, err := json.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, validateExactBoundsTokens(wire, schemaPath...))
}

// TestJSONSchemaExactNumbers decodes ordinary response_format schemas without rounding nested bounds.
func TestJSONSchemaExactNumbers(t *testing.T) {
	var format ResponseFormat
	require.NoError(t, json.Unmarshal([]byte(`{"type":"json_schema","json_schema":{"name":"bounds","schema":`+exactBoundsSchema+`}}`), &format))
	assertExactBoundsWire(t, format, "json_schema", "schema")
	format.JsonSchema.PreserveNativeConstraintsFor(28)
	require.NoError(t, json.Unmarshal([]byte(`{"name":"caller","nativeSchemaChannel":28,"schema":`+exactBoundsSchema+`}`), format.JsonSchema))
	require.False(t, format.JsonSchema.PreservesNativeConstraintsFor(28), "wire reconstruction must clear trusted provenance on reused receivers")
	assertExactBoundsWire(t, format, "json_schema", "schema")
}

// TestClaudeToolExactSchemaNumbers preserves exact input schema numbers while clearing caller provenance.
func TestClaudeToolExactSchemaNumbers(t *testing.T) {
	tool := ClaudeTool{TrustedDeferredLoading: true}
	require.NoError(t, json.Unmarshal([]byte(`{"name":"bounds","input_schema":`+exactBoundsSchema+`,"TrustedDeferredLoading":true}`), &tool))
	require.False(t, tool.TrustedDeferredLoading)
	assertExactBoundsWire(t, tool, "input_schema")
}

// TestFunctionSchemaNumbersKeepArgumentsContract preserves only schema precision, keeping ordinary argument number types.
func TestFunctionSchemaNumbersKeepArgumentsContract(t *testing.T) {
	var fn Function
	require.NoError(t, json.Unmarshal([]byte(`{"name":"bounds","parameters":`+exactBoundsSchema+`,"arguments":{"count":1}}`), &fn))
	assertExactBoundsWire(t, fn, "parameters")
	require.IsType(t, float64(0), fn.Arguments.(map[string]any)["count"])
}

// TestToolSchemaNumbersKeepArgumentsContract covers nested and flattened Chat tool definitions.
func TestToolSchemaNumbersKeepArgumentsContract(t *testing.T) {
	for _, nested := range []bool{false, true} {
		raw := `{"name":"bounds","parameters":` + exactBoundsSchema + `,"arguments":{"count":1}}`
		if nested {
			raw = `{"type":"function","function":` + raw + `}`
		}
		var tool Tool
		require.NoError(t, json.Unmarshal([]byte(raw), &tool))
		assertExactBoundsWire(t, tool, "function", "parameters")
		require.IsType(t, float64(0), tool.Function.Arguments.(map[string]any)["count"])
	}
}

// TestFunctionSchemaDecoderMergeCompatibility retains standard field merge and opaque argument decoding.
func TestFunctionSchemaDecoderMergeCompatibility(t *testing.T) {
	type legacyFunction Function
	for _, wire := range []string{
		`{"arguments":{"other":2}}`,
		`{"arguments":null}`,
		`{"parameters":null}`,
		`{"name":"renamed","arguments":3}`,
	} {
		got := Function{Name: "existing", Parameters: map[string]any{"type": "integer", "minimum": 1}, Arguments: map[string]any{"count": float64(1)}, Required: []string{"count"}}
		want := legacyFunction{Name: "existing", Parameters: map[string]any{"type": "integer", "minimum": 1}, Arguments: map[string]any{"count": float64(1)}, Required: []string{"count"}}
		require.NoError(t, json.Unmarshal([]byte(wire), &want))
		require.NoError(t, json.Unmarshal([]byte(wire), &got))
		require.Equal(t, Function(want), got)
	}
}

// TestSchemaJSONDecoderRejectsTrailingValues retains strict single-document input boundaries.
func TestSchemaJSONDecoderRejectsTrailingValues(t *testing.T) {
	var value any
	require.ErrorContains(t, DecodeSchemaJSON([]byte(`{} {}`), &value), "exactly one value")
	require.Error(t, DecodeSchemaJSON([]byte(`{"minimum":`), &value))
	require.NoError(t, DecodeSchemaJSON([]byte(`9007199254740993`), &value))
	require.Equal(t, json.Number("9007199254740993"), value)
}

// TestToolNullSchemaKeepsFunctionShape retains omitted/null semantics in flattened tool decoding.
func TestToolNullSchemaKeepsFunctionShape(t *testing.T) {
	for _, raw := range []string{`{"type":"function"}`, `{"type":"function","parameters":null}`} {
		var tool Tool
		require.NoError(t, json.Unmarshal([]byte(raw), &tool))
		require.Nil(t, tool.Function)
	}
}
