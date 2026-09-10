package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/deepseek"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestDeepSeekReviewNativeRouting exercises the production route decision after
// model mapping, including aliases and third-party-host negative controls.
func TestDeepSeekReviewNativeRouting(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp", "deepseek-v4-pro"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, channel := range []int{channeltype.DeepSeek, channeltype.OpenAI, channeltype.OpenAICompatible} {
				t.Run("channel-"+strconv.Itoa(channel), func(t *testing.T) {
					meta := &metalib.Meta{ChannelType: channel, BaseURL: "https://api.deepseek.com", OriginModelName: "my-model", ActualModelName: name}
					require.True(t, supportsNativeResponseAPI(meta), "mapped model must retain native Responses routing")
				})
			}
			t.Run("origin-only", func(t *testing.T) {
				meta := &metalib.Meta{ChannelType: channeltype.DeepSeek, OriginModelName: name}
				require.True(t, supportsNativeResponseAPI(meta), "origin name must work when actual name is absent")
			})
			t.Run("third-party", func(t *testing.T) {
				thirdParty := &metalib.Meta{ChannelType: channeltype.OpenAI, BaseURL: "https://third-party.example/v1", ActualModelName: name}
				require.False(t, supportsNativeResponseAPI(thirdParty), "a model name alone must not opt a third-party host into DeepSeek's contract")
			})
		})
	}
	require.False(t, supportsNativeResponseAPI(nil))
	require.False(t, supportsNativeResponseAPI(&metalib.Meta{ChannelType: channeltype.DeepSeek, ActualModelName: "deepseek-chat"}))
}

// TestDeepSeekReviewResponsesImageReservation decodes real Responses payloads
// and checks the production pre-consume estimator for messages and tool outputs.
func TestDeepSeekReviewResponsesImageReservation(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp"} {
		for _, itemType := range []string{"message", "function_call_output", "custom_tool_call_output"} {
			t.Run(name+"/"+itemType, func(t *testing.T) {
				t.Parallel()
				text := map[string]any{"type": "input_text", "text": strings.Repeat("Describe the image and compare its visible details. ", 10)}
				count := func(parts []any) int {
					item := map[string]any{"type": itemType, "role": "user", "content": parts}
					if itemType != "message" {
						item = map[string]any{"type": itemType, "call_id": "call-image", "output": parts}
					}
					body, err := json.Marshal(map[string]any{"model": name, "input": []any{item}})
					require.NoError(t, err)
					var request openai.ResponseAPIRequest
					require.NoError(t, json.Unmarshal(body, &request))
					return getResponseAPIPromptTokens(context.Background(), &request)
				}
				base := count([]any{text})
				images := []any{
					map[string]any{"type": "input_image", "image_url": "https://image.invalid/picture.png", "detail": "low"},
					map[string]any{"type": "input_image", "image_url": "data:image/png;base64,AAAA", "detail": "low"},
					map[string]any{"type": "input_image", "file_id": "file-api-image"},
					map[string]any{"type": "input_image", "file_data": "data:image/png;base64,AAAA"},
				}
				for index, image := range images {
					t.Run([]string{"url", "inline", "file_id", "file_data"}[index], func(t *testing.T) {
						require.Equal(t, base+1024, count([]any{text, image}), "each image source must reserve the current bound")
					})
				}
				t.Run("multiple", func(t *testing.T) {
					require.Equal(t, base+len(images)*1024, count(append([]any{text}, images...)), "each image must be counted once")
				})
				t.Run("empty", func(t *testing.T) {
					require.Equal(t, base, count([]any{text, map[string]any{"type": "input_image"}}), "an empty image source must not reserve tokens")
				})
			})
		}
	}
}

// TestDeepSeekReviewJSONReasoningAndSampling is the negative reproduction for
// the claim that ultra never reaches the adaptor. It also guards top_p forwarding.
func TestDeepSeekReviewJSONReasoningAndSampling(t *testing.T) {
	for _, effort := range []string{"minimal", "ultra"} {
		t.Run(effort, func(t *testing.T) {
			body := `{"model":"deepseek-flash","messages":[{"role":"user","content":"hello"}],"reasoning_effort":"` + effort + `","thinking":{"type":"enabled"},"top_p":0.97}`
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			request, err := getAndValidateTextRequest(c, relaymode.ChatCompletions)
			require.NoError(t, err)
			sanitizeChatCompletionRequest(request)
			converted, err := (&deepseek.Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, request)
			require.NoError(t, err)
			encoded, err := json.Marshal(converted)
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(encoded, &payload))
			want := "max"
			if effort == "minimal" {
				want = "low"
			}
			require.Equal(t, want, payload["reasoning_effort"])
			require.Equal(t, 0.97, payload["top_p"], "the gateway must not discard the provider's supported sampling control")
		})
	}
}

// TestDeepSeekReviewExplicitJSONBinding proves ultra must also survive the
// validating JSON boundary, rather than depending on unvalidated JSON decoding.
func TestDeepSeekReviewExplicitJSONBinding(t *testing.T) {
	for _, effort := range []string{"minimal", "ultra"} {
		t.Run(effort, func(t *testing.T) {
			body := `{"model":"deepseek-flash","messages":[{"role":"user","content":"hello"}],"reasoning_effort":"` + effort + `"}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			var request relaymodel.GeneralOpenAIRequest
			require.NoError(t, binding.JSON.Bind(req, &request))
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			converted, err := (&deepseek.Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, &request)
			require.NoError(t, err)
			want := "max"
			if effort == "minimal" {
				want = "low"
			}
			require.Equal(t, want, *converted.(*relaymodel.GeneralOpenAIRequest).ReasoningEffort)
		})
	}
}
