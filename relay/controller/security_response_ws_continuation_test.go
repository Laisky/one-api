package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	quotautil "github.com/Laisky/one-api/relay/quota"
	"github.com/Laisky/one-api/relay/relaymode"
)

// This test is a permanent behavior regression for issue #596. Without the
// fix, native Responses WebSocket handshakes reserve one estimate but a
// response.create with previous_response_id can execute inherited context
// without an additional funded reservation. HTTP continuations were already
// repaired by #555 and #557.
//
// Before the fix, the independent red run 40050884387 confirmed four failures:
//   - owner_low: an underfunded continuation frame reaches the provider.
//   - token_low: an underfunded continuation frame reaches the provider.
//   - funded_multiround: rounds dispatch with only the handshake hold.
//   - after_paid_known_work: an underfunded later frame reaches the provider.
//
// Ordinary, free_group and flat controls protect prior behavior. The green
// acceptance requires every known-context debit to precede provider dispatch.

// TestSecurityNativeResponseWebSocketKnownContinuationAdmission uses real supported
// OpenAI WebSocket handlers and an isolated SQLite ledger. Parameters: t owns a
// loopback provider with completed parent context. Returns: none; paid unbounded
// continuation frames must not execute, while ordinary/free/flat frames retain
// their settlement. Known underfunded context uses the existing funding errors,
// while funded multi-round work is reserved before I/O and settled exactly once.
// No unknown-context rejection or new WebSocket close-reason policy is assumed.
func TestSecurityNativeResponseWebSocketKnownContinuationAdmission(t *testing.T) {
	const name = "native-ws-tier-fixture"
	const output = 20
	for _, scenario := range []string{"owner_low", "token_low", "funded_multiround", "incomplete_local", "alias_echo", "after_paid_known_work", "ordinary", "free_group", "flat"} {
		t.Run(scenario, func(t *testing.T) {
			const initial int64 = 500000
			xaiVideoSetup(t, initial, false)
			store := enableStateForTest(t)
			cfg := model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2, MaxTokens: 20, Tiers: []model.ModelRatioTierLocal{{InputTokenThreshold: 100, Ratio: 5, CompletionRatio: 100}}}
			var frames atomic.Int32
			var handshakes atomic.Int32
			var retainedInput atomic.Int64
			var fullInputTotal atomic.Int64
			type fundingObservation struct {
				prompt      int
				user, token int64
				err         error
			}
			funding := make(chan fundingObservation, 2)
			providerErrors := make(chan error, 2)
			receipt := func(id string, prompt int, storeMode bool) map[string]any {
				return map[string]any{"id": id, "object": "response", "status": "completed", "model": name, "store": storeMode,
					"output": []any{map[string]any{"type": "message", "id": "msg_" + id, "status": "completed", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "local output", "annotations": []any{}}}}},
					"usage":  map[string]int{"input_tokens": prompt, "output_tokens": output, "total_tokens": prompt + output}}
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !websocket.IsWebSocketUpgrade(r) {
					var request openai.ResponseAPIRequest
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						providerErrors <- err
						http.Error(w, "invalid fixture request", 400)
						return
					}
					retainedInput.Store(int64(getResponseAPIPromptTokens(r.Context(), &request)))
					response := receipt("resp_ws_stored", int(retainedInput.Load()), true)
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
					return
				}
				handshakes.Add(1)
				upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					providerErrors <- err
					return
				}
				defer conn.Close()
				_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
				localInput := 0
				for {
					_, body, err := conn.ReadMessage()
					if err != nil {
						return
					}
					round := frames.Add(1)
					var wire map[string]any
					var request openai.ResponseAPIRequest
					if err = json.Unmarshal(body, &wire); err == nil {
						err = json.Unmarshal(body, &request)
					}
					if err != nil || wire["type"] != "response.create" || request.Model != name || request.MaxOutputTokens == nil || *request.MaxOutputTokens != output {
						providerErrors <- fmt.Errorf("invalid fixture create frame: %v", err)
						return
					}
					current := getResponseAPIPromptTokens(r.Context(), &request)
					full := current
					if request.PreviousResponseId != nil && *request.PreviousResponseId != "" {
						switch *request.PreviousResponseId {
						case "resp_ws_stored":
							full += int(retainedInput.Load()) + output
						case "resp_ws_local":
							full += localInput + output
						default:
							providerErrors <- fmt.Errorf("unknown local parent")
							return
						}
					}
					if request.Prompt != nil && request.Prompt.Id == "prompt_ws_fixture" {
						full += 5000
					}
					fullInputTotal.Add(int64(full))
					var account model.User
					var finite model.Token
					accountErr := model.DB.First(&account, fallbackUserID).Error
					if accountErr == nil {
						accountErr = model.DB.First(&finite, fallbackTokenID).Error
					}
					funding <- fundingObservation{full, account.Quota, finite.RemainQuota, accountErr}
					id := "resp_ws_second"
					if (scenario == "funded_multiround" || scenario == "incomplete_local" || scenario == "alias_echo" || scenario == "after_paid_known_work") && round == 1 {
						id = "resp_ws_local"
						localInput = full
					}
					eventType := "response.completed"
				response := receipt(id, full, false)
				if scenario == "incomplete_local" && round == 1 {
					eventType = "response.incomplete"
					response["status"] = "incomplete"
				}
				if scenario == "alias_echo" && round == 1 {
					response["model"] = name + "-2026-10-01"
				}
				if err = conn.WriteJSON(map[string]any{"type": eventType, "response": response}); err != nil {
						return
					}
				}
			}))
			t.Cleanup(upstream.Close)
			oldClient := client.HTTPClient
			client.HTTPClient = upstream.Client()
			t.Cleanup(func() { client.HTTPClient = oldClient })
			body, err := json.Marshal(map[string]any{"model": "alias", "input": strings.Repeat("parent ", 12000), "max_output_tokens": output, "store": true})
			require.NoError(t, err)
			first, _, firstID := protocolContext(t, channeltype.OpenAI, name, "/v1/responses", string(body), upstream.URL+"/api.openai.com", initial, 1, false, &cfg)
			firstMeta := metalib.GetByContext(first)
			firstMeta.OriginModelName = name
			firstMeta.ActualModelName = name
			require.True(t, supportsNativeResponseAPI(firstMeta))
			require.Nil(t, RelayResponseAPIHelper(first))
			drainCriticalTasks(t)
			parent, err := store.GetResponse(context.Background(), stateOwnerFromMeta(firstMeta), "resp_ws_stored")
			require.NoError(t, err)
			require.NotNil(t, parent.Binding)
			require.Equal(t, firstMeta.APIType, parent.Binding.APIType)
			var parentUsage openai.ResponseAPIUsage
			require.NoError(t, json.Unmarshal(parent.Usage, &parentUsage))
			require.EqualValues(t, retainedInput.Load(), parentUsage.InputTokens)
			require.Equal(t, output, parentUsage.OutputTokens)
			require.Positive(t, parentUsage.InputTokens)
			firstCharge := consumeLogQuota(t, firstID)
			// Reset only disposable fixture balances to represent later account activity.
			balance := int64(1000000)
			tokenBalance := balance
			require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", fallbackUserID).Update("quota", balance).Error)
			require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fallbackTokenID).Update("remain_quota", tokenBalance).Error)
			group := 1.0
			if scenario == "free_group" {
				group = 0
			}
			if scenario == "flat" {
				cfg.Tiers = nil
			}
			template, _, requestID := protocolContext(t, channeltype.OpenAI, name, "/v1/responses?model="+name, "", upstream.URL+"/api.openai.com", balance, group, false, &cfg)
			requestID += "-socket"
			template.Set(ctxkey.RequestId, requestID)
			if scenario == "owner_low" || scenario == "token_low" || scenario == "after_paid_known_work" {
				meta := metalib.GetByContext(template)
				estimatedPrompt := config.PreconsumeTokenForBackgroundRequest
				if estimatedPrompt <= 0 {
					estimatedPrompt = 2000
				}
				handshake, tiered, quoteErr := quoteResponseTierAdmission(template, meta, &openai.ResponseAPIRequest{Model: name}, estimatedPrompt, nil)
				require.NoError(t, quoteErr)
				require.True(t, tiered)
				if scenario == "after_paid_known_work" {
					balance, tokenBalance = handshake+100, handshake+100
				} else {
					cap := output
					incremental := getResponseAPIPromptTokens(context.Background(), &openai.ResponseAPIRequest{Model: name, Input: openai.ResponseAPIInput{"next"}})
					knownPrompt := int(retainedInput.Load()) + output + incremental
					knownQuote, applies, knownErr := quoteResponseTierAdmission(template, meta, &openai.ResponseAPIRequest{Model: name, MaxOutputTokens: &cap}, knownPrompt, nil)
					require.NoError(t, knownErr)
					require.True(t, applies)
					require.Greater(t, knownQuote, handshake+1, "the fixture must expose work beyond the affordable handshake")
					limited := handshake + (knownQuote-handshake)/2
					require.GreaterOrEqual(t, limited, handshake)
					require.Less(t, limited, knownQuote)
					if scenario == "owner_low" {
						balance = limited
					} else {
						tokenBalance = limited
					}
				}
				require.GreaterOrEqual(t, balance, handshake)
				require.GreaterOrEqual(t, tokenBalance, handshake)
				require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", fallbackUserID).Update("quota", balance).Error)
				require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", fallbackTokenID).Update("remain_quota", tokenBalance).Error)
			}
			template.Set(ctxkey.TokenQuota, tokenBalance)
			if scenario == "after_paid_known_work" {
				cap := output
				firstRequest := &openai.ResponseAPIRequest{Model: name, Input: openai.ResponseAPIInput{"first counted paid turn"}, MaxOutputTokens: &cap}
				secondRequest := &openai.ResponseAPIRequest{Model: name, Input: openai.ResponseAPIInput{strings.Repeat("additional counted text ", 5000)}, MaxOutputTokens: &cap}
				firstPrompt := getResponseAPIPromptTokens(context.Background(), firstRequest)
				secondPrompt := firstPrompt + output + getResponseAPIPromptTokens(context.Background(), secondRequest)
				rates, completions := getChannelRatios(template)
				quote := func(prompt int) int64 {
					input := quotautil.ComputeInput{Usage: &relaymodel.Usage{PromptTokens: prompt}, ModelName: name,
						ModelRatio: rates[name], ChannelModelRatio: rates, GroupRatio: group,
						ChannelModelConfigs: getChannelModelConfigs(template), ChannelCompletionRatio: completions,
						PricingAdaptor: &openai.Adaptor{}, RequestTime: metalib.GetByContext(template).StartTime}
					amount, applies, err := quotautil.EstimateTierAdmission(input, output, 0, quotautil.AdmissionOptions{})
					require.NoError(t, err)
					require.True(t, applies)
					return amount
				}
				firstQuote, secondQuote := quote(firstPrompt), quote(secondPrompt)
				require.LessOrEqual(t, firstQuote, balance, "the first counted frame must be independently affordable")
				require.LessOrEqual(t, firstQuote, tokenBalance)
				require.Greater(t, firstQuote+secondQuote, balance, "only the later known-context work is unaffordable")
				require.Greater(t, firstQuote+secondQuote, tokenBalance)
			}
			type result struct {
				err  *relaymodel.ErrorWithStatusCode
				hold int64
			}
			done := make(chan result, 1)
			router := gin.New()
			router.GET("/v1/responses", func(c *gin.Context) {
				for key, value := range template.Keys {
					c.Set(key, value)
				}
				meta := metalib.GetByContext(c)
				meta.Mode = relaymode.ResponseAPI
				meta.OriginModelName = name
				meta.ActualModelName = name
				handled, bizErr := maybeHandleResponseAPIWebSocket(c, meta)
				if !handled {
					bizErr = openai.ErrorWrapper(fmt.Errorf("socket fixture was not handled"), "fixture_error", 500)
				}
				done <- result{bizErr, c.GetInt64(ctxkey.PreConsumedQuotaAmount)}
			})
			proxy := httptest.NewServer(router)
			t.Cleanup(proxy.Close)
			conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(proxy.URL, "http")+"/v1/responses?model="+name, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = conn.Close() })
			_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			frame := map[string]any{"type": "response.create", "model": name, "input": "next", "max_output_tokens": output}
			if scenario == "funded_multiround" || scenario == "incomplete_local" || scenario == "alias_echo" {
				frame["previous_response_id"] = parent.GatewayResponseID
				require.NoError(t, conn.WriteJSON(frame))
				_, firstReply, firstErr := conn.ReadMessage()
				require.NoError(t, firstErr)
				require.Contains(t, string(firstReply), "resp_ws_local")
				frame["previous_response_id"] = "resp_ws_local"
				frame["input"] = "third counted turn"
			} else if scenario == "after_paid_known_work" {
				frame["input"] = "first counted paid turn"
				require.NoError(t, conn.WriteJSON(frame))
				_, firstReply, firstErr := conn.ReadMessage()
				require.NoError(t, firstErr)
				require.Contains(t, string(firstReply), "resp_ws_local")
				frame["previous_response_id"] = "resp_ws_local"
				frame["input"] = strings.Repeat("additional counted text ", 5000)
			} else if scenario != "ordinary" {
				frame["previous_response_id"] = parent.GatewayResponseID
			}
			require.NoError(t, conn.WriteJSON(frame))
			_, _, readErr := conn.ReadMessage()
			paidDenied := scenario == "owner_low" || scenario == "token_low" || scenario == "after_paid_known_work"
			if readErr == nil {
				_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "complete"), time.Now().Add(time.Second))
			}
			_ = conn.Close()
			var got result
			select {
			case got = <-done:
			case <-time.After(6 * time.Second):
				t.Fatal("bounded raw socket did not settle")
			}
			drainCriticalTasks(t)
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			finalUser := reloadUserQuota(t)
			t.Logf("RAW_WS scenario=%s handshakes=%d frames=%d hold=%d socket_charge=%d final_user=%d final_token=%d", scenario, handshakes.Load(), frames.Load(), got.hold, consumeLogQuota(t, requestID), finalUser, token.RemainQuota)
			select {
			case fixtureErr := <-providerErrors:
				require.NoError(t, fixtureErr)
			default:
			}
			require.EqualValues(t, 1, handshakes.Load())
			rates, completions := getChannelRatios(template)
			compute := func(prompt, completion int) int64 {
				return quotautil.Compute(quotautil.ComputeInput{
					Usage:     &relaymodel.Usage{PromptTokens: prompt, CompletionTokens: completion},
					ModelName: name, ModelRatio: rates[name], ChannelModelRatio: rates,
					GroupRatio: group, ChannelModelConfigs: getChannelModelConfigs(template),
					ChannelCompletionRatio: completions, PricingAdaptor: &openai.Adaptor{},
					RequestTime: metalib.GetByContext(template).StartTime,
				}).TotalQuota
			}
			requireSingleSettlement := func(total int64) {
				var rows []model.Log
				require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&rows).Error)
				require.Len(t, rows, 1, "completed socket work must finalize one consume row")
				require.EqualValues(t, total, rows[0].Quota)
				var costs []model.UserRequestCost
				require.NoError(t, model.DB.Where("request_id = ?", requestID).Find(&costs).Error)
				require.Len(t, costs, 1, "multi-round socket work must retain one request-cost row")
				require.Equal(t, total, costs[0].Quota)
			}
			if paidDenied {
				expectedFrames := int32(0)
				if scenario == "after_paid_known_work" {
					expectedFrames = 1
				}
				require.Equal(t, expectedFrames, frames.Load(), "known underfunded continuation must reject before its create frame reaches provider")
				require.Error(t, readErr)
				if expectedFrames == 0 {
					require.NotNil(t, got.err, "known allowance must produce the established funding error")
					require.Equal(t, http.StatusForbidden, got.err.StatusCode)
					code := "insufficient_user_quota"
					if scenario == "token_low" {
						code = "pre_consume_token_quota_failed"
					}
					require.Equal(t, code, got.err.Code)
				} else {
					require.Nil(t, got.err, "completed paid work must retain settlement instead of taking the whole-session failure refund")
				}
				// Compute enforces a one-unit minimum for an admitted paid
				// request, but a pre-dispatch denied WebSocket did no paid work.
				// Its refunded/voided provisional log must therefore be zero.
				expectedCharge := int64(0)
				if expectedFrames > 0 {
					expectedCharge = compute(int(fullInputTotal.Load()), int(expectedFrames)*output)
					requireSingleSettlement(expectedCharge)
				}
				require.Equal(t, expectedCharge, consumeLogQuota(t, requestID))
				require.Equal(t, balance-expectedCharge, finalUser)
				require.Equal(t, tokenBalance-expectedCharge, token.RemainQuota)
				require.Equal(t, firstCharge+expectedCharge, token.UsedQuota)
				return
			}
			require.NoError(t, readErr)
			require.Nil(t, got.err)
			expectedFrames := int32(1)
			if scenario == "funded_multiround" || scenario == "incomplete_local" || scenario == "alias_echo" {
				expectedFrames = 2
			}
			require.Equal(t, expectedFrames, frames.Load())
			expectedCharge := compute(int(fullInputTotal.Load()), int(expectedFrames)*output)
			require.Equal(t, expectedCharge, consumeLogQuota(t, requestID), "all terminal rounds must settle once without duplicate handshake charges")
			require.Equal(t, balance-expectedCharge, finalUser)
			require.Equal(t, tokenBalance-expectedCharge, token.RemainQuota)
			require.Equal(t, firstCharge+expectedCharge, token.UsedQuota)
			if scenario == "funded_multiround" || scenario == "incomplete_local" || scenario == "alias_echo" {
				requireSingleSettlement(expectedCharge)
				cumulativeInput := 0
				for round := 1; round <= int(expectedFrames); round++ {
					observed := <-funding
					require.NoError(t, observed.err)
					cumulativeInput += observed.prompt
					minimumHold := compute(cumulativeInput, round*output)
					require.GreaterOrEqual(t, balance-observed.user, minimumHold, "each funded round needs a known context allowance before provider I/O")
					require.GreaterOrEqual(t, tokenBalance-observed.token, minimumHold, "finite-token funding must also precede provider I/O")
				}
			}
		})
	}
}
