package model

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestNativeSchemaProvenanceOwnership verifies provenance follows schema copies, not serialized caller input.
func TestNativeSchemaProvenanceOwnership(t *testing.T) {
	const destination = 28
	schema := &JSONSchema{Name: "native", Schema: map[string]any{"type": "integer", "minimum": 2}}
	schema.PreserveNativeConstraintsFor(destination)
	require.True(t, schema.PreservesNativeConstraintsFor(destination))
	copied := *schema
	require.True(t, copied.PreservesNativeConstraintsFor(destination))
	require.False(t, schema.PreservesNativeConstraintsFor(destination+1))
	require.False(t, schema.PreservesNativeConstraintsFor(0))
	wire, err := json.Marshal(schema)
	require.NoError(t, err)
	require.JSONEq(t, `{"name":"native","schema":{"type":"integer","minimum":2}}`, string(wire))
	var parsed JSONSchema
	require.NoError(t, json.Unmarshal(wire, &parsed))
	require.False(t, parsed.PreservesNativeConstraintsFor(destination))
	require.NoError(t, json.Unmarshal([]byte(`{"name":"caller","nativeSchemaChannel":28,"schema":{"type":"integer"}}`), &parsed))
	require.False(t, parsed.PreservesNativeConstraintsFor(destination))
	var absent *JSONSchema
	absent.PreserveNativeConstraintsFor(destination)
	require.False(t, absent.PreservesNativeConstraintsFor(destination))
}
