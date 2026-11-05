package baidu

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/one-api/relay/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// responseWriteFailure records the response before rejecting client delivery.
type responseWriteFailure struct {
	gin.ResponseWriter
	cause  error
	body   []byte
	writes int
}

// Write records the encoded payload and reports the selected failure.
func (w *responseWriteFailure) Write(p []byte) (int, error) {
	w.writes++
	w.body = append([]byte(nil), p...)
	return 0, w.cause
}

// TestHandlerResponseWriteFailure preserves usage and response metadata on failed client delivery.
func TestHandlerResponseWriteFailure(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		run           func(*gin.Context, *http.Response) (*model.ErrorWithStatusCode, *model.Usage)
	}{{name: "chat", payload: `{"id":"id","result":"hello","usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`, run: func(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
		return Handler(c, resp)
	}},
		{name: "embedding", payload: `{"data":[{"object":"embedding","index":0,"embedding":[1]}],"usage":{"prompt_tokens":7,"total_tokens":7}}`, run: func(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
			return EmbeddingHandler(c, resp)
		}}} {
		t.Run(tc.name, func(t *testing.T) {
			control := httptest.NewRecorder()
			controlCtx, _ := gin.CreateTestContext(control)
			controlCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			controlBody := &externalFailureBody{reader: strings.NewReader(tc.payload)}
			controlFailure, controlUsage := tc.run(controlCtx, &http.Response{StatusCode: http.StatusOK, Body: controlBody})
			require.Nil(t, controlFailure)
			require.NotNil(t, controlUsage)
			require.Equal(t, 1, controlBody.closes)
			cause := &externalFailure{}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			writer := &responseWriteFailure{ResponseWriter: c.Writer, cause: cause}
			c.Writer = writer
			body := &externalFailureBody{reader: strings.NewReader(tc.payload)}
			failure, usage := tc.run(c, &http.Response{StatusCode: http.StatusOK, Body: body})
			require.NotNil(t, failure)
			require.Equal(t, http.StatusInternalServerError, failure.StatusCode)
			require.Equal(t, "write_response_body_failed", failure.Code)
			require.Equal(t, model.ErrorTypeOneAPI, failure.Type)
			require.Equal(t, cause.Error(), failure.Message)
			require.ErrorIs(t, failure.RawError, cause)
			require.Contains(t, fmt.Sprintf("%+v", failure.RawError), "baidu.")
			require.NotNil(t, usage)
			require.Equal(t, 7, usage.PromptTokens)
			require.Equal(t, controlUsage, usage)
			require.Equal(t, 1, body.closes)
			require.Equal(t, 1, writer.writes)
			require.Equal(t, http.StatusOK, c.Writer.Status())
			require.Equal(t, "application/json", c.Writer.Header().Get("Content-Type"))
			require.True(t, json.Valid(writer.body))
			var expected, actual map[string]any
			require.NoError(t, json.Unmarshal(control.Body.Bytes(), &expected))
			require.NoError(t, json.Unmarshal(writer.body, &actual))
			delete(expected, "id")
			delete(actual, "id")
			delete(expected, "created")
			delete(actual, "created")
			require.Equal(t, expected, actual)
		})
	}
}
