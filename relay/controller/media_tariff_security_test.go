package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestSecurityLyriaGenerationTariff exercises real relay admission, provider HTTP
// and SQLite settlement. Parameters: t owns assertions. Returns: none; the flat
// generation tariff must be reserved before dispatch and charged once afterward.
func TestSecurityLyriaGenerationTariff(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "messages"} {
		for _, tc := range []struct {
			name, actual    string
			balance, charge int64
			group           float64
			local           *model.ModelConfigLocal
			blocked         bool
		}{
			{name: "clip", actual: "google/lyria-3-clip-preview", balance: 100000, charge: 20000, group: 1},
			{name: "song", actual: "google/lyria-3-pro-preview", balance: 100000, charge: 40000, group: 1},
			{name: "insufficient", actual: "google/lyria-3-clip-preview", balance: 19999, group: 1, blocked: true},
			{name: "free_group", actual: "google/lyria-3-clip-preview", balance: 100000, group: 0},
			{name: "operator_free", actual: "google/lyria-3-clip-preview", balance: 100000, group: 1, local: &model.ModelConfigLocal{PerCall: &model.PerCallPricingLocal{}}},
		} {
			t.Run(protocol+"/"+tc.name, func(t *testing.T) {
				xaiVideoSetup(t, tc.balance, false)
				var calls atomic.Int32
				observed := make(chan xaiVideoObservation, 1)
				upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					var body map[string]any
					err := json.NewDecoder(r.Body).Decode(&body)
					var user model.User
					if err == nil {
						err = model.DB.First(&user, fallbackUserID).Error
					}
					observed <- xaiVideoObservation{Path: r.URL.Path, Body: body, UserQuota: user.Quota, Err: err}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"id":"generation-fixture","object":"chat.completion","model":"`+tc.actual+`","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"Generated music","audio":{"id":"audio-fixture","data":"AQI=","transcript":"Music"}}}],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`)
				}))
				t.Cleanup(upstream.Close)
				previous := client.HTTPClient
				client.HTTPClient = upstream.Client()
				t.Cleanup(func() { client.HTTPClient = previous })
				path, body := "/v1/chat/completions", `{"model":"alias","messages":[{"role":"user","content":"Create music"}],"max_tokens":128}`
				if protocol == "responses" {
					path, body = "/v1/responses", `{"model":"alias","input":"Create music","max_output_tokens":128}`
				}
				if protocol == "messages" {
					path, body = "/v1/messages", `{"model":"alias","messages":[{"role":"user","content":"Create music"}],"max_tokens":128}`
				}
				c, _, id := protocolContext(t, channeltype.OpenRouter, tc.actual, path, body, upstream.URL+"/v1", tc.balance, tc.group, false, tc.local)
				var apiErrStatus int
				switch protocol {
				case "chat":
					if err := RelayTextHelper(c); err != nil {
						apiErrStatus = err.StatusCode
					}
				case "responses":
					if err := RelayResponseAPIHelper(c); err != nil {
						apiErrStatus = err.StatusCode
					}
				case "messages":
					if err := RelayClaudeMessagesHelper(c); err != nil {
						apiErrStatus = err.StatusCode
					}
				}
				drainCriticalTasks(t)
				if tc.blocked {
					require.Equal(t, http.StatusForbidden, apiErrStatus)
					require.Zero(t, calls.Load())
					require.Equal(t, tc.balance, reloadUserQuota(t))
					return
				}
				require.Zero(t, apiErrStatus)
				require.EqualValues(t, 1, calls.Load())
				wire := <-observed
				require.NoError(t, wire.Err)
				require.Equal(t, tc.actual, wire.Body["model"])
				require.Equal(t, tc.balance-tc.charge, wire.UserQuota, "paid generation must be reserved before provider work")
				require.Equal(t, tc.balance-tc.charge, reloadUserQuota(t))
				require.Equal(t, tc.charge, requestCostQuota(t, id))
				var token model.Token
				require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
				require.Equal(t, tc.balance-tc.charge, token.RemainQuota)
			})
		}
	}
}
