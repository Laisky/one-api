package openai

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestSecurityResponseProvisionalEOF distinguishes clean transport EOF from a
// provider's authoritative terminal receipt on both actual adaptor paths.
func TestSecurityResponseProvisionalEOF(t *testing.T) {
	for _, converted := range []bool{false, true} {
		for _, terminal := range []bool{false, true} {
			name := "native"
			if converted {
				name = "converted"
			}
			if terminal {
				name += "/terminal_control"
			} else {
				name += "/provisional_eof"
			}
			t.Run(name, func(t *testing.T) {
				c, _ := newDirectStreamContext()
				kind, status := "response.in_progress", "in_progress"
				if terminal {
					kind, status = "response.completed", "completed"
				}
				payload := `data: {"type":"` + kind + `","response":{"id":"resp_provisional","object":"response","status":"` + status + `","output":[],"usage":{"input_tokens":11,"output_tokens":0,"total_tokens":11}}}` + "\n\n"
				if terminal {
					payload += "data: [DONE]\n\n"
				}
				metadata := &metalib.Meta{Mode: relaymode.ResponseAPI, IsStream: true, ActualModelName: "gpt-4o", PromptTokens: 11}
				if converted {
					metadata.Mode = relaymode.ChatCompletions
					c.Set(ctxkey.ConvertedRequest, &ResponseAPIRequest{})
				}
				resp := fakeUpstream(payload)
				resp.Header.Set("Content-Type", "text/event-stream")
				usage, apiErr := (&Adaptor{}).DoResponse(c, resp, metadata)
				require.Nil(t, apiErr, "clean EOF need not fabricate an upstream I/O error")
				require.NotNil(t, usage)
				require.Equal(t, 11, usage.PromptTokens)
				if terminal {
					require.Empty(t, usage.BillingEstimateReason, "authoritative measured zero output remains valid")
				} else {
					require.NotEmpty(t, usage.BillingEstimateReason, "a provisional receipt cannot release the held allowance as final usage")
				}
			})
		}
	}
}

// TestSecurityResponseLegacyFinalReceipt keeps accepted status-less full-response
// envelopes compatible instead of silently relabelling measured usage as unknown.
func TestSecurityResponseLegacyFinalReceipt(t *testing.T) {
	c, _ := newDirectStreamContext()
	resp := fakeUpstream(strings.Join([]string{`data: {"id":"resp_legacy","object":"response","output":[],"usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18}}`, "", "data: [DONE]", ""}, "\n"))
	usage, apiErr := (&Adaptor{}).DoResponse(c, resp, &metalib.Meta{Mode: relaymode.ResponseAPI, IsStream: true, ActualModelName: "gpt-4o", PromptTokens: 11})
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 18, usage.TotalTokens)
	require.Empty(t, usage.BillingEstimateReason)
}
