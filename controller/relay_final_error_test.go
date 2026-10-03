package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	rcontroller "github.com/Laisky/one-api/relay/controller"
	"github.com/Laisky/one-api/relay/model"
)

// TestRelayFinalErrorDoesNotCorruptCommittedOutput checks actual Gin writes and
// retry eligibility for JSON, SSE and flushed headers without any usage receipt.
func TestRelayFinalErrorDoesNotCorruptCommittedOutput(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body string
	}{
		{"json", "application/json", `{"choices":[],"usage":null}`},
		{"sse", "text/event-stream", "data: {\"delta\":\"text\"}\n\n"},
		{"flushed_headers", "text/event-stream", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			c.Set(ctxkey.Channel, 1) // Prove this is not only a Jina-specific gate.
			require.True(t, rcontroller.BillingAllowsRetry(c))
			c.Header("Content-Type", tc.contentType)
			if tc.body != "" {
				_, err := c.Writer.WriteString(tc.body)
				require.NoError(t, err)
			}
			c.Writer.Flush()
			before := recorder.Body.String()
			apiErr := &model.ErrorWithStatusCode{StatusCode: http.StatusBadGateway, Error: model.Error{Message: "late receipt error"}}
			require.False(t, writeRelayFinalError(c, apiErr))
			require.False(t, rcontroller.BillingAllowsRetry(c), "committed output cannot authorize another paid dispatch even without usage")
			require.Equal(t, before, recorder.Body.String())
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, tc.contentType, recorder.Header().Get("Content-Type"))
		})
	}
}

// TestRelayFinalErrorBeforeOutput preserves normal JSON errors and writes exactly
// one response; selecting a status without flushing is not a committed response.
func TestRelayFinalErrorBeforeOutput(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Writer.WriteHeader(http.StatusOK)
	require.True(t, rcontroller.BillingAllowsRetry(c))
	apiErr := &model.ErrorWithStatusCode{StatusCode: http.StatusBadGateway, Error: model.Error{Message: "invalid usage"}}
	require.True(t, writeRelayFinalError(c, apiErr))
	require.Equal(t, http.StatusBadGateway, recorder.Code)
	require.Contains(t, recorder.Body.String(), "invalid usage")
	before := recorder.Body.String()
	require.False(t, writeRelayFinalError(c, apiErr))
	require.False(t, writeRelayFinalError(c, nil))
	require.False(t, writeRelayFinalError(nil, apiErr))
	require.Equal(t, before, recorder.Body.String())
}
