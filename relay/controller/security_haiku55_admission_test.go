package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/stretchr/testify/require"
)

// haikuAdmissionObservation records physical reservations and the real provider
// request without requiring assertions from its HTTP goroutine.
type haikuAdmissionObservation struct {
	User, Token int64
	Body        map[string]any
	Err         error
}

// TestSecurityHaiku55HTTPAdmission exercises both production handlers with real
// loopback TLS dispatch and disposable SQLite accounts. Parameters: t owns the
// fixtures. Returns: none; underfunded requests reject before I/O, funded streams
// and ordinary requests reconcile once, and explicit upstream failures refund.
func TestSecurityHaiku55HTTPAdmission(t *testing.T) {
	for _, protocol := range []string{"chat", "messages"} {
		for _, stream := range []bool{false, true} {
			for _, scenario := range []string{"owner_low", "token_low", "funded", "refunded"} {
				t.Run(fmt.Sprintf("%s/stream=%t/%s", protocol, stream, scenario), func(t *testing.T) {
					balance, tokenBalance := int64(300000), int64(300000)
					if scenario == "owner_low" {
						balance = 40000
						tokenBalance = 40000
					}
					if scenario == "token_low" {
						tokenBalance = 40000
					}
					xaiVideoSetup(t, balance, false)
					require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fallbackTokenID).Update("remain_quota", tokenBalance).Error)
					previousEnforcement := config.EnforceIncludeUsage
					config.EnforceIncludeUsage = false
					t.Cleanup(func() { config.EnforceIncludeUsage = previousEnforcement })
					var calls atomic.Int32
					observed := make(chan haikuAdmissionObservation, 1)
					upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						var user model.User
						var token model.Token
						got := haikuAdmissionObservation{}
						got.Err = json.NewDecoder(r.Body).Decode(&got.Body)
						if got.Err == nil {
							got.Err = model.DB.First(&user, fallbackUserID).Error
						}
						if got.Err == nil {
							got.Err = model.DB.First(&token, fallbackTokenID).Error
						}
						got.User, got.Token = user.Quota, token.RemainQuota
						observed <- got
						w.Header().Set("Content-Type", "application/json")
						if scenario == "refunded" {
							w.WriteHeader(http.StatusServiceUnavailable)
							_, _ = io.WriteString(w, `{"type":"error","error":{"type":"overloaded_error","message":"fixture unavailable"}}`)
							return
						}
						usage := `{"input_tokens":106407,"output_tokens":128000}`
						if stream {
							w.Header().Set("Content-Type", "text/event-stream")
							for _, event := range []string{
								`{"type":"message_start","message":{"id":"fixture-msg","type":"message","role":"assistant","model":"claude-haiku-5-5","content":[],"usage":` + usage + `}}`,
								`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
								`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`,
								`{"type":"content_block_stop","index":0}`,
								`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":` + usage + `}`,
								`{"type":"message_stop"}`,
							} {
								if _, err := io.WriteString(w, "data: "+event+"\n\n"); err != nil {
									return
								}
							}
							return
						}
						_, _ = io.WriteString(w, `{"id":"fixture-msg","type":"message","role":"assistant","model":"claude-haiku-5-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":`+usage+`}`)
					}))
					t.Cleanup(upstream.Close)
					previousClient := client.HTTPClient
					client.HTTPClient = upstream.Client()
					t.Cleanup(func() { client.HTTPClient = previousClient })
					content, err := json.Marshal(strings.Repeat("test ", 56000))
					require.NoError(t, err)
					limit := `"max_tokens":128000`
					if protocol == "chat" && scenario == "funded" {
						limit = `"max_completion_tokens":128000`
					}
					body := fmt.Sprintf(`{"model":"alias",%s,"stream":%t,"messages":[{"role":"user","content":%s}]}`, limit, stream, content)
					path := "/v1/chat/completions"
					if protocol == "messages" {
						path = "/v1/messages"
					}
					c, _, id := protocolContext(t, channeltype.Anthropic, "claude-haiku-5-5", path, body, upstream.URL, balance, 1, false, nil)
					c.Set(ctxkey.TokenQuota, tokenBalance)
					apiErr := relayLyriaProtocol(c, protocol)
					drainCriticalTasks(t)
					var token model.Token
					require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
					if strings.HasSuffix(scenario, "low") {
						if calls.Load() > 0 {
							got := <-observed
							t.Logf("REPRODUCED_550 protocol=%s stream=%t held_user=%d held_token=%d final_user=%d final_token=%d calls=%d", protocol, stream, balance-got.User, tokenBalance-got.Token, reloadUserQuota(t), token.RemainQuota, calls.Load())
						}
						require.Zero(t, calls.Load(), "underfunded tier quote crossed provider boundary")
						require.NotNil(t, apiErr)
						require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
						require.Equal(t, balance, reloadUserQuota(t))
						require.Equal(t, tokenBalance, token.RemainQuota)
						require.Zero(t, token.UsedQuota)
						require.Zero(t, consumeLogQuota(t, id))
						return
					}
					require.EqualValues(t, 1, calls.Load())
					got := <-observed
					require.NoError(t, got.Err)
					require.Equal(t, float64(128000), got.Body["max_tokens"])
					require.GreaterOrEqual(t, balance-got.User, int64(186602))
					require.Equal(t, balance-got.User, tokenBalance-got.Token)
					if scenario == "refunded" {
						require.NotNil(t, apiErr)
						require.True(t, BillingAllowsRetry(c))
						ResetPerAttemptBillingForRetry(gmw.Ctx(c), c)
						drainCriticalTasks(t)
						require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
						require.Equal(t, balance, reloadUserQuota(t))
						require.Equal(t, tokenBalance, token.RemainQuota)
						require.Zero(t, token.UsedQuota)
						require.Zero(t, consumeLogQuota(t, id))
						return
					}
					require.Nil(t, apiErr)
					require.EqualValues(t, balance-186602, reloadUserQuota(t))
					require.EqualValues(t, tokenBalance-186602, token.RemainQuota)
					require.EqualValues(t, 186602, token.UsedQuota)
					require.EqualValues(t, 186602, requestCostQuota(t, id))
					var logs []model.Log
					require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", id, model.LogTypeConsume).Find(&logs).Error)
					require.Len(t, logs, 1)
					require.EqualValues(t, 186602, logs[0].Quota)
				})
			}
		}
	}
}

// TestSecurityHaiku55PassthroughCacheAdmission preserves valid raw automatic and
// tool-definition cache markers omitted by typed DTOs. Parameters: t owns local
// TLS and SQLite fixtures. Returns: none; an underfunded one-hour cache creation
// cannot dispatch, while the ordinary token quote alone would fit the balance.
func TestSecurityHaiku55PassthroughCacheAdmission(t *testing.T) {
	for _, marker := range []string{`"cache_control":{"type":"ephemeral","ttl":"1h"}`, `"tools":[{"name":"lookup","description":"fixture","input_schema":{"type":"object"},"cache_control":{"type":"ephemeral","ttl":"1h"}}]`} {
		t.Run(marker, func(t *testing.T) {
			const balance = int64(200000)
			xaiVideoSetup(t, balance, false)
			var calls atomic.Int32
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"fixture-msg","type":"message","role":"assistant","model":"claude-haiku-5-5","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":0,"cache_creation_input_tokens":106407,"cache_creation":{"ephemeral_1h_input_tokens":106407,"ephemeral_5m_input_tokens":0},"output_tokens":128000}}`)
			}))
			t.Cleanup(upstream.Close)
			previous := client.HTTPClient
			client.HTTPClient = upstream.Client()
			t.Cleanup(func() { client.HTTPClient = previous })
			content, err := json.Marshal(strings.Repeat("test ", 56000))
			require.NoError(t, err)
			body := fmt.Sprintf(`{"model":"alias","max_tokens":128000,"messages":[{"role":"user","content":%s}],%s}`, content, marker)
			c, _, id := protocolContext(t, channeltype.Anthropic, "claude-haiku-5-5", "/v1/messages", body, upstream.URL, balance, 1, false, nil)
			apiErr := RelayClaudeMessagesHelper(c)
			drainCriticalTasks(t)
			require.True(t, c.GetBool(ctxkey.ClaudeDirectPassthrough), "fixture must exercise the actual raw provider branch")
			require.Zero(t, calls.Load())
			require.NotNil(t, apiErr)
			require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
			require.Equal(t, balance, reloadUserQuota(t))
			require.Zero(t, consumeLogQuota(t, id))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, balance, token.RemainQuota)
			require.Zero(t, token.UsedQuota)
		})
	}
}
