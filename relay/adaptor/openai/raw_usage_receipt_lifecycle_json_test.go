package openai

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// rawReceiptJSONResponse wraps body as a 200 JSON provider response.
// Parameters: body is the provider body. Returns: the response.
func rawReceiptJSONResponse(body io.ReadCloser) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: body,
		Header: http.Header{"Content-Type": {"application/json"}}}
}

// rawReceiptSettledUsage returns the usage the controller settles for a
// non-stream attempt: DoResponse's result, or for a converted Claude body the
// receipt published beside the deferred rendering. Parameters: t asserts, c is
// the request context, mode the client mode and usage the DoResponse result.
// Returns: the settlement usage, which may be nil.
func rawReceiptSettledUsage(t *testing.T, c *gin.Context, mode int, usage *model.Usage) *model.Usage {
	t.Helper()
	if mode != relaymode.ClaudeMessages {
		return usage
	}
	stored, ok := c.Get(ctxkey.ConvertedResponseUsage)
	if !ok {
		return usage
	}
	receipt, _ := stored.(*model.Usage)
	return receipt
}

// TestRawUsageReceiptTruncatedJSONEnvelope cuts a native Cyber JSON response
// mid-envelope, after its usage object was already sent, by an unexpected EOF,
// a TCP reset, or a clean EOF that leaves the JSON unparsable. No client format
// may settle counters parsed from, or estimated from the bytes of, an envelope
// the gateway never received whole: DoResponse returns no usage, so the
// controller's conservative path retains the forwarded attempt's reservation.
// A transport failure is reported as an error; for Claude clients the
// unparsable body is passed through unchanged with no published receipt.
func TestRawUsageReceiptTruncatedJSONEnvelope(t *testing.T) {
	prefix := `{"id":"resp_truncated","object":"response","model":"gpt-6-cyber","status":"completed","service_tier":"default",` +
		`"usage":{"input_tokens":20,"output_tokens":2,"total_tokens":22},"output":[{"type":"message","id":"msg_truncated",` +
		`"role":"assistant","content":[{"type":"output_text","text":"` + strings.Repeat("partial ", 2048)
	for _, cut := range []struct {
		name string
		err  func() error
	}{
		{name: "unexpected_eof", err: func() error { return io.ErrUnexpectedEOF }},
		{name: "connection_reset", err: rawReceiptConnReset},
		{name: "clean_eof", err: func() error { return io.EOF }},
	} {
		for _, mode := range rawReceiptModes {
			t.Run(cut.name+"/"+mode.name, func(t *testing.T) {
				c, w, info := rawReceiptContext(mode.mode, false)
				body := newRawReceiptScriptedBody(prefix, 4096, cut.err())
				usage, apiErr := (&Adaptor{ChannelType: channeltype.OpenAI}).DoResponse(c, rawReceiptJSONResponse(body), info)
				require.Nil(t, usage, "a truncated envelope proves no counters and must not be byte-estimated")
				require.Nil(t, rawReceiptSettledUsage(t, c, mode.mode, usage), "no receipt may be published for a truncated envelope")
				if cut.name != "clean_eof" {
					require.NotNil(t, apiErr, "a provider transport failure must be reported")
					require.Equal(t, "read_response_body_failed", apiErr.Error.Code)
					require.Empty(t, w.Body.String(), "nothing may be delivered from an unread envelope")
					return
				}
				if mode.mode == relaymode.ClaudeMessages {
					require.Nil(t, apiErr, "the unparsable body is handed to the controller unchanged")
					converted, ok := c.Get(ctxkey.ConvertedResponse)
					require.True(t, ok)
					rendered, ok := converted.(*http.Response)
					require.True(t, ok)
					payload, err := io.ReadAll(rendered.Body)
					require.NoError(t, err)
					require.Equal(t, prefix, string(payload))
					return
				}
				require.NotNil(t, apiErr, "an unparsable envelope must not be rendered as success")
				require.Equal(t, "unmarshal_response_body_failed", apiErr.Error.Code)
			})
		}
	}
}

// TestRawUsageReceiptJSONCaptureLimit sends native Cyber JSON envelopes larger
// than the accumulator's capture limit through every client format. The
// observer cannot parse such an envelope, so the settlement usage must be
// flagged response_usage_capture_limit (keeping the reservation as the floor)
// while its counters are exactly the rendering handler's measured ones: the
// envelope's JSON bytes are never billed as tokens. The Chat and Responses
// handlers fall back to the admitted prompt estimate when usage is absent; the
// Claude converter measures nothing, and the controller's zero-usage path then
// retains the reservation. A failed oversized envelope settles a zero-counter
// flagged receipt beside its error so it is retained, not refunded; the Claude
// converter renders that envelope itself and publishes its measured receipt.
func TestRawUsageReceiptJSONCaptureLimit(t *testing.T) {
	padding := strings.Repeat("x", 2<<20+4096)
	head := `{"id":"resp_large","object":"response","model":"gpt-6-cyber","service_tier":"default","metadata":{"padding":"` + padding + `"},`
	measured := head + `"status":"completed","output":[{"type":"message","id":"msg_large","role":"assistant","content":[{"type":"output_text","text":"ack"}]}],"usage":` + rawReceiptCompleteUsage + `}`
	unmeasured := head + `"status":"completed","output":[]}`
	failed := head + `"status":"failed","output":[],"usage":` + rawReceiptCompleteUsage +
		`,"error":{"type":"server_error","code":"server_error","message":"generation failed after usage"}}`
	type counters struct{ prompt, completion int }
	for _, tc := range []struct {
		name, body string
		// native is the settlement of the Responses and Chat renderings; claude
		// is the receipt published beside the converted Claude body.
		native, claude counters
		// nativeErr reports that the Responses and Chat handlers return the
		// provider failure beside the retained receipt.
		nativeErr bool
	}{
		{name: "measured_usage", body: measured, native: counters{20, 2}, claude: counters{20, 2}},
		{name: "no_usage_non_text_padding", body: unmeasured, native: counters{rawReceiptPromptTokens, 0}, claude: counters{0, 0}},
		{name: "failed_with_usage", body: failed, native: counters{0, 0}, claude: counters{20, 2}, nativeErr: true},
	} {
		for _, mode := range rawReceiptModes {
			t.Run(tc.name+"/"+mode.name, func(t *testing.T) {
				c, _, info := rawReceiptContext(mode.mode, false)
				body := newRawReceiptScriptedBody(tc.body, 64<<10, io.EOF)
				usage, apiErr := (&Adaptor{ChannelType: channeltype.OpenAI}).DoResponse(c, rawReceiptJSONResponse(body), info)
				settled := rawReceiptSettledUsage(t, c, mode.mode, usage)
				want := tc.native
				switch {
				case mode.mode == relaymode.ClaudeMessages:
					want = tc.claude
					require.Nil(t, apiErr)
					require.Nil(t, usage, "the controller must still render the converted Claude body")
				case tc.nativeErr:
					require.NotNil(t, apiErr, "the provider failure must still reach the client")
				default:
					require.Nil(t, apiErr)
				}
				require.NotNil(t, settled, "an oversized paid envelope must be retained, not refunded")
				require.Equal(t, want.prompt, settled.PromptTokens, "envelope bytes must never become prompt tokens")
				require.Equal(t, want.completion, settled.CompletionTokens, "envelope bytes must never become completion tokens")
				require.Equal(t, want.prompt+want.completion, settled.TotalTokens)
				// The unparsed envelope's service tier is unknown; the flagged
				// estimate keeps the reservation floor an unpriced tier would.
				require.Equal(t, "response_usage_capture_limit", settled.BillingEstimateReason)
			})
		}
	}
}
