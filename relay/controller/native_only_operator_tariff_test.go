package controller

import (
	"fmt"
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
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestNativeOnlyModelOnAzureRequiresOperatorTariffHTTP drives Azure requests for
// a native-only OpenAI model through the production controllers and the SQLite
// quota ledger. Without an operator tariff the gateway must neither contact the
// upstream nor bill native OpenAI's $20/$100 global tariff; with one, the
// operator's price is charged exactly.
func TestNativeOnlyModelOnAzureRequiresOperatorTariffHTTP(t *testing.T) {
	for _, protocol := range []string{"chat", "messages"} {
		for _, priced := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/priced=%t", protocol, priced), func(t *testing.T) {
				const balance = int64(100_000)
				xaiVideoSetup(t, balance, false)
				var calls atomic.Int32
				paths := make(chan string, 1)
				upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					paths <- r.URL.Path
					_, _ = io.Copy(io.Discard, r.Body)
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"id":"chatcmpl-azure","object":"chat.completion","model":"gpt-6-cyber","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ack"}}],"usage":{"prompt_tokens":20,"completion_tokens":2,"total_tokens":22}}`)
				}))
				t.Cleanup(upstream.Close)
				previous := client.HTTPClient
				client.HTTPClient = upstream.Client()
				t.Cleanup(func() { client.HTTPClient = previous })

				var local *model.ModelConfigLocal
				if priced {
					local = &model.ModelConfigLocal{Ratio: 2 * billingratio.MilliTokensUsd, CompletionRatio: 4}
				}
				path, body := lyriaProtocolRequest(protocol, false)
				c, writer, requestID := protocolContext(t, channeltype.Azure, "gpt-6-cyber", path, body, upstream.URL, balance, 1, false, local)
				c.Set(ctxkey.Config, model.ChannelConfig{APIVersion: "2025-04-01-preview"})
				clearNativeSettledLedger(t, requestID)

				apiErr := relayLyriaProtocol(c, protocol)
				drainCriticalTasks(t)
				if !priced {
					require.NotNil(t, apiErr, "an unpriced native-only model must be rejected")
					require.Contains(t, apiErr.Error.Message, "explicit positive input and output pricing")
					require.Zero(t, calls.Load(), "the upstream must never be contacted without an operator tariff")
					require.Equal(t, balance, reloadUserQuota(t), "the admission reservation must be refunded")
					require.Zero(t, consumeLogQuota(t, requestID), "no native OpenAI tariff may be charged")
					return
				}
				require.Nil(t, apiErr)
				require.EqualValues(t, 1, calls.Load())
				require.Contains(t, <-paths, "/openai/deployments/gpt-6-cyber/chat/completions")
				require.Contains(t, writer.Body.String(), "ack")
				// 20 input tokens at $2/M plus 2 output tokens at $8/M, in quota units.
				const charge = int64(28)
				require.Equal(t, balance-charge, reloadUserQuota(t))
				require.Equal(t, charge, consumeLogQuota(t, requestID))
				require.False(t, strings.Contains(writer.Body.String(), "explicit positive"))
				// The reconciled provisional row keeps the client-requested model.
				var logs []model.Log
				require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&logs).Error)
				require.Len(t, logs, 1)
				require.Equal(t, "alias", logs[0].OriginModelName)
			})
		}
	}
}
