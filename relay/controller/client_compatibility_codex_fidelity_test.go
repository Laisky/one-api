package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/state"
)

const clientCompatibilityCodexCalls = `{"type":"function_call","id":"fc_alpha","call_id":"call_alpha","namespace":"alpha","name":"lookup","arguments":"{}"},{"type":"function_call","id":"fc_beta","call_id":"call_beta","namespace":"beta","name":"lookup","arguments":"{}"}`
const clientCompatibilityCodexOutputs = `{"type":"function_call_output","call_id":"call_alpha","output":"alpha-result"},{"type":"function_call_output","call_id":"call_beta","output":"beta-result"}`

// clientCompatibilityCodexCleanLedger isolates repeated disposable SQLite fixtures.
func clientCompatibilityCodexCleanLedger(t *testing.T, requestID string) {
	t.Helper()
	require.NoError(t, model.DB.Where("request_id = ?", requestID).Delete(&model.UserRequestCost{}).Error)
	require.NoError(t, model.LOG_DB.Where("request_id = ?", requestID).Delete(&model.Log{}).Error)
}

// clientCompatibilityCodexNoSideEffects checks admission before reservations and pending state.
func clientCompatibilityCodexNoSideEffects(t *testing.T, c *gin.Context, requestID string, calls int32, balance int64) {
	t.Helper()
	require.Zero(t, calls, "unsafe fallback must never reach provider")
	require.Zero(t, c.GetInt64(ctxkey.PreConsumedQuotaAmount))
	require.Nil(t, pendingCommitFromContext(c), "rejection must not stage response state")
	require.Equal(t, balance, reloadUserQuota(t))
	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	require.Equal(t, balance, token.RemainQuota)
	require.Zero(t, token.UsedQuota)
	var costs, logs int64
	require.NoError(t, model.DB.Model(&model.UserRequestCost{}).Where("request_id = ?", requestID).Count(&costs).Error)
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("request_id = ?", requestID).Count(&logs).Error)
	require.Zero(t, costs)
	require.Zero(t, logs)
}

// clientCompatibilityCodexSeedParent stores a completed parent response whose
// output items are parent, so a request can hydrate it by previous_response_id.
// Parameters: t owns assertions, store is the enabled state store and parent
// is the comma-joined output item JSON. Returns the parent response ID.
func clientCompatibilityCodexSeedParent(t *testing.T, store *state.MemoryStore, parent string) string {
	t.Helper()
	id, err := state.NewResponseID()
	require.NoError(t, err)
	var items []json.RawMessage
	require.NoError(t, json.Unmarshal([]byte("["+parent+"]"), &items))
	outputs := make([]state.ItemEnvelope, 0, len(items))
	for _, raw := range items {
		outputs = append(outputs, mustEnv(t, string(raw)))
	}
	seedResponse(t, store, &state.ResponseStateRecord{
		GatewayResponseID: id,
		Owner:             state.OwnerScope{UserID: fallbackUserID, TokenID: fallbackTokenID},
		Status:            state.StatusCompleted,
		InputItems:        []state.ItemEnvelope{mustEnv(t, `{"type":"message","role":"user","content":"lookup both namespaces"}`)},
		OutputItems:       outputs,
	})
	return id
}

// TestClientCompatibilityCodexFallbackNamespaceHistory proves same-leaf calls
// in different namespaces, sent directly or hydrated from a stored parent,
// reach the Chat fallback as distinct flattened calls (alpha__lookup and
// beta__lookup) paired with their tool results and settle token billing, while
// an unresolvable parent still fails before any provider, state, quota or
// ledger effect.
func TestClientCompatibilityCodexFallbackNamespaceHistory(t *testing.T) {
	const balance = int64(1_000_000)
	cases := []struct {
		name, input, parent string
		missingParent       bool
	}{
		{name: "direct", input: clientCompatibilityCodexCalls + "," + clientCompatibilityCodexOutputs},
		{name: "missing_parent", input: clientCompatibilityCodexCalls + "," + clientCompatibilityCodexOutputs, missingParent: true},
		{name: "hydrated", input: clientCompatibilityCodexOutputs, parent: clientCompatibilityCodexCalls},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			xaiVideoSetup(t, balance, false)
			store := enableStateForTest(t)
			provider, observed, calls := responseNamespaceFallbackProvider(t, channeltype.OpenAICompatible)
			selector := ""
			if tc.missingParent {
				selector = `,"previous_response_id":"resp_ffffffffffffffffffffffffffffffff"`
			}
			if tc.parent != "" {
				selector = fmt.Sprintf(`,"previous_response_id":%q`, clientCompatibilityCodexSeedParent(t, store, tc.parent))
			}
			payload := `{"model":"alias","max_output_tokens":32,"input":[` + tc.input + `]` + selector + `}`
			c, recorder, requestID := protocolContext(t, channeltype.OpenAICompatible, "codex-fidelity-fixture", "/v1/responses", payload, provider.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2})
			clientCompatibilityCodexCleanLedger(t, requestID)
			require.False(t, supportsNativeResponseAPI(metalib.GetByContext(c)))
			apiErr := RelayResponseAPIHelper(c)
			drainCriticalTasks(t)
			if tc.missingParent {
				require.NotNil(t, apiErr)
				require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
				require.Equal(t, "previous_response_not_found", apiErr.Code)
				clientCompatibilityCodexNoSideEffects(t, c, requestID, calls.Load(), balance)
				return
			}
			require.Nil(t, apiErr, "namespaced history is representable on fallback")
			require.Equal(t, http.StatusOK, recorder.Code)
			require.EqualValues(t, 1, calls.Load())
			got := <-observed
			require.NoError(t, got.Err)
			require.Equal(t, "/v1/chat/completions", got.Path)
			wire := decodeFallbackWire(t, channeltype.OpenAICompatible, got.Body)
			require.Empty(t, wire.Tools, "history alone declares no tools")
			want := []fallbackWireTurn{
				{Role: "assistant", Calls: []fallbackWireCall{
					{ID: "alpha", Name: "alpha__lookup", Arguments: `{}`},
					{ID: "beta", Name: "beta__lookup", Arguments: `{}`},
				}},
				{Role: "tool", Results: []fallbackWireResult{{ID: "alpha", Content: "alpha-result"}}},
				{Role: "tool", Results: []fallbackWireResult{{ID: "beta", Content: "beta-result"}}},
			}
			if tc.parent != "" {
				want = append([]fallbackWireTurn{{Role: "user", Text: "lookup both namespaces"}}, want...)
			}
			requireFallbackWireTurns(t, want, wire.Turns)
			requireFallbackTokenSettlement(t, c, requestID, balance, got.User, got.Token)
		})
	}
}

// TestClientCompatibilityCodexFallbackAdmission traverses direct and hydrated
// admission of provider-bound or unknown state that has no faithful fallback
// representation, and proves it fails before any paid call or state effect.
func TestClientCompatibilityCodexFallbackAdmission(t *testing.T) {
	const balance = int64(1_000_000)
	cases := []struct {
		name, input, parent string
		notPortable         bool
	}{
		{name: "compaction_direct", input: `{"type":"compaction","id":"cmp_fixture","encrypted_content":"opaque-compaction-canary"},{"type":"message","role":"user","content":"continue"}`, notPortable: true},
		{name: "compaction_hydrated", input: `{"type":"message","role":"user","content":"continue"}`, parent: `{"type":"compaction","id":"cmp_fixture","encrypted_content":"opaque-compaction-canary"}`, notPortable: true},
		{name: "unknown_canonical_root", input: `{"type":"future_provider_state","id":"opaque_fixture","payload":"synthetic-state"},{"type":"message","role":"user","content":"continue"}`, notPortable: true},
		{name: "unknown_content_beside_text", input: `{"type":"message","role":"user","content":[{"type":"input_text","text":"inspect"},{"type":"future_binary","text":"not-a-text-part","opaque_data":"opaque-canary"}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			xaiVideoSetup(t, balance, false)
			store := enableStateForTest(t)
			var calls atomic.Int32
			provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"chatcmpl_fixture","object":"chat.completion","model":"codex-fidelity-fixture","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`)
			}))
			t.Cleanup(provider.Close)
			previous := client.HTTPClient
			client.HTTPClient = provider.Client()
			t.Cleanup(func() { client.HTTPClient = previous })
			selector := ""
			if tc.parent != "" {
				selector = fmt.Sprintf(`,"previous_response_id":%q`, clientCompatibilityCodexSeedParent(t, store, tc.parent))
			}
			payload := `{"model":"alias","max_output_tokens":32,"input":[` + tc.input + `]` + selector + `}`
			c, _, requestID := protocolContext(t, channeltype.OpenAICompatible, "codex-fidelity-fixture", "/v1/responses", payload, provider.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2})
			clientCompatibilityCodexCleanLedger(t, requestID)
			require.False(t, supportsNativeResponseAPI(metalib.GetByContext(c)))
			apiErr := RelayResponseAPIHelper(c)
			drainCriticalTasks(t)
			require.NotNil(t, apiErr, "lossy fallback must fail explicitly")
			if tc.notPortable {
				require.Equal(t, http.StatusConflict, apiErr.StatusCode)
				require.Equal(t, "state_not_portable", apiErr.Code)
			} else {
				require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
				require.Equal(t, "response_fallback_not_supported", apiErr.Code)
			}
			clientCompatibilityCodexNoSideEffects(t, c, requestID, calls.Load(), balance)
		})
	}
}

// TestClientCompatibilityCodexNativeWire preserves provider-owned history and strict flags through actual HTTP.
func TestClientCompatibilityCodexNativeWire(t *testing.T) {
	const balance = int64(1_000_000)
	const response = `{"id":"resp_codex_fixture","object":"response","status":"completed","model":"codex-fidelity-fixture","output":[{"type":"message","id":"msg_fixture","role":"assistant","content":[{"type":"output_text","text":"done"}]}],"usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18}}`
	cases := map[string]string{
		"namespace_history":     `{"model":"alias","max_output_tokens":32,"input":[` + clientCompatibilityCodexCalls + "," + clientCompatibilityCodexOutputs + `]}`,
		"opaque_state":          `{"model":"alias","max_output_tokens":32,"input":[{"type":"compaction","id":"cmp_fixture","encrypted_content":"opaque-compaction-canary"},{"type":"reasoning","id":"rs_fixture","encrypted_content":"opaque-reasoning-canary","summary":[]},{"type":"message","role":"user","content":"continue"}]}`,
		"file_and_strict_false": `{"model":"alias","max_output_tokens":32,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"inspect"},{"type":"input_file","file_id":"file_fixture","filename":"report.pdf"}]}],"text":{"format":{"type":"json_schema","name":"answer","schema":{"type":"object","properties":{"answer":{"type":"string"}},"additionalProperties":false},"strict":false}}}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			xaiVideoSetup(t, balance, false)
			type observation struct {
				path        string
				input, text json.RawMessage
				err         error
			}
			observed := make(chan observation, 1)
			var calls atomic.Int32
			provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var wire struct {
					Input json.RawMessage `json:"input"`
					Text  json.RawMessage `json:"text"`
				}
				err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&wire)
				observed <- observation{r.URL.Path, wire.Input, wire.Text, err}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, response)
			}))
			t.Cleanup(provider.Close)
			previous := client.HTTPClient
			client.HTTPClient = provider.Client()
			t.Cleanup(func() { client.HTTPClient = previous })
			c, recorder, requestID := protocolContext(t, channeltype.OpenAICompatible, "codex-fidelity-fixture", "/v1/responses", payload, provider.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2})
			c.Set(ctxkey.Config, model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse})
			clientCompatibilityCodexCleanLedger(t, requestID)
			require.True(t, supportsNativeResponseAPI(metalib.GetByContext(c)))
			apiErr := RelayResponseAPIHelper(c)
			drainCriticalTasks(t)
			require.Nil(t, apiErr)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.EqualValues(t, 1, calls.Load())
			got := <-observed
			require.NoError(t, got.err)
			require.Equal(t, "/v1/responses", got.path)
			var want struct {
				Input json.RawMessage `json:"input"`
				Text  json.RawMessage `json:"text"`
			}
			require.NoError(t, json.Unmarshal([]byte(payload), &want))
			require.JSONEq(t, string(want.Input), string(got.input), "preserve namespace identities, raw opaque state and files")
			if len(want.Text) > 0 {
				require.JSONEq(t, string(want.Text), string(got.text))
			}
			require.False(t, metalib.GetByContext(c).ResponseAPIFallback)
			require.EqualValues(t, 25, requestCostQuota(t, requestID))
			require.EqualValues(t, 25, consumeLogQuota(t, requestID))
			require.Equal(t, balance-25, reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, balance-25, token.RemainQuota)
			require.EqualValues(t, 25, token.UsedQuota)
		})
	}
}
