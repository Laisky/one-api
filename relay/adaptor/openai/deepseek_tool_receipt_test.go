package openai

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// deepseekReceiptJSON is an unsolicited OpenAI-shaped item, not a verified
// DeepSeek successful Search/Fetch receipt. Unknown wire fields stay unchanged.
const deepseekReceiptJSON = `{"id":"resp_receipt","object":"response","status":"completed","model":"deepseek-v4-pro","vendor_extension":{"precise":9007199254740993},"output":[{"id":"ws1","type":"web_search_call","status":"completed","action":{"type":"search","query":"synthetic"},"unknown_field":"kept"}],"usage":{"input_tokens":12,"output_tokens":3,"total_tokens":15,"input_tokens_details":{"cached_tokens":4,"web_search":{"requests":9}}}}`

// TestDeepSeekToolReceiptJSON keeps native token/cache and wire evidence while
// refusing to promote generic items or usage extensions into paid-tool counts.
func TestDeepSeekToolReceiptJSON(t *testing.T) {
	for _, tc := range []struct {
		name, base    string
		channel, want int
	}{
		{"dedicated", "https://example.invalid", channeltype.DeepSeek, 0},
		{"official_compatible_host", "https://api.deepseek.com", channeltype.OpenAI, 0},
		{"third_party_same_model", "https://provider.example/v1", channeltype.OpenAI, 1},
		{"ordinary_openai", "https://api.openai.com", channeltype.OpenAI, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, w := newDirectStreamContext()
			c.Set(ctxkey.Channel, tc.channel)
			c.Set(ctxkey.BaseURL, tc.base)
			metalib.Set2Context(c, &metalib.Meta{ChannelType: tc.channel, BaseURL: tc.base, ActualModelName: "deepseek-v4-pro"})
			response := fakeUpstream(deepseekReceiptJSON)
			response.Header.Set("Content-Type", "application/json")
			apiErr, usage := ResponseAPIDirectHandler(c, response, 12, "deepseek-v4-pro")
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			require.Equal(t, 12, usage.PromptTokens)
			require.Equal(t, 3, usage.CompletionTokens)
			require.NotNil(t, usage.PromptTokensDetails)
			require.Equal(t, 4, usage.PromptTokensDetails.CachedTokens)
			require.Equal(t, tc.want, c.GetInt(ctxkey.WebSearchCallCount))
			require.Equal(t, deepseekReceiptJSON, w.Body.String(), "suppress billing inference, never the original wire fields")
			require.Zero(t, usage.ToolsCost)
		})
	}
}

// TestDeepSeekToolReceiptStream preserves actual terminal token usage and
// duplicate-event wire delivery without turning progress/final items into prices.
func TestDeepSeekToolReceiptStream(t *testing.T) {
	for _, channel := range []int{channeltype.DeepSeek, channeltype.OpenAI} {
		t.Run(fmt.Sprint(channel), func(t *testing.T) {
			c, w := newDirectStreamContext()
			c.Set(ctxkey.Channel, channel)
			base := "https://api.openai.com"
			if channel == channeltype.DeepSeek {
				base = "https://api.deepseek.com"
			}
			c.Set(ctxkey.BaseURL, base)
			metalib.Set2Context(c, &metalib.Meta{ChannelType: channel, BaseURL: base, ActualModelName: "deepseek-v4-pro"})
			item := `{"type":"response.output_item.added","output_index":0,"item":{"id":"ws1","type":"web_search_call","status":"in_progress","action":{"type":"search","query":"synthetic"}}}`
			terminal := `{"type":"response.completed","response":` + deepseekReceiptJSON + "}"
			stream := "event: response.output_item.added\ndata: " + item + "\n\n" +
				"event: response.output_item.added\ndata: " + item + "\n\n" +
				"event: response.completed\ndata: " + terminal + "\n\n" +
				"event: response.completed\ndata: " + terminal + "\n\n"
			apiErr, _, usage := ResponseAPIDirectStreamHandler(c, fakeUpstream(stream), relaymode.ResponseAPI)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			require.Equal(t, 12, usage.PromptTokens)
			require.Equal(t, 3, usage.CompletionTokens)
			require.NotNil(t, usage.PromptTokensDetails)
			require.Equal(t, 4, usage.PromptTokensDetails.CachedTokens)
			want := 1
			if channel == channeltype.DeepSeek {
				want = 0
			}
			require.Equal(t, want, c.GetInt(ctxkey.WebSearchCallCount))
			require.Contains(t, w.Body.String(), `"unknown_field":"kept"`)
			require.Contains(t, w.Body.String(), "9007199254740993")
			require.Equal(t, 2, strings.Count(w.Body.String(), "event: response.completed"))
			require.Zero(t, usage.ToolsCost)
		})
	}
}

// TestDeepSeekToolReceiptUsageOnly covers the separate generic usage-extension
// fallback when no output item is available, across JSON and terminal SSE.
func TestDeepSeekToolReceiptUsageOnly(t *testing.T) {
	const body = `{"id":"usage_only","object":"response","status":"completed","output":[],"usage":{"input_tokens":12,"output_tokens":3,"total_tokens":15,"input_tokens_details":{"cached_tokens":4,"web_search":{"requests":9}}}}`
	for _, deepseek := range []bool{true, false} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("deepseek=%t/stream=%t", deepseek, stream), func(t *testing.T) {
				c, w := newDirectStreamContext()
				channel := channeltype.OpenAI
				base := "https://api.openai.com"
				if deepseek {
					channel = channeltype.DeepSeek
					base = "https://api.deepseek.com"
				}
				c.Set(ctxkey.Channel, channel)
				c.Set(ctxkey.BaseURL, base)
				metalib.Set2Context(c, &metalib.Meta{ChannelType: channel, BaseURL: base, ActualModelName: "deepseek-v4-pro"})
				payload := body
				if stream {
					payload = "event: response.completed\ndata: " + `{"type":"response.completed","response":` + body + "}\n\n"
				}
				if stream {
					apiErr, _, usage := ResponseAPIDirectStreamHandler(c, fakeUpstream(payload), relaymode.ResponseAPI)
					require.Nil(t, apiErr)
					require.Equal(t, 12, usage.PromptTokens)
					require.Equal(t, 3, usage.CompletionTokens)
				} else {
					apiErr, usage := ResponseAPIDirectHandler(c, fakeUpstream(payload), 12, "deepseek-v4-pro")
					require.Nil(t, apiErr)
					require.Equal(t, 12, usage.PromptTokens)
					require.Equal(t, 3, usage.CompletionTokens)
					require.Equal(t, body, w.Body.String())
				}
				want := 9
				if deepseek {
					want = 0
				}
				require.Equal(t, want, c.GetInt(ctxkey.WebSearchCallCount))
				require.Contains(t, w.Body.String(), `"requests":9`, "unknown usage is preserved on the wire")
			})
		}
	}
}

// TestDeepSeekToolReceiptConverted covers the same inference boundary when an
// OpenAI Responses body is converted for a Chat-format downstream client.
func TestDeepSeekToolReceiptConverted(t *testing.T) {
	for _, deepseek := range []bool{true, false} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("deepseek=%t/stream=%t", deepseek, stream), func(t *testing.T) {
				c, _ := newDirectStreamContext()
				channel := channeltype.OpenAI
				base := "https://api.openai.com"
				if deepseek {
					channel = channeltype.DeepSeek
					base = "https://api.deepseek.com"
				}
				c.Set(ctxkey.Channel, channel)
				c.Set(ctxkey.BaseURL, base)
				metalib.Set2Context(c, &metalib.Meta{ChannelType: channel, BaseURL: base, ActualModelName: "deepseek-v4-pro"})
				if stream {
					payload := "event: response.completed\ndata: " + `{"type":"response.completed","response":` + deepseekReceiptJSON + "}\n\n"
					apiErr, _, usage := ResponseAPIStreamHandler(c, fakeUpstream(payload), relaymode.ChatCompletions)
					require.Nil(t, apiErr)
					require.Equal(t, 12, usage.PromptTokens)
					require.Equal(t, 3, usage.CompletionTokens)
				} else {
					apiErr, usage := ResponseAPIHandler(c, fakeUpstream(deepseekReceiptJSON), 12, "deepseek-v4-pro")
					require.Nil(t, apiErr)
					require.Equal(t, 12, usage.PromptTokens)
					require.Equal(t, 3, usage.CompletionTokens)
				}
				want := 1
				if deepseek {
					want = 0
				}
				require.Equal(t, want, c.GetInt(ctxkey.WebSearchCallCount))
			})
		}
	}
}

// TestDeepSeekToolReceiptPreview prevents an OpenAI-only implicit-model pricing
// fallback from applying to an actual DeepSeek API compatibility endpoint.
func TestDeepSeekToolReceiptPreview(t *testing.T) {
	for _, deepseek := range []bool{true, false} {
		c, _ := newDirectStreamContext()
		base := "https://api.openai.com"
		if deepseek {
			base = "https://api.deepseek.com"
		}
		meta := &metalib.Meta{ChannelType: channeltype.OpenAI, BaseURL: base, Mode: relaymode.ChatCompletions, ActualModelName: "gpt-4o-search-preview"}
		recordWebSearchPreviewInvocation(c, meta)
		counts := normalizeToolInvocationCounts(c)
		want := 1
		if deepseek {
			want = 0
		}
		require.Equal(t, want, counts["web_search_preview_non_reasoning"])
	}
}

// TestDeepSeekToolReceiptEndpointCounters binds publication to the planned
// current endpoint and leaves both the original wire and token usage unchanged.
func TestDeepSeekToolReceiptEndpointCounters(t *testing.T) {
	for _, tc := range []struct {
		name, base, override, observed string
		channel, want                  int
	}{
		{"planned_deepseek", "https://api.openai.com", "https://api.deepseek.com/v1/responses", "https://api.openai.com/v1/responses", channeltype.OpenAI, 0},
		{"reverse_base", "https://api.deepseek.com", "https://api.openai.com/v1/responses", "https://api.deepseek.com/v1/responses", channeltype.OpenAI, 1},
		{"reverse_channel", "https://api.deepseek.com", "https://api.openai.com/v1/responses", "", channeltype.DeepSeek, 1},
		{"stale_observed", "https://api.openai.com", "", "https://api.deepseek.com/v1/responses", channeltype.OpenAI, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, w := newDirectStreamContext()
			info := &metalib.Meta{ChannelType: tc.channel, BaseURL: tc.base, Mode: relaymode.ResponseAPI, UpstreamRequestURL: tc.observed, Config: model.ChannelConfig{EndpointURLs: map[string]string{channeltype.RelayModeToEndpointName(relaymode.ResponseAPI): tc.override}}}
			metalib.Set2Context(c, info)
			apiErr, usage := ResponseAPIDirectHandler(c, fakeUpstream(deepseekReceiptJSON), 12, "deepseek-v4-pro")
			require.Nil(t, apiErr)
			require.Equal(t, tc.want, c.GetInt(ctxkey.WebSearchCallCount))
			require.Equal(t, 12, usage.PromptTokens)
			require.Equal(t, 3, usage.CompletionTokens)
			require.Equal(t, 4, usage.PromptTokensDetails.CachedTokens)
			require.Equal(t, deepseekReceiptJSON, w.Body.String())
		})
	}
}
