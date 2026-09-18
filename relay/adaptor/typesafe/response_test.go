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
	for _, status := range []int{401, 422, 429, 529, 500, 503} {
		c, writer := responseContext(t)
		result, usage, apiErr := (&Adaptor{}).ReadResponse(c, &http.Response{StatusCode: status,
			Header: http.Header{"Retry-After": {"10"}}, Body: io.NopCloser(strings.NewReader(`{"detail":"try later"}`))}, nil)
		require.NotNil(t, apiErr)
		require.Equal(t, status, apiErr.StatusCode)
		if IsAdmissionRejection(status) {
			require.Zero(t, usage.PromptTokens)
		} else {
			require.Equal(t, AdmissionInputTokens, usage.PromptTokens)
			require.NotEmpty(t, usage.BillingEstimateReason)
		}
		result.Write(c)
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
