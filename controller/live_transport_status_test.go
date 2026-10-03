package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestLiveOnlyModelOverRESTIsClientError asserts the status a caller sees when it
// posts a Live-only model to a REST endpoint. Parameters: t is the test handle.
// Returns: none. Google answers the same request with HTTP 400, and the choice
// matters beyond the status text: a 5xx is retried onto further channels and is
// attributed to channel health, so a caller mistake could suspend a healthy
// Gemini channel. Non-Live Gemini models must keep flowing to the adaptor.
func TestLiveOnlyModelOverRESTIsClientError(t *testing.T) {
	for _, tc := range []struct {
		name        string
		channelType int
		model       string
		clientError bool
	}{
		{"gemini_live", channeltype.Gemini, "gemini-3.8-live", true},
		{"gemini_live_extended", channeltype.Gemini, "gemini-3.8-live-extended-thinking", true},
		{"gemini_live_preview", channeltype.Gemini, "gemini-3.1-flash-live-preview", true},
		{"gemini_transcribe_live", channeltype.GeminiOpenAICompatible, "gemini-3.5-transcribe-live", true},
		{"vertex_live", channeltype.VertextAI, "gemini-3.8-live", true},
		{"third_party_bridge", channeltype.OpenAICompatible, "gemini-3.8-live", false},
		{"ordinary_gemini", channeltype.Gemini, "gemini-3.8-flash", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
				strings.NewReader(`{"model":"`+tc.model+`","messages":[{"role":"user","content":"hi"}]}`))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set(ctxkey.RequestModel, tc.model)
			gmw.SetLogger(c, logger.Logger)
			base := "https://generativelanguage.googleapis.com"
			middleware.SetupContextForSelectedChannel(c, &model.Channel{
				Id: 904, Type: tc.channelType, Name: "selected", Group: "default", BaseURL: &base, Key: "operator-owned-key",
			}, tc.model)

			bizErr := relayHelper(c, relaymode.ChatCompletions)
			if !tc.clientError {
				// Everything else is dispatched; this test fixture has no upstream,
				// so it must fail somewhere other than the transport guard.
				if bizErr != nil {
					require.NotEqual(t, "unsupported_model_transport", bizErr.Code)
				}
				return
			}
			require.NotNil(t, bizErr, "a Live-only model must not reach the REST adaptor")
			require.Equal(t, http.StatusBadRequest, bizErr.StatusCode)
			require.Equal(t, "unsupported_model_transport", bizErr.Code)
			require.Contains(t, bizErr.Message, "Live API")
			require.Contains(t, bizErr.Message, "/v1/realtime")
		})
	}
}

// TestRelayHelperAllowsLiveOnlyModelForRealtime verifies that the REST-only
// transport guard does not intercept a native Gemini Live request. Parameters:
// t is the test handle. Returns: none.
func TestRelayHelperAllowsLiveOnlyModelForRealtime(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime?model=gemini-3.8-live", nil)
	c.Set(ctxkey.RequestModel, "gemini-3.8-live")
	gmw.SetLogger(c, logger.Logger)
	base := "https://generativelanguage.googleapis.com"
	middleware.SetupContextForSelectedChannel(c, &model.Channel{
		Id: 905, Type: channeltype.Gemini, Name: "selected", Group: "default", BaseURL: &base, Key: "operator-owned-key",
	}, "gemini-3.8-live")

	called := false
	relayHelperForTest = func(*gin.Context, int) *relaymodel.ErrorWithStatusCode {
		called = true
		return nil
	}
	t.Cleanup(func() { relayHelperForTest = nil })

	require.Nil(t, relayHelper(c, relaymode.Realtime))
	require.True(t, called)
}

// TestLiveOnlyRESTMismatchDoesNotCountAgainstChannelHealth verifies that an
// excluded Google REST channel remains healthy while routing continues to a
// bridge. Parameters: t is the test handle. Returns: none.
func TestLiveOnlyRESTMismatchDoesNotCountAgainstChannelHealth(t *testing.T) {
	err := adaptor.ValidateRESTModelTransport(&meta.Meta{
		ChannelType:     channeltype.Gemini,
		ActualModelName: "gemini-3.8-live",
	})
	require.Error(t, err)
	require.False(t, countsAgainstChannelHealth(&relaymodel.ErrorWithStatusCode{
		StatusCode: http.StatusBadRequest,
		Error: relaymodel.Error{
			Type:     relaymodel.ErrorTypeOneAPI,
			RawError: err,
		},
	}))
}
