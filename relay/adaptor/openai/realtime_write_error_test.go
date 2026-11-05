package openai

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// realtimeFailedWriter injects a client response write failure.
type realtimeFailedWriter struct {
	gin.ResponseWriter
	err error
}

// Write returns the configured client write failure.
func (w realtimeFailedWriter) Write([]byte) (int, error) { return 0, w.err }

// TestRealtimeSessionsResponseWriteFailure verifies delivery errors preserve
// the upstream business error when the upstream rejects the request.
func TestRealtimeSessionsResponseWriteFailure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
	}{{"success", http.StatusOK}, {"upstream rejected", http.StatusBadRequest}} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, `{"message":"synthetic upstream"}`)
			}))
			defer upstream.Close()
			failure := errors.New("synthetic client write failure")
			c, recorder := gin.CreateTestContext(httptest.NewRecorder())
			_ = recorder
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/realtime/sessions", strings.NewReader(`{}`))
			c.Writer = realtimeFailedWriter{ResponseWriter: c.Writer, err: failure}
			biz, err := RealtimeSessionsHandler(c, &meta.Meta{BaseURL: upstream.URL})
			require.ErrorIs(t, err, failure)
			require.Contains(t, fmt.Sprintf("%+v", err), "RealtimeSessionsHandler")
			require.NotNil(t, biz)
			if tc.status == http.StatusBadRequest {
				require.Equal(t, model.ErrorTypeUpstream, biz.Type)
				require.Equal(t, tc.status, biz.StatusCode)
				require.Equal(t, tc.status, biz.Code)
				require.Equal(t, `{"message":"synthetic upstream"}`, biz.Message)
			} else {
				require.Equal(t, model.ErrorTypeOneAPI, biz.Type)
				require.Equal(t, http.StatusInternalServerError, biz.StatusCode)
				require.Equal(t, "write_response_body_failed", biz.Code)
				require.ErrorIs(t, biz.RawError, failure)
			}
		})
	}
}
