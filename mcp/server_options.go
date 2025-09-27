package mcp

import (
	"fmt"
)

// InstructionType represents the available instruction types that can be generated.
// Each type corresponds to a specific instruction template for MCP server guidance.
type InstructionType string

// Supported instruction types for various MCP server configurations.
const (
	GeneralInstructions       InstructionType = "general"        // General MCP server usage instructions
	ToolUsageInstructions     InstructionType = "tool_usage"     // Tool-specific usage instructions
	APIEndpointInstructions   InstructionType = "api_endpoints"  // API endpoint documentation instructions
	ErrorHandlingInstructions InstructionType = "error_handling" // Error handling and troubleshooting instructions
	BestPracticesInstructions InstructionType = "best_practices" // Best practices for using the MCP server
)

// ServerOptions contains configuration options for creating an MCP server instance.
// It allows customization of server behavior, instructions, and template rendering.
type ServerOptions struct {
	// Name is the server implementation name (defaults to "one-api-official-mcp")
	Name string

	// Version is the server implementation version (defaults to "1.0.0")
	Version string

	// Instructions contains custom instructions for the server
	Instructions *InstructionConfig

	// BaseURL overrides the default base URL for documentation generation
	BaseURL string

	// EnableInstructions determines whether instruction generation is enabled
	EnableInstructions bool

	// CustomTemplateData allows passing additional data to templates
	CustomTemplateData map[string]any
}

// InstructionConfig holds configuration for server instructions.
type InstructionConfig struct {
	// Type specifies which instruction template to use
	Type InstructionType

	// CustomInstructions provides custom instruction text (overrides template)
	CustomInstructions string

	// TemplateData contains data to be passed to instruction templates
	TemplateData map[string]any

	// EnableFallback determines whether to use fallback instructions if template fails
	EnableFallback bool
}

// InstructionTemplateData holds the data structure used for rendering instruction templates.
// It extends the basic TemplateData with instruction-specific fields.
type InstructionTemplateData struct {
	BaseURL        string         // The base URL for the API endpoint
	ServerName     string         // Name of the MCP server
	ServerVersion  string         // Version of the MCP server
	AvailableTools []string       // List of available tools
	CustomData     map[string]any // Custom data from ServerOptions
}

// DefaultServerOptions returns a ServerOptions instance with sensible defaults.
func DefaultServerOptions() *ServerOptions {
	return &ServerOptions{
		Name:               "one-api-official-mcp",
		Version:            "1.0.0",
		EnableInstructions: true,
		Instructions: &InstructionConfig{
			Type:           GeneralInstructions,
			EnableFallback: true,
			TemplateData:   make(map[string]any),
		},
		CustomTemplateData: make(map[string]any),
	}
}

// WithName sets the server name.
func (opts *ServerOptions) WithName(name string) *ServerOptions {
	opts.Name = name
	return opts
}

// WithVersion sets the server version.
func (opts *ServerOptions) WithVersion(version string) *ServerOptions {
	opts.Version = version
	return opts
}

// WithInstructions sets the instruction configuration.
func (opts *ServerOptions) WithInstructions(config *InstructionConfig) *ServerOptions {
	opts.Instructions = config
	opts.EnableInstructions = true
	return opts
}

// WithCustomInstructions sets custom instruction text directly.
func (opts *ServerOptions) WithCustomInstructions(instructions string) *ServerOptions {
	if opts.Instructions == nil {
		opts.Instructions = &InstructionConfig{
			EnableFallback: true,
			TemplateData:   make(map[string]any),
		}
	}
	opts.Instructions.CustomInstructions = instructions
	opts.EnableInstructions = true
	return opts
}

// WithInstructionType sets the instruction template type.
func (opts *ServerOptions) WithInstructionType(instructionType InstructionType) *ServerOptions {
	if opts.Instructions == nil {
		opts.Instructions = &InstructionConfig{
			EnableFallback: true,
			TemplateData:   make(map[string]any),
		}
	}
	opts.Instructions.Type = instructionType
	opts.EnableInstructions = true
	return opts
}

// WithBaseURL sets a custom base URL for documentation generation.
func (opts *ServerOptions) WithBaseURL(baseURL string) *ServerOptions {
	opts.BaseURL = baseURL
	return opts
}

// WithCustomTemplateData adds custom data that will be available in templates.
func (opts *ServerOptions) WithCustomTemplateData(key string, value any) *ServerOptions {
	if opts.CustomTemplateData == nil {
		opts.CustomTemplateData = make(map[string]any)
	}
	opts.CustomTemplateData[key] = value
	return opts
}

// DisableInstructions disables instruction generation for this server.
func (opts *ServerOptions) DisableInstructions() *ServerOptions {
	opts.EnableInstructions = false
	return opts
}

// Validate checks if the ServerOptions configuration is valid.
func (opts *ServerOptions) Validate() error {
	if opts.Name == "" {
		return fmt.Errorf("server name cannot be empty")
	}

	if opts.Version == "" {
		return fmt.Errorf("server version cannot be empty")
	}

	if opts.EnableInstructions && opts.Instructions != nil {
		if opts.Instructions.Type == "" && opts.Instructions.CustomInstructions == "" {
			return fmt.Errorf("instruction type or custom instructions must be specified when instructions are enabled")
		}
	}

	return nil
}

// GetEffectiveBaseURL returns the base URL to use, considering the options and fallbacks.
func (opts *ServerOptions) GetEffectiveBaseURL() string {
	if opts.BaseURL != "" {
		return opts.BaseURL
	}
	return getBaseURL() // Use the existing getBaseURL function as fallback
}
