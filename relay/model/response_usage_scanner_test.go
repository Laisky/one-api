package model

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// oversizedPadding outgrows the accumulator's capture window on its own.
var oversizedPadding = strings.Repeat("p", responseUsageCaptureLimit+(64<<10))

// receiptView is the billing-relevant projection of a settled usage.
type receiptView struct {
	Prompt, Completion, Cached         int
	Write5m, Write1h, Write12h         int
	Tier, Fingerprint, Reason          string
	Unverified                         bool
	ReasoningTokens, CompletionDetails int
}

// viewOf projects usage onto receiptView. Parameter u is the settled usage.
// Returns: the projection.
func viewOf(u *Usage) receiptView {
	view := receiptView{Prompt: u.PromptTokens, Completion: u.CompletionTokens,
		Write5m: u.CacheWrite5mTokens, Write1h: u.CacheWrite1hTokens, Write12h: u.CacheWrite12hTokens,
		Tier: u.ServiceTier, Fingerprint: u.SystemFingerprint, Reason: u.BillingEstimateReason, Unverified: u.ServiceTierUnverified}
	if u.PromptTokensDetails != nil {
		view.Cached = u.PromptTokensDetails.CachedTokens
	}
	if u.CompletionTokensDetails != nil {
		view.ReasoningTokens = u.CompletionTokensDetails.ReasoningTokens
		view.CompletionDetails = 1
	}
	return view
}

// observeInChunks feeds body to a fresh accumulator in chunk-sized writes, as
// a network body arrives. Parameters: stream selects SSE framing, body is the
// response and chunk the write size. Returns: the finished usage.
func observeInChunks(stream bool, body string, chunk int) *Usage {
	acc := NewResponseUsageAccumulator(stream)
	for start := 0; start < len(body); start += chunk {
		acc.Observe([]byte(body[start:min(len(body), start+chunk)]))
	}
	return acc.Finish(0, nil)
}

// TestResponseUsageOversizedEnvelopeDifferential proves that a receipt read
// past the capture window equals the receipt of the same envelope without the
// bytes that made it large, for every protocol shape the accumulator reads.
// Each fixture inserts the padding member before the receipt members, where
// providers render the output that makes a response large.
func TestResponseUsageOversizedEnvelopeDifferential(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stream bool
		// body renders the envelope; pad is inserted as a non-receipt member.
		body func(pad string) string
	}{
		{name: "responses_json", body: func(pad string) string {
			return `{"id":"resp_1","object":"response","status":"completed","output":[{"type":"reasoning","encrypted_content":"` + pad +
				`"}],"service_tier":"priority","usage":{"input_tokens":1200,"input_tokens_details":{"cached_tokens":200,"cache_write_tokens":300,` +
				`"cache_write_tokens_details":{"ttl_30m_tokens":300,"ttl_12h_tokens":0}},"output_tokens":40,"output_tokens_details":{"reasoning_tokens":30},"total_tokens":1240}}`
		}},
		{name: "chat_json", body: func(pad string) string {
			return `{"id":"chatcmpl_1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"` + pad +
				`"}}],"service_tier":"default","system_fingerprint":"fp_1","usage":{"prompt_tokens":900,"completion_tokens":70,"total_tokens":970,` +
				`"prompt_tokens_details":{"cached_tokens":100}}}`
		}},
		{name: "claude_json", body: func(pad string) string {
			return `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"` + pad +
				`"}],"usage":{"input_tokens":50,"cache_read_input_tokens":400,"cache_creation_input_tokens":600,` +
				`"cache_creation":{"ephemeral_5m_input_tokens":100,"ephemeral_1h_input_tokens":500},"output_tokens":25}}`
		}},
		{name: "responses_sse_terminal", stream: true, body: func(pad string) string {
			created := `{"type":"response.created","response":{"id":"resp_1","status":"in_progress","service_tier":"default","output":[],"usage":null}}`
			completed := `{"type":"response.completed","response":{"id":"resp_1","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"` + pad +
				`"}]}],"service_tier":"flex","usage":{"input_tokens":700,"output_tokens":60,"total_tokens":760}}}`
			return "event: response.created\ndata: " + created + "\n\nevent: response.completed\ndata: " + completed + "\n\n"
		}},
		{name: "chat_sse_usage_chunk", stream: true, body: func(pad string) string {
			return "data: {\"id\":\"c1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"" + pad + "\"}}],\"service_tier\":\"default\"}\r\n\r\n" +
				"data: {\"id\":\"c1\",\"choices\":[],\"service_tier\":\"default\",\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15}}\r\n\r\n" +
				"data: [DONE]\r\n\r\n"
		}},
		{name: "claude_sse", stream: true, body: func(pad string) string {
			return "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"usage\":{\"input_tokens\":30,\"cache_read_input_tokens\":70,\"output_tokens\":1}}}\n\n" +
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"" + pad + "\"}}\n\n" +
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":45}}\n\n" +
				"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := viewOf(observeInChunks(tc.stream, tc.body("x"), 1<<20))
			require.Empty(t, want.Reason, "the control envelope carries a complete receipt")
			require.Positive(t, want.Prompt)
			for _, chunk := range []int{1 << 20, 4093, 1} {
				if chunk == 1 && len(tc.body(oversizedPadding)) > 3<<20 {
					// One-byte writes cover framing; keep them to the smaller fixtures.
					continue
				}
				got := viewOf(observeInChunks(tc.stream, tc.body(oversizedPadding), chunk))
				require.Equal(t, want, got, "chunk=%d: the oversized envelope must yield the same receipt", chunk)
			}
		})
	}
}

// TestResponseUsageOversizedEnvelopeUnfollowable keeps the capture-limit
// estimate for oversized envelopes the scanner cannot follow, and marks their
// actual service tier unverified so a tier-restricted tariff does not assume
// Standard. A later Responses terminal event supersedes a lost envelope.
func TestResponseUsageOversizedEnvelopeUnfollowable(t *testing.T) {
	usage := `"usage":{"input_tokens":20,"output_tokens":2,"total_tokens":22}`
	for _, tc := range []struct {
		name, body string
		stream     bool
		// reason is the expected estimate label and unverified the tier flag.
		reason     string
		unverified bool
	}{
		{name: "json_truncated", body: `{"object":"response","output":"` + oversizedPadding + `","service_tier":"default",` + usage,
			reason: "response_usage_capture_limit", unverified: true},
		{name: "json_mismatched_brackets", body: `{"object":"response","output":["` + oversizedPadding + `"},"service_tier":"default",` + usage + `}`,
			reason: "response_usage_capture_limit", unverified: true},
		{name: "json_trailing_value", body: `{"object":"response","output":"` + oversizedPadding + `","service_tier":"default",` + usage + `} {}`,
			reason: "response_usage_capture_limit", unverified: true},
		{name: "json_root_array", body: `[{"output":"` + oversizedPadding + `",` + usage + `}]`,
			reason: "response_usage_capture_limit", unverified: true},
		{name: "json_receipt_member_too_large", body: `{"output":"` + oversizedPadding + `","usage":{"input_tokens":20,"output_tokens":2,"note":"` +
			strings.Repeat("n", responseReceiptValueLimit) + `"}}`, reason: "response_usage_capture_limit", unverified: true},
		{name: "sse_terminal_cut_off", stream: true, body: "data: {\"type\":\"response.completed\",\"response\":{\"output\":\"" + oversizedPadding +
			"\",\"service_tier\":\"default\",", reason: "response_usage_capture_limit", unverified: true},
		{name: "sse_lost_delta_superseded_by_terminal", stream: true, body: "data: {\"type\":\"response.output_text.delta\",\"delta\":\"" + oversizedPadding + "\n\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"service_tier\":\"default\"," + usage + "}}\n\n"},
		{name: "json_without_usage_is_followed", body: `{"object":"response","output":"` + oversizedPadding + `","service_tier":"default"}`,
			reason: "response_usage_capture_limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := observeInChunks(tc.stream, tc.body, 64<<10)
			require.Equal(t, tc.reason, got.BillingEstimateReason)
			require.Equal(t, tc.unverified, got.ServiceTierUnverified)
			if tc.reason == "" {
				require.Equal(t, 20, got.PromptTokens)
				require.Equal(t, 2, got.CompletionTokens)
				require.Equal(t, "default", got.ServiceTier)
			}
		})
	}
}

// TestResponseReceiptScannerMemberSemantics pins the scanner's agreement with
// encoding/json map decoding, which the in-window path uses: escaped member
// names are unescaped, a later duplicate replaces an earlier one, members of
// non-receipt containers are ignored, and only receipt paths are retained.
func TestResponseReceiptScannerMemberSemantics(t *testing.T) {
	for _, tc := range []struct {
		name, doc, want string
	}{
		{name: "escaped_name_replaces_plain",
			doc:  `{"usage":{"input_tokens":1},"\u0075sage":{"input_tokens":3}}`,
			want: `{"usage":{"input_tokens":3}}`},
		{name: "plain_name_replaces_escaped",
			doc:  `{"servic\u0065_tier":"flex","service_tier":"auto"}`,
			want: `{"service_tier":"auto"}`},
		{name: "nested_and_ignored_members",
			doc: `{"output":[{"usage":{"input_tokens":99}}],"response":{"usage":{"input_tokens":7},"service_tier":"flex","output":"x"},` +
				`"message":"not an object","type":"response.completed","created":12,"usage":null}`,
			want: `{"response":{"usage":{"input_tokens":7},"service_tier":"flex"},"type":"response.completed","usage":null}`},
		{name: "escaped_quotes_and_brackets_in_strings",
			doc:  `{"output":"a\"}],{\\","usage":{"input_tokens":5,"note":"}\"{"},"service_tier":"default"}`,
			want: `{"usage":{"input_tokens":5,"note":"}\"{"},"service_tier":"default"}`},
		{name: "literal_members", doc: `{"type" : "x" , "system_fingerprint" :null}`, want: `{"type":"x","system_fingerprint":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var decoded map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(tc.doc), &decoded), "fixtures are valid JSON")
			scan := newResponseReceiptScanner()
			_, _ = scan.Write([]byte(tc.doc))
			receipt, ok := scan.Result()
			require.True(t, ok)
			require.JSONEq(t, tc.want, string(receipt))
			var got map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(receipt, &got))
			for key, raw := range got {
				if key == "response" {
					continue
				}
				require.JSONEq(t, string(decoded[key]), string(raw), "member %q must equal encoding/json's", key)
			}
		})
	}
}

// BenchmarkResponseReceiptScanner measures following a large string-dominated
// envelope, the shape of image and ciphertext payloads.
func BenchmarkResponseReceiptScanner(b *testing.B) {
	body := []byte(`{"output":[{"type":"image_generation_call","result":"` + strings.Repeat("A", 32<<20) +
		`"}],"service_tier":"default","usage":{"input_tokens":20,"output_tokens":2}}`)
	b.SetBytes(int64(len(body)))
	for b.Loop() {
		scan := newResponseReceiptScanner()
		_, _ = scan.Write(body)
		if _, ok := scan.Result(); !ok {
			b.Fatal("scan failed")
		}
	}
}
