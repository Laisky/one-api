package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestSecurityImageQuoteRefundHTTP distinguishes a token-only prepayment from
// an earned render fee and verifies the free-group contract after real usage.
// Expected amounts are literal internal quota units, not live provider invoices.
func TestSecurityImageQuoteRefundHTTP(t *testing.T) {
	const name = "gpt-image-1-mini"
	const receipt = `{"input_tokens":0,"output_tokens":1,"total_tokens":1,"input_tokens_details":{"text_tokens":0,"image_tokens":0}}`
	for _, unlimited := range []bool{false, true} {
		for _, tc := range []struct {
			name, usage string
			group       float64
			hold, cost  int64
		}{
			{"measured_below_quote", receipt, 1, 100, 4},
			{"missing_receipt_control", "null", 1, 100, 100},
			{"free_group_measured", receipt, 0, 0, 0},
		} {
			t.Run(fmt.Sprintf("%s/unlimited=%v", tc.name, unlimited), func(t *testing.T) {
				const initial int64 = 1000
				securityImageAccount(t, initial, initial, unlimited)
				// A nonzero prompt tariff is an existing complete operator image
				// override; it must not inherit a provider per-render fee.
				ch := securityImageChannel(t, channeltype.OpenAI, name, `{"ratio":100,"image":{"prompt_ratio":1}}`)
				cfg, ok := pricing.ResolveImagePricing(name, ch.GetModelPriceConfigs(), &openai.Adaptor{}, time.Now())
				require.True(t, ok)
				require.NotNil(t, cfg)
				require.Zero(t, cfg.PricePerImageUsd, "fixture must select token-only rather than additive render billing")
				var calls atomic.Int32
				var seenOwner, seenToken atomic.Int64
				var bad atomic.Bool
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					var user model.User
					var token model.Token
					if model.DB.First(&user, 1).Error != nil || model.DB.First(&token, 1).Error != nil || r.URL.Path != "/v1/images/generations" {
						bad.Store(true)
					}
					seenOwner.Store(user.Quota)
					seenToken.Store(token.RemainQuota)
					w.Header().Set("Content-Type", "application/json")
					if _, err := fmt.Fprintf(w, `{"created":1,"data":[{"b64_json":"AQ=="}],"usage":%s}`, tc.usage); err != nil {
						bad.Store(true)
					}
				}))
				securityImageClient(t, server)
				id := fmt.Sprintf("image-refund-%s-%v", tc.name, unlimited)
				c := securityImageContext(ch, name, server.URL, id, 1, tc.group)
				require.Nil(t, RelayImageHelper(c, relaymode.ImagesGenerations))
				require.EqualValues(t, 1, calls.Load())
				require.False(t, bad.Load())
				require.Equal(t, initial-tc.hold, seenOwner.Load(), "observe actual prepayment before the receipt exists")
				wantTokenBefore, wantTokenAfter := initial-tc.hold, initial-tc.cost
				if unlimited {
					wantTokenBefore, wantTokenAfter = initial, initial
				}
				require.Equal(t, wantTokenBefore, seenToken.Load())
				var cost model.UserRequestCost
				var user model.User
				require.NoError(t, model.DB.Where("request_id = ?", id).First(&cost).Error)
				require.NoError(t, model.DB.First(&user, 1).Error)
				t.Logf("IMAGE_REFUND_LEDGER hold=%d recorded=%d physical=%d expected=%d", tc.hold, cost.Quota, initial-user.Quota, tc.cost)
				securityImageLedger(t, id, initial-tc.cost, wantTokenAfter, tc.cost)
			})
		}
	}
}
