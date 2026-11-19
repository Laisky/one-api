package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// openAIVideoUpstreamCase describes one provider answer to an OpenAI
// POST /v1/videos creation and the ledger outcome RelayVideoHelper must reach.
type openAIVideoUpstreamCase struct {
	name        string
	status      int
	contentType string
	body        string
	// wantErrJSON is the byte-exact json.Marshal of the returned error; empty
	// for an accepted job.
	wantErrJSON string
	// forbidden lists raw provider bytes the client-visible error must not echo.
	forbidden []string
	charge    int64
	taskID    string
}

// countVideoTaskBindings returns how many async task bindings the fixture user
// owns, so a test can prove a failed creation persisted no task.
//
// Parameters:
//   - t: the running test; a query failure fails it.
//
// Returns:
//   - int64: the number of bindings owned by the fixture user.
func countVideoTaskBindings(t *testing.T) int64 {
	t.Helper()
	var count int64
	require.NoError(t, model.DB.Model(&model.AsyncTaskBinding{}).Where("user_id = ?", fallbackUserID).Count(&count).Error)
	return count
}

// requireOpenAIVideoLedger asserts the physical user, token and request-cost
// ledgers after billing goroutines have drained.
//
// Parameters:
//   - t: the running test.
//   - balance: the user and token balance before the request.
//   - charge: the net quota the request must have consumed.
//   - requestID: the request whose recorded cost is checked.
//
// Returns: none.
func requireOpenAIVideoLedger(t *testing.T, balance, charge int64, requestID string) {
	t.Helper()
	require.Equal(t, balance-charge, reloadUserQuota(t), "user ledger")
	require.Equal(t, charge, requestCostQuota(t, requestID), "request cost ledger")
	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	require.Equal(t, balance-charge, token.RemainQuota, "token remaining quota")
	require.Equal(t, charge, token.UsedQuota, "token used quota")
}

// TestOpenAIVideoUpstreamNon2xxRefundsAndFails drives the real RelayVideoHelper
// over HTTP against a fake OpenAI provider. A non-2xx creation answer without a
// typed nested error (the issue-#625 flat envelope, an untyped nested error, a
// non-JSON page, an empty body) must fail the attempt with the provider status,
// be attributed to the provider, refund the whole hold, persist no task, write
// nothing to the client, and never echo raw non-JSON bytes. A typed nested error
// keeps its exact client JSON and refund; an accepted job is still charged and
// persisted unchanged.
//
// Parameters:
//   - t: the running test.
//
// Returns: none.
func TestOpenAIVideoUpstreamNon2xxRefundsAndFails(t *testing.T) {
	const htmlBody = `<html><head><title>500</title></head><body>Internal Server Error RAW-HTML-MARKER</body></html>`
	for _, tc := range []openAIVideoUpstreamCase{
		{
			name: "flat_625_envelope_429", status: http.StatusTooManyRequests, contentType: "application/json",
			body:        `{"message":"You have exceeded the monthly usage quota","type":"TooManyRequests","param":"quota","code":"AccountQuotaExceeded"}`,
			wantErrJSON: `{"message":"You have exceeded the monthly usage quota","type":"one_api_error","param":"","code":"AccountQuotaExceeded","status_code":429}`,
			forbidden:   []string{"TooManyRequests"},
		},
		{
			name: "untyped_nested_503", status: http.StatusServiceUnavailable, contentType: "application/json",
			body:        `{"error":{"message":"overloaded"}}`,
			wantErrJSON: `{"message":"overloaded","type":"one_api_error","param":"","code":"upstream_video_error","status_code":503}`,
		},
		{
			name: "non_json_500", status: http.StatusInternalServerError, contentType: "text/html",
			body:        htmlBody,
			wantErrJSON: `{"message":"OpenAI video upstream returned HTTP 500","type":"one_api_error","param":"","code":"upstream_video_error","status_code":500}`,
			forbidden:   []string{"<html", "RAW-HTML-MARKER", "Internal Server Error"},
		},
		{
			name: "empty_503", status: http.StatusServiceUnavailable, contentType: "application/json",
			wantErrJSON: `{"message":"OpenAI video upstream returned HTTP 503","type":"one_api_error","param":"","code":"upstream_video_error","status_code":503}`,
		},
		{
			name: "typed_nested_400_control", status: http.StatusBadRequest, contentType: "application/json",
			body:        `{"error":{"message":"m","type":"invalid_request_error","code":"c"}}`,
			wantErrJSON: `{"message":"m","type":"invalid_request_error","param":"","code":"c","status_code":400}`,
		},
		{
			name: "accepted_200_control", status: http.StatusOK, contentType: "application/json",
			body:   `{"id":"video_openai_accepted","object":"video","model":"sora-2","status":"queued","seconds":"4"}`,
			charge: 200000, taskID: "video_openai_accepted",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(10_000_000)
			xaiVideoSetup(t, balance, false)
			var creates atomic.Int32
			paths := make(chan string, 1)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				creates.Add(1)
				select {
				case paths <- r.Method + " " + r.URL.Path:
				default:
					// The atomic counter detects duplicate creations without blocking.
				}
				w.Header().Set("Content-Type", tc.contentType)
				w.Header().Set("X-Provider-Header", "provider-only")
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			previous := client.HTTPClient
			client.HTTPClient = server.Client()
			defer func() { client.HTTPClient = previous }()

			bindingsBefore := countVideoTaskBindings(t)
			c, w, requestID := protocolContext(t, channeltype.OpenAI, "sora-2", "/v1/videos",
				`{"model":"alias","prompt":"A paper boat drifting.","seconds":4}`, server.URL, balance, 1, false, nil)
			apiErr := RelayVideoHelper(c)
			drainCriticalTasks(t)

			require.EqualValues(t, 1, creates.Load(), "exactly one provider creation")
			require.Equal(t, http.MethodPost+" /v1/videos", <-paths)
			if tc.wantErrJSON == "" {
				require.Nil(t, apiErr)
				requireOpenAIVideoLedger(t, balance, tc.charge, requestID)
				require.Equal(t, tc.status, w.Code)
				require.Equal(t, tc.body, w.Body.String(), "accepted receipt forwarded byte-identically")
				require.Equal(t, "provider-only", w.Header().Get("X-Provider-Header"))
				binding, err := model.GetAsyncTaskBindingByTaskID(context.Background(), tc.taskID)
				require.NoError(t, err)
				require.Equal(t, fallbackUserID, binding.UserID)
				require.Equal(t, channeltype.OpenAI, binding.ChannelType)
				require.Equal(t, "sora-2", binding.ActualModel)
				return
			}

			require.NotNil(t, apiErr, "a failed provider creation must fail the attempt")
			require.Equal(t, tc.status, apiErr.StatusCode)
			require.True(t, apiErr.FromUpstream, "provider-status failure must be provider-attributed")
			require.False(t, apiErr.IsOneAPIError(), "provider-status failure must not be one-api's own")
			clientJSON, err := json.Marshal(apiErr)
			require.NoError(t, err)
			require.Equal(t, tc.wantErrJSON, string(clientJSON))
			for _, raw := range tc.forbidden {
				require.NotContains(t, apiErr.Message, raw)
				require.NotContains(t, string(clientJSON), raw)
			}
			require.False(t, c.Writer.Written(), "the provider body must not be forwarded to the client")
			require.Empty(t, w.Body.String())
			requireOpenAIVideoLedger(t, balance, tc.charge, requestID)
			require.Equal(t, bindingsBefore, countVideoTaskBindings(t), "a failed creation persists no task")
		})
	}
}
