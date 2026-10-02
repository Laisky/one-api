package controller

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGeminiLiveDestinationIsChannelOwned tests the actual selected-channel to
// metadata boundary against caller URL/header/body injection. Parameters: t is
// the test handle. Returns: none. Operator-configured proxy URLs remain valid;
// no request parameter can change the selected channel's destination or key.
func TestGeminiLiveDestinationIsChannelOwned(t *testing.T) {
	t.Parallel()
	for _, override := range []bool{false, true} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		attack := "https://attacker.invalid/collect?key=caller-key"
		q := url.Values{"model": {"gemini-3.8-live"}, "base_url": {attack}, "endpoint": {attack}, "key": {"caller-key"}}
		c.Request = httptest.NewRequest("GET", "/v1/realtime?"+q.Encode(), strings.NewReader(`{"base_url":"https://attacker.invalid","endpoint_urls":{"realtime":"https://attacker.invalid"}}`))
		c.Request.Header.Set("Authorization", "Bearer caller-key")
		c.Request.Header.Set("X-Base-URL", attack)
		c.Request.Header.Set("X-Forwarded-Host", "attacker.invalid")
		c.Set(ctxkey.RequestModel, "gemini-3.8-live")
		gmw.SetLogger(c, logger.Logger)
		base := "https://generativelanguage.googleapis.com"
		channel := &model.Channel{Id: 903, Type: channeltype.Gemini, Name: "selected", Group: "default", BaseURL: &base, Key: "operator-owned-key"}
		if override {
			channel.Config = `{"endpoint_urls":{"realtime":"wss://operator-proxy.invalid/live"}}`
		}
		middleware.SetupContextForSelectedChannel(c, channel, "gemini-3.8-live")
		m := meta.GetByContext(c)
		require.Equal(t, relaymode.Realtime, m.Mode)
		require.Equal(t, "operator-owned-key", m.APIKey)
		endpoint, err := gemini.LiveRequestURL(m)
		require.NoError(t, err)
		if override {
			require.Equal(t, "wss://operator-proxy.invalid/live", endpoint)
		} else {
			require.Equal(t, "wss://generativelanguage.googleapis.com/ws/google.ai.generativelanguage.v1alpha.GenerativeService.BidiGenerateContent", endpoint)
		}
		require.NotContains(t, endpoint, "attacker")
		require.NotContains(t, endpoint, "key")
	}
}
