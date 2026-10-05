package controller

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
			tokenOnly       bool
			receipt         string
			writeFail       bool
		}{
			{name: "clip", actual: "google/lyria-3-clip-preview", balance: 100000, charge: 20000, group: 1},
			{name: "song", actual: "google/lyria-3-pro-preview", balance: 100000, charge: 40000, group: 1},
			{name: "finite_token_insufficient", actual: "google/lyria-3-clip-preview", balance: 100000, group: 1, blocked: true, tokenOnly: true},
			{name: "insufficient", actual: "google/lyria-3-clip-preview", balance: 19999, group: 1, blocked: true},
			{name: "malformed_receipt", actual: "google/lyria-3-clip-preview", balance: 100000, charge: 20000, group: 1, receipt: "malformed"},
			{name: "missing_receipt", actual: "google/lyria-3-clip-preview", balance: 100000, charge: 20000, group: 1, receipt: "missing"},
			{name: "write_failure", actual: "google/lyria-3-clip-preview", balance: 100000, charge: 20000, group: 1, writeFail: true},
			{name: "operator_paid", actual: "google/lyria-3-clip-preview", balance: 100000, charge: 30000, group: 1, local: &model.ModelConfigLocal{PerCall: &model.PerCallPricingLocal{UsdPerThousandCalls: 60}}},
			{name: "operator_ratio_free", actual: "google/lyria-3-clip-preview", balance: 100000, group: 1, local: &model.ModelConfigLocal{CompletionRatio: 1}},
			{name: "free_group", actual: "google/lyria-3-clip-preview", balance: 100000, group: 0},
			{name: "operator_free", actual: "google/lyria-3-clip-preview", balance: 100000, group: 1, local: &model.ModelConfigLocal{PerCall: &model.PerCallPricingLocal{}}},
		} {
			t.Run(protocol+"/"+tc.name, func(t *testing.T) {
				xaiVideoSetup(t, tc.balance, false)
				if tc.tokenOnly {
					require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fallbackTokenID).Update("remain_quota", 19999).Error)
				}
				audioData := base64.StdEncoding.EncodeToString(silentWAV(t, 1))
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
					reply := `{"id":"generation-fixture","object":"chat.completion","model":"` + tc.actual + `","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"Generated music","audio":{"id":"audio-fixture","data":"` + audioData + `","transcript":"Music"}}}],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`
					if tc.receipt == "missing" {
						reply = strings.Replace(reply, `,"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}`, "", 1)
					}
					if tc.receipt == "malformed" {
						reply = "{"
					}
					_, _ = io.WriteString(w, reply)
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
				if tc.writeFail {
					c.Writer = xaiDisconnectedWriter{ResponseWriter: c.Writer}
				}
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
				if !tc.writeFail && tc.receipt != "malformed" {
					require.Zero(t, apiErrStatus)
				}
				require.EqualValues(t, 1, calls.Load())
				wire := <-observed
				require.NoError(t, wire.Err)
				require.Equal(t, tc.actual, wire.Body["model"])
				require.Equal(t, tc.balance-tc.charge, wire.UserQuota, "paid generation must be reserved before provider work")
				require.Equal(t, tc.balance-tc.charge, reloadUserQuota(t))
				if protocol != "messages" {
					require.Equal(t, tc.charge, requestCostQuota(t, id))
				}
				var logs []model.Log
				require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", id, model.LogTypeConsume).Find(&logs).Error)
				if tc.charge > 0 {
					require.Len(t, logs, 1)
					require.EqualValues(t, tc.charge, logs[0].Quota)
				}
				var token model.Token
				require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
				require.Equal(t, tc.balance-tc.charge, token.RemainQuota)
			})
		}
	}
}
