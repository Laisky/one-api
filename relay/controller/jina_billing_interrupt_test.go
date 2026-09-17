package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// jinaReceiptFaultReader emits fragmented data followed by the requested error.
// No live provider, network timing, or sleeps are needed to reproduce truncation.
type jinaReceiptFaultReader struct {
	wire     string
	terminal error
}

// Read returns a bounded fragment, or the terminal result after all supplied bytes.
func (r *jinaReceiptFaultReader) Read(dst []byte) (int, error) {
	if len(r.wire) == 0 {
		return 0, r.terminal
	}
	n := copy(dst[:min(len(dst), 17)], r.wire)
	r.wire = r.wire[n:]
	return n, nil
}

// jinaReceiptFaultTransport counts actual dispatches and injects a provider response.
type jinaReceiptFaultTransport struct {
	wire, contentType string
	terminal          error
	calls             atomic.Int32
}

// RoundTrip supplies a deterministic response with an interruptible body.
func (r *jinaReceiptFaultTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.calls.Add(1)
	return &http.Response{StatusCode: http.StatusOK, Request: req,
		Header: http.Header{"Content-Type": []string{r.contentType}},
		Body:   io.NopCloser(&jinaReceiptFaultReader{wire: r.wire, terminal: r.terminal})}, nil
}

// TestJinaBillingInterruptedOCRLedger verifies real balance, token, consume-log
// and request-cost reconciliation for every supported chat-facing API format.
func TestJinaBillingInterruptedOCRLedger(t *testing.T) {
	const receipt = "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":20,\"total_tokens\":120}}\n\n"
	for _, path := range []string{"/v1/chat/completions", "/v1/responses", "/v1/messages"} {
		for _, tc := range []struct {
			name, wire string
			terminal   error
			charge     int64
			estimated  bool
		}{
			{"complete_control", receipt + "data: [DONE]\n\n", io.EOF, 45, false},
			{"missing_done", receipt, io.EOF, -1, true},
			{"read_failure", receipt, io.ErrUnexpectedEOF, -1, true},
			{"cancelled_reader", receipt, context.Canceled, -1, true},
			{"output_after_receipt", receipt + "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"more paid output\"}}]}\n\ndata: [DONE]\n\n", io.EOF, -1, true},
			{"larger_known_work", "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1000000,\"completion_tokens\":20,\"total_tokens\":1000020}}\n\n", io.ErrUnexpectedEOF, 250032, true},
			{"unattributed_work_records_debt", "data: {\"choices\":[],\"usage\":{\"total_tokens\":1000000}}\n\n", io.EOF, 1250000, true},
		} {
			t.Run(fmt.Sprintf("%s/%s", path, tc.name), func(t *testing.T) {
				const balance = int64(1000000)
				transport := &jinaReceiptFaultTransport{wire: tc.wire, contentType: "text/event-stream", terminal: tc.terminal}
				old := client.HTTPClient
				client.HTTPClient = &http.Client{Transport: transport}
				defer func() { client.HTTPClient = old }()
				payload := map[string]any{"model": "alias", "stream": true}
				if path == "/v1/responses" {
					payload["input"] = "read this image"
					payload["max_output_tokens"] = 32
				} else {
					payload["messages"] = []map[string]any{{"role": "user", "content": "read this image"}}
					payload["max_tokens"] = 32
				}
				body, err := json.Marshal(payload)
				require.NoError(t, err)
				c, requestID := jinaAuditContext(t, path, string(body), "https://api.jina.ai", "jina-ocr-v1", balance, false, 1, -1)
				var apiErr *relaymodel.ErrorWithStatusCode
				switch path {
				case "/v1/responses":
					apiErr = RelayResponseAPIHelper(c)
				case "/v1/messages":
					apiErr = RelayClaudeMessagesHelper(c)
				default:
					apiErr = RelayTextHelper(c)
				}
				if tc.estimated {
					require.NotNil(t, apiErr)
					require.True(t, JinaAttemptMayHaveCost(c))
				} else {
					require.Nil(t, apiErr)
				}
				charge := tc.charge
				if charge < 0 {
					charge = c.GetInt64(ctxkey.PreConsumedQuotaAmount)
					require.Greater(t, charge, int64(45), "an intermediate receipt cannot release the hold")
				}
				jinaAuditAssertLedger(t, requestID, balance, charge, false, tc.estimated)
				require.EqualValues(t, 1, transport.calls.Load(), "accounting must not replay paid inference")
			})
		}
	}
}

// TestJinaBillingInterruptedSearchLedger verifies a larger receipt survives both
// JSON truncation and body read errors all the way into real account debits.
func TestJinaBillingInterruptedSearchLedger(t *testing.T) {
	for _, path := range []string{"/v1/embeddings", "/v1/rerank"} {
		for _, terminal := range []error{io.EOF, io.ErrUnexpectedEOF} {
			t.Run(fmt.Sprintf("%s/%v", path, terminal), func(t *testing.T) {
				const balance = int64(1000000)
				transport := &jinaReceiptFaultTransport{wire: `{"usage":{"total_tokens":1000000,`, contentType: "application/json", terminal: terminal}
				old := client.HTTPClient
				client.HTTPClient = &http.Client{Transport: transport}
				defer func() { client.HTTPClient = old }()
				body, actual := `{"model":"alias","input":["hello"]}`, "jina-embeddings-v3"
				if path == "/v1/rerank" {
					body, actual = `{"model":"alias","query":"q","documents":["one","two"]}`, "jina-reranker-v3.5"
				}
				c, requestID := jinaAuditContext(t, path, body, "https://api.jina.ai", actual, balance, false, 1, -1)
				var apiErr *relaymodel.ErrorWithStatusCode
				if path == "/v1/rerank" {
					apiErr = RelayRerankHelper(c)
				} else {
					apiErr = RelayTextHelper(c)
				}
				require.NotNil(t, apiErr)
				require.Less(t, c.GetInt64(ctxkey.PreConsumedQuotaAmount), int64(25000))
				jinaAuditAssertLedger(t, requestID, balance, 25000, false, true)
				require.EqualValues(t, 1, transport.calls.Load())
			})
		}
	}
}
