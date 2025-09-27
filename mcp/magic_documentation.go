package mcp

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
)

//go:embed docs/templates/*.tmpl
var templateFS embed.FS

// TemplateData holds the data for rendering documentation templates
type TemplateData struct {
	BaseURL string
}

// DocumentationRenderer handles template-based documentation generation
type DocumentationRenderer struct {
	templates map[string]*template.Template
}

// NewDocumentationRenderer creates a new template-based documentation renderer
func NewDocumentationRenderer() (*DocumentationRenderer, error) {
	renderer := &DocumentationRenderer{
		templates: make(map[string]*template.Template),
	}

	// Load all template files
	templateFiles := []string{
		"chat_completions.tmpl",
		"completions.tmpl",
		"embeddings.tmpl",
		"images.tmpl",
		"audio_transcriptions.tmpl",
		"audio_translations.tmpl",
		"audio_speech.tmpl",
		"moderations.tmpl",
		"models_list.tmpl",
		"claude_messages.tmpl",
	}

	for _, filename := range templateFiles {
		templateName := filename[:len(filename)-5] // Remove .tmpl extension
		tmplContent, err := templateFS.ReadFile("docs/templates/" + filename)
		if err != nil {
			// Skip missing templates for now
			continue
		}

		tmpl, err := template.New(templateName).Parse(string(tmplContent))
		if err != nil {
			return nil, err
		}
		renderer.templates[templateName] = tmpl
	}

	return renderer, nil
}

// RenderDocumentation renders a template with the given data
func (r *DocumentationRenderer) RenderDocumentation(templateName string, data TemplateData) (string, error) {
	tmpl, exists := r.templates[templateName]
	if !exists {
		// Fallback to original implementation if template doesn't exist
		return r.fallbackToOriginal(templateName, data.BaseURL), nil
	}

	var buf bytes.Buffer
	err := tmpl.Execute(&buf, data)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// fallbackToOriginal provides a simple fallback when templates are not available
func (r *DocumentationRenderer) fallbackToOriginal(templateName, baseURL string) string {
	return fmt.Sprintf(`# API Documentation

## Template Error
The documentation template for '%s' could not be loaded.

## Base URL
%s

## Note
Please ensure all template files are properly embedded and accessible.`, templateName, baseURL)
}

// Global renderer instance
var globalRenderer *DocumentationRenderer

// init initializes the global renderer
func init() {
	var err error
	globalRenderer, err = NewDocumentationRenderer()
	if err != nil {
		// Fall back to original implementation if template loading fails
		globalRenderer = nil
	}
}

// Helper function for fallback when renderer is not available
func getFallbackDocumentation(templateName, baseURL string) string {
	return fmt.Sprintf(`# API Documentation

## Template Error
The documentation template for '%s' could not be loaded.

## Base URL
%s

## Note
Please ensure all template files are properly embedded and accessible.`, templateName, baseURL)
}

// Template-based documentation generation functions
func generateChatCompletionsDocumentationFromTemplate(baseURL string) string {
	if globalRenderer == nil {
		return getFallbackDocumentation("chat_completions", baseURL)
	}

	doc, err := globalRenderer.RenderDocumentation("chat_completions", TemplateData{BaseURL: baseURL})
	if err != nil {
		return globalRenderer.fallbackToOriginal("chat_completions", baseURL)
	}
	return doc
}

func generateCompletionsDocumentationFromTemplate(baseURL string) string {
	if globalRenderer == nil {
		return getFallbackDocumentation("completions", baseURL)
	}

	doc, err := globalRenderer.RenderDocumentation("completions", TemplateData{BaseURL: baseURL})
	if err != nil {
		return globalRenderer.fallbackToOriginal("completions", baseURL)
	}
	return doc
}

func generateEmbeddingsDocumentationFromTemplate(baseURL string) string {
	if globalRenderer == nil {
		return getFallbackDocumentation("embeddings", baseURL)
	}

	doc, err := globalRenderer.RenderDocumentation("embeddings", TemplateData{BaseURL: baseURL})
	if err != nil {
		return globalRenderer.fallbackToOriginal("embeddings", baseURL)
	}
	return doc
}

func generateImagesDocumentationFromTemplate(baseURL string) string {
	if globalRenderer == nil {
		return getFallbackDocumentation("images", baseURL)
	}

	doc, err := globalRenderer.RenderDocumentation("images", TemplateData{BaseURL: baseURL})
	if err != nil {
		return globalRenderer.fallbackToOriginal("images", baseURL)
	}
	return doc
}

func generateAudioTranscriptionsDocumentationFromTemplate(baseURL string) string {
	if globalRenderer == nil {
		return getFallbackDocumentation("audio_transcriptions", baseURL)
	}

	doc, err := globalRenderer.RenderDocumentation("audio_transcriptions", TemplateData{BaseURL: baseURL})
	if err != nil {
		return globalRenderer.fallbackToOriginal("audio_transcriptions", baseURL)
	}
	return doc
}

func generateAudioTranslationsDocumentationFromTemplate(baseURL string) string {
	if globalRenderer == nil {
		return getFallbackDocumentation("audio_translations", baseURL)
	}

	doc, err := globalRenderer.RenderDocumentation("audio_translations", TemplateData{BaseURL: baseURL})
	if err != nil {
		return globalRenderer.fallbackToOriginal("audio_translations", baseURL)
	}
	return doc
}

func generateAudioSpeechDocumentationFromTemplate(baseURL string) string {
	if globalRenderer == nil {
		return getFallbackDocumentation("audio_speech", baseURL)
	}

	doc, err := globalRenderer.RenderDocumentation("audio_speech", TemplateData{BaseURL: baseURL})
	if err != nil {
		return globalRenderer.fallbackToOriginal("audio_speech", baseURL)
	}
	return doc
}

func generateModerationsDocumentationFromTemplate(baseURL string) string {
	if globalRenderer == nil {
		return getFallbackDocumentation("moderations", baseURL)
	}

	doc, err := globalRenderer.RenderDocumentation("moderations", TemplateData{BaseURL: baseURL})
	if err != nil {
		return globalRenderer.fallbackToOriginal("moderations", baseURL)
	}
	return doc
}

func generateModelsListDocumentationFromTemplate(baseURL string) string {
	if globalRenderer == nil {
		return getFallbackDocumentation("models_list", baseURL)
	}

	doc, err := globalRenderer.RenderDocumentation("models_list", TemplateData{BaseURL: baseURL})
	if err != nil {
		return globalRenderer.fallbackToOriginal("models_list", baseURL)
	}
	return doc
}

func generateClaudeMessagesDocumentationFromTemplate(baseURL string) string {
	if globalRenderer == nil {
		return getFallbackDocumentation("claude_messages", baseURL)
	}

	doc, err := globalRenderer.RenderDocumentation("claude_messages", TemplateData{BaseURL: baseURL})
	if err != nil {
		return globalRenderer.fallbackToOriginal("claude_messages", baseURL)
	}
	return doc
}
