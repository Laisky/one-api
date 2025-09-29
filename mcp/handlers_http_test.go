package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/stretchr/testify/assert"
)

func TestHandler(t *testing.T) {
	// Set test mode for gin
	gin.SetMode(gin.TestMode)

	// Create test router
	router := gin.New()
	router.GET("/mcp", Handler)
	router.POST("/mcp", Handler)

	testCases := []struct {
		name           string
		method         string
		expectedStatus int
	}{
		{
			name:           "GET_request",
			method:         "GET",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "POST_request",
			method:         "POST",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create request
			req, err := http.NewRequest(tc.method, "/mcp", nil)
			assert.NoError(t, err, "Should create request without error")

			// Create response recorder
			w := httptest.NewRecorder()

			// Perform request
			router.ServeHTTP(w, req)

			// Check status code
			assert.Equal(t, tc.expectedStatus, w.Code, "Should return correct status code")

			// Parse response
			var response map[string]any
			err = json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err, "Should parse JSON response successfully")

			// Check response structure
			assert.Contains(t, response, "message", "Response should contain message field")
			assert.Contains(t, response, "info", "Response should contain info field")
			assert.Contains(t, response, "tools", "Response should contain tools field")
			assert.Contains(t, response, "note", "Response should contain note field")

			// Check message content
			message, ok := response["message"].(string)
			assert.True(t, ok, "Message should be a string")
			assert.Contains(t, message, "One-API Official MCP", "Message should mention One-API MCP")

			t.Logf("✓ %s request handled correctly", tc.method)
		})
	}
}

func TestHandlerResponseContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/mcp", Handler)

	// Create request
	req, _ := http.NewRequest("GET", "/mcp", nil)
	w := httptest.NewRecorder()

	// Perform request
	router.ServeHTTP(w, req)

	// Parse response
	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// Test specific response content
	expectedFields := map[string]string{
		"message": "One-API Official MCP for Documentation is available",
		"info":    "This is an One-API Official MCP server implementation using the official Go SDK",
		"tools":   "Use an MCP client to connect and access available tools",
		"note":    "Direct HTTP access is limited - use MCP protocol for full functionality",
	}

	for field, expectedValue := range expectedFields {
		actualValue, exists := response[field]
		assert.True(t, exists, "Response should contain field: %s", field)
		assert.Equal(t, expectedValue, actualValue, "Field %s should have correct value", field)
	}

	t.Logf("✓ Handler response contains all expected content")
}

func TestHandlerWithDifferentContentTypes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/mcp", Handler)

	contentTypes := []string{
		"application/json",
		"text/plain",
		"application/xml",
		"",
	}

	for _, contentType := range contentTypes {
		t.Run("content_type_"+contentType, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/mcp", nil)
			if contentType != "" {
				req.Header.Set("Content-Type", contentType)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Should handle all content types the same way (returns info message)
			assert.Equal(t, http.StatusOK, w.Code)

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err, "Should parse JSON response regardless of request content type")

			assert.Contains(t, response, "message", "Should contain message field")

			t.Logf("✓ Handler works with content-type: %s", contentType)
		})
	}
}

func TestHandlerWithDifferentHTTPMethods(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Register handler for various methods
	router.GET("/mcp", Handler)
	router.POST("/mcp", Handler)
	router.PUT("/mcp", Handler)
	router.DELETE("/mcp", Handler)

	methods := []string{"GET", "POST", "PUT", "DELETE"}

	for _, method := range methods {
		t.Run("method_"+method, func(t *testing.T) {
			req, _ := http.NewRequest(method, "/mcp", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			// All methods should return the same response
			assert.Equal(t, http.StatusOK, w.Code, "Method %s should return 200", method)

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err, "Should parse JSON for method %s", method)

			assert.Contains(t, response, "message", "Should contain message for method %s", method)

			t.Logf("✓ Handler works with HTTP method: %s", method)
		})
	}
}

func TestHandlerJSONResponseFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/mcp", Handler)

	req, _ := http.NewRequest("GET", "/mcp", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Check content type
	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))

	// Check that response is valid JSON
	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err, "Response should be valid JSON")

	// Check JSON structure types
	assert.IsType(t, "", response["message"], "message should be string")
	assert.IsType(t, "", response["info"], "info should be string")
	assert.IsType(t, "", response["tools"], "tools should be string")
	assert.IsType(t, "", response["note"], "note should be string")

	t.Logf("✓ Handler returns properly formatted JSON")
}

func TestHandlerConcurrentRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/mcp", Handler)

	const numRequests = 50
	results := make(chan int, numRequests)

	// Send concurrent requests
	for range numRequests {
		go func() {
			req, _ := http.NewRequest("GET", "/mcp", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			results <- w.Code
		}()
	}

	// Collect results
	successCount := 0
	for range numRequests {
		statusCode := <-results
		if statusCode == http.StatusOK {
			successCount++
		}
	}

	assert.Equal(t, numRequests, successCount, "All concurrent requests should succeed")
	t.Logf("✓ Handler handles %d concurrent requests successfully", numRequests)
}

func TestHandlerWithConfigChanges(t *testing.T) {
	// Save original config
	originalServerAddress := config.ServerAddress

	testCases := []struct {
		name          string
		serverAddress string
	}{
		{"with_custom_server", "https://custom.example.com"},
		{"with_localhost", "http://localhost:8080"},
		{"with_empty_config", ""},
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/mcp", Handler)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set test config
			config.ServerAddress = tc.serverAddress

			req, _ := http.NewRequest("GET", "/mcp", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code, "Should work with any config")

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err, "Should parse response with config: %s", tc.serverAddress)

			assert.Contains(t, response, "message", "Should contain message")

			t.Logf("✓ Handler works with ServerAddress: %s", tc.serverAddress)
		})
	}

	// Restore original config
	config.ServerAddress = originalServerAddress
}

// Test middleware compatibility
func TestHandlerWithMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Add some test middleware
	router.Use(func(c *gin.Context) {
		c.Header("X-Test-Header", "test-value")
		c.Next()
	})

	router.GET("/mcp", Handler)

	req, _ := http.NewRequest("GET", "/mcp", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Check that middleware executed
	assert.Equal(t, "test-value", w.Header().Get("X-Test-Header"))

	// Check that handler still works
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response, "message")

	t.Logf("✓ Handler works correctly with middleware")
}

// Test error handling in handler
func TestHandlerErrorScenarios(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/mcp", Handler)

	// Test with malformed requests (though our handler doesn't parse request body)
	req, _ := http.NewRequest("GET", "/mcp", nil)
	req.Header.Set("Content-Length", "invalid")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Handler should still work since it doesn't depend on request parsing
	assert.Equal(t, http.StatusOK, w.Code)

	t.Logf("✓ Handler is resilient to malformed requests")
}

func TestHandlerResponseSize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/mcp", Handler)

	req, _ := http.NewRequest("GET", "/mcp", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	responseSize := len(w.Body.Bytes())

	// Response should be reasonably sized (not too small, not too large)
	assert.Greater(t, responseSize, 50, "Response should contain meaningful content")
	assert.Less(t, responseSize, 5000, "Response should not be excessively large")

	t.Logf("✓ Handler response size is appropriate: %d bytes", responseSize)
}

// TestMCPToolsIntegration tests the actual MCP tools functionality
// by calling the registered tools and verifying their responses
func TestMCPToolsIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create MCP server instance with tools registered
	mcpServer := NewServer()
	handler := NewGinStreamableHTTPHandler(mcpServer)

	// Create test router
	router := gin.New()
	router.POST("/mcp", handler)

	t.Log("=== Testing MCP Tools Integration ===")

	// First, we need to initialize the session
	initializeRequest := `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "initialize",
		"params": {
			"protocolVersion": "2024-11-05",
			"capabilities": {},
			"clientInfo": {
				"name": "test-client",
				"version": "1.0.0"
			}
		}
	}`

	req, err := http.NewRequest("POST", "/mcp", strings.NewReader(initializeRequest))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Initialize request should succeed")
	t.Logf("✓ MCP session initialized successfully")

	// Extract session ID from response headers for subsequent requests
	sessionID := w.Header().Get("Mcp-Session-Id")
	assert.NotEmpty(t, sessionID, "Should receive session ID")

	// Test cases for different tools
	toolTestCases := []struct {
		name         string
		toolName     string
		arguments    map[string]any
		expectResult bool
	}{
		{
			name:     "chat_completions_tool",
			toolName: "chat_completions",
			arguments: map[string]any{
				"model": "gpt-3.5-turbo",
				"messages": []map[string]any{
					{"role": "user", "content": "Hello"},
				},
			},
			expectResult: true,
		},
		{
			name:     "completions_tool",
			toolName: "completions",
			arguments: map[string]any{
				"model":  "gpt-3.5-turbo",
				"prompt": "Hello world",
			},
			expectResult: true,
		},
		{
			name:     "embeddings_tool",
			toolName: "embeddings",
			arguments: map[string]any{
				"model": "text-embedding-ada-002",
				"input": "Hello world",
			},
			expectResult: true,
		},
		{
			name:     "images_generations_tool",
			toolName: "images_generations",
			arguments: map[string]any{
				"model":  "dall-e-3",
				"prompt": "A beautiful sunset",
			},
			expectResult: true,
		},
		{
			name:     "audio_transcriptions_tool",
			toolName: "audio_transcriptions",
			arguments: map[string]any{
				"model": "whisper-1",
				"file":  "audio.mp3",
			},
			expectResult: true,
		},
		{
			name:     "audio_translations_tool",
			toolName: "audio_translations",
			arguments: map[string]any{
				"model": "whisper-1",
				"file":  "audio.mp3",
			},
			expectResult: true,
		},
		{
			name:     "audio_speech_tool",
			toolName: "audio_speech",
			arguments: map[string]any{
				"model": "tts-1",
				"input": "Hello world",
				"voice": "alloy",
			},
			expectResult: true,
		},
		{
			name:     "moderations_tool",
			toolName: "moderations",
			arguments: map[string]any{
				"input": "Hello world",
			},
			expectResult: true,
		},
		{
			name:         "models_list_tool",
			toolName:     "models_list",
			arguments:    map[string]any{},
			expectResult: true,
		},
		{
			name:     "claude_messages_tool",
			toolName: "claude_messages",
			arguments: map[string]any{
				"model": "claude-3-sonnet-20240229",
				"messages": []map[string]any{
					{"role": "user", "content": "Hello"},
				},
				"max_tokens": 100,
			},
			expectResult: true,
		},
	}

	for _, tc := range toolTestCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("\n--- Testing Tool: %s ---", tc.toolName)

			// Create tools/call request
			toolCallRequest := map[string]any{
				"jsonrpc": "2.0",
				"id":      2,
				"method":  "tools/call",
				"params": map[string]any{
					"name":      tc.toolName,
					"arguments": tc.arguments,
				},
			}

			requestBody, err := json.Marshal(toolCallRequest)
			assert.NoError(t, err, "Should marshal request body")

			req, err := http.NewRequest("POST", "/mcp", strings.NewReader(string(requestBody)))
			assert.NoError(t, err, "Should create request")
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			if sessionID != "" {
				req.Header.Set("Mcp-Session-Id", sessionID)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			t.Logf("Request Body: %s", string(requestBody))
			t.Logf("Response Status: %d", w.Code)
			t.Logf("Response Headers: %v", w.Header())
			t.Logf("Response Body: %s", w.Body.String())

			if tc.expectResult {
				assert.Equal(t, http.StatusOK, w.Code, "Tool call should succeed")

				// Check if response contains documentation
				responseBody := w.Body.String()
				assert.Contains(t, responseBody, "data:", "Should contain SSE data")

				// Extract JSON data from SSE response
				lines := strings.Split(responseBody, "\n")
				var jsonData string
				for _, line := range lines {
					if strings.HasPrefix(line, "data: ") {
						jsonData = strings.TrimPrefix(line, "data: ")
						break
					}
				}

				if jsonData != "" {
					var response map[string]any
					err = json.Unmarshal([]byte(jsonData), &response)
					if err == nil {
						// Check for result containing documentation
						if result, exists := response["result"]; exists {
							if resultMap, ok := result.(map[string]any); ok {
								if content, exists := resultMap["content"]; exists {
									t.Logf("✓ Tool %s returned documentation content", tc.toolName)

									// Verify content structure
									if contentArray, ok := content.([]any); ok && len(contentArray) > 0 {
										if textContent, ok := contentArray[0].(map[string]any); ok {
											if text, exists := textContent["text"]; exists {
												textStr := text.(string)
												assert.NotEmpty(t, textStr, "Documentation should not be empty")
												assert.Contains(t, textStr, "API", "Documentation should mention API")
												t.Logf("✓ Documentation length: %d characters", len(textStr))
											}
										}
									}
								}
							}
						}
					}
				}
			}

			t.Logf("--- End Tool Test: %s ---\n", tc.toolName)
		})
	}
}

// TestMCPToolsList tests the tools/list functionality
func TestMCPToolsList(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mcpServer := NewServer()
	handler := NewGinStreamableHTTPHandler(mcpServer)

	router := gin.New()
	router.POST("/mcp", handler)

	t.Log("=== Testing MCP Tools List ===")

	// Initialize session first
	initializeRequest := `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "initialize",
		"params": {
			"protocolVersion": "2024-11-05",
			"capabilities": {},
			"clientInfo": {
				"name": "test-client",
				"version": "1.0.0"
			}
		}
	}`

	req, err := http.NewRequest("POST", "/mcp", strings.NewReader(initializeRequest))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	sessionID := w.Header().Get("Mcp-Session-Id")

	// Now test tools/list
	toolsListRequest := `{
		"jsonrpc": "2.0",
		"id": 2,
		"method": "tools/list",
		"params": {}
	}`

	req, err = http.NewRequest("POST", "/mcp", strings.NewReader(toolsListRequest))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}

	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	t.Logf("Tools List Response Status: %d", w.Code)
	t.Logf("Tools List Response Body: %s", w.Body.String())

	assert.Equal(t, http.StatusOK, w.Code, "Tools list should succeed")

	responseBody := w.Body.String()

	// Expected tools from addRelayAPITools
	expectedTools := []string{
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

	// Extract JSON data from SSE response
	lines := strings.Split(responseBody, "\n")
	var jsonData string
	for _, line := range lines {
		if strings.HasPrefix(line, "data: ") {
			jsonData = strings.TrimPrefix(line, "data: ")
			break
		}
	}

	if jsonData != "" {
		var response map[string]any
		err = json.Unmarshal([]byte(jsonData), &response)
		if err == nil {
			if result, exists := response["result"]; exists {
				if resultMap, ok := result.(map[string]any); ok {
					if tools, exists := resultMap["tools"]; exists {
						if toolsArray, ok := tools.([]any); ok {
							t.Logf("✓ Found %d tools in response", len(toolsArray))

							// Verify all expected tools are present
							foundTools := make(map[string]bool)
							for _, tool := range toolsArray {
								if toolMap, ok := tool.(map[string]any); ok {
									if name, exists := toolMap["name"]; exists {
										foundTools[name.(string)] = true
									}
								}
							}

							for _, expectedTool := range expectedTools {
								assert.True(t, foundTools[expectedTool], "Should find tool: %s", expectedTool)
							}

							t.Logf("✓ All expected tools found in tools/list response")
						}
					}
				}
			}
		}
	}
}

// TestMCPDocumentationContent tests the actual documentation content generated by tools
func TestMCPDocumentationContent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Save original config
	originalServerAddress := config.ServerAddress
	config.ServerAddress = "https://api.example.com"
	defer func() {
		config.ServerAddress = originalServerAddress
	}()

	mcpServer := NewServer()
	handler := NewGinStreamableHTTPHandler(mcpServer)

	router := gin.New()
	router.POST("/mcp", handler)

	t.Log("=== Testing MCP Documentation Content Quality ===")

	// Initialize session
	initializeRequest := `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "initialize",
		"params": {
			"protocolVersion": "2024-11-05",
			"capabilities": {},
			"clientInfo": {
				"name": "test-client",
				"version": "1.0.0"
			}
		}
	}`

	req, err := http.NewRequest("POST", "/mcp", strings.NewReader(initializeRequest))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	sessionID := w.Header().Get("Mcp-Session-Id")

	// Test documentation content for specific tools
	contentTests := []struct {
		name            string
		toolName        string
		arguments       map[string]any
		expectedContent []string
	}{
		{
			name:     "chat_completions_documentation",
			toolName: "chat_completions",
			arguments: map[string]any{
				"model":    "gpt-3.5-turbo",
				"messages": []map[string]any{{"role": "user", "content": "test"}},
			},
			expectedContent: []string{
				"Chat Completions",
				"POST",
				"/v1/chat/completions",
				"https://api.example.com",
				"model",
				"messages",
			},
		},
		{
			name:     "embeddings_documentation",
			toolName: "embeddings",
			arguments: map[string]any{
				"model": "text-embedding-ada-002",
				"input": "test text",
			},
			expectedContent: []string{
				"Embeddings",
				"POST",
				"/v1/embeddings",
				"https://api.example.com",
				"model",
				"input",
			},
		},
		{
			name:      "models_list_documentation",
			toolName:  "models_list",
			arguments: map[string]any{},
			expectedContent: []string{
				"Models",
				"GET",
				"/v1/models",
				"https://api.example.com",
			},
		},
	}

	for _, tc := range contentTests {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("\n--- Testing Documentation Content: %s ---", tc.toolName)

			// Create tools/call request
			toolCallRequest := map[string]any{
				"jsonrpc": "2.0",
				"id":      2,
				"method":  "tools/call",
				"params": map[string]any{
					"name":      tc.toolName,
					"arguments": tc.arguments,
				},
			}

			requestBody, err := json.Marshal(toolCallRequest)
			assert.NoError(t, err, "Should marshal request body")

			req, err := http.NewRequest("POST", "/mcp", strings.NewReader(string(requestBody)))
			assert.NoError(t, err, "Should create request")
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			if sessionID != "" {
				req.Header.Set("Mcp-Session-Id", sessionID)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code, "Tool call should succeed")

			// Extract and verify documentation content
			responseBody := w.Body.String()
			lines := strings.Split(responseBody, "\n")
			var jsonData string
			for _, line := range lines {
				if strings.HasPrefix(line, "data: ") {
					jsonData = strings.TrimPrefix(line, "data: ")
					break
				}
			}

			if jsonData != "" {
				var response map[string]any
				err = json.Unmarshal([]byte(jsonData), &response)
				if err == nil {
					if result, exists := response["result"]; exists {
						if resultMap, ok := result.(map[string]any); ok {
							if content, exists := resultMap["content"]; exists {
								if contentArray, ok := content.([]any); ok && len(contentArray) > 0 {
									if textContent, ok := contentArray[0].(map[string]any); ok {
										if text, exists := textContent["text"]; exists {
											textStr := text.(string)
											t.Logf("✓ Documentation generated for %s (%d chars)", tc.toolName, len(textStr))

											// Verify expected content is present
											for _, expectedStr := range tc.expectedContent {
												assert.Contains(t, textStr, expectedStr,
													"Documentation should contain: %s", expectedStr)
											}

											t.Logf("✓ All expected content found in %s documentation", tc.toolName)
										}
									}
								}
							}
						}
					}
				}
			}

			t.Logf("--- End Documentation Test: %s ---\n", tc.toolName)
		})
	}
}

// TestAddRelayAPIToolsFunction tests the addRelayAPITools function directly
func TestAddRelayAPIToolsFunction(t *testing.T) {
	t.Log("=== Testing addRelayAPITools Function ===")

	// Create a new server instance
	mcpServer := NewServer()

	// Verify that tools are registered
	assert.NotNil(t, mcpServer, "MCP server should be created")
	assert.NotNil(t, mcpServer.server, "Underlying MCP server should exist")

	// The addRelayAPITools function is called during NewServer()
	// We can verify it worked by checking if we can get tool information
	t.Logf("✓ addRelayAPITools function executed during server creation")

	// Test that the server can handle tool-related requests
	gin.SetMode(gin.TestMode)
	handler := NewGinStreamableHTTPHandler(mcpServer)
	router := gin.New()
	router.POST("/mcp", handler)

	// Initialize session
	initReq := `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "initialize",
		"params": {
			"protocolVersion": "2024-11-05",
			"capabilities": {},
			"clientInfo": {"name": "test", "version": "1.0"}
		}
	}`

	req, _ := http.NewRequest("POST", "/mcp", strings.NewReader(initReq))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Initialize should succeed")
	sessionID := w.Header().Get("Mcp-Session-Id")

	// Test tools/list to verify tools were registered
	toolsReq := `{
		"jsonrpc": "2.0",
		"id": 2,
		"method": "tools/list",
		"params": {}
	}`

	req, _ = http.NewRequest("POST", "/mcp", strings.NewReader(toolsReq))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}

	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Tools list should succeed")

	// Verify response contains tools
	responseBody := w.Body.String()
	assert.Contains(t, responseBody, "tools", "Response should contain tools")
	assert.Contains(t, responseBody, "chat_completions", "Should contain chat_completions tool")
	assert.Contains(t, responseBody, "embeddings", "Should contain embeddings tool")

	t.Logf("✓ addRelayAPITools function successfully registered tools")
}

// TestMCPToolsErrorHandling tests error scenarios for MCP tools
func TestMCPToolsErrorHandling(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mcpServer := NewServer()
	handler := NewGinStreamableHTTPHandler(mcpServer)

	router := gin.New()
	router.POST("/mcp", handler)

	t.Log("=== Testing MCP Tools Error Handling ===")

	// Initialize session
	initReq := `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "initialize",
		"params": {
			"protocolVersion": "2024-11-05",
			"capabilities": {},
			"clientInfo": {"name": "test", "version": "1.0"}
		}
	}`

	req, _ := http.NewRequest("POST", "/mcp", strings.NewReader(initReq))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	sessionID := w.Header().Get("Mcp-Session-Id")

	// Test invalid tool call
	invalidToolReq := `{
		"jsonrpc": "2.0",
		"id": 2,
		"method": "tools/call",
		"params": {
			"name": "nonexistent_tool",
			"arguments": {}
		}
	}`

	req, _ = http.NewRequest("POST", "/mcp", strings.NewReader(invalidToolReq))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}

	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	t.Logf("Invalid tool call response: %d - %s", w.Code, w.Body.String())

	// Should return an error response but still 200 OK (JSON-RPC error)
	assert.Equal(t, http.StatusOK, w.Code, "Should return 200 with JSON-RPC error")

	responseBody := w.Body.String()
	assert.Contains(t, responseBody, "error", "Response should contain error")

	t.Logf("✓ Error handling works correctly for invalid tool calls")
}

// TestAddDocumentationResourcesFunction tests the addDocumentationResources function directly
func TestAddDocumentationResourcesFunction(t *testing.T) {
	t.Log("=== Testing addDocumentationResources Function ===")

	// Test with resources enabled (default)
	optsWithResources := DefaultServerOptions().
		WithName("test-resources-server").
		WithVersion("1.0.0")

	serverWithResources := NewServerWithOptions(optsWithResources)

	// Verify that server is created with documentation resources
	assert.NotNil(t, serverWithResources, "MCP server with resources should be created")
	assert.NotNil(t, serverWithResources.server, "Underlying MCP server should exist")

	// Test that the server can handle resource-related requests
	gin.SetMode(gin.TestMode)
	handler := NewGinStreamableHTTPHandler(serverWithResources)
	router := gin.New()
	router.POST("/mcp", handler)

	// Initialize session
	initReq := `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "initialize",
		"params": {
			"protocolVersion": "2024-11-05",
			"capabilities": {},
			"clientInfo": {"name": "test", "version": "1.0"}
		}
	}`

	req, _ := http.NewRequest("POST", "/mcp", strings.NewReader(initReq))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Initialize should succeed")
	sessionID := w.Header().Get("Mcp-Session-Id")

	// Test resources/list to verify documentation resources were registered
	resourcesReq := `{
		"jsonrpc": "2.0",
		"id": 2,
		"method": "resources/list",
		"params": {}
	}`

	req, _ = http.NewRequest("POST", "/mcp", strings.NewReader(resourcesReq))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}

	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Resources list should succeed")

	// Verify response contains documentation resources
	responseBody := w.Body.String()
	assert.Contains(t, responseBody, "resources", "Response should contain resources")
	assert.Contains(t, responseBody, "oneapi://docs/api-endpoints", "Should contain API endpoints resource")
	assert.Contains(t, responseBody, "oneapi://docs/tool-usage-guide", "Should contain tool usage guide resource")
	assert.Contains(t, responseBody, "oneapi://docs/authentication-guide", "Should contain authentication guide resource")
	assert.Contains(t, responseBody, "oneapi://docs/integration-patterns", "Should contain integration patterns resource")

	t.Logf("✓ addDocumentationResources function successfully registered documentation resources")

	// Test with resources disabled
	optsWithoutResources := DefaultServerOptions().
		WithName("test-no-resources-server").
		DisableResources().
		WithVersion("1.0.0")

	serverWithoutResources := NewServerWithOptions(optsWithoutResources)

	// Verify that server is created without documentation resources
	assert.NotNil(t, serverWithoutResources, "MCP server without resources should be created")
	assert.NotNil(t, serverWithoutResources.server, "Underlying MCP server should exist")

	// Test that the server can handle resource-related requests
	handler2 := NewGinStreamableHTTPHandler(serverWithoutResources)
	router2 := gin.New()
	router2.POST("/mcp", handler2)

	req2, _ := http.NewRequest("POST", "/mcp", strings.NewReader(initReq))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Accept", "application/json, text/event-stream")

	w2 := httptest.NewRecorder()
	router2.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code, "Initialize should succeed")
	sessionID2 := w2.Header().Get("Mcp-Session-Id")

	// Test resources/list to verify documentation resources were not registered
	resourcesReq2 := `{
		"jsonrpc": "2.0",
		"id": 2,
		"method": "resources/list",
		"params": {}
	}`

	req2, _ = http.NewRequest("POST", "/mcp", strings.NewReader(resourcesReq2))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID2 != "" {
		req2.Header.Set("Mcp-Session-Id", sessionID2)
	}

	w2 = httptest.NewRecorder()
	router2.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code, "Resources list should succeed")

	// Verify response does not contain documentation resources
	responseBody2 := w2.Body.String()
	// Even if resources are disabled, the server might still return an empty resources list
	// but it should not contain the specific documentation resources
	if strings.Contains(responseBody2, "resources") {
		assert.NotContains(t, responseBody2, "oneapi://docs/api-endpoints", "Should not contain API endpoints resource")
		assert.NotContains(t, responseBody2, "oneapi://docs/tool-usage-guide", "Should not contain tool usage guide resource")
		assert.NotContains(t, responseBody2, "oneapi://docs/authentication-guide", "Should not contain authentication guide resource")
		assert.NotContains(t, responseBody2, "oneapi://docs/integration-patterns", "Should not contain integration patterns resource")
	}

	t.Logf("✓ addDocumentationResources function properly respects EnableResources flag")
}
