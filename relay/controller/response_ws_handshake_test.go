package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// newWSUpgradeContext builds a gin.Context whose Request looks like a
// well-formed WebSocket upgrade. It's enough to make websocket.IsWebSocketUpgrade
// return true so we can exercise maybeHandleResponseAPIWebSocket's early
// validation branches without standing up a full HTTP server.
func newWSUpgradeContext(t *testing.T, rawQuery string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/v1/responses?"+rawQuery, nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	c.Request = req
	return c, rec
}

// TestMaybeHandleResponseAPIWebSocket_RejectsMissingModel is the
// security-regression test for the legacy no-`?model=` path. Before this
// fix, a WS handshake with no model resulted in `meta.ActualModelName == ""`,
// the model guard was a no-op, and a client could then send any
// `response.create` with any model field. Billing fell back to whatever
// default the channel resolves to, producing the same under-billing class
// as gh #1 (realtime ephemeral) and the original Response WS report.
//
// The fix is to refuse the handshake at the very beginning when no model
// is bound. Clients must include `?model=` in the upgrade URL so billing
// can be pinned for the entire WS lifetime.
func TestMaybeHandleResponseAPIWebSocket_RejectsMissingModel(t *testing.T) {
	c, _ := newWSUpgradeContext(t, "" /* no model= */)

	meta := &metalib.Meta{
		ChannelType: channeltype.OpenAI,
		// ActualModelName intentionally left empty to exercise the gap.
	}

	handled, bizErr := maybeHandleResponseAPIWebSocket(c, meta)
	require.True(t, handled, "WS upgrade must be handled (not delegated to HTTP path)")
	require.NotNil(t, bizErr, "missing model MUST be rejected at handshake")
	require.Equal(t, http.StatusBadRequest, bizErr.StatusCode)
	require.Equal(t, "response_websocket_missing_model_query", bizErr.Error.Code,
		"error code must be machine-stable for client SDKs")
}

// TestMaybeHandleResponseAPIWebSocket_AcceptsWhitespaceOnlyModel guards
// against a client trying to bypass the missing-model check by passing
// `?model=%20` (whitespace). The bound model after trimming must be
// non-empty.
func TestMaybeHandleResponseAPIWebSocket_AcceptsWhitespaceOnlyModel(t *testing.T) {
	c, _ := newWSUpgradeContext(t, "model=%20%20%20")

	meta := &metalib.Meta{
		ChannelType:     channeltype.OpenAI,
		ActualModelName: "   ", // whitespace only — same effect as empty
	}

	handled, bizErr := maybeHandleResponseAPIWebSocket(c, meta)
	require.True(t, handled)
	require.NotNil(t, bizErr, "whitespace-only model must be rejected")
	require.Equal(t, http.StatusBadRequest, bizErr.StatusCode)
	require.Equal(t, "response_websocket_missing_model_query", bizErr.Error.Code)
}

// TestMaybeHandleResponseAPIWebSocket_NonOpenAIChannelStillRejectedFirst
// documents the priority of early checks: the OpenAI-channel guard fires
// before the missing-model check so non-OpenAI callers still see the
// existing error code, preserving back-compat for existing clients that
// rely on it.
func TestMaybeHandleResponseAPIWebSocket_NonOpenAIChannelStillRejectedFirst(t *testing.T) {
	c, _ := newWSUpgradeContext(t, "" /* missing model */)
	meta := &metalib.Meta{
		ChannelType: channeltype.Anthropic, // any non-OpenAI channel
	}

	handled, bizErr := maybeHandleResponseAPIWebSocket(c, meta)
	require.True(t, handled)
	require.NotNil(t, bizErr)
	require.Equal(t, "response_websocket_only_supported_for_openai_channel", bizErr.Error.Code,
		"non-OpenAI rejection must take precedence over the missing-model rejection")
}
