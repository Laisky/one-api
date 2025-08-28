package utils

import (
	"testing"

	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

func TestGetProviderCapabilities(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		expectedCaps ProviderCapabilities
	}{
		{
			name:         "Claude provider capabilities",
			providerName: "claude",
			expectedCaps: ProviderCapabilities{
				SupportsTools:          true,
				SupportsThinking:       true,
				SupportsResponseFormat: true,
				SupportsFunctions:      false,
				SupportsLogprobs:       false,
			},
		},
		{
			name:         "DeepSeek provider capabilities",
			providerName: "deepseek",
			expectedCaps: ProviderCapabilities{
				SupportsReasoningEffort: true,
				SupportsTools:           false,
				SupportsThinking:        false,
				SupportsLogprobs:        false,
			},
		},
		{
			name:         "Mistral provider capabilities",
			providerName: "mistral",
			expectedCaps: ProviderCapabilities{
				SupportsTools:     true,
				SupportsFunctions: false,
				SupportsLogprobs:  false,
				SupportsThinking:  false,
			},
		},
		{
			name:         "Nova provider capabilities",
			providerName: "nova",
			expectedCaps: ProviderCapabilities{
				SupportsTools:      true,
				SupportsModalities: true,
				SupportsAudio:      false,
				SupportsLogprobs:   false,
			},
		},
		{
			name:         "Titan provider capabilities",
			providerName: "titan",
			expectedCaps: ProviderCapabilities{
				SupportsTools:    false,
				SupportsLogprobs: false,
				SupportsThinking: false,
			},
		},
		{
			name:         "Unknown provider defaults to minimal capabilities",
			providerName: "unknown",
			expectedCaps: ProviderCapabilities{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := GetProviderCapabilities(tt.providerName)

			if caps.SupportsTools != tt.expectedCaps.SupportsTools {
				t.Errorf("Expected SupportsTools=%v, got %v", tt.expectedCaps.SupportsTools, caps.SupportsTools)
			}
			if caps.SupportsFunctions != tt.expectedCaps.SupportsFunctions {
				t.Errorf("Expected SupportsFunctions=%v, got %v", tt.expectedCaps.SupportsFunctions, caps.SupportsFunctions)
			}
			if caps.SupportsLogprobs != tt.expectedCaps.SupportsLogprobs {
				t.Errorf("Expected SupportsLogprobs=%v, got %v", tt.expectedCaps.SupportsLogprobs, caps.SupportsLogprobs)
			}
			if caps.SupportsThinking != tt.expectedCaps.SupportsThinking {
				t.Errorf("Expected SupportsThinking=%v, got %v", tt.expectedCaps.SupportsThinking, caps.SupportsThinking)
			}
			if caps.SupportsModalities != tt.expectedCaps.SupportsModalities {
				t.Errorf("Expected SupportsModalities=%v, got %v", tt.expectedCaps.SupportsModalities, caps.SupportsModalities)
			}
			if caps.SupportsReasoningEffort != tt.expectedCaps.SupportsReasoningEffort {
				t.Errorf("Expected SupportsReasoningEffort=%v, got %v", tt.expectedCaps.SupportsReasoningEffort, caps.SupportsReasoningEffort)
			}
			if caps.SupportsResponseFormat != tt.expectedCaps.SupportsResponseFormat {
				t.Errorf("Expected SupportsResponseFormat=%v, got %v", tt.expectedCaps.SupportsResponseFormat, caps.SupportsResponseFormat)
			}
		})
	}
}

func TestValidateUnsupportedParameters_NoUnsupportedParams(t *testing.T) {
	// Test with a request that has no unsupported parameters for Claude
	request := &relaymodel.GeneralOpenAIRequest{
		Messages: []relaymodel.Message{
			{Role: "user", Content: "Hello"},
		},
		Temperature: &[]float64{0.7}[0],
		MaxTokens:   100,
		Tools: []relaymodel.Tool{
			{
				Type: "function",
				Function: &relaymodel.Function{
					Name:        "test_function",
					Description: "A test function",
				},
			},
		},
	}

	err := ValidateUnsupportedParameters(request, "claude")
	if err != nil {
		t.Errorf("Expected no error for supported parameters, got: %v", err.Error.Message)
	}
}

func TestValidateUnsupportedParameters_ToolsNotSupported(t *testing.T) {
	// Test with tools on a provider that doesn't support them
	request := &relaymodel.GeneralOpenAIRequest{
		Messages: []relaymodel.Message{
			{Role: "user", Content: "Hello"},
		},
		Tools: []relaymodel.Tool{
			{
				Type: "function",
				Function: &relaymodel.Function{
					Name: "test_function",
				},
			},
		},
	}

	err := ValidateUnsupportedParameters(request, "deepseek")
	if err == nil {
		t.Error("Expected error for unsupported tools parameter")
		return
	}

	if err.StatusCode != 400 {
		t.Errorf("Expected status code 400, got %d", err.StatusCode)
	}

	if err.Error.Code != "unsupported_parameter" {
		t.Errorf("Expected error code 'unsupported_parameter', got '%s'", err.Error.Code)
	}

	expectedMessage := "Unsupported parameter 'tools': Tool calling is not supported by this provider"
	if err.Error.Message != expectedMessage {
		t.Errorf("Expected message '%s', got '%s'", expectedMessage, err.Error.Message)
	}
}

func TestValidateUnsupportedParameters_FunctionsNotSupported(t *testing.T) {
	// Test with deprecated functions parameter
	request := &relaymodel.GeneralOpenAIRequest{
		Messages: []relaymodel.Message{
			{Role: "user", Content: "Hello"},
		},
		Functions: []relaymodel.Function{
			{
				Name:        "test_function",
				Description: "A test function",
			},
		},
	}

	err := ValidateUnsupportedParameters(request, "mistral")
	if err == nil {
		t.Error("Expected error for unsupported functions parameter")
		return
	}

	expectedMessage := "Unsupported parameter 'functions': Functions (deprecated OpenAI feature) are not supported by this provider. Use 'tools' instead"
	if err.Error.Message != expectedMessage {
		t.Errorf("Expected message '%s', got '%s'", expectedMessage, err.Error.Message)
	}
}

func TestValidateUnsupportedParameters_LogprobsNotSupported(t *testing.T) {
	// Test with logprobs parameter
	logprobs := true
	topLogprobs := 5
	request := &relaymodel.GeneralOpenAIRequest{
		Messages: []relaymodel.Message{
			{Role: "user", Content: "Hello"},
		},
		Logprobs:    &logprobs,
		TopLogprobs: &topLogprobs,
	}

	err := ValidateUnsupportedParameters(request, "claude")
	if err == nil {
		t.Error("Expected error for unsupported logprobs parameters")
		return
	}

	// Should contain multiple unsupported parameters
	if !containsString(err.Error.Message, "logprobs") {
		t.Error("Expected error message to contain 'logprobs'")
	}
	if !containsString(err.Error.Message, "top_logprobs") {
		t.Error("Expected error message to contain 'top_logprobs'")
	}
}

func TestValidateUnsupportedParameters_MultipleUnsupportedParams(t *testing.T) {
	// Test with multiple unsupported parameters
	logprobs := true
	frequencyPenalty := 0.5
	presencePenalty := 0.3
	request := &relaymodel.GeneralOpenAIRequest{
		Messages: []relaymodel.Message{
			{Role: "user", Content: "Hello"},
		},
		Logprobs:         &logprobs,
		FrequencyPenalty: &frequencyPenalty,
		PresencePenalty:  &presencePenalty,
		Tools: []relaymodel.Tool{
			{Type: "function", Function: &relaymodel.Function{Name: "test"}},
		},
	}

	err := ValidateUnsupportedParameters(request, "titan") // Titan supports none of these
	if err == nil {
		t.Error("Expected error for multiple unsupported parameters")
		return
	}

	// Check that the error message contains information about multiple parameters
	if !containsString(err.Error.Message, "Unsupported parameters for provider 'titan'") {
		t.Error("Expected error message to indicate multiple unsupported parameters")
	}
}

func TestValidateUnsupportedParameters_ReasoningEffortSupported(t *testing.T) {
	// Test that reasoning_effort is supported for deepseek but not for others
	reasoningEffort := "high"
	request := &relaymodel.GeneralOpenAIRequest{
		Messages: []relaymodel.Message{
			{Role: "user", Content: "Hello"},
		},
		ReasoningEffort: &reasoningEffort,
	}

	// Should work for deepseek
	err := ValidateUnsupportedParameters(request, "deepseek")
	if err != nil {
		t.Errorf("Expected no error for deepseek reasoning_effort, got: %v", err.Error.Message)
	}

	// Should fail for claude
	err = ValidateUnsupportedParameters(request, "claude")
	if err == nil {
		t.Error("Expected error for claude reasoning_effort")
		return
	}

	expectedMessage := "Unsupported parameter 'reasoning_effort': Reasoning effort is not supported by this provider"
	if err.Error.Message != expectedMessage {
		t.Errorf("Expected message '%s', got '%s'", expectedMessage, err.Error.Message)
	}
}

func TestValidateUnsupportedParameters_ModalitiesSupported(t *testing.T) {
	// Test that modalities are supported for nova but not for others
	request := &relaymodel.GeneralOpenAIRequest{
		Messages: []relaymodel.Message{
			{Role: "user", Content: "Hello"},
		},
		Modalities: []string{"text", "image"},
	}

	// Should work for nova
	err := ValidateUnsupportedParameters(request, "nova")
	if err != nil {
		t.Errorf("Expected no error for nova modalities, got: %v", err.Error.Message)
	}

	// Should fail for titan
	err = ValidateUnsupportedParameters(request, "titan")
	if err == nil {
		t.Error("Expected error for titan modalities")
		return
	}

	expectedMessage := "Unsupported parameter 'modalities': Modalities are not supported by this provider"
	if err.Error.Message != expectedMessage {
		t.Errorf("Expected message '%s', got '%s'", expectedMessage, err.Error.Message)
	}
}

func TestValidateUnsupportedParameters_ThinkingSupported(t *testing.T) {
	// Test that thinking is supported for claude but not for others
	request := &relaymodel.GeneralOpenAIRequest{
		Messages: []relaymodel.Message{
			{Role: "user", Content: "Hello"},
		},
		Thinking: &relaymodel.Thinking{
			Type:         "extended",
			BudgetTokens: 2048,
		},
	}

	// Should work for claude
	err := ValidateUnsupportedParameters(request, "claude")
	if err != nil {
		t.Errorf("Expected no error for claude thinking, got: %v", err.Error.Message)
	}

	// Should fail for deepseek
	err = ValidateUnsupportedParameters(request, "deepseek")
	if err == nil {
		t.Error("Expected error for deepseek thinking")
		return
	}

	expectedMessage := "Unsupported parameter 'thinking': Extended thinking is not supported by this provider"
	if err.Error.Message != expectedMessage {
		t.Errorf("Expected message '%s', got '%s'", expectedMessage, err.Error.Message)
	}
}

func TestValidateUnsupportedParameters_EmptyRequest(t *testing.T) {
	// Test with empty request
	request := &relaymodel.GeneralOpenAIRequest{}

	err := ValidateUnsupportedParameters(request, "claude")
	if err != nil {
		t.Errorf("Expected no error for empty request, got: %v", err.Error.Message)
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || containsString(s[1:], substr) || (len(s) > 0 && s[:len(substr)] == substr))
}

// Simple implementation since we can't import strings package in test
func containsString2(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestContainsStringHelper(t *testing.T) {
	// Test our helper function
	if !containsString2("hello world", "world") {
		t.Error("containsString2 should find 'world' in 'hello world'")
	}
	if containsString2("hello", "world") {
		t.Error("containsString2 should not find 'world' in 'hello'")
	}
}
