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

// TestSystemOneExplicitFreeInputBehavior verifies an explicit zero price remains
// free across admission, settlement, and accounting, using real HTTP and SQLite.
// Parameters: t is the test handle. Returns: none.
func TestSystemOneExplicitFreeInputBehavior(t *testing.T) {
	for _, tc := range []struct {
		name      string
		direct    bool
		unlimited bool
		price     float64
		wantQuota int64
	}{
		{name: "free_mapped_model", price: 0},
		{name: "free_direct_model", direct: true, price: 0},
		{name: "free_unlimited_token", unlimited: true, price: 0},
		{name: "missing_override_uses_provider_price", price: -1, wantQuota: 7},
		{name: "positive_override_still_bills", price: 1, wantQuota: 312},
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
			c, writer, requestID := systemOneContext(t, upstream.URL, balance, tc.unlimited, 1, tc.price)
			if tc.direct {
				c.Request.Body = io.NopCloser(strings.NewReader(strings.Replace(systemOneRequest, `"alias"`, `"jev-latest"`, 1)))
			}
			RelaySystemOne(c)
			drainCriticalTasks(t)
			require.Equal(t, http.StatusOK, writer.Code, writer.Body.String())
			require.EqualValues(t, 1, calls.Load())
			t.Logf("reserved=%d final_charge=%d", c.GetInt64(ctxkey.PreConsumedQuotaAmount), balance-reloadUserQuota(t))
			if tc.price == 0 {
				require.Zero(t, c.GetInt64(ctxkey.PreConsumedQuotaAmount), "free calls must not reserve provider-priced quota")
			}
			require.Equal(t, balance-tc.wantQuota, reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, balance-tc.wantQuota, token.RemainQuota)
			require.Equal(t, tc.wantQuota, token.UsedQuota)
			require.Equal(t, tc.wantQuota, requestCostQuota(t, requestID))
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1)
			require.EqualValues(t, tc.wantQuota, logs[0].Quota)
		})
	}
}
