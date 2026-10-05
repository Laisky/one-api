package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// providerImageFixture creates a bounded inline PNG for provider admission and wire-format tests.
func providerImageFixture(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1024, 1024))))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// TestClaudeImageDetailAdmissionAndConversion rejects invalid hints and keeps valid base64 semantics identical in quoting and provider conversion.
func TestClaudeImageDetailAdmissionAndConversion(t *testing.T) {
	for _, detail := range []string{"low", "high", "auto", "original", "invalid-detail"} {
		t.Run(detail, func(t *testing.T) {
			payload, err := json.Marshal(map[string]any{"model": "gpt-4o", "max_tokens": 16, "messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": providerImageFixture(t), "detail": detail}}}}}})
			require.NoError(t, err)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(payload))
			c.Request.Header.Set("Content-Type", "application/json")
			request, err := getAndValidateClaudeMessagesRequest(c)
			if detail == "invalid-detail" {
				require.Error(t, err)
				require.Nil(t, request)
				return
			}
			require.NoError(t, err)
			converted, err := openai_compatible.ConvertClaudeRequest(c, request)
			require.NoError(t, err)
			chat := converted.(*relaymodel.GeneralOpenAIRequest)
			require.Equal(t, detail, chat.Messages[0].ParseContent()[0].ImageURL.Detail, "the outbound image must retain the same detail used for admission")
			require.Positive(t, requireClaudePromptTokens(t, context.Background(), request))
		})
	}
}

// TestGeminiImageDetailAdmissionHTTP proves ignored low detail cannot admit work above a finite budget and funded requests settle authoritative receipts once.
func TestGeminiImageDetailAdmissionHTTP(t *testing.T) {
	for _, balance := range []int64{500, 10000} {
		name := "underfunded"
		if balance > 500 {
			name = "funded"
		}
		t.Run(name, func(t *testing.T) {
			xaiVideoSetup(t, balance, false)
			oldPre := config.PreConsumedQuota
			config.PreConsumedQuota = 0
			t.Cleanup(func() { config.PreConsumedQuota = oldPre })
			encoded := providerImageFixture(t)
			var calls atomic.Int32
			observed := make(chan []byte, 1)
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var body json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode provider fixture body: %v", err)
					return
				}
				observed <- body
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":20,"candidatesTokenCount":10,"totalTokenCount":30}}`)); err != nil {
					t.Errorf("write provider receipt: %v", err)
				}
			}))
			defer upstream.Close()
			old := client.HTTPClient
			client.HTTPClient = upstream.Client()
			t.Cleanup(func() { client.HTTPClient = old })
			payload := `{"model":"alias","max_tokens":16,"messages":[{"role":"user","content":[{"type":"text","text":"hello"},{"type":"image_url","image_url":{"url":"data:image/png;base64,` + encoded + `","detail":"low"}}]}]}`
			c, _, id := protocolContext(t, channeltype.Gemini, "gemini-2.5-flash", "/v1/chat/completions", payload, upstream.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
			apiErr := RelayTextHelper(c)
			drainCriticalTasks(t)
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			if balance == 500 {
				require.NotNil(t, apiErr, "low detail is not forwarded to Gemini and cannot reduce admission")
				require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
				require.Zero(t, calls.Load())
				require.Equal(t, balance, reloadUserQuota(t))
				require.Equal(t, balance, token.RemainQuota)
				require.Zero(t, token.UsedQuota)
				return
			}
			require.Nil(t, apiErr)
			require.EqualValues(t, 1, calls.Load())
			body := <-observed
			require.Contains(t, string(body), encoded)
			require.False(t, strings.Contains(string(body), `"detail"`))
			require.EqualValues(t, 30, requestCostQuota(t, id))
			require.Equal(t, balance-30, reloadUserQuota(t))
			require.Equal(t, balance-30, token.RemainQuota)
			require.EqualValues(t, 30, token.UsedQuota)
		})
	}
}

// TestSecurityClaudeToolResultImagesAreAdmitted proves images and text nested in
// tool results, which are forwarded upstream, are part of the admission quote.
func TestSecurityClaudeToolResultImagesAreAdmitted(t *testing.T) {
	ctx := context.Background()
	inline := map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": providerImageFixture(t)}}
	file := map[string]any{"type": "image", "source": map[string]any{"type": "file", "file_id": "file-fixture"}}
	quote := func(model string, content any) int {
		return requireClaudePromptTokens(t, ctx, &ClaudeMessagesRequest{Model: model, MaxTokens: 16, Messages: []relaymodel.ClaudeMessage{{Role: "user", Content: content}}})
	}
	toolResult := func(blocks ...any) []any {
		return []any{map[string]any{"type": "tool_result", "tool_use_id": "toolu_fixture", "content": blocks}}
	}
	empty := quote("gpt-4o", toolResult())
	require.GreaterOrEqual(t, quote("gpt-4o", toolResult(inline))-empty, 765, "a nested inline image must be quoted like a top-level one")
	require.GreaterOrEqual(t, quote("gpt-4o", toolResult(file))-empty, claudeFileImageFallbackTokens, "a nested file image must keep its allowance")
	require.Greater(t, quote("gpt-4o", toolResult(map[string]any{"type": "text", "text": strings.Repeat("hello world ", 200)}))-empty, 300, "nested tool-result text is forwarded and billable")
	// A URL source is forwarded by URL even when it also carries a stray media_type.
	urlSource := map[string]any{"type": "image", "source": map[string]any{"type": "url", "url": "data:image/png;base64," + providerImageFixture(t), "media_type": "image/png"}}
	require.GreaterOrEqual(t, quote("gpt-4o", []any{urlSource})-quote("gpt-4o", []any{}), 765, "a forwarded URL image must be quoted")

	// Sonnet 5.5 keeps exactly one network-free allowance per image wherever it is nested.
	top := quote("claude-sonnet-5-5", []any{inline})
	nested := quote("claude-sonnet-5-5", toolResult(inline))
	require.InDelta(t, top, nested, 8)
	require.Less(t, nested, 2*4784)
	require.Greater(t, nested, 4784)
}

// TestSecurityOpenAIPatchLowDetailAdmissionHTTP proves gpt-5.4 low detail, which
// OpenAI processes with a larger patch budget than high, cannot admit an image
// for its legacy flat base cost; the funded request forwards the same hint.
func TestSecurityOpenAIPatchLowDetailAdmissionHTTP(t *testing.T) {
	for _, balance := range []int64{500, 10000} {
		name := "underfunded"
		if balance > 500 {
			name = "funded"
		}
		t.Run(name, func(t *testing.T) {
			xaiVideoSetup(t, balance, false)
			oldPre := config.PreConsumedQuota
			config.PreConsumedQuota = 0
			t.Cleanup(func() { config.PreConsumedQuota = oldPre })
			encoded := providerImageFixture(t)
			var calls atomic.Int32
			observed := make(chan []byte, 1)
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var body json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode provider fixture body: %v", err)
					return
				}
				observed <- body
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(`{"id":"chatcmpl-fixture","object":"chat.completion","created":1,"model":"gpt-5.4","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":10,"total_tokens":30}}`)); err != nil {
					t.Errorf("write provider receipt: %v", err)
				}
			}))
			defer upstream.Close()
			old := client.HTTPClient
			client.HTTPClient = upstream.Client()
			t.Cleanup(func() { client.HTTPClient = old })
			payload := `{"model":"alias","max_tokens":16,"messages":[{"role":"user","content":[{"type":"text","text":"hello"},{"type":"image_url","image_url":{"url":"data:image/png;base64,` + encoded + `","detail":"low"}}]}]}`
			c, _, id := protocolContext(t, channeltype.OpenAICompatible, "gpt-5.4", "/v1/chat/completions", payload, upstream.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
			apiErr := RelayTextHelper(c)
			drainCriticalTasks(t)
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			if balance == 500 {
				require.NotNil(t, apiErr, "gpt-5.4 low detail keeps a 6,144-patch budget and cannot be admitted at a flat base cost")
				require.Equal(t, http.StatusForbidden, apiErr.StatusCode)
				require.Zero(t, calls.Load())
				require.Equal(t, balance, reloadUserQuota(t))
				require.Equal(t, balance, token.RemainQuota)
				require.Zero(t, token.UsedQuota)
				return
			}
			require.Nil(t, apiErr)
			require.EqualValues(t, 1, calls.Load())
			body := <-observed
			require.Contains(t, string(body), encoded)
			require.Contains(t, string(body), `"detail":"low"`, "the provider must receive the hint used for admission")
			require.EqualValues(t, 30, requestCostQuota(t, id))
			require.Equal(t, balance-30, reloadUserQuota(t))
			require.Equal(t, balance-30, token.RemainQuota)
			require.EqualValues(t, 30, token.UsedQuota)
		})
	}
}
