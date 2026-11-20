package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// errorBodySecret stands for provider-internal data (an internal host or an
// account identifier) that an upstream error body can carry and that must never
// reach an API caller (OWASP API8:2023 security misconfiguration, API10:2023
// unsafe consumption of APIs).
const errorBodySecret = "internal-account-123"

// errorBodyCountingReader serves remaining bytes of 'a', records how many bytes
// were consumed and whether it was closed, so tests can prove an error-body read
// is bounded and the body is released.
type errorBodyCountingReader struct {
	remaining int64
	read      atomic.Int64
	closed    atomic.Bool
}

// Read implements io.Reader for errorBodyCountingReader.
//
// Parameters:
//   - p: the destination buffer.
//
// Returns:
//   - int: the number of bytes written to p.
//   - error: io.EOF once the configured size was served.
func (r *errorBodyCountingReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	n := min(int64(len(p)), r.remaining)
	for i := range n {
		p[i] = 'a'
	}
	r.remaining -= n
	r.read.Add(n)
	return int(n), nil
}

// Close implements io.Closer for errorBodyCountingReader and records the call.
//
// Returns:
//   - error: always nil.
func (r *errorBodyCountingReader) Close() error {
	r.closed.Store(true)
	return nil
}

// errorBodyFailingReader fails every read with an error whose text carries a
// secret, standing for a transport error that embeds an upstream URL.
type errorBodyFailingReader struct{}

// Read implements io.Reader for errorBodyFailingReader.
//
// Parameters:
//   - p: the destination buffer, never written.
//
// Returns:
//   - int: always 0.
//   - error: a read failure whose text contains errorBodySecret.
func (errorBodyFailingReader) Read(p []byte) (int, error) {
	return 0, errors.Errorf("read tcp https://%s.example.invalid: connection reset", errorBodySecret)
}

// errorBodyResponse builds an upstream HTTP response for RelayErrorHandler.
//
// Parameters:
//   - status: the upstream HTTP status.
//   - contentType: the Content-Type header, or "" for none.
//   - body: the response body.
//
// Returns:
//   - *http.Response: the synthetic response.
func errorBodyResponse(status int, contentType string, body io.Reader) *http.Response {
	header := http.Header{}
	if contentType != "" {
		header.Set("Content-Type", contentType)
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(body)}
}

// errorBodyHandlers runs both relay error handlers on equivalent responses, so
// every disclosure assertion covers the plain and the context-aware variant.
//
// Parameters:
//   - t: the running test.
//   - build: returns a fresh upstream response for each handler.
//   - check: receives the handler name and the normalized error.
func errorBodyHandlers(t *testing.T, build func() *http.Response, check func(name string, got *model.ErrorWithStatusCode)) {
	t.Helper()
	check("plain", RelayErrorHandler(build()))
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	check("context", RelayErrorHandlerWithContext(c, build()))
}

// requireNoErrorBodyDisclosure requires that neither the client-visible JSON nor
// the diagnostic RawError of got carries the secret.
//
// Parameters:
//   - t: the running test.
//   - got: the normalized relay error.
//   - secret: the provider-internal text that must not appear.
func requireNoErrorBodyDisclosure(t *testing.T, got *model.ErrorWithStatusCode, secret string) {
	t.Helper()
	clientJSON, err := json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(clientJSON), secret, "the API caller must not receive the raw provider body")
	require.NotContains(t, got.Message, secret)
	require.NotContains(t, got.Param, secret)
	require.False(t, got.MessageContainsRawBody, "the generic message carries no body bytes")
	if got.RawError != nil {
		require.NotContains(t, got.RawError.Error(), secret, "diagnostics that reach logs must not carry the body")
	}
}

// errorBodyExpected fills the body size into a generic-message template.
//
// Parameters:
//   - template: the expected message; a "%d" verb stands for the body size.
//   - body: the upstream body whose length is substituted.
//
// Returns:
//   - string: the expected client-visible message.
func errorBodyExpected(template, body string) string {
	if !strings.Contains(template, "%d") {
		return template
	}
	return fmt.Sprintf(template, len(body))
}

// TestRelayErrorBodyNeverEchoed requires that unparseable and unrecognized
// provider bodies yield a generic message naming only status, sanitized media
// type and size, for both relay error handlers.
func TestRelayErrorBodyNeverEchoed(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body, want string
	}{
		{"html", "text/html; charset=utf-8", "<html>" + errorBodySecret + "</html>",
			"upstream returned HTTP 502 with an unrecognized error body (content-type text/html, %d bytes)"},
		{"json_unknown_keys", "application/json", `{"private_payload":"` + errorBodySecret + `"}`,
			"upstream returned HTTP 502 with an unrecognized error body (content-type application/json, %d bytes)"},
		{"json_message_object", "application/json", `{"message":{"detail":"` + errorBodySecret + `"}}`,
			"upstream returned HTTP 502 with an unrecognized error body (content-type application/json, %d bytes)"},
		{"json_array", "application/json", `["` + errorBodySecret + `"]`,
			"upstream returned HTTP 502 with an unrecognized error body (content-type application/json, %d bytes)"},
		{"json_string", "", `"` + errorBodySecret + `"`,
			"upstream returned HTTP 502 with an unrecognized error body (%d bytes)"},
		{"fastapi_detail_list", "application/json", `{"detail":[{"msg":"` + errorBodySecret + `"}]}`,
			"upstream returned HTTP 502 with an unrecognized error body (content-type application/json, %d bytes)"},
		{"plain_text", "text/plain", errorBodySecret,
			"upstream returned HTTP 502 with an unrecognized error body (content-type text/plain, %d bytes)"},
		{"null_root", "application/json", `null`,
			"upstream returned HTTP 502 with an unrecognized error body (content-type application/json, %d bytes)"},
		{"content_type_injection", "text/html\r\nX-Leak: " + errorBodySecret, "<html>" + errorBodySecret + "</html>",
			"upstream returned HTTP 502 with an unrecognized error body (%d bytes)"},
		{"empty", "text/html", "", "upstream returned HTTP 502 with an empty error body"},
		{"blank", "text/html", " \r\n\t", "upstream returned HTTP 502 with an empty error body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			errorBodyHandlers(t, func() *http.Response {
				return errorBodyResponse(http.StatusBadGateway, tc.contentType, strings.NewReader(tc.body))
			}, func(name string, got *model.ErrorWithStatusCode) {
				require.Equal(t, http.StatusBadGateway, got.StatusCode, name)
				require.Equal(t, errorBodyExpected(tc.want, tc.body), got.Message, name)
				require.Equal(t, "upstream_error", string(got.Type), name)
				require.Equal(t, "bad_response_status_code", got.Code, name)
				require.Equal(t, "502", got.Param, name)
				require.True(t, got.FromUpstream, name)
				require.Error(t, got.RawError, name)
				requireNoErrorBodyDisclosure(t, got, errorBodySecret)
			})
		})
	}
}

// TestRelayErrorBodyRecognizedMessages requires the parsed provider message to
// keep passing through, including the FastAPI string "detail" convention.
func TestRelayErrorBodyRecognizedMessages(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"openai_nested", `{"error":{"message":"visible","type":"server_error"},"debug":"` + errorBodySecret + `"}`, "visible"},
		{"fastapi_detail", `{"detail":"The specified version does not exist","trace":"` + errorBodySecret + `"}`, "The specified version does not exist"},
		{"message_before_detail", `{"message":"capacity","detail":"other"}`, "capacity"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			errorBodyHandlers(t, func() *http.Response {
				return errorBodyResponse(http.StatusBadGateway, "application/json", strings.NewReader(tc.body))
			}, func(name string, got *model.ErrorWithStatusCode) {
				require.Equal(t, tc.want, got.Message, name)
				require.Equal(t, http.StatusBadGateway, got.StatusCode, name)
				requireNoErrorBodyDisclosure(t, got, errorBodySecret)
			})
		})
	}
}

// TestRelayErrorBodyBoundedRead requires the error-body read to stop one byte
// past the 1 MiB limit (OWASP API4:2023 unrestricted resource consumption), the
// body to be closed, and an over-limit body to be reported without parsing.
func TestRelayErrorBodyBoundedRead(t *testing.T) {
	const limit = 1 << 20
	for _, withContext := range []bool{false, true} {
		reader := &errorBodyCountingReader{remaining: 8 << 20}
		resp := &http.Response{StatusCode: http.StatusBadGateway, Header: http.Header{"Content-Type": {"text/html"}}, Body: reader}
		var got *model.ErrorWithStatusCode
		if withContext {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			got = RelayErrorHandlerWithContext(c, resp)
			restored, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Len(t, restored, limit+1, "only the bounded prefix is kept for later readers")
		} else {
			got = RelayErrorHandler(resp)
		}
		require.LessOrEqual(t, reader.read.Load(), int64(limit+1), "error body read must be bounded")
		require.True(t, reader.closed.Load(), "the upstream body must be closed")
		require.Equal(t, http.StatusBadGateway, got.StatusCode)
		require.Equal(t, "upstream returned HTTP 502 with an error body larger than the 1048576-byte limit (content-type text/html)", got.Message)
		require.Less(t, len(got.Message), 200)
		require.True(t, got.FromUpstream)
	}
}

// TestRelayErrorBodyLimitBoundary requires a JSON body of exactly the limit to
// be parsed and one byte more to be treated as oversized: a truncated prefix
// is never parsed, since it could yield a misleading message.
func TestRelayErrorBodyLimitBoundary(t *testing.T) {
	const limit = 1 << 20
	build := func(size int) string {
		prefix, suffix := `{"error":{"message":"`, `"}}`
		return prefix + strings.Repeat("a", size-len(prefix)-len(suffix)) + suffix
	}
	atLimit := RelayErrorHandler(errorBodyResponse(http.StatusBadGateway, "application/json", strings.NewReader(build(limit))))
	require.Len(t, atLimit.Message, limit-len(`{"error":{"message":""}}`), "a body of exactly the limit is parsed")

	overLimit := RelayErrorHandler(errorBodyResponse(http.StatusBadGateway, "application/json", strings.NewReader(build(limit+1))))
	require.Equal(t, "upstream returned HTTP 502 with an error body larger than the 1048576-byte limit (content-type application/json)", overLimit.Message)
}

// TestRelayErrorBodyReadFailure requires a failed body read to yield a generic
// message: the transport error can carry upstream URLs, so it stays in
// RawError for diagnostics and never reaches the client.
func TestRelayErrorBodyReadFailure(t *testing.T) {
	errorBodyHandlers(t, func() *http.Response {
		return errorBodyResponse(http.StatusServiceUnavailable, "text/html", errorBodyFailingReader{})
	}, func(name string, got *model.ErrorWithStatusCode) {
		require.Equal(t, http.StatusServiceUnavailable, got.StatusCode, name)
		require.Equal(t, "upstream returned HTTP 503 with an error body that could not be read", got.Message, name)
		require.True(t, got.FromUpstream, name)
		clientJSON, err := json.Marshal(got)
		require.NoError(t, err)
		require.NotContains(t, string(clientJSON), errorBodySecret, name)
		require.ErrorContains(t, got.RawError, "read upstream error body", name)
	})
}
