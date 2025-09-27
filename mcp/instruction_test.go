package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewServerWithOptions(t *testing.T) {
	opts := DefaultServerOptions().
		WithName("test-mcp-server").
		WithVersion("1.5.0").
		WithInstructionType(ToolUsageInstructions)

	server := NewServerWithOptions(opts)

	assert.NotNil(t, server, "Server should not be nil")
	assert.NotNil(t, server.server, "Internal MCP server should not be nil")
	assert.Equal(t, opts, server.options, "Server options should be stored")

	t.Logf("✓ NewServerWithOptions creates server correctly")
}

func TestNewServerWithInvalidOptions(t *testing.T) {
	// Create invalid options (empty name)
	opts := DefaultServerOptions()
	opts.Name = ""

	server := NewServerWithOptions(opts)

	// Should fall back to default options
	assert.NotNil(t, server, "Server should not be nil")
	assert.Equal(t, "one-api-official-mcp", server.options.Name, "Should use default name")

	t.Logf("✓ NewServerWithOptions handles invalid options correctly")
}

func TestServerGetEffectiveBaseURL(t *testing.T) {
	testCases := []struct {
		name        string
		optionsURL  string
		expectedURL string
	}{
		{
			name:        "with_custom_base_url",
			optionsURL:  "https://custom.test.com",
			expectedURL: "https://custom.test.com",
		},
		{
			name:        "without_custom_base_url",
			optionsURL:  "",
			expectedURL: "", // Will use getBaseURL()
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := DefaultServerOptions()
			if tc.optionsURL != "" {
				opts.WithBaseURL(tc.optionsURL)
			}

			server := NewServerWithOptions(opts)
			effectiveURL := server.getEffectiveBaseURL()

			if tc.expectedURL != "" {
				assert.Equal(t, tc.expectedURL, effectiveURL, "Should return custom base URL")
			} else {
				// Should return result from getBaseURL()
				assert.NotEmpty(t, effectiveURL, "Should return some base URL")
			}

			t.Logf("✓ Server effective base URL test '%s' passed: %s", tc.name, effectiveURL)
		})
	}
}

func TestServerGetAvailableToolNames(t *testing.T) {
	testCases := []struct {
		name                 string
		enableInstructions   bool
		expectedToolsContain []string
		shouldContainInstr   bool
	}{
		{
			name:               "with_instructions_enabled",
			enableInstructions: true,
			expectedToolsContain: []string{
				"chat_completions",
				"completions",
				"embeddings",
				"images_generations",
			},
			shouldContainInstr: true,
		},
		{
			name:               "with_instructions_disabled",
			enableInstructions: false,
			expectedToolsContain: []string{
				"chat_completions",
				"completions",
				"embeddings",
				"images_generations",
			},
			shouldContainInstr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := DefaultServerOptions()
			if !tc.enableInstructions {
				opts.DisableInstructions()
			}

			server := NewServerWithOptions(opts)
			tools := server.getAvailableToolNames()

			// Check expected tools are present
			for _, expectedTool := range tc.expectedToolsContain {
				assert.Contains(t, tools, expectedTool, "Should contain expected tool: %s", expectedTool)
			}

			// Check instructions tool presence
			if tc.shouldContainInstr {
				assert.Contains(t, tools, "instructions", "Should contain instructions tool")
			} else {
				assert.NotContains(t, tools, "instructions", "Should not contain instructions tool")
			}

			t.Logf("✓ Available tools test '%s' passed: %d tools found", tc.name, len(tools))
		})
	}
}

func TestInstructionRenderer(t *testing.T) {
	renderer, err := NewInstructionRenderer()
	assert.NoError(t, err, "Should create instruction renderer without error")
	assert.NotNil(t, renderer, "Renderer should not be nil")

	// Test available types
	types := renderer.GetAvailableInstructionTypes()
	assert.NotEmpty(t, types, "Should have available instruction types")

	expectedTypes := []InstructionType{
		GeneralInstructions,
		ToolUsageInstructions,
		APIEndpointInstructions,
		ErrorHandlingInstructions,
		BestPracticesInstructions,
	}

	for _, expectedType := range expectedTypes {
		assert.True(t, renderer.IsInstructionTypeSupported(expectedType), "Should support instruction type: %s", expectedType)
	}

	t.Logf("✓ Instruction renderer supports %d instruction types", len(types))
}

func TestInstructionGeneration(t *testing.T) {
	renderer, err := NewInstructionRenderer()
	assert.NoError(t, err, "Should create instruction renderer without error")

	templateData := InstructionTemplateData{
		BaseURL:        "https://test.example.com",
		ServerName:     "test-server",
		ServerVersion:  "1.0.0",
		AvailableTools: []string{"chat_completions", "embeddings"},
		CustomData:     map[string]any{"test": "value"},
	}

	testCases := []InstructionType{
		GeneralInstructions,
		ToolUsageInstructions,
		APIEndpointInstructions,
		ErrorHandlingInstructions,
		BestPracticesInstructions,
	}

	for _, instructionType := range testCases {
		t.Run(string(instructionType), func(t *testing.T) {
			instructions, err := renderer.GenerateInstructions(instructionType, templateData)

			// Should either succeed or fallback gracefully
			assert.NoError(t, err, "Should generate instructions without error")
			assert.NotEmpty(t, instructions, "Instructions should not be empty")

			// Check that template data is used
			assert.Contains(t, instructions, templateData.BaseURL, "Should contain base URL")
			assert.Contains(t, instructions, templateData.ServerName, "Should contain server name")
			assert.Contains(t, instructions, templateData.ServerVersion, "Should contain server version")

			t.Logf("✓ Generated %s instructions (%d chars)", instructionType, len(instructions))
		})
	}
}

func TestInstructionGenerationFallback(t *testing.T) {
	renderer, err := NewInstructionRenderer()
	assert.NoError(t, err, "Should create instruction renderer without error")

	templateData := InstructionTemplateData{
		BaseURL:        "https://test.example.com",
		ServerName:     "test-server",
		ServerVersion:  "1.0.0",
		AvailableTools: []string{"chat_completions"},
		CustomData:     make(map[string]any),
	}

	// Test with unsupported instruction type
	_, err = renderer.GenerateInstructions(InstructionType("unsupported"), templateData)
	assert.Error(t, err, "Should return error for unsupported instruction type")

	// Test fallback generation
	fallback := renderer.generateFallbackInstructions("test_type", templateData)
	assert.NotEmpty(t, fallback, "Fallback instructions should not be empty")
	assert.Contains(t, fallback, "test-server", "Should contain server name")
	assert.Contains(t, fallback, "https://test.example.com", "Should contain base URL")

	t.Logf("✓ Instruction fallback generation works correctly")
}

func TestGenerateFallbackInstructions(t *testing.T) {
	templateData := InstructionTemplateData{
		BaseURL:        "https://fallback.test.com",
		ServerName:     "fallback-server",
		ServerVersion:  "2.0.0",
		AvailableTools: []string{"tool1", "tool2", "tool3"},
		CustomData:     make(map[string]any),
	}

	fallback := generateFallbackInstructions("test_instruction_type", templateData)

	assert.NotEmpty(t, fallback, "Fallback should not be empty")
	assert.Contains(t, fallback, "fallback-server", "Should contain server name")
	assert.Contains(t, fallback, "2.0.0", "Should contain server version")
	assert.Contains(t, fallback, "https://fallback.test.com", "Should contain base URL")
	assert.Contains(t, fallback, "test_instruction_type", "Should contain instruction type")

	// Check that tools are listed
	for _, tool := range templateData.AvailableTools {
		assert.Contains(t, fallback, tool, "Should contain tool: %s", tool)
	}

	t.Logf("✓ Fallback instruction generation works correctly")
}

func TestJoinTools(t *testing.T) {
	testCases := []struct {
		name     string
		tools    []string
		expected string
	}{
		{
			name:     "empty_tools",
			tools:    []string{},
			expected: "No tools available",
		},
		{
			name:     "single_tool",
			tools:    []string{"chat_completions"},
			expected: "- chat_completions\n",
		},
		{
			name:     "multiple_tools",
			tools:    []string{"chat_completions", "embeddings", "images"},
			expected: "- chat_completions\n- embeddings\n- images\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := joinTools(tc.tools)
			assert.Equal(t, tc.expected, result, "Should format tools correctly")

			t.Logf("✓ joinTools test '%s' passed", tc.name)
		})
	}
}

func TestInstructionTemplateIntegration(t *testing.T) {
	// Test that the instruction system integrates with the global renderer
	if globalRenderer == nil {
		t.Skip("Global renderer not available, skipping integration test")
	}

	instructionRenderer := globalRenderer.GetInstructionRenderer()
	if instructionRenderer == nil {
		t.Skip("Instruction renderer not available, skipping integration test")
	}

	templateData := InstructionTemplateData{
		BaseURL:        "https://integration.test.com",
		ServerName:     "integration-server",
		ServerVersion:  "1.0.0",
		AvailableTools: []string{"chat_completions", "embeddings"},
		CustomData:     make(map[string]any),
	}

	// Test generation with global renderer
	instructions, err := instructionRenderer.GenerateInstructions(GeneralInstructions, templateData)

	// Should either succeed with template or fallback gracefully
	if err != nil {
		// If template loading fails, should still get fallback
		fallback := instructionRenderer.generateFallbackInstructions(string(GeneralInstructions), templateData)
		assert.NotEmpty(t, fallback, "Should get fallback instructions")
		t.Logf("✓ Integration test used fallback instructions")
	} else {
		assert.NotEmpty(t, instructions, "Should get generated instructions")
		assert.Contains(t, instructions, templateData.BaseURL, "Should contain base URL")
		t.Logf("✓ Integration test generated instructions successfully (%d chars)", len(instructions))
	}
}

func TestInstructionTypesConstants(t *testing.T) {
	// Test that all instruction type constants are defined correctly
	types := []struct {
		constant InstructionType
		expected string
	}{
		{GeneralInstructions, "general"},
		{ToolUsageInstructions, "tool_usage"},
		{APIEndpointInstructions, "api_endpoints"},
		{ErrorHandlingInstructions, "error_handling"},
		{BestPracticesInstructions, "best_practices"},
	}

	for _, typeTest := range types {
		t.Run(string(typeTest.constant), func(t *testing.T) {
			assert.Equal(t, typeTest.expected, string(typeTest.constant), "Instruction type constant should match expected value")
			t.Logf("✓ Instruction type '%s' defined correctly", typeTest.constant)
		})
	}
}

func TestServerWithInstructionsIntegration(t *testing.T) {
	// Create server with instructions enabled
	opts := DefaultServerOptions().
		WithName("integration-test-server").
		WithInstructionType(GeneralInstructions).
		WithCustomTemplateData("test_integration", "value")

	server := NewServerWithOptions(opts)

	assert.NotNil(t, server, "Server should be created")
	assert.True(t, server.options.EnableInstructions, "Instructions should be enabled")
	assert.Equal(t, GeneralInstructions, server.options.Instructions.Type, "Instruction type should be set")

	// Test that available tools include instructions
	tools := server.getAvailableToolNames()
	assert.Contains(t, tools, "instructions", "Should include instructions tool")

	// Test effective base URL
	baseURL := server.getEffectiveBaseURL()
	assert.NotEmpty(t, baseURL, "Should have effective base URL")

	t.Logf("✓ Server with instructions integration test passed")
}
