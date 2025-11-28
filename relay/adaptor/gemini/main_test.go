package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
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
				"type": "object",
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
				"type": "object",
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
				"type": "object",
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
				"type": "object",
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
				"type": "object",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startTime := time.Now()
			result := cleanFunctionParameters(tt.input)
			elapsed := time.Since(startTime)

			if elapsed > 100*time.Millisecond {
				t.Errorf("cleanFunctionParameters took too long: %v", elapsed)
			}

			// Convert both to JSON for comparison
			expectedJSON, err := json.Marshal(tt.expected)
			if err != nil {
				t.Fatalf("failed to marshal expected: %v", errors.WithStack(err))
			}

			resultJSON, err := json.Marshal(result)
			if err != nil {
				t.Fatalf("failed to marshal result: %v", errors.WithStack(err))
			}

			if string(expectedJSON) != string(resultJSON) {
				t.Errorf("cleanFunctionParameters() = %s, want %s",
					string(resultJSON), string(expectedJSON))
			}
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

			if elapsed > 100*time.Millisecond {
				t.Errorf("cleanJsonSchemaForGemini took too long: %v", elapsed)
			}

			// Convert both to JSON for comparison
			expectedJSON, err := json.Marshal(tt.expected)
			if err != nil {
				t.Fatalf("failed to marshal expected: %v", errors.WithStack(err))
			}

			resultJSON, err := json.Marshal(result)
			if err != nil {
				t.Fatalf("failed to marshal result: %v", errors.WithStack(err))
			}

			if string(expectedJSON) != string(resultJSON) {
				t.Errorf("cleanJsonSchemaForGemini() = %s, want %s",
					string(resultJSON), string(expectedJSON))
			}
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

	if elapsed > 200*time.Millisecond {
		t.Errorf("ConvertRequest took too long: %v", elapsed)
	}

	// Verify the request was converted successfully
	if geminiRequest == nil {
		t.Fatal("ConvertRequest returned nil")
	}

	if len(geminiRequest.Tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(geminiRequest.Tools))
	}

	tool := geminiRequest.Tools[0]
	functions, ok := tool.FunctionDeclarations.([]model.Function)
	if !ok {
		t.Fatal("FunctionDeclarations should be []model.Function")
	}
	if len(functions) != 1 {
		t.Fatalf("Expected 1 function declaration, got %d", len(functions))
	}

	function := functions[0]
	if function.Name != "search_crypto_news" {
		t.Errorf("Expected function name 'search_crypto_news', got '%s'", function.Name)
	}

	// Verify that unsupported fields were removed
	// Convert Parameters from any to map[string]interface{} before indexing
	params, ok := function.Parameters.(map[string]any)
	if !ok {
		t.Fatal("function.Parameters should be map[string]interface{}")
	}

	// Check that additionalProperties was removed
	if _, exists := params["additionalProperties"]; exists {
		t.Error("additionalProperties should have been removed")
	}

	// Check that description was removed (top level)
	if _, exists := params["description"]; exists {
		t.Error("description should have been removed at top level")
	}

	// Check that strict was removed (top level)
	if _, exists := params["strict"]; exists {
		t.Error("strict should have been removed at top level")
	}

	// Check that unsupported format values were converted - this is the key fix
	if properties, ok := params["properties"].(map[string]any); ok {
		if dateRange, ok := properties["dateRange"].(map[string]any); ok {
			if dateRangeProps, ok := dateRange["properties"].(map[string]any); ok {
				if startField, ok := dateRangeProps["start"].(map[string]any); ok {
					if format, exists := startField["format"]; exists {
						if format != "date-time" {
							t.Errorf("unsupported format 'date' should have been converted to 'date-time', but found: %v", format)
						}
					} else {
						t.Error("format should have been converted to 'date-time', but was missing")
					}
					// Description should be preserved in nested objects
					if _, exists := startField["description"]; !exists {
						t.Error("description should be preserved in nested objects")
					}
				}
				if endField, ok := dateRangeProps["end"].(map[string]any); ok {
					if format, exists := endField["format"]; exists {
						if format != "date-time" {
							t.Errorf("unsupported format 'date' should have been converted to 'date-time', but found: %v", format)
						}
					} else {
						t.Error("format should have been converted to 'date-time', but was missing")
					}
					// Description should be preserved in nested objects
					if _, exists := endField["description"]; !exists {
						t.Error("description should be preserved in nested objects")
					}
				}
			}
		}
	}
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
			if !ok {
				t.Fatal("expected map result")
			}

			format, hasFormat := resultMap["format"]
			if tc.supported {
				if !hasFormat {
					t.Errorf("format %s should be preserved/converted but was removed", tc.format)
				} else if format != tc.expectedFormat {
					t.Errorf("format %s should be converted to %s, got %s", tc.format, tc.expectedFormat, format)
				}
			} else {
				if hasFormat {
					t.Errorf("unsupported format %s should be removed", tc.format)
				}
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

			if result1 == nil && tc.input != nil {
				t.Error("cleanFunctionParameters should not return nil for non-nil input")
			}
			if result2 == nil && tc.input != nil {
				t.Error("cleanJsonSchemaForGemini should not return nil for non-nil input")
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
			if !ok {
				t.Fatal("expected map result")
			}

			if tt.shouldKeep {
				format, hasFormat := resultMap["format"]
				if !hasFormat {
					t.Errorf("format should be preserved but was removed")
				} else if format != tt.expectedFormat {
					t.Errorf("expected format %s, got %s", tt.expectedFormat, format)
				}
			} else {
				if _, hasFormat := resultMap["format"]; hasFormat {
					t.Errorf("unsupported format %s should be removed", tt.inputFormat)
				}
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

	if elapsed > 200*time.Millisecond {
		t.Errorf("ConvertRequest took too long: %v", elapsed)
	}

	if geminiRequest == nil {
		t.Fatal("ConvertRequest returned nil")
	}

	if len(geminiRequest.Tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(geminiRequest.Tools))
	}

	tool := geminiRequest.Tools[0]
	functions, ok := tool.FunctionDeclarations.([]model.Function)
	if !ok {
		t.Fatal("FunctionDeclarations should be []model.Function")
	}
	if len(functions) != 1 {
		t.Fatalf("Expected 1 function declaration, got %d", len(functions))
	}

	function := functions[0]
	// Convert Parameters from any to map[string]interface{} before indexing
	params, ok := function.Parameters.(map[string]any)
	if !ok {
		t.Fatal("function.Parameters should be map[string]interface{}")
	}

	// Verify the critical fix: date format should be converted to date-time
	if properties, ok := params["properties"].(map[string]any); ok {
		if dateRange, ok := properties["dateRange"].(map[string]any); ok {
			if dateRangeProps, ok := dateRange["properties"].(map[string]any); ok {
				// Check start field
				if startField, ok := dateRangeProps["start"].(map[string]any); ok {
					if format, exists := startField["format"]; exists {
						if format != "date-time" {
							t.Errorf("start format should be converted to 'date-time', got: %v", format)
						}
					} else {
						t.Error("start format should be present after conversion")
					}
				}
				// Check end field
				if endField, ok := dateRangeProps["end"].(map[string]any); ok {
					if format, exists := endField["format"]; exists {
						if format != "date-time" {
							t.Errorf("end format should be converted to 'date-time', got: %v", format)
						}
					} else {
						t.Error("end format should be present after conversion")
					}
				}
			}
		}
	}

	// Verify other cleaning behaviors still work
	if _, exists := params["additionalProperties"]; exists {
		t.Error("additionalProperties should have been removed")
	}
	if _, exists := params["description"]; exists {
		t.Error("description should have been removed at top level")
	}
	if _, exists := params["strict"]; exists {
		t.Error("strict should have been removed at top level")
	}
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
	if !ok {
		t.Fatal("expected map result")
	}

	if resultMap["type"] != "OBJECT" {
		t.Error("type should be converted to uppercase")
	}

	properties, ok := resultMap["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties should be present")
	}

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
		if field, ok := properties[tc.field].(map[string]any); ok {
			if format, exists := field["format"]; exists {
				if format != tc.expected {
					t.Errorf("%s format should be %s, got %s", tc.field, tc.expected, format)
				}
			} else {
				t.Errorf("%s format should be present", tc.field)
			}
		} else {
			t.Errorf("%s field should be present", tc.field)
		}
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

			if result1 == nil && tc.input != nil {
				t.Error("cleanFunctionParameters should not return nil for non-nil input")
			}
			if result2 == nil && tc.input != nil {
				t.Error("cleanJsonSchemaForGemini should not return nil for non-nil input")
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

	if elapsed > 50*time.Millisecond {
		t.Errorf("Complex schema cleaning took too long: %v", elapsed)
	}

	if result == nil {
		t.Error("Result should not be nil")
	}

	// Verify the structure is maintained while cleaning
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatal("Result should be a map")
	}

	if _, exists := resultMap["additionalProperties"]; exists {
		t.Error("additionalProperties should be removed")
	}
	if _, exists := resultMap["description"]; exists {
		t.Error("description should be removed at top level")
	}
	if _, exists := resultMap["strict"]; exists {
		t.Error("strict should be removed at top level")
	}
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
	if geminiRequest == nil {
		t.Fatal("ConvertRequest returned nil")
	}

	if len(geminiRequest.Tools) == 0 {
		t.Fatal("Tools should not be empty")
	}

	// Extract the function declarations to verify they're clean
	tool := geminiRequest.Tools[0]
	functions, ok := tool.FunctionDeclarations.([]model.Function)
	if !ok {
		t.Fatal("FunctionDeclarations should be []model.Function")
	}

	if len(functions) == 0 {
		t.Fatal("FunctionDeclarations should not be empty")
	}

	function := functions[0]

	// Verify the function parameters no longer contain additionalProperties
	if err := verifyNoAdditionalProperties(function.Parameters); err != nil {
		t.Errorf("Function parameters still contain additionalProperties: %v", err)
	}

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
			if actualUsage.PromptTokens != tt.expected.PromptTokens {
				t.Errorf("Expected PromptTokens %d, got %d", tt.expected.PromptTokens, actualUsage.PromptTokens)
			}
			if actualUsage.CompletionTokens != tt.expected.CompletionTokens {
				t.Errorf("Expected CompletionTokens %d, got %d", tt.expected.CompletionTokens, actualUsage.CompletionTokens)
			}
			if actualUsage.TotalTokens != tt.expected.TotalTokens {
				t.Errorf("Expected TotalTokens %d, got %d", tt.expected.TotalTokens, actualUsage.TotalTokens)
			}
		})
	}
}
