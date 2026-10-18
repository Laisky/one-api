package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// TestClientCompatibilityCodexFallbackFileHTTP rejects unqualified file conversion before real HTTP dispatch.
// The loopback returns a valid completion so an invalid file wire cannot hide behind a provider rejection.
func TestClientCompatibilityCodexFallbackFileHTTP(t *testing.T) {
	const balance = int64(1_000_000)
	cases := map[string]string{
		"pdf_id":      `{"type":"input_file","file_id":"file_synthetic_pdf","filename":"fixture.pdf"}`,
		"pdf_inline":  `{"type":"input_file","file_data":"data:application/pdf;base64,JVBERi0xLjQKJSVFT0YK","filename":"fixture.pdf"}`,
		"text_inline": `{"type":"input_file","file_data":"data:text/plain;base64,c3ludGhldGlj","filename":"fixture.txt"}`,
		"file_url":    `{"type":"input_file","file_url":"https://example.invalid/fixture.pdf"}`,
		"pdf_detail":  `{"type":"input_file","file_id":"file_synthetic_pdf","filename":"fixture.pdf","detail":"high"}`,
	}
	for name, part := range cases {
		t.Run(name, func(t *testing.T) {
			xaiVideoSetup(t, balance, false)
			enableStateForTest(t)
			observed := make(chan string, 1)
			var calls atomic.Int32
			provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, _ := io.ReadAll(io.LimitReader(r.Body, 64<<10))
				observed <- r.URL.Path + " " + string(body)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"chatcmpl_file_fixture","object":"chat.completion","model":"codex-file-fixture","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`)
			}))
			t.Cleanup(provider.Close)
			previous := client.HTTPClient
			client.HTTPClient = provider.Client()
			t.Cleanup(func() { client.HTTPClient = previous })
			payload := `{"model":"alias","max_output_tokens":32,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"inspect the attachment"},` + part + `]}]}`
			c, _, requestID := protocolContext(t, channeltype.OpenAICompatible, "codex-file-fixture", "/v1/responses", payload, provider.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2})
			clientCompatibilityCodexCleanLedger(t, requestID)
			require.False(t, supportsNativeResponseAPI(metalib.GetByContext(c)))
			apiErr := RelayResponseAPIHelper(c)
			drainCriticalTasks(t)
			if calls.Load() > 0 {
				t.Logf("unexpected synthetic fallback provider wire: %s", <-observed)
			}
			require.NotNil(t, apiErr, "unqualified Chat file shape must fail locally rather than reach the provider")
			require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			require.Equal(t, "response_fallback_not_supported", apiErr.Code)
			clientCompatibilityCodexNoSideEffects(t, c, requestID, calls.Load(), balance)
		})
	}
}
