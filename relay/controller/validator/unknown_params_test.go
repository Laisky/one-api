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
		"n", "logit_bias", "user", "seed", "verbosity",
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

func TestValidateUnknownParameters_GPT5Verbosity(t *testing.T) {
	// Test with GPT-5 verbosity parameter (should be valid - recognized as known parameter)
	// https://cookbook.openai.com/examples/gpt-5/gpt-5_new_params_and_tools
	testCases := []struct {
		name string
		json string
	}{
		{
			name: "verbosity=low",
			json: `{
				"model": "gpt-5",
				"messages": [{"role": "user", "content": "Hello"}],
				"verbosity": "low",
				"reasoning_effort": "minimal"
			}`,
		},
		{
			name: "verbosity=medium",
			json: `{
				"model": "gpt-5",
				"messages": [{"role": "user", "content": "Hello"}],
				"verbosity": "medium"
			}`,
		},
		{
			name: "verbosity=high",
			json: `{
				"model": "gpt-5-mini",
				"messages": [{"role": "user", "content": "Hello"}],
				"verbosity": "high",
				"stream": true,
				"stream_options": {"include_usage": true}
			}`,
		},
		{
			name: "verbosity with reasoning_effort minimal",
			json: `{
				"model": "gpt-5",
				"messages": [{"role": "system", "content": "You are an expert."}, {"role": "user", "content": "Explain something"}],
				"reasoning_effort": "minimal",
				"verbosity": "medium"
			}`,
		},
		{
			// This is the exact user's request that triggered the bug
			name: "real user request with verbosity and reasoning_effort",
			json: `{"messages":[{"content":"你现在是一名资深的软件工程师，你熟悉多种编程语言和开发框架，对软件开发的生命周期有深入的理解。你擅长解决技术问题，并具有优秀的逻辑思维能力。请在这个角色下为我解答以下问题。","role":"system"},{"content":"详细帮我介绍一下 go work 机制，我是否可以使用这个来做本地开发一些 replace 替换，而不用每次针对项目中进行手动更改替换到\n","role":"user"}],"model":"gpt-5","reasoning_effort":"minimal","stream":true,"stream_options":{"include_usage":true},"verbosity":"medium"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.ValidateUnknownParameters([]byte(tc.json))
			if err != nil {
				t.Errorf("Expected no error for GPT-5 verbosity parameter, got: %v", err)
			}
		})
	}
}

// TestValidateUnknownParameters_UnknownParamsAllowed tests that unknown parameters
// are silently ignored instead of causing errors. This ensures forward compatibility
// when upstream services add new parameters before one-api has been updated.
func TestValidateUnknownParameters_UnknownParamsAllowed(t *testing.T) {
	testCases := []struct {
		name string
		json string
	}{
		{
			name: "single unknown parameter",
			json: `{
				"model": "gpt-3.5-turbo",
				"messages": [{"role": "user", "content": "Hello"}],
				"temperature": 0.7,
				"unknown_param": "value"
			}`,
		},
		{
			name: "multiple unknown parameters",
			json: `{
				"model": "gpt-3.5-turbo",
				"messages": [{"role": "user", "content": "Hello"}],
				"unknown_param1": "value1",
				"unknown_param2": "value2",
				"unknown_param3": "value3"
			}`,
		},
		{
			name: "mixed known and unknown parameters",
			json: `{
				"model": "gpt-3.5-turbo",
				"messages": [{"role": "user", "content": "Hello"}],
				"temperature": 0.7,
				"max_tokens": 100,
				"unknown_param": "value",
				"another_unknown": "value2"
			}`,
		},
		{
			name: "complex nested structures with unknown top-level",
			json: `{
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
				"unknown_top_level": "should be ignored too"
			}`,
		},
		{
			name: "common typo: max_token instead of max_tokens",
			json: `{"model": "gpt-3.5-turbo", "messages": [{"role": "user", "content": "Hello"}], "max_token": 100}`,
		},
		{
			name: "common typo: temprature instead of temperature",
			json: `{"model": "gpt-3.5-turbo", "messages": [{"role": "user", "content": "Hello"}], "temprature": 0.7}`,
		},
		{
			name: "common typo: stream_option instead of stream_options",
			json: `{"model": "gpt-3.5-turbo", "messages": [{"role": "user", "content": "Hello"}], "stream_option": {"include_usage": true}}`,
		},
		{
			name: "future unknown parameter from upstream",
			json: `{
				"model": "gpt-6",
				"messages": [{"role": "user", "content": "Hello"}],
				"future_param": "some_value",
				"another_future_param": {"nested": "value"}
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.ValidateUnknownParameters([]byte(tc.json))
			if err != nil {
				t.Errorf("Expected no error (unknown params should be silently ignored), got: %v", err)
			}
		})
	}
}

func TestValidateUnknownParameters_InvalidJSON(t *testing.T) {
	// Test with invalid JSON - should not error (let normal validation handle it)
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

// TestFindUnknownParametersInternal tests the internal unknown param detection
// by using the GetKnownParameters function to verify what's known vs unknown.
func TestFindUnknownParametersInternal(t *testing.T) {
	knownParams := validator.GetKnownParameters()

	// These should be known (not trigger warnings in logs)
	expectedKnown := []string{
		"model", "messages", "temperature", "max_tokens", "stream",
		"verbosity", "reasoning_effort", "tools", "tool_choice",
	}
	for _, param := range expectedKnown {
		if !knownParams[param] {
			t.Errorf("Expected '%s' to be a known parameter", param)
		}
	}

	// These should be unknown (would trigger DEBUG log warnings, but not errors)
	expectedUnknown := []string{
		"unknown_param", "future_param", "typo_temperture",
	}
	for _, param := range expectedUnknown {
		if knownParams[param] {
			t.Errorf("Expected '%s' to be an unknown parameter", param)
		}
	}
}
