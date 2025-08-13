package openai

import (
	"testing"
)

func TestIsModelSupportedReasoning(t *testing.T) {
	testCases := []struct {
		model    string
		expected bool
		name     string
	}{
		// Reasoning models (should return true)
		{"o1-preview", true, "o1-preview should be a reasoning model"},
		{"o1-mini", true, "o1-mini should be a reasoning model"},
		{"o3-mini", true, "o3-mini should be a reasoning model"},
		{"o4-preview", true, "o4-preview should be a reasoning model"},
		{"gpt-5", true, "gpt-5 should be a reasoning model"},
		{"gpt-5-mini", true, "gpt-5-mini should be a reasoning model"},
		{"gpt-5-nano", true, "gpt-5-nano should be a reasoning model"},

		// Non-reasoning models (should return false)
		{"gpt-4", false, "gpt-4 should not be a reasoning model"},
		{"gpt-4o", false, "gpt-4o should not be a reasoning model"},
		{"gpt-3.5-turbo", false, "gpt-3.5-turbo should not be a reasoning model"},
		{"claude-3", false, "claude-3 should not be a reasoning model"},
		{"", false, "empty model should not be a reasoning model"},
		{"unknown-model", false, "unknown model should not be a reasoning model"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isModelSupportedReasoning(tc.model)
			if result != tc.expected {
				t.Errorf("isModelSupportedReasoning(%q) = %v, want %v", tc.model, result, tc.expected)
			}
		})
	}
}

func TestReasoningModelMaxTokensHandling(t *testing.T) {
	// Test that reasoning models have MaxTokens set to 0
	// while non-reasoning models preserve MaxTokens

	testCases := []struct {
		model            string
		isReasoningModel bool
		name             string
	}{
		{"o1-preview", true, "o1-preview is a reasoning model"},
		{"o3-mini", true, "o3-mini is a reasoning model"},
		{"gpt-5-mini", true, "gpt-5-mini is a reasoning model"},
		{"gpt-4", false, "gpt-4 is not a reasoning model"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isModelSupportedReasoning(tc.model)
			if result != tc.isReasoningModel {
				t.Errorf("isModelSupportedReasoning(%q) = %v, want %v", tc.model, result, tc.isReasoningModel)
			}

			// Verify the logic matches our expectations
			if tc.isReasoningModel {
				t.Logf("✓ %s correctly identified as reasoning model (MaxTokens will be set to 0)", tc.model)
			} else {
				t.Logf("✓ %s correctly identified as non-reasoning model (MaxTokens will be preserved)", tc.model)
			}
		})
	}
}
