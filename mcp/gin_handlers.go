package mcp

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewGinStreamableHTTPHandler creates a Gin handler function that uses the MCP SDK's
// built-in StreamableHTTPHandler for proper MCP protocol handling.
//
// This function wraps the official MCP SDK's StreamableHTTPHandler to provide:
//   - Full MCP JSON-RPC protocol compliance
//   - Automatic delegation to registered MCP tools
//   - Built-in streaming support for long-running operations
//   - Integration with Gin middleware (auth, logging, etc.)
//
// The handler properly processes all MCP methods including:
//   - initialize: Server capabilities and information
//   - tools/list: List of available tools
//   - tools/call: Execute registered tools with real functionality
//   - resources/list: Available resources
//   - prompts/list: Available prompts
//
// Parameters:
//   - server: The MCP Server instance with registered tools
//
// Returns:
//   - gin.HandlerFunc: A Gin-compatible handler function
//
// Example usage in router:
//
//	mcpServer := mcp.NewServer()
//	handler := mcp.NewStreamableHTTPHandler(mcpServer)
//	apiRouter.POST("/mcp", handler)
func NewGinStreamableHTTPHandler(server *Server) gin.HandlerFunc {
	// Create the MCP SDK's StreamableHTTPHandler
	// This provides proper MCP protocol handling and delegates to registered tools
	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(req *http.Request) *mcp.Server {
			// Return the underlying MCP server instance
			// The SDK handler will process requests and call registered tools
			return server.server
		},
		nil, // Use default options
	)

	// Wrap the MCP handler in a Gin handler function
	return func(c *gin.Context) {
		// Delegate to the official MCP SDK handler
		// This ensures proper JSON-RPC protocol handling and tool execution
		mcpHandler.ServeHTTP(c.Writer, c.Request)
	}
}
