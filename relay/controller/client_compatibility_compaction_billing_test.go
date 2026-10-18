package controller

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

const clientCompatibilityCompactionBlock = `{"type":"compaction","content":"Synthetic signed summary","signature":"<signed>&fixture","opaque":9007199254740993}`

// clientCompatibilityCompactionResponse returns synthetic native JSON or named SSE.
// It emits a complete signed block without deltas, a ping, and repeated cumulative
// receipts so the returned payload exercises forwarding as well as exact accounting.
func clientCompatibilityCompactionResponse(t *testing.T, stream bool, usage string) string {
	t.Helper()
	if !stream {
		return `{"id":"msg_compaction","type":"message","role":"assistant","model":"claude-sonnet-4","content":[` + clientCompatibilityCompactionBlock + `],"stop_reason":"compaction","usage":` + usage + `}`
	}
	events := []string{
		`{"type":"message_start","message":{"id":"msg_compaction","type":"message","model":"claude-sonnet-4","role":"assistant","content":[],"usage":` + usage + `}}`,
		`{"type":"content_block_start","index":0,"content_block":` + clientCompatibilityCompactionBlock + `}`,
		`{"type":"ping"}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"compaction"},"usage":` + usage + `}`,
		`{"type":"message_delta","delta":{"stop_reason":"compaction"},"usage":` + usage + `}`,
		`{"type":"message_stop"}`,
	}
	var body strings.Builder
	for _, event := range events {
		var envelope struct {
			Type string `json:"type"`
		}
		// Fixture constants are JSON; extracting the discriminator does not rewrite payload bytes.
		require.NoError(t, json.Unmarshal([]byte(event), &envelope))
		body.WriteString("event: " + envelope.Type + "\ndata: " + event + "\n\n")
	}
	return body.String()
}

// TestClientCompatibilityCompactionHTTPBilling runs native JSON and SSE through the
// real controller and local TLS upstream, then checks user/token/request/log debits.
// Official usage semantics: iterations are canonical, not additional to root totals.
func TestClientCompatibilityCompactionHTTPBilling(t *testing.T) {
	const balance = int64(1_000_000)
	for _, tc := range []struct {
		name, usage string
		want        int64
	}{
		{name: "on_demand", usage: `{"input_tokens":0,"output_tokens":0,"iterations":[{"type":"compaction","input_tokens":144,"output_tokens":276}]}`, want: 696},
		{name: "threshold", usage: `{"input_tokens":23000,"output_tokens":1000,"iterations":[{"type":"compaction","input_tokens":180000,"output_tokens":3500},{"type":"message","input_tokens":23000,"output_tokens":1000}]}`, want: 212000},
		{name: "ordinary_control", usage: `{"input_tokens":11,"output_tokens":7}`, want: 25},
	} {
		for _, stream := range []bool{false, true} {
			name := tc.name + "/json"
			contentType := "application/json"
			if stream {
				name = tc.name + "/sse"
				contentType = "text/event-stream"
			}
			t.Run(name, func(t *testing.T) {
				result := runClaudeToolPolicyCase(t, balance, claudeToolPolicyCase{
					channel: channeltype.Anthropic, actual: "claude-sonnet-4", contentType: contentType,
					payload:  claudeToolPolicyPayload(t, stream, nil, nil),
					response: clientCompatibilityCompactionResponse(t, stream, tc.usage), beta: "compact-2026-09-04",
					prepare: func(c *gin.Context) {
						responseNamespaceCleanLedger(t, c.GetString(ctxkey.RequestId))
						if tc.name == "ordinary_control" {
							c.Request.Header.Del("anthropic-beta")
						}
						channel := c.MustGet(ctxkey.ChannelModel).(*model.Channel)
						require.NoError(t, channel.SetModelPriceConfigs(map[string]model.ModelConfigLocal{"claude-sonnet-4": {Ratio: 1, CompletionRatio: 2}}))
					},
				})
				require.Nil(t, result.apiErr)
				require.EqualValues(t, 1, result.calls)
				require.Contains(t, result.w.Body.String(), clientCompatibilityCompactionBlock)
				requireClaudeToolPolicyLedger(t, result, balance, tc.want)
				if stream {
					require.Contains(t, result.w.Body.String(), "event: ping\n")
					require.Equal(t, 1, strings.Count(result.w.Body.String(), "event: message_stop\n"))
					require.NotContains(t, result.w.Body.String(), "[DONE]")
				}
			})
		}
	}
}
