package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestAPIFormatAutoDetect_Disabled(t *testing.T) {
	// Save and restore original config
	originalEnabled := config.AutoDetectAPIFormat
	config.AutoDetectAPIFormat = false
	defer func() { config.AutoDetectAPIFormat = originalEnabled }()

	engine := gin.New()
	engine.Use(APIFormatAutoDetect(engine))

	chatCompletionsCalled := false
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		chatCompletionsCalled = true
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Send Response API format to chat/completions - should NOT be redirected
	body := `{"model": "gpt-4", "input": "Hello"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, chatCompletionsCalled, "chat completions handler should be called when auto-detect is disabled")
}

func TestAPIFormatAutoDetect_MatchingFormat(t *testing.T) {
	// Save and restore original config
	originalEnabled := config.AutoDetectAPIFormat
	originalAction := config.AutoDetectAPIFormatAction
	config.AutoDetectAPIFormat = true
	config.AutoDetectAPIFormatAction = "transparent"
	defer func() {
		config.AutoDetectAPIFormat = originalEnabled
		config.AutoDetectAPIFormatAction = originalAction
	}()

	engine := gin.New()
	engine.Use(APIFormatAutoDetect(engine))

	chatCompletionsCalled := false
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		chatCompletionsCalled = true
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Send ChatCompletion format to chat/completions - should proceed normally
	body := `{"model": "gpt-4", "messages": [{"role": "user", "content": "Hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, chatCompletionsCalled, "chat completions handler should be called for matching format")
}

func TestAPIFormatAutoDetect_TransparentRouting_ResponseToChatCompletions(t *testing.T) {
	// Save and restore original config
	originalEnabled := config.AutoDetectAPIFormat
	originalAction := config.AutoDetectAPIFormatAction
	config.AutoDetectAPIFormat = true
	config.AutoDetectAPIFormatAction = "transparent"
	defer func() {
		config.AutoDetectAPIFormat = originalEnabled
		config.AutoDetectAPIFormatAction = originalAction
	}()

	engine := gin.New()
	engine.Use(APIFormatAutoDetect(engine))

	chatCompletionsCalled := false
	responseAPICalled := false

	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		chatCompletionsCalled = true
		c.JSON(http.StatusOK, gin.H{"handler": "chat_completions"})
	})
	engine.POST("/v1/responses", func(c *gin.Context) {
		responseAPICalled = true
		// Verify body is still available
		body, _ := io.ReadAll(c.Request.Body)
		require.NotEmpty(t, body, "body should be available in redirected handler")
		c.JSON(http.StatusOK, gin.H{"handler": "responses"})
	})

	// Send Response API format to chat/completions
	body := `{"model": "gpt-4", "input": "Hello"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.False(t, chatCompletionsCalled, "chat completions handler should NOT be called")
	require.True(t, responseAPICalled, "responses handler should be called for Response API format")
}

func TestAPIFormatAutoDetect_AmbiguousFormat_NoRerouting(t *testing.T) {
	// Save and restore original config
	originalEnabled := config.AutoDetectAPIFormat
	originalAction := config.AutoDetectAPIFormatAction
	config.AutoDetectAPIFormat = true
	config.AutoDetectAPIFormatAction = "transparent"
	defer func() {
		config.AutoDetectAPIFormat = originalEnabled
		config.AutoDetectAPIFormatAction = originalAction
	}()

	engine := gin.New()
	engine.Use(APIFormatAutoDetect(engine))

	responseAPICalled := false

	engine.POST("/v1/responses", func(c *gin.Context) {
		responseAPICalled = true
		c.JSON(http.StatusOK, gin.H{"handler": "responses"})
	})

	// Send ambiguous format (simple messages) to /v1/responses
	// This could be either ChatCompletion or Claude, so it should NOT be rerouted
	// to preserve backward compatibility
	body := `{"model": "gpt-4", "messages": [{"role": "user", "content": "Hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, responseAPICalled, "responses handler should be called - ambiguous format should NOT be rerouted")
}

func TestAPIFormatAutoDetect_RedirectMode(t *testing.T) {
	// Save and restore original config
	originalEnabled := config.AutoDetectAPIFormat
	originalAction := config.AutoDetectAPIFormatAction
	config.AutoDetectAPIFormat = true
	config.AutoDetectAPIFormatAction = "redirect"
	defer func() {
		config.AutoDetectAPIFormat = originalEnabled
		config.AutoDetectAPIFormatAction = originalAction
	}()

	engine := gin.New()
	engine.Use(APIFormatAutoDetect(engine))

	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"handler": "chat_completions"})
	})
	engine.POST("/v1/responses", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"handler": "responses"})
	})

	// Send Response API format to chat/completions
	body := `{"model": "gpt-4", "input": "Hello"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusFound, w.Code)
	require.Equal(t, "/v1/responses", w.Header().Get("Location"))
}

func TestAPIFormatAutoDetect_RedirectWithQueryParams(t *testing.T) {
	// Save and restore original config
	originalEnabled := config.AutoDetectAPIFormat
	originalAction := config.AutoDetectAPIFormatAction
	config.AutoDetectAPIFormat = true
	config.AutoDetectAPIFormatAction = "redirect"
	defer func() {
		config.AutoDetectAPIFormat = originalEnabled
		config.AutoDetectAPIFormatAction = originalAction
	}()

	engine := gin.New()
	engine.Use(APIFormatAutoDetect(engine))

	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"handler": "chat_completions"})
	})

	// Send Response API format to chat/completions with query params
	body := `{"model": "gpt-4", "input": "Hello"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?stream=true", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusFound, w.Code)
	require.Equal(t, "/v1/responses?stream=true", w.Header().Get("Location"))
}

func TestAPIFormatAutoDetect_ClaudeMessages_Unambiguous(t *testing.T) {
	// Save and restore original config
	originalEnabled := config.AutoDetectAPIFormat
	originalAction := config.AutoDetectAPIFormatAction
	config.AutoDetectAPIFormat = true
	config.AutoDetectAPIFormatAction = "transparent"
	defer func() {
		config.AutoDetectAPIFormat = originalEnabled
		config.AutoDetectAPIFormatAction = originalAction
	}()

	engine := gin.New()
	engine.Use(APIFormatAutoDetect(engine))

	chatCompletionsCalled := false
	claudeMessagesCalled := false

	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		chatCompletionsCalled = true
		c.JSON(http.StatusOK, gin.H{"handler": "chat_completions"})
	})
	engine.POST("/v1/messages", func(c *gin.Context) {
		claudeMessagesCalled = true
		c.JSON(http.StatusOK, gin.H{"handler": "claude_messages"})
	})

	// Send UNAMBIGUOUS Claude Messages format to chat/completions
	// Uses tool_use content block which is Claude-exclusive
	body := `{
		"model": "claude-3-opus",
		"max_tokens": 1024,
		"messages": [
			{"role": "user", "content": "Get weather"},
			{"role": "assistant", "content": [{"type": "tool_use", "id": "toolu_1", "name": "get_weather", "input": {}}]}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.False(t, chatCompletionsCalled, "chat completions handler should NOT be called")
	require.True(t, claudeMessagesCalled, "claude messages handler should be called for unambiguous Claude format")
}

func TestAPIFormatAutoDetect_ClaudeMessages_Ambiguous(t *testing.T) {
	// Save and restore original config
	originalEnabled := config.AutoDetectAPIFormat
	originalAction := config.AutoDetectAPIFormatAction
	config.AutoDetectAPIFormat = true
	config.AutoDetectAPIFormatAction = "transparent"
	defer func() {
		config.AutoDetectAPIFormat = originalEnabled
		config.AutoDetectAPIFormatAction = originalAction
	}()

	engine := gin.New()
	engine.Use(APIFormatAutoDetect(engine))

	chatCompletionsCalled := false

	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		chatCompletionsCalled = true
		c.JSON(http.StatusOK, gin.H{"handler": "chat_completions"})
	})

	// Send AMBIGUOUS Claude-like format to chat/completions
	// Has system field, but this is ambiguous (some clients send incorrectly)
	// Should NOT be rerouted to preserve backward compatibility
	body := `{
		"model": "claude-3-opus",
		"max_tokens": 1024,
		"system": "You are helpful",
		"messages": [{"role": "user", "content": "Hello"}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, chatCompletionsCalled, "chat completions handler should be called - ambiguous format should NOT be rerouted")
}

func TestAPIFormatAutoDetect_EmptyBody(t *testing.T) {
	// Save and restore original config
	originalEnabled := config.AutoDetectAPIFormat
	config.AutoDetectAPIFormat = true
	defer func() { config.AutoDetectAPIFormat = originalEnabled }()

	engine := gin.New()
	engine.Use(APIFormatAutoDetect(engine))

	handlerCalled := false
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		handlerCalled = true
		c.JSON(http.StatusBadRequest, gin.H{"error": "empty body"})
	})

	// Send empty body - should proceed to handler which will handle the error
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	require.True(t, handlerCalled, "handler should be called for empty body")
}

func TestAPIFormatAutoDetect_InvalidJSON(t *testing.T) {
	// Save and restore original config
	originalEnabled := config.AutoDetectAPIFormat
	config.AutoDetectAPIFormat = true
	defer func() { config.AutoDetectAPIFormat = originalEnabled }()

	engine := gin.New()
	engine.Use(APIFormatAutoDetect(engine))

	handlerCalled := false
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		handlerCalled = true
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
	})

	// Send invalid JSON - should proceed to handler
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString("{invalid}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	require.True(t, handlerCalled, "handler should be called for invalid JSON")
}

func TestAPIFormatAutoDetect_NonChatEndpoint(t *testing.T) {
	// Save and restore original config
	originalEnabled := config.AutoDetectAPIFormat
	config.AutoDetectAPIFormat = true
	defer func() { config.AutoDetectAPIFormat = originalEnabled }()

	engine := gin.New()
	engine.Use(APIFormatAutoDetect(engine))

	handlerCalled := false
	engine.POST("/v1/embeddings", func(c *gin.Context) {
		handlerCalled = true
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Send to non-chat endpoint - should proceed normally
	body := `{"model": "text-embedding-ada-002", "input": "Hello"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, handlerCalled, "embeddings handler should be called normally")
}

// TestAPIFormatAutoDetect_RealWorldCursor simulates the real-world case where
// Cursor sends Response API format to the chat/completions endpoint.
func TestAPIFormatAutoDetect_RealWorldCursor(t *testing.T) {
	// Save and restore original config
	originalEnabled := config.AutoDetectAPIFormat
	originalAction := config.AutoDetectAPIFormatAction
	config.AutoDetectAPIFormat = true
	config.AutoDetectAPIFormatAction = "transparent"
	defer func() {
		config.AutoDetectAPIFormat = originalEnabled
		config.AutoDetectAPIFormatAction = originalAction
	}()

	engine := gin.New()
	engine.Use(APIFormatAutoDetect(engine))

	chatCompletionsCalled := false
	responseAPICalled := false
	var receivedBody []byte

	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		chatCompletionsCalled = true
		c.JSON(http.StatusOK, gin.H{"handler": "chat_completions"})
	})
	engine.POST("/v1/responses", func(c *gin.Context) {
		responseAPICalled = true
		receivedBody, _ = io.ReadAll(c.Request.Body)
		c.JSON(http.StatusOK, gin.H{"handler": "responses"})
	})

	// Simulate Cursor sending Response API format to chat/completions
	cursorRequest := `{
		"model": "claude-3-5-sonnet-20241022",
		"input": [
			{"type": "input_text", "text": "Write a hello world program in Python"}
		],
		"max_output_tokens": 8096,
		"stream": true
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(cursorRequest))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.False(t, chatCompletionsCalled, "chat completions should NOT be called")
	require.True(t, responseAPICalled, "responses handler should be called")
	require.NotEmpty(t, receivedBody, "body should be passed to the correct handler")
}
