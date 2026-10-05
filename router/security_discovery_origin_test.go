package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Laisky/one-api/common/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// discoveryTransport restricts the synthetic metadata-following client to local fixtures.
type discoveryTransport func(*http.Request) (*http.Response, error)

// RoundTrip sends one synthetic request through the explicitly supplied local transport.
func (f discoveryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// setDiscoveryOrigin changes the administrator setting and restores it after a test.
func setDiscoveryOrigin(t *testing.T, value string) {
	t.Helper()
	config.OptionMapRWMutex.Lock()
	previous := config.ServerAddress
	config.ServerAddress = value
	config.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		config.OptionMapRWMutex.Lock()
		config.ServerAddress = previous
		config.OptionMapRWMutex.Unlock()
	})
}

// TestSecurityDiscoveryOriginHTTP uses the actual shipped manifest and production
// discovery handlers. The deliberately metadata-following client demonstrates
// conditional credential routing, not the behavior of every MCP client.
func TestSecurityDiscoveryOriginHTTP(t *testing.T) {
	manifest, err := os.ReadFile("../web/modern/public/mcp-manifest.json")
	require.NoError(t, err)
	for _, name := range []string{"deployment_a", "deployment_b"} {
		t.Run(name, func(t *testing.T) {
			const credential = "Bearer synthetic-discovery-fixture-only"
			var localCalls, otherCalls atomic.Int32
			other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") == credential {
					otherCalls.Add(1)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			t.Cleanup(other.Close)
			router := gin.New()
			router.Use(addAgentDiscoveryHeaders())
			router.GET("/.well-known/mcp/manifest.json", servePreparedAgentData(manifest, "application/json"))
			router.POST("/mcp", func(c *gin.Context) {
				if c.GetHeader("Authorization") == credential {
					localCalls.Add(1)
				}
				c.Status(http.StatusNoContent)
			})
			server := httptest.NewServer(router)
			t.Cleanup(server.Close)
			setDiscoveryOrigin(t, server.URL)
			req, err := http.NewRequest(http.MethodGet, server.URL+"/.well-known/mcp/manifest.json", nil)
			require.NoError(t, err)
			req.Host = "attacker.invalid"
			req.Header.Set("X-Forwarded-Host", "attacker.invalid")
			req.Header.Set("Forwarded", "host=attacker.invalid;proto=https")
			response, err := server.Client().Do(req)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, http.StatusOK, response.StatusCode)
			var parsed struct {
				AuthenticatedServer struct {
					URL string `json:"url"`
				} `json:"authenticatedServer"`
			}
			require.NoError(t, json.NewDecoder(response.Body).Decode(&parsed))
			otherURL, err := url.Parse(other.URL)
			require.NoError(t, err)
			localURL, err := url.Parse(server.URL)
			require.NoError(t, err)
			client := &http.Client{Transport: discoveryTransport(func(request *http.Request) (*http.Response, error) {
				copyRequest := request.Clone(request.Context())
				copyURL := *request.URL
				copyRequest.URL = &copyURL
				switch request.URL.Host {
				case "oneapi.laisky.com":
					// Represent the separately operated hosted origin with a local server.
					copyURL.Scheme, copyURL.Host = otherURL.Scheme, otherURL.Host
				case localURL.Host:
				default:
					return nil, fmt.Errorf("fixture rejects non-local destination %q", request.URL.Host)
				}
				return server.Client().Transport.RoundTrip(copyRequest)
			})}
			call, err := http.NewRequest(http.MethodPost, parsed.AuthenticatedServer.URL, bytes.NewBufferString(`{}`))
			require.NoError(t, err)
			call.Header.Set("Authorization", credential)
			result, err := client.Do(call)
			require.NoError(t, err)
			require.NoError(t, result.Body.Close())
			if otherCalls.Load() != 0 {
				t.Log("REPRODUCED_490_CONDITIONAL_SYNTHETIC_CREDENTIAL_MISROUTING")
			}
			require.Zero(t, otherCalls.Load(), "a local credential must not be advertised to a separate deployment")
			require.EqualValues(t, 1, localCalls.Load())
			require.Equal(t, server.URL+"/mcp", parsed.AuthenticatedServer.URL)
			require.Contains(t, response.Header.Get("Link"), server.URL+"/llms.txt")
			require.False(t, strings.Contains(response.Header.Get("Link"), "attacker.invalid"))
		})
	}
}
