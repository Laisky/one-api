package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupTestEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Canonical handler
	v1 := r.Group("/v1")
	v1.POST("/messages", func(c *gin.Context) { c.String(200, "ok") })
	return r
}

func TestRewriteClaudeMessagesPrefix(t *testing.T) {
	t.Parallel()
	engine := setupTestEngine()
	engine.Use(RewriteClaudeMessagesPrefix("/v1/v1/messages", engine))
	engine.Use(RewriteClaudeMessagesPrefix("/openai/v1/messages", engine))
	engine.Use(RewriteClaudeMessagesPrefix("/openai/v1/v1/messages", engine))
	engine.Use(RewriteClaudeMessagesPrefix("/api/v1/v1/messages", engine))

	cases := []string{
		"/v1/v1/messages",
		"/openai/v1/messages",
		"/openai/v1/v1/messages",
		"/api/v1/v1/messages",
	}

	for _, path := range cases {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, "expected 200 for %s", path)
		require.Equal(t, "ok", w.Body.String(), "expected body 'ok' for %s", path)
	}
}

func TestRewriteNonMatchingPassThrough(t *testing.T) {
	t.Parallel()
	engine := setupTestEngine()
	engine.Use(RewriteClaudeMessagesPrefix("/v1/v1/messages", engine))
	// Register an unrelated route to ensure pass-through works
	engine.GET("/healthz", func(c *gin.Context) { c.String(200, "healthy") })

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "expected 200")
	require.Equal(t, "healthy", w.Body.String(), "unexpected body")
}
