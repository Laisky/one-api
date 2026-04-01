package anthropic

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
)

// TestStreamHandler_ClientContextCanceledReturnsUsage verifies Anthropic streaming keeps
// backward-compatible return values when the downstream client disconnects before data arrives.
// Parameters:
//   - t: the test context.
//
// Returns:
//   - nothing.
func TestStreamHandler_ClientContextCanceledReturnsUsage(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	gmw.SetLogger(c, glog.Shared.Named("anthropic-stream-test"))

	pr, pw := io.Pipe()
	t.Cleanup(func() {
		_ = pw.Close()
	})

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     http.Header{},
	}

	errResp, usage := StreamHandler(c, resp)
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	require.Equal(t, 0, usage.PromptTokens)
	require.Equal(t, 0, usage.CompletionTokens)
	require.Contains(t, recorder.Body.String(), "[DONE]")
}
