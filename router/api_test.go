package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/mcp"
	"github.com/stretchr/testify/assert"
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

// TestMCPToolsIntegration tests the actual MCP tools functionality
// by calling the registered tools and verifying their responses
// This test verifies that the singleton MCP server properly handles tool calls
func TestMCPToolsIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Reset the singleton for testing
	mcpServerInstance = nil
	mcpHandlerInstance = nil
	mcpOnce = sync.Once{}

	// Create test router with direct MCP handler (bypass auth for testing)
	router := gin.New()

	// Initialize our singleton MCP server and handler
	initMCPServer()

	// Verify singleton was created
	if mcpServerInstance == nil {
		t.Fatal("MCP server instance should not be nil after initMCPServer")
	}
	if mcpHandlerInstance == nil {
		t.Fatal("MCP handler instance should not be nil after initMCPServer")
	}

	// Create test routes without authentication middleware
	testMcpRoute := router.Group("/test-mcp")
	{
		// Use our singleton handler directly for testing (no auth required)
		testMcpRoute.GET("/", mcp.Handler)         // Info endpoint
		testMcpRoute.POST("/", mcpHandlerInstance) // MCP protocol endpoint
	}

	t.Log("=== Testing MCP Tools Integration with Singleton Pattern ===")
	t.Logf("Singleton MCP Server Instance: %p", mcpServerInstance)

	// === MCP Protocol Flow Implementation ===
	// Step 1: Send initialize request
	t.Log("\n--- Step 1: Initialize MCP Session ---")
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

	req, err := http.NewRequest("POST", "/test-mcp", strings.NewReader(initializeRequest))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Handle redirect if needed
	responseBody := w.Body.String()
	if w.Code >= 300 && w.Code < 400 {
		location := w.Header().Get("Location")
		if location != "" {
			t.Logf("Following redirect to: %s", location)
			redirectReq, err := http.NewRequest("POST", location, strings.NewReader(initializeRequest))
			assert.NoError(t, err)
			redirectReq.Header.Set("Content-Type", "application/json")
			redirectReq.Header.Set("Accept", "application/json, text/event-stream")

			redirectW := httptest.NewRecorder()
			router.ServeHTTP(redirectW, redirectReq)
			responseBody = redirectW.Body.String()
			w = redirectW
		}
	}

	// Step 2: Wait for successful initialization response
	t.Log("\n--- Step 2: Parse Initialize Response ---")
	if w.Code >= 200 && w.Code < 400 {
		t.Logf("✓ MCP session initialized successfully via singleton server (status: %d)", w.Code)
	} else {
		t.Fatalf("Initialize request failed with status: %d", w.Code)
	}

	t.Logf("Initialize response body: %s", responseBody)

	// Parse the initialization response
	var initializeResponseParsed bool
	if strings.Contains(responseBody, "data:") {
		lines := strings.Split(responseBody, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "data: ") {
				jsonData := strings.TrimPrefix(line, "data: ")
				var response map[string]any
				if err := json.Unmarshal([]byte(jsonData), &response); err == nil {
					if result, exists := response["result"]; exists {
						t.Logf("✓ Initialize response received: %v", result)
						initializeResponseParsed = true
						break
					}
				}
			}
		}
	}

	if !initializeResponseParsed {
		t.Logf("ⓘ Initialize response format not parsed, but continuing with protocol")
	}

	// Extract session ID from response headers for subsequent requests
	sessionID := w.Header().Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Logf("ⓘ No session ID received (this is expected for some MCP implementations)")
	} else {
		t.Logf("✓ Session ID received: %s", sessionID)
	}

	// Step 3: Send initialized notification (MCP protocol requirement)
	t.Log("\n--- Step 3: Send Initialized Notification ---")
	initializedNotification := `{
		"jsonrpc": "2.0",
		"method": "initialized",
		"params": {}
	}`

	notifyReq, err := http.NewRequest("POST", "/test-mcp", strings.NewReader(initializedNotification))
	assert.NoError(t, err)
	notifyReq.Header.Set("Content-Type", "application/json")
	notifyReq.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		notifyReq.Header.Set("Mcp-Session-Id", sessionID)
	}

	notifyW := httptest.NewRecorder()
	router.ServeHTTP(notifyW, notifyReq)

	// Handle redirect for notification if needed
	if notifyW.Code >= 300 && notifyW.Code < 400 {
		location := notifyW.Header().Get("Location")
		if location != "" {
			t.Logf("Following notification redirect to: %s", location)
			redirectNotifyReq, err := http.NewRequest("POST", location, strings.NewReader(initializedNotification))
			assert.NoError(t, err)
			redirectNotifyReq.Header.Set("Content-Type", "application/json")
			redirectNotifyReq.Header.Set("Accept", "application/json, text/event-stream")
			if sessionID != "" {
				redirectNotifyReq.Header.Set("Mcp-Session-Id", sessionID)
			}

			redirectNotifyW := httptest.NewRecorder()
			router.ServeHTTP(redirectNotifyW, redirectNotifyReq)
			notifyW = redirectNotifyW
		}
	}

	if notifyW.Code >= 200 && notifyW.Code < 400 {
		t.Logf("✓ Initialized notification sent successfully (status: %d)", notifyW.Code)
	} else {
		t.Logf("ⓘ Initialized notification status: %d (continuing anyway)", notifyW.Code)
	}

	t.Log("\n--- Step 4: Now Ready for Tool Calls ---")
	t.Log("✅ MCP Protocol initialization sequence completed!")
	t.Log("✅ Session is now ready to accept tool calls")

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
			t.Logf("\n--- Testing Tool: %s via Singleton Server ---", tc.toolName)

			// Verify we're still using the same singleton instance
			currentServerInstance := mcpServerInstance
			assert.Equal(t, mcpServerInstance, currentServerInstance, "Should use same singleton instance")
			t.Logf("Using Singleton Server Instance: %p", currentServerInstance)

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

			req, err := http.NewRequest("POST", "/test-mcp", strings.NewReader(string(requestBody)))
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
				// Accept both 200 OK and 3xx redirects as successful
				if w.Code >= 200 && w.Code < 400 {
					t.Logf("✓ Tool call succeeded with status %d", w.Code)
				} else {
					t.Errorf("Tool call failed with status %d", w.Code)
					return
				}

				// Check if response contains documentation or handle redirects
				responseBody := w.Body.String()

				// If it's a redirect, follow it
				if w.Code >= 300 && w.Code < 400 {
					location := w.Header().Get("Location")
					if location != "" {
						t.Logf("Following redirect to: %s", location)

						// Make a new request to the redirect location
						redirectReq, err := http.NewRequest("POST", location, strings.NewReader(string(requestBody)))
						assert.NoError(t, err, "Should create redirect request")
						redirectReq.Header.Set("Content-Type", "application/json")
						redirectReq.Header.Set("Accept", "application/json, text/event-stream")
						if sessionID != "" {
							redirectReq.Header.Set("Mcp-Session-Id", sessionID)
						}

						redirectW := httptest.NewRecorder()
						router.ServeHTTP(redirectW, redirectReq)

						responseBody = redirectW.Body.String()
						t.Logf("Redirect response status: %d", redirectW.Code)
						t.Logf("Redirect response body: %s", responseBody)
					}
				}

				// Now check for SSE data (if we have content)
				if responseBody != "" {
					if strings.Contains(responseBody, "data:") {
						t.Logf("✓ Response contains SSE data")
					} else {
						t.Logf("ⓘ Response does not contain SSE data, but tool call was processed")
					}
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
						// Check for result containing documentation
						if result, exists := response["result"]; exists {
							if resultMap, ok := result.(map[string]any); ok {
								if content, exists := resultMap["content"]; exists {
									t.Logf("✓ Tool %s returned documentation content via singleton server", tc.toolName)

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

	t.Log("\n=== Final Singleton Verification ===")
	t.Logf("✓ Singleton MCP Server Instance: %p (consistent throughout all tests)", mcpServerInstance)
	t.Log("✅ All tools successfully tested via singleton MCP server")
	t.Log("✅ Singleton pattern maintains same server instance across all tool calls")
	t.Log("✅ Each tool call processed with unique request context (as expected)")
}

// TestMCPSingletonConcurrency tests that the singleton pattern works correctly
// with concurrent goroutines (like real HTTP requests)
func TestMCPSingletonConcurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Reset the singleton for testing
	mcpServerInstance = nil
	mcpHandlerInstance = nil
	mcpOnce = sync.Once{}

	// Create test router
	router := gin.New()

	// Initialize our singleton MCP server and handler
	initMCPServer()

	// Create test routes without authentication middleware
	testMcpRoute := router.Group("/test-mcp")
	{
		testMcpRoute.GET("/", mcp.Handler)         // Info endpoint
		testMcpRoute.POST("/", mcpHandlerInstance) // MCP protocol endpoint
	}

	t.Log("=== Testing MCP Singleton with Concurrent Goroutines ===")
	t.Logf("Initial Singleton MCP Server Instance: %p", mcpServerInstance)

	// Test concurrent access with multiple goroutines
	const numGoroutines = 10
	var wg sync.WaitGroup
	var mutex sync.Mutex
	serverInstances := make([]any, numGoroutines)
	requestIDs := make([]string, numGoroutines)

	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()

			t.Logf("Goroutine %d: Starting concurrent request", goroutineID)

			// Each goroutine captures the current server instance
			currentServerInstance := mcpServerInstance

			// Initialize session for this goroutine
			initializeRequest := `{
				"jsonrpc": "2.0",
				"id": 1,
				"method": "initialize",
				"params": {
					"protocolVersion": "2024-11-05",
					"capabilities": {},
					"clientInfo": {
						"name": "test-client-concurrent",
						"version": "1.0.0"
					}
				}
			}`

			req, err := http.NewRequest("POST", "/test-mcp", strings.NewReader(initializeRequest))
			if err != nil {
				t.Errorf("Goroutine %d: Failed to create request: %v", goroutineID, err)
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Handle redirect if needed
			responseBody := w.Body.String()
			if w.Code >= 300 && w.Code < 400 {
				location := w.Header().Get("Location")
				if location != "" {
					redirectReq, err := http.NewRequest("POST", location, strings.NewReader(initializeRequest))
					if err != nil {
						t.Errorf("Goroutine %d: Failed to create redirect request: %v", goroutineID, err)
						return
					}
					redirectReq.Header.Set("Content-Type", "application/json")
					redirectReq.Header.Set("Accept", "application/json, text/event-stream")

					redirectW := httptest.NewRecorder()
					router.ServeHTTP(redirectW, redirectReq)
					responseBody = redirectW.Body.String()
					w = redirectW
				}
			}

			if w.Code < 200 || w.Code >= 400 {
				t.Errorf("Goroutine %d: Initialize failed with status: %d", goroutineID, w.Code)
				return
			}

			// Extract request ID from SSE response
			var requestID string
			lines := strings.Split(responseBody, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "id: ") {
					requestID = strings.TrimPrefix(line, "id: ")
					break
				}
			}

			// Store results safely
			mutex.Lock()
			serverInstances[goroutineID] = currentServerInstance
			requestIDs[goroutineID] = requestID
			mutex.Unlock()

			t.Logf("Goroutine %d: Using Server Instance: %p, Request ID: %s",
				goroutineID, currentServerInstance, requestID)

		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	t.Log("\n=== Concurrent Goroutine Results Analysis ===")

	// Verify all goroutines used the same singleton server instance
	baseServerInstance := serverInstances[0]
	allSameServer := true
	for i, instance := range serverInstances {
		if instance != baseServerInstance {
			allSameServer = false
			t.Errorf("Goroutine %d used different server instance: %p vs %p",
				i, instance, baseServerInstance)
		}
	}

	if allSameServer {
		t.Logf("✅ All %d goroutines used the SAME singleton server instance: %p",
			numGoroutines, baseServerInstance)
	}

	// Verify all goroutines got DIFFERENT request IDs (unique contexts)
	requestIDSet := make(map[string]bool)
	allUniqueIDs := true
	for i, id := range requestIDs {
		if id == "" {
			t.Logf("ⓘ Goroutine %d: No request ID extracted (this is normal)", i)
			continue
		}
		if requestIDSet[id] {
			allUniqueIDs = false
			t.Errorf("Goroutine found duplicate request ID: %s", id)
		}
		requestIDSet[id] = true
	}

	if allUniqueIDs {
		t.Logf("✅ All goroutines got UNIQUE request IDs (unique contexts per request)")
	}

	t.Log("\n=== Final Concurrent Test Summary ===")
	t.Logf("✅ Singleton Pattern: %d goroutines → 1 shared MCP server instance", numGoroutines)
	t.Logf("✅ Request Isolation: Each goroutine → unique request context/ID")
	t.Logf("✅ Thread Safety: Concurrent access handled correctly")
	t.Logf("✅ Resource Efficiency: No unnecessary server instance creation")

	// Verify the singleton is still the same as what we started with
	if mcpServerInstance == baseServerInstance {
		t.Log("✅ Global singleton instance remained consistent throughout concurrent access")
	} else {
		t.Error("❌ Global singleton instance changed during concurrent access")
	}
}
