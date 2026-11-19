package controller

import (
	"bufio"
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
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// cyberPartialUsage is the complete Standard receipt used by fixtures: 20
// input tokens at $20/M plus 2 output tokens at $100/M settle at 300 quota.
const cyberPartialUsage = `{"input_tokens":20,"output_tokens":2,"total_tokens":22}`

// cyberPartialExactCharge is the exact Standard charge of cyberPartialUsage.
const cyberPartialExactCharge = int64(300)

// cyberDeltaEvent renders one Responses output_text delta SSE event.
// Parameters: text is the delta. Returns: the SSE frame.
func cyberDeltaEvent(text string) string {
	payload, _ := json.Marshal(map[string]any{"type": "response.output_text.delta", "item_id": "msg_cyber",
		"output_index": 0, "content_index": 0, "delta": text})
	return "event: response.output_text.delta\ndata: " + string(payload) + "\n\n"
}

// cyberTerminalEvent renders a terminal Responses SSE event. Parameters: status
// is completed or failed, output is the response output JSON, and usage is the
// usage JSON or empty for none. Returns: the SSE frame.
func cyberTerminalEvent(status, output, usage string) string {
	body := `{"id":"resp_cyber_partial","object":"response","model":"gpt-6-cyber","status":"` + status +
		`","service_tier":"default","output":` + output
	if usage != "" {
		body += `,"usage":` + usage
	}
	if status == "failed" {
		body += `,"error":{"type":"server_error","message":"generation failed after partial output"}`
	}
	body += `}`
	return "event: response." + status + "\ndata: {\"type\":\"response." + status + "\",\"response\":" + body + "}\n\n"
}

// cyberOutput renders one assistant message output array. Parameters: text is
// the delivered text. Returns: the JSON output array.
func cyberOutput(text string) string {
	encoded, _ := json.Marshal(text)
	return `[{"type":"message","id":"msg_cyber","role":"assistant","content":[{"type":"output_text","text":` + string(encoded) + `}]}]`
}

// cyberPartialScenario describes one upstream delivery and its settlement contract.
type cyberPartialScenario struct {
	name   string
	stream bool
	// serve writes the upstream response; it may hijack the connection.
	serve func(t *testing.T, w http.ResponseWriter)
	// exact is the required charge; zero means at least the reservation.
	exact int64
	// estimated requires an auditable billing_estimate_reason on the ledger.
	estimated bool
}

// TestGPT6CyberPartialDeliverySettlementHTTP drives partial, failed and
// oversized native Cyber deliveries through the real controllers, quota ledger
// and SQLite logs for every client protocol. A receipt the gateway cannot fully
// observe must retain at least the reservation as a flagged estimate; a complete
// Standard receipt, including a paid failure, must settle exactly; and no attempt
// that reached the provider may be replayed on another channel.
func TestGPT6CyberPartialDeliverySettlementHTTP(t *testing.T) {
	hugeText := strings.Repeat("x", 2<<20+1024)
	scenarios := []cyberPartialScenario{
		{
			name: "eof_after_deltas_without_terminal_event", stream: true, estimated: true,
			serve: func(t *testing.T, w http.ResponseWriter) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, cyberDeltaEvent("partial work ")+cyberDeltaEvent("before the stream ended"))
			},
		},
		{
			name: "transport_reset_after_deltas", stream: true, estimated: true,
			serve: func(t *testing.T, w http.ResponseWriter) {
				hijacker, ok := w.(http.Hijacker)
				require.True(t, ok)
				conn, buffered, err := hijacker.Hijack()
				require.NoError(t, err)
				defer conn.Close()
				frame := cyberDeltaEvent("partial work before a transport reset")
				// Leave the chunked body unterminated so the client observes an
				// unexpected EOF rather than a clean end of stream.
				_, _ = fmt.Fprintf(buffered, "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\n\r\n%x\r\n%s\r\n", len(frame), frame)
				_ = buffered.Flush()
			},
		},
		{
			name: "eof_after_provisional_usage_and_deltas", stream: true, estimated: true,
			serve: func(t *testing.T, w http.ResponseWriter) {
				w.Header().Set("Content-Type", "text/event-stream")
				// Lifecycle counters are placeholders, not a receipt.
				_, _ = io.WriteString(w, "event: response.in_progress\ndata: {\"type\":\"response.in_progress\",\"response\":{\"id\":\"resp_cyber_partial\",\"status\":\"in_progress\",\"service_tier\":\"default\",\"output\":[],\"usage\":{\"input_tokens\":20,\"output_tokens\":0,\"total_tokens\":20}}}\n\n"+
					cyberDeltaEvent("partial work after a provisional usage snapshot"))
			},
		},
		{
			name: "transport_reset_after_complete_receipt", stream: true, exact: cyberPartialExactCharge,
			serve: func(t *testing.T, w http.ResponseWriter) {
				hijacker, ok := w.(http.Hijacker)
				require.True(t, ok)
				conn, buffered, err := hijacker.Hijack()
				require.NoError(t, err)
				defer conn.Close()
				frame := cyberDeltaEvent("ack") + cyberTerminalEvent("completed", cyberOutput("ack"), cyberPartialUsage)
				// The authoritative terminal receipt arrived; only [DONE] is lost.
				_, _ = fmt.Fprintf(buffered, "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\n\r\n%x\r\n%s\r\n", len(frame), frame)
				_ = buffered.Flush()
			},
		},
		{
			name: "failed_without_usage_after_deltas", stream: true, estimated: true,
			serve: func(t *testing.T, w http.ResponseWriter) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, cyberDeltaEvent("partial work before failure")+
					cyberTerminalEvent("failed", "[]", "")+"data: [DONE]\n\n")
			},
		},
		{
			name: "failed_with_complete_standard_usage_stream", stream: true, exact: cyberPartialExactCharge,
			serve: func(t *testing.T, w http.ResponseWriter) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, cyberDeltaEvent("ack")+cyberTerminalEvent("failed", "[]", cyberPartialUsage)+"data: [DONE]\n\n")
			},
		},
		{
			name: "failed_with_complete_standard_usage_json", exact: cyberPartialExactCharge,
			serve: func(t *testing.T, w http.ResponseWriter) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"resp_cyber_partial","object":"response","model":"gpt-6-cyber","status":"failed","service_tier":"default","output":[],"usage":`+
					cyberPartialUsage+`,"error":{"type":"server_error","message":"generation failed after usage"}}`)
			},
		},
		{
			name: "oversized_json_receipt_with_measured_usage", estimated: true,
			serve: func(t *testing.T, w http.ResponseWriter) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"resp_cyber_partial","object":"response","model":"gpt-6-cyber","status":"completed","service_tier":"default","output":`+
					cyberOutput("ack")+`,"padding":"`+hugeText+`","usage":`+cyberPartialUsage+`}`)
			},
		},
		{
			name: "oversized_delta_then_complete_receipt", stream: true, exact: cyberPartialExactCharge,
			serve: func(t *testing.T, w http.ResponseWriter) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, cyberDeltaEvent(hugeText)+cyberTerminalEvent("completed", cyberOutput("ack"), cyberPartialUsage)+"data: [DONE]\n\n")
			},
		},
		{
			name: "oversized_terminal_receipt", stream: true, estimated: true,
			serve: func(t *testing.T, w http.ResponseWriter) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, cyberDeltaEvent("ack")+cyberTerminalEvent("completed", cyberOutput(hugeText), cyberPartialUsage)+"data: [DONE]\n\n")
			},
		},
	}
	for _, protocol := range []string{"chat", "responses", "messages"} {
		for _, sc := range scenarios {
			t.Run(protocol+"/"+sc.name, func(t *testing.T) {
				runCyberPartialScenario(t, protocol, sc)
			})
		}
	}
}

// runCyberPartialScenario executes one scenario and asserts its settlement.
// Parameters: t owns fixtures, protocol selects the client API, and sc
// describes the upstream delivery. Nothing is returned.
func runCyberPartialScenario(t *testing.T, protocol string, sc cyberPartialScenario) {
	const balance = int64(100_000)
	xaiVideoSetup(t, balance, false)
	var calls atomic.Int32
	reservations := make(chan int64, 1)
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, bufio.NewReader(r.Body))
		var user model.User
		if err := model.DB.First(&user, fallbackUserID).Error; err == nil {
			reservations <- balance - user.Quota
		} else {
			reservations <- -1
		}
		sc.serve(t, w)
	}))
	t.Cleanup(upstream.Close)
	previous := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = previous })

	path, body := lyriaProtocolRequest(protocol, sc.stream)
	c, _, requestID := protocolContext(t, channeltype.OpenAI, "gpt-6-cyber", path, body, upstream.URL, balance, 1, false, nil)
	clearNativeSettledLedger(t, requestID)
	if protocol == "responses" {
		c.Set(ctxkey.BaseURL, "https://api.openai.com")
		c.Set(ctxkey.Config, model.ChannelConfig{EndpointURLs: map[string]string{
			channeltype.RelayModeToEndpointName(relaymode.ResponseAPI): upstream.URL + "/v1/responses",
		}})
	}
	apiErr := relayLyriaProtocol(c, protocol)
	drainCriticalTasks(t)
	require.EqualValues(t, 1, calls.Load())
	reserved := <-reservations
	require.Greater(t, reserved, cyberPartialExactCharge, "the fixture must reserve more than the exact charge")

	charge := balance - reloadUserQuota(t)
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&logs).Error)
	require.Len(t, logs, 1, "provider work must settle exactly once (apiErr=%v)", describeCyberErr(apiErr))
	require.EqualValues(t, charge, logs[0].Quota)
	reason, _ := logs[0].Metadata["billing_estimate_reason"].(string)
	if sc.exact > 0 {
		require.Equal(t, sc.exact, charge, "a complete Standard receipt settles exactly")
		require.Empty(t, reason)
	} else {
		require.GreaterOrEqual(t, charge, reserved, "unobservable provider work must retain at least the reservation")
	}
	if sc.estimated {
		require.NotEmpty(t, reason, "an incomplete receipt must be recorded as an estimate")
	}
	require.False(t, BillingAllowsRetry(c), "provider work must not be replayed and billed twice")
}

// describeCyberErr renders an optional relay error for assertion messages.
// Parameters: apiErr may be nil. Returns: a short description.
func describeCyberErr(apiErr *relaymodel.ErrorWithStatusCode) string {
	if apiErr == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%d %s", apiErr.StatusCode, apiErr.Error.Message)
}
