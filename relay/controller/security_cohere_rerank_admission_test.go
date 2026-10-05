package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/cohere"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// cohereAdmissionContext constructs a real routed request without sharing Gin state.
func cohereAdmissionContext(ch *model.Channel, target, id, name string, count int, cap *int, group float64) *gin.Context {
	docs := make([]string, count)
	for i := range docs {
		docs[i] = "synthetic document"
	}
	top := 1
	req := relaymodel.RerankRequest{Model: name, Query: "synthetic query", Documents: docs, TopN: &top, MaxTokensPerDoc: cap}
	body, err := json.Marshal(req)
	if err != nil {
		panic(err)
	}
	c := securityImageContext(ch, name, target, id, 1, group)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/rerank", strings.NewReader(string(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	meta := metalib.GetByContext(c)
	meta.Mode, meta.RequestURLPath = relaymode.Rerank, "/v1/rerank"
	metalib.Set2Context(c, meta)
	return c
}

// TestSecurityCohereRerankAggregateAdmission checks the actual provider-received
// truncation contract, outstanding reservation, and all final accounting records.
// Synthetic receipts test gateway policy, not measured provider invoice amounts.
func TestSecurityCohereRerankAggregateAdmission(t *testing.T) {
	for _, tc := range []struct {
		name, model, receipt, tariff       string
		docs, cap                          int
		owner, token, quote, charge        int64
		group                              float64
		reject, invalid, unlimited, mapped bool
	}{
		{name: "single_search_control", docs: 3, cap: 4096, owner: 20000, token: 20000, quote: 1000, charge: 1000, group: 1, receipt: `1`},
		{name: "owner_budget_reject", docs: 100, owner: 12999, token: 50000, quote: 13000, group: 1, reject: true},
		{name: "finite_token_reject", docs: 100, owner: 50000, token: 12999, quote: 13000, group: 1, reject: true},
		{name: "funded_receipt_refund", docs: 100, owner: 50000, token: 50000, quote: 13000, charge: 3000, group: 1, receipt: `3`},
		{name: "missing_receipt_retains_bound", docs: 100, owner: 50000, token: 50000, quote: 13000, charge: 13000, group: 1},
		{name: "invalid_receipt_retains_bound", docs: 100, owner: 50000, token: 50000, quote: 13000, charge: 13000, group: 1, receipt: `-1`},
		{name: "group_multiplier", docs: 100, owner: 100000, token: 100000, quote: 26000, charge: 6000, group: 2, receipt: `3`},
		{name: "unlimited_token_owner_hold", docs: 100, owner: 50000, token: 0, quote: 13000, charge: 3000, group: 1, receipt: `3`, unlimited: true},
		{name: "free_group_control", docs: 100, cap: 4096, quote: 0, charge: 0, group: 0, receipt: `3`},
		{name: "free_operator_control", tariff: `{"ratio":0,"per_call":{"usd_per_thousand_calls":0}}`, docs: 100, cap: 4096, group: 1, receipt: `3`},
		{name: "custom_context_control", model: "rerank-private", tariff: `{"ratio":1000,"per_call":{"usd_per_thousand_calls":2},"context_length":32768}`, docs: 100, owner: 50000, token: 50000, quote: 41000, charge: 3000, group: 1, receipt: `3`},
		{name: "custom_context_absent", model: "rerank-private", docs: 100, owner: 50000, token: 50000, group: 1, reject: true, invalid: true},
		{name: "known_context_cannot_shrink", tariff: `{"ratio":1000,"per_call":{"usd_per_thousand_calls":2},"context_length":2}`, docs: 100, owner: 50000, token: 50000, quote: 13000, charge: 3000, group: 1, receipt: `3`},
		{name: "larger_document_cap_reject", docs: 100, cap: 8192, owner: 20000, token: 20000, quote: 21000, group: 1, reject: true},
		{name: "small_cap_boundary", docs: 100, cap: 448, owner: 20000, token: 20000, quote: 5000, charge: 1000, group: 1, receipt: `1`},
		{name: "next_chunk_boundary", docs: 100, cap: 449, owner: 20000, token: 20000, quote: 6000, charge: 1000, group: 1, receipt: `1`},
		{name: "v4_query_bound", model: "rerank-v4.0-fast", docs: 100, owner: 50000, token: 50000, quote: 41000, charge: 3000, group: 1, receipt: `3`},
		{name: "mapped_model_budget", docs: 100, owner: 12999, token: 50000, quote: 13000, group: 1, reject: true, mapped: true},
		{name: "zero_cap_reject", docs: 1, cap: -1, owner: 50000, token: 50000, group: 1, reject: true, invalid: true},
		{name: "overflow_cap_reject", docs: 10000, cap: math.MaxInt, owner: 50000, token: 50000, group: 1, reject: true, invalid: true},
		{name: "too_many_documents_reject", docs: 10001, owner: 2000000, token: 2000000, group: 1, reject: true, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			securityImageAccount(t, tc.owner, tc.token, tc.unlimited)
			name := tc.model
			if name == "" {
				name = "rerank-v3.5"
			}
			tariff := tc.tariff
			if tariff == "" {
				tariff = `{"ratio":1000,"per_call":{"usd_per_thousand_calls":2}}`
			}
			ch := securityImageChannel(t, channeltype.Cohere, name, tariff)
			cfg, ok := pricing.ResolveModelConfig(name, ch.GetModelPriceConfigs(), &cohere.Adaptor{}, time.Now())
			require.True(t, ok)
			require.NotNil(t, cfg.PerCall)
			if tc.name == "free_operator_control" {
				require.Zero(t, cfg.Ratio)
				require.Zero(t, cfg.PerCall.UsdPerThousandCalls)
			} else {
				require.Equal(t, float64(1000), cfg.Ratio)
			}
			var calls atomic.Int32
			var heldOwner, heldToken atomic.Int64
			var sentCap atomic.Int64
			var bad atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var u model.User
				var key model.Token
				if model.DB.First(&u, 1).Error != nil || model.DB.First(&key, 1).Error != nil {
					bad.Store(true)
				}
				heldOwner.Store(u.Quota)
				heldToken.Store(key.RemainQuota)
				var outbound cohere.RerankRequest
				if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&outbound) != nil || len(outbound.Documents) != tc.docs || outbound.Model != name || outbound.TopN == nil || *outbound.TopN != 1 {
					bad.Store(true)
				}
				if outbound.MaxTokensPerDoc != nil {
					sentCap.Store(int64(*outbound.MaxTokensPerDoc))
				}
				receipt := ""
				if tc.receipt != "" {
					receipt = `,"billed_units":{"search_units":` + tc.receipt + `}`
				}
				w.Header().Set("Content-Type", "application/json")
				if _, err := fmt.Fprint(w, `{"results":[],"meta":{"tokens":{"input_tokens":11}`+receipt+`}}`); err != nil {
					bad.Store(true)
				}
			}))
			securityImageClient(t, server)
			var cap *int
			if tc.cap != 0 {
				value := tc.cap
				if value == -1 {
					value = 0
				}
				cap = &value
			}
			incoming := name
			if tc.mapped {
				incoming = "tenant-rerank"
			}
			id := "cohere-admission-" + tc.name
			c := cohereAdmissionContext(ch, server.URL, id, incoming, tc.docs, cap, tc.group)
			if tc.mapped {
				meta := metalib.GetByContext(c)
				meta.ModelMapping = map[string]string{incoming: name}
				metalib.Set2Context(c, meta)
			}
			apiErr := RelayRerankHelper(c)
			drainCriticalTasks(t)
			var u model.User
			var key model.Token
			var costs []model.UserRequestCost
			var logs []model.Log
			require.NoError(t, model.DB.First(&u, 1).Error)
			require.NoError(t, model.DB.First(&key, 1).Error)
			require.NoError(t, model.DB.Where("request_id = ?", id).Find(&costs).Error)
			require.NoError(t, model.LOG_DB.Where("request_id = ? AND type IN ?", id, []int{model.LogTypeConsume, model.LogTypeProvisional}).Find(&logs).Error)
			t.Logf("calls=%d held_owner=%d held_token=%d sent_cap=%d final_owner=%d final_token=%d cost=%v log=%v", calls.Load(), heldOwner.Load(), heldToken.Load(), sentCap.Load(), u.Quota, key.RemainQuota, costs, logs)
			require.False(t, bad.Load())
			if tc.reject {
				require.Zero(t, calls.Load(), "aggregate quote must be funded before provider dispatch")
				require.NotNil(t, apiErr)
				status := http.StatusForbidden
				if tc.invalid {
					status = http.StatusBadRequest
				}
				require.Equal(t, status, apiErr.StatusCode)
				require.Equal(t, tc.owner, u.Quota)
				require.Equal(t, tc.token, key.RemainQuota)
				require.Empty(t, costs)
				require.Empty(t, logs)
				return
			}
			require.Nil(t, apiErr)
			require.EqualValues(t, 1, calls.Load())
			require.Equal(t, tc.owner-tc.quote, heldOwner.Load())
			expectedToken := tc.token - tc.quote
			if tc.unlimited {
				expectedToken = tc.token
			}
			require.Equal(t, expectedToken, heldToken.Load())
			expectedCap := tc.cap
			if expectedCap == 0 {
				expectedCap = 4096
			}
			require.EqualValues(t, expectedCap, sentCap.Load(), "the reserved cap must be present in the outgoing body")
			finalToken := tc.token - tc.charge
			if tc.unlimited {
				finalToken = tc.token
			}
			securityImageLedger(t, id, tc.owner-tc.charge, finalToken, tc.charge)
			require.True(t, c.GetBool(ctxkey.BillingReconciled))
		})
	}
}

// TestSecurityCohereRerankConcurrentBudget freezes provider responses while four
// calls compete for two prepaid aggregate quotes on shared physical SQL balances.
func TestSecurityCohereRerankConcurrentBudget(t *testing.T) {
	securityImageAccount(t, 26001, 26001, false)
	ch := securityImageChannel(t, channeltype.Cohere, "rerank-v3.5", `{"ratio":1000,"per_call":{"usd_per_thousand_calls":2}}`)
	decisions := make(chan struct{}, 4)
	release := make(chan struct{})
	var calls, rejections atomic.Int32
	var bad atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		decisions <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprint(w, `{"results":[],"meta":{"tokens":{"input_tokens":11}}}`); err != nil {
			bad.Store(true)
		}
	}))
	securityImageClient(t, server)
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := cohereAdmissionContext(ch, server.URL, fmt.Sprintf("cohere-concurrent-%d", i), "rerank-v3.5", 100, nil, 1)
			ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
			if apiErr := RelayRerankHelper(c); apiErr != nil {
				rejections.Add(1)
				if apiErr.StatusCode != http.StatusForbidden {
					bad.Store(true)
				}
				decisions <- struct{}{}
			}
		}(i)
	}
	allDecided := true
	for i := 0; i < 4; i++ {
		select {
		case <-decisions:
		case <-time.After(5 * time.Second):
			allDecided = false
		}
	}
	var u model.User
	var key model.Token
	ownerErr, tokenErr := model.DB.First(&u, 1).Error, model.DB.First(&key, 1).Error
	unblock()
	wg.Wait()
	drainCriticalTasks(t)
	t.Logf("provider_calls=%d rejected=%d outstanding_owner=%d outstanding_token=%d", calls.Load(), rejections.Load(), u.Quota, key.RemainQuota)
	require.True(t, allDecided)
	require.NoError(t, ownerErr)
	require.NoError(t, tokenErr)
	require.False(t, bad.Load())
	require.EqualValues(t, 2, calls.Load())
	require.EqualValues(t, 2, rejections.Load())
	require.EqualValues(t, 1, u.Quota)
	require.EqualValues(t, 1, key.RemainQuota)
	require.NoError(t, model.DB.First(&u, 1).Error)
	require.NoError(t, model.DB.First(&key, 1).Error)
	require.EqualValues(t, 1, u.Quota)
	require.EqualValues(t, 1, key.RemainQuota)
	var costs []model.UserRequestCost
	var logs []model.Log
	require.NoError(t, model.DB.Where("request_id LIKE ?", "cohere-concurrent-%").Find(&costs).Error)
	require.NoError(t, model.LOG_DB.Where("request_id LIKE ? AND type = ?", "cohere-concurrent-%", model.LogTypeConsume).Find(&logs).Error)
	require.Len(t, costs, 2)
	require.Len(t, logs, 2)
	for _, cost := range costs {
		require.EqualValues(t, 13000, cost.Quota)
	}
	for _, log := range logs {
		require.EqualValues(t, 13000, log.Quota)
	}
}
