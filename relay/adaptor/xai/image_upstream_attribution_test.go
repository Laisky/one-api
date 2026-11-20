package xai

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
	"github.com/Laisky/one-api/relay/relaymode"
)

// xaiImageAttributionSecret stands for private data a provider may put in an
// error body outside its documented message members; it must never reach the
// client.
const xaiImageAttributionSecret = "internal-account-123"

// TestXAIImageRefusalKeepsProviderStatusAndProvenance requires a non-2xx xAI
// image answer to be a provider-marked failure with the provider's HTTP status,
// never a fabricated 200 {"data":[]} (which refunded a 429 silently and charged a
// 503 as a success) nor a local 500 parse_response_failed. Only the parsed
// message and code may reach the client; the raw body never does, and a body
// without a recognized message gets the shared generic description (an empty
// wantMessage below).
func TestXAIImageRefusalKeepsProviderStatusAndProvenance(t *testing.T) {
	cases := []struct {
		name, contentType, body, wantMessage, wantCode string
		status                                         int
	}{
		{name: "flat_429_quota", status: http.StatusTooManyRequests, contentType: "application/json",
			body:        `{"code":"AccountQuotaExceeded","error":"You have exceeded the monthly usage quota.","team":"` + xaiImageAttributionSecret + `"}`,
			wantMessage: "upstream returned HTTP 429: You have exceeded the monthly usage quota.",
			wantCode:    "AccountQuotaExceeded"},
		{name: "flat_503_json", status: http.StatusServiceUnavailable, contentType: "application/json",
			body:        `{"error":"overloaded","host":"` + xaiImageAttributionSecret + `"}`,
			wantMessage: "upstream returned HTTP 503: overloaded",
			wantCode:    "upstream_image_error"},
		{name: "nested_400", status: http.StatusBadRequest, contentType: "application/json",
			body:        `{"error":{"message":"prompt too long","type":"invalid_request_error","code":"context_length","param":"` + xaiImageAttributionSecret + `"}}`,
			wantMessage: "upstream returned HTTP 400: prompt too long",
			wantCode:    "context_length"},
		{name: "top_level_message_403", status: http.StatusForbidden, contentType: "application/json",
			body:        `{"message":"team is blocked","trace":"` + xaiImageAttributionSecret + `"}`,
			wantMessage: "upstream returned HTTP 403: team is blocked",
			wantCode:    "upstream_image_error"},
		{name: "plain_text_503", status: http.StatusServiceUnavailable, contentType: "text/plain",
			body:     `upstream unavailable ` + xaiImageAttributionSecret,
			wantCode: "upstream_image_error"},
		{name: "non_string_members_502", status: http.StatusBadGateway, contentType: "application/json",
			body:     `{"error":{"message":{"x":"` + xaiImageAttributionSecret + `"}},"code":7}`,
			wantCode: "upstream_image_error"},
		{name: "oversized_429", status: http.StatusTooManyRequests, contentType: "application/json",
			body:     `{"error":"` + xaiImageAttributionSecret + strings.Repeat("a", model.MaxUpstreamErrorBodyBytes) + `"}`,
			wantCode: "upstream_image_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
			header := http.Header{}
			header.Set("Content-Type", tc.contentType)
			resp := &http.Response{StatusCode: tc.status, Header: header, Body: io.NopCloser(strings.NewReader(tc.body))}

			usage, failure := (&Adaptor{}).DoResponse(c, resp, &meta.Meta{Mode: relaymode.ImagesGenerations})
			require.Nil(t, usage)
			require.NotNil(t, failure, "a provider refusal must be a failed attempt, not a fabricated success")
			require.Equal(t, tc.status, failure.StatusCode, "the provider status must be preserved")
			require.True(t, failure.FromUpstream)
			require.False(t, failure.IsOneAPIError())
			wantMessage := tc.wantMessage
			if wantMessage == "" {
				// No recognized message: the shared generic description applies.
				wantMessage = model.UpstreamErrorBodyMessage(tc.status, tc.contentType, []byte(tc.body))
				require.True(t, strings.HasPrefix(wantMessage, fmt.Sprintf("upstream returned HTTP %d with ", tc.status)))
			}
			require.Equal(t, wantMessage, failure.Message)
			require.Equal(t, tc.wantCode, failure.Code)

			encoded, err := json.Marshal(failure)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), xaiImageAttributionSecret, "raw provider body must not be echoed")
			require.Empty(t, rec.Body.String(), "the adaptor must not write a response for a refusal")
			require.False(t, c.Writer.Written())
		})
	}
}
