package openai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
)

// TestParseErrorUsageReceipt covers every receipt carrier accepted from an
// OpenAI-wire error body and every body that must prove no billable work.
func TestParseErrorUsageReceipt(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                       string
		body                       string
		prompt, completion, cached int
		tier                       string
	}{
		{name: "chat_root_usage", body: `{"error":{"message":"boom"},"usage":{"prompt_tokens":120,"completion_tokens":30,"total_tokens":150,"prompt_tokens_details":{"cached_tokens":20}}}`, prompt: 120, completion: 30, cached: 20},
		{name: "responses_object", body: `{"id":"resp_1","object":"response","status":"failed","service_tier":"default","error":{"code":"server_error"},"usage":{"input_tokens":90,"output_tokens":10,"total_tokens":100}}`, prompt: 90, completion: 10, tier: "default"},
		{name: "responses_envelope", body: `{"type":"response.failed","response":{"id":"resp_2","status":"failed","service_tier":"priority","usage":{"input_tokens":7,"output_tokens":3,"total_tokens":10}}}`, prompt: 7, completion: 3, tier: "priority"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usage := ParseErrorUsageReceipt([]byte(tc.body))
			require.NotNil(t, usage)
			require.Equal(t, tc.prompt, usage.PromptTokens)
			require.Equal(t, tc.completion, usage.CompletionTokens)
			require.Equal(t, tc.prompt+tc.completion, usage.TotalTokens)
			require.Equal(t, tc.tier, usage.ServiceTier, "the actual tier must reach the pricing guard")
			if tc.cached > 0 {
				require.NotNil(t, usage.PromptTokensDetails)
				require.Equal(t, tc.cached, usage.PromptTokensDetails.CachedTokens)
			}
		})
	}

	oversized := `{"usage":{"prompt_tokens":1,"completion_tokens":1},"pad":"` + strings.Repeat("x", adaptor.MaxPaidErrorReceiptBytes) + `"}`
	for name, body := range map[string]string{
		"plain_error":      `{"error":{"message":"boom","type":"server_error"}}`,
		"null_usage":       `{"error":{"message":"boom"},"usage":null}`,
		"scalar_usage":     `{"error":{"message":"boom"},"usage":42}`,
		"zero_counters":    `{"error":{"message":"boom"},"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`,
		"null_envelope":    `{"response":{"status":"failed","usage":null}}`,
		"invalid_json":     `{"usage":{"prompt_tokens":5`,
		"empty":            ``,
		"json_array":       `[{"usage":{"prompt_tokens":5}}]`,
		"oversized_body":   oversized,
		"nested_elsewhere": `{"error":{"usage":{"prompt_tokens":5,"completion_tokens":5}}}`,
	} {
		require.Nil(t, ParseErrorUsageReceipt([]byte(body)), name)
	}
}

// TestPaidErrorReceiptScope proves the receipt contract is trusted only for
// native OpenAI and Azure OpenAI channels and only for non-2xx statuses.
func TestPaidErrorReceiptScope(t *testing.T) {
	t.Parallel()
	body := []byte(`{"error":{"message":"boom"},"usage":{"prompt_tokens":10,"completion_tokens":5}}`)
	a := &Adaptor{}
	for _, channel := range []int{channeltype.OpenAI, channeltype.Azure} {
		usage := a.PaidErrorReceipt(nil, &meta.Meta{ChannelType: channel}, http.StatusInternalServerError, body)
		require.NotNil(t, usage, channeltype.IdToName(channel))
		require.Equal(t, 10, usage.PromptTokens)
		require.Nil(t, a.PaidErrorReceipt(nil, &meta.Meta{ChannelType: channel}, http.StatusOK, body), "2xx receipts belong to DoResponse")
	}
	for _, channel := range []int{channeltype.DeepSeek, channeltype.OpenRouter, channeltype.Groq, channeltype.Custom} {
		require.Nil(t, a.PaidErrorReceipt(nil, &meta.Meta{ChannelType: channel}, http.StatusInternalServerError, body), channeltype.IdToName(channel))
	}
	require.Nil(t, a.PaidErrorReceipt(nil, nil, http.StatusInternalServerError, body))
	var reader adaptor.PaidErrorReceiptReader = a
	require.NotNil(t, reader)
}

// TestPaidErrorReceiptRecordsWebSearchCalls proves a paid error records the
// web searches its failed Responses object or envelope lists, exactly once per
// distinct call, and that an error without a receipt records none.
func TestPaidErrorReceiptRecordsWebSearchCalls(t *testing.T) {
	t.Parallel()
	const output = `"output":[{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"a"}},{"type":"web_search_call","id":"ws_2","status":"completed","action":{"type":"search","query":"b"}},{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"a"}}]`
	for name, tc := range map[string]struct {
		body string
		want int
	}{
		"root_object": {`{"object":"response","status":"failed",` + output + `,"usage":{"input_tokens":5,"output_tokens":1}}`, 2},
		"envelope":    {`{"type":"response.failed","response":{"status":"failed",` + output + `,"usage":{"input_tokens":5,"output_tokens":1}}}`, 2},
		"no_receipt":  {`{"error":{"message":"boom"},` + output + `}`, 0},
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		usage := (&Adaptor{}).PaidErrorReceipt(c, &meta.Meta{ChannelType: channeltype.OpenAI}, http.StatusInternalServerError, []byte(tc.body))
		require.Equal(t, tc.want > 0, usage != nil, name)
		require.Equal(t, tc.want, c.GetInt(ctxkey.WebSearchCallCount), name)
	}
}
