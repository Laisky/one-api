package router

import (
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/mcp"
)

func TestMCPSingletonPattern(t *testing.T) {
	// Reset the singleton for testing
	mcpServerInstance = nil
	mcpHandlerInstance = nil
	mcpOnce = sync.Once{}

	// Create separate test routers to avoid route conflicts
	gin.SetMode(gin.TestMode)
	router1 := gin.New()
	router2 := gin.New()

	// Call SetApiRouter on first router
	SetApiRouter(router1)
	firstInstance := mcpServerInstance
	firstHandler := mcpHandlerInstance

	// Call SetApiRouter on second router
	SetApiRouter(router2)
	secondInstance := mcpServerInstance
	secondHandler := mcpHandlerInstance

	// Verify that the same instances are reused
	if firstInstance != secondInstance {
		t.Error("MCP server instance should be the same (singleton pattern)")
	}

	if firstHandler == nil || secondHandler == nil {
		t.Error("MCP handler instances should not be nil")
	}

	// Verify instances are not nil
	if firstInstance == nil {
		t.Error("MCP server instance should not be nil")
	}

	t.Log("✓ MCP singleton pattern works correctly - same instances reused")
}

func TestInitMCPServerIdempotent(t *testing.T) {
	// Reset the singleton for testing
	mcpServerInstance = nil
	mcpHandlerInstance = nil
	mcpOnce = sync.Once{}

	// Call initMCPServer multiple times
	initMCPServer()
	firstInstance := mcpServerInstance
	firstHandler := mcpHandlerInstance

	initMCPServer()
	secondInstance := mcpServerInstance
	secondHandler := mcpHandlerInstance

	initMCPServer()
	thirdInstance := mcpServerInstance
	thirdHandler := mcpHandlerInstance

	// All instances should be the same
	if firstInstance != secondInstance || secondInstance != thirdInstance {
		t.Error("initMCPServer should be idempotent - same instances should be returned")
	}

	if firstInstance == nil || secondInstance == nil || thirdInstance == nil {
		t.Error("MCP server instances should not be nil")
	}

	if firstHandler == nil || secondHandler == nil || thirdHandler == nil {
		t.Error("MCP handler instances should not be nil")
	}

	t.Log("✓ initMCPServer is idempotent - creates instances only once")
}

func TestMCPServerType(t *testing.T) {
	// Reset the singleton for testing
	mcpServerInstance = nil
	mcpHandlerInstance = nil
	mcpOnce = sync.Once{}

	// Initialize MCP server
	initMCPServer()

	// Verify the server instance is of correct type
	if mcpServerInstance == nil {
		t.Fatal("MCP server instance should not be nil")
	}

	// Verify it's the expected type
	if _, ok := any(mcpServerInstance).(*mcp.Server); !ok {
		t.Error("MCP server instance should be of type *mcp.Server")
	}

	// Verify handler is of correct type
	if mcpHandlerInstance == nil {
		t.Fatal("MCP handler instance should not be nil")
	}

	if _, ok := any(mcpHandlerInstance).(gin.HandlerFunc); !ok {
		t.Error("MCP handler instance should be of type gin.HandlerFunc")
	}

	t.Log("✓ MCP server and handler are of correct types")
}
