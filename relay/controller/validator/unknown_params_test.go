package validator_test

import (
	"testing"

	"github.com/songquanpeng/one-api/relay/controller/validator"
)

func TestGetKnownParameters(t *testing.T) {
	knownParams := validator.GetKnownParameters()

	// Test that some key parameters are present
	expectedParams := []string{
		"messages", "model", "temperature", "max_tokens", "tools", "tool_choice",
		"functions", "function_call", "logprobs", "top_logprobs", "frequency_penalty",
		"presence_penalty", "response_format", "reasoning_effort", "modalities",
		"audio", "web_search_options", "thinking", "service_tier", "prediction",
		"max_completion_tokens", "stream", "stream_options", "stop", "top_p",
		"n", "logit_bias", "user", "seed",
	}

	for _, param := range expectedParams {
		if !knownParams[param] {
			t.Errorf("Expected parameter '%s' to be in known parameters", param)
		}
	}

	// Verify we have a reasonable number of parameters (should be 30+ from GeneralOpenAIRequest)
	if len(knownParams) < 30 {
		t.Errorf("Expected at least 30 known parameters, got %d", len(knownParams))
	}
}

func TestValidateUnknownParameters_NoUnknownParams(t *testing.T) {
	// Test with valid JSON containing only known parameters
	validJSON := `{
		"model": "gpt-3.5-turbo",
		"messages": [{"role": "user", "content": "Hello"}],
		"temperature": 0.7,
		"max_tokens": 100
	}`

	err := validator.ValidateUnknownParameters([]byte(validJSON))
	if err != nil {
		t.Errorf("Expected no error for valid parameters, got: %v", err)
	}
}

func TestValidateUnknownParameters_SingleUnknownParam(t *testing.T) {
	// Test with one unknown parameter
	invalidJSON := `{
		"model": "gpt-3.5-turbo",
		"messages": [{"role": "user", "content": "Hello"}],
		"temperature": 0.7,
		"unknown_param": "value"
	}`

	err := validator.ValidateUnknownParameters([]byte(invalidJSON))
	if err == nil {
		t.Error("Expected error for unknown parameter")
		return
	}

	expectedMessage := "unknown parameter 'unknown_param' is not supported"
	if err.Error() != expectedMessage {
		t.Errorf("Expected message '%s', got '%s'", expectedMessage, err.Error())
	}
}

func TestValidateUnknownParameters_MultipleUnknownParams(t *testing.T) {
	// Test with multiple unknown parameters
	invalidJSON := `{
		"model": "gpt-3.5-turbo",
		"messages": [{"role": "user", "content": "Hello"}],
		"unknown_param1": "value1",
		"unknown_param2": "value2",
		"unknown_param3": "value3"
	}`

	err := validator.ValidateUnknownParameters([]byte(invalidJSON))
	if err == nil {
		t.Error("Expected error for multiple unknown parameters")
		return
	}

	// Should contain indication of multiple parameters
	errorMessage := err.Error()
	if !containsSubstring(errorMessage, "unknown parameters are not supported") {
		t.Errorf("Expected error message to indicate multiple unknown parameters, got: %s", errorMessage)
	}

	// Should contain all unknown parameter names
	for _, param := range []string{"unknown_param1", "unknown_param2", "unknown_param3"} {
		if !containsSubstring(errorMessage, param) {
			t.Errorf("Expected error message to contain '%s', got: %s", param, errorMessage)
		}
	}
}

func TestValidateUnknownParameters_MixedKnownAndUnknown(t *testing.T) {
	// Test with mix of known and unknown parameters
	invalidJSON := `{
		"model": "gpt-3.5-turbo",
		"messages": [{"role": "user", "content": "Hello"}],
		"temperature": 0.7,
		"max_tokens": 100,
		"unknown_param": "value",
		"another_unknown": "value2"
	}`

	err := validator.ValidateUnknownParameters([]byte(invalidJSON))
	if err == nil {
		t.Error("Expected error for unknown parameters")
		return
	}

	// Should only mention unknown parameters
	errorMessage := err.Error()
	if !containsSubstring(errorMessage, "unknown parameters are not supported") {
		t.Errorf("Expected error message to indicate multiple unknown parameters, got: %s", errorMessage)
	}

	// Should contain unknown parameter names but not known ones
	if !containsSubstring(errorMessage, "unknown_param") {
		t.Error("Expected error message to contain 'unknown_param'")
	}
	if !containsSubstring(errorMessage, "another_unknown") {
		t.Error("Expected error message to contain 'another_unknown'")
	}

	// Should NOT contain known parameter names
	if containsSubstring(errorMessage, "model") {
		t.Error("Error message should not contain known parameter 'model'")
	}
	if containsSubstring(errorMessage, "temperature") {
		t.Error("Error message should not contain known parameter 'temperature'")
	}
}

func TestValidateUnknownParameters_InvalidJSON(t *testing.T) {
	// Test with invalid JSON
	invalidJSON := `{invalid json`

	err := validator.ValidateUnknownParameters([]byte(invalidJSON))
	if err != nil {
		t.Errorf("Expected no error for invalid JSON (should be handled by normal validation), got: %v", err)
	}
}

func TestValidateUnknownParameters_EmptyJSON(t *testing.T) {
	// Test with empty JSON object
	emptyJSON := `{}`

	err := validator.ValidateUnknownParameters([]byte(emptyJSON))
	if err != nil {
		t.Errorf("Expected no error for empty JSON, got: %v", err)
	}
}

func TestValidateUnknownParameters_ComplexNestedStructures(t *testing.T) {
	// Test with complex nested structures (should only validate top-level parameters)
	complexJSON := `{
		"model": "gpt-3.5-turbo",
		"messages": [
			{
				"role": "user", 
				"content": "Hello",
				"unknown_nested_param": "should be ignored"
			}
		],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "test",
					"unknown_function_param": "should be ignored"
				}
			}
		],
		"unknown_top_level": "should be caught"
	}`

	err := validator.ValidateUnknownParameters([]byte(complexJSON))
	if err == nil {
		t.Error("Expected error for unknown top-level parameter")
		return
	}

	expectedMessage := "unknown parameter 'unknown_top_level' is not supported"
	if err.Error() != expectedMessage {
		t.Errorf("Expected message '%s', got '%s'", expectedMessage, err.Error())
	}
}

func TestValidateUnknownParameters_CommonTypos(t *testing.T) {
	// Test with common parameter typos
	testCases := []struct {
		name         string
		json         string
		unknownParam string
	}{
		{
			name:         "max_token instead of max_tokens",
			json:         `{"model": "gpt-3.5-turbo", "messages": [{"role": "user", "content": "Hello"}], "max_token": 100}`,
			unknownParam: "max_token",
		},
		{
			name:         "temprature instead of temperature",
			json:         `{"model": "gpt-3.5-turbo", "messages": [{"role": "user", "content": "Hello"}], "temprature": 0.7}`,
			unknownParam: "temprature",
		},
		{
			name:         "stream_option instead of stream_options",
			json:         `{"model": "gpt-3.5-turbo", "messages": [{"role": "user", "content": "Hello"}], "stream_option": {"include_usage": true}}`,
			unknownParam: "stream_option",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.ValidateUnknownParameters([]byte(tc.json))
			if err == nil {
				t.Error("Expected error for typo in parameter name")
				return
			}

			if !containsSubstring(err.Error(), tc.unknownParam) {
				t.Errorf("Expected error message to contain '%s', got: %s", tc.unknownParam, err.Error())
			}
		})
	}
}

// Helper function to check if a string contains a substring
func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
