package router

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// e2eOversizedPadding is reasoning ciphertext larger than the 2 MiB window in
// which relay/model.ResponseUsageAccumulator retains a raw receipt, so the
// receipt fields of the envelope that carries it arrive after that window.
var e2eOversizedPadding = strings.Repeat("Q", (2<<20)+(256<<10))

// e2eOversizedReasoningItem renders a completed reasoning item whose
// encrypted_content exceeds the receipt capture window, as OpenAI returns when
// a client includes reasoning.encrypted_content.
func e2eOversizedReasoningItem() string {
	return `{"type":"reasoning","id":"rs_e2e","summary":[],"encrypted_content":"` + e2eOversizedPadding + `"}`
}

// e2eOversizedResponsesJSON renders a completed Responses envelope answering
// "ack" after an oversized reasoning item. Parameters: modelName is echoed,
// tier is the actual service_tier and u the receipt. Returns: the JSON body.
// Like OpenAI, service_tier and usage follow the output array.
func e2eOversizedResponsesJSON(modelName, tier string, u e2eUsage) string {
	message := `{"type":"message","id":"msg_e2e","status":"completed","role":"assistant","content":[{"type":"output_text","text":"ack","annotations":[]}]}`
	return `{"id":"resp_e2e","object":"response","created_at":1767225600,"status":"completed","model":"` + modelName +
		`","error":null,"output":[` + e2eOversizedReasoningItem() + `,` + message + `],"service_tier":"` + tier +
		`","usage":` + e2eResponsesUsageJSON(u) + `}`
}

// e2eOversizedResponsesSSE renders a Responses stream whose reasoning item and
// response.completed events each exceed the capture window. The lifecycle
// events report the requested Standard tier; only response.completed carries
// the actual tier, as OpenAI does. Parameters match e2eOversizedResponsesJSON.
func e2eOversizedResponsesSSE(modelName, tier string, u e2eUsage) string {
	created := `{"id":"resp_e2e","object":"response","created_at":1767225600,"status":"in_progress","model":"` + modelName +
		`","service_tier":"default","output":[],"usage":null}`
	message := `{"type":"message","id":"msg_e2e","status":"in_progress","role":"assistant","content":[]}`
	doneMessage := `{"type":"message","id":"msg_e2e","status":"completed","role":"assistant","content":[{"type":"output_text","text":"ack","annotations":[]}]}`
	events := []string{
		`{"type":"response.created","sequence_number":0,"response":` + created + `}`,
		`{"type":"response.in_progress","sequence_number":1,"response":` + created + `}`,
		`{"type":"response.output_item.added","sequence_number":2,"output_index":0,"item":{"type":"reasoning","id":"rs_e2e","summary":[]}}`,
		`{"type":"response.output_item.done","sequence_number":3,"output_index":0,"item":` + e2eOversizedReasoningItem() + `}`,
		`{"type":"response.output_item.added","sequence_number":4,"output_index":1,"item":` + message + `}`,
		`{"type":"response.content_part.added","sequence_number":5,"item_id":"msg_e2e","output_index":1,"content_index":0,"part":{"type":"output_text","text":"","annotations":[]}}`,
		`{"type":"response.output_text.delta","sequence_number":6,"item_id":"msg_e2e","output_index":1,"content_index":0,"delta":"ack"}`,
		`{"type":"response.output_text.done","sequence_number":7,"item_id":"msg_e2e","output_index":1,"content_index":0,"text":"ack"}`,
		`{"type":"response.content_part.done","sequence_number":8,"item_id":"msg_e2e","output_index":1,"content_index":0,"part":{"type":"output_text","text":"ack","annotations":[]}}`,
		`{"type":"response.output_item.done","sequence_number":9,"output_index":1,"item":` + doneMessage + `}`,
		`{"type":"response.completed","sequence_number":10,"response":` + e2eOversizedResponsesJSON(modelName, tier, u) + `}`,
	}
	var b strings.Builder
	for _, event := range events {
		var probe struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal([]byte(event), &probe)
		b.WriteString("event: " + probe.Type + "\ndata: " + event + "\n\n")
	}
	return b.String()
}

// TestE2ECyberOversizedReceiptSettlement drives gpt-6-cyber responses whose
// receipt fields arrive after the 2 MiB raw-receipt capture window through
// every client protocol and delivery mode. The receipt must still be read in
// full: a Standard receipt settles exactly instead of retaining the admission
// reservation, and an unpriced actual tier reported only by the final event is
// still recognised and flagged instead of being settled as Standard.
func TestE2ECyberOversizedReceiptSettlement(t *testing.T) {
	for _, tc := range []struct {
		name   string
		tier   string
		usage  e2eUsage
		charge int64
		// unpriced marks a receipt whose actual tier has no catalog price.
		unpriced bool
	}{
		// 20 x 10 + 2 x 50 = 300, below the admission reservation.
		{name: "standard_small", tier: "default", usage: e2eUsage{Input: 20, Output: 2}, charge: 300},
		// 16,000 x 10 + 4,000 x 1 + 4,000 x 50 = 364,000 at Standard prices.
		{name: "priority_unpriced", tier: "priority", usage: e2eUsage{Input: 20_000, Cached: 4_000, Output: 4_000}, charge: 364_000, unpriced: true},
	} {
		for _, protocol := range e2eProtocols {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", tc.name, protocol, stream), func(t *testing.T) {
					env := newE2EEnv(t, func(call e2eCall) e2eReply {
						if call.Stream() {
							return e2eReply{ContentType: "text/event-stream", Body: e2eOversizedResponsesSSE(e2eCyberModel, tc.tier, tc.usage)}
						}
						return e2eReply{Body: e2eOversizedResponsesJSON(e2eCyberModel, tc.tier, tc.usage)}
					}, e2eNativeChannel(939531, e2eNativeKey, 0, e2eCyberModel, ""))
					before := env.ledger(t)
					path, body := e2eClientRequest(protocol, e2eCyberModel, stream, "")

					res := env.post(t, path, body)

					e2eRequireSuccessWire(t, protocol, stream, res)
					calls := env.upstream.callsFor(e2eNativeKey)
					require.Len(t, calls, 1, "exactly one upstream dispatch")
					require.Equal(t, "/v1/responses", calls[0].Path)
					reserved := before.UserQuota - calls[0].UserQuota
					require.Positive(t, reserved, "admission must reserve before dispatch")
					if !tc.unpriced {
						require.Greater(t, reserved, tc.charge, "the control receipt must be cheaper than the reservation")
						env.requireSettled(t, res, before, tc.charge, false)
						return
					}
					row := env.requireSettled(t, res, before, max(reserved, tc.charge), true)
					require.Equal(t, "unpriced_usage_retained_reservation", e2eEstimateReason(row),
						"the actual tier must be recognised as unpriced, not lost with the envelope")
				})
			}
		}
	}
}

// TestE2EOversizedReceiptSettlesExactly drives a general native OpenAI model
// whose Responses envelope exceeds the 2 MiB raw-receipt capture window through
// every client protocol and delivery mode. The receipt carries far more work
// than the admission reservation, so a lost receipt would under-bill; it must
// settle exactly on every route, including the Claude Messages stream
// converter, whose only usage source is the bounded accumulator.
func TestE2EOversizedReceiptSettlesExactly(t *testing.T) {
	// 100,000 x 0.075 + 10,000 x 0.3 = 7,500 + 3,000 = 10,500.
	usage, charge := e2eUsage{Input: 100_000, Output: 10_000}, int64(10_500)
	for _, protocol := range e2eProtocols {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", protocol, stream), func(t *testing.T) {
				env := newE2EEnv(t, func(call e2eCall) e2eReply {
					if e2eUpstreamSurface(call.Path) != "responses" {
						return e2eSuccess(call, e2eMiniModel, "default", usage)
					}
					if call.Stream() {
						return e2eReply{ContentType: "text/event-stream", Body: e2eOversizedResponsesSSE(e2eMiniModel, "default", usage)}
					}
					return e2eReply{Body: e2eOversizedResponsesJSON(e2eMiniModel, "default", usage)}
				}, e2eNativeChannel(939541, e2eNativeKey, 0, e2eMiniModel, ""))
				before := env.ledger(t)
				path, body := e2eClientRequest(protocol, e2eMiniModel, stream, "")

				res := env.post(t, path, body)

				e2eRequireSuccessWire(t, protocol, stream, res)
				calls := env.upstream.callsFor(e2eNativeKey)
				require.Len(t, calls, 1, "exactly one upstream dispatch")
				reserved := before.UserQuota - calls[0].UserQuota
				require.Less(t, reserved, charge, "the receipt must exceed the reservation so a lost receipt under-bills")
				t.Logf("upstream path %s", calls[0].Path)
				env.requireSettled(t, res, before, charge, false)
			})
		}
	}
}
