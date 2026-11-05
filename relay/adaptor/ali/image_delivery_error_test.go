package ali

import (
	"encoding/json"
	stderrors "errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type imageRoundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip serves an in-memory Ali task receipt without contacting a provider.
func (f imageRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type imageDeliveryWriter struct {
	gin.ResponseWriter
	mode  string
	fault error
}

// Write injects downstream delivery failures without altering committed headers.
func (w *imageDeliveryWriter) Write(p []byte) (int, error) {
	if w.mode != "success" {
		w.ResponseWriter.WriteHeaderNow()
	}
	if w.mode == "failure" {
		return 0, w.fault
	}
	if w.mode == "short" {
		return len(p) - 1, nil
	}
	return w.ResponseWriter.Write(p)
}

// TestImageHandlerReportsDeliveryFailures checks actual async task handling and preserves status, response format and usage semantics.
func TestImageHandlerReportsDeliveryFailures(t *testing.T) {
	oldTransport := http.DefaultTransport
	http.DefaultTransport = imageRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "dashscope.aliyuncs.com", r.URL.Host)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"output":{"task_status":"SUCCEEDED","results":[{"url":"https://image.example/result.png"}]}}`)), Request: r}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	for _, mode := range []string{"success", "failure", "short"} {
		t.Run(mode, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/images", nil)
			c.Request.Header.Set("Authorization", "Bearer fixture")
			fault := stderrors.New("downstream disconnected")
			c.Writer = &imageDeliveryWriter{ResponseWriter: c.Writer, mode: mode, fault: fault}
			resp := &http.Response{StatusCode: http.StatusAccepted, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"output":{"task_id":"fixture-task"}}`))}
			apiErr, usage := ImageHandler(c, resp)
			require.Nil(t, usage)
			require.Equal(t, http.StatusAccepted, rec.Code)
			require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			if mode == "success" {
				require.Nil(t, apiErr)
				var response struct {
					Created int64 `json:"created"`
					Data    []struct {
						URL string `json:"url"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
				require.Positive(t, response.Created)
				require.Len(t, response.Data, 1)
				require.Equal(t, "https://image.example/result.png", response.Data[0].URL)
				return
			}
			require.NotNil(t, apiErr)
			require.Equal(t, "write_response_body_failed", apiErr.Code)
			require.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
			if mode == "failure" {
				require.ErrorIs(t, apiErr.RawError, fault)
			} else {
				require.ErrorIs(t, apiErr.RawError, io.ErrShortWrite)
			}
		})
	}
}
