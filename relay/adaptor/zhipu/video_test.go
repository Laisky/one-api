package zhipu

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newVideoTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	return c, w
}

func TestVideoHandlerPassesThroughTaskResponse(t *testing.T) {
	t.Parallel()

	body := []byte(`{"id":"vidu_task_1","model":"viduq1-text","task_status":"PROCESSING","request_id":"req_1"}`)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	c, w := newVideoTestContext()

	bizErr, usage := VideoHandler(c, resp)
	require.Nil(t, bizErr)
	require.Nil(t, usage)
	require.Equal(t, http.StatusOK, c.Writer.Status())
	require.JSONEq(t, string(body), w.Body.String())
}

func TestVideoHandlerDetectsZhipuError(t *testing.T) {
	t.Parallel()

	body := []byte(`{"error":{"code":"1201","message":"model not found"}}`)
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
	c, _ := newVideoTestContext()

	bizErr, usage := VideoHandler(c, resp)
	require.NotNil(t, bizErr)
	require.Nil(t, usage)
	require.Equal(t, http.StatusBadRequest, bizErr.StatusCode)
	require.Equal(t, "model not found", bizErr.Error.Message)
}
