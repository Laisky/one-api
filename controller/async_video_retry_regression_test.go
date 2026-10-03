package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestAsyncVideoSubmissionRetryBoundary exercises the real retry decision, not
// source text: a failed acknowledgement must never create another paid job.
func TestAsyncVideoSubmissionRetryBoundary(t *testing.T) {
	for _, path := range []string{"/v1/videos", "/v1/videos/generations", "/v1/async/videos"} {
		t.Run(path, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, path, nil)
			failure := openai.ErrorWrapper(errors.New("upstream acknowledgement lost"), "do_request_failed", http.StatusBadGateway)
			require.NoError(t, shouldRetry(c, failure), "unforwarded request should remain retryable")
			c.Set(ctxkey.UpstreamRequestPossiblyForwarded, true)
			require.Error(t, shouldRetry(c, failure), "possibly accepted paid submission must not be replayed")
		})
	}
}

// TestAsyncVideoTransientClientErrorCannotOverrideReplayVeto drives the complete
// Relay loop, not only shouldRetry. The transient 400 exception must not replay
// a paid video request that could already have reached its provider.
func TestAsyncVideoTransientClientErrorCannotOverrideReplayVeto(t *testing.T) {
	for _, path := range []string{"/v1/videos", "/v1/videos/generations", "/v1/async/videos"} {
		t.Run(path, func(t *testing.T) {
			first := retryOrderChannel(1, "first", 100)
			second := retryOrderChannel(2, "second", 90)
			setupRetryOrderDB(t, []*dbmodel.Channel{first, second})
			result := runRelayRetryScenarioForRequest(t, first, retryOrderModel, http.MethodPost, path, func(id int) *relaymodel.ErrorWithStatusCode {
				return &relaymodel.ErrorWithStatusCode{StatusCode: http.StatusBadRequest, Error: relaymodel.Error{Code: "output_parse_failed", Message: "generated output that could not be parsed"}}
			}, false, 3)
			require.Equal(t, []int{1}, result.order, "normally transient 4xx must not override an irreversible video boundary")
		})
	}
}
