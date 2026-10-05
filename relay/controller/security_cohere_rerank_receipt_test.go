package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/cohere"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/stretchr/testify/require"
)

// TestSecurityCohereRerankReceiptLedger exercises real HTTP adaptation and all
// durable accounting records with synthetic search-unit receipts. It tests
// settlement, not a claim that the existing admission estimate bounds long jobs.
func TestSecurityCohereRerankReceiptLedger(t *testing.T) {
	for _, tc := range []struct {
		name, units, reason, tokens      string
		group                            float64
		charge, balance                  int64
		unlimited, truncate, decodeError bool
	}{
		{name: "one_search_control", units: `1`, group: 1, charge: 1000, balance: 10000},
		{name: "three_searches", units: `3`, group: 1, charge: 3000, balance: 10000},
		{name: "decimal_units", units: `3.0`, group: 1, charge: 3000, balance: 10000},
		{name: "scientific_units", units: `3e0`, group: 1, charge: 3000, balance: 10000},
		{name: "decimal_tokens", units: `3`, tokens: `{"input_tokens":11.0,"output_tokens":0}`, group: 1, charge: 3000, balance: 10000},
		{name: "unusable_tokens", units: `3`, tokens: `{"input_tokens":"unusable"}`, decodeError: true, group: 1, charge: 3000, balance: 10000},
		{name: "absent_tokens", units: `3`, tokens: `null`, group: 1, charge: 3000, balance: 10000},
		{name: "group_multiplier", units: `3`, group: 2, charge: 6000, balance: 10000},
		{name: "unlimited_token", units: `3`, group: 1, charge: 3000, balance: 10000, unlimited: true},
		{name: "measured_debt", units: `3`, group: 1, charge: 3000, balance: 2000},
		{name: "free_group_control", units: `3`, group: 0, charge: 0, balance: 0},
		{name: "missing_units", group: 1, charge: 1000, balance: 10000, reason: "cohere_search_units_missing"},
		{name: "zero_units", units: `0`, group: 1, charge: 1000, balance: 10000, reason: "cohere_search_units_invalid"},
		{name: "negative_units", units: `-2`, group: 1, charge: 1000, balance: 10000, reason: "cohere_search_units_invalid"},
		{name: "fractional_units", units: `1.5`, group: 1, charge: 1000, balance: 10000, reason: "cohere_search_units_invalid"},
		{name: "overflowing_units", units: `9223372036854775808`, group: 1, charge: 1000, balance: 10000, reason: "cohere_search_units_invalid"},
		{name: "cost_overflow", units: `9223372036854775807`, group: 1, charge: 1000, balance: 10000, reason: "cohere_search_units_cost_overflow"},
		{name: "accepted_transport_failure", units: `3`, group: 1, charge: 1000, balance: 10000, reason: "cohere_rerank_response_incomplete", truncate: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			securityImageAccount(t, tc.balance, tc.balance, tc.unlimited)
			ch := securityImageChannel(t, channeltype.Cohere, "rerank-v3.5", `{"ratio":1000,"per_call":{"usd_per_thousand_calls":2}}`)
			cfg, ok := pricing.ResolveModelConfig("rerank-v3.5", ch.GetModelPriceConfigs(), &cohere.Adaptor{}, time.Now())
			require.True(t, ok)
			require.NotNil(t, cfg.PerCall, "fixture must select the actual search tariff")
			require.Equal(t, float64(1000), cfg.Ratio)
			var calls atomic.Int32
			var invalidRequest atomic.Bool
			var heldOwner, heldToken atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var user model.User
				var token model.Token
				if model.DB.First(&user, 1).Error != nil || model.DB.First(&token, 1).Error != nil {
					invalidRequest.Store(true)
				}
				heldOwner.Store(user.Quota)
				heldToken.Store(token.RemainQuota)
				var payload struct {
					Model     string   `json:"model"`
					Query     string   `json:"query"`
					Documents []string `json:"documents"`
				}
				if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&payload) != nil ||
					payload.Model != "rerank-v3.5" || payload.Query != "synthetic query" ||
					len(payload.Documents) != 3 || r.URL.Path != "/v2/rerank" {
					invalidRequest.Store(true)
				}
				billed := ""
				if tc.units != "" {
					billed = `,"billed_units":{"search_units":` + tc.units + `}`
				}
				tokens := tc.tokens
				if tokens == "" {
					tokens = `{"input_tokens":11,"output_tokens":0}`
				}
				body := `{"id":"synthetic-rerank","results":[{"index":0,"relevance_score":0.9}],"meta":{"tokens":` + tokens + billed + `}}`
				w.Header().Set("Content-Type", "application/json")
				if tc.truncate {
					w.Header().Set("Content-Length", "9999")
					body = `{"id":"synthetic-rerank","results":[`
				}
				if _, err := fmt.Fprint(w, body); err != nil {
					invalidRequest.Store(true)
				}
			}))
			securityImageClient(t, server)
			id := "cohere-units-" + tc.name
			c := securityImageContext(ch, "rerank-v3.5", server.URL, id, 1, tc.group)
			c.Set(ctxkey.Channel, channeltype.Cohere)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/rerank", strings.NewReader(
				`{"model":"rerank-v3.5","query":"synthetic query","documents":["first","second","third"]}`))
			c.Request.Header.Set("Content-Type", "application/json")
			meta := metalib.GetByContext(c)
			meta.Mode = relaymode.Rerank
			meta.RequestURLPath = "/v1/rerank"
			metalib.Set2Context(c, meta)
			apiErr := RelayRerankHelper(c)
			drainCriticalTasks(t)

			// Collect physical balances, request cost, and both log types before
			// asserting the price so a failure retains evidence from every ledger.
			var user model.User
			var token model.Token
			var cost model.UserRequestCost
			var logs []model.Log
			require.NoError(t, model.DB.First(&user, 1).Error)
			require.NoError(t, model.DB.First(&token, 1).Error)
			require.NoError(t, model.DB.Where("request_id = ?", id).First(&cost).Error)
			require.NoError(t, model.LOG_DB.Where("request_id = ? AND type IN ?", id,
				[]int{model.LogTypeConsume, model.LogTypeProvisional}).Find(&logs).Error)
			metadata := ""
			logQuota := int64(-1)
			if len(logs) == 1 {
				encoded, err := json.Marshal(logs[0].Metadata)
				require.NoError(t, err)
				metadata = string(encoded)
				logQuota = int64(logs[0].Quota)
			}
			t.Logf("receipt=%s calls=%d owner=%d token=%d cost=%d log=%d metadata=%s",
				tc.name, calls.Load(), user.Quota, token.RemainQuota, cost.Quota, logQuota, metadata)
			require.EqualValues(t, 1, calls.Load())
			require.False(t, invalidRequest.Load())
			if tc.truncate || tc.decodeError {
				require.NotNil(t, apiErr, "real upstream transport failure must not be hidden")
			} else {
				require.Nil(t, apiErr)
			}
			wantToken := tc.balance - tc.charge
			if tc.unlimited {
				wantToken = tc.balance
			}
			require.Equal(t, tc.balance-tc.charge, user.Quota, "charge authoritative search units, not one HTTP call")
			require.Equal(t, wantToken, token.RemainQuota)
			require.Equal(t, tc.charge, cost.Quota)
			require.Len(t, logs, 1, "one final settlement replaces its provisional log")
			require.Equal(t, model.LogTypeConsume, logs[0].Type)
			require.Equal(t, tc.charge, logQuota)
			require.True(t, c.GetBool(ctxkey.BillingReconciled))
			if tc.reason != "" {
				require.Contains(t, metadata, tc.reason)
			}
			// Admission must still physically reserve the existing one-call floor;
			// aggregate document/chunk budgeting is outside this settlement matrix.
			floor := int64(1000 * tc.group)
			require.Equal(t, tc.balance-floor, heldOwner.Load())
			if !tc.unlimited {
				require.Equal(t, tc.balance-floor, heldToken.Load())
			}
		})
	}
}

// TestSecurityCohereRerankProviderErrors pins how the aggregate allowance settles
// when Cohere answers with an HTTP error instead of a receipt. Explicit admission
// rejections release the complete hold and keep retry available; other statuses
// follow the existing uncertain-execution policy and retain the quoted allowance
// exactly once with estimate provenance. No status creates measured usage.
func TestSecurityCohereRerankProviderErrors(t *testing.T) {
	for _, tc := range []struct {
		status    int
		charge    int64
		retryable bool
	}{
		{status: http.StatusUnauthorized, retryable: true},
		{status: http.StatusTooManyRequests, retryable: true},
		{status: http.StatusBadRequest, charge: 13000},
		{status: http.StatusInternalServerError, charge: 13000},
	} {
		t.Run(fmt.Sprintf("status_%d", tc.status), func(t *testing.T) {
			securityImageAccount(t, 50000, 50000, false)
			ch := securityImageChannel(t, channeltype.Cohere, "rerank-v3.5", `{"ratio":1000,"per_call":{"usd_per_thousand_calls":2}}`)
			var calls atomic.Int32
			var heldOwner, heldToken atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var user model.User
				var token model.Token
				if model.DB.First(&user, 1).Error == nil && model.DB.First(&token, 1).Error == nil {
					heldOwner.Store(user.Quota)
					heldToken.Store(token.RemainQuota)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, `{"message":"synthetic provider error"}`)
			}))
			securityImageClient(t, server)
			id := fmt.Sprintf("cohere-provider-error-%d", tc.status)
			c := cohereAdmissionContext(ch, server.URL, id, "rerank-v3.5", 100, nil, 1)
			c.Set(ctxkey.Channel, channeltype.Cohere)
			apiErr := RelayRerankHelper(c)
			drainCriticalTasks(t)

			require.NotNil(t, apiErr, "a provider error must not be reported as success")
			require.EqualValues(t, 1, calls.Load())
			require.EqualValues(t, 50000-13000, heldOwner.Load(), "the aggregate allowance is reserved before dispatch")
			require.EqualValues(t, 50000-13000, heldToken.Load())
			securityImageLedger(t, id, 50000-tc.charge, 50000-tc.charge, tc.charge)
			var entry model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ?", id).First(&entry).Error)
			encoded, err := json.Marshal(entry.Metadata)
			require.NoError(t, err)
			if tc.charge > 0 {
				require.Contains(t, string(encoded), "uncertain_upstream_admission_upstream_http_error")
			} else {
				require.NotContains(t, string(encoded), "billing_estimated")
			}
			require.Equal(t, tc.retryable, BillingAllowsRetry(c), "only a released hold may be retried")
		})
	}
}
