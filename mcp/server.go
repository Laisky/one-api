package mcp

import (
	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/songquanpeng/one-api/common/config"
)

// Server wraps the official MCP SDK server and provides a high-level interface
// for creating and managing MCP servers with One-API relay tools.
//
// The Server struct encapsulates the MCP SDK server instance and automatically
// registers all available One-API relay tools during initialization.
type Server struct {
	server  *mcp.Server    // The underlying MCP SDK server instance
	options *ServerOptions // Server configuration options
}

// NewServer creates a new MCP server instance using the official MCP SDK.
// It initializes the server with One-API implementation information and
// automatically registers all available relay API tools.
//
// The server is configured with:
//   - Name: "one-api-official-mcp"
//   - Version: "1.0.0"
//   - All One-API relay endpoint tools (chat completions, embeddings, etc.)
//
// Returns a fully configured Server instance ready to handle MCP requests.
// The server includes tools for all supported API endpoints including OpenAI-compatible
// APIs and Claude messages.
//
// Example:
//
//	server := NewServer()
//	// Server is now ready to handle MCP protocol requests
func NewServer() *Server {
	return NewServerWithOptions(DefaultServerOptions())
}

// NewServerWithOptions creates a new MCP server instance with custom options.
// It allows full customization of server behavior, instructions, and configuration.
//
// The server can be configured with:
//   - Custom name and version
//   - Instruction templates and custom instructions
//   - Custom base URL for documentation
//   - Additional template data
//
// Returns a fully configured Server instance ready to handle MCP requests.
// The server includes tools for all supported API endpoints and optional
// instruction generation capabilities.
//
// Example:
//
//	opts := DefaultServerOptions().
//		WithName("my-custom-mcp").
//		WithInstructionType(ToolUsageInstructions).
//		WithBaseURL("https://my-api.com")
//	server := NewServerWithOptions(opts)
func NewServerWithOptions(options *ServerOptions) *Server {
	// Validate options
	if err := options.Validate(); err != nil {
		// Use default options if validation fails
		options = DefaultServerOptions()
	}

	// Create the MCP server with implementation info
	server := mcp.NewServer(&mcp.Implementation{
		Name:    options.Name,
		Version: options.Version,
	}, nil)

	mcpServer := &Server{
		server:  server,
		options: options,
	}

	// Add tools for each One-API relay endpoint
	mcpServer.addRelayAPITools()

	// Add instruction tools if enabled
	if options.EnableInstructions {
		mcpServer.addInstructionTools()
	}

	return mcpServer
}

// getBaseURL returns the base URL for API documentation generation.
// It first checks the config.ServerAddress setting, and if not configured,
// falls back to a placeholder URL.
//
// This function is used by the documentation generation system to create
// complete endpoint URLs in the generated documentation.
//
// Returns:
//   - The configured server address from config.ServerAddress if available
//   - A fallback placeholder URL "https://your-one-api-host" if not configured
//
// Example:
//
//	baseURL := getBaseURL()
//	// Returns "https://api.example.com" if configured, or the fallback URL
func getBaseURL() string {
	if config.ServerAddress != "" {
		return config.ServerAddress
	}
	return "https://your-one-api-host" // fallback placeholder
}

// getEffectiveBaseURL returns the base URL to use for this server instance,
// considering both server options and global configuration.
func (s *Server) getEffectiveBaseURL() string {
	if s.options != nil && s.options.BaseURL != "" {
		return s.options.BaseURL
	}
	return getBaseURL()
}

// getAvailableToolNames returns a list of available tool names for this server.
func (s *Server) getAvailableToolNames() []string {
	tools := []string{
		"chat_completions",
		"completions",
		"embeddings",
		"images_generations",
		"audio_transcriptions",
		"audio_translations",
		"audio_speech",
		"moderations",
		"models_list",
		"claude_messages",
	}

	if s.options != nil && s.options.EnableInstructions {
		tools = append(tools, "instructions")
	}

	return tools
}

// Handler provides an HTTP endpoint for MCP server information and status.
// This is a bridge function that allows HTTP clients to get basic information
// about the MCP server capabilities without using the full MCP protocol.
//
// The handler returns a JSON response containing:
//   - message: Confirmation that the MCP server is available
//   - info: Description of the server implementation
//   - tools: Information about how to access the tools
//   - note: Explanation of HTTP limitations vs full MCP protocol
//
// This endpoint is useful for:
//   - Health checks and server status monitoring
//   - Discovery of MCP server capabilities
//   - Basic information for integration documentation
//
// Note: This handler provides limited functionality compared to the full MCP
// protocol. For complete access to all tools and features, clients should
// connect using a proper MCP client with the appropriate transport mechanism.
//
// Parameters:
//   - c: The Gin context for the HTTP request
//
// Response: JSON object with server information and status
func Handler(c *gin.Context) {
	// For now, we'll provide information about how to connect to the MCP server
	// In a full implementation, this would need to set up the appropriate transport
	// and handle the MCP protocol properly

	response := map[string]any{
		"message": "One-API Official MCP for Documentation is available",
		"info":    "This is an One-API Official MCP server implementation using the official Go SDK",
		"tools":   "Use an MCP client to connect and access available tools",
		"note":    "Direct HTTP access is limited - use MCP protocol for full functionality",
	}

	c.JSON(200, response)
}
