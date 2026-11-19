package anthropic

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// provenanceAdmissionCase describes one Claude admission rejection envelope.
type provenanceAdmissionCase struct {
	name, errorType, message string
	httpStatus, wantStatus   int
}

// provenanceAdmissionCases returns admission envelopes covering a matching
// provider status, a status-rewriting intermediary (HTTP 200) and the 429 that
// feeds the adaptive cooldown ladder only when the failure is provider-marked.
//
// Returns:
//   - []provenanceAdmissionCase: the cases.
func provenanceAdmissionCases() []provenanceAdmissionCase {
	return []provenanceAdmissionCase{
		{name: "rate_limit_429", errorType: "rate_limit_error", message: "Number of request tokens has exceeded your per-minute rate limit", httpStatus: http.StatusTooManyRequests, wantStatus: http.StatusTooManyRequests},
		{name: "invalid_request_rewritten_200", errorType: "invalid_request_error", message: "messages: field required", httpStatus: http.StatusOK, wantStatus: http.StatusBadRequest},
		{name: "authentication_401", errorType: "authentication_error", message: "invalid x-api-key", httpStatus: http.StatusUnauthorized, wantStatus: http.StatusUnauthorized},
	}
}

// provenanceAdmissionBody renders a receipt-free Claude error envelope.
//
// Parameters:
//   - errorType: the Claude error type.
//   - message: the Claude error message.
//
// Returns:
//   - string: the JSON body.
func provenanceAdmissionBody(errorType, message string) string {
	return `{"type":"error","error":{"type":"` + errorType + `","message":"` + message + `"}}`
}

// legacyAdmissionRejection rebuilds the admission literal exactly as it was
// constructed before provenance was recorded, so tests can prove the
// client-visible JSON did not change.
//
// Parameters:
//   - status: the classified admission status.
//   - errorType: the Claude error type.
//   - message: the Claude error message.
//
// Returns:
//   - *model.ErrorWithStatusCode: the legacy construction.
func legacyAdmissionRejection(status int, errorType, message string) *model.ErrorWithStatusCode {
	return &model.ErrorWithStatusCode{StatusCode: status, Error: model.Error{
		Type: model.ErrorType(errorType), Code: errorType,
		Message: message, RawError: errors.WithStack(&admissionRejectionError{}),
	}}
}

// requireProvenanceAdmission asserts failure is a provider-marked admission
// rejection whose proof, status and client-visible JSON are unchanged.
//
// Parameters:
//   - t: the test.
//   - failure: the admission rejection under test.
//   - tc: the case it was built from.
func requireProvenanceAdmission(t *testing.T, failure *model.ErrorWithStatusCode, tc provenanceAdmissionCase) {
	t.Helper()
	require.NotNil(t, failure)
	require.True(t, failure.FromUpstream, "a provider admission rejection must be provider-marked")
	require.False(t, failure.IsOneAPIError())
	require.True(t, IsAdmissionRejection(failure), "the private admission proof must survive")
	require.Equal(t, tc.wantStatus, failure.StatusCode)

	legacy := legacyAdmissionRejection(tc.wantStatus, tc.errorType, tc.message)
	got, err := json.Marshal(failure)
	require.NoError(t, err)
	want, err := json.Marshal(legacy)
	require.NoError(t, err)
	require.Equal(t, string(want), string(got), "client-visible JSON must stay byte-identical")
	gotError, err := json.Marshal(failure.Error)
	require.NoError(t, err)
	wantError, err := json.Marshal(legacy.Error)
	require.NoError(t, err)
	require.Equal(t, string(wantError), string(gotError))
}

// TestClaudeMessageAdmissionRejectionProvenance requires the MCP-round admission
// parser to mark the provider's rejection FromUpstream while keeping its proof
// and byte-identical JSON.
func TestClaudeMessageAdmissionRejectionProvenance(t *testing.T) {
	for _, tc := range provenanceAdmissionCases() {
		t.Run(tc.name, func(t *testing.T) {
			failure := ParseClaudeMessageAdmissionRejection([]byte(provenanceAdmissionBody(tc.errorType, tc.message)), tc.httpStatus, nil)
			requireProvenanceAdmission(t, failure, tc)
		})
	}
}

// TestClaudeHTTPAdmissionRejectionProvenance requires the native and converted
// non-streaming handlers to mark the provider's admission rejection
// FromUpstream while keeping its proof, its no-usage settlement and
// byte-identical JSON.
func TestClaudeHTTPAdmissionRejectionProvenance(t *testing.T) {
	handlers := map[string]func(*gin.Context, *http.Response, int, string) (*model.ErrorWithStatusCode, *model.Usage){
		"converted": Handler,
		"native":    ClaudeNativeHandler,
	}
	for handlerName, handler := range handlers {
		for _, tc := range provenanceAdmissionCases() {
			t.Run(handlerName+"/"+tc.name, func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				resp := &http.Response{StatusCode: tc.httpStatus, Header: make(http.Header),
					Body: io.NopCloser(strings.NewReader(provenanceAdmissionBody(tc.errorType, tc.message)))}
				failure, usage := handler(c, resp, 0, "claude-sonnet-5-5")
				requireProvenanceAdmission(t, failure, tc)
				require.Nil(t, usage, "an admission rejection carries no usage")
				require.Empty(t, rec.Body.String())
			})
		}
	}
}
