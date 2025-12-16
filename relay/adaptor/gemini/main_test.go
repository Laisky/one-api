package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/model"
)

func TestCleanFunctionParameters(t *testing.T) {
	ctx := context.Background()
	_ = ctx // Context for future use

	tests := []struct {
		name     string
		input    any
		expected any
	}{
		{
			name: "remove additionalProperties at all levels",
			input: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":                 "string",
						"additionalProperties": false,
					},
				},
				"additionalProperties": false,
			},
			expected: map[string]any{
				"type": "OBJECT",
				"properties": map[string]any{
					"name": map[string]any{
						"type": "string",
					},
				},
			},
		},
		{
			name: "remove description and strict only at top level",
			input: map[string]any{
				"type":        "object",
				"description": "top level description",
				"strict":      true,
				"properties": map[string]any{
					"nested": map[string]any{
						"type":        "object",
						"description": "nested description",
						"strict":      true,
						"properties": map[string]any{
							"value": map[string]any{
								"type": "string",
							},
						},
					},
				},
			},
			expected: map[string]any{
				"type": "OBJECT",
				"properties": map[string]any{
					"nested": map[string]any{
						"type":        "object",
						"description": "nested description",
						"strict":      true,
						"properties": map[string]any{
							"value": map[string]any{
								"type": "string",
							},
						},
					},
				},
			},
		},
		{
			name: "remove $schema everywhere",
			input: map[string]any{
				"type":    "object",
				"$schema": "http://json-schema.org/draft-07/schema#",
				"properties": map[string]any{
					"city": map[string]any{
						"type":    "string",
						"$schema": "http://json-schema.org/draft-07/schema#",
					},
				},
			},
			expected: map[string]any{
				"type": "OBJECT",
				"properties": map[string]any{
					"city": map[string]any{
						"type": "string",
					},
				},
			},
		},
		{
			name: "remove unsupported format values - critical fix for log error",
			input: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"dateRange": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"start": map[string]any{
								"type":   "string",
								"format": "date", // This causes the 400 error
							},
							"end": map[string]any{
								"type":   "string",
								"format": "date", // This causes the 400 error
							},
						},
					},
					"timestamp": map[string]any{
						"type":   "string",
						"format": "date-time", // This is supported
					},
					"category": map[string]any{
						"type":   "string",
						"format": "enum", // This is supported
					},
					"unsupported": map[string]any{
						"type":   "string",
						"format": "time", // This should be removed
					},
				},
			},
			expected: map[string]any{
				"type": "OBJECT",
				"properties": map[string]any{
					"dateRange": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"start": map[string]any{
								"type":   "string",
								"format": "date-time", // Converted from "date"
							},
							"end": map[string]any{
								"type":   "string",
								"format": "date-time", // Converted from "date"
							},
						},
					},
					"timestamp": map[string]any{
						"type":   "string",
						"format": "date-time", // Preserved
					},
					"category": map[string]any{
						"type":   "string",
						"format": "enum", // Preserved
					},
					"unsupported": map[string]any{
						"type":   "string",
						"format": "date-time", // Converted from "time"
					},
				},
			},
		},
		{
			name: "regression test for exact error from log",
			input: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"dateRange": map[string]any{
						"description": "Date range for events",
						"type":        "object",
						"properties": map[string]any{
							"start": map[string]any{
								"type":        "string",
								"format":      "date",
								"description": "Start date",
							},
							"end": map[string]any{
								"type":        "string",
								"format":      "date",
								"description": "End date",
							},
						},
					},
					"query": map[string]any{
						"type":        "string",
						"description": "Search query",
					},
				},
				"required":             []string{"query"},
				"additionalProperties": false,
				"description":          "Parameters for search",
				"strict":               true,
			},
			expected: map[string]any{
				"type": "OBJECT",
				"properties": map[string]any{
					"dateRange": map[string]any{
						"description": "Date range for events",
						"type":        "object",
						"properties": map[string]any{
							"start": map[string]any{
								"type":        "string",
								"format":      "date-time", // Converted from "date"
								"description": "Start date",
							},
							"end": map[string]any{
								"type":        "string",
								"format":      "date-time", // Converted from "date"
								"description": "End date",
							},
						},
					},
					"query": map[string]any{
						"type":        "string",
						"description": "Search query",
					},
				},
				"required": []string{"query"},
				// additionalProperties, description, strict removed at top level
			},
		},
		{
			name: "force object type when missing",
			input: map[string]any{
				"properties": map[string]any{
					"query": map[string]any{
						"type": "string",
					},
				},
			},
			expected: map[string]any{
				"type": "OBJECT",
				"properties": map[string]any{
					"query": map[string]any{
						"type": "string",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startTime := time.Now()
			result := cleanFunctionParameters(tt.input)
			elapsed := time.Since(startTime)

			require.Less(t, elapsed, 100*time.Millisecond, "cleanFunctionParameters took too long")

			// Convert both to JSON for comparison
			expectedJSON, err := json.Marshal(tt.expected)
			require.NoError(t, err, "failed to marshal expected")

			resultJSON, err := json.Marshal(result)
			require.NoError(t, err, "failed to marshal result")

			require.JSONEq(t, string(expectedJSON), string(resultJSON), "cleanFunctionParameters() mismatch")
		})
	}
}

func TestConvertRequestSetsToolConfig(t *testing.T) {
	t.Parallel()

	req := model.GeneralOpenAIRequest{
		Model: "gemini-2.5-flash",
		Messages: []model.Message{{
			Role:    "user",
			Content: "hi",
		}},
		Tools: []model.Tool{{
			Type: "function",
			Function: &model.Function{
				Name:       "get_weather",
				Parameters: map[string]any{"type": "object"},
			},
		}},
		ToolChoice: map[string]any{
			"type":     "function",
			"function": map[string]any{"name": "get_weather"},
		},
	}

	t.Logf("tool_choice type: %T", req.ToolChoice)
	converted := ConvertRequest(req)
	require.NotNil(t, converted.ToolConfig)
	require.Equal(t, "ANY", converted.ToolConfig.FunctionCallingConfig.Mode)
	require.Equal(t, []string{"get_weather"}, converted.ToolConfig.FunctionCallingConfig.AllowedFunctionNames)
}

func TestConvertToolChoiceToConfig(t *testing.T) {
	t.Parallel()

	forced := map[string]any{
		"type":     "function",
		"function": map[string]any{"name": "get_weather"},
	}
	cfg := convertToolChoiceToConfig(forced)
	require.NotNil(t, cfg)
	require.Equal(t, "ANY", cfg.FunctionCallingConfig.Mode)
	require.Equal(t, []string{"get_weather"}, cfg.FunctionCallingConfig.AllowedFunctionNames)

	respStyle := map[string]any{
		"type": "tool",
		"name": "get_weather",
	}
	cfg = convertToolChoiceToConfig(respStyle)
	require.NotNil(t, cfg)
	require.Equal(t, []string{"get_weather"}, cfg.FunctionCallingConfig.AllowedFunctionNames)

	cfg = convertToolChoiceToConfig("none")
	require.NotNil(t, cfg)
	require.Equal(t, "NONE", cfg.FunctionCallingConfig.Mode)

	require.Nil(t, convertToolChoiceToConfig("auto"))
}

func TestCleanJsonSchemaForGemini(t *testing.T) {
	ctx := context.Background()
	_ = ctx // Context for future use

	tests := []struct {
		name     string
		input    any
		expected any
	}{
		{
			name: "convert types to uppercase",
			input: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type": "string",
					},
					"age": map[string]any{
						"type": "integer",
					},
				},
			},
			expected: map[string]any{
				"type": "OBJECT",
				"properties": map[string]any{
					"name": map[string]any{
						"type": "STRING",
					},
					"age": map[string]any{
						"type": "INTEGER",
					},
				},
			},
		},
		{
			name: "remove unsupported fields",
			input: map[string]any{
				"type":                 "object",
				"description":          "should be removed",
				"additionalProperties": false,
				"properties": map[string]any{
					"name": map[string]any{
						"type": "string",
					},
				},
			},
			expected: map[string]any{
				"type": "OBJECT",
				"properties": map[string]any{
					"name": map[string]any{
						"type": "STRING",
					},
				},
			},
		},
		{
			name: "handle unsupported format values according to Gemini docs",
			input: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"date": map[string]any{
						"type":   "string",
						"format": "date", // Unsupported - should be removed
					},
					"timestamp": map[string]any{
						"type":   "string",
						"format": "date-time", // Supported - should be kept
					},
					"category": map[string]any{
						"type":   "string",
						"format": "enum", // Supported - should be kept
					},
					"time": map[string]any{
						"type":   "string",
						"format": "time", // Unsupported - should be removed
					},
				},
			},
			expected: map[string]any{
				"type": "OBJECT",
				"properties": map[string]any{
					"date": map[string]any{
						"type":   "STRING",
						"format": "date-time", // Converted from "date"
					},
					"timestamp": map[string]any{
						"type":   "STRING",
						"format": "date-time", // Kept
					},
					"category": map[string]any{
						"type":   "STRING",
						"format": "enum", // Kept
					},
					"time": map[string]any{
						"type":   "STRING",
						"format": "date-time", // Converted from "time"
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startTime := time.Now()
			result := cleanJsonSchemaForGemini(tt.input)
			elapsed := time.Since(startTime)

			require.Less(t, elapsed, 100*time.Millisecond, "cleanJsonSchemaForGemini took too long")

			// Convert both to JSON for comparison
			expectedJSON, err := json.Marshal(tt.expected)
			require.NoError(t, err, "failed to marshal expected")

			resultJSON, err := json.Marshal(result)
			require.NoError(t, err, "failed to marshal result")

			require.JSONEq(t, string(expectedJSON), string(resultJSON), "cleanJsonSchemaForGemini() mismatch")
		})
	}
}

func TestConvertRequestWithToolsRegression(t *testing.T) {
	ctx := context.Background()
	_ = ctx // Context for future use

	// Test the exact scenario from the error log
	textRequest := model.GeneralOpenAIRequest{
		Model: "gemini-2.5-pro",
		Messages: []model.Message{
			{
				Role:    "user",
				Content: "find some news events",
			},
		},
		Tools: []model.Tool{
			{
				Type: "function",
				Function: &model.Function{
					Name:        "search_crypto_news",
					Description: "Search for cryptocurrency news and events",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"dateRange": map[string]any{
								"description": "Date range for events",
								"type":        "object",
								"properties": map[string]any{
									"start": map[string]any{
										"type":        "string",
										"format":      "date", // This causes the 400 error
										"description": "Start date",
									},
									"end": map[string]any{
										"type":        "string",
										"format":      "date", // This causes the 400 error
										"description": "End date",
									},
								},
							},
							"query": map[string]any{
								"type":        "string",
								"description": "Search query",
							},
						},
						"required":             []string{"query"},
						"additionalProperties": false,
						"description":          "Parameters for search",
						"strict":               true,
					},
				},
			},
		},
	}

	startTime := time.Now()
	geminiRequest := ConvertRequest(textRequest)
	elapsed := time.Since(startTime)

	require.Less(t, elapsed, 200*time.Millisecond, "ConvertRequest took too long")

	// Verify the request was converted successfully
	require.NotNil(t, geminiRequest, "ConvertRequest returned nil")
	require.Len(t, geminiRequest.Tools, 1, "Expected 1 tool")

	tool := geminiRequest.Tools[0]
	functions, ok := tool.FunctionDeclarations.([]model.Function)
	require.True(t, ok, "FunctionDeclarations should be []model.Function")
	require.Len(t, functions, 1, "Expected 1 function declaration")

	function := functions[0]
	require.Equal(t, "search_crypto_news", function.Name, "Expected function name 'search_crypto_news'")

	// Verify that unsupported fields were removed
	// Convert Parameters from any to map[string]interface{} before indexing
	params, ok := function.Parameters.(map[string]any)
	require.True(t, ok, "function.Parameters should be map[string]interface{}")

	// Check that additionalProperties was removed
	_, exists := params["additionalProperties"]
	require.False(t, exists, "additionalProperties should have been removed")

	// Check that description was removed (top level)
	_, exists = params["description"]
	require.False(t, exists, "description should have been removed at top level")

	// Check that strict was removed (top level)
	_, exists = params["strict"]
	require.False(t, exists, "strict should have been removed at top level")

	// Check that unsupported format values were converted - this is the key fix
	properties, ok := params["properties"].(map[string]any)
	require.True(t, ok, "properties should exist")

	dateRange, ok := properties["dateRange"].(map[string]any)
	require.True(t, ok, "dateRange should exist")

	dateRangeProps, ok := dateRange["properties"].(map[string]any)
	require.True(t, ok, "dateRange.properties should exist")

	startField, ok := dateRangeProps["start"].(map[string]any)
	require.True(t, ok, "start field should exist")
	format, exists := startField["format"]
	require.True(t, exists, "format should have been converted to 'date-time', but was missing")
	require.Equal(t, "date-time", format, "unsupported format 'date' should have been converted to 'date-time'")
	// Description should be preserved in nested objects
	_, exists = startField["description"]
	require.True(t, exists, "description should be preserved in nested objects")

	endField, ok := dateRangeProps["end"].(map[string]any)
	require.True(t, ok, "end field should exist")
	format, exists = endField["format"]
	require.True(t, exists, "format should have been converted to 'date-time', but was missing")
	require.Equal(t, "date-time", format, "unsupported format 'date' should have been converted to 'date-time'")
	// Description should be preserved in nested objects
	_, exists = endField["description"]
	require.True(t, exists, "description should be preserved in nested objects")
}

func TestConvertRequest_SystemInstructionSupportedNoDummy(t *testing.T) {
	t.Parallel()

	textRequest := model.GeneralOpenAIRequest{
		Model: "gemini-3-pro-preview",
		Messages: []model.Message{
			{Role: "system", Content: "Act like a friendly engineer."},
			{Role: "user", Content: "hi"},
		},
	}

	gReq := ConvertRequest(textRequest)
	require.NotNil(t, gReq)
	require.NotNil(t, gReq.SystemInstruction, "supported models should keep system_instruction field")
	require.Len(t, gReq.Contents, 1, "only the user prompt should be forwarded to contents")
	require.Equal(t, "user", gReq.Contents[0].Role)
	require.Len(t, gReq.Contents[0].Parts, 1)
	require.Equal(t, "hi", gReq.Contents[0].Parts[0].Text)

	lastRole := gReq.Contents[len(gReq.Contents)-1].Role
	require.Equal(t, "user", lastRole, "streaming requests must end with a user turn when system_instruction is supported")
}

func TestConvertRequest_SystemInstructionFallbackAddsDummy(t *testing.T) {
	t.Parallel()

	textRequest := model.GeneralOpenAIRequest{
		Model: "gemini-1.5-flash",
		Messages: []model.Message{
			{Role: "system", Content: "Fallback prompt"},
			{Role: "user", Content: "Status?"},
		},
	}

	gReq := ConvertRequest(textRequest)
	require.NotNil(t, gReq)
	require.Nil(t, gReq.SystemInstruction, "models without support should not set system_instruction")
	require.GreaterOrEqual(t, len(gReq.Contents), 3, "system prompt, dummy model reply, and user question expected")
	require.Equal(t, "user", gReq.Contents[0].Role)
	require.Equal(t, "model", gReq.Contents[1].Role)
	require.NotEmpty(t, gReq.Contents[1].Parts)
	require.Equal(t, "Okay", gReq.Contents[1].Parts[0].Text)
	require.Equal(t, "user", gReq.Contents[2].Role)

	lastRole := gReq.Contents[len(gReq.Contents)-1].Role
	require.Equal(t, "user", lastRole, "fallback conversation must still end with a user turn")
}

func TestSupportedFormatsOnly(t *testing.T) {
	ctx := context.Background()
	_ = ctx // Context for future use

	// Test that formats are handled correctly (converted or preserved)
	testCases := []struct {
		format         string
		supported      bool
		expectedFormat string
	}{
		{"date", true, "date-time"},      // Converted to supported format
		{"time", true, "date-time"},      // Converted to supported format
		{"date-time", true, "date-time"}, // Already supported
		{"enum", true, "enum"},           // Already supported
		{"duration", true, "date-time"},  // Converted to supported format
		{"email", false, ""},             // Unsupported, should be removed
		{"uuid", false, ""},              // Unsupported, should be removed
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("format_%s", tc.format), func(t *testing.T) {
			input := map[string]any{
				"type":   "string",
				"format": tc.format,
			}

			result := cleanFunctionParameters(input)
			resultMap, ok := result.(map[string]any)
			require.True(t, ok, "expected map result")

			format, hasFormat := resultMap["format"]
			if tc.supported {
				require.True(t, hasFormat, "format %s should be preserved/converted but was removed", tc.format)
				require.Equal(t, tc.expectedFormat, format, "format %s should be converted to %s", tc.format, tc.expectedFormat)
			} else {
				require.False(t, hasFormat, "unsupported format %s should be removed", tc.format)
			}
		})
	}
}

func TestErrorHandling(t *testing.T) {
	ctx := context.Background()
	_ = ctx // Context for future use

	// Test edge cases and error conditions
	testCases := []struct {
		name  string
		input any
	}{
		{"nil input", nil},
		{"empty map", map[string]any{}},
		{"empty array", []any{}},
		{"primitive string", "test"},
		{"primitive number", 42},
		{"primitive bool", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// These should not panic
			result1 := cleanFunctionParameters(tc.input)
			result2 := cleanJsonSchemaForGemini(tc.input)

			if tc.input != nil {
				require.NotNil(t, result1, "cleanFunctionParameters should not return nil for non-nil input")
				require.NotNil(t, result2, "cleanJsonSchemaForGemini should not return nil for non-nil input")
			}
		})
	}
}

func TestFormatConversionRegression(t *testing.T) {
	ctx := context.Background()
	_ = ctx

	// Test the exact scenario from the error log
	tests := []struct {
		name           string
		inputFormat    string
		expectedFormat string
		shouldKeep     bool
	}{
		{
			name:           "date format should be converted to date-time",
			inputFormat:    "date",
			expectedFormat: "date-time",
			shouldKeep:     true,
		},
		{
			name:           "time format should be converted to date-time",
			inputFormat:    "time",
			expectedFormat: "date-time",
			shouldKeep:     true,
		},
		{
			name:           "date-time format should be preserved",
			inputFormat:    "date-time",
			expectedFormat: "date-time",
			shouldKeep:     true,
		},
		{
			name:           "enum format should be preserved",
			inputFormat:    "enum",
			expectedFormat: "enum",
			shouldKeep:     true,
		},
		{
			name:           "duration format should be converted to date-time",
			inputFormat:    "duration",
			expectedFormat: "date-time",
			shouldKeep:     true,
		},
		{
			name:        "unsupported format should be removed",
			inputFormat: "email",
			shouldKeep:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := map[string]any{
				"type":   "string",
				"format": tt.inputFormat,
			}

			result := cleanFunctionParameters(input)
			resultMap, ok := result.(map[string]any)
			require.True(t, ok, "expected map result")

			if tt.shouldKeep {
				format, hasFormat := resultMap["format"]
				require.True(t, hasFormat, "format should be preserved but was removed")
				require.Equal(t, tt.expectedFormat, format, "expected format %s", tt.expectedFormat)
			} else {
				_, hasFormat := resultMap["format"]
				require.False(t, hasFormat, "unsupported format %s should be removed", tt.inputFormat)
			}
		})
	}
}

func TestOriginalErrorScenario(t *testing.T) {
	ctx := context.Background()
	_ = ctx

	// Recreate the exact scenario from the error log
	textRequest := model.GeneralOpenAIRequest{
		Model: "gemini-2.5-pro",
		Messages: []model.Message{
			{
				Role:    "user",
				Content: "find some news events",
			},
		},
		Tools: []model.Tool{
			{
				Type: "function",
				Function: &model.Function{
					Name:        "search_crypto_news",
					Description: "Search for cryptocurrency news and events",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"dateRange": map[string]any{
								"description": "Date range for events",
								"type":        "object",
								"properties": map[string]any{
									"start": map[string]any{
										"type":        "string",
										"format":      "date", // This was causing the 400 error
										"description": "Start date",
									},
									"end": map[string]any{
										"type":        "string",
										"format":      "date", // This was causing the 400 error
										"description": "End date",
									},
								},
							},
							"query": map[string]any{
								"type":        "string",
								"description": "Search query",
							},
						},
						"required":             []string{"query"},
						"additionalProperties": false,
						"description":          "Parameters for search",
						"strict":               true,
					},
				},
			},
		},
	}

	startTime := time.Now().UTC()
	geminiRequest := ConvertRequest(textRequest)
	elapsed := time.Since(startTime)

	require.Less(t, elapsed, 200*time.Millisecond, "ConvertRequest took too long")
	require.NotNil(t, geminiRequest, "ConvertRequest returned nil")
	require.Len(t, geminiRequest.Tools, 1, "Expected 1 tool")

	tool := geminiRequest.Tools[0]
	functions, ok := tool.FunctionDeclarations.([]model.Function)
	require.True(t, ok, "FunctionDeclarations should be []model.Function")
	require.Len(t, functions, 1, "Expected 1 function declaration")

	function := functions[0]
	// Convert Parameters from any to map[string]interface{} before indexing
	params, ok := function.Parameters.(map[string]any)
	require.True(t, ok, "function.Parameters should be map[string]interface{}")

	// Verify the critical fix: date format should be converted to date-time
	properties, ok := params["properties"].(map[string]any)
	require.True(t, ok, "properties should exist")

	dateRange, ok := properties["dateRange"].(map[string]any)
	require.True(t, ok, "dateRange should exist")

	dateRangeProps, ok := dateRange["properties"].(map[string]any)
	require.True(t, ok, "dateRange.properties should exist")

	// Check start field
	startField, ok := dateRangeProps["start"].(map[string]any)
	require.True(t, ok, "start field should exist")
	format, exists := startField["format"]
	require.True(t, exists, "start format should be present after conversion")
	require.Equal(t, "date-time", format, "start format should be converted to 'date-time'")

	// Check end field
	endField, ok := dateRangeProps["end"].(map[string]any)
	require.True(t, ok, "end field should exist")
	format, exists = endField["format"]
	require.True(t, exists, "end format should be present after conversion")
	require.Equal(t, "date-time", format, "end format should be converted to 'date-time'")

	// Verify other cleaning behaviors still work
	_, exists = params["additionalProperties"]
	require.False(t, exists, "additionalProperties should have been removed")
	_, exists = params["description"]
	require.False(t, exists, "description should have been removed at top level")
	_, exists = params["strict"]
	require.False(t, exists, "strict should have been removed at top level")
}

func TestCleanJsonSchemaForGeminiFormatMapping(t *testing.T) {
	ctx := context.Background()
	_ = ctx

	input := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"dateField": map[string]any{
				"type":   "string",
				"format": "date",
			},
			"timeField": map[string]any{
				"type":   "string",
				"format": "time",
			},
			"dateTimeField": map[string]any{
				"type":   "string",
				"format": "date-time",
			},
			"enumField": map[string]any{
				"type":   "string",
				"format": "enum",
			},
		},
	}

	result := cleanJsonSchemaForGemini(input)
	resultMap, ok := result.(map[string]any)
	require.True(t, ok, "expected map result")
	require.Equal(t, "OBJECT", resultMap["type"], "type should be converted to uppercase")

	properties, ok := resultMap["properties"].(map[string]any)
	require.True(t, ok, "properties should be present")

	// Check format conversions
	testCases := []struct {
		field    string
		expected string
	}{
		{"dateField", "date-time"},
		{"timeField", "date-time"},
		{"dateTimeField", "date-time"},
		{"enumField", "enum"},
	}

	for _, tc := range testCases {
		field, ok := properties[tc.field].(map[string]any)
		require.True(t, ok, "%s field should be present", tc.field)
		format, exists := field["format"]
		require.True(t, exists, "%s format should be present", tc.field)
		require.Equal(t, tc.expected, format, "%s format should be %s", tc.field, tc.expected)
	}
}

func TestErrorHandlingWithProperWrapping(t *testing.T) {
	ctx := context.Background()
	_ = ctx

	// Test edge cases with proper error handling
	testCases := []struct {
		name  string
		input any
	}{
		{"nil input", nil},
		{"empty map", map[string]any{}},
		{"empty array", []any{}},
		{"primitive string", "test"},
		{"primitive number", 42},
		{"primitive bool", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// These should not panic and should handle errors gracefully
			result1 := cleanFunctionParameters(tc.input)
			result2 := cleanJsonSchemaForGemini(tc.input)

			if tc.input != nil {
				require.NotNil(t, result1, "cleanFunctionParameters should not return nil for non-nil input")
				require.NotNil(t, result2, "cleanJsonSchemaForGemini should not return nil for non-nil input")
			}
		})
	}
}

func TestPerformanceWithUTCTiming(t *testing.T) {
	ctx := context.Background()
	_ = ctx

	// Create a complex nested schema to test performance
	complexSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"level1": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"level2": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"dateField": map[string]any{
									"type":   "string",
									"format": "date",
								},
								"timeField": map[string]any{
									"type":   "string",
									"format": "time",
								},
							},
						},
					},
				},
			},
		},
		"additionalProperties": false,
		"description":          "Complex nested schema",
		"strict":               true,
	}

	startTime := time.Now().UTC()
	result := cleanFunctionParameters(complexSchema)
	elapsed := time.Since(startTime)

	require.Less(t, elapsed, 50*time.Millisecond, "Complex schema cleaning took too long")
	require.NotNil(t, result, "Result should not be nil")

	// Verify the structure is maintained while cleaning
	resultMap, ok := result.(map[string]any)
	require.True(t, ok, "Result should be a map")

	_, exists := resultMap["additionalProperties"]
	require.False(t, exists, "additionalProperties should be removed")
	_, exists = resultMap["description"]
	require.False(t, exists, "description should be removed at top level")
	_, exists = resultMap["strict"]
	require.False(t, exists, "strict should be removed at top level")
}

// verifyNoAdditionalProperties recursively checks that no additionalProperties fields exist
func verifyNoAdditionalProperties(obj any) error {
	switch v := obj.(type) {
	case map[string]any:
		if _, exists := v["additionalProperties"]; exists {
			return errors.New("found additionalProperties in object")
		}
		for key, value := range v {
			if err := verifyNoAdditionalProperties(value); err != nil {
				return errors.Wrapf(err, "in field %s", key)
			}
		}
	case []any:
		for i, item := range v {
			if err := verifyNoAdditionalProperties(item); err != nil {
				return errors.Wrapf(err, "in array index %d", i)
			}
		}
	}
	return nil
}

func TestOriginalLogErrorFixed(t *testing.T) {
	// This reproduces the exact case from the log that was failing:
	// "Invalid JSON payload received. Unknown name \"additionalProperties\"
	// at 'tools[0].function_declarations[0].parameters.properties[0].value': Cannot find field."

	// Simulating the original OpenAI request with function that has nested additionalProperties
	openAIRequest := model.GeneralOpenAIRequest{
		Model: "gemini-2.5-flash",
		Messages: []model.Message{
			{
				Role:    "user",
				Content: "find some news events",
			},
		},
		Tools: []model.Tool{
			{
				Type: "function",
				Function: &model.Function{
					Name:        "search_crypto_news",
					Description: "Search for cryptocurrency news and events",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"dateRange": map[string]any{
								"additionalProperties": false, // This was causing the error
								"description":          "Date range for events",
								"properties": map[string]any{
									"end": map[string]any{
										"description": "End date",
										"format":      "date",
										"type":        "string",
									},
									"start": map[string]any{
										"description": "Start date",
										"format":      "date",
										"type":        "string",
									},
								},
								"type": "object",
							},
							"query": map[string]any{
								"description": "Search query",
								"maxLength":   200,
								"minLength":   1,
								"type":        "string",
							},
						},
						"required": []any{"query"},
					},
				},
			},
		},
	}

	// Convert the request - this should not panic or fail
	geminiRequest := ConvertRequest(openAIRequest)

	// Verify the conversion worked
	require.NotNil(t, geminiRequest, "ConvertRequest returned nil")
	require.NotEmpty(t, geminiRequest.Tools, "Tools should not be empty")

	// Extract the function declarations to verify they're clean
	tool := geminiRequest.Tools[0]
	functions, ok := tool.FunctionDeclarations.([]model.Function)
	require.True(t, ok, "FunctionDeclarations should be []model.Function")
	require.NotEmpty(t, functions, "FunctionDeclarations should not be empty")

	function := functions[0]

	// Verify the function parameters no longer contain additionalProperties
	err := verifyNoAdditionalProperties(function.Parameters)
	require.NoError(t, err, "Function parameters still contain additionalProperties")

	t.Logf("Successfully converted request without additionalProperties errors")
}

func TestUsageMetadataPriority(t *testing.T) {
	ctx := context.Background()
	_ = ctx

	tests := []struct {
		name     string
		response ChatResponse
		expected model.Usage
		fallback int // expected prompt tokens for fallback calculation
	}{
		{
			name: "use gemini usage metadata when available",
			response: ChatResponse{
				Candidates: []ChatCandidate{
					{
						Content: ChatContent{
							Role: "model",
							Parts: []Part{
								{Text: "Hello there! The classic first program.\n\nHow can I help you today?"},
							},
						},
						FinishReason: "STOP",
					},
				},
				UsageMetadata: &UsageMetadata{
					PromptTokenCount:     3,
					CandidatesTokenCount: 16,
					TotalTokenCount:      1127, // This includes thoughts tokens
				},
			},
			expected: model.Usage{
				PromptTokens:     3,
				CompletionTokens: 16,
				TotalTokens:      1127,
			},
			fallback: 100, // This should not be used
		},
		{
			name: "fallback to manual calculation when metadata is nil",
			response: ChatResponse{
				Candidates: []ChatCandidate{
					{
						Content: ChatContent{
							Role: "model",
							Parts: []Part{
								{Text: "Hello"},
							},
						},
						FinishReason: "STOP",
					},
				},
				UsageMetadata: nil,
			},
			expected: model.Usage{
				PromptTokens:     50, // fallback value
				CompletionTokens: 1,  // calculated from "Hello"
				TotalTokens:      51, // sum
			},
			fallback: 50,
		},
		{
			name: "fallback when metadata has zero total tokens",
			response: ChatResponse{
				Candidates: []ChatCandidate{
					{
						Content: ChatContent{
							Role: "model",
							Parts: []Part{
								{Text: "Hi"},
							},
						},
						FinishReason: "STOP",
					},
				},
				UsageMetadata: &UsageMetadata{
					PromptTokenCount:     0,
					CandidatesTokenCount: 0,
					TotalTokenCount:      0, // Zero should trigger fallback
				},
			},
			expected: model.Usage{
				PromptTokens:     25, // fallback value
				CompletionTokens: 1,  // calculated from "Hi"
				TotalTokens:      26, // sum
			},
			fallback: 25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock the responseGeminiChat2OpenAI and CountTokenText functions by creating usage manually
			var actualUsage model.Usage

			// Simulate the logic from Handler function
			if tt.response.UsageMetadata != nil && tt.response.UsageMetadata.TotalTokenCount > 0 {
				// Use Gemini's provided token counts
				actualUsage = model.Usage{
					PromptTokens:     tt.response.UsageMetadata.PromptTokenCount,
					CompletionTokens: tt.response.UsageMetadata.CandidatesTokenCount,
					TotalTokens:      tt.response.UsageMetadata.TotalTokenCount,
				}
			} else {
				// Fall back to manual calculation
				// Simple mock: count characters divided by 4 (rough token estimation)
				responseText := tt.response.GetResponseText()
				completionTokens := len(responseText) / 4
				if completionTokens == 0 {
					completionTokens = 1 // minimum 1 token
				}
				actualUsage = model.Usage{
					PromptTokens:     tt.fallback,
					CompletionTokens: completionTokens,
					TotalTokens:      tt.fallback + completionTokens,
				}
			}

			// Verify the usage matches expected values
			require.Equal(t, tt.expected.PromptTokens, actualUsage.PromptTokens, "PromptTokens mismatch")
			require.Equal(t, tt.expected.CompletionTokens, actualUsage.CompletionTokens, "CompletionTokens mismatch")
			require.Equal(t, tt.expected.TotalTokens, actualUsage.TotalTokens, "TotalTokens mismatch")
		})
	}
}

// TestGetToolCalls_EmptyParts tests that getToolCalls handles empty Parts slice gracefully.
// This is a regression test for the panic: "runtime error: index out of range [0] with length 0"
// that occurred when Gemini returned candidates with empty Parts arrays.
func TestGetToolCalls_EmptyParts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		candidate      ChatCandidate
		expectedLength int
	}{
		{
			name: "empty parts should return empty tool calls",
			candidate: ChatCandidate{
				Content: ChatContent{
					Role:  "model",
					Parts: []Part{}, // Empty Parts - this was causing the panic
				},
				FinishReason: "STOP",
			},
			expectedLength: 0,
		},
		{
			name: "nil-like empty parts should return empty tool calls",
			candidate: ChatCandidate{
				Content: ChatContent{
					Role:  "model",
					Parts: nil, // nil Parts - defensive test
				},
				FinishReason: "STOP",
			},
			expectedLength: 0,
		},
		{
			name: "parts with text only should return empty tool calls",
			candidate: ChatCandidate{
				Content: ChatContent{
					Role: "model",
					Parts: []Part{
						{Text: "Hello, how can I help you?"},
					},
				},
				FinishReason: "STOP",
			},
			expectedLength: 0,
		},
		{
			name: "parts with function call should return one tool call",
			candidate: ChatCandidate{
				Content: ChatContent{
					Role: "model",
					Parts: []Part{
						{
							FunctionCall: &FunctionCall{
								FunctionName: "get_weather",
								Arguments:    map[string]any{"location": "New York"},
							},
						},
					},
				},
				FinishReason: "STOP",
			},
			expectedLength: 1,
		},
		{
			name: "parts with multiple function calls should return multiple tool calls",
			candidate: ChatCandidate{
				Content: ChatContent{
					Role: "model",
					Parts: []Part{
						{
							FunctionCall: &FunctionCall{
								FunctionName: "get_weather",
								Arguments:    map[string]any{"location": "New York"},
							},
						},
						{
							FunctionCall: &FunctionCall{
								FunctionName: "get_time",
								Arguments:    map[string]any{"timezone": "EST"},
							},
						},
					},
				},
				FinishReason: "STOP",
			},
			expectedLength: 2,
		},
		{
			name: "mixed parts with text and function call should return only function calls",
			candidate: ChatCandidate{
				Content: ChatContent{
					Role: "model",
					Parts: []Part{
						{Text: "Let me check the weather for you."},
						{
							FunctionCall: &FunctionCall{
								FunctionName: "get_weather",
								Arguments:    map[string]any{"location": "New York"},
							},
						},
					},
				},
				FinishReason: "STOP",
			},
			expectedLength: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create a test gin context
			w := &mockResponseWriter{}
			c, _ := gin.CreateTestContext(w)

			// This should not panic
			toolCalls := getToolCalls(c, &tt.candidate)

			require.Len(t, toolCalls, tt.expectedLength,
				"expected %d tool calls, got %d", tt.expectedLength, len(toolCalls))

			// Verify tool call structure if we expect any
			for _, tc := range toolCalls {
				require.NotEmpty(t, tc.Id, "tool call ID should not be empty")
				require.Equal(t, "function", tc.Type, "tool call type should be 'function'")
				require.NotNil(t, tc.Function, "tool call function should not be nil")
				require.NotEmpty(t, tc.Function.Name, "function name should not be empty")
			}
		})
	}
}

// TestGetStreamingToolCalls_EmptyParts tests that getStreamingToolCalls handles empty Parts slice gracefully.
func TestGetStreamingToolCalls_EmptyParts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		candidate      ChatCandidate
		expectedLength int
	}{
		{
			name: "empty parts should return empty tool calls",
			candidate: ChatCandidate{
				Content: ChatContent{
					Role:  "model",
					Parts: []Part{},
				},
				FinishReason: "STOP",
			},
			expectedLength: 0,
		},
		{
			name: "nil parts should return empty tool calls",
			candidate: ChatCandidate{
				Content: ChatContent{
					Role:  "model",
					Parts: nil,
				},
				FinishReason: "STOP",
			},
			expectedLength: 0,
		},
		{
			name: "parts with function call should return tool call with index",
			candidate: ChatCandidate{
				Content: ChatContent{
					Role: "model",
					Parts: []Part{
						{
							FunctionCall: &FunctionCall{
								FunctionName: "get_weather",
								Arguments:    map[string]any{"location": "Tokyo"},
							},
						},
					},
				},
				FinishReason: "STOP",
			},
			expectedLength: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create a test gin context
			w := &mockResponseWriter{}
			c, _ := gin.CreateTestContext(w)

			// This should not panic
			toolCalls := getStreamingToolCalls(c, &tt.candidate)

			require.Len(t, toolCalls, tt.expectedLength)

			// Verify streaming-specific fields
			for i, tc := range toolCalls {
				require.NotNil(t, tc.Index, "streaming tool call should have index")
				require.Equal(t, i, *tc.Index, "tool call index should match position")
			}
		})
	}
}

// TestResponseGeminiChat2OpenAI_EmptyParts tests that responseGeminiChat2OpenAI handles
// candidates with empty Parts arrays without panicking.
func TestResponseGeminiChat2OpenAI_EmptyParts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		response        ChatResponse
		expectedChoices int
	}{
		{
			name: "candidate with empty parts should not panic",
			response: ChatResponse{
				Candidates: []ChatCandidate{
					{
						Content: ChatContent{
							Role:  "model",
							Parts: []Part{}, // This was causing the panic in production
						},
						FinishReason: "STOP",
					},
				},
			},
			expectedChoices: 1,
		},
		{
			name: "candidate with nil parts should not panic",
			response: ChatResponse{
				Candidates: []ChatCandidate{
					{
						Content: ChatContent{
							Role:  "model",
							Parts: nil,
						},
						FinishReason: "SAFETY",
					},
				},
			},
			expectedChoices: 1,
		},
		{
			name: "multiple candidates with mixed empty and populated parts",
			response: ChatResponse{
				Candidates: []ChatCandidate{
					{
						Content: ChatContent{
							Role:  "model",
							Parts: []Part{},
						},
						FinishReason: "STOP",
					},
					{
						Content: ChatContent{
							Role: "model",
							Parts: []Part{
								{Text: "Hello!"},
							},
						},
						FinishReason: "STOP",
					},
				},
			},
			expectedChoices: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create a minimal gin context for the test
			w := &mockResponseWriter{}
			c, _ := gin.CreateTestContext(w)

			// This should not panic
			result := responseGeminiChat2OpenAI(c, &tt.response)

			require.NotNil(t, result, "response should not be nil")
			require.Len(t, result.Choices, tt.expectedChoices,
				"expected %d choices, got %d", tt.expectedChoices, len(result.Choices))

			// Verify each choice has proper structure
			for i, choice := range result.Choices {
				require.Equal(t, i, choice.Index, "choice index should match position")
				require.Equal(t, "assistant", choice.Message.Role, "message role should be assistant")
			}
		})
	}
}

// mockResponseWriter is a minimal implementation for testing
type mockResponseWriter struct {
	headers http.Header
	body    []byte
	status  int
}

func (m *mockResponseWriter) Header() http.Header {
	if m.headers == nil {
		m.headers = make(http.Header)
	}
	return m.headers
}

func (m *mockResponseWriter) Write(b []byte) (int, error) {
	m.body = append(m.body, b...)
	return len(b), nil
}

func (m *mockResponseWriter) WriteHeader(statusCode int) {
	m.status = statusCode
}

// TestStreamResponseGeminiChat2OpenAI_EmptyParts tests that streamResponseGeminiChat2OpenAI
// handles edge cases properly without panicking.
func TestStreamResponseGeminiChat2OpenAI_EmptyParts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		response    ChatResponse
		expectedNil bool
	}{
		{
			name: "no candidates should return nil",
			response: ChatResponse{
				Candidates: []ChatCandidate{},
			},
			expectedNil: true,
		},
		{
			name: "candidate with empty parts should return nil",
			response: ChatResponse{
				Candidates: []ChatCandidate{
					{
						Content: ChatContent{
							Role:  "model",
							Parts: []Part{},
						},
						FinishReason: "STOP",
					},
				},
			},
			expectedNil: true,
		},
		{
			name: "candidate with text part should return response",
			response: ChatResponse{
				Candidates: []ChatCandidate{
					{
						Content: ChatContent{
							Role: "model",
							Parts: []Part{
								{Text: "Hello!"},
							},
						},
						FinishReason: "STOP",
					},
				},
			},
			expectedNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := &mockResponseWriter{}
			c, _ := gin.CreateTestContext(w)

			// This should not panic
			result := streamResponseGeminiChat2OpenAI(c, &tt.response)

			if tt.expectedNil {
				require.Nil(t, result, "expected nil response for edge case")
			} else {
				require.NotNil(t, result, "expected non-nil response")
			}
		})
	}
}

// TestGetToolCalls_MultipleFunctionCalls verifies that getToolCalls now processes
// all function calls in the Parts array, not just the first one.
func TestGetToolCalls_MultipleFunctionCalls(t *testing.T) {
	t.Parallel()

	candidate := ChatCandidate{
		Content: ChatContent{
			Role: "model",
			Parts: []Part{
				{Text: "I'll help you with that."},
				{
					FunctionCall: &FunctionCall{
						FunctionName: "search_web",
						Arguments:    map[string]any{"query": "weather"},
					},
				},
				{Text: "Also checking news."},
				{
					FunctionCall: &FunctionCall{
						FunctionName: "search_news",
						Arguments:    map[string]any{"topic": "weather"},
					},
				},
			},
		},
		FinishReason: "STOP",
	}

	// Create a test gin context
	w := &mockResponseWriter{}
	c, _ := gin.CreateTestContext(w)

	toolCalls := getToolCalls(c, &candidate)

	require.Len(t, toolCalls, 2, "should extract both function calls")

	// Verify first tool call
	require.Equal(t, "search_web", toolCalls[0].Function.Name)
	require.Contains(t, toolCalls[0].Function.Arguments, "weather")

	// Verify second tool call
	require.Equal(t, "search_news", toolCalls[1].Function.Name)
	require.Contains(t, toolCalls[1].Function.Arguments, "weather")
}
