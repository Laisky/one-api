package vertexai

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// vertexLiveTestMeta builds syntactically valid operator settings. Parameters:
// none. Returns: metadata without performing credential or entitlement discovery.
func vertexLiveTestMeta() *meta.Meta {
	m := &meta.Meta{ChannelType: channeltype.VertextAI, Mode: relaymode.Realtime, ActualModelName: "gemini-3.8-live"}
	m.Config.VertexAIProjectID = "fixture-project"
	m.Config.VertexAIADC = `{"type":"service_account","client_email":"fixture@example.invalid"}`
	return m
}

// TestVertexLiveRoutingUsesConfiguredLocation verifies default, regional, global,
// versioned and private-gateway routing. Parameters: t owns the test. Returns:
// none; ordinary REST's model-version global override is deliberately not reused.
func TestVertexLiveRoutingUsesConfiguredLocation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, location, version, base, want string
	}{
		{"default", "", "", "", "wss://us-central1-aiplatform.googleapis.com/ws/google.cloud.aiplatform.v1.LlmBidiService/BidiGenerateContent"},
		{"regional", "europe-west4", "", "", "wss://europe-west4-aiplatform.googleapis.com/ws/google.cloud.aiplatform.v1.LlmBidiService/BidiGenerateContent"},
		{"global explicit", "global", "v1beta1", "", "wss://aiplatform.googleapis.com/ws/google.cloud.aiplatform.v1beta1.LlmBidiService/BidiGenerateContent"},
		{"gateway prefix", "asia-northeast1", "v1beta1", "https://gateway.example.test/prefix/v1/", "wss://gateway.example.test/prefix/ws/google.cloud.aiplatform.v1beta1.LlmBidiService/BidiGenerateContent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := vertexLiveTestMeta()
			m.Config.Region, m.Config.APIVersion, m.BaseURL = tc.location, tc.version, tc.base
			got, err := LiveRequestURL(m)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			m.ActualModelName = "operator-configured-native-model"
			got, err = LiveRequestURL(m)
			require.NoError(t, err)
			require.Equal(t, tc.want, got, "the catalog is not a transport allowlist")
		})
	}
}

// TestVertexLiveEndpointSafety distinguishes syntax/security checks from access
// policy. Parameters: t owns the test. Returns: none; no hosts are contacted.
func TestVertexLiveEndpointSafety(t *testing.T) {
	t.Parallel()
	for _, edit := range []func(*meta.Meta){
		func(m *meta.Meta) { m.Config.VertexAIProjectID = "" },
		func(m *meta.Meta) { m.Config.VertexAIProjectID = "other/project" },
		func(m *meta.Meta) { m.Config.Region = "../global" },
		func(m *meta.Meta) { m.ActualModelName = "../other-model" },
		func(m *meta.Meta) { m.Config.APIVersion = "v1alpha" },
		func(m *meta.Meta) { m.BaseURL = "http://remote.example.test" },
		func(m *meta.Meta) { m.BaseURL = "https://secret@example.test" },
		func(m *meta.Meta) { m.BaseURL = "https://example.test?key=secret" },
		func(m *meta.Meta) { m.BaseURL = "https://example.test#fragment" },
	} {
		m := vertexLiveTestMeta()
		edit(m)
		_, err := LiveRequestURL(m)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret")
	}
}

// TestVertexLiveUpstreamDenialsAreNotLocalModelRestrictions checks real HTTP
// upgrade rejections, OAuth isolation and sanitized errors. Parameters: t owns
// the servers and token cache entries. Returns: none; no Google account is used.
func TestVertexLiveUpstreamDenialsAreNotLocalModelRestrictions(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 429, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Header.Get("Authorization") != "Bearer provider-fixture" || r.Header.Get("X-Goog-Api-Key") != "" || r.URL.RawQuery != "" {
					t.Error("upstream credentials were not isolated")
				}
				http.Error(w, "private-provider-detail", status)
			}))
			t.Cleanup(upstream.Close)
			m := vertexLiveTestMeta()
			m.ActualModelName = "operator-configured-native-model"
			m.ChannelId, m.BaseURL = 984300+status, upstream.URL
			cacheKey := fmt.Sprintf("vertexai-token-%d", m.ChannelId)
			Cache.Set(cacheKey, "provider-fixture", time.Minute)
			t.Cleanup(func() { Cache.Delete(cacheKey) })
			engine := gin.New()
			engine.GET("/v1/realtime", func(c *gin.Context) {
				gmw.SetLogger(c, logger.Logger)
				biz, usage := LiveHandler(c, m)
				if biz == nil || usage != nil {
					t.Error("denial was not returned before downstream upgrade")
					c.Status(http.StatusInternalServerError)
					return
				}
				c.JSON(biz.StatusCode, gin.H{"error": biz.Error})
			})
			gateway := httptest.NewServer(engine)
			t.Cleanup(gateway.Close)
			req, err := http.NewRequest(http.MethodGet, gateway.URL+"/v1/realtime", nil)
			require.NoError(t, err)
			req.Header.Set("Connection", "Upgrade")
			req.Header.Set("Upgrade", "websocket")
			req.Header.Set("Authorization", "Bearer caller-fixture")
			resp, err := gateway.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, status, resp.StatusCode)
			require.EqualValues(t, 1, calls.Load())
			require.Contains(t, string(body), "gemini_live_connect")
			for _, secret := range []string{"provider-fixture", "caller-fixture", "private-provider-detail"} {
				require.False(t, strings.Contains(string(body), secret))
			}
		})
	}
}
