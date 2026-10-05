package controller

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// generationBoundaryObservation captures the actual provider wire and both
// physical reservations without making fatal assertions in the HTTP goroutine.
type generationBoundaryObservation struct {
	Body        map[string]json.RawMessage
	Raw, Path   string
	User, Token int64
	Err         error
}

// TestSecurityLyriaSingleGenerationWireLedger verifies valid one-generation
// requests across mapping, direct Chat passthrough, protocol conversion and SSE.
// Parameters: t owns each isolated fixture. Returns: none; provider-visible work,
// reservation, user/token debits, request cost and exactly one consume log agree.
func TestSecurityLyriaSingleGenerationWireLedger(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "messages"} {
		for _, stream := range []bool{false, true} {
			for _, mapped := range []bool{false, true} {
				for _, ct := range []string{"application/json", "Application/JSON; charset=UTF-8"} {
					t.Run(fmt.Sprintf("%s/stream=%t/mapped=%t/%s", protocol, stream, mapped, ct), func(t *testing.T) {
						const balance, charge = int64(100000), int64(20000)
						xaiVideoSetup(t, balance, false)
						previousEnforcement := config.EnforceIncludeUsage
						config.EnforceIncludeUsage = false
						t.Cleanup(func() { config.EnforceIncludeUsage = previousEnforcement })
						var calls atomic.Int32
						observed := make(chan generationBoundaryObservation, 1)
						reply := lyriaChatReply(t)
						audio := base64.StdEncoding.EncodeToString(silentWAV(t, 1))
						upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							calls.Add(1)
							got := generationBoundaryObservation{Path: r.URL.Path}
							raw, err := io.ReadAll(r.Body)
							got.Raw, got.Err = string(raw), err
							if got.Err == nil {
								got.Err = json.Unmarshal(raw, &got.Body)
							}
							var user model.User
							var token model.Token
							if got.Err == nil {
								got.Err = model.DB.First(&user, fallbackUserID).Error
							}
							if got.Err == nil {
								got.Err = model.DB.First(&token, fallbackTokenID).Error
							}
							got.User, got.Token = user.Quota, token.RemainQuota
							select {
							case observed <- got:
							default:
								// The atomic count reports extra dispatches without blocking cleanup.
							}
							if stream {
								w.Header().Set("Content-Type", "text/event-stream")
								chunks := []string{
									`{"id":"gen-boundary","object":"chat.completion.chunk","model":"` + lyriaClipModel + `","choices":[{"index":0,"delta":{"role":"assistant","content":"Generated music","audio":{"id":"audio-fixture","data":"` + audio + `","transcript":"Music"}}}]}`,
									`{"id":"gen-boundary","object":"chat.completion.chunk","model":"` + lyriaClipModel + `","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
									`{"id":"gen-boundary","object":"chat.completion.chunk","model":"` + lyriaClipModel + `","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`,
									`[DONE]`,
								}
								for _, chunk := range chunks {
									if _, err := io.WriteString(w, "data: "+chunk+"\n\n"); err != nil {
										return
									}
								}
								return
							}
							w.Header().Set("Content-Type", "application/json")
							if _, err := io.WriteString(w, reply); err != nil {
								return
							}
						}))
						t.Cleanup(upstream.Close)
						previousClient := client.HTTPClient
						client.HTTPClient = upstream.Client()
						t.Cleanup(func() { client.HTTPClient = previousClient })

						path, body := lyriaProtocolRequest(protocol, stream)
						countField := `"n":1`
						if ct != "application/json" {
							countField = `"\u006e":1`
						}
						body = strings.TrimSuffix(body, "}") + "," + countField + `,"metadata":{"N":"2","num_outputs":"2"}}`
						if !mapped {
							body = strings.Replace(body, `"model":"alias"`, `"model":"`+lyriaClipModel+`"`, 1)
						}
						// A body exists, so a query cannot override the billable count or model.
						c, _, id := protocolContext(t, channeltype.OpenRouter, lyriaClipModel, path+"?n=2&N=2&model=not-the-body", body, upstream.URL+"/v1", balance, 1, false, nil)
						c.Request.Header.Set("Content-Type", ct)
						c.Set(ctxkey.ContentType, ct)
						if !mapped {
							c.Set(ctxkey.RequestModel, lyriaClipModel)
							c.Set(ctxkey.ModelMapping, map[string]string{})
						}
						apiErr := relayLyriaProtocol(c, protocol)
						drainCriticalTasks(t)
						require.Nil(t, apiErr)
						require.EqualValues(t, 1, calls.Load())
						var got generationBoundaryObservation
						select {
						case got = <-observed:
						default:
							t.Fatal("provider observation missing after relay completed")
						}
						require.NoError(t, got.Err)
						require.Equal(t, "/v1/chat/completions", got.Path)
						require.JSONEq(t, `"`+lyriaClipModel+`"`, string(got.Body["model"]))
						if count, present := got.Body["n"]; present {
							require.JSONEq(t, "1", string(count))
						}
						require.NotContains(t, got.Body, "N")
						require.Contains(t, got.Body, "messages")
						require.NotContains(t, got.Body, "input")
						if protocol == "chat" && !mapped {
							require.Equal(t, body, got.Raw, "identity-model Chat must exercise the raw passthrough path")
						}
						require.Equal(t, balance-charge, got.User)
						require.Equal(t, balance-charge, got.Token)
						require.Equal(t, balance-charge, reloadUserQuota(t))
						var token model.Token
						require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
						require.Equal(t, balance-charge, token.RemainQuota)
						require.Equal(t, charge, token.UsedQuota)
						require.Equal(t, charge, requestCostQuota(t, id))
						var logs []model.Log
						require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", id, model.LogTypeConsume).Find(&logs).Error)
						require.Len(t, logs, 1)
						require.EqualValues(t, charge, logs[0].Quota)
					})
				}
			}
		}
	}
}
