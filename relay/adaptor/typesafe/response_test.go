package typesafe

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
)

// trackedBody records closure of buffered responses.
type trackedBody struct {
	io.Reader
	closed bool
}

// Close records response ownership cleanup.
func (b *trackedBody) Close() error { b.closed = true; return nil }

// failingReader simulates an interrupted provider response.
type failingReader struct{}

// Read returns a deterministic partial-body failure.
func (failingReader) Read(_ []byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// responseContext creates a native request context with stable question identities.
func responseContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/systemone", nil)
	gmw.SetLogger(c, logger.Logger)
	request, err := DecodeRequest([]byte(`{"model":"jev-latest","state":"x","questions":{"q":{"type":"noul","instructions":"?"}}}`))
	require.NoError(t, err)
	c.Set(ctxkey.ConvertedRequest, request)
	return c, writer
}

// TestNativeResponsePreserved verifies byte preservation and independently billed input.
func TestNativeResponsePreserved(t *testing.T) {
	c, writer := responseContext(t)
	body := `{"model":"jev-1.13.0","answers":{"q":{"type":"noul","noul":0.9234567890123456789}},"usage":{"input_tokens":312,"output_tokens":999999},"extension":{"id":9007199254740993}}`
	reader := &trackedBody{Reader: strings.NewReader(body)}
	result, usage, apiErr := (&Adaptor{}).ReadResponse(c, &http.Response{StatusCode: 200, Body: reader}, nil)
	require.Nil(t, apiErr)
	require.True(t, reader.closed)
	require.Equal(t, 312, usage.PromptTokens)
	require.Equal(t, 999999, usage.CompletionTokens)
	charge, err := InputQuota(usage.PromptTokens, 0.021, 1)
	require.NoError(t, err)
	require.EqualValues(t, 7, charge)
	require.False(t, c.Writer.Written(), "billing must precede downstream output")
	result.Write(c)
	require.Equal(t, body, writer.Body.String())
}

// TestResponseEvidenceSeparatesFailuresFromMeasuredUsage covers unsafe zero-cost fallbacks.
func TestResponseEvidenceSeparatesFailuresFromMeasuredUsage(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		wantInput  int
		estimated  bool
	}{
		{"missing", `{"model":"jev-latest","answers":{}}`, AdmissionInputTokens, true},
		{"zero", `{"usage":{"input_tokens":0,"output_tokens":1}}`, AdmissionInputTokens, true},
		{"negative", `{"usage":{"input_tokens":-1,"output_tokens":1}}`, AdmissionInputTokens, true},
		{"fractional", `{"usage":{"input_tokens":1.5,"output_tokens":1}}`, AdmissionInputTokens, true},
		{"overflow", `{"usage":{"input_tokens":999999999999999999999999,"output_tokens":1}}`, AdmissionInputTokens, true},
		{"duplicate", `{"usage":{"input_tokens":1,"input_tokens":99,"output_tokens":1}}`, AdmissionInputTokens, true},
		{"bad_answers", `{"model":"jev-latest","answers":{},"usage":{"input_tokens":312,"output_tokens":1}}`, 312, false},
		{"wrong_id", `{"model":"jev-latest","answers":{"other":{"type":"noul","noul":1}},"usage":{"input_tokens":312,"output_tokens":1}}`, 312, false},
		{"wrong_type", `{"model":"jev-latest","answers":{"q":{"type":"choice"}},"usage":{"input_tokens":312,"output_tokens":1}}`, 312, false},
		{"bad_output", `{"usage":{"input_tokens":312,"output_tokens":-1}}`, 312, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := responseContext(t)
			result, usage, apiErr := (&Adaptor{}).ReadResponse(c, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body))}, nil)
			require.Nil(t, result)
			require.NotNil(t, apiErr)
			require.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
			require.Equal(t, tc.wantInput, usage.PromptTokens)
			require.Equal(t, tc.estimated, usage.BillingEstimateReason != "")
		})
	}
}

// TestHTTPRejectionsAndInterruptedBodies checks refunds, retry guidance and cleanup.
func TestHTTPRejectionsAndInterruptedBodies(t *testing.T) {
	for status, body := range map[int]string{
		// Bodies captured from the live service on 2026-09-18.
		400: `{"detail":{"error_type":"max_tokens_exceeded"}}`,
		401: `{"detail":{"error_type":"authentication_error","message":"Cannot authenticate with the server. Please check your API key and try again."}}`,
		403: `{"detail":{"error_type":"authentication_error","message":"Must supply an API key! Check your request and try again."}}`,
		404: `{"detail":"Not Found"}`,
		405: `{"detail":"Method Not Allowed"}`,
		422: `{"detail":[{"type":"missing","loc":["body","state"],"msg":"Field required"}]}`,
		429: `{"detail":"rate limited"}`,
		529: `{"detail":"overloaded"}`,
		500: `{"detail":"server error"}`,
		503: `{"detail":"unavailable"}`,
	} {
		c, writer := responseContext(t)
		result, usage, apiErr := (&Adaptor{}).ReadResponse(c, &http.Response{StatusCode: status,
			Header: http.Header{"Retry-After": {"10"}}, Body: io.NopCloser(strings.NewReader(body))}, nil)
		require.NotNil(t, apiErr)
		require.Equal(t, status, apiErr.StatusCode)
		if IsAdmissionRejection(status) {
			require.Zero(t, usage.PromptTokens)
		} else {
			require.Equal(t, AdmissionInputTokens, usage.PromptTokens)
			require.NotEmpty(t, usage.BillingEstimateReason)
		}
		result.Write(c)
		require.Equal(t, body, writer.Body.String(), "native error body must be forwarded verbatim")
		require.Equal(t, "10", writer.Header().Get("Retry-After"))
	}
	c, _ := responseContext(t)
	body := &trackedBody{Reader: failingReader{}}
	result, usage, apiErr := (&Adaptor{}).ReadResponse(c, &http.Response{StatusCode: 200, Body: body}, nil)
	require.Nil(t, result)
	require.NotNil(t, apiErr)
	require.True(t, body.closed)
	require.Equal(t, AdmissionInputTokens, usage.PromptTokens)
}

// TestUpstreamRequestIDIsForwarded keeps estimated charges reconcilable: without
// the provider's own request id an operator cannot match a retained reservation
// to an upstream attempt.
func TestUpstreamRequestIDIsForwarded(t *testing.T) {
	const requestID = "req_01a0b48b219b7d528f238466de776e44"
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"success", `{"model":"jev-1.13.0","answers":{"q":{"type":"noul","noul":0.99}},"usage":{"input_tokens":370,"output_tokens":61}}`, 200},
		{"rejection", `{"detail":{"error_type":"max_tokens_exceeded"}}`, 400},
		{"ambiguous", `{"detail":"server error"}`, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, writer := responseContext(t)
			header := http.Header{RequestIDHeader: {requestID}, "Retry-After-Ms": {"1500"}}
			result, _, _ := (&Adaptor{}).ReadResponse(c, &http.Response{StatusCode: tc.status, Header: header,
				Body: io.NopCloser(strings.NewReader(tc.body))}, nil)
			require.Equal(t, requestID, c.GetString(ctxkey.UpstreamRequestId))
			require.NotNil(t, result)
			result.Write(c)
			require.Equal(t, requestID, writer.Header().Get(RequestIDHeader))
			require.Equal(t, "1500", writer.Header().Get("Retry-After-Ms"))
		})
	}
	// An interrupted body still records the identifier it already observed.
	c, _ := responseContext(t)
	_, _, apiErr := (&Adaptor{}).ReadResponse(c, &http.Response{StatusCode: 200,
		Header: http.Header{RequestIDHeader: {requestID}}, Body: &trackedBody{Reader: failingReader{}}}, nil)
	require.NotNil(t, apiErr)
	require.Equal(t, requestID, c.GetString(ctxkey.UpstreamRequestId))
}

// TestMeasuredUsageOutranksErrorStatus ensures evidence wins over classification:
// a rejection that nevertheless reports consumed input is billed for that input.
func TestMeasuredUsageOutranksErrorStatus(t *testing.T) {
	c, _ := responseContext(t)
	_, usage, apiErr := (&Adaptor{}).ReadResponse(c, &http.Response{StatusCode: 400,
		Body: io.NopCloser(strings.NewReader(`{"detail":"late failure","usage":{"input_tokens":312,"output_tokens":0}}`))}, nil)
	require.NotNil(t, apiErr)
	require.Equal(t, 312, usage.PromptTokens)
	require.Empty(t, usage.BillingEstimateReason, "a measured receipt is not an estimate")
}

// TestLiveAnswerShapesAreForwarded pins the three primitives' real answer bodies,
// including noul's missing confidence field and score's string-keyed legend.
func TestLiveAnswerShapesAreForwarded(t *testing.T) {
	const body = `{"model":"jev-1.13.0","answers":{"positive":{"type":"noul","noul":0.99},` +
		`"topic":{"type":"choice","choice":"food","confidence":0.98,"probabilities":{"service":0.01,"food":0.99}},` +
		`"quality":{"type":"score","score":2.0,"confidence":1.0,"legend":{"0":"Poor","1":"Average","2":"Excellent"},` +
		`"probabilities":{"0":0.0,"1":0.0,"2":1.0}}},"usage":{"input_tokens":370,"output_tokens":61}}`
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/systemone", nil)
	gmw.SetLogger(c, logger.Logger)
	request, err := DecodeRequest([]byte(`{"model":"jev-latest","state":{"review":"The food was excellent and service was quick."},` +
		`"questions":{"positive":{"type":"noul","instructions":"Is the review positive?"},` +
		`"topic":{"type":"choice","instructions":"Choose the primary topic.","criteria":{"food":"Food quality","service":"Service quality"}},` +
		`"quality":{"type":"score","instructions":"Rate the overall experience.","criteria":["Poor","Average","Excellent"]}}}`))
	require.NoError(t, err)
	c.Set(ctxkey.ConvertedRequest, request)
	result, usage, apiErr := (&Adaptor{}).ReadResponse(c, &http.Response{StatusCode: 200,
		Body: io.NopCloser(strings.NewReader(body))}, nil)
	require.Nil(t, apiErr)
	require.Equal(t, 370, usage.PromptTokens)
	require.Equal(t, 61, usage.CompletionTokens)
	result.Write(c)
	require.Equal(t, body, writer.Body.String())
}
