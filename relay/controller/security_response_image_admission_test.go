package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// responseImageRequest builds a native Responses request carrying one text part and one image part.
func responseImageRequest(model string, image map[string]any) *openai.ResponseAPIRequest {
	image["type"] = "input_image"
	return &openai.ResponseAPIRequest{
		Model: model,
		Input: openai.ResponseAPIInput{map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "input_text", "text": "hello"},
				image,
			},
		}},
	}
}

// TestSecurityResponseImageEstimateNeverFree proves native Responses images keep a
// nonzero conservative allowance when the gateway cannot measure them locally.
func TestSecurityResponseImageEstimateNeverFree(t *testing.T) {
	ctx := context.Background()
	measured := getResponseAPIPromptTokens(ctx, responseImageRequest("gpt-4o", map[string]any{"image_url": "data:image/png;base64," + providerImageFixture(t), "detail": "high"}))
	require.Greater(t, measured, 765, "fixture must measure a 1024x1024 high-detail image plus its text")

	for name, image := range map[string]map[string]any{
		"file_id":        {"file_id": "file-fixture", "detail": "high"},
		"malformed_data": {"image_url": "data:image/png;base64,AAAA", "detail": "high"},
		"unknown_detail": {"image_url": "data:image/png;base64," + providerImageFixture(t), "detail": "unsupported"},
	} {
		t.Run(name, func(t *testing.T) {
			quoted := getResponseAPIPromptTokens(ctx, responseImageRequest("gpt-4o", image))
			require.GreaterOrEqual(t, quoted, measured, "an unmeasured image must not be admitted below a measured one")
		})
	}
}

// TestSecurityResponseFileImageAdmissionHTTP proves a file-backed image forwarded
// on the native Responses path cannot be admitted for free above a finite budget,
// while a funded request forwards the same image and settles its receipt once.
func TestSecurityResponseFileImageAdmissionHTTP(t *testing.T) {
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
			var calls atomic.Int32
			observed := make(chan map[string]any, 1)
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode provider fixture body: %v", err)
					return
				}
				observed <- body
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(`{"id":"resp_fixture","object":"response","created_at":1,"status":"completed","model":"gpt-4o","output":[{"type":"message","id":"msg_fixture","status":"completed","role":"assistant","content":[{"type":"output_text","text":"ok","annotations":[]}]}],"usage":{"input_tokens":20,"output_tokens":10,"total_tokens":30}}`)); err != nil {
					t.Errorf("write provider receipt: %v", err)
				}
			}))
			defer upstream.Close()
			old := client.HTTPClient
			client.HTTPClient = upstream.Client()
			t.Cleanup(func() { client.HTTPClient = old })
			payload := `{"model":"alias","max_output_tokens":16,"input":[{"role":"user","content":[{"type":"input_text","text":"hello"},{"type":"input_image","file_id":"file-fixture","detail":"high"}]}]}`
			c, _, id := protocolContext(t, channeltype.OpenAICompatible, "gpt-4o", "/v1/responses", payload, upstream.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
			c.Set(ctxkey.Config, model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse})
			require.True(t, supportsNativeResponseAPI(metalib.GetByContext(c)))
			apiErr := RelayResponseAPIHelper(c)
			drainCriticalTasks(t)
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			if balance == 500 {
				require.NotNil(t, apiErr, "a file-backed image the gateway cannot measure must not be admitted for free")
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
			input, ok := body["input"].([]any)
			require.True(t, ok)
			content := input[0].(map[string]any)["content"].([]any)
			require.Equal(t, "file-fixture", content[1].(map[string]any)["file_id"], "the provider must receive the same image that was admitted")
			require.EqualValues(t, 30, requestCostQuota(t, id))
			require.Equal(t, balance-30, reloadUserQuota(t))
			require.Equal(t, balance-30, token.RemainQuota)
			require.EqualValues(t, 30, token.UsedQuota)
		})
	}
}
