package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/baidu"
	"github.com/Laisky/one-api/relay/adaptor/xai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// decodeImage26ControlRequest parses body through the production image request
// decoder. Parameters: t fails the test on decode errors and body is a JSON
// generation request. Returns: the gin context and the normalized request.
func decodeImage26ControlRequest(t *testing.T, body string) (*gin.Context, *relaymodel.ImageRequest) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	request, err := getImageRequest(c, relaymode.ImagesGenerations)
	require.NoError(t, err)
	return c, request
}

// TestImageRequestControlsScopedToOpenAIUpstreams verifies the GPT Image 2.6
// control contract is enforced for OpenAI and Azure only; OpenAI-compatible
// third parties keep their own contracts and are never rejected by it.
func TestImageRequestControlsScopedToOpenAIUpstreams(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body, code string
	}{
		{"complexity_on_gpt_image_25", `{"model":"gpt-image-2.5-sunburst","prompt":"p","complexity":"low"}`, "invalid_value"},
		{"complexity_unknown_value", `{"model":"gpt-image-2.6-flare","prompt":"p","complexity":"max"}`, "invalid_value"},
		{"svg_output", `{"model":"gpt-image-2.6-flare","prompt":"p","output_format":"svg"}`, "unsupported_output_format"},
		{"gif_output", `{"model":"gpt-image-1","prompt":"p","size":"1024x1024","output_format":"gif"}`, "unsupported_output_format"},
		{"unqualified_output_format", `{"model":"gpt-image-2.6-sunburst","prompt":"p","output_format":"avif"}`, "unsupported_output_format"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, channelType := range []int{channeltype.OpenAI, channeltype.Azure} {
				_, request := decodeImage26ControlRequest(t, tc.body)
				got := validateImageRequest(request, &metalib.Meta{ChannelType: channelType}, nil)
				require.NotNil(t, got, "channel type %d must enforce the OpenAI image contract", channelType)
				require.Equal(t, http.StatusBadRequest, got.StatusCode)
				require.Equal(t, tc.code, got.Code)
			}
			for _, channelType := range []int{channeltype.OpenAICompatible, channeltype.OpenRouter, channeltype.XAI, channeltype.Custom} {
				_, request := decodeImage26ControlRequest(t, tc.body)
				require.Nil(t, validateImageRequest(request, &metalib.Meta{ChannelType: channelType}, nil),
					"channel type %d has its own image contract", channelType)
			}
			_, request := decodeImage26ControlRequest(t, tc.body)
			require.Nil(t, validateImageRequest(request, nil, nil), "a missing upstream identity cannot select the OpenAI contract")
		})
	}
}

// TestImageRequestControlsDoNotLeakToXAIOrBaidu verifies converters that
// reuse the shared DTO strip OpenAI-only controls from their upstream payloads
// while the billing snapshot keeps the client's values untouched.
func TestImageRequestControlsDoNotLeakToXAIOrBaidu(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, model string
		convert     func(*gin.Context, *relaymodel.ImageRequest) (any, error)
	}{
		{"xai", "grok-imagine-image-2.0", (&xai.Adaptor{}).ConvertImageRequest},
		{"baidu", "stable-diffusion-xl", (&baidu.Adaptor{}).ConvertImageRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, request := decodeImage26ControlRequest(t, `{"model":"`+tc.model+`","prompt":"p","complexity":"low","safety_identifier":"sid-7f3a","output_format":"svg","user":"legacy-user"}`)
			before, err := json.Marshal(request)
			require.NoError(t, err)
			for _, key := range []string{`"complexity":"low"`, `"safety_identifier":"sid-7f3a"`, `"output_format":"svg"`} {
				require.Contains(t, string(before), key, "the shared request must decode %s", key)
			}

			converted, err := convertImageRequestForUpstream(c, request, tc.convert)
			require.NoError(t, err)
			payload, err := json.Marshal(converted)
			require.NoError(t, err)
			var wire map[string]any
			require.NoError(t, json.Unmarshal(payload, &wire))
			for _, key := range []string{"complexity", "safety_identifier", "output_format"} {
				require.NotContains(t, wire, key, "%s is an OpenAI-only image control", key)
			}
			require.Equal(t, tc.model, wire["model"])

			after, err := json.Marshal(request)
			require.NoError(t, err)
			require.Equal(t, string(before), string(after), "provider conversion must not mutate the billing snapshot")
		})
	}
}

// TestBuildOpenAIImageRequestControlScope verifies safety_identifier reaches
// every OpenAI image model while complexity and output_format stay GPT
// Image-only, and that requests without controls serialize as before.
func TestBuildOpenAIImageRequestControlScope(t *testing.T) {
	t.Parallel()
	controls := func(model string) *relaymodel.ImageRequest {
		return &relaymodel.ImageRequest{
			Model: model, Prompt: "p", N: 1, Size: "1024x1024", Quality: "high", User: "legacy-user",
			OutputFormat: "webp", Complexity: "low", SafetyIdentifier: "sid-7f3a",
		}
	}
	for _, tc := range []struct {
		name string
		req  *relaymodel.ImageRequest
		want string
	}{
		{"gpt_image_26", controls("gpt-image-2.6-sunburst"),
			`{"model":"gpt-image-2.6-sunburst","prompt":"p","n":1,"size":"1024x1024","quality":"high","user":"legacy-user","output_format":"webp","complexity":"low","safety_identifier":"sid-7f3a"}`},
		{"chatgpt_image", controls("chatgpt-image-latest"),
			`{"model":"chatgpt-image-latest","prompt":"p","n":1,"size":"1024x1024","quality":"high","user":"legacy-user","output_format":"webp","complexity":"low","safety_identifier":"sid-7f3a"}`},
		{"dalle_drops_gpt_image_fields", controls("dall-e-2"),
			`{"model":"dall-e-2","prompt":"p","n":1,"size":"1024x1024","quality":"high","user":"legacy-user","safety_identifier":"sid-7f3a"}`},
		{"no_controls_unchanged", &relaymodel.ImageRequest{Model: "gpt-image-1", Prompt: "p", N: 1, Size: "1024x1536", Quality: "high"},
			`{"model":"gpt-image-1","prompt":"p","n":1,"size":"1024x1536","quality":"high"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			payload, err := json.Marshal(buildOpenAIImageRequest(tc.req))
			require.NoError(t, err)
			require.Equal(t, tc.want, string(payload))
		})
	}
}

// TestGetImageRequestMultipartBindsOpenAIControls verifies multipart edit
// bodies decode the OpenAI controls the gateway validates before forwarding.
func TestGetImageRequestMultipartBindsOpenAIControls(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	body, contentType := image26ControlMultipart(t, [][2]string{
		{"model", "gpt-image-2.6-flare"},
		{"prompt", "Add a red door"},
		{"complexity", "high"},
		{"safety_identifier", "sid-7f3a"},
		{"output_format", "jpeg"},
	})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", contentType)
	request, err := getImageRequest(c, relaymode.ImagesEdits)
	require.NoError(t, err)
	require.True(t, request.IsEdit)
	require.Equal(t, "high", request.Complexity)
	require.Equal(t, "sid-7f3a", request.SafetyIdentifier)
	require.Equal(t, "jpeg", request.OutputFormat)
}

// TestIsGPTImage26Model verifies complexity is scoped to the exact published
// GPT Image 2.6 IDs and never inherited by 2.5 models or guessed aliases.
func TestIsGPTImage26Model(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"gpt-image-2.6-sunburst", "gpt-image-2.6-flare"} {
		require.True(t, isGPTImage26Model(name), name)
	}
	for _, name := range []string{
		"gpt-image-2.5-sunburst", "gpt-image-2.5-flare-2026-09-08", "gpt-image-2", "gpt-image-1",
		"gpt-image-2.6", "gpt-image-2.6-flare-2026-11-04", "GPT-IMAGE-2.6-FLARE", "dall-e-3",
	} {
		require.False(t, isGPTImage26Model(name), name)
	}
}
