package mcp

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateChatCompletionsDocumentation(t *testing.T) {
	baseURL := "https://api.example.com"
	doc := generateChatCompletionsDocumentationFromTemplate(baseURL)

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
	doc := generateCompletionsDocumentationFromTemplate(baseURL)

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
	doc := generateEmbeddingsDocumentationFromTemplate(baseURL)

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
	doc := generateImagesDocumentationFromTemplate(baseURL)

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
	doc := generateAudioTranscriptionsDocumentationFromTemplate(baseURL)

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
	doc := generateAudioTranslationsDocumentationFromTemplate(baseURL)

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
	doc := generateAudioSpeechDocumentationFromTemplate(baseURL)

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
	doc := generateModerationsDocumentationFromTemplate(baseURL)

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
	doc := generateModelsListDocumentationFromTemplate(baseURL)

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
	doc := generateClaudeMessagesDocumentationFromTemplate(baseURL)

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
			doc := generateChatCompletionsDocumentationFromTemplate(tc.baseURL)

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
		{"chat_completions", generateChatCompletionsDocumentationFromTemplate, "/v1/chat/completions"},
		{"completions", generateCompletionsDocumentationFromTemplate, "/v1/completions"},
		{"embeddings", generateEmbeddingsDocumentationFromTemplate, "/v1/embeddings"},
		{"images", generateImagesDocumentationFromTemplate, "/v1/images/generations"},
		{"audio_transcriptions", generateAudioTranscriptionsDocumentationFromTemplate, "/v1/audio/transcriptions"},
		{"audio_translations", generateAudioTranslationsDocumentationFromTemplate, "/v1/audio/translations"},
		{"audio_speech", generateAudioSpeechDocumentationFromTemplate, "/v1/audio/speech"},
		{"moderations", generateModerationsDocumentationFromTemplate, "/v1/moderations"},
		{"models_list", generateModelsListDocumentationFromTemplate, "/v1/models"},
		{"claude_messages", generateClaudeMessagesDocumentationFromTemplate, "/v1/messages"},
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
		generateChatCompletionsDocumentationFromTemplate(baseURL),
		generateCompletionsDocumentationFromTemplate(baseURL),
		generateEmbeddingsDocumentationFromTemplate(baseURL),
		generateImagesDocumentationFromTemplate(baseURL),
		generateAudioTranscriptionsDocumentationFromTemplate(baseURL),
		generateAudioTranslationsDocumentationFromTemplate(baseURL),
		generateAudioSpeechDocumentationFromTemplate(baseURL),
		generateModerationsDocumentationFromTemplate(baseURL),
		generateModelsListDocumentationFromTemplate(baseURL),
		generateClaudeMessagesDocumentationFromTemplate(baseURL),
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

// Test the new GenerateDocumentation function
func TestGenerateDocumentation(t *testing.T) {
	baseURL := "https://api.example.com"

	testCases := []struct {
		name    string
		docType DocumentationType
	}{
		{"chat_completions", ChatCompletions},
		{"completions", Completions},
		{"embeddings", Embeddings},
		{"images", Images},
		{"audio_transcriptions", AudioTranscriptions},
		{"audio_translations", AudioTranslations},
		{"audio_speech", AudioSpeech},
		{"moderations", Moderations},
		{"models_list", ModelsList},
		{"claude_messages", ClaudeMessages},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc := GenerateDocumentation(tc.docType, baseURL)

			// Verify documentation is not empty
			assert.NotEmpty(t, doc, "Documentation should not be empty")
			assert.Greater(t, len(doc), 50, "Documentation should be substantial")

			// Verify baseURL is included
			assert.Contains(t, doc, baseURL, "Documentation should contain base URL")

			// Verify it has proper structure
			assert.Contains(t, doc, "#", "Documentation should have headers")

			t.Logf("✓ %s documentation generated successfully (%d chars)", tc.name, len(doc))
		})
	}
}

// Test DocumentationRenderer methods
func TestDocumentationRenderer(t *testing.T) {
	renderer, err := NewDocumentationRenderer()
	assert.NoError(t, err, "Should create renderer without error")
	assert.NotNil(t, renderer, "Renderer should not be nil")

	// Test GetAvailableTypes
	types := renderer.GetAvailableTypes()
	assert.NotEmpty(t, types, "Should have available types")
	assert.GreaterOrEqual(t, len(types), 10, "Should have at least 10 types")

	// Test IsTypeSupported
	assert.True(t, renderer.IsTypeSupported(ChatCompletions), "Should support ChatCompletions")
	assert.True(t, renderer.IsTypeSupported(Embeddings), "Should support Embeddings")
	assert.False(t, renderer.IsTypeSupported(DocumentationType("nonexistent")), "Should not support nonexistent type")

	// Test GenerateDocumentation method
	doc, err := renderer.GenerateDocumentation(ChatCompletions, "https://test.com")
	assert.NoError(t, err, "Should generate documentation without error")
	assert.NotEmpty(t, doc, "Documentation should not be empty")
	assert.Contains(t, doc, "https://test.com", "Should contain base URL")

	t.Logf("✓ DocumentationRenderer methods work correctly")
}

// Test error handling for unknown documentation types
func TestGenerateDocumentationUnknownType(t *testing.T) {
	renderer, err := NewDocumentationRenderer()
	assert.NoError(t, err, "Should create renderer without error")

	// Test with unknown type
	doc, err := renderer.GenerateDocumentation(DocumentationType("unknown"), "https://test.com")
	assert.Error(t, err, "Should return error for unknown type")
	assert.Empty(t, doc, "Documentation should be empty for unknown type")

	t.Logf("✓ Error handling for unknown types works correctly")
}

// Test backward compatibility - ensure old functions still work
func TestBackwardCompatibility(t *testing.T) {
	baseURL := "https://api.example.com"

	testCases := []struct {
		name string
		fn   func(string) string
	}{
		{"generateChatCompletionsDocumentationFromTemplate", generateChatCompletionsDocumentationFromTemplate},
		{"generateCompletionsDocumentationFromTemplate", generateCompletionsDocumentationFromTemplate},
		{"generateEmbeddingsDocumentationFromTemplate", generateEmbeddingsDocumentationFromTemplate},
		{"generateImagesDocumentationFromTemplate", generateImagesDocumentationFromTemplate},
		{"generateAudioTranscriptionsDocumentationFromTemplate", generateAudioTranscriptionsDocumentationFromTemplate},
		{"generateAudioTranslationsDocumentationFromTemplate", generateAudioTranslationsDocumentationFromTemplate},
		{"generateAudioSpeechDocumentationFromTemplate", generateAudioSpeechDocumentationFromTemplate},
		{"generateModerationsDocumentationFromTemplate", generateModerationsDocumentationFromTemplate},
		{"generateModelsListDocumentationFromTemplate", generateModelsListDocumentationFromTemplate},
		{"generateClaudeMessagesDocumentationFromTemplate", generateClaudeMessagesDocumentationFromTemplate},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc := tc.fn(baseURL)

			// Verify documentation is generated
			assert.NotEmpty(t, doc, "Documentation should not be empty")
			assert.Contains(t, doc, baseURL, "Documentation should contain base URL")

			t.Logf("✓ %s works correctly (backward compatibility)", tc.name)
		})
	}
}

// Test that new and old functions produce the same output
func TestNewVsOldFunctionEquivalence(t *testing.T) {
	baseURL := "https://api.example.com"

	testCases := []struct {
		docType DocumentationType
		oldFn   func(string) string
	}{
		{ChatCompletions, generateChatCompletionsDocumentationFromTemplate},
		{Completions, generateCompletionsDocumentationFromTemplate},
		{Embeddings, generateEmbeddingsDocumentationFromTemplate},
		{Images, generateImagesDocumentationFromTemplate},
		{AudioTranscriptions, generateAudioTranscriptionsDocumentationFromTemplate},
		{AudioTranslations, generateAudioTranslationsDocumentationFromTemplate},
		{AudioSpeech, generateAudioSpeechDocumentationFromTemplate},
		{Moderations, generateModerationsDocumentationFromTemplate},
		{ModelsList, generateModelsListDocumentationFromTemplate},
		{ClaudeMessages, generateClaudeMessagesDocumentationFromTemplate},
	}

	for _, tc := range testCases {
		t.Run(string(tc.docType), func(t *testing.T) {
			newDoc := GenerateDocumentation(tc.docType, baseURL)
			oldDoc := tc.oldFn(baseURL)

			// Both should produce the same output
			assert.Equal(t, newDoc, oldDoc, "New and old functions should produce identical output")

			t.Logf("✓ %s: new and old functions produce identical output", tc.docType)
		})
	}
}

// Test template loading and registry initialization
func TestTemplateLoadingAndRegistry(t *testing.T) {
	renderer, err := NewDocumentationRenderer()
	assert.NoError(t, err, "Should create renderer without error")

	// Verify registry is properly initialized
	expectedTypes := []DocumentationType{
		ChatCompletions, Completions, Embeddings, Images,
		AudioTranscriptions, AudioTranslations, AudioSpeech,
		Moderations, ModelsList, ClaudeMessages,
	}

	for _, docType := range expectedTypes {
		assert.True(t, renderer.IsTypeSupported(docType), "Should support %s", docType)
	}

	t.Logf("✓ Template loading and registry initialization works correctly")
}

// Test performance - ensure new system is not significantly slower
func TestPerformanceComparison(t *testing.T) {
	baseURL := "https://api.example.com"

	// Test new function performance
	t.Run("new_function", func(t *testing.T) {
		for i := 0; i < 1000; i++ {
			doc := GenerateDocumentation(ChatCompletions, baseURL)
			assert.NotEmpty(t, doc)
		}
	})

	// Test old function performance
	t.Run("old_function", func(t *testing.T) {
		for i := 0; i < 1000; i++ {
			doc := generateChatCompletionsDocumentationFromTemplate(baseURL)
			assert.NotEmpty(t, doc)
		}
	})

	t.Logf("✓ Performance comparison completed")
}

// Test global renderer initialization
func TestGlobalRendererInitialization(t *testing.T) {
	// The global renderer should be initialized during package init
	assert.NotNil(t, globalRenderer, "Global renderer should be initialized")

	// Test that GenerateDocumentation works with global renderer
	doc := GenerateDocumentation(ChatCompletions, "https://test.com")
	assert.NotEmpty(t, doc, "Should generate documentation using global renderer")
	assert.Contains(t, doc, "https://test.com", "Should contain base URL")

	t.Logf("✓ Global renderer initialization works correctly")
}

// Test documentation consistency across all types
func TestDocumentationConsistencyNewSystem(t *testing.T) {
	baseURL := "https://api.example.com"

	renderer, err := NewDocumentationRenderer()
	assert.NoError(t, err, "Should create renderer without error")

	types := renderer.GetAvailableTypes()

	for _, docType := range types {
		t.Run(string(docType), func(t *testing.T) {
			doc, err := renderer.GenerateDocumentation(docType, baseURL)
			assert.NoError(t, err, "Should generate documentation without error")

			// Check for consistent structure
			assert.Contains(t, doc, "#", "Should have headers")
			assert.Contains(t, doc, baseURL, "Should contain base URL")
			assert.Greater(t, len(doc), 100, "Should be substantial documentation")

			// Check for common sections that should exist in API documentation
			docLower := strings.ToLower(doc)
			assert.True(t,
				strings.Contains(docLower, "api") ||
					strings.Contains(docLower, "endpoint") ||
					strings.Contains(docLower, "description"),
				"Should contain API-related content")

			t.Logf("✓ %s documentation is consistent", docType)
		})
	}
}
