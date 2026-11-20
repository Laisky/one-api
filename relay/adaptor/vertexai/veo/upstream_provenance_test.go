package veo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

const (
	// veoTestModelPath is the Vertex publisher model path the fake provider serves.
	veoTestModelPath = "/v1/projects/p/locations/us-central1/publishers/google/models/veo-3.1-generate-001"
	// veoTestOperation is the long-running operation name the fake provider returns.
	veoTestOperation = "projects/p/locations/us-central1/publishers/google/models/veo-3.1-generate-001/operations/op-1"
)

// veoProviderFailureStatuses lists the provider HTTP statuses that the
// adaptive ability cooldown must see as provider-caused failures.
var veoProviderFailureStatuses = []int{
	http.StatusTooManyRequests,
	http.StatusInternalServerError,
	http.StatusServiceUnavailable,
}

// veoFailingReadCloser is a response body whose Read always fails, standing in
// for a connection dropped while one-api reads a Veo response.
type veoFailingReadCloser struct{}

// Read always returns an error so the caller observes a local read failure.
// It accepts p and returns zero bytes with an error.
func (veoFailingReadCloser) Read(p []byte) (int, error) {
	return 0, errors.New("simulated veo body read failure")
}

// Close reports success because nothing is held open.
// It returns nil.
func (veoFailingReadCloser) Close() error {
	return nil
}

// newVeoTestContext builds a gin test context with a request attached so the
// poll loop derives a live request context as in the relay. It accepts t and
// returns the context.
func newVeoTestContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c
}

// fastAdaptor returns an Adaptor that polls every millisecond and gives up
// after five seconds, so tests exercise the real loop without real delays.
func fastAdaptor() *Adaptor {
	return &Adaptor{pollInterval: time.Millisecond, pollTimeout: 5 * time.Second}
}

// veoChatMeta returns the relay meta for a Veo chat-completions attempt.
func veoChatMeta() *meta.Meta {
	return &meta.Meta{Mode: relaymode.ChatCompletions, ActualModelName: "veo-3.1-generate-001"}
}

// requireVeoUpstreamProvenance asserts that got is a provider-attributed error
// whose client-visible JSON equals what the legacy ErrorWrapper produced for
// the same message, code, and status. It accepts t, got, the expected message,
// code, and status.
func requireVeoUpstreamProvenance(t *testing.T, got *model.ErrorWithStatusCode, message, code string, status int) {
	t.Helper()
	require.NotNil(t, got)
	require.Equal(t, status, got.StatusCode)
	require.True(t, got.FromUpstream, "provider status %d must be attributed to the provider", status)
	require.False(t, got.IsOneAPIError(), "provider status %d must not be treated as a one-api internal error", status)

	legacy := openai.ErrorWrapper(errors.New(message), code, status)
	gotJSON, err := json.Marshal(got)
	require.NoError(t, err)
	legacyJSON, err := json.Marshal(legacy)
	require.NoError(t, err)
	require.JSONEq(t, string(legacyJSON), string(gotJSON), "client-visible JSON must be unchanged")

	var wire map[string]any
	require.NoError(t, json.Unmarshal(gotJSON, &wire))
	require.Equal(t, message, wire["message"])
	require.Equal(t, string(model.ErrorTypeOneAPI), wire["type"])
	require.Equal(t, code, wire["code"])
	require.InDelta(t, float64(status), wire["status_code"], 0)
}

// requireVeoLocalProvenance asserts that got is a failure one-api produced
// itself. It accepts t, got, the expected code, and the expected status.
func requireVeoLocalProvenance(t *testing.T, got *model.ErrorWithStatusCode, code string, status int) {
	t.Helper()
	require.NotNil(t, got)
	require.Equal(t, status, got.StatusCode)
	require.Equal(t, code, got.Code)
	require.False(t, got.FromUpstream, "local failure %s must stay one-api owned", code)
	require.True(t, got.IsOneAPIError())
}

// TestDoResponse_CreateProviderStatusIsUpstream drives Adaptor.DoResponse with
// live 429/500/503 predictLongRunning responses and verifies the veo_api_error
// is attributed to the provider and carries only the parsed provider message.
func TestDoResponse_CreateProviderStatusIsUpstream(t *testing.T) {
	t.Parallel()
	for _, status := range veoProviderFailureStatuses {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			body := fmt.Sprintf(`{"error":{"code":%d,"message":"veo create failure"}}`, status)
			provider := newFakeVertex(t, status, body)
			resp := provider.createLikeRelay(t)

			usage, got := (&Adaptor{}).DoResponse(newVeoTestContext(t), resp, veoChatMeta())
			require.Nil(t, usage)
			requireVeoUpstreamProvenance(t, got, "veo create failure", "veo_api_error", status)
			require.Zero(t, provider.pollCount(), "a rejected create must not be polled")
		})
	}
}

// TestDoResponse_PollProviderStatusIsUpstream drives Adaptor.DoResponse through
// an accepted create into pollVideoTask, whose fetchPredictOperation calls the
// fake provider keeps answering with 429/500/503, and verifies polling gives up
// after maxConsecutivePollFailures transient failures with a provider-attributed
// poll_request_failed_status error that names the accepted operation.
func TestDoResponse_PollProviderStatusIsUpstream(t *testing.T) {
	t.Parallel()
	for _, status := range veoProviderFailureStatuses {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			pollBody := fmt.Sprintf(`{"error":{"code":%d,"message":"veo poll failure"}}`, status)
			provider := newFakeVertex(t, http.StatusOK, acceptedCreateBody, pollReply{status: status, body: pollBody})
			resp := provider.createLikeRelay(t)

			usage, got := fastAdaptor().DoResponse(newVeoTestContext(t), resp, veoChatMeta())
			require.NotNil(t, usage, "an accepted create still reports its billable duration")
			require.Equal(t, maxConsecutivePollFailures, provider.pollCount())
			requireVeoUpstreamProvenance(t, got, "veo operation op-1 was accepted but polling failed: veo poll failure",
				"poll_request_failed_status", status)
		})
	}
}

// TestVeoLocalFailuresStayOneAPIOwned is the control: failures one-api
// produces itself in the same functions (body read failure, undecodable create
// or poll body, poll transport failure with no provider response) must keep
// FromUpstream false.
func TestVeoLocalFailuresStayOneAPIOwned(t *testing.T) {
	t.Parallel()

	t.Run("create body read failure", func(t *testing.T) {
		t.Parallel()
		resp := &http.Response{StatusCode: http.StatusServiceUnavailable, Body: veoFailingReadCloser{}}
		_, got := (&Adaptor{}).DoResponse(newVeoTestContext(t), resp, veoChatMeta())
		requireVeoLocalProvenance(t, got, "read_response_body_failed", http.StatusInternalServerError)
	})

	t.Run("undecodable create body", func(t *testing.T) {
		t.Parallel()
		provider := newFakeVertex(t, http.StatusOK, "not-json")
		resp := provider.createLikeRelay(t)
		usage, got := (&Adaptor{}).DoResponse(newVeoTestContext(t), resp, veoChatMeta())
		require.NotNil(t, usage, "a 200 create may have started paid work")
		requireVeoLocalProvenance(t, got, "unmarshal_poll_response_failed", http.StatusInternalServerError)
	})

	t.Run("undecodable poll body", func(t *testing.T) {
		t.Parallel()
		provider := newFakeVertex(t, http.StatusOK, acceptedCreateBody, pollReply{status: http.StatusOK, body: "not-json"})
		resp := provider.createLikeRelay(t)
		_, got := fastAdaptor().DoResponse(newVeoTestContext(t), resp, veoChatMeta())
		requireVeoLocalProvenance(t, got, "unmarshal_poll_response_failed", http.StatusInternalServerError)
	})

	t.Run("poll transport failure without provider response", func(t *testing.T) {
		t.Parallel()
		provider := newFakeVertex(t, http.StatusOK, acceptedCreateBody)
		resp := provider.createLikeRelay(t)
		// Close the provider so every poll fails in transport with no status.
		provider.srv.Close()
		usage, got := fastAdaptor().DoResponse(newVeoTestContext(t), resp, veoChatMeta())
		require.NotNil(t, usage, "an accepted create still reports its billable duration")
		requireVeoLocalProvenance(t, got, "do_poll_request_failed", http.StatusServiceUnavailable)
		require.Contains(t, got.Message, "veo operation op-1 was accepted but polling failed")
	})
}
