package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

func init() {
	// Enable approximate token counting for tests to avoid tiktoken initialization issues
	config.ApproximateTokenEnabled = true
}

func TestGetPromptTokens(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		relayMode   int
		request     *model.GeneralOpenAIRequest
		expectError bool
		expectZero  bool
	}{
		{
			name:      "ChatCompletions with messages",
			relayMode: relaymode.ChatCompletions,
			request: &model.GeneralOpenAIRequest{
				Model: "gpt-3.5-turbo",
				Messages: []model.Message{
					{Role: "user", Content: "Hello, world!"},
				},
			},
			expectError: false,
			expectZero:  false,
		},
		{
			name:      "Completions with prompt",
			relayMode: relaymode.Completions,
			request: &model.GeneralOpenAIRequest{
				Model:  "text-davinci-003",
				Prompt: "Hello, world!",
			},
			expectError: false,
			expectZero:  false,
		},
		{
			name:      "Moderations with input",
			relayMode: relaymode.Moderations,
			request: &model.GeneralOpenAIRequest{
				Model: "text-moderation-latest",
				Input: "Hello, world!",
			},
			expectError: false,
			expectZero:  false,
		},
		{
			name:      "Embeddings with string input",
			relayMode: relaymode.Embeddings,
			request: &model.GeneralOpenAIRequest{
				Model: "text-embedding-ada-002",
				Input: "The food was delicious and the waiter was very friendly.",
			},
			expectError: false,
			expectZero:  false,
		},
		{
			name:      "Embeddings with array input",
			relayMode: relaymode.Embeddings,
			request: &model.GeneralOpenAIRequest{
				Model: "text-embedding-ada-002",
				Input: []any{"Hello", "World", "Test"},
			},
			expectError: false,
			expectZero:  false,
		},
		{
			name:      "Edits with instruction",
			relayMode: relaymode.Edits,
			request: &model.GeneralOpenAIRequest{
				Model:       "text-davinci-edit-001",
				Instruction: "Fix the grammar in this sentence",
			},
			expectError: false,
			expectZero:  false,
		},
		{
			name:      "Unknown relay mode",
			relayMode: relaymode.Unknown,
			request: &model.GeneralOpenAIRequest{
				Model: "test-model",
			},
			expectError: false, // Should not error, but should return 0 and log
			expectZero:  true,
		},
		{
			name:      "ImagesGenerations (should return 0)",
			relayMode: relaymode.ImagesGenerations,
			request: &model.GeneralOpenAIRequest{
				Model: "dall-e-3",
			},
			expectError: false,
			expectZero:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := getPromptTokens(ctx, tt.request, tt.relayMode)

			if tt.expectZero {
				require.Zero(t, tokens, "Expected 0 tokens for %s", tt.name)
			}

			if !tt.expectZero {
				require.NotZero(t, tokens, "Expected non-zero tokens for %s", tt.name)
			}

			require.GreaterOrEqual(t, tokens, 0, "Token count should never be negative for %s", tt.name)
		})
	}
}

func TestGetPromptTokensEmbeddingsSpecific(t *testing.T) {
	ctx := context.Background()

	// Test different input formats for embeddings
	testCases := []struct {
		name     string
		input    any
		expected bool // whether we expect tokens > 0
	}{
		{
			name:     "String input",
			input:    "The food was delicious and the waiter was very friendly.",
			expected: true,
		},
		{
			name:     "Array of strings",
			input:    []any{"Hello", "World", "Test embedding"},
			expected: true,
		},
		{
			name:     "Empty string",
			input:    "",
			expected: false, // Empty string should result in 0 tokens
		},
		{
			name:     "Nil input",
			input:    nil,
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			request := &model.GeneralOpenAIRequest{
				Model: "text-embedding-ada-002",
				Input: tc.input,
			}

			tokens := getPromptTokens(ctx, request, relaymode.Embeddings)

			if tc.expected {
				require.NotZero(t, tokens, "Expected tokens > 0 for %s", tc.name)
			}

			if !tc.expected {
				require.Zero(t, tokens, "Expected tokens = 0 for %s", tc.name)
			}
		})
	}
}
