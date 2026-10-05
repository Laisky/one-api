package zhipu

import (
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

// TestParseOCRReceipt verifies that only exact nonnegative integer counters
// within bounds become measured evidence, and that every other shape is
// labelled per dimension instead of being coerced to zero.
func TestParseOCRReceipt(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                       string
		body                       string
		prompt, completion, cached int
		usageProblem               string
		pages                      int
		pagesProblem               string
	}{
		{name: "measured", body: `{"usage":{"prompt_tokens":800,"completion_tokens":400,"total_tokens":1200,"prompt_tokens_details":{"cached_tokens":7}},"data_info":{"num_pages":3}}`,
			prompt: 800, completion: 400, cached: 7, pages: 3},
		{name: "explicit_zero", body: `{"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0},"data_info":{"num_pages":0}}`},
		{name: "total_optional", body: `{"usage":{"prompt_tokens":5,"completion_tokens":6}}`, prompt: 5, completion: 6, pagesProblem: model.OCRReceiptMissing},
		{name: "missing_usage", body: `{"data_info":{"num_pages":2}}`, usageProblem: model.OCRReceiptMissing, pages: 2},
		{name: "null_usage", body: `{"usage":null}`, usageProblem: model.OCRReceiptMissing, pagesProblem: model.OCRReceiptMissing},
		{name: "missing_counter", body: `{"usage":{"total_tokens":30}}`, usageProblem: model.OCRReceiptMissing, pagesProblem: model.OCRReceiptMissing},
		{name: "null_counter", body: `{"usage":{"prompt_tokens":null,"completion_tokens":2}}`, usageProblem: model.OCRReceiptMissing, pagesProblem: model.OCRReceiptMissing},
		{name: "negative", body: `{"usage":{"prompt_tokens":-1,"completion_tokens":2}}`, usageProblem: model.OCRReceiptInvalid, pagesProblem: model.OCRReceiptMissing},
		{name: "fraction", body: `{"usage":{"prompt_tokens":1.5,"completion_tokens":2}}`, usageProblem: model.OCRReceiptInvalid, pagesProblem: model.OCRReceiptMissing},
		{name: "exponent", body: `{"usage":{"prompt_tokens":1e3,"completion_tokens":2}}`, usageProblem: model.OCRReceiptInvalid, pagesProblem: model.OCRReceiptMissing},
		{name: "string", body: `{"usage":{"prompt_tokens":"10","completion_tokens":2}}`, usageProblem: model.OCRReceiptInvalid, pagesProblem: model.OCRReceiptMissing},
		{name: "usage_not_object", body: `{"usage":[1,2]}`, usageProblem: model.OCRReceiptInvalid, pagesProblem: model.OCRReceiptMissing},
		{name: "beyond_uint64", body: `{"usage":{"prompt_tokens":99999999999999999999999,"completion_tokens":2}}`, usageProblem: model.OCRReceiptOverflow, pagesProblem: model.OCRReceiptMissing},
		{name: "above_bound", body: `{"usage":{"prompt_tokens":67108865,"completion_tokens":2}}`, usageProblem: model.OCRReceiptOverflow, pagesProblem: model.OCRReceiptMissing},
		{name: "at_bound", body: `{"usage":{"prompt_tokens":67108864,"completion_tokens":67108864}}`, prompt: 67108864, completion: 67108864, pagesProblem: model.OCRReceiptMissing},
		{name: "inconsistent_total", body: `{"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":4}}`, usageProblem: model.OCRReceiptInconsistent, pagesProblem: model.OCRReceiptMissing},
		{name: "cached_above_prompt", body: `{"usage":{"prompt_tokens":1,"completion_tokens":2,"prompt_tokens_details":{"cached_tokens":5}}}`, usageProblem: model.OCRReceiptInconsistent, pagesProblem: model.OCRReceiptMissing},
		{name: "pages_invalid", body: `{"usage":{"prompt_tokens":1,"completion_tokens":2},"data_info":{"num_pages":"3"}}`, prompt: 1, completion: 2, pagesProblem: model.OCRReceiptInvalid},
		{name: "pages_overflow", body: `{"usage":{"prompt_tokens":1,"completion_tokens":2},"data_info":{"num_pages":100001}}`, prompt: 1, completion: 2, pagesProblem: model.OCRReceiptOverflow},
		{name: "top_level_array", body: `[{"usage":{"prompt_tokens":1}}]`, usageProblem: model.OCRReceiptUnreadable, pagesProblem: model.OCRReceiptUnreadable},
		{name: "not_json", body: `<html>gateway</html>`, usageProblem: model.OCRReceiptUnreadable, pagesProblem: model.OCRReceiptUnreadable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			receipt := ParseOCRReceipt([]byte(tc.body))
			require.Equal(t, tc.usageProblem, receipt.UsageProblem)
			require.Equal(t, tc.pagesProblem, receipt.PagesProblem)
			require.Equal(t, tc.pages, receipt.Pages)
			if tc.usageProblem != "" {
				require.Nil(t, receipt.Usage, "a problem must never come with fabricated counters")
				return
			}
			require.NotNil(t, receipt.Usage)
			require.Equal(t, tc.prompt, receipt.Usage.PromptTokens)
			require.Equal(t, tc.completion, receipt.Usage.CompletionTokens)
			require.Equal(t, tc.prompt+tc.completion, receipt.Usage.TotalTokens)
			if tc.cached > 0 {
				require.NotNil(t, receipt.Usage.PromptTokensDetails)
				require.Equal(t, tc.cached, receipt.Usage.PromptTokensDetails.CachedTokens)
			}
		})
	}
}

// ocrFaultBody injects a read fault after its payload and a close fault.
type ocrFaultBody struct {
	io.Reader
	readErr, closeErr error
}

// Read returns payload bytes and substitutes the configured fault for EOF.
func (b *ocrFaultBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if errors.Is(err, io.EOF) && b.readErr != nil {
		return n, b.readErr
	}
	return n, err
}

// Close returns the configured close fault.
func (b *ocrFaultBody) Close() error { return b.closeErr }

// ocrFailingWriter accepts headers but fails every body write, as a client
// that disconnected after the provider accepted the work would.
type ocrFailingWriter struct{ *httptest.ResponseRecorder }

// Write reports a reset client connection.
func (w ocrFailingWriter) Write([]byte) (int, error) { return 0, errors.New("client connection reset") }

// TestForwardOCRResponseEvidence verifies the receipt survives transport faults:
// a failed client write still returns the measured receipt, a close fault after
// a complete read is not an error, and an interrupted read is unreadable.
func TestForwardOCRResponseEvidence(t *testing.T) {
	t.Parallel()
	const measured = `{"usage":{"prompt_tokens":800,"completion_tokens":400,"total_tokens":1200},"data_info":{"num_pages":1}}`
	gin.SetMode(gin.TestMode)

	t.Run("client_delivery_failure_keeps_receipt", func(t *testing.T) {
		t.Parallel()
		c, _ := gin.CreateTestContext(ocrFailingWriter{httptest.NewRecorder()})
		receipt, apiErr := forwardOCRResponse(c, makeHTTPResponse(http.StatusOK, measured))
		require.NotNil(t, apiErr)
		require.Empty(t, receipt.UsageProblem)
		require.Equal(t, 1200, receipt.Usage.TotalTokens)
		require.Equal(t, 1, receipt.Pages)
	})
	t.Run("close_fault_after_complete_read", func(t *testing.T) {
		t.Parallel()
		c, w := newTestGinContext()
		resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
			Body: &ocrFaultBody{Reader: strings.NewReader(measured), closeErr: errors.New("close fault")}}
		receipt, apiErr := forwardOCRResponse(c, resp)
		require.Nil(t, apiErr)
		require.Equal(t, 1200, receipt.Usage.TotalTokens)
		require.JSONEq(t, measured, w.Body.String())
	})
	t.Run("interrupted_read_is_unreadable", func(t *testing.T) {
		t.Parallel()
		c, w := newTestGinContext()
		resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
			Body: &ocrFaultBody{Reader: strings.NewReader(measured), readErr: errors.New("connection reset")}}
		receipt, apiErr := forwardOCRResponse(c, resp)
		require.NotNil(t, apiErr)
		require.Equal(t, model.OCRReceiptUnreadable, receipt.UsageProblem)
		require.Empty(t, w.Body.String(), "a partial body is never forwarded")
	})
}
