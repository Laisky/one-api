package openai

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGPT6CyberProviderReceiptPreservation exercises all production response
// renderings with completed and failed receipts. Parameter t asserts; none return.
func TestGPT6CyberProviderReceiptPreservation(t *testing.T) {
	for _, mode := range []int{relaymode.ChatCompletions, relaymode.ResponseAPI, relaymode.ClaudeMessages} {
		for _, stream := range []bool{false, true} {
			for _, failed := range []bool{false, true} {
				t.Run(fmt.Sprintf("%d/stream=%t/failed=%t", mode, stream, failed), func(t *testing.T) {
					c, _ := newDirectStreamContext()
					info := &metalib.Meta{ChannelType: channeltype.OpenAI, ActualModelName: "gpt-6-cyber",
						Mode: mode, IsStream: stream, PromptTokens: 10}
					metalib.Set2Context(c, info)
					c.Set(ctxkey.ConvertedRequest, &ResponseAPIRequest{Model: "gpt-6-cyber"})
					if mode == relaymode.ClaudeMessages {
						c.Set(ctxkey.ClaudeMessagesConversion, true)
					}
					status, failure := "completed", ""
					if failed {
						status, failure = "failed", `,"error":{"type":"server_error","message":"work failed after usage"}`
					}
					body := `{"id":"resp_cyber","object":"response","model":"gpt-6-cyber","status":"` + status +
						`","service_tier":"fast","output":[],"usage":{"input_tokens":300000,"output_tokens":10,"total_tokens":300010,"input_tokens_details":{"cached_tokens":1000,"cache_write_tokens":2000,"cache_write_tokens_details":{"ttl_30m_tokens":1000,"ttl_12h_tokens":1000}}}` + failure + `}`
					if stream {
						body = "event: response." + status + "\ndata: {\"type\":\"response." + status + "\",\"response\":" + body + "}\n\ndata: [DONE]\n\n"
					}
					response := fakeUpstream(body)
					if !stream {
						response.Header.Set("Content-Type", "application/json")
					}
					usage, apiErr := (&Adaptor{ChannelType: channeltype.OpenAI}).DoResponse(c, response, info)
					if failed && !stream && mode != relaymode.ClaudeMessages {
						require.NotNil(t, apiErr, "paid error receipts must retain their original error")
					}
					if mode == relaymode.ClaudeMessages && !stream {
						require.Nil(t, usage, "the controller must still write the converted Claude body")
						stored, ok := c.Get(ctxkey.ConvertedResponseUsage)
						require.True(t, ok)
						usage, ok = stored.(*model.Usage)
						require.True(t, ok)
					}
					require.NotNil(t, usage, "paid failed responses must not lose their receipt")
					require.Equal(t, "fast", usage.ServiceTier, "the envelope tier drives the Standard-only billing guard")
					require.Equal(t, 300000, usage.PromptTokens)
					require.Equal(t, 10, usage.CompletionTokens)
					require.Equal(t, 1000, usage.CacheWrite5mTokens)
					require.Equal(t, 1000, usage.CacheWrite12hTokens)
					if mode == relaymode.ClaudeMessages {
						stored, ok := c.Get(ctxkey.ConvertedResponseUsage)
						require.True(t, ok)
						receipt, ok := stored.(*model.Usage)
						require.True(t, ok)
						require.NotNil(t, receipt)
						require.Equal(t, "fast", receipt.ServiceTier)
					}
				})
			}
		}
	}
}

// TestGPT6CyberReceiptScopeAndEmptyError preserves the existing response behavior
// for other hosts/models and unpaid errors. Parameter t asserts; none return.
func TestGPT6CyberReceiptScopeAndEmptyError(t *testing.T) {
	for _, tc := range []struct {
		name, actual string
		channel      int
		errorBody    bool
	}{
		{name: "other_native_model", actual: "gpt-4o", channel: channeltype.OpenAI},
		{name: "other_host", actual: "gpt-6-cyber", channel: channeltype.OpenAICompatible},
		{name: "unpaid_error", actual: "gpt-6-cyber", channel: channeltype.OpenAI, errorBody: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newDirectStreamContext()
			info := &metalib.Meta{ChannelType: tc.channel, ActualModelName: tc.actual, Mode: relaymode.ResponseAPI, PromptTokens: 10}
			metalib.Set2Context(c, info)
			body := `{"id":"resp_scope","object":"response","model":"` + tc.actual + `","service_tier":"fast","output":[],"usage":{"input_tokens":20,"output_tokens":2,"total_tokens":22}}`
			if tc.errorBody {
				body = `{"error":{"type":"invalid_request_error","message":"rejected before generation"},"service_tier":"fast"}`
			}
			response := fakeUpstream(body)
			response.Header.Set("Content-Type", "application/json")
			usage, apiErr := (&Adaptor{ChannelType: tc.channel}).DoResponse(c, response, info)
			if tc.errorBody {
				require.NotNil(t, apiErr)
				require.Nil(t, usage, "a tier alone must not turn an unpaid error into a charge")
				return
			}
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			require.Empty(t, usage.ServiceTier, "unrelated host/model behavior must stay unchanged")
			require.Empty(t, usage.BillingEstimateReason)
			require.Equal(t, 22, usage.TotalTokens)
		})
	}
}

// TestGPT6CyberReceiptCaptureLimit keeps large envelope bytes out of token
// billing while marking the missing envelope evidence. Parameter t asserts;
// none return, and the native handler's measured counts must remain intact.
func TestGPT6CyberReceiptCaptureLimit(t *testing.T) {
	c, _ := newDirectStreamContext()
	info := &metalib.Meta{ChannelType: channeltype.OpenAI, ActualModelName: "gpt-6-cyber", Mode: relaymode.ResponseAPI}
	metalib.Set2Context(c, info)
	body := `{"id":"resp_large","object":"response","model":"gpt-6-cyber","padding":"` + strings.Repeat("x", 2<<20) +
		`","service_tier":"fast","output":[],"usage":{"input_tokens":20,"output_tokens":2,"total_tokens":22}}`
	response := fakeUpstream(body)
	response.Header.Set("Content-Type", "application/json")
	usage, apiErr := (&Adaptor{ChannelType: channeltype.OpenAI}).DoResponse(c, response, info)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 20, usage.PromptTokens)
	require.Equal(t, 2, usage.CompletionTokens)
	require.Equal(t, "response_usage_capture_limit", usage.BillingEstimateReason)
}
