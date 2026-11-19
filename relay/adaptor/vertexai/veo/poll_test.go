package veo

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// newRecordingContext builds a gin test context whose writer is a recorder,
// with a request attached. It accepts t and returns the context and recorder.
func newRecordingContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c, recorder
}

// TestDoResponse_PollsFetchPredictOperationWithCreateCredentials drives a
// production-shaped create response (RequestURI empty) through the poll loop
// and verifies every poll targets the same publisher model's
// fetchPredictOperation with the create call's query, bearer token and channel
// headers, names the operation, and that both Vertex video shapes reach the
// client with the accepted duration billed.
func TestDoResponse_PollsFetchPredictOperationWithCreateCredentials(t *testing.T) {
	t.Parallel()
	provider := newFakeVertex(t, http.StatusOK, acceptedCreateBody, pendingReply(), pendingReply(), doneReply())
	resp := provider.createLikeRelay(t)
	c, recorder := newRecordingContext(t)

	usage, got := fastAdaptor().DoResponse(c, resp, veoChatMeta())
	require.Nil(t, got)
	require.NotNil(t, usage)
	require.Equal(t, defaultVideoDurationSec*ratio.TokensPerSec, usage.CompletionTokens)

	polls := provider.pollRecords()
	require.Len(t, polls, 3)
	for _, poll := range polls {
		require.Equal(t, veoTestModelPath+actionFetchOperation, poll.path)
		require.Equal(t, fakeCreateQuery, poll.rawQuery)
		require.Equal(t, "Bearer "+fakeVertexToken, poll.authorization)
		require.Equal(t, "custom-value", poll.custom)
		require.Equal(t, veoTestOperation, poll.operationName)
	}

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Len(t, payload.Data, 2)
	require.Equal(t, "gs://bucket/sample_0.mp4", payload.Data[0].URL)
	require.Equal(t, "AAAA", payload.Data[1].B64JSON)
}

// TestDoResponse_LegacyGeneratedSamplesStillConverted verifies the Gemini
// Developer API generatedSamples shape keeps mapping to data[].url.
func TestDoResponse_LegacyGeneratedSamplesStillConverted(t *testing.T) {
	t.Parallel()
	legacy := pollReply{status: http.StatusOK, body: `{"name":"` + veoTestOperation +
		`","done":true,"response":{"generatedSamples":[{"video":{"uri":"https://example.test/v.mp4"}}]}}`}
	provider := newFakeVertex(t, http.StatusOK, acceptedCreateBody, legacy)
	c, recorder := newRecordingContext(t)

	_, got := fastAdaptor().DoResponse(c, provider.createLikeRelay(t), veoChatMeta())
	require.Nil(t, got)
	require.Contains(t, recorder.Body.String(), `"url":"https://example.test/v.mp4"`)
}

// TestDoResponse_PollUsesSharedRelayClient verifies polls go through the
// shared relay client (proxy and timeout settings) instead of
// http.DefaultClient. It swaps the package-global client, so it is not parallel.
func TestDoResponse_PollUsesSharedRelayClient(t *testing.T) {
	var viaShared atomic.Int32
	original := client.HTTPClient
	client.HTTPClient = &http.Client{Transport: recordingTransport(func(r *http.Request) (*http.Response, error) {
		viaShared.Add(1)
		return http.DefaultTransport.RoundTrip(r)
	})}
	t.Cleanup(func() { client.HTTPClient = original })

	provider := newFakeVertex(t, http.StatusOK, acceptedCreateBody, doneReply())
	c, _ := newRecordingContext(t)
	_, got := fastAdaptor().DoResponse(c, provider.createLikeRelay(t), veoChatMeta())
	require.Nil(t, got)
	require.Equal(t, int32(1), viaShared.Load(), "the poll must use client.HTTPClient")
}

// recordingTransport adapts a function to http.RoundTripper.
type recordingTransport func(*http.Request) (*http.Response, error)

// RoundTrip calls the wrapped function with r and returns its result.
func (f recordingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// TestDoResponse_PollStopsWhenRequestEnds verifies polling stops promptly with
// a 408 once the client request context ends, while the accepted duration is
// still billed.
func TestDoResponse_PollStopsWhenRequestEnds(t *testing.T) {
	t.Parallel()
	provider := newFakeVertex(t, http.StatusOK, acceptedCreateBody, pendingReply())
	resp := provider.createLikeRelay(t)
	c, _ := newRecordingContext(t)
	ctx, cancel := context.WithCancel(context.Background())
	c.Request = c.Request.WithContext(ctx)
	time.AfterFunc(50*time.Millisecond, cancel)

	started := time.Now()
	usage, got := (&Adaptor{pollInterval: 10 * time.Millisecond, pollTimeout: time.Minute}).DoResponse(c, resp, veoChatMeta())
	require.Less(t, time.Since(started), 10*time.Second)
	require.NotNil(t, usage)
	requireVeoLocalProvenance(t, got, "request_context_done_while_waiting_for_poll", http.StatusRequestTimeout)
}

// TestDoResponse_PollIsBounded verifies an operation that never finishes ends
// with a local 504 naming only the opaque operation ID, never the project path.
func TestDoResponse_PollIsBounded(t *testing.T) {
	t.Parallel()
	provider := newFakeVertex(t, http.StatusOK, acceptedCreateBody, pendingReply())
	c, _ := newRecordingContext(t)

	usage, got := (&Adaptor{pollInterval: time.Millisecond, pollTimeout: 100 * time.Millisecond}).
		DoResponse(c, provider.createLikeRelay(t), veoChatMeta())
	require.NotNil(t, usage)
	requireVeoLocalProvenance(t, got, "veo_poll_timeout", http.StatusGatewayTimeout)
	require.Contains(t, got.Message, "veo operation op-1 was accepted")
	require.NotContains(t, got.Message, "projects/")
	require.Greater(t, provider.pollCount(), 1)
}

// TestDoResponse_PollRetriesTransientFailure verifies one transient provider
// failure does not abandon an accepted operation.
func TestDoResponse_PollRetriesTransientFailure(t *testing.T) {
	t.Parallel()
	provider := newFakeVertex(t, http.StatusOK, acceptedCreateBody,
		pollReply{status: http.StatusServiceUnavailable, body: `{"error":{"code":503,"message":"try later"}}`}, doneReply())
	c, recorder := newRecordingContext(t)

	_, got := fastAdaptor().DoResponse(c, provider.createLikeRelay(t), veoChatMeta())
	require.Nil(t, got)
	require.Equal(t, 2, provider.pollCount())
	require.Equal(t, http.StatusOK, recorder.Code)
}

// TestDoResponse_PollNonTransientFailureIsImmediate verifies a non-transient
// provider poll status ends polling at once with provider attribution.
func TestDoResponse_PollNonTransientFailureIsImmediate(t *testing.T) {
	t.Parallel()
	provider := newFakeVertex(t, http.StatusOK, acceptedCreateBody,
		pollReply{status: http.StatusForbidden, body: `{"error":{"code":403,"message":"Permission denied","status":"PERMISSION_DENIED"}}`})
	c, _ := newRecordingContext(t)

	usage, got := fastAdaptor().DoResponse(c, provider.createLikeRelay(t), veoChatMeta())
	require.NotNil(t, usage)
	require.Equal(t, 1, provider.pollCount())
	requireVeoUpstreamProvenance(t, got, "veo operation op-1 was accepted but polling failed: Permission denied",
		"poll_request_failed_status", http.StatusForbidden)
}

// TestDoResponse_OperationErrorIsReported verifies an operation the provider
// finished with an error is reported (status mapped from the google.rpc code)
// instead of an empty success, and that the accepted job stays billed.
func TestDoResponse_OperationErrorIsReported(t *testing.T) {
	t.Parallel()
	failed := pollReply{status: http.StatusOK, body: `{"name":"` + veoTestOperation +
		`","done":true,"error":{"code":3,"message":"The prompt was filtered."}}`}
	provider := newFakeVertex(t, http.StatusOK, acceptedCreateBody, failed)
	c, recorder := newRecordingContext(t)

	usage, got := fastAdaptor().DoResponse(c, provider.createLikeRelay(t), veoChatMeta())
	require.NotNil(t, usage)
	requireVeoUpstreamProvenance(t, got, "veo operation op-1 failed: The prompt was filtered.",
		"veo_operation_failed", http.StatusBadRequest)
	require.Empty(t, recorder.Body.String(), "a failed operation must not be written as a success")
}

// TestDoResponse_PollErrorBodyReadFailureIsNotSwallowed verifies a poll error
// body that breaks mid-read keeps the provider status with a generic message
// and never leaks the partial body.
func TestDoResponse_PollErrorBodyReadFailureIsNotSwallowed(t *testing.T) {
	t.Parallel()
	provider := newFakeVertex(t, http.StatusOK, acceptedCreateBody,
		pollReply{status: http.StatusBadRequest, body: `{"error":{"message":"SECRET-PARTIAL`, hijack: true})
	c, _ := newRecordingContext(t)

	_, got := fastAdaptor().DoResponse(c, provider.createLikeRelay(t), veoChatMeta())
	requireVeoUpstreamProvenance(t, got, "veo operation op-1 was accepted but polling failed: upstream returned HTTP 400",
		"poll_request_failed_status", http.StatusBadRequest)
}

// TestDoResponse_AcceptedCreateReadFailureStaysBilled verifies a 200 create
// whose body cannot be read still reports the billable duration, so the relay
// neither refunds nor resubmits possibly accepted paid work.
func TestDoResponse_AcceptedCreateReadFailureStaysBilled(t *testing.T) {
	t.Parallel()
	resp := &http.Response{StatusCode: http.StatusOK, Body: veoFailingReadCloser{}}
	usage, got := (&Adaptor{}).DoResponse(newVeoTestContext(t), resp, veoChatMeta())
	require.NotNil(t, usage)
	requireVeoLocalProvenance(t, got, "read_response_body_failed", http.StatusInternalServerError)
}

// TestDoResponse_MissingCreateRequestIsLocalFailure verifies an accepted
// create without its request cannot be polled and fails locally, still billed.
func TestDoResponse_MissingCreateRequestIsLocalFailure(t *testing.T) {
	t.Parallel()
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(acceptedCreateBody))}
	usage, got := (&Adaptor{}).DoResponse(newVeoTestContext(t), resp, veoChatMeta())
	require.NotNil(t, usage)
	requireVeoLocalProvenance(t, got, "veo_poll_endpoint_unresolved", http.StatusInternalServerError)
}

// TestDoResponse_EmptyOperationNameIsLocalFailure verifies a 200 create that
// names no operation fails locally without polling, still billed.
func TestDoResponse_EmptyOperationNameIsLocalFailure(t *testing.T) {
	t.Parallel()
	provider := newFakeVertex(t, http.StatusOK, `{"name":"  "}`)
	usage, got := (&Adaptor{}).DoResponse(newVeoTestContext(t), provider.createLikeRelay(t), veoChatMeta())
	require.NotNil(t, usage)
	requireVeoLocalProvenance(t, got, "veo_operation_name_missing", http.StatusBadGateway)
	require.Zero(t, provider.pollCount())
}

// TestFetchOperationURL verifies the poll endpoint derivation from the create
// request, including rejection of non-predictLongRunning URLs.
func TestFetchOperationURL(t *testing.T) {
	t.Parallel()
	req, err := http.NewRequest(http.MethodPost,
		"https://us-central1-aiplatform.googleapis.com/v1/projects/p/locations/us-central1/publishers/google/models/veo-3.1-generate-001:predictLongRunning?alt=json#frag", nil)
	require.NoError(t, err)
	got, err := fetchOperationURL(req)
	require.NoError(t, err)
	require.Equal(t, "https://us-central1-aiplatform.googleapis.com/v1/projects/p/locations/us-central1/publishers/google/models/veo-3.1-generate-001:fetchPredictOperation?alt=json", got)

	other, err := http.NewRequest(http.MethodPost, "https://example.test/v1/models/veo:generate", nil)
	require.NoError(t, err)
	_, err = fetchOperationURL(other)
	require.Error(t, err)

	_, err = fetchOperationURL(nil)
	require.Error(t, err)
}

// TestOperationRef verifies only the opaque trailing operation ID is exposed.
func TestOperationRef(t *testing.T) {
	t.Parallel()
	require.Equal(t, "op-1", operationRef(veoTestOperation))
	require.Empty(t, operationRef("op-without-path"))
	require.Empty(t, operationRef("projects/p/operations/bad id/with space"))
}

// TestRPCCodeHTTPStatus verifies representative google.rpc code mappings.
func TestRPCCodeHTTPStatus(t *testing.T) {
	t.Parallel()
	require.Equal(t, http.StatusBadRequest, rpcCodeHTTPStatus(3))
	require.Equal(t, http.StatusTooManyRequests, rpcCodeHTTPStatus(8))
	require.Equal(t, http.StatusServiceUnavailable, rpcCodeHTTPStatus(14))
	require.Equal(t, http.StatusUnauthorized, rpcCodeHTTPStatus(16))
	require.Equal(t, http.StatusBadGateway, rpcCodeHTTPStatus(0))
	require.Equal(t, http.StatusBadGateway, rpcCodeHTTPStatus(99))
}
