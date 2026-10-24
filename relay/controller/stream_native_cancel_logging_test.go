package controller

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	relayadaptor "github.com/Laisky/one-api/relay/adaptor"
)

// TestIssue599LoggingBodyPreservesNativeCancellation exercises the actual
// controller response wrapper on a native HTTP/1.1 connection. It must remain
// visible as a transparent reader while continuing to count every byte.
func TestIssue599LoggingBodyPreservesNativeCancellation(t *testing.T) {
	const wire = "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":7}}\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, wire)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil).WithContext(ctx)
	lg, err := glog.New(glog.WithName("issue599-native-cancel"))
	require.NoError(t, err)
	gmw.SetLogger(c, lg)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	resp, err := relayadaptor.DoRequest(c, req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.True(t, relayadaptor.ResponseTransportOwnsCancellation(c, resp))
	capture := wrapUpstreamResponse(resp)
	require.NotNil(t, capture)
	require.True(t, relayadaptor.ResponseTransportOwnsCancellation(c, resp),
		"controller logging must not hide the transport's proven cancellation owner")

	payload := make([]byte, len(wire))
	_, err = io.ReadFull(resp.Body, payload)
	require.NoError(t, err)
	require.Equal(t, wire, string(payload))
	cancel()
	_, err = io.ReadAll(resp.Body)
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, resp.Body.Close())
	_, _, total := capture.Snapshot()
	require.Equal(t, len(wire), total)
}
