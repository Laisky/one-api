package mcp

import (
	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/songquanpeng/one-api/common/config"
)

// Server wraps the official MCP SDK server
type Server struct {
	server *mcp.Server
}

// NewServer creates a new MCP server using the official SDK
func NewServer() *Server {
	// Create the MCP server with implementation info
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "one-api-official-mcp",
		Version: "1.0.0",
	}, nil)

	Server := &Server{
		server: server,
	}

	// Add tools for each One-API relay endpoint
	Server.addRelayAPITools()

	return Server
}

// getBaseURL returns the base URL for API documentation
func getBaseURL() string {
	if config.ServerAddress != "" {
		return config.ServerAddress
	}
	return "https://your-one-api-host" // fallback
}

// Handler handles MCP requests through HTTP (bridge between HTTP and MCP SDK)
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
