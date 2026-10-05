package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/billing"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/state"
)

// securityBackgroundUpstream records every request body forwarded to the fake
// native Responses provider so tests can prove what reached the wire.
type securityBackgroundUpstream struct {
	server *httptest.Server
	mu     sync.Mutex
	bodies [][]byte
}

// newSecurityBackgroundUpstream starts a fake provider that records request bodies
// and answers every call with reply. It returns the running upstream fixture.
func newSecurityBackgroundUpstream(t *testing.T, reply string) *securityBackgroundUpstream {
	t.Helper()
	upstream := &securityBackgroundUpstream{}
	upstream.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read forwarded fixture body: %v", err)
		}
		upstream.mu.Lock()
		upstream.bodies = append(upstream.bodies, body)
		upstream.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(reply)); err != nil {
			t.Errorf("write fixture reply: %v", err)
		}
	}))
	t.Cleanup(upstream.server.Close)
	return upstream
}

// forwarded returns every body the fake provider received, as strings.
func (u *securityBackgroundUpstream) forwarded() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	out := make([]string, 0, len(u.bodies))
	for _, body := range u.bodies {
		out = append(out, string(body))
	}
	return out
}

// securityLedger is the durable balance state of the fallback fixture user and token.
type securityLedger struct {
	userQuota   int64
	tokenRemain int64
	tokenUsed   int64
}

// readSecurityLedger loads the persisted user and token balances and returns them.
func readSecurityLedger(t *testing.T) securityLedger {
	t.Helper()
	var token model.Token
	require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
	return securityLedger{userQuota: fallbackUserQuota(t), tokenRemain: token.RemainQuota, tokenUsed: token.UsedQuota}
}

// runNativeResponseRelay drives RelayResponseAPIHelper for a JSON payload
// against the native OpenAI Responses route backed by upstream, waits for
// detached billing to drain, and returns the relay error.
func runNativeResponseRelay(t *testing.T, upstream *securityBackgroundUpstream, payload string) *relaymodel.ErrorWithStatusCode {
	t.Helper()
	return runNativeResponseRelayAs(t, upstream, payload, "application/json", "")
}

// runNativeResponseRelayAs is runNativeResponseRelay with an explicit request
// Content-Type and raw query string, so tests can bind the typed request from
// somewhere other than the JSON body. It returns the relay error.
func runNativeResponseRelayAs(t *testing.T, upstream *securityBackgroundUpstream, payload, contentType, rawQuery string) *relaymodel.ErrorWithStatusCode {
	t.Helper()
	c := setupResponseStateBillingContext(t, httptest.NewRecorder(), payload)
	c.Request.Header.Set("Content-Type", contentType)
	c.Request.URL.RawQuery = rawQuery
	c.Set(ctxkey.Channel, channeltype.OpenAI)
	c.Set(ctxkey.ChannelId, fallbackOpenAIChannelID)
	c.Set(ctxkey.ChannelModel, &model.Channel{Id: fallbackOpenAIChannelID, Type: channeltype.OpenAI})
	c.Set(ctxkey.BaseURL, upstream.server.URL+"/api.openai.com")
	err := RelayResponseAPIHelper(c)
	drain, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, graceful.Drain(drain))
	return err
}

// backgroundKeysOnWire lists every root key of body that case-folds to
// "background" together with its raw value, preserving duplicates.
func backgroundKeysOnWire(t *testing.T, body []byte) []string {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(body))
	opening, err := decoder.Token()
	require.NoError(t, err)
	require.Equal(t, json.Delim('{'), opening)
	var found []string
	for decoder.More() {
		keyToken, err := decoder.Token()
		require.NoError(t, err)
		key, ok := keyToken.(string)
		require.True(t, ok)
		var value json.RawMessage
		require.NoError(t, decoder.Decode(&value))
		if strings.EqualFold(key, "background") {
			found = append(found, key+"="+string(value))
		}
	}
	return found
}

// TestSecurityResponseBackgroundKeyVariants proves case-folded, Unicode-folded and
// duplicate background keys cannot smuggle a truthy background flag past the
// typed admission check to the provider or change the durable ledger.
func TestSecurityResponseBackgroundKeyVariants(t *testing.T) {
	cases := map[string]string{
		"case_variant_override":      `"background":true,"Background":false`,
		"upper_null_override":        `"background":true,"BACKGROUND":null`,
		"unicode_fold_override":      `"background":true,"bacKground":false`,
		"exact_duplicate_last_false": `"background":true,"background":false`,
		"exact_duplicate_both_true":  `"background":true,"background":true`,
		"case_variant_only":          `"Background":true`,
		"string_truthy":              `"background":"true"`,
		"duplicate_both_false":       `"background":false,"Background":false`,
	}
	for name, fields := range cases {
		for _, streaming := range []string{"false", "true"} {
			t.Run(name+"/stream="+streaming, func(t *testing.T) {
				securityAdmissionSetup(t, 1_000_000)
				before := readSecurityLedger(t)
				upstream := newSecurityBackgroundUpstream(t, `{"id":"resp_queued","object":"response","status":"queued","output":[],"usage":null}`)
				payload := `{"model":"gpt-4o-mini","input":"hello",` + fields + `,"stream":` + streaming + `}`
				err := runNativeResponseRelay(t, upstream, payload)
				for _, body := range upstream.forwarded() {
					t.Logf("provider received background keys %v", backgroundKeysOnWire(t, []byte(body)))
				}
				require.Empty(t, upstream.forwarded(), "a rejected background request must never reach the provider")
				require.Equal(t, before, readSecurityLedger(t), "a rejected background request must not touch the ledger")
				require.NotNil(t, err, "ambiguous or truthy background flags must be rejected at the boundary")
				require.Equal(t, http.StatusBadRequest, err.StatusCode)
				require.Equal(t, "background_not_supported", err.Code)
			})
		}
	}
}

// TestSecurityResponseBackgroundQueryBoundRequest proves a non-JSON Content-Type
// cannot bind a background-free typed request from the query string while the
// raw JSON body, which the native path forwards, still carries background:true.
func TestSecurityResponseBackgroundQueryBoundRequest(t *testing.T) {
	securityAdmissionSetup(t, 1_000_000)
	before := readSecurityLedger(t)
	upstream := newSecurityBackgroundUpstream(t, `{"id":"resp_queued","object":"response","status":"queued","output":[],"usage":null}`)
	err := runNativeResponseRelayAs(t, upstream, `{"model":"gpt-4o-mini","input":"hello","background":true}`, "text/plain", "Model=gpt-4o-mini&Id=pmpt_fixture")
	for _, body := range upstream.forwarded() {
		t.Logf("provider received background keys %v", backgroundKeysOnWire(t, []byte(body)))
	}
	require.Empty(t, upstream.forwarded(), "a query-bound request must not smuggle the raw background flag to the provider")
	require.Equal(t, before, readSecurityLedger(t), "a rejected background request must not touch the ledger")
	require.NotNil(t, err)
	require.Equal(t, http.StatusBadRequest, err.StatusCode)
	require.Equal(t, "background_not_supported", err.Code)
}

// TestSecurityResponseForegroundNeverCarriesBackground keeps ordinary foreground
// requests working while proving the outgoing provider body never contains any
// background key, even when the client sent an explicit false.
func TestSecurityResponseForegroundNeverCarriesBackground(t *testing.T) {
	for name, fields := range map[string]string{"omitted": ``, "explicit_false": `,"background":false`, "explicit_null": `,"background":null`} {
		t.Run(name, func(t *testing.T) {
			securityAdmissionSetup(t, 1_000_000)
			upstream := newSecurityBackgroundUpstream(t, `{"id":"resp_foreground","object":"response","status":"completed","output":[],"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}`)
			err := runNativeResponseRelay(t, upstream, `{"model":"gpt-4o-mini","input":"hello"`+fields+`}`)
			require.Nil(t, err)
			bodies := upstream.forwarded()
			require.Len(t, bodies, 1)
			require.Empty(t, backgroundKeysOnWire(t, []byte(bodies[0])))
		})
	}
}

// TestSecurityResponseNormalizedBodyStripsBackground proves the outgoing body
// builder is a last line of defence: even if admission were bypassed, no
// background key survives normalization, including on the unchanged-raw path.
func TestSecurityResponseNormalizedBodyStripsBackground(t *testing.T) {
	for name, fields := range map[string]string{
		"exact_true":                 `,"background":true`,
		"exact_duplicate_last_false": `,"background":true,"background":false`,
		"case_variant":               `,"Background":true`,
		"explicit_false":             `,"background":false`,
	} {
		t.Run(name, func(t *testing.T) {
			raw := []byte(`{"model":"gpt-4o-mini","input":"hello"` + fields + `}`)
			request := &openai.ResponseAPIRequest{Model: "gpt-4o-mini", Input: openai.ResponseAPIInput{"hello"}}
			patched, _, _, err := normalizeResponseAPIRawBody(raw, request, channeltype.OpenAI)
			require.NoError(t, err)
			require.Empty(t, backgroundKeysOnWire(t, patched), "normalized provider body: %s", patched)
		})
	}
}

// TestSecurityResponseNonTerminalReplyRetainsReservation proves an unexpected
// queued or in-progress provider reply never synthesizes prompt-only usage that
// releases the hold, and never commits a continuation binding for unfinished
// work. A completed reply without usage keeps its existing reconciliation.
func TestSecurityResponseNonTerminalReplyRetainsReservation(t *testing.T) {
	for _, tc := range []struct {
		status   string
		retained bool
	}{{"queued", true}, {"in_progress", true}, {"completed", false}} {
		t.Run(tc.status, func(t *testing.T) {
			securityAdmissionSetup(t, 1_000_000)
			store := enableStateForTest(t)
			var mu sync.Mutex
			var settlements []billing.QuotaConsumeDetail
			original := postConsumeResponseAPIQuotaDetailed
			postConsumeResponseAPIQuotaDetailed = func(detail billing.QuotaConsumeDetail) {
				mu.Lock()
				settlements = append(settlements, detail)
				mu.Unlock()
				original(detail)
			}
			t.Cleanup(func() { postConsumeResponseAPIQuotaDetailed = original })

			responseID := "resp_nonterminal_" + tc.status
			output := `[]`
			if tc.status == "completed" {
				output = `[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}]`
			}
			upstream := newSecurityBackgroundUpstream(t, fmt.Sprintf(`{"id":%q,"object":"response","status":%q,"output":%s,"usage":null}`, responseID, tc.status, output))
			before := readSecurityLedger(t)
			err := runNativeResponseRelay(t, upstream, `{"model":"gpt-4o-mini","input":"hello","max_output_tokens":4000}`)
			require.Nil(t, err)
			require.Len(t, upstream.forwarded(), 1)

			mu.Lock()
			require.Len(t, settlements, 1, "exactly one final settlement")
			settled := settlements[0]
			mu.Unlock()
			after := readSecurityLedger(t)
			require.Positive(t, settled.TotalQuota)
			require.Equal(t, settled.TotalQuota, before.userQuota-after.userQuota, "user ledger must agree with the settlement")
			require.Equal(t, settled.TotalQuota, before.tokenRemain-after.tokenRemain, "token ledger must agree with the settlement")

			_, getErr := store.GetResponse(context.Background(), state.OwnerScope{UserID: fallbackUserID, TokenID: fallbackTokenID}, responseID)
			t.Logf("status=%s settled_total=%d settled_delta=%d binding_committed=%v", tc.status, settled.TotalQuota, settled.QuotaDelta, getErr == nil)
			if tc.retained {
				require.Zero(t, settled.QuotaDelta, "a non-terminal reply must retain the full reservation")
				require.Equal(t, true, settled.Metadata["billing_estimated"])
				require.Error(t, getErr, "a non-terminal response must not become a continuation binding")
				return
			}
			require.Negative(t, settled.QuotaDelta, "control: a completed reply still reconciles below the max_output hold")
			require.NoError(t, getErr, "control: a completed response is still committed")
		})
	}
}
