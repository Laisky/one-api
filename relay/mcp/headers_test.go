package mcp

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestToolArgumentHeadersDerivesNestedValues verifies primitive schema annotations become mirrored headers.
func TestToolArgumentHeadersDerivesNestedValues(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tenant": map[string]any{
				"type":         "string",
				"x-mcp-header": "Tenant-ID",
			},
			"options": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"attempt": map[string]any{
						"type":         "integer",
						"x-mcp-header": "Attempt",
					},
					"enabled": map[string]any{
						"type":         "boolean",
						"x-mcp-header": "Enabled",
					},
				},
			},
		},
	}
	arguments := map[string]any{
		"tenant":  "acme",
		"options": map[string]any{"attempt": float64(3), "enabled": true},
	}

	headers, err := ToolArgumentHeaders(schema, arguments)
	require.NoError(t, err)
	require.Equal(t, "acme", headers.Get("Mcp-Param-Tenant-ID"))
	require.Equal(t, "3", headers.Get("Mcp-Param-Attempt"))
	require.Equal(t, "true", headers.Get("Mcp-Param-Enabled"))
}

// TestToolArgumentHeadersEncodesUnsafeValues verifies the exact Base64 sentinel prevents invalid HTTP header values.
func TestToolArgumentHeadersEncodesUnsafeValues(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"value": map[string]any{"type": "string", "x-mcp-header": "Value"},
		},
	}
	headers, err := ToolArgumentHeaders(schema, map[string]any{"value": "=?base64?already-prefixed?="})
	require.NoError(t, err)
	require.Equal(t, "=?base64?PT9iYXNlNjQ/YWxyZWFkeS1wcmVmaXhlZD89?=", headers.Get("Mcp-Param-Value"))

	decoded, err := DecodeMCPHeaderValue("=?base64?SGVsbG8sIOS4lueVjA==?=")
	require.NoError(t, err)
	require.Equal(t, "Hello, 世界", decoded)
}

// TestToolArgumentHeadersOmitsNullValues verifies null and absent parameters do not produce headers.
func TestToolArgumentHeadersOmitsNullValues(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"value": map[string]any{"type": "string", "x-mcp-header": "Value"},
		},
	}
	headers, err := ToolArgumentHeaders(schema, map[string]any{"value": nil})
	require.NoError(t, err)
	require.Empty(t, headers)
}

// TestToolArgumentHeadersRejectsUnsafeIntegerRange verifies integer headers remain exactly representable in JavaScript.
func TestToolArgumentHeadersRejectsUnsafeIntegerRange(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"value": map[string]any{"type": "integer", "x-mcp-header": "Value"},
		},
	}
	_, err := ToolArgumentHeaders(schema, map[string]any{"value": float64(1 << 53)})
	require.ErrorContains(t, err, "JavaScript-safe")
}

// TestValidateToolSchemaHeadersRejectsDuplicateNames verifies header annotations are unique case-insensitively.
func TestValidateToolSchemaHeadersRejectsDuplicateNames(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"first":  map[string]any{"type": "string", "x-mcp-header": "Tenant"},
			"second": map[string]any{"type": "string", "x-mcp-header": "tenant"},
		},
	}
	require.ErrorContains(t, ValidateToolSchemaHeaders(schema), "duplicated")
}

// TestValidateToolSchemaHeadersRejectsUnreachableAnnotations verifies annotations cannot hide behind arrays or composition keywords.
func TestValidateToolSchemaHeadersRejectsUnreachableAnnotations(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":         "string",
					"x-mcp-header": "Hidden",
				},
			},
		},
	}
	require.ErrorContains(t, ValidateToolSchemaHeaders(schema), "not statically reachable")
}

// TestValidateToolArgumentHeadersRejectsMismatch verifies server-side mirrored header checks reject tampering.
func TestValidateToolArgumentHeadersRejectsMismatch(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"tenant": map[string]any{"type": "string", "x-mcp-header": "Tenant"},
		},
	}
	headers := make(http.Header)
	headers.Set("Mcp-Param-Tenant", "other")
	require.ErrorContains(t, ValidateToolArgumentHeaders(headers, schema, map[string]any{"tenant": "acme"}), "mismatch")
}

// TestFilterValidToolDescriptorsExcludesInvalidTools verifies one malformed tool does not hide valid tools.
func TestFilterValidToolDescriptorsExcludesInvalidTools(t *testing.T) {
	tools := []ToolDescriptor{
		{Name: "valid", InputSchema: map[string]any{"type": "object"}},
		{Name: "invalid", InputSchema: map[string]any{"items": map[string]any{"x-mcp-header": "Hidden"}}},
	}
	valid, rejected := FilterValidToolDescriptors(tools)
	require.Equal(t, []ToolDescriptor{tools[0]}, valid)
	require.Len(t, rejected, 1)
	require.Equal(t, "invalid", rejected[0].Name)
}
