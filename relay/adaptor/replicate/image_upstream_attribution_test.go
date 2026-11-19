package replicate

import (
	"encoding/json"
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

// replicateAttributionSecret stands for private data a provider may put in an
// error body outside its documented message members; it must never reach the
// client.
const replicateAttributionSecret = "internal-account-123"

// newReplicateAttributionContext builds a request context with the Replicate
// meta the handlers read.
//
// Parameters:
//   - t: the test.
//
// Returns:
//   - *gin.Context: the request context.
//   - *httptest.ResponseRecorder: the client response recorder.
func newReplicateAttributionContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	meta.Set2Context(c, &meta.Meta{ActualModelName: "black-forest-labs/flux-1.1-pro", APIKey: "replicate-fixture-key"})
	return c, rec
}

// replicateAttributionResponse builds a provider response with the given status,
// content type and body.
//
// Parameters:
//   - status: the provider HTTP status.
//   - contentType: the Content-Type header value.
//   - body: the response body.
//
// Returns:
//   - *http.Response: the response.
func replicateAttributionResponse(status int, contentType, body string) *http.Response {
	header := http.Header{}
	header.Set("Content-Type", contentType)
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}
}

// requireNoReplicateSecret asserts the client-visible JSON of failure carries no
// private provider data.
//
// Parameters:
//   - t: the test.
//   - failure: the relay error.
func requireNoReplicateSecret(t *testing.T, failure *model.ErrorWithStatusCode) {
	t.Helper()
	encoded, err := json.Marshal(failure)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), replicateAttributionSecret, "raw provider body must not be echoed")
}

// TestReplicateCreationRefusalKeepsProviderStatusAndProvenance requires a
// non-2xx prediction-creation answer (for example 429 rate limiting) to keep
// the provider's HTTP status, be marked FromUpstream (so the relay cools down
// and fails over instead of treating it as a one-api 500), and expose only the
// parsed problem-details detail/title, never the raw body, on both the image and
// chat handlers. A body without a recognized message gets the shared generic
// description (an empty wantMessage below).
func TestReplicateCreationRefusalKeepsProviderStatusAndProvenance(t *testing.T) {
	cases := []struct {
		name, contentType, body, wantMessage string
		status                               int
	}{
		{name: "problem_detail_429", status: http.StatusTooManyRequests, contentType: "application/problem+json",
			body:        `{"title":"Too many requests","detail":"Request was throttled.","status":429,"private":"` + replicateAttributionSecret + `"}`,
			wantMessage: "upstream returned HTTP 429: Request was throttled."},
		{name: "problem_title_only_402", status: http.StatusPaymentRequired, contentType: "application/problem+json",
			body:        `{"title":"Insufficient credit","status":402,"account":"` + replicateAttributionSecret + `"}`,
			wantMessage: "upstream returned HTTP 402: Insufficient credit"},
		{name: "html_503", status: http.StatusServiceUnavailable, contentType: "text/html",
			body: `<html>` + replicateAttributionSecret + `</html>`,
		},
		{name: "detail_object_502", status: http.StatusBadGateway, contentType: "application/json",
			body: `{"detail":{"inner":"` + replicateAttributionSecret + `"}}`,
		},
		{name: "oversized_429", status: http.StatusTooManyRequests, contentType: "application/json",
			body: `{"detail":"` + replicateAttributionSecret + strings.Repeat("a", model.MaxUpstreamErrorBodyBytes) + `"}`,
		},
	}
	handlers := map[string]func(*gin.Context, *http.Response) (*model.ErrorWithStatusCode, *model.Usage){
		"image": ImageHandler,
		"chat":  ChatHandler,
	}
	for handlerName, handler := range handlers {
		for _, tc := range cases {
			t.Run(handlerName+"/"+tc.name, func(t *testing.T) {
				c, rec := newReplicateAttributionContext(t)
				failure, usage := handler(c, replicateAttributionResponse(tc.status, tc.contentType, tc.body))
				require.Nil(t, usage)
				require.NotNil(t, failure)
				require.Equal(t, tc.status, failure.StatusCode, "the provider status must be preserved")
				require.True(t, failure.FromUpstream)
				require.False(t, failure.IsOneAPIError())
				require.Equal(t, "bad_status_code", failure.Code)
				wantMessage := tc.wantMessage
				if wantMessage == "" {
					// No recognized message: the shared generic description applies.
					wantMessage = model.UpstreamErrorBodyMessage(tc.status, tc.contentType, []byte(tc.body))
					require.True(t, strings.HasPrefix(wantMessage, fmt.Sprintf("upstream returned HTTP %d with ", tc.status)))
				}
				require.Equal(t, wantMessage, failure.Message)
				requireNoReplicateSecret(t, failure)
				require.Empty(t, rec.Body.String(), "nothing is written to the client by the adaptor")
			})
		}
	}
}

// TestReplicateUnexpectedCreationSuccessStaysLocal requires an unexpected 2xx
// creation status (the provider accepted the request) to keep the earlier local
// 500 classification while still never echoing the raw body.
func TestReplicateUnexpectedCreationSuccessStaysLocal(t *testing.T) {
	c, _ := newReplicateAttributionContext(t)
	body := `{"id":"p1","output":"` + replicateAttributionSecret + `"}`
	failure, _ := ImageHandler(c, replicateAttributionResponse(http.StatusOK, "application/json", body))
	require.NotNil(t, failure)
	require.Equal(t, http.StatusInternalServerError, failure.StatusCode)
	require.False(t, failure.FromUpstream)
	require.Equal(t, model.UpstreamErrorBodyMessage(http.StatusOK, "application/json", []byte(body)), failure.Message)
	requireNoReplicateSecret(t, failure)
}

// TestReplicateFollowUpRefusalIsProviderMarkedWithoutRawBody requires a failed
// prediction poll (the provider answered the task URL with a non-200 status) to
// be provider-marked and to carry only the parsed detail, never the raw body,
// while a failed prediction status keeps its local classification.
func TestReplicateFollowUpRefusalIsProviderMarkedWithoutRawBody(t *testing.T) {
	cases := []struct {
		name, pollBody, wantMessage string
		pollStatus                  int
		wantUpstream                bool
	}{
		{name: "poll_429_detail", pollStatus: http.StatusTooManyRequests,
			pollBody:     `{"detail":"Request was throttled.","trace":"` + replicateAttributionSecret + `"}`,
			wantMessage:  "get task: upstream returned HTTP 429: Request was throttled.",
			wantUpstream: true},
		{name: "poll_502_html", pollStatus: http.StatusBadGateway,
			pollBody:     `<html>` + replicateAttributionSecret + `</html>`,
			wantMessage:  "get task: " + model.UpstreamErrorBodyMessage(http.StatusBadGateway, "application/json", []byte(`<html>`+replicateAttributionSecret+`</html>`)),
			wantUpstream: true},
		{name: "prediction_failed", pollStatus: http.StatusOK,
			pollBody:     `{"id":"p1","status":"failed","error":"NSFW content detected"}`,
			wantMessage:  "task failed, [failed]NSFW content detected",
			wantUpstream: false},
	}
	handlers := map[string]struct {
		handler func(*gin.Context, *http.Response) (*model.ErrorWithStatusCode, *model.Usage)
		code    string
	}{
		"image": {handler: ImageHandler, code: "image_task_failed"},
		"chat":  {handler: ChatHandler, code: "chat_task_failed"},
	}
	for handlerName, h := range handlers {
		for _, tc := range cases {
			t.Run(handlerName+"/"+tc.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tc.pollStatus)
					_, _ = io.WriteString(w, tc.pollBody)
				}))
				defer server.Close()

				c, _ := newReplicateAttributionContext(t)
				created := `{"id":"p1","status":"starting","urls":{"get":"` + server.URL + `/v1/predictions/p1"}}`
				failure, _ := h.handler(c, replicateAttributionResponse(http.StatusCreated, "application/json", created))
				require.NotNil(t, failure)
				require.Equal(t, http.StatusInternalServerError, failure.StatusCode, "the accepted prediction keeps the 500 contract")
				require.Equal(t, h.code, failure.Code)
				require.Equal(t, tc.wantMessage, failure.Message)
				require.Equal(t, tc.wantUpstream, failure.FromUpstream)
				require.Equal(t, !tc.wantUpstream, failure.IsOneAPIError())
				requireNoReplicateSecret(t, failure)
			})
		}
	}
}
