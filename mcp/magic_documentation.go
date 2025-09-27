package mcp

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed docs/templates/*.tmpl
var templateFS embed.FS

// TemplateData holds the data for rendering documentation templates
type TemplateData struct {
	BaseURL string
}

// DocumentationType represents the available documentation types
type DocumentationType string

const (
	ChatCompletions     DocumentationType = "chat_completions"
	Completions         DocumentationType = "completions"
	Embeddings          DocumentationType = "embeddings"
	Images              DocumentationType = "images"
	AudioTranscriptions DocumentationType = "audio_transcriptions"
	AudioTranslations   DocumentationType = "audio_translations"
	AudioSpeech         DocumentationType = "audio_speech"
	Moderations         DocumentationType = "moderations"
	ModelsList          DocumentationType = "models_list"
	ClaudeMessages      DocumentationType = "claude_messages"
)

// DocumentationRenderer handles template-based documentation generation
type DocumentationRenderer struct {
	templates map[string]*template.Template
	registry  map[DocumentationType]string // Maps doc type to template name
}

// NewDocumentationRenderer creates a new template-based documentation renderer
func NewDocumentationRenderer() (*DocumentationRenderer, error) {
	renderer := &DocumentationRenderer{
		templates: make(map[string]*template.Template),
		registry:  make(map[DocumentationType]string),
	}

	// Initialize the registry with available documentation types
	renderer.initializeRegistry()

	// Load all template files dynamically
	if err := renderer.loadTemplates(); err != nil {
		return nil, err
	}

	return renderer, nil
}

// loadTemplates dynamically loads all available template files
func (r *DocumentationRenderer) loadTemplates() error {
	// Get list of template files
	entries, err := templateFS.ReadDir("docs/templates")
	if err != nil {
		return fmt.Errorf("failed to read templates directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".tmpl") {
			templateName := strings.TrimSuffix(entry.Name(), ".tmpl")

			// Skip api_base.tmpl as it's a base template
			if templateName == "api_base" {
				continue
			}

			tmplContent, err := templateFS.ReadFile("docs/templates/" + entry.Name())
			if err != nil {
				// Log warning but continue with other templates
				continue
			}

			tmpl, err := template.New(templateName).Parse(string(tmplContent))
			if err != nil {
				return fmt.Errorf("failed to parse template %s: %w", entry.Name(), err)
			}

			r.templates[templateName] = tmpl
		}
	}

	return nil
}

// GenerateDocumentation renders documentation for the specified type
func (r *DocumentationRenderer) GenerateDocumentation(docType DocumentationType, baseURL string) (string, error) {
	templateName, exists := r.registry[docType]
	if !exists {
		return "", fmt.Errorf("unknown documentation type: %s", docType)
	}

	tmpl, exists := r.templates[templateName]
	if !exists {
		// Fallback to generic documentation if template doesn't exist
		return r.generateFallbackDocumentation(string(docType), baseURL), nil
	}

	var buf bytes.Buffer
	data := TemplateData{BaseURL: baseURL}

	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", templateName, err)
	}

	return buf.String(), nil
}

// GetAvailableTypes returns all available documentation types
func (r *DocumentationRenderer) GetAvailableTypes() []DocumentationType {
	types := make([]DocumentationType, 0, len(r.registry))
	for docType := range r.registry {
		types = append(types, docType)
	}
	return types
}

// IsTypeSupported checks if a documentation type is supported
func (r *DocumentationRenderer) IsTypeSupported(docType DocumentationType) bool {
	_, exists := r.registry[docType]
	return exists
}

// generateFallbackDocumentation provides a generic fallback when templates are not available
func (r *DocumentationRenderer) generateFallbackDocumentation(docType, baseURL string) string {
	return fmt.Sprintf(`# API Documentation

## Template Error
The documentation template for '%s' could not be loaded.

## Base URL
%s

## Note
Please ensure all template files are properly embedded and accessible.`, docType, baseURL)
}

// GenerateDocumentation is the main entry point for generating documentation
// This replaces all the individual generate*DocumentationFromTemplate functions
func GenerateDocumentation(docType DocumentationType, baseURL string) string {
	if globalRenderer == nil {
		return getFallbackDocumentation(string(docType), baseURL)
	}

	doc, err := globalRenderer.GenerateDocumentation(docType, baseURL)
	if err != nil {
		return globalRenderer.generateFallbackDocumentation(string(docType), baseURL)
	}

	return doc
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
