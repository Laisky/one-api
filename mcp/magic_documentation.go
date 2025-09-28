package mcp

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// TemplateData holds the data structure used for rendering documentation templates.
// It contains the base URL and optional parameters from tool calls that will be
// substituted into the template placeholders.
type TemplateData struct {
	BaseURL    string         // The base URL for the API endpoint (e.g., "https://api.example.com")
	Parameters map[string]any // Parameters passed from the MCP tool call
}

// DocumentationType represents the available documentation types that can be generated.
// Each type corresponds to a specific API endpoint and its associated template.
type DocumentationType string

// Supported documentation types for various API endpoints.
const (
	ChatCompletions     DocumentationType = "chat_completions"     // OpenAI Chat Completions API
	Completions         DocumentationType = "completions"          // OpenAI Completions API
	Embeddings          DocumentationType = "embeddings"           // OpenAI Embeddings API
	Images              DocumentationType = "images"               // OpenAI Image Generation API
	AudioTranscriptions DocumentationType = "audio_transcriptions" // OpenAI Audio Transcriptions API
	AudioTranslations   DocumentationType = "audio_translations"   // OpenAI Audio Translations API
	AudioSpeech         DocumentationType = "audio_speech"         // OpenAI Audio Speech API
	Moderations         DocumentationType = "moderations"          // OpenAI Moderations API
	ModelsList          DocumentationType = "models_list"          // Models List API
	ClaudeMessages      DocumentationType = "claude_messages"      // Claude Messages API
)

// InstructionRenderer handles template-based instruction generation with
// automatic template discovery and caching. It provides a centralized system
// for rendering MCP server instructions from magic embedded Go templates.
type InstructionRenderer struct {
	templates map[string]*template.Template // Cache of parsed instruction templates
	registry  map[InstructionType]string    // Maps instruction types to template names
}

// DocumentationRenderer handles template-based documentation generation with
// automatic template discovery and caching. It provides a centralized system
// for rendering API documentation from magic embedded Go templates.
type DocumentationRenderer struct {
	templates           map[string]*template.Template // Cache of parsed templates
	registry            map[DocumentationType]string  // Maps documentation types to template names
	instructionRenderer *InstructionRenderer          // Embedded instruction renderer
}

// NewDocumentationRenderer creates a new template-based documentation renderer
// with automatic template loading and registry initialization.
//
// It scans the magic embedded template filesystem, loads all available templates,
// and initializes the type-to-template registry for efficient lookups.
//
// Returns an error if template parsing fails or the templates directory
// cannot be accessed.
func NewDocumentationRenderer() (*DocumentationRenderer, error) {
	// Create instruction renderer
	instructionRenderer, err := NewInstructionRenderer()
	if err != nil {
		return nil, fmt.Errorf("failed to create instruction renderer: %w", err)
	}

	renderer := &DocumentationRenderer{
		templates:           make(map[string]*template.Template),
		registry:            make(map[DocumentationType]string),
		instructionRenderer: instructionRenderer,
	}

	// Initialize the registry with available documentation types
	renderer.initializeRegistry()

	// Load all template files dynamically
	if err := renderer.loadTemplates(); err != nil {
		return nil, err
	}

	return renderer, nil
}

// loadTemplates dynamically discovers and loads all available template files
// from the magic embedded filesystem. It parses each .tmpl file (except api_base.tmpl)
// and stores the compiled templates in the renderer's cache.
//
// Template files are expected to be in the "docs/templates/" directory and
// have a .tmpl extension. The api_base.tmpl file is skipped as it serves
// as a base template for composition.
//
// Returns an error if the templates directory cannot be read or if any
// template fails to parse.
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

// GenerateDocumentation renders documentation for the specified documentation type
// using the appropriate template and the provided base URL.
//
// It looks up the template associated with the documentation type, executes it
// with the provided base URL, and returns the rendered documentation as a string.
//
// Parameters:
//   - docType: The type of documentation to generate (e.g., ChatCompletions, Embeddings)
//   - baseURL: The base URL to substitute in the template (e.g., "https://api.example.com")
//
// Returns the rendered documentation string, or an error if the documentation type
// is unknown or template execution fails. Falls back to generic documentation
// if the specific template is not available.
func (r *DocumentationRenderer) GenerateDocumentation(docType DocumentationType, baseURL string) (string, error) {
	return r.GenerateDocumentationWithParams(docType, baseURL, nil)
}

// GenerateDocumentationWithParams renders documentation for the specified documentation type
// using the appropriate template, base URL, and optional parameters from tool calls.
//
// It looks up the template associated with the documentation type, executes it
// with the provided data, and returns the rendered documentation as a string.
//
// Parameters:
//   - docType: The type of documentation to generate (e.g., ChatCompletions, Embeddings)
//   - baseURL: The base URL to substitute in the template (e.g., "https://api.example.com")
//   - parameters: Optional parameters from MCP tool calls to include in examples
//
// Returns the rendered documentation string, or an error if the documentation type
// is unknown or template execution fails. Falls back to generic documentation
// if the specific template is not available.
func (r *DocumentationRenderer) GenerateDocumentationWithParams(docType DocumentationType, baseURL string, parameters map[string]any) (string, error) {
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
	data := TemplateData{
		BaseURL:    baseURL,
		Parameters: parameters,
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", templateName, err)
	}

	return buf.String(), nil
}

// GetAvailableTypes returns a slice of all available documentation types
// that are registered in the renderer. This can be used to discover what
// documentation types are supported without attempting to generate them.
//
// The returned slice contains all DocumentationType constants that have
// been registered in the type-to-template mapping.
func (r *DocumentationRenderer) GetAvailableTypes() []DocumentationType {
	types := make([]DocumentationType, 0, len(r.registry))
	for docType := range r.registry {
		types = append(types, docType)
	}
	return types
}

// IsTypeSupported checks whether a specific documentation type is supported
// by this renderer instance. It returns true if the type is registered in
// the type-to-template mapping, false otherwise.
//
// This is useful for validating documentation types before attempting to
// generate documentation, avoiding unnecessary error handling.
//
// Parameters:
//   - docType: The documentation type to check for support
//
// Returns true if the documentation type is supported, false otherwise.
func (r *DocumentationRenderer) IsTypeSupported(docType DocumentationType) bool {
	_, exists := r.registry[docType]
	return exists
}

// generateFallbackDocumentation provides a generic fallback documentation
// when the specific template for a documentation type is not available.
//
// This ensures that the system gracefully degrades and always returns
// some form of documentation, even if the specific template is missing
// or fails to load.
//
// Parameters:
//   - docType: The name of the documentation type that failed to load
//   - baseURL: The base URL to include in the fallback documentation
//
// Returns a formatted string containing basic API documentation with
// an error message explaining the template issue.
func (r *DocumentationRenderer) generateFallbackDocumentation(docType, baseURL string) string {
	return fmt.Sprintf(`# API Documentation

## Template Error
The documentation template for '%s' could not be loaded.

## Base URL
%s

## Note
Please ensure all template files are properly magic embedded and accessible.`, docType, baseURL)
}

// GenerateDocumentation is the main entry point for generating API documentation
// using the global renderer instance. This function replaces all the individual
// generate*DocumentationFromTemplate functions with a unified, type-safe interface.
//
// It uses the global renderer to generate documentation for the specified type
// and base URL. If the global renderer is not available or template execution
// fails, it falls back to generic documentation to ensure the system remains
// functional.
//
// This is the recommended way to generate documentation as it provides:
//   - Type safety through the DocumentationType enum
//   - Automatic template discovery and caching
//   - Graceful error handling with fallback documentation
//   - Consistent API across all documentation types
//
// Parameters:
//   - docType: The type of documentation to generate (e.g., ChatCompletions, Embeddings)
//   - baseURL: The base URL for the API endpoint (e.g., "https://api.example.com")
//
// Returns a string containing the rendered API documentation. Always returns
// valid documentation, even if the specific template fails to load.
//
// Example:
//
//	doc := GenerateDocumentation(ChatCompletions, "https://api.example.com")
//	fmt.Println(doc) // Prints complete Chat Completions API documentation
func GenerateDocumentation(docType DocumentationType, baseURL string) string {
	return GenerateDocumentationWithParams(docType, baseURL, nil)
}

// GenerateDocumentationWithParams is the main entry point for generating API documentation
// with parameters using the global renderer instance. This function extends the basic
// GenerateDocumentation function to include parameters from MCP tool calls.
//
// It uses the global renderer to generate documentation for the specified type,
// base URL, and optional parameters. If the global renderer is not available or template
// execution fails, it falls back to generic documentation to ensure the system remains
// functional.
//
// This function provides:
//   - Type safety through the DocumentationType enum
//   - Automatic template discovery and caching
//   - Parameter inclusion in documentation examples
//   - Graceful error handling with fallback documentation
//   - Consistent API across all documentation types
//
// Parameters:
//   - docType: The type of documentation to generate (e.g., ChatCompletions, Embeddings)
//   - baseURL: The base URL for the API endpoint (e.g., "https://api.example.com")
//   - parameters: Optional parameters from MCP tool calls to include in examples
//
// Returns a string containing the rendered API documentation. Always returns
// valid documentation, even if the specific template fails to load.
//
// Example:
//
//	params := map[string]any{"model": "gpt-4", "max_tokens": 100}
//	doc := GenerateDocumentationWithParams(ChatCompletions, "https://api.example.com", params)
//	fmt.Println(doc) // Prints complete Chat Completions API documentation with examples
func GenerateDocumentationWithParams(docType DocumentationType, baseURL string, parameters map[string]any) string {
	if globalRenderer == nil {
		return getFallbackDocumentation(string(docType), baseURL)
	}

	doc, err := globalRenderer.GenerateDocumentationWithParams(docType, baseURL, parameters)
	if err != nil {
		return globalRenderer.generateFallbackDocumentation(string(docType), baseURL)
	}

	return doc
}

// getFallbackDocumentation is a helper function that provides fallback documentation
// when the global renderer is not available or fails to initialize.
//
// This ensures that the system can still provide basic documentation even in
// error conditions, maintaining system reliability and user experience.
//
// Parameters:
//   - templateName: The name of the template that could not be loaded
//   - baseURL: The base URL to include in the fallback documentation
//
// Returns a formatted string containing basic error documentation that
// explains the template loading issue and provides the base URL.
func getFallbackDocumentation(templateName, baseURL string) string {
	return fmt.Sprintf(`# API Documentation

## Template Error
The documentation template for '%s' could not be loaded.

## Base URL
%s

## Note
Please ensure all template files are properly magic embedded and accessible.`, templateName, baseURL)
}

// NewInstructionRenderer creates a new template-based instruction renderer
// with automatic template loading and registry initialization.
//
// It scans the magic embedded template filesystem for instruction templates,
// loads all available templates, and initializes the type-to-template registry
// for efficient lookups.
//
// Returns an error if template parsing fails or the templates directory
// cannot be accessed.
func NewInstructionRenderer() (*InstructionRenderer, error) {
	renderer := &InstructionRenderer{
		templates: make(map[string]*template.Template),
		registry:  make(map[InstructionType]string),
	}

	// Initialize the registry with available instruction types
	renderer.initializeInstructionRegistry()

	// Load all instruction template files dynamically
	if err := renderer.loadInstructionTemplates(); err != nil {
		return nil, err
	}

	return renderer, nil
}

// loadInstructionTemplates dynamically discovers and loads all available instruction template files
// from the magic embedded filesystem. It parses each instruction .tmpl file
// and stores the compiled templates in the renderer's cache.
//
// Instruction template files are expected to be in the "docs/templates/instructions/" directory
// and have a .tmpl extension.
//
// Returns an error if the templates directory cannot be read or if any
// template fails to parse.
func (r *InstructionRenderer) loadInstructionTemplates() error {
	// Get list of instruction template files
	entries, err := templateFS.ReadDir("docs/templates/instructions")
	if err != nil {
		// If instructions directory doesn't exist, that's okay - just return without error
		return nil
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".tmpl") {
			templateName := strings.TrimSuffix(entry.Name(), ".tmpl")

			tmplContent, err := templateFS.ReadFile("docs/templates/instructions/" + entry.Name())
			if err != nil {
				// Log warning but continue with other templates
				continue
			}

			tmpl, err := template.New(templateName).Parse(string(tmplContent))
			if err != nil {
				return fmt.Errorf("failed to parse instruction template %s: %w", entry.Name(), err)
			}

			r.templates[templateName] = tmpl
		}
	}

	return nil
}

// GenerateInstructions renders instructions for the specified instruction type
// using the appropriate template and the provided template data.
//
// It looks up the template associated with the instruction type, executes it
// with the provided template data, and returns the rendered instructions as a string.
//
// Parameters:
//   - instructionType: The type of instructions to generate (e.g., GeneralInstructions, ToolUsageInstructions)
//   - data: The template data to substitute in the template
//
// Returns the rendered instructions string, or an error if the instruction type
// is unknown or template execution fails. Falls back to generic instructions
// if the specific template is not available.
func (r *InstructionRenderer) GenerateInstructions(instructionType InstructionType, data InstructionTemplateData) (string, error) {
	templateName, exists := r.registry[instructionType]
	if !exists {
		return "", fmt.Errorf("unknown instruction type: %s", instructionType)
	}

	tmpl, exists := r.templates[templateName]
	if !exists {
		// Fallback to generic instructions if template doesn't exist
		return r.generateFallbackInstructions(string(instructionType), data), nil
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute instruction template %s: %w", templateName, err)
	}

	return buf.String(), nil
}

// GetAvailableInstructionTypes returns a slice of all available instruction types
// that are registered in the renderer.
func (r *InstructionRenderer) GetAvailableInstructionTypes() []InstructionType {
	types := make([]InstructionType, 0, len(r.registry))
	for instructionType := range r.registry {
		types = append(types, instructionType)
	}
	return types
}

// IsInstructionTypeSupported checks whether a specific instruction type is supported
// by this renderer instance.
func (r *InstructionRenderer) IsInstructionTypeSupported(instructionType InstructionType) bool {
	_, exists := r.registry[instructionType]
	return exists
}

// generateFallbackInstructions provides generic fallback instructions
// when the specific template for an instruction type is not available.
func (r *InstructionRenderer) generateFallbackInstructions(instructionType string, data InstructionTemplateData) string {
	return fmt.Sprintf(`# MCP Server Instructions

## Server Information
- **Name**: %s
- **Version**: %s
- **Base URL**: %s

## Template Error
The instruction template for '%s' could not be loaded.

## Available Tools
%s

## Note
Please ensure all instruction template files are properly magic embedded and accessible.`,
		data.ServerName, data.ServerVersion, data.BaseURL, instructionType,
		strings.Join(data.AvailableTools, ", "))
}

// GetInstructionRenderer returns the embedded instruction renderer.
// This allows access to instruction generation capabilities from the documentation renderer.
func (r *DocumentationRenderer) GetInstructionRenderer() *InstructionRenderer {
	return r.instructionRenderer
}
