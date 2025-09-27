package mcp

// Backward compatibility functions - these maintain the existing API
// while internally using the new reusable system

// GenerateChatCompletionsDocumentationFromTemplate generates chat completions documentation
func GenerateChatCompletionsDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(ChatCompletions, baseURL)
}

// GenerateCompletionsDocumentationFromTemplate generates completions documentation
func GenerateCompletionsDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(Completions, baseURL)
}

// GenerateEmbeddingsDocumentationFromTemplate generates embeddings documentation
func GenerateEmbeddingsDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(Embeddings, baseURL)
}

// GenerateImagesDocumentationFromTemplate generates images documentation
func GenerateImagesDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(Images, baseURL)
}

// GenerateAudioTranscriptionsDocumentationFromTemplate generates audio transcriptions documentation
func GenerateAudioTranscriptionsDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(AudioTranscriptions, baseURL)
}

// GenerateAudioTranslationsDocumentationFromTemplate generates audio translations documentation
func GenerateAudioTranslationsDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(AudioTranslations, baseURL)
}

// GenerateAudioSpeechDocumentationFromTemplate generates audio speech documentation
func GenerateAudioSpeechDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(AudioSpeech, baseURL)
}

// GenerateModerationsDocumentationFromTemplate generates moderations documentation
func GenerateModerationsDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(Moderations, baseURL)
}

// GenerateModelsListDocumentationFromTemplate generates models list documentation
func GenerateModelsListDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(ModelsList, baseURL)
}

// GenerateClaudeMessagesDocumentationFromTemplate generates Claude messages documentation
func GenerateClaudeMessagesDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(ClaudeMessages, baseURL)
}

// Legacy function names for backward compatibility (lowercase versions)
func generateChatCompletionsDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(ChatCompletions, baseURL)
}

func generateCompletionsDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(Completions, baseURL)
}

func generateEmbeddingsDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(Embeddings, baseURL)
}

func generateImagesDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(Images, baseURL)
}

func generateAudioTranscriptionsDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(AudioTranscriptions, baseURL)
}

func generateAudioTranslationsDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(AudioTranslations, baseURL)
}

func generateAudioSpeechDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(AudioSpeech, baseURL)
}

func generateModerationsDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(Moderations, baseURL)
}

func generateModelsListDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(ModelsList, baseURL)
}

func generateClaudeMessagesDocumentationFromTemplate(baseURL string) string {
	return GenerateDocumentation(ClaudeMessages, baseURL)
}
