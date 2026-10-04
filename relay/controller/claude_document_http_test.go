package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestClaudeNativeDocumentHTTP verifies equal native PDF admission, unchanged
// outbound bytes, conservative missing-receipt settlement and measured controls.
func TestClaudeNativeDocumentHTTP(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		for _, scenario := range []string{"measured", "missing", "underfunded"} {
			t.Run(fmt.Sprintf("compressed=%v/%s", compressed, scenario), func(t *testing.T) {
				runClaudeDocumentHTTP(t, compressed, scenario, false)
			})
		}
	}
}

// TestClaudeConvertedDocumentHTTPControl keeps a binary document's real
// converted tool-string charge instead of applying the native-source allowance.
func TestClaudeConvertedDocumentHTTPControl(t *testing.T) {
	for _, scenario := range []string{"measured", "underfunded"} {
		t.Run(scenario, func(t *testing.T) { runClaudeDocumentHTTP(t, false, scenario, true) })
	}
}

// runClaudeDocumentHTTP executes production admission, registered adaptors,
// local TLS transport and SQLite settlement without any paid provider request.
func runClaudeDocumentHTTP(t *testing.T, compressed bool, scenario string, converted bool) {
	t.Helper()
	const actualModel = "claude-3-5-haiku-20241022"
	balance := int64(100000)
	if converted {
		balance = 4000000
	}
	if scenario == "underfunded" {
		balance = 100
	}
	xaiVideoSetup(t, balance, false)
	canonicalAdmissionConfiguration(t)
	pdf := claudeDocumentQuotePDF(t, compressed)
	document := map[string]any{"type": "document", "title": "Synthetic PDF", "source": map[string]any{"type": "base64", "media_type": "application/pdf", "data": pdf}}
	var calls atomic.Int32
	seen := make(chan claudeToolQuotaWire, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		select {
		case seen <- claudeToolQuotaWire{Body: body, Path: r.URL.Path, Err: err}:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		var response map[string]any
		if converted {
			response = map[string]any{"id": "synthetic-doc", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18}}
		} else {
			response = map[string]any{"id": "synthetic-doc", "type": "message", "role": "assistant", "model": actualModel, "content": []any{map[string]any{"type": "text", "text": "ok"}}, "stop_reason": "end_turn"}
			if scenario != "missing" {
				response["usage"] = map[string]any{"input_tokens": 11, "output_tokens": 7}
			}
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	t.Cleanup(server.Close)
	oldClient := client.HTTPClient
	client.HTTPClient = server.Client()
	client.HTTPClient.Timeout = 5 * time.Second
	t.Cleanup(func() { client.HTTPClient = oldClient })
	messages := []any{map[string]any{"role": "user", "content": []any{document}}}
	payload := map[string]any{"model": "alias", "max_tokens": 128, "messages": messages}
	kind := channeltype.Anthropic
	if converted {
		kind = channeltype.OpenAICompatible
		payload["messages"] = []any{
			map[string]any{"role": "user", "content": "Run the synthetic lookup."},
			map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": "synthetic-call", "name": "lookup", "input": map[string]any{}}}},
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "synthetic-call", "content": []any{document}}}},
		}
		payload["tools"] = []any{map[string]any{"name": "lookup", "input_schema": map[string]any{"type": "object"}}}
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	c, _, id := protocolContext(t, kind, actualModel, "/v1/messages", string(body), server.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
	apiErr := RelayClaudeMessagesHelper(c)
	drainCriticalTasks(t)
	t.Logf("DOCUMENT_HTTP converted=%v compressed=%v scenario=%s encoded_bytes=%d calls=%d hold=%d", converted, compressed, scenario, len(pdf), calls.Load(), c.GetInt64(ctxkey.PreConsumedQuotaAmount))
	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	if scenario == "underfunded" {
		require.Zero(t, calls.Load())
		require.NotNil(t, apiErr)
		require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
		require.Equal(t, balance, reloadUserQuota(t))
		require.Equal(t, balance, token.RemainQuota)
		var n int64
		require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Where("request_id = ?", id).Count(&n).Error)
		require.Zero(t, n)
		return
	}
	require.EqualValues(t, 1, calls.Load(), "native binary transport size must not cause text-price rejection")
	observed := <-seen
	require.NoError(t, observed.Err)
	require.Contains(t, string(observed.Body), pdf, "quote projection must not change provider document bytes")
	wantPath := "/v1/messages"
	if converted {
		wantPath = "/v1/chat/completions"
		var wire relaymodel.GeneralOpenAIRequest
		require.NoError(t, json.Unmarshal(observed.Body, &wire))
		var toolText string
		for _, message := range wire.Messages {
			if message.Role == "tool" {
				toolText = message.StringContent()
			}
		}
		require.Contains(t, toolText, pdf)
		floor := openai.CountTokenText(toolText, actualModel)
		require.Greater(t, floor, 65536)
		require.GreaterOrEqual(t, c.GetInt64(ctxkey.PreConsumedQuotaAmount), int64(floor))
	}
	require.Equal(t, wantPath, observed.Path)
	want := int64(18)
	if scenario == "missing" {
		require.NotNil(t, apiErr, "preserve native incomplete-receipt error semantics")
		want = c.GetInt64(ctxkey.PreConsumedQuotaAmount)
		require.Greater(t, want, int64(1000))
	} else {
		require.Nil(t, apiErr)
	}
	require.Equal(t, balance-want, reloadUserQuota(t))
	require.Equal(t, balance-want, token.RemainQuota)
	require.Equal(t, want, requestCostQuota(t, id))
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type IN ?", id, []int{model.LogTypeConsume, model.LogTypeProvisional}).Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Equal(t, model.LogTypeConsume, logs[0].Type)
	require.EqualValues(t, want, logs[0].Quota)
	if scenario == "missing" {
		require.Equal(t, true, logs[0].Metadata["billing_estimated"])
	}
}
