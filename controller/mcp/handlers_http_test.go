package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	for i := 0; i < numRequests; i++ {
		go func() {
			req, _ := http.NewRequest("GET", "/mcp", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			results <- w.Code
		}()
	}

	// Collect results
	successCount := 0
	for i := 0; i < numRequests; i++ {
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
