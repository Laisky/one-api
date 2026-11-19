package openai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGPT6CyberReceiptKeepsNativeCancellationOwnership dispatches a real native
// HTTP request, wraps its body with the Cyber receipt observer, and proves the
// wrapper is transparent to transport cancellation provenance. Without Unwrap,
// stream handlers would install a second close-on-cancel owner that can turn a
// client cancellation into a spurious net.ErrClosed failure.
func TestGPT6CyberReceiptKeepsNativeCancellationOwnership(t *testing.T) {
	previous := config.TraceSinks
	config.TraceSinks = []string{config.TraceSinkNone}
	t.Cleanup(func() { config.TraceSinks = previous })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"service_tier\":\"default\",\"usage\":{\"input_tokens\":20,\"output_tokens\":2}}}\n\n")
	}))
	t.Cleanup(server.Close)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	// Native provenance requires a cancellable client request context.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{}`)).WithContext(ctx)
	info := &metalib.Meta{ChannelType: channeltype.OpenAI, APIKey: "fixture", BaseURL: server.URL,
		ActualModelName: "gpt-6-cyber", OriginModelName: "gpt-6-cyber", Mode: relaymode.ResponseAPI,
		RequestURLPath: "/v1/responses", IsStream: true, StartTime: time.Now().UTC()}
	resp, err := adaptor.DoRequestHelper(&Adaptor{ChannelType: channeltype.OpenAI}, c, info, strings.NewReader(`{"model":"gpt-6-cyber"}`))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.True(t, adaptor.ResponseTransportOwnsCancellation(c, resp), "control: the native body owns cancellation")

	receipt := observeRawUsageReceipt(resp, info)
	require.NotNil(t, receipt)
	require.Same(t, receipt, resp.Body)
	require.True(t, adaptor.ResponseTransportOwnsCancellation(c, resp), "the receipt wrapper must stay transparent")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "response.completed", "observation must not alter delivered bytes")
}
