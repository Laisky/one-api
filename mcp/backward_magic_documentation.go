package mcp

// Public API Functions (Exported)
// These functions maintain the original public API for external consumers.

// GenerateChatCompletionsDocumentationFromTemplate generates documentation for
// the OpenAI Chat Completions API using the provided base URL.
//
// This function maintains backward compatibility with the original API while
// internally using the new unified documentation generation system.
//
// Parameters:
//   - baseURL: The base URL for the API endpoint (e.g., "https://api.example.com")
//
// Returns a string containing the complete Chat Completions API documentation.
func GenerateChatCompletionsDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(ChatCompletions, baseURL)
}

// GenerateCompletionsDocumentationFromTemplate generates documentation for
// the OpenAI Completions API using the provided base URL.
//
// This function maintains backward compatibility with the original API while
// internally using the new unified documentation generation system.
//
// Parameters:
//   - baseURL: The base URL for the API endpoint (e.g., "https://api.example.com")
//
// Returns a string containing the complete Completions API documentation.
func GenerateCompletionsDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(Completions, baseURL)
}

// GenerateEmbeddingsDocumentationFromTemplate generates documentation for
// the OpenAI Embeddings API using the provided base URL.
//
// This function maintains backward compatibility with the original API while
// internally using the new unified documentation generation system.
//
// Parameters:
//   - baseURL: The base URL for the API endpoint (e.g., "https://api.example.com")
//
// Returns a string containing the complete Embeddings API documentation.
func GenerateEmbeddingsDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(Embeddings, baseURL)
}

// GenerateImagesDocumentationFromTemplate generates documentation for
// the OpenAI Image Generation API using the provided base URL.
//
// This function maintains backward compatibility with the original API while
// internally using the new unified documentation generation system.
//
// Parameters:
//   - baseURL: The base URL for the API endpoint (e.g., "https://api.example.com")
//
// Returns a string containing the complete Image Generation API documentation.
func GenerateImagesDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(Images, baseURL)
}

// GenerateAudioTranscriptionsDocumentationFromTemplate generates documentation for
// the OpenAI Audio Transcriptions API using the provided base URL.
//
// This function maintains backward compatibility with the original API while
// internally using the new unified documentation generation system.
//
// Parameters:
//   - baseURL: The base URL for the API endpoint (e.g., "https://api.example.com")
//
// Returns a string containing the complete Audio Transcriptions API documentation.
func GenerateAudioTranscriptionsDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(AudioTranscriptions, baseURL)
}

// GenerateAudioTranslationsDocumentationFromTemplate generates documentation for
// the OpenAI Audio Translations API using the provided base URL.
//
// This function maintains backward compatibility with the original API while
// internally using the new unified documentation generation system.
//
// Parameters:
//   - baseURL: The base URL for the API endpoint (e.g., "https://api.example.com")
//
// Returns a string containing the complete Audio Translations API documentation.
func GenerateAudioTranslationsDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(AudioTranslations, baseURL)
}

// GenerateAudioSpeechDocumentationFromTemplate generates documentation for
// the OpenAI Audio Speech API using the provided base URL.
//
// This function maintains backward compatibility with the original API while
// internally using the new unified documentation generation system.
//
// Parameters:
//   - baseURL: The base URL for the API endpoint (e.g., "https://api.example.com")
//
// Returns a string containing the complete Audio Speech API documentation.
func GenerateAudioSpeechDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(AudioSpeech, baseURL)
}

// GenerateModerationsDocumentationFromTemplate generates documentation for
// the OpenAI Moderations API using the provided base URL.
//
// This function maintains backward compatibility with the original API while
// internally using the new unified documentation generation system.
//
// Parameters:
//   - baseURL: The base URL for the API endpoint (e.g., "https://api.example.com")
//
// Returns a string containing the complete Moderations API documentation.
func GenerateModerationsDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(Moderations, baseURL)
}

// GenerateModelsListDocumentationFromTemplate generates documentation for
// the Models List API using the provided base URL.
//
// This function maintains backward compatibility with the original API while
// internally using the new unified documentation generation system.
//
// Parameters:
//   - baseURL: The base URL for the API endpoint (e.g., "https://api.example.com")
//
// Returns a string containing the complete Models List API documentation.
func GenerateModelsListDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(ModelsList, baseURL)
}

// GenerateClaudeMessagesDocumentationFromTemplate generates documentation for
// the Claude Messages API using the provided base URL.
//
// This function maintains backward compatibility with the original API while
// internally using the new unified documentation generation system.
//
// Parameters:
//   - baseURL: The base URL for the API endpoint (e.g., "https://api.example.com")
//
// Returns a string containing the complete Claude Messages API documentation.
func GenerateClaudeMessagesDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(ClaudeMessages, baseURL)
}

// Internal API Functions (Unexported)
// These functions maintain backward compatibility for internal package usage.

// generateChatCompletionsDocumentationFromTemplate is the internal version of
// the Chat Completions documentation generator, maintaining compatibility with
// existing internal code that uses lowercase function names.
func generateChatCompletionsDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(ChatCompletions, baseURL)
}

// generateCompletionsDocumentationFromTemplate is the internal version of
// the Completions documentation generator, maintaining compatibility with
// existing internal code that uses lowercase function names.
func generateCompletionsDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(Completions, baseURL)
}

// generateEmbeddingsDocumentationFromTemplate is the internal version of
// the Embeddings documentation generator, maintaining compatibility with
// existing internal code that uses lowercase function names.
func generateEmbeddingsDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(Embeddings, baseURL)
}

// generateImagesDocumentationFromTemplate is the internal version of
// the Images documentation generator, maintaining compatibility with
// existing internal code that uses lowercase function names.
func generateImagesDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(Images, baseURL)
}

// generateAudioTranscriptionsDocumentationFromTemplate is the internal version of
// the Audio Transcriptions documentation generator, maintaining compatibility with
// existing internal code that uses lowercase function names.
func generateAudioTranscriptionsDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(AudioTranscriptions, baseURL)
}

// generateAudioTranslationsDocumentationFromTemplate is the internal version of
// the Audio Translations documentation generator, maintaining compatibility with
// existing internal code that uses lowercase function names.
func generateAudioTranslationsDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(AudioTranslations, baseURL)
}

// generateAudioSpeechDocumentationFromTemplate is the internal version of
// the Audio Speech documentation generator, maintaining compatibility with
// existing internal code that uses lowercase function names.
func generateAudioSpeechDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(AudioSpeech, baseURL)
}

// generateModerationsDocumentationFromTemplate is the internal version of
// the Moderations documentation generator, maintaining compatibility with
// existing internal code that uses lowercase function names.
func generateModerationsDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(Moderations, baseURL)
}

// generateModelsListDocumentationFromTemplate is the internal version of
// the Models List documentation generator, maintaining compatibility with
// existing internal code that uses lowercase function names.
func generateModelsListDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(ModelsList, baseURL)
}

// generateClaudeMessagesDocumentationFromTemplate is the internal version of
// the Claude Messages documentation generator, maintaining compatibility with
// existing internal code that uses lowercase function names.
func generateClaudeMessagesDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(ClaudeMessages, baseURL)
}
