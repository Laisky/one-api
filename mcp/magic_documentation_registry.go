package mcp

// globalRenderer is the package-level singleton instance of DocumentationRenderer
// used by the GenerateDocumentation function. It is initialized during package
// initialization and provides template caching for optimal performance.
var globalRenderer *DocumentationRenderer

// init initializes the global documentation renderer during package initialization.
// This ensures that templates are loaded once when the package is first imported,
// providing optimal performance for subsequent documentation generation calls.
//
// If template loading fails during initialization, the global renderer is set to nil,
// and the system will fall back to generic documentation generation to maintain
// system reliability.
func init() {
	var err error
	globalRenderer, err = NewDocumentationRenderer()
	if err != nil {
		// Fall back to nil renderer if template loading fails
		// This ensures the package remains functional even if templates are missing
		globalRenderer = nil
	}
}

// initializeRegistry sets up the mapping between documentation types and their
// corresponding template names. This registry provides the association between
// DocumentationType constants and the actual template file names.
//
// The registry enables:
//   - Type-safe documentation generation
//   - Automatic template discovery
//   - Centralized mapping management
//   - Easy addition of new documentation types
//
// Each entry maps a DocumentationType constant to its corresponding template
// file name (without the .tmpl extension). The template files must exist in
// the docs/templates/ directory for successful documentation generation.
func (r *DocumentationRenderer) initializeRegistry() {
	r.registry = map[DocumentationType]string{
		ChatCompletions:     "chat_completions",     // maps to chat_completions.tmpl
		Completions:         "completions",          // maps to completions.tmpl
		Embeddings:          "embeddings",           // maps to embeddings.tmpl
		Images:              "images",               // maps to images.tmpl
		AudioTranscriptions: "audio_transcriptions", // maps to audio_transcriptions.tmpl
		AudioTranslations:   "audio_translations",   // maps to audio_translations.tmpl
		AudioSpeech:         "audio_speech",         // maps to audio_speech.tmpl
		Moderations:         "moderations",          // maps to moderations.tmpl
		ModelsList:          "models_list",          // maps to models_list.tmpl
		ClaudeMessages:      "claude_messages",      // maps to claude_messages.tmpl
	}
}

// initializeInstructionRegistry sets up the mapping between instruction types and their
// corresponding template names. This registry provides the association between
// InstructionType constants and the actual instruction template file names.
//
// The registry enables:
//   - Type-safe instruction generation
//   - Automatic template discovery
//   - Centralized mapping management
//   - Easy addition of new instruction types
//
// Each entry maps an InstructionType constant to its corresponding template
// file name (without the .tmpl extension). The template files must exist in
// the docs/templates/instructions/ directory for successful instruction generation.
func (r *InstructionRenderer) initializeInstructionRegistry() {
	r.registry = map[InstructionType]string{
		GeneralInstructions:       "general",        // maps to general.tmpl
		ToolUsageInstructions:     "tool_usage",     // maps to tool_usage.tmpl
		APIEndpointInstructions:   "api_endpoints",  // maps to api_endpoints.tmpl
		ErrorHandlingInstructions: "error_handling", // maps to error_handling.tmpl
		BestPracticesInstructions: "best_practices", // maps to best_practices.tmpl
	}
}
