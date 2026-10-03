package jina

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

const auditOCRReceipt = `{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120}}`

// receiptFaultBody exposes a wire prefix and then the requested transport error,
// optionally returning the error alongside the final bytes rather than later.
type receiptFaultBody struct {
	wire     string
	terminal error
	sameRead bool
	closeErr error
	closed   bool
}

// Read preserves fragment boundaries and injects the configured terminal error.
func (b *receiptFaultBody) Read(dst []byte) (int, error) {
	if b.wire == "" {
		return 0, b.terminal
	}
	n := copy(dst, b.wire)
	b.wire = b.wire[n:]
	if b.wire == "" && b.sameRead {
		return n, b.terminal
	}
	return n, nil
}

// Close records release of the body and exposes a configured close error.
func (b *receiptFaultBody) Close() error { b.closed = true; return b.closeErr }

// TestOCRReceiptRequiresUpstreamCompletion verifies that intermediate positive
// receipts, transport errors and output after a receipt cannot authorize refunds.
func TestOCRReceiptRequiresUpstreamCompletion(t *testing.T) {
	t.Parallel()
	chunk := "data: " + auditOCRReceipt + "\r\n\r\n"
	for _, tc := range []struct {
		name, wire string
		terminal   error
		valid      bool
	}{
		{"complete", chunk + "data: [DONE]\r\n\r\n", io.EOF, true},
		{"repeated_cumulative", chunk + chunk + "data: [DONE]\n\n", io.EOF, true},
		{"usage_without_done", chunk, io.EOF, false},
		{"unexpected_eof", chunk, io.ErrUnexpectedEOF, false},
		{"cancelled", chunk, context.Canceled, false},
		{"deadline", chunk, context.DeadlineExceeded, false},
		{"unbilled_tail", chunk + "data: {\"choices\":[{\"delta\":{\"content\":\"more paid output\"}}]}\n\ndata: [DONE]\n\n", io.EOF, false},
		{"error_after_usage", chunk + "data: {\"error\":{\"message\":\"failed\"}}\n\ndata: [DONE]\n\n", io.EOF, false},
		{"data_after_done", chunk + "data: [DONE]\n\n" + chunk, io.EOF, false},
		{"json_complete", auditOCRReceipt, io.EOF, true},
		{"json_read_failure", auditOCRReceipt, io.ErrUnexpectedEOF, false},
	} {
		for _, sameRead := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/sameRead=%v", tc.name, sameRead), func(t *testing.T) {
				t.Parallel()
				body := &receiptFaultBody{wire: tc.wire, terminal: tc.terminal, sameRead: sameRead}
				observer := &receiptBody{ReadCloser: body, stream: !strings.HasPrefix(tc.name, "json_")}
				buffer := make([]byte, 7)
				for {
					_, err := observer.Read(buffer)
					if err != nil {
						require.ErrorIs(t, err, tc.terminal)
						break
					}
				}
				require.NoError(t, observer.Close())
				require.True(t, body.closed)
				usage, invalid := observer.snapshot()
				require.NotNil(t, usage, "known work survives an interrupted response")
				require.Equal(t, tc.valid, !invalid)
				require.Equal(t, 100, usage.PromptTokens)
				require.Equal(t, 20, usage.CompletionTokens, "cumulative snapshots are not summed")
			})
		}
	}
}

// TestOCRReceiptCloseCannotCertifyUnreadTail checks the exact boundary that
// cancelled stream readers and early-stopping JSON consumers can leave behind.
func TestOCRReceiptCloseCannotCertifyUnreadTail(t *testing.T) {
	t.Parallel()
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			wire := auditOCRReceipt
			if stream {
				wire = "data: " + wire + "\n\n"
			}
			observer := &receiptBody{ReadCloser: io.NopCloser(strings.NewReader(wire)), stream: stream}
			buffer := make([]byte, len(wire))
			n, err := observer.Read(buffer)
			require.NoError(t, err)
			require.Equal(t, len(wire), n)
			require.NoError(t, observer.Close())
			usage, invalid := observer.snapshot()
			require.NotNil(t, usage)
			require.True(t, invalid, "Close does not prove that no paid tail was lost")
		})
	}
}

// TestSearchInterruptedReceiptKeepsHigherEvidence verifies a partial or unreadable
// response cannot discard already-observed work greater than the original hold.
func TestSearchInterruptedReceiptKeepsHigherEvidence(t *testing.T) {
	t.Parallel()
	for _, mode := range []int{relaymode.Embeddings, relaymode.Rerank} {
		for _, wire := range []string{
			`{"usage":{"total_tokens":100000},"data":[],"results":[]}`,
			`{"usage":{"total_tokens":100000,`,
			`{"usage":{"total_tokens":100000},"data":[`,
			`{"usage":{"total_tokens":100000,"total_tokens":1}}`,
			`{"usage":{"total_tokens":100000},"usage":{"total_tokens":1}}`,
		} {
			for _, terminal := range []error{io.EOF, io.ErrUnexpectedEOF} {
				t.Run(fmt.Sprintf("%d/%s/%v", mode, wire, terminal), func(t *testing.T) {
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					StoreBillingBudget(c, BillingBudget{Input: 100})
					body := &receiptFaultBody{wire: wire, terminal: terminal, sameRead: true}
					usage, apiErr := handleSearchResponse(c, &http.Response{StatusCode: http.StatusOK, Body: body}, mode)
					if terminal == io.EOF && wire == `{"usage":{"total_tokens":100000},"data":[],"results":[]}` {
						require.Nil(t, apiErr)
					} else {
						require.NotNil(t, apiErr)
					}
					require.NotNil(t, usage)
					require.GreaterOrEqual(t, usage.PromptTokens, 100000)
					require.Zero(t, usage.CompletionTokens)
					require.True(t, body.closed)
					if terminal != io.EOF {
						require.NotEmpty(t, usage.BillingEstimateReason)
					}
				})
			}
		}
	}
}

// TestOCRUnattributedUsageCoversEitherPricingDirection ensures conservative
// attribution is safe even when a channel override makes input the expensive side.
func TestOCRUnattributedUsageCoversEitherPricingDirection(t *testing.T) {
	t.Parallel()
	for _, completionRatio := range []float64{0, .1, 1, 4, 20} {
		estimate := mergeReceiptEstimate(&model.Usage{PromptTokens: 500, CompletionTokens: 32, BillingEstimateReason: "test"},
			&model.Usage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 10000}, false)
		charge, err := BudgetQuota(BillingBudget{Input: estimate.PromptTokens, Output: estimate.CompletionTokens}, 1, completionRatio, 1)
		require.NoError(t, err)
		for _, actual := range []BillingBudget{{Input: 9980, Output: 20}, {Input: 100, Output: 9900}} {
			minimum, err := BudgetQuota(actual, 1, completionRatio, 1)
			require.NoError(t, err)
			require.GreaterOrEqual(t, charge, minimum)
		}
	}
	require.Equal(t, math.MaxInt, saturatedTokenSum(math.MaxInt, 1))
	require.Equal(t, math.MaxInt, saturatedTokenSum(1, math.MaxInt))
}
