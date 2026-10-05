package cohere

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

	metalib "github.com/Laisky/one-api/relay/meta"
)

// TestCohereSearchReceiptDecode checks presence, invalid numbers, and reused receivers.
func TestCohereSearchReceiptDecode(t *testing.T) {
	for _, tc := range []struct {
		name, body, reason string
		count              int
	}{
		{name: "one", body: `{"search_units":1}`, count: 1},
		{name: "many", body: `{"search_units":19}`, count: 19},
		{name: "decimal", body: `{"search_units":3.0}`, count: 3},
		{name: "exponent", body: `{"search_units":3e0}`, count: 3},
		{name: "negative_exponent", body: `{"search_units":300e-2}`, count: 3},
		{name: "positive_exponent", body: `{"search_units":0.003e3}`, count: 3},
		{name: "large_exact", body: `{"search_units":9007199254740993.0}`, count: 9007199254740993},
		{name: "max_exact", body: `{"search_units":9223372036854775807.0}`, count: 9223372036854775807},
		{name: "oversized_exponent", body: `{"search_units":3e999999999}`, reason: cohereSearchUnitsInvalid},
		{name: "oversized_negative_exponent", body: `{"search_units":3e-999999999}`, reason: cohereSearchUnitsInvalid},
		{name: "tiny_fraction", body: `{"search_units":3.0000000000000000001}`, reason: cohereSearchUnitsInvalid},
		{name: "exponent_overflow", body: `{"search_units":9223372036854775808e0}`, reason: cohereSearchUnitsInvalid},
		{name: "missing", body: `{}`, reason: cohereSearchUnitsMissing},
		{name: "null", body: `{"search_units":null}`, reason: cohereSearchUnitsMissing},
		{name: "zero", body: `{"search_units":0}`, reason: cohereSearchUnitsInvalid},
		{name: "negative", body: `{"search_units":-1}`, reason: cohereSearchUnitsInvalid},
		{name: "fraction", body: `{"search_units":1.5}`, reason: cohereSearchUnitsInvalid},
		{name: "string", body: `{"search_units":"3"}`, reason: cohereSearchUnitsInvalid},
		{name: "boolean", body: `{"search_units":true}`, reason: cohereSearchUnitsInvalid},
		{name: "overflow", body: `{"search_units":9223372036854775808}`, reason: cohereSearchUnitsInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			units := RerankBilledUnits{SearchUnits: 99}
			require.NoError(t, json.Unmarshal([]byte(tc.body), &units))
			require.Equal(t, tc.count, units.SearchUnits)
			require.Equal(t, tc.reason, units.receiptReason)
		})
	}
}

// cohereReceiptBody records actual closure and optionally fails after a complete receipt.
type cohereReceiptBody struct {
	io.Reader
	closeError error
	closed     int
}

// Close records a single body close and returns the configured synthetic error.
func (body *cohereReceiptBody) Close() error {
	body.closed++
	return body.closeError
}

// cohereReceiptWriter preserves Gin's transport methods but rejects client delivery.
type cohereReceiptWriter struct {
	gin.ResponseWriter
}

// Write rejects downstream delivery with a synthetic error instead of accepting bytes.
func (writer *cohereReceiptWriter) Write([]byte) (int, error) {
	return 0, errors.New("synthetic downstream delivery failure")
}

// TestCohereRerankReceiptSurvivesDeliveryFailure preserves real errors and owned usage.
func TestCohereRerankReceiptSurvivesDeliveryFailure(t *testing.T) {
	for _, tc := range []struct {
		name       string
		closeFail  bool
		writeFail  bool
		malformed  bool
		statusCode int
	}{
		{name: "successful_control", statusCode: http.StatusOK},
		{name: "close_failure", closeFail: true, statusCode: http.StatusOK},
		{name: "client_failure", writeFail: true, statusCode: http.StatusOK},
		{name: "missing_body", malformed: true, statusCode: http.StatusOK},
		{name: "rejected_provider", statusCode: http.StatusTooManyRequests},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/rerank", nil)
			if tc.writeFail {
				c.Writer = &cohereReceiptWriter{ResponseWriter: c.Writer}
			}
			payload := `{"results":[],"meta":{"billed_units":{"search_units":3},"tokens":{"input_tokens":11}}}`
			if tc.malformed {
				payload = `{"results":[`
			}
			body := &cohereReceiptBody{Reader: strings.NewReader(payload)}
			if tc.closeFail {
				body.closeError = errors.New("synthetic close failure")
			}
			apiErr, usage := RerankHandler(c, &http.Response{StatusCode: tc.statusCode, Body: body},
				&metalib.Meta{ActualModelName: "rerank-v3.5", PromptTokens: 7})
			require.Equal(t, 1, body.closed)
			if tc.statusCode != http.StatusOK {
				require.NotNil(t, apiErr)
				require.Nil(t, usage)
				return
			}
			require.NotNil(t, usage)
			if tc.malformed {
				require.NotNil(t, apiErr)
				require.Nil(t, usage.BilledSearchUnits)
				require.Equal(t, "cohere_rerank_response_incomplete", usage.BillingEstimateReason)
				return
			}
			require.NotNil(t, usage.BilledSearchUnits)
			require.EqualValues(t, 3, *usage.BilledSearchUnits)
			require.Equal(t, 11, usage.PromptTokens)
			if tc.closeFail || tc.writeFail {
				require.NotNil(t, apiErr)
			} else {
				require.Nil(t, apiErr)
			}
		})
	}
}
