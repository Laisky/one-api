package azure

import (
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestClaude431AzureAuthentication checks explicit administrator Entra tokens and API-key isolation.
func TestClaude431AzureAuthentication(t *testing.T) {
	t.Parallel()
	for _, bearer := range []bool{false, true} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
		c.Request.Header.Set("Authorization", "Bearer downstream-gateway-key")
		c.Request.Header.Set("X-Api-Key", "downstream-untrusted-key")
		m := &meta.Meta{ChannelType: channeltype.Azure, OriginModelName: "claude-sonnet-5-5", ActualModelName: "prod", APIKey: "synthetic-upstream-key"}
		if bearer {
			m.Config.CustomHeaders = map[string]string{"authorization": "Bearer {{key}}"}
		}
		out := httptest.NewRequest(http.MethodPost, "https://example.invalid/anthropic/v1/messages", nil)
		require.NoError(t, (&Adaptor{}).SetupRequestHeader(c, out, m))
		if bearer {
			require.Equal(t, "Bearer synthetic-upstream-key", out.Header.Get("Authorization"))
			require.Empty(t, out.Header.Get("X-Api-Key"))
			require.Empty(t, out.Header.Get("Api-Key"))
		} else {
			require.Equal(t, "synthetic-upstream-key", out.Header.Get("X-Api-Key"))
			require.Empty(t, out.Header.Get("Authorization"))
		}
	}
}
