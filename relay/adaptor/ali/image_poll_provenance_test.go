package ali

import (
	"io"
	"net/http"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/model"
)

// aliPollReadFailure is a poll body whose every Read fails, standing for a
// connection reset while the 2xx task status is being read.
type aliPollReadFailure struct{}

// Read always fails with io.ErrUnexpectedEOF.
//
// Parameters:
//   - p: the destination buffer, left untouched.
//
// Returns:
//   - int: always 0.
//   - error: always io.ErrUnexpectedEOF.
func (aliPollReadFailure) Read(p []byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// Close succeeds.
//
// Returns:
//   - error: always nil.
func (aliPollReadFailure) Close() error { return nil }

// installAliPollTransport replaces the default transport used by the DashScope
// task poll with serve for the duration of the test. Every poll must target the
// DashScope task endpoint of the created task.
//
// Parameters:
//   - t: the test; the original transport is restored on cleanup.
//   - serve: answers each poll.
func installAliPollTransport(t *testing.T, serve func(*http.Request) (*http.Response, error)) {
	t.Helper()
	oldTransport := http.DefaultTransport
	http.DefaultTransport = imageRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "dashscope.aliyuncs.com", r.URL.Host)
		require.Equal(t, "/api/v1/tasks/task-poll-1", r.URL.Path)
		return serve(r)
	})
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
}

// runAliPollImage runs ImageHandler for an accepted task (task-poll-1) whose
// poll is answered by the installed transport.
//
// Parameters:
//   - t: the test.
//
// Returns:
//   - *model.ErrorWithStatusCode: the handler's failure.
//   - bool: whether the handler marked the image receipt rejected (refund).
//   - string: what the handler wrote to the client.
func runAliPollImage(t *testing.T) (*model.ErrorWithStatusCode, bool, string) {
	t.Helper()
	c, rec := newAliAttributionContext()
	failure, usage := ImageHandler(c, aliAttributionResponse(http.StatusOK, "application/json",
		`{"output":{"task_id":"task-poll-1","task_status":"PENDING"},"request_id":"req-create"}`))
	require.Nil(t, usage)
	return failure, c.GetBool(adaptor.ImageReceiptRejectedKey), rec.Body.String()
}

// TestAliImagePollNon2xxIsProviderMarked requires a non-2xx DashScope task poll
// answer (429, 503, 401, and a provider "one_api_error" code) to become a
// provider-marked error with the poll's HTTP status and the parsed DashScope
// message, code and request id, never the raw body. Before the fix the poll
// status was ignored: a JSON refusal became an empty-message error with the
// creation's 200 status and an HTML one a local 500 one_api_error. The task was
// accepted, so the receipt is not marked rejected: the existing settlement of an
// accepted task (charged) is unchanged.
func TestAliImagePollNon2xxIsProviderMarked(t *testing.T) {
	cases := []struct {
		name, contentType, body, wantMessage, wantCode, wantParam string
		status                                                    int
	}{
		{name: "throttled_429", status: http.StatusTooManyRequests, contentType: "application/json",
			body:        `{"code":"Throttling.RateQuota","message":"Requests rate limit exceeded, please try again later.","request_id":"req-p1","account":"` + aliAttributionSecret + `"}`,
			wantMessage: "Requests rate limit exceeded, please try again later.", wantCode: "Throttling.RateQuota", wantParam: "req-p1"},
		{name: "invalid_key_401", status: http.StatusUnauthorized, contentType: "application/json",
			body:        `{"code":"InvalidApiKey","message":"Invalid API-key provided.","request_id":"req-p2"}`,
			wantMessage: "Invalid API-key provided.", wantCode: "InvalidApiKey", wantParam: "req-p2"},
		{name: "provider_one_api_code_403", status: http.StatusForbidden, contentType: "application/json",
			body:        `{"code":"one_api_error","message":"user quota is not enough"}`,
			wantMessage: "user quota is not enough", wantCode: "one_api_error"},
		{name: "html_503", status: http.StatusServiceUnavailable, contentType: "text/html",
			body: `<html>` + aliAttributionSecret + `</html>`, wantCode: aliImageErrorCode},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installAliPollTransport(t, func(r *http.Request) (*http.Response, error) {
				response := aliAttributionResponse(tc.status, tc.contentType, tc.body)
				response.Request = r
				return response, nil
			})

			failure, rejected, written := runAliPollImage(t)
			require.NotNil(t, failure)
			require.Equal(t, tc.status, failure.StatusCode, "the poll's provider status must be preserved")
			require.True(t, failure.FromUpstream)
			require.False(t, failure.IsOneAPIError(), "a provider poll refusal must not read as one-api's own error")
			require.Equal(t, model.ErrorTypeAli, failure.Type)
			wantMessage := tc.wantMessage
			if wantMessage == "" {
				wantMessage = model.UpstreamErrorBodyMessage(tc.status, tc.contentType, []byte(tc.body))
			}
			require.Equal(t, wantMessage, failure.Message)
			require.Equal(t, tc.wantCode, failure.Code)
			require.Equal(t, tc.wantParam, failure.Param)
			requireNoAliSecret(t, failure)
			require.False(t, rejected, "the task was accepted; its settlement must not change")
			require.Empty(t, written)
		})
	}
}

// TestAliImagePollLocalFailuresStayLocal requires a poll transport failure and
// an unreadable 2xx poll body to stay one-api's own 500
// ali_async_task_wait_failed errors: they prove nothing about the provider.
func TestAliImagePollLocalFailuresStayLocal(t *testing.T) {
	transportFailure := errors.New("synthetic poll transport failure")
	cases := map[string]func(*http.Request) (*http.Response, error){
		"transport": func(*http.Request) (*http.Response, error) { return nil, transportFailure },
		"read_2xx": func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: aliPollReadFailure{}, Request: r}, nil
		},
	}
	for name, serve := range cases {
		t.Run(name, func(t *testing.T) {
			installAliPollTransport(t, serve)

			failure, rejected, written := runAliPollImage(t)
			require.NotNil(t, failure)
			require.Equal(t, http.StatusInternalServerError, failure.StatusCode)
			require.Equal(t, "ali_async_task_wait_failed", failure.Code)
			require.False(t, failure.FromUpstream, "a local poll failure must not be provider-marked")
			require.True(t, failure.IsOneAPIError())
			if name == "read_2xx" {
				require.ErrorIs(t, failure.RawError, io.ErrUnexpectedEOF, "the read failure must not be swallowed")
			}
			require.False(t, rejected)
			require.Empty(t, written)
		})
	}
}
