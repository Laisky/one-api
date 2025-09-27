package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultServerOptions(t *testing.T) {
	opts := DefaultServerOptions()

	assert.NotNil(t, opts, "DefaultServerOptions should not return nil")
	assert.Equal(t, "one-api-official-mcp", opts.Name, "Default name should be set")
	assert.Equal(t, "1.0.0", opts.Version, "Default version should be set")
	assert.True(t, opts.EnableInstructions, "Instructions should be enabled by default")
	assert.NotNil(t, opts.Instructions, "Instructions config should not be nil")
	assert.Equal(t, GeneralInstructions, opts.Instructions.Type, "Default instruction type should be general")
	assert.True(t, opts.Instructions.EnableFallback, "Fallback should be enabled by default")
	assert.NotNil(t, opts.CustomTemplateData, "Custom template data should be initialized")

	t.Logf("✓ Default server options configured correctly")
}

func TestServerOptionsBuilder(t *testing.T) {
	opts := DefaultServerOptions().
		WithName("test-server").
		WithVersion("2.0.0").
		WithInstructionType(ToolUsageInstructions).
		WithBaseURL("https://test.example.com").
		WithCustomTemplateData("test_key", "test_value")

	assert.Equal(t, "test-server", opts.Name, "Name should be set")
	assert.Equal(t, "2.0.0", opts.Version, "Version should be set")
	assert.Equal(t, ToolUsageInstructions, opts.Instructions.Type, "Instruction type should be set")
	assert.Equal(t, "https://test.example.com", opts.BaseURL, "Base URL should be set")
	assert.Equal(t, "test_value", opts.CustomTemplateData["test_key"], "Custom template data should be set")
	assert.True(t, opts.EnableInstructions, "Instructions should remain enabled")

	t.Logf("✓ Server options builder pattern works correctly")
}

func TestServerOptionsValidation(t *testing.T) {
	testCases := []struct {
		name        string
		setupFunc   func() *ServerOptions
		expectError bool
	}{
		{
			name: "valid_options",
			setupFunc: func() *ServerOptions {
				return DefaultServerOptions()
			},
			expectError: false,
		},
		{
			name: "empty_name",
			setupFunc: func() *ServerOptions {
				opts := DefaultServerOptions()
				opts.Name = ""
				return opts
			},
			expectError: true,
		},
		{
			name: "empty_version",
			setupFunc: func() *ServerOptions {
				opts := DefaultServerOptions()
				opts.Version = ""
				return opts
			},
			expectError: true,
		},
		{
			name: "instructions_enabled_but_no_type_or_custom",
			setupFunc: func() *ServerOptions {
				opts := DefaultServerOptions()
				opts.EnableInstructions = true
				opts.Instructions.Type = ""
				opts.Instructions.CustomInstructions = ""
				return opts
			},
			expectError: true,
		},
		{
			name: "instructions_disabled",
			setupFunc: func() *ServerOptions {
				opts := DefaultServerOptions()
				opts.EnableInstructions = false
				return opts
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := tc.setupFunc()
			err := opts.Validate()

			if tc.expectError {
				assert.Error(t, err, "Should return validation error")
			} else {
				assert.NoError(t, err, "Should not return validation error")
			}

			t.Logf("✓ Validation test '%s' passed", tc.name)
		})
	}
}

func TestCustomInstructions(t *testing.T) {
	customText := "This is a custom instruction"
	opts := DefaultServerOptions().
		WithCustomInstructions(customText)

	assert.Equal(t, customText, opts.Instructions.CustomInstructions, "Custom instructions should be set")
	assert.True(t, opts.EnableInstructions, "Instructions should be enabled")
	assert.NotNil(t, opts.Instructions, "Instructions config should not be nil")

	t.Logf("✓ Custom instructions configuration works correctly")
}

func TestInstructionConfig(t *testing.T) {
	config := &InstructionConfig{
		Type:               BestPracticesInstructions,
		CustomInstructions: "Custom text",
		TemplateData:       map[string]any{"key": "value"},
		EnableFallback:     false,
	}

	opts := DefaultServerOptions().WithInstructions(config)

	assert.Equal(t, config, opts.Instructions, "Instruction config should be set")
	assert.Equal(t, BestPracticesInstructions, opts.Instructions.Type, "Instruction type should be set")
	assert.Equal(t, "Custom text", opts.Instructions.CustomInstructions, "Custom instructions should be set")
	assert.Equal(t, "value", opts.Instructions.TemplateData["key"], "Template data should be set")
	assert.False(t, opts.Instructions.EnableFallback, "Fallback should be disabled")

	t.Logf("✓ Instruction config assignment works correctly")
}

func TestGetEffectiveBaseURL(t *testing.T) {
	testCases := []struct {
		name        string
		optionsURL  string
		expectedURL string
	}{
		{
			name:        "with_custom_base_url",
			optionsURL:  "https://custom.example.com",
			expectedURL: "https://custom.example.com",
		},
		{
			name:        "without_custom_base_url",
			optionsURL:  "",
			expectedURL: "", // Will use getBaseURL() which depends on config
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := DefaultServerOptions()
			if tc.optionsURL != "" {
				opts.WithBaseURL(tc.optionsURL)
			}

			effectiveURL := opts.GetEffectiveBaseURL()

			if tc.expectedURL != "" {
				assert.Equal(t, tc.expectedURL, effectiveURL, "Should return custom base URL")
			} else {
				// Should return result from getBaseURL()
				assert.NotEmpty(t, effectiveURL, "Should return some base URL")
			}

			t.Logf("✓ Effective base URL test '%s' passed: %s", tc.name, effectiveURL)
		})
	}
}

func TestDisableInstructions(t *testing.T) {
	opts := DefaultServerOptions().DisableInstructions()

	assert.False(t, opts.EnableInstructions, "Instructions should be disabled")

	t.Logf("✓ Disable instructions works correctly")
}

func TestInstructionTypes(t *testing.T) {
	types := []InstructionType{
		GeneralInstructions,
		ToolUsageInstructions,
		APIEndpointInstructions,
		ErrorHandlingInstructions,
		BestPracticesInstructions,
	}

	for _, instructionType := range types {
		t.Run(string(instructionType), func(t *testing.T) {
			opts := DefaultServerOptions().WithInstructionType(instructionType)

			assert.Equal(t, instructionType, opts.Instructions.Type, "Instruction type should be set correctly")
			assert.True(t, opts.EnableInstructions, "Instructions should be enabled")

			t.Logf("✓ Instruction type '%s' configured correctly", instructionType)
		})
	}
}

func TestChainedBuilderPattern(t *testing.T) {
	opts := DefaultServerOptions().
		WithName("chained-server").
		WithVersion("3.0.0").
		WithInstructionType(ErrorHandlingInstructions).
		WithBaseURL("https://chained.example.com").
		WithCustomTemplateData("chain1", "value1").
		WithCustomTemplateData("chain2", "value2").
		WithCustomInstructions("Chained custom instructions")

	// Verify all chained settings
	assert.Equal(t, "chained-server", opts.Name)
	assert.Equal(t, "3.0.0", opts.Version)
	assert.Equal(t, ErrorHandlingInstructions, opts.Instructions.Type)
	assert.Equal(t, "https://chained.example.com", opts.BaseURL)
	assert.Equal(t, "value1", opts.CustomTemplateData["chain1"])
	assert.Equal(t, "value2", opts.CustomTemplateData["chain2"])
	assert.Equal(t, "Chained custom instructions", opts.Instructions.CustomInstructions)

	// Validate the final configuration
	err := opts.Validate()
	assert.NoError(t, err, "Chained configuration should be valid")

	t.Logf("✓ Chained builder pattern works correctly")
}
