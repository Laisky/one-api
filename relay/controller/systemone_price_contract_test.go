package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

// TestSystemOnePriceContractBehavior distinguishes an inherited catalog price
// from a genuinely free group using real HTTP and SQLite accounting. A zero
// channel ModelConfig ratio inherits; it does not explicitly request free input.
// Parameters: t is the test handle. Returns: none.
func TestSystemOnePriceContractBehavior(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		direct, unlimited       bool
		price, group            float64
		wantQuota, wantReserved int64
	}{
		{name: "zero_channel_mapped_inherits", price: 0, group: 1, wantQuota: 7, wantReserved: 1377},
		{name: "zero_channel_direct_inherits", direct: true, price: 0, group: 1, wantQuota: 7, wantReserved: 1377},
		{name: "zero_channel_unlimited_inherits", unlimited: true, price: 0, group: 1, wantQuota: 7, wantReserved: 1377},
		{name: "missing_override_uses_provider_price", price: -1, group: 1, wantQuota: 7, wantReserved: 1377},
		{name: "positive_override_still_bills", price: 1, group: 1, wantQuota: 312, wantReserved: 65536},
		{name: "free_group_with_catalog_price", price: -1, group: 0},
		{name: "free_group_with_zero_channel", price: 0, group: 0},
		{name: "free_group_with_unlimited_token", unlimited: true, price: 0, group: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(100000000)
			var calls atomic.Int32
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if _, err := io.WriteString(w, systemOneSuccess); err != nil {
					t.Errorf("write local upstream response: %v", err)
				}
			}))
			t.Cleanup(upstream.Close)
			previous := client.HTTPClient
			client.HTTPClient = upstream.Client()
			t.Cleanup(func() { client.HTTPClient = previous })
			c, writer, requestID := systemOneContext(t, upstream.URL, balance, tc.unlimited, tc.group, tc.price)
			if tc.direct {
				c.Request.Body = io.NopCloser(strings.NewReader(strings.Replace(systemOneRequest, `"alias"`, `"jev-latest"`, 1)))
			}
			RelaySystemOne(c)
			drainCriticalTasks(t)
			require.Equal(t, http.StatusOK, writer.Code, writer.Body.String())
			require.EqualValues(t, 1, calls.Load())
			t.Logf("reserved=%d final_charge=%d", c.GetInt64(ctxkey.PreConsumedQuotaAmount), balance-reloadUserQuota(t))
			require.Equal(t, tc.wantReserved, c.GetInt64(ctxkey.PreConsumedQuotaAmount))
			require.Equal(t, balance-tc.wantQuota, reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			tokenCharge := tc.wantQuota
			if tc.unlimited {
				// Unlimited tokens bypass their own limit, not the user's balance.
				tokenCharge = 0
			}
			require.Equal(t, balance-tokenCharge, token.RemainQuota)
			require.Equal(t, tokenCharge, token.UsedQuota)
			require.Equal(t, tc.wantQuota, requestCostQuota(t, requestID))
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1)
			require.EqualValues(t, tc.wantQuota, logs[0].Quota)
		})
	}
}
