package openai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestHandlerDoesNotLogUpstreamContent verifies that successful upstream response
// content is forwarded to the client without being copied into structured logs.
func TestHandlerDoesNotLogUpstreamContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, observed := observer.New(zapcore.DebugLevel)
	lg, err := glog.NewConsoleWithName("test", glog.LevelDebug,
		zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	gmw.SetLogger(c, lg)

	const sentinel = "sentinel-private-upstream-content"
	responseBody := []byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"` + sentinel + `"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(responseBody)),
	}

	errResponse, usage := Handler(c, response, 1, "test-model")
	require.Nil(t, errResponse)
	require.NotNil(t, usage)
	require.Contains(t, recorder.Body.String(), sentinel)

	for _, entry := range observed.All() {
		logged, marshalErr := json.Marshal(entry.ContextMap())
		require.NoError(t, marshalErr)
		require.NotContains(t, entry.Message, sentinel)
		require.NotContains(t, string(logged), sentinel)
	}
}
