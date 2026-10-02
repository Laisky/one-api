package controller

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestReviewProxyRejectsPaidChannels observes actual forwarding and the real
// user balance; no URL expectation alone is accepted as proof of billing safety.
func TestReviewProxyRejectsPaidChannels(t *testing.T) {
	const balance int64 = 1000000
	billingAccountingSetup(t, balance)
	var hits atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"paid output"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer upstream.Close()
	previous := client.HTTPClient
	u, parseErr := url.Parse(upstream.URL)
	require.NoError(t, parseErr)
	client.HTTPClient = &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, u.Host)
		},
	}}
	t.Cleanup(func() { drainCriticalTasks(t); client.HTTPClient = previous })
	for _, tc := range []struct {
		name         string
		channel, api int
		base         string
	}{
		{"openai-compatible", channeltype.OpenAICompatible, apitype.OpenAI, upstream.URL},
		{"openai", channeltype.OpenAI, apitype.OpenAI, upstream.URL},
		{"azure", channeltype.Azure, apitype.OpenAI, upstream.URL},
		{"github-models", channeltype.OpenAICompatible, apitype.OpenAI, "http://models.github.ai"},
		{"forged-api-type", channeltype.OpenAICompatible, apitype.Proxy, upstream.URL},
		{"forged-channel-type", channeltype.Proxy, apitype.OpenAI, upstream.URL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/oneapi/proxy/1/chat/completions", strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set(ctxkey.Id, fallbackUserID)
			c.Set(ctxkey.RequestId, "proxy-security-"+tc.name)
			m := &meta.Meta{Mode: relaymode.Proxy, ChannelType: tc.channel, APIType: tc.api,
				BaseURL: tc.base, RequestURLPath: c.Request.URL.RequestURI(), ChannelId: fallbackChannelID, UserId: fallbackUserID,
				TokenId: fallbackTokenID, TokenName: "fallback-token", ActualModelName: "gpt-4o", StartTime: time.Now(),
				Config: model.ChannelConfig{APIVersion: "2025-04-01-preview"}}
			meta.Set2Context(c, m)
			before := hits.Load()
			err := RelayProxyHelper(c, relaymode.Proxy)
			drainCriticalTasks(t)
			require.Equal(t, before, hits.Load(), "paid upstream must not be reached via zero-quota proxy")
			require.NotNil(t, err)
			require.Equal(t, http.StatusForbidden, err.StatusCode)
			require.Equal(t, "proxy_channel_required", err.Code)
			require.False(t, c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded))
			require.Equal(t, balance, reloadUserQuota(t), "rejected requests must not charge users")
		})
	}
}

// TestReviewProxyPreservesExplicitProxyChannel verifies the deliberate no-quota
// operator feature still forwards the original payload to a real upstream.
func TestReviewProxyPreservesExplicitProxyChannel(t *testing.T) {
	const balance int64 = 1000000
	billingAccountingSetup(t, balance)
	received := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		received <- r.URL.RequestURI() + "|" + string(b) + "|" + r.Header.Get("Authorization")
		_, _ = io.WriteString(w, "fixture-proxy-response")
	}))
	defer upstream.Close()
	previous := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { drainCriticalTasks(t); client.HTTPClient = previous })
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/oneapi/proxy/1/health?check=1", strings.NewReader("fixture-body"))
	c.Set(ctxkey.Id, fallbackUserID)
	c.Set(ctxkey.RequestId, "proxy-security-positive")
	meta.Set2Context(c, &meta.Meta{Mode: relaymode.Proxy, ChannelType: channeltype.Proxy, APIType: apitype.Proxy,
		ChannelId: 1, UserId: fallbackUserID, TokenId: fallbackTokenID, TokenName: "fallback-token", APIKey: "fixture-provider-key",
		BaseURL: upstream.URL, RequestURLPath: "/v1/oneapi/proxy/1/health?check=1", StartTime: time.Now()})
	require.Nil(t, RelayProxyHelper(c, relaymode.Proxy))
	drainCriticalTasks(t)
	require.Equal(t, "/health?check=1|fixture-body|fixture-provider-key", <-received)
	require.Equal(t, "fixture-proxy-response", w.Body.String())
	require.Equal(t, balance, reloadUserQuota(t))
}
