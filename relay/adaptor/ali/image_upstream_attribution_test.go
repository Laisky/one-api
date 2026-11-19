package ali

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

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// aliAttributionSecret stands for private data a provider may put in an error
// body outside its documented message members; it must never reach the client.
const aliAttributionSecret = "internal-account-123"

// newAliAttributionContext builds an image request context carrying a task key.
//
// Returns:
//   - *gin.Context: the request context.
//   - *httptest.ResponseRecorder: the client response recorder.
func newAliAttributionContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Request.Header.Set("Authorization", "Bearer ali-fixture-key")
	return c, rec
}

// aliAttributionResponse builds a provider response with the given status,
// content type and body.
//
// Parameters:
//   - status: the provider HTTP status.
//   - contentType: the Content-Type header value.
//   - body: the response body.
//
// Returns:
//   - *http.Response: the response.
func aliAttributionResponse(status int, contentType, body string) *http.Response {
	header := http.Header{}
	header.Set("Content-Type", contentType)
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}
}

// requireNoAliSecret asserts the client-visible JSON of failure carries no
// private provider data.
//
// Parameters:
//   - t: the test.
//   - failure: the relay error.
func requireNoAliSecret(t *testing.T, failure *model.ErrorWithStatusCode) {
	t.Helper()
	encoded, err := json.Marshal(failure)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), aliAttributionSecret, "raw provider body must not be echoed")
}

// TestAliImageTaskRefusalKeepsProviderStatusAndProvenance requires a non-2xx
// DashScope task-creation answer (for example 429 Throttling.RateQuota) to keep
// the provider's HTTP status and be marked FromUpstream, instead of becoming a
// 500 ali_async_task_failed one_api_error that skipped ability cooldown and
// channel health. Only the parsed message, code and request id may reach the
// client; a non-JSON, oversized or message-less body gets the shared generic
// description (an empty wantMessage below).
func TestAliImageTaskRefusalKeepsProviderStatusAndProvenance(t *testing.T) {
	cases := []struct {
		name, contentType, body, wantMessage, wantCode, wantParam string
		status                                                    int
	}{
		{name: "throttled_429", status: http.StatusTooManyRequests, contentType: "application/json",
			body:        `{"code":"Throttling.RateQuota","message":"Requests rate limit exceeded, please try again later.","request_id":"req-1","account":"` + aliAttributionSecret + `"}`,
			wantMessage: "Requests rate limit exceeded, please try again later.", wantCode: "Throttling.RateQuota", wantParam: "req-1"},
		{name: "arrearage_400", status: http.StatusBadRequest, contentType: "application/json",
			body:        `{"code":"Arrearage","message":"Access denied, please make sure your account is in good standing.","request_id":"req-2"}`,
			wantMessage: "Access denied, please make sure your account is in good standing.", wantCode: "Arrearage", wantParam: "req-2"},
		{name: "provider_one_api_code_403", status: http.StatusForbidden, contentType: "application/json",
			body:        `{"code":"one_api_error","message":"user quota is not enough"}`,
			wantMessage: "user quota is not enough", wantCode: "one_api_error"},
		{name: "html_503", status: http.StatusServiceUnavailable, contentType: "text/html",
			body:     `<html>` + aliAttributionSecret + `</html>`,
			wantCode: aliImageErrorCode},
		{name: "message_object_502", status: http.StatusBadGateway, contentType: "application/json",
			body:     `{"code":"InternalError","message":{"detail":"` + aliAttributionSecret + `"}}`,
			wantCode: aliImageErrorCode},
		{name: "oversized_429", status: http.StatusTooManyRequests, contentType: "application/json",
			body:     `{"code":"Throttling","message":"` + aliAttributionSecret + strings.Repeat("a", model.MaxUpstreamErrorBodyBytes) + `"}`,
			wantCode: aliImageErrorCode},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := newAliAttributionContext()
			usage, failure := (&Adaptor{}).DoResponse(c, aliAttributionResponse(tc.status, tc.contentType, tc.body),
				&meta.Meta{Mode: relaymode.ImagesGenerations})
			require.Nil(t, usage)
			require.NotNil(t, failure)
			require.Equal(t, tc.status, failure.StatusCode, "the provider status must be preserved")
			require.True(t, failure.FromUpstream)
			require.False(t, failure.IsOneAPIError(), "a provider refusal must not read as one-api's own error")
			require.Equal(t, model.ErrorTypeAli, failure.Type)
			wantMessage := tc.wantMessage
			if wantMessage == "" {
				// No recognized message: the shared generic description applies.
				wantMessage = model.UpstreamErrorBodyMessage(tc.status, tc.contentType, []byte(tc.body))
				require.True(t, strings.HasPrefix(wantMessage, fmt.Sprintf("upstream returned HTTP %d with ", tc.status)))
			}
			require.Equal(t, wantMessage, failure.Message)
			require.Equal(t, tc.wantCode, failure.Code)
			require.Equal(t, tc.wantParam, failure.Param)
			requireNoAliSecret(t, failure)
			require.Empty(t, rec.Body.String(), "the adaptor must not write a response for a refusal")
		})
	}
}

// TestAliImageTaskErrorWithoutTaskIsRejectedReceipt requires a 2xx creation
// answer that carries an error message and no task id to be a provider-marked
// failure that marks the image receipt rejected: no task exists, so no image
// can be billed and the image helper must refund the reservation.
func TestAliImageTaskErrorWithoutTaskIsRejectedReceipt(t *testing.T) {
	c, _ := newAliAttributionContext()
	failure, usage := ImageHandler(c, aliAttributionResponse(http.StatusOK, "application/json",
		`{"code":"InvalidParameter","message":"size is invalid","request_id":"req-3"}`))
	require.Nil(t, usage)
	require.NotNil(t, failure)
	require.Equal(t, http.StatusInternalServerError, failure.StatusCode)
	require.Equal(t, "ali_async_task_failed", failure.Code)
	require.True(t, failure.FromUpstream)
	require.False(t, failure.IsOneAPIError())
	require.True(t, c.GetBool(adaptor.ImageReceiptRejectedKey), "no task was created, so the reservation must be refunded")
}

// TestAliImageFailedTaskIsProviderMarked requires a DashScope task that ends
// FAILED to be reported as a provider-marked error with the provider's task
// message and code. Its settlement is unchanged: the receipt is not marked
// rejected, because DashScope billing for failed tasks is not yet verified.
func TestAliImageFailedTaskIsProviderMarked(t *testing.T) {
	oldTransport := http.DefaultTransport
	http.DefaultTransport = imageRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.Equal(t, "dashscope.aliyuncs.com", r.URL.Host)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Request: r,
			Body: io.NopCloser(strings.NewReader(`{"output":{"task_id":"t1","task_status":"FAILED","code":"DataInspectionFailed","message":"Output data may contain inappropriate content."}}`))}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = oldTransport })

	c, rec := newAliAttributionContext()
	failure, usage := ImageHandler(c, aliAttributionResponse(http.StatusOK, "application/json", `{"output":{"task_id":"t1","task_status":"PENDING"}}`))
	require.Nil(t, usage)
	require.NotNil(t, failure)
	require.Equal(t, http.StatusOK, failure.StatusCode, "the accepted task keeps its existing status contract")
	require.Equal(t, model.ErrorTypeAli, failure.Type)
	require.Equal(t, "DataInspectionFailed", failure.Code)
	require.Equal(t, "Output data may contain inappropriate content.", failure.Message)
	require.True(t, failure.FromUpstream)
	require.False(t, c.GetBool(adaptor.ImageReceiptRejectedKey))
	require.Empty(t, rec.Body.String())
}

// TestAliTextErrorsAreProviderMarked requires the DashScope error envelope on
// the chat and embedding handlers to be built by model.NewUpstreamError: the
// provider's status, message, code and request id are kept, and a provider code
// of "one_api_error" never reads as one-api's own error.
func TestAliTextErrorsAreProviderMarked(t *testing.T) {
	handlers := map[string]func(*gin.Context, *http.Response) (*model.ErrorWithStatusCode, *model.Usage){
		"chat":      Handler,
		"embedding": EmbeddingHandler,
	}
	for name, handler := range handlers {
		t.Run(name, func(t *testing.T) {
			c, _ := newAliAttributionContext()
			failure, _ := handler(c, aliAttributionResponse(http.StatusForbidden, "application/json",
				`{"code":"one_api_error","message":"user quota is not enough","request_id":"req-4"}`))
			require.NotNil(t, failure)
			require.Equal(t, http.StatusForbidden, failure.StatusCode)
			require.Equal(t, model.ErrorType("one_api_error"), failure.Type)
			require.Equal(t, "one_api_error", failure.Code)
			require.Equal(t, "req-4", failure.Param)
			require.Equal(t, "user quota is not enough", failure.Message)
			require.True(t, failure.FromUpstream)
			require.False(t, failure.IsOneAPIError())
			require.EqualError(t, failure.RawError, "user quota is not enough")
		})
	}
}
