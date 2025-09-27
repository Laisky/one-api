package mcp

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateChatCompletionsDocumentation(t *testing.T) {
	baseURL := "https://api.example.com"
	doc := generateChatCompletionsDocumentation(baseURL)

	// Test that documentation contains expected elements
	assert.Contains(t, doc, "# Chat Completions API")
	assert.Contains(t, doc, "POST https://api.example.com/v1/chat/completions")
	assert.Contains(t, doc, "Authorization: Bearer YOUR_API_KEY")
	assert.Contains(t, doc, `"model": "gpt-4"`)
	assert.Contains(t, doc, `"messages"`)
	assert.Contains(t, doc, `"temperature"`)
	assert.Contains(t, doc, `"max_tokens"`)
	assert.Contains(t, doc, `**stream**`)

	// Test response format structure
	assert.Contains(t, doc, `"id": "chatcmpl-123"`)
	assert.Contains(t, doc, `"object": "chat.completion"`)
	assert.Contains(t, doc, `"choices"`)
	assert.Contains(t, doc, `"usage"`)

	// Test that baseURL is properly substituted
	assert.Contains(t, doc, baseURL)

	t.Logf("Chat completions documentation length: %d characters", len(doc))
}

func TestGenerateCompletionsDocumentation(t *testing.T) {
	baseURL := "https://api.example.com"
	doc := generateCompletionsDocumentation(baseURL)

	// Test that documentation contains expected elements
	assert.Contains(t, doc, "# Completions API")
	assert.Contains(t, doc, "POST https://api.example.com/v1/completions")
	assert.Contains(t, doc, "Authorization: Bearer YOUR_API_KEY")
	assert.Contains(t, doc, `"model": "gpt-3.5-turbo-instruct"`)
	assert.Contains(t, doc, `"prompt"`)
	assert.Contains(t, doc, `"max_tokens"`)
	assert.Contains(t, doc, `"temperature"`)

	// Test that baseURL is properly substituted
	assert.Contains(t, doc, baseURL)

	t.Logf("Completions documentation length: %d characters", len(doc))
}

func TestGenerateEmbeddingsDocumentation(t *testing.T) {
	baseURL := "https://api.example.com"
	doc := generateEmbeddingsDocumentation(baseURL)

	// Test that documentation contains expected elements
	assert.Contains(t, doc, "# Embeddings API")
	assert.Contains(t, doc, "POST https://api.example.com/v1/embeddings")
	assert.Contains(t, doc, "Authorization: Bearer YOUR_API_KEY")
	assert.Contains(t, doc, `"model": "text-embedding-ada-002"`)
	assert.Contains(t, doc, `"input"`)

	// Test that baseURL is properly substituted
	assert.Contains(t, doc, baseURL)

	t.Logf("Embeddings documentation length: %d characters", len(doc))
}

func TestGenerateImagesDocumentation(t *testing.T) {
	baseURL := "https://api.example.com"
	doc := generateImagesDocumentation(baseURL)

	// Test that documentation contains expected elements
	assert.Contains(t, doc, "# Image Generation API")
	assert.Contains(t, doc, "POST https://api.example.com/v1/images/generations")
	assert.Contains(t, doc, "Authorization: Bearer YOUR_API_KEY")
	assert.Contains(t, doc, `"model": "dall-e-3"`)
	assert.Contains(t, doc, `"prompt"`)
	assert.Contains(t, doc, `"n"`)
	assert.Contains(t, doc, `"size"`)

	// Test that baseURL is properly substituted
	assert.Contains(t, doc, baseURL)

	t.Logf("Images documentation length: %d characters", len(doc))
}

func TestGenerateAudioTranscriptionsDocumentation(t *testing.T) {
	baseURL := "https://api.example.com"
	doc := generateAudioTranscriptionsDocumentation(baseURL)

	// Test that documentation contains expected elements
	assert.Contains(t, doc, "# Audio Transcriptions API")
	assert.Contains(t, doc, "POST https://api.example.com/v1/audio/transcriptions")
	assert.Contains(t, doc, "Authorization: Bearer YOUR_API_KEY")
	assert.Contains(t, doc, `"whisper-1"`)
	assert.Contains(t, doc, `file=`)
	assert.Contains(t, doc, `model=`)

	// Test that baseURL is properly substituted
	assert.Contains(t, doc, baseURL)

	t.Logf("Audio transcriptions documentation length: %d characters", len(doc))
}

func TestGenerateAudioTranslationsDocumentation(t *testing.T) {
	baseURL := "https://api.example.com"
	doc := generateAudioTranslationsDocumentation(baseURL)

	// Test that documentation contains expected elements
	assert.Contains(t, doc, "# Audio Translations API")
	assert.Contains(t, doc, "POST https://api.example.com/v1/audio/translations")
	assert.Contains(t, doc, "Authorization: Bearer YOUR_API_KEY")
	assert.Contains(t, doc, `"whisper-1"`)
	assert.Contains(t, doc, `file=`)

	// Test that baseURL is properly substituted
	assert.Contains(t, doc, baseURL)

	t.Logf("Audio translations documentation length: %d characters", len(doc))
}

func TestGenerateAudioSpeechDocumentation(t *testing.T) {
	baseURL := "https://api.example.com"
	doc := generateAudioSpeechDocumentation(baseURL)

	// Test that documentation contains expected elements
	assert.Contains(t, doc, "# Audio Speech API")
	assert.Contains(t, doc, "POST https://api.example.com/v1/audio/speech")
	assert.Contains(t, doc, "Authorization: Bearer YOUR_API_KEY")
	assert.Contains(t, doc, `"model": "tts-1"`)
	assert.Contains(t, doc, `"input"`)
	assert.Contains(t, doc, `"voice"`)
	assert.Contains(t, doc, `"alloy"`)

	// Test that baseURL is properly substituted
	assert.Contains(t, doc, baseURL)

	t.Logf("Audio speech documentation length: %d characters", len(doc))
}

func TestGenerateModerationsDocumentation(t *testing.T) {
	baseURL := "https://api.example.com"
	doc := generateModerationsDocumentation(baseURL)

	// Test that documentation contains expected elements
	assert.Contains(t, doc, "# Moderations API")
	assert.Contains(t, doc, "POST https://api.example.com/v1/moderations")
	assert.Contains(t, doc, "Authorization: Bearer YOUR_API_KEY")
	assert.Contains(t, doc, `"input"`)

	// Test that baseURL is properly substituted
	assert.Contains(t, doc, baseURL)

	t.Logf("Moderations documentation length: %d characters", len(doc))
}

func TestGenerateModelsListDocumentation(t *testing.T) {
	baseURL := "https://api.example.com"
	doc := generateModelsListDocumentation(baseURL)

	// Test that documentation contains expected elements
	assert.Contains(t, doc, "# Models List API")
	assert.Contains(t, doc, "GET https://api.example.com/v1/models")
	assert.Contains(t, doc, "Authorization: Bearer YOUR_API_KEY")

	// Test that baseURL is properly substituted
	assert.Contains(t, doc, baseURL)

	t.Logf("Models list documentation length: %d characters", len(doc))
}

func TestGenerateClaudeMessagesDocumentation(t *testing.T) {
	baseURL := "https://api.example.com"
	doc := generateClaudeMessagesDocumentation(baseURL)

	// Test that documentation contains expected elements
	assert.Contains(t, doc, "# Claude Messages API")
	assert.Contains(t, doc, "POST https://api.example.com/v1/messages")
	assert.Contains(t, doc, "Authorization: Bearer YOUR_API_KEY")
	assert.Contains(t, doc, `"model": "claude-3-opus-20240229"`)
	assert.Contains(t, doc, `"messages"`)
	assert.Contains(t, doc, `"max_tokens"`)

	// Test that baseURL is properly substituted
	assert.Contains(t, doc, baseURL)

	t.Logf("Claude messages documentation length: %d characters", len(doc))
}

func TestDocumentationWithDifferentBaseURLs(t *testing.T) {
	testCases := []struct {
		name    string
		baseURL string
	}{
		{"localhost", "http://localhost:3000"},
		{"https", "https://api.example.com"},
		{"http", "http://api.example.com"},
		{"with_port", "https://api.example.com:8080"},
		{"with_path", "https://api.example.com/api"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc := generateChatCompletionsDocumentation(tc.baseURL)

			// Verify the base URL appears correctly in the documentation
			assert.Contains(t, doc, tc.baseURL)

			// Verify the full endpoint URL is constructed correctly
			expectedEndpoint := tc.baseURL + "/v1/chat/completions"
			assert.Contains(t, doc, expectedEndpoint)

			t.Logf("✓ Documentation correctly uses baseURL: %s", tc.baseURL)
		})
	}
}

func TestDocumentationStructure(t *testing.T) {
	baseURL := "https://api.example.com"

	testCases := []struct {
		name     string
		genFunc  func(string) string
		endpoint string
	}{
		{"chat_completions", generateChatCompletionsDocumentation, "/v1/chat/completions"},
		{"completions", generateCompletionsDocumentation, "/v1/completions"},
		{"embeddings", generateEmbeddingsDocumentation, "/v1/embeddings"},
		{"images", generateImagesDocumentation, "/v1/images/generations"},
		{"audio_transcriptions", generateAudioTranscriptionsDocumentation, "/v1/audio/transcriptions"},
		{"audio_translations", generateAudioTranslationsDocumentation, "/v1/audio/translations"},
		{"audio_speech", generateAudioSpeechDocumentation, "/v1/audio/speech"},
		{"moderations", generateModerationsDocumentation, "/v1/moderations"},
		{"models_list", generateModelsListDocumentation, "/v1/models"},
		{"claude_messages", generateClaudeMessagesDocumentation, "/v1/messages"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc := tc.genFunc(baseURL)

			// Test common documentation structure
			assert.Contains(t, doc, "# ", "Should have header")
			assert.Contains(t, doc, "## Endpoint", "Should have endpoint section")
			assert.Contains(t, doc, "## Description", "Should have description section")
			assert.Contains(t, doc, "## Authentication", "Should have authentication section")
			assert.Contains(t, doc, "Authorization: Bearer YOUR_API_KEY", "Should have auth header")
			assert.Contains(t, doc, tc.endpoint, "Should contain correct endpoint")

			// Verify documentation is not empty
			assert.Greater(t, len(doc), 100, "Documentation should be substantial")

			// Verify no template placeholders remain
			assert.NotContains(t, doc, "%s", "Should not contain format placeholders")

			t.Logf("✓ %s documentation structure is valid", tc.name)
		})
	}
}

func TestDocumentationConsistency(t *testing.T) {
	baseURL := "https://api.example.com"

	docs := []string{
		generateChatCompletionsDocumentation(baseURL),
		generateCompletionsDocumentation(baseURL),
		generateEmbeddingsDocumentation(baseURL),
		generateImagesDocumentation(baseURL),
		generateAudioTranscriptionsDocumentation(baseURL),
		generateAudioTranslationsDocumentation(baseURL),
		generateAudioSpeechDocumentation(baseURL),
		generateModerationsDocumentation(baseURL),
		generateModelsListDocumentation(baseURL),
		generateClaudeMessagesDocumentation(baseURL),
	}

	// Test that all documentations follow consistent patterns
	for i, doc := range docs {
		// All should contain authentication section
		assert.Contains(t, doc, "Authorization: Bearer YOUR_API_KEY",
			"Documentation %d should have consistent auth format", i)

		// All should use consistent markdown formatting
		assert.True(t, strings.Contains(doc, "# ") && strings.Contains(doc, "## "),
			"Documentation %d should use consistent markdown headers", i)

		// All should contain the base URL
		assert.Contains(t, doc, baseURL,
			"Documentation %d should contain the base URL", i)
	}

	t.Logf("✓ All %d documentation functions follow consistent patterns", len(docs))
}
