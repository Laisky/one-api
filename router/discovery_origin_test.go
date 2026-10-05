package router

import (
	"bytes"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Laisky/one-api/common/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// discoveryTestBuild loads the shipped public assets into the production build
// layout for t and returns an immutable filesystem without fabricated metadata.
func discoveryTestBuild(t *testing.T) fs.FS {
	t.Helper()
	build := fstest.MapFS{}
	public := os.DirFS("../web/modern/public")
	if directory := os.Getenv("ONEAPI_DISCOVERY_BUILD_DIR"); directory != "" {
		public = os.DirFS(directory)
	}
	require.NoError(t, fs.WalkDir(public, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		require.NoError(t, walkErr)
		if entry.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(public, name)
		require.NoError(t, err)
		build[path.Join("web/build", config.Theme, name)] = &fstest.MapFile{Data: data}
		return nil
	}))
	return build
}

// TestDiscoveryOriginValidation checks administrator origin validation and
// confirms unsafe origins cannot be embedded in JSON, HTML, or HTTP headers.
func TestDiscoveryOriginValidation(t *testing.T) {
	for _, origin := range []string{"https://gateway.example", "http://localhost:3000", "http://[::1]:8000", "https://oneapi.laisky.com:8443"} {
		t.Run(origin, func(t *testing.T) {
			setDiscoveryOrigin(t, origin+"/")
			require.Equal(t, origin, configuredDiscoveryOrigin())
			require.Equal(t, origin+"/mcp", bindDiscoveryText(discoveryTemplateOrigin+"/mcp", origin))
		})
	}
	for _, origin := range []string{"", "//evil.example", "javascript:alert(1)", "https://user:secret@example.com", "https://example.com/path", "https://example.com?next=evil", "https://example.com#fragment", "https://example.com?", "https://example.com:0", "https://example.com:65536", "https://example.com:", "https://bad_label.example", "https://example.com\r\nLink: bad", "https://example.com\""} {
		t.Run("invalid_"+origin, func(t *testing.T) {
			setDiscoveryOrigin(t, origin)
			require.Empty(t, configuredDiscoveryOrigin())
			r := gin.New()
			r.Use(addAgentDiscoveryHeaders())
			r.GET("/metadata", servePreparedAgentData([]byte(`{"resource":"https://oneapi.laisky.com/mcp"}`), "application/json"))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metadata", nil))
			require.Equal(t, http.StatusServiceUnavailable, w.Code)
			require.Empty(t, w.Header().Get("Link"))
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		})
	}
}

// TestDiscoveryShippedRoutes binds production routes and every shipped textual
// discovery alias to two configured origins without trusting request headers.
func TestDiscoveryShippedRoutes(t *testing.T) {
	previousRateLimit := config.RateLimitDisabled
	config.RateLimitDisabled = true
	t.Cleanup(func() { config.RateLimitDisabled = previousRateLimit })
	build := discoveryTestBuild(t)
	r := gin.New()
	SetWebRouter(r, build)
	paths := []string{"/.well-known/mcp", "/.well-known/mcp/manifest.json", "/.well-known/mcp/server-card.json", "/.well-known/api-catalog", "/.well-known/api-catalog.json", "/.well-known/ai-catalog.json", "/.well-known/agent-card.json", "/.well-known/oauth-protected-resource", "/.well-known/oauth-authorization-server", "/openapi.json", "/swagger.json", "/openapi.json.md", "/docs", "/developers", "/api-reference", "/robots.txt", "/mcp-apps/public-discovery.html"}
	require.NoError(t, fs.WalkDir(os.DirFS("../web/modern/public"), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		require.NoError(t, walkErr)
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile("../web/modern/public/" + name)
		require.NoError(t, err)
		if bytes.Contains(data, []byte(discoveryTemplateOrigin)) {
			paths = append(paths, "/"+name)
		}
		return nil
	}))
	for _, origin := range []string{"https://first.example", "https://second.example:8443"} {
		setDiscoveryOrigin(t, origin)
		for _, endpoint := range paths {
			t.Run(origin+endpoint, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, "http://spoofed.example"+endpoint, nil)
				req.Header.Set("X-Forwarded-Host", "attacker.invalid")
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)
				require.NotContains(t, w.Body.String(), "oneapi.laisky.com")
				require.NotContains(t, w.Body.String(), "attacker.invalid")
				if endpoint != "/docs" && endpoint != "/developers" {
					require.Contains(t, w.Body.String(), origin)
				}
				require.Contains(t, w.Header().Get("Link"), origin+"/llms.txt")
				require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			})
		}
		for _, method := range []string{"initialize", "tools/call", "resources/list", "resources/read"} {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/.well-known/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"`+method+`"}`)))
			require.Equal(t, http.StatusOK, w.Code)
			require.Contains(t, w.Body.String(), origin)
			require.NotContains(t, w.Body.String(), "oneapi.laisky.com")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ask", nil))
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), origin)
		require.NotContains(t, w.Body.String(), "oneapi.laisky.com")
	}
}
