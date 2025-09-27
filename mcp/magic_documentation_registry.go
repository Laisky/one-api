package mcp

// Global renderer instance
var globalRenderer *DocumentationRenderer

// init initializes the global renderer
func init() {
	var err error
	globalRenderer, err = NewDocumentationRenderer()
	if err != nil {
		// Fall back to nil renderer if template loading fails
		globalRenderer = nil
	}
}

// initializeRegistry sets up the mapping between documentation types and template names
func (r *DocumentationRenderer) initializeRegistry() {
	r.registry = map[DocumentationType]string{
		ChatCompletions:     "chat_completions",
		Completions:         "completions",
		Embeddings:          "embeddings",
		Images:              "images",
		AudioTranscriptions: "audio_transcriptions",
		AudioTranslations:   "audio_translations",
		AudioSpeech:         "audio_speech",
		Moderations:         "moderations",
		ModelsList:          "models_list",
		ClaudeMessages:      "claude_messages",
	}
}
