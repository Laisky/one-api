package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// claudeAliasPrefixes are the Claude Code alias prefixes the shipped router rewrites.
var claudeAliasPrefixes = []string{
	"/v1/v1/messages",
	"/openai/v1/messages",
	"/openai/v1/v1/messages",
	"/api/v1/v1/messages",
}

// setupTestEngine returns an engine whose canonical Messages routes answer
// "generation" and "count_tokens", with every alias rewrite installed.
func setupTestEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	for _, prefix := range claudeAliasPrefixes {
		r.Use(RewriteClaudeMessagesPrefix(prefix, r))
	}
	v1 := r.Group("/v1")
	v1.POST("/messages", func(c *gin.Context) { c.String(http.StatusOK, "generation") })
	v1.POST("/messages/count_tokens", func(c *gin.Context) { c.String(http.StatusOK, "count_tokens") })
	return r
}

// TestRewriteClaudeMessagesPrefix proves exact aliases reach their canonical
// endpoint: the bare alias the generation route, and the count_tokens alias the
// free count_tokens route.
func TestRewriteClaudeMessagesPrefix(t *testing.T) {
	t.Parallel()
	engine := setupTestEngine()

	for _, prefix := range claudeAliasPrefixes {
		for path, want := range map[string]string{
			prefix:                        "generation",
			prefix + "/":                  "generation",
			prefix + "/count_tokens":      "count_tokens",
			prefix + "/count_tokens/":     "count_tokens",
			prefix + "/count_tokens?a=b":  "count_tokens",
			prefix + "?beta=true":         "generation",
			prefix + "/count_tokens?beta": "count_tokens",
		} {
			req := httptest.NewRequest(http.MethodPost, path, nil)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, "expected 200 for %s", path)
			require.Equal(t, want, w.Body.String(), "unexpected handler for %s", path)
		}
	}
}

// TestRewriteClaudeMessagesPrefixRejectsNonAliases proves a prefix match alone
// never rewrites: other sub-resources and longer names are not aliases, so they
// can never be turned into a billed generation.
func TestRewriteClaudeMessagesPrefixRejectsNonAliases(t *testing.T) {
	t.Parallel()
	engine := setupTestEngine()

	for _, prefix := range claudeAliasPrefixes {
		for _, path := range []string{
			prefix + "x",
			prefix + "/batches",
			prefix + "/count_tokens/extra",
			prefix + "/count_tokensx",
			prefix + "//",
		} {
			req := httptest.NewRequest(http.MethodPost, path, nil)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code, "expected 404 for %s, got %q", path, w.Body.String())
		}
	}
}

// TestRewriteNonMatchingPassThrough proves unrelated routes are untouched.
func TestRewriteNonMatchingPassThrough(t *testing.T) {
	t.Parallel()
	engine := setupTestEngine()
	// Register an unrelated route to ensure pass-through works
	engine.GET("/healthz", func(c *gin.Context) { c.String(200, "healthy") })

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "expected 200")
	require.Equal(t, "healthy", w.Body.String(), "unexpected body")
}

// TestClaudeMessagesAliasTarget covers the alias mapping table directly.
func TestClaudeMessagesAliasTarget(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		path   string
		target string
		ok     bool
	}{
		{"/v1/v1/messages", ClaudeMessagesPath, true},
		{"/v1/v1/messages/", ClaudeMessagesPath, true},
		{"/v1/v1/messages/count_tokens", ClaudeCountTokensPath, true},
		{"/v1/v1/messages/count_tokens/", ClaudeCountTokensPath, true},
		{"/v1/v1/messages/batches", "", false},
		{"/v1/v1/messagesx", "", false},
		{"/v1/v1/messages/count_tokens/x", "", false},
		{"/v1/messages", "", false},
		{"/other", "", false},
	} {
		target, ok := claudeMessagesAliasTarget(tc.path, "/v1/v1/messages")
		require.Equal(t, tc.ok, ok, tc.path)
		require.Equal(t, tc.target, target, tc.path)
	}
}
