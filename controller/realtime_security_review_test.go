package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
)

// TestReviewRealtimeRetiredHandlerNeverMintsCredentials observes real upstream
// HTTP traffic, guarding against re-exposure through an alias or internal caller.
func TestReviewRealtimeRetiredHandlerNeverMintsCredentials(t *testing.T) {
	var hits atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"client_secret":{"value":"fixture-ephemeral-credential"}}`))
	}))
	defer upstream.Close()
	for _, withMeta := range []bool{true, false} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/realtime/sessions", strings.NewReader(`{"model":"gpt-realtime"}`))
		c.Request.Header.Set("Content-Type", "application/json")
		if withMeta {
			meta.Set2Context(c, &meta.Meta{APIType: apitype.OpenAI, ChannelType: channeltype.OpenAI,
				BaseURL: upstream.URL, APIKey: "fixture-provider-key"})
		}
		RelayRealtimeSessions(c)
		require.Equal(t, int64(0), hits.Load(), "retired handler must not contact upstream")
		require.Equal(t, http.StatusForbidden, w.Code)
		require.True(t, c.IsAborted())
		require.Contains(t, w.Body.String(), "realtime_sessions_disabled")
		require.NotContains(t, w.Body.String(), "fixture-ephemeral-credential")
	}
}
