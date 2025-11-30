package openai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestHandlerUpdatesContentLengthAfterRewriting verifies Handler aligns Content-Length with the rewritten payload size.
// It accepts a *testing.T to record assertions and returns no values because Go testing
// functions signal failures through t.Fatalf/t.Errorf.
func TestHandlerUpdatesContentLengthAfterRewriting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	originalBody := []byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop","content_filter_results":{"hate":{"filtered":false,"severity":"safe"}}}],"usage":{"prompt_tokens":5,"completion_tokens":7,"total_tokens":12},"system_fingerprint":"fp_test"}`)
	upstream := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(originalBody)),
		Header: http.Header{
			"Content-Type":   []string{"application/json"},
			"Content-Length": []string{strconv.Itoa(len(originalBody))},
		},
	}

	errResp, _ := Handler(c, upstream, 0, "gpt-4o")
	require.Nil(t, errResp, "handler returned unexpected error")
	require.Equal(t, http.StatusOK, w.Code, "unexpected status code")

	var respBody SlimTextResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &respBody), "handler produced invalid JSON")

	bodyLen := len(w.Body.Bytes())
	require.Less(t, bodyLen, len(originalBody), "expected rewritten body to be smaller than upstream payload")

	headerLen := w.Header().Get("Content-Length")
	require.Equal(t, strconv.Itoa(bodyLen), headerLen, "content-length header does not match body size")
}
