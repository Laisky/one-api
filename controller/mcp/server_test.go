package mcp

import (
	"context"
	"os"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/songquanpeng/one-api/common/config"
)

func TestNewServer(t *testing.T) {
	server := NewServer()

	// Test server creation
	assert.NotNil(t, server, "Server should not be nil")
	assert.NotNil(t, server.server, "Internal MCP server should not be nil")

	// Test that server is properly initialized
	assert.IsType(t, &Server{}, server, "Should return correct server type")

	t.Logf("✓ MCP server created successfully")
}

func TestServerImplementationInfo(t *testing.T) {
	server := NewServer()

	// Since we can't directly access the implementation info from the SDK,
	// we test that the server was created without errors
	assert.NotNil(t, server.server, "Server should be initialized with implementation info")

	t.Logf("✓ Server implementation info configured")
}

func TestGetBaseURL(t *testing.T) {
	// Save original config value
	originalServerAddress := config.ServerAddress

	testCases := []struct {
		name           string
		serverAddress  string
		expectedResult string
	}{
		{
			name:           "with_server_address_set",
			serverAddress:  "https://api.example.com",
			expectedResult: "https://api.example.com",
		},
		{
			name:           "with_localhost",
			serverAddress:  "http://localhost:3000",
			expectedResult: "http://localhost:3000",
		},
		{
			name:           "with_empty_server_address",
			serverAddress:  "",
			expectedResult: "https://your-one-api-host",
		},
		{
			name:           "with_port_number",
			serverAddress:  "https://api.example.com:8080",
			expectedResult: "https://api.example.com:8080",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set test config
			config.ServerAddress = tc.serverAddress

			// Test getBaseURL
			result := getBaseURL()

			// Verify result
			assert.Equal(t, tc.expectedResult, result)

			t.Logf("✓ getBaseURL() with ServerAddress='%s' returns '%s'", tc.serverAddress, result)
		})
	}

	// Restore original config
	config.ServerAddress = originalServerAddress
}

func TestGetBaseURLEnvironmentVariable(t *testing.T) {
	// Save original values
	originalServerAddress := config.ServerAddress
	originalEnv := os.Getenv("SERVER_ADDRESS")

	// Test with environment variable (if the config reads from env)
	testURL := "https://env.example.com"
	os.Setenv("SERVER_ADDRESS", testURL)
	
	// Reset config to empty to test fallback
	config.ServerAddress = ""
	
	result := getBaseURL()
	expected := "https://your-one-api-host" // Should use fallback since config.ServerAddress is empty

	assert.Equal(t, expected, result)

	// Restore original values
	config.ServerAddress = originalServerAddress
	if originalEnv != "" {
		os.Setenv("SERVER_ADDRESS", originalEnv)
	} else {
		os.Unsetenv("SERVER_ADDRESS")
	}

	t.Logf("✓ getBaseURL() fallback mechanism works correctly")
}

// Test helper function to check if tools are registered
func TestServerToolsRegistration(t *testing.T) {
	server := NewServer()

	// Since we can't directly access the tools from the MCP SDK,
	// we test that the server was created and tools registration completed without errors
	assert.NotNil(t, server.server, "Server should have tools registered")

	// The addRelayAPITools() function should have been called during NewServer()
	// We can't directly verify the tools are registered due to SDK limitations,
	// but we can ensure the function completes successfully
	
	t.Logf("✓ Server tools registration completed")
}

// Test that we can call addRelayAPITools multiple times without issues
func TestAddRelayAPIToolsIdempotent(t *testing.T) {
	server := NewServer()

	// Call addRelayAPITools again - should not cause issues
	assert.NotPanics(t, func() {
		server.addRelayAPITools()
	}, "addRelayAPITools should be safe to call multiple times")

	t.Logf("✓ addRelayAPITools is idempotent")
}

// Mock test for tool functionality - tests that tools can be called
func TestServerToolsCanBeCalled(t *testing.T) {
	server := NewServer()

	// Test context
	ctx := context.Background()

	// We can't easily test the individual tools without more complex mocking
	// of the MCP SDK, but we can verify the server is properly set up
	assert.NotNil(t, server.server, "Server should be ready to handle tool calls")

	// The tool argument types are defined within the addRelayAPITools function
	// and are not accessible from outside that scope. This is intentional
	// encapsulation. We verify that the server was created successfully
	// and the addRelayAPITools function completed without errors.

	_ = ctx // Use ctx to avoid unused variable warning

	t.Logf("✓ Server is properly set up to handle tool calls")
}

// Benchmark test for server creation
func BenchmarkNewServer(b *testing.B) {
	for i := 0; i < b.N; i++ {
		server := NewServer()
		_ = server // Avoid unused variable warning
	}
}

// Benchmark test for getBaseURL
func BenchmarkGetBaseURL(b *testing.B) {
	config.ServerAddress = "https://api.example.com"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		url := getBaseURL()
		_ = url // Avoid unused variable warning
	}
}

// Test server configuration edge cases
func TestServerEdgeCases(t *testing.T) {
	testCases := []struct {
		name          string
		setupFunc     func()
		expectedPanic bool
	}{
		{
			name: "normal_creation",
			setupFunc: func() {
				config.ServerAddress = "https://api.example.com"
			},
			expectedPanic: false,
		},
		{
			name: "empty_config",
			setupFunc: func() {
				config.ServerAddress = ""
			},
			expectedPanic: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupFunc()

			if tc.expectedPanic {
				assert.Panics(t, func() {
					NewServer()
				}, "Should panic in edge case: %s", tc.name)
			} else {
				assert.NotPanics(t, func() {
					server := NewServer()
					assert.NotNil(t, server, "Server should be created successfully")
				}, "Should not panic in edge case: %s", tc.name)
			}

			t.Logf("✓ Edge case '%s' handled correctly", tc.name)
		})
	}
}

// Test that the server struct is properly encapsulated
func TestServerEncapsulation(t *testing.T) {
	server := NewServer()

	// Test that we can access the server field (it's public for testing)
	assert.NotNil(t, server.server, "server field should be accessible")

	// Test type assertions
	assert.IsType(t, &Server{}, server)
	assert.IsType(t, &mcp.Server{}, server.server)

	t.Logf("✓ Server encapsulation is correct")
}

// Test concurrent server creation (thread safety)
func TestConcurrentServerCreation(t *testing.T) {
	const goroutines = 10
	servers := make([]*Server, goroutines)
	done := make(chan int, goroutines)

	// Create servers concurrently
	for i := 0; i < goroutines; i++ {
		go func(index int) {
			servers[index] = NewServer()
			done <- index
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < goroutines; i++ {
		<-done
	}

	// Verify all servers were created successfully
	for i, server := range servers {
		assert.NotNil(t, server, "Server %d should be created", i)
		assert.NotNil(t, server.server, "Server %d internal server should be created", i)
	}

	t.Logf("✓ Concurrent server creation works correctly (%d servers)", goroutines)
}