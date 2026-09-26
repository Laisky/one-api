package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/model"
)

// TestVideoRetrySafety ensures an ambiguous paid creation is never replayed,
// without disabling pre-dispatch recovery or unrelated request retry policies.
func TestXAIVideoRetrySafety(t *testing.T) {
	for _, tc := range []struct {
		name, method, path string
		channel            int
		forwarded, retry   bool
	}{
		{"native_sent", "POST", "/v1/videos/generations", channeltype.XAI, true, false},
		{"alias_sent", "POST", "/v1/videos", channeltype.XAI, true, false},
		{"not_sent", "POST", "/v1/videos", channeltype.XAI, false, true},
		{"poll", "GET", "/v1/videos/job", channeltype.XAI, true, true},
		{"chat", "POST", "/v1/chat/completions", channeltype.XAI, true, true},
		{"any_provider", "POST", "/v1/videos", channeltype.OpenAI, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(tc.method, tc.path, nil)
			c.Set(ctxkey.Channel, tc.channel)
			c.Set(ctxkey.UpstreamRequestPossiblyForwarded, tc.forwarded)
			err := shouldRetry(c, &model.ErrorWithStatusCode{StatusCode: http.StatusBadGateway})
			if tc.retry {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
