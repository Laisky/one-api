package adaptor

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"
)

// TestTransportObservationHTTPFailureVisibleAtInfo sends synthetic HTTP responses
// through DoRequest and verifies INFO-floor evidence without changing the wire
// status, body, request or dispatch count. It accepts t and returns no value.
func TestTransportObservationHTTPFailureVisibleAtInfo(t *testing.T) {
	for _, status := range []int{200, 201, 204, 400, 401, 403, 404, 413, 429, 500, 502, 503, 599} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, err := io.ReadAll(r.Body)
				if err != nil || string(body) != transportPrivatePayload ||
					r.Method != http.MethodPost || r.Header.Get("X-Synthetic") != "unchanged" {
					t.Error("transport observation changed the synthetic request")
				}
				w.WriteHeader(status)
				if status != http.StatusNoContent {
					_, _ = io.WriteString(w, "synthetic response")
				}
			}))
			defer server.Close()

			c, _, _ := transportObservationFixture(t, &http.Transport{Proxy: nil})
			core, logs := observer.New(zapcore.InfoLevel)
			lg, err := glog.NewConsoleWithName("transport-info-fixture", glog.LevelInfo,
				zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
			require.NoError(t, err)
			gmw.SetLogger(c, lg)
			request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost,
				server.URL, strings.NewReader(transportPrivatePayload))
			require.NoError(t, err)
			request.Header.Set("X-Synthetic", "unchanged")

			response, err := DoRequest(c, request)
			require.NoError(t, err, "HTTP error statuses are not Client.Do transport errors")
			require.NotNil(t, response)
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.Equal(t, status, response.StatusCode)
			require.EqualValues(t, 1, calls.Load(), "observation must not add dispatches")
			if status == http.StatusNoContent {
				require.Empty(t, body)
			} else {
				require.Equal(t, "synthetic response", string(body))
			}

			if status < http.StatusBadRequest {
				require.Empty(t, logs.FilterMessage(transportObservationEvent).All(),
					"ordinary success must retain DEBUG volume")
				return
			}
			fields := transportObservationFields(t, logs, zapcore.InfoLevel)
			require.EqualValues(t, status, fields["upstream_http_status"])
			require.Equal(t, true, fields["response_headers_received"])
			require.Equal(t, "none", fields["transport_failure_class"])
			require.Equal(t, "none", fields["transport_failure_phase"])
			require.NotContains(t, fields, "normalized_status_code")
		})
	}
}
