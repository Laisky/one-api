package router

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// TestDiscoveryStaticAssetCachePolicy checks the production router's cache and
// Link policy for ordinary assets while retaining deployment-bound discovery.
func TestDiscoveryStaticAssetCachePolicy(t *testing.T) {
	previousTheme, previousRateLimit, previousMode := config.Theme, config.RateLimitDisabled, gin.Mode()
	config.Theme, config.RateLimitDisabled = "modern", true
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		config.Theme, config.RateLimitDisabled = previousTheme, previousRateLimit
		gin.SetMode(previousMode)
	})
	build, ok := discoveryTestBuild(t).(fstest.MapFS)
	require.True(t, ok)
	assets := map[string][]byte{
		"/assets/cache-policy.js":  []byte(`console.log("local cache fixture");`),
		"/assets/cache-policy.css": []byte(`body { color: black; }`),
		"/assets/cache-policy.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`),
		"/cache-policy.ico":        []byte("local icon fixture"),
	}
	for endpoint, data := range assets {
		build[path.Join("web/build/modern", endpoint)] = &fstest.MapFile{Data: data}
	}
	// When the existing build opt-in is set, exercise real Vite JS and CSS too.
	selected := map[string]bool{}
	require.NoError(t, fs.WalkDir(build, "web/build/modern", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		ext := path.Ext(name)
		if entry.IsDir() || (ext != ".js" && ext != ".css") || selected[ext] || strings.Contains(name, "cache-policy") {
			return nil
		}
		data, err := fs.ReadFile(build, name)
		if err != nil {
			return err
		}
		endpoint := strings.TrimPrefix(name, "web/build/modern")
		assets[endpoint], selected[ext] = data, true
		t.Logf("built_asset=%s bytes=%d sha256=%x", endpoint, len(data), sha256.Sum256(data))
		return nil
	}))
	r := gin.New()
	SetWebRouter(r, build)
	for _, origin := range []string{"https://first.example", "https://second.example:8443"} {
		setDiscoveryOrigin(t, origin)
		for endpoint, data := range assets {
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				t.Run(origin+method+endpoint, func(t *testing.T) {
					req := httptest.NewRequest(method, endpoint, nil)
					req.Host = "malformed host:bad-port"
					req.Header.Set("X-Forwarded-Host", "spoofed.invalid")
					req.Header.Set("Accept", "text/markdown")
					w := httptest.NewRecorder()
					r.ServeHTTP(w, req)
					t.Logf("origin=%q method=%s path=%q status=%d cache=%q link=%q", origin, method, endpoint, w.Code, w.Header().Get("Cache-Control"), w.Header().Get("Link"))
					require.Equal(t, http.StatusOK, w.Code)
					require.Equal(t, "max-age=604800", w.Header().Get("Cache-Control"))
					if method == http.MethodGet {
						require.True(t, bytes.Equal(data, w.Body.Bytes()), "static bytes must remain unchanged")
					} else {
						require.Empty(t, w.Body.Bytes())
					}
					require.Empty(t, w.Header().Get("Link"), "ordinary cacheable assets must not retain origin-dependent discovery links")
				})
			}
		}
		t.Run(origin+"/assets-directory", func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/assets", nil))
			require.Equal(t, http.StatusMovedPermanently, w.Code)
			require.Equal(t, "assets/", w.Header().Get("Location"))
			require.Equal(t, "max-age=604800", w.Header().Get("Cache-Control"))
			require.Empty(t, w.Header().Get("Link"))
		})
		for _, control := range []struct{ endpoint, accept string }{
			{"/", "text/html"}, {"/", "text/markdown"}, {"/?mode=agent", "text/html"},
			{"/llms.txt", "*/*"}, {"/openapi.json", "application/json"},
			{"/.well-known/api-catalog", "application/linkset+json"}, {"/missing-cache-policy.js", "text/html"},
		} {
			t.Run(origin+control.endpoint+control.accept, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, control.endpoint, nil)
				req.Host = "malformed host:bad-port"
				req.Header.Set("X-Forwarded-Host", "spoofed.invalid")
				req.Header.Set("Accept", control.accept)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code)
				require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
				require.Contains(t, w.Header().Get("Link"), origin+"/llms.txt")
				require.NotContains(t, w.Header().Get("Link"), "spoofed.invalid")
				require.NotContains(t, w.Body.String(), "malformed host")
			})
		}
	}
	setDiscoveryOrigin(t, "")
	for endpoint, data := range assets {
		t.Run("invalid_origin"+endpoint, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, endpoint, nil))
			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, "max-age=604800", w.Header().Get("Cache-Control"))
			require.Empty(t, w.Header().Get("Link"))
			require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(data)), fmt.Sprintf("%x", sha256.Sum256(w.Body.Bytes())))
		})
	}
}
