package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/stretchr/testify/require"
)

// TestSecurityMediaAdmissionHeldBeforeDispatch observes durable holds through real audio and video HTTP relays.
func TestSecurityMediaAdmissionHeldBeforeDispatch(t *testing.T) {
	for _, audio := range []bool{false, true} {
		for _, unlimited := range []bool{false, true} {
			t.Run(fmt.Sprintf("audio=%v/unlimited=%v", audio, unlimited), func(t *testing.T) {
				const balance = int64(100_000_000)
				xaiVideoSetup(t, balance, unlimited)
				seen := make(chan int64, 1)
				upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var user model.User
					if err := model.DB.First(&user, fallbackUserID).Error; err != nil {
						http.Error(w, "fixture ledger error", 500)
						return
					}
					seen <- balance - user.Quota
					if audio {
						w.Header().Set("Content-Type", "audio/wav")
						_, _ = w.Write([]byte("synthetic-audio"))
					} else {
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"request_id":"security-prepaid-video"}`)
					}
				}))
				defer upstream.Close()
				oldClient := client.HTTPClient
				client.HTTPClient = upstream.Client()
				defer func() { client.HTTPClient = oldClient }()
				var requestID string
				var held int64
				if audio {
					c, _, id := protocolContext(t, channeltype.OpenAI, "tts-1", "/v1/audio/speech", `{"model":"alias","input":"hello","voice":"alloy","response_format":"wav"}`, upstream.URL, balance, 1, unlimited, nil)
					require.Nil(t, RelayAudioHelper(c, relaymode.AudioSpeech))
					requestID, held = id, c.GetInt64(ctxkey.PreConsumedQuotaAmount)
				} else {
					c, _, id := xaiVideoContext(t, http.MethodPost, "/v1/videos", `{"model":"alias","prompt":"A paper boat.","duration":5}`, upstream.URL+"/v1", balance, 1, unlimited, nil)
					require.Nil(t, RelayVideoHelper(c))
					requestID, held = id, c.GetInt64(ctxkey.PreConsumedQuotaAmount)
				}
				drainCriticalTasks(t)
				require.Positive(t, held, "high balances still require prepaid admission")
				require.Equal(t, held, <-seen, "the owner debit must precede provider work")
				cost := requestCostQuota(t, requestID)
				require.Positive(t, cost)
				require.Equal(t, balance-cost, reloadUserQuota(t))
			})
		}
	}
}
