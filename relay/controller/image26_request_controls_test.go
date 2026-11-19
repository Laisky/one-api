package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// image26ControlBalance is the starting user and token quota of every relay case.
const image26ControlBalance = int64(1_000_000)

// image26ControlRelayResult records one relay attempt together with the request
// the fake upstream observed, so assertions run on the test goroutine.
type image26ControlRelayResult struct {
	// Err is the client-facing relay error, or nil on success.
	Err *relaymodel.ErrorWithStatusCode
	// Calls counts upstream HTTP requests.
	Calls int32
	// Body is the exact upstream request body.
	Body []byte
	// ContentType is the upstream request Content-Type header.
	ContentType string
	// Path is the upstream request path.
	Path string
	// UserQuota and TokenQuota are the balances after the relay settles.
	UserQuota, TokenQuota int64
}

// image26ControlMultipart builds a multipart image edit body. Parameters: t
// fails the test on writer errors and fields are written in order before one
// PNG image part. Returns: the encoded body and its Content-Type header.
func image26ControlMultipart(t *testing.T, fields [][2]string) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, field := range fields {
		require.NoError(t, writer.WriteField(field[0], field[1]))
	}
	part, err := writer.CreateFormFile("image", "input.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("\x89PNG\r\n\x1a\nimage26-control-fixture"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return body.Bytes(), writer.FormDataContentType()
}

// runImage26ControlRelay relays one image request through RelayImageHelper to
// a fake upstream on a real channel with an operator image tariff. Parameters:
// t owns the fixture, channelType selects the upstream contract, modelName is
// the channel-mapped upstream model (the body may name a public alias), path
// selects generation or edit, and contentType/body are the exact client
// request. Returns: the observed result.
func runImage26ControlRelay(t *testing.T, channelType int, modelName, path, contentType string, body []byte) image26ControlRelayResult {
	t.Helper()
	cleanup := setupCacheBillingLogTest(t)
	t.Cleanup(cleanup)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelGroup{}, &model.UserRequestCost{}))
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 1).Update("quota", image26ControlBalance).Error)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Update("remain_quota", image26ControlBalance).Error)
	oldLog := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(true)
	t.Cleanup(func() { config.SetLogConsumeEnabled(oldLog) })
	oldBatch := config.BatchUpdateEnabled
	config.BatchUpdateEnabled = false
	t.Cleanup(func() { config.BatchUpdateEnabled = oldBatch })

	var calls atomic.Int32
	var mu sync.Mutex
	var seenBody []byte
	var seenContentType, seenPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		raw, err := io.ReadAll(r.Body)
		mu.Lock()
		seenBody, seenContentType, seenPath = raw, r.Header.Get("Content-Type"), r.URL.Path
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"fixture could not read the request body"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"created":1,"data":[{"b64_json":"AQ=="}],"usage":{"input_tokens":100,"output_tokens":100,"input_tokens_details":{"text_tokens":100,"image_tokens":0}}}`)
	}))
	t.Cleanup(upstream.Close)

	pricingJSON := fmt.Sprintf(`{%q:{"image":{"price_per_image_usd":0.1}}}`, modelName)
	channel := &model.Channel{Id: 1, Type: channelType, Name: "image26-controls", Status: model.ChannelStatusEnabled, ModelConfigs: &pricingJSON}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NotNil(t, channel.GetModelPriceConfigsWithContext(context.Background())[modelName].Image)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", contentType)
	gmw.SetLogger(c, logger.Logger)
	c.Set(ctxkey.Id, 1)
	c.Set(ctxkey.TokenId, 1)
	c.Set(ctxkey.TokenName, "relay-cache-log-token")
	c.Set(ctxkey.ChannelId, 1)
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.ChannelModel, channel)
	c.Set(ctxkey.ContentType, contentType)
	c.Set(ctxkey.RequestId, "image26-controls")
	mode := relaymode.GetByPath(path)
	metalib.Set2Context(c, &metalib.Meta{
		Mode: mode, ChannelType: channelType, APIType: channeltype.ToAPIType(channelType),
		ChannelId: 1, UserId: 1, TokenId: 1, TokenName: "relay-cache-log-token",
		BaseURL: upstream.URL, APIKey: "test-only", OriginModelName: modelName,
		ActualModelName: modelName, RequestURLPath: path, StartTime: time.Now(),
	})

	result := image26ControlRelayResult{Err: RelayImageHelper(c, mode)}
	result.Calls = calls.Load()
	mu.Lock()
	result.Body, result.ContentType, result.Path = seenBody, seenContentType, seenPath
	mu.Unlock()
	var user model.User
	var token model.Token
	require.NoError(t, model.DB.First(&user, 1).Error)
	require.NoError(t, model.DB.First(&token, 1).Error)
	result.UserQuota, result.TokenQuota = user.Quota, token.RemainQuota
	return result
}

// TestGPTImage26RelayForwardsRequestControls verifies the exact bytes OpenAI
// receives when clients set GPT Image 2.6 complexity, safety_identifier, and a
// billed raster output_format, plus byte-identical controls without them.
func TestGPTImage26RelayForwardsRequestControls(t *testing.T) {
	for _, tc := range []struct {
		name, model, body, want string
	}{
		{
			name:  "sunburst_generation_controls",
			model: "gpt-image-2.6-sunburst",
			body:  `{"model":"gpt-image-2.6-sunburst","prompt":"A lighthouse","complexity":" LOW ","safety_identifier":"sid-7f3a","user":"legacy-user","output_format":"webp"}`,
			want:  `{"model":"gpt-image-2.6-sunburst","prompt":"A lighthouse","n":1,"size":"auto","quality":"auto","user":"legacy-user","output_format":"webp","complexity":"low","safety_identifier":"sid-7f3a"}`,
		},
		{
			name:  "flare_generation_high_complexity",
			model: "gpt-image-2.6-flare",
			body:  `{"model":"gpt-image-2.6-flare","prompt":"A lighthouse","complexity":"high","safety_identifier":"sid-é"}`,
			want:  `{"model":"gpt-image-2.6-flare","prompt":"A lighthouse","n":1,"size":"auto","quality":"auto","complexity":"high","safety_identifier":"sid-é"}`,
		},
		{
			name:  "safety_identifier_at_128_runes",
			model: "gpt-image-2.6-sunburst",
			body:  `{"model":"gpt-image-2.6-sunburst","prompt":"A lighthouse","safety_identifier":"` + strings.Repeat("é", 128) + `"}`,
			want:  `{"model":"gpt-image-2.6-sunburst","prompt":"A lighthouse","n":1,"size":"auto","quality":"auto","safety_identifier":"` + strings.Repeat("é", 128) + `"}`,
		},
		{
			name:  "public_alias_mapped_to_flare",
			model: "gpt-image-2.6-flare",
			body:  `{"model":"image-alias","prompt":"A lighthouse","complexity":"Medium","output_format":"JPEG"}`,
			want:  `{"model":"gpt-image-2.6-flare","prompt":"A lighthouse","n":1,"size":"auto","quality":"auto","output_format":"jpeg","complexity":"medium"}`,
		},
		{
			name:  "legacy_dalle_keeps_safety_identifier_only",
			model: "dall-e-3",
			body:  `{"model":"dall-e-3","prompt":"A lighthouse","size":"1024x1024","safety_identifier":"sid-7f3a","user":"legacy-user","style":"vivid"}`,
			want:  `{"model":"dall-e-3","prompt":"A lighthouse","n":1,"size":"1024x1024","quality":"standard","style":"vivid","user":"legacy-user","safety_identifier":"sid-7f3a"}`,
		},
		{
			name:  "control_gpt_image_25_without_new_fields",
			model: "gpt-image-2.5-sunburst",
			body:  `{"model":"gpt-image-2.5-sunburst","prompt":"A lighthouse","user":"legacy-user","quality":"high"}`,
			want:  `{"model":"gpt-image-2.5-sunburst","prompt":"A lighthouse","n":1,"size":"auto","quality":"high","user":"legacy-user"}`,
		},
		{
			name:  "control_dalle_without_new_fields",
			model: "dall-e-3",
			body:  `{"model":"dall-e-3","prompt":"A lighthouse","size":"1024x1024","response_format":"b64_json"}`,
			want:  `{"model":"dall-e-3","prompt":"A lighthouse","n":1,"size":"1024x1024","quality":"standard","response_format":"b64_json"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runImage26ControlRelay(t, channeltype.OpenAI, tc.model, "/v1/images/generations", "application/json", []byte(tc.body))
			require.Nil(t, got.Err)
			require.Equal(t, int32(1), got.Calls)
			require.Equal(t, "/v1/images/generations", got.Path)
			require.Equal(t, tc.want, string(got.Body), "exact upstream generation payload")
		})
	}
}

// TestGPTImage26RelayForwardsMultipartEditVerbatim verifies OpenAI multipart
// edits keep their raw client bytes after the gateway validates the controls.
func TestGPTImage26RelayForwardsMultipartEditVerbatim(t *testing.T) {
	body, contentType := image26ControlMultipart(t, [][2]string{
		{"model", "gpt-image-2.6-flare"},
		{"prompt", "Add a red door"},
		{"complexity", "medium"},
		{"safety_identifier", "sid-7f3a"},
		{"output_format", "png"},
	})
	got := runImage26ControlRelay(t, channeltype.OpenAI, "gpt-image-2.6-flare", "/v1/images/edits", contentType, body)
	require.Nil(t, got.Err)
	require.Equal(t, int32(1), got.Calls)
	require.Equal(t, "/v1/images/edits", got.Path)
	require.Equal(t, contentType, got.ContentType)
	require.Equal(t, body, got.Body, "validated multipart edits must reach OpenAI byte-for-byte")
}

// TestGPTImage26RelayRejectsUnsupportedControls verifies unsupported controls
// fail with HTTP 400 before quota reservation or any paid upstream dispatch.
func TestGPTImage26RelayRejectsUnsupportedControls(t *testing.T) {
	jsonCase := func(model, extra string) (string, string, []byte, string) {
		return model, "/v1/images/generations", []byte(`{"model":"` + model + `","prompt":"A lighthouse"` + extra + `}`), "application/json"
	}
	editCase := func(model string, fields ...[2]string) (string, string, []byte, string) {
		body, contentType := image26ControlMultipart(t, append([][2]string{{"model", model}, {"prompt", "Add a red door"}}, fields...))
		return model, "/v1/images/edits", body, contentType
	}
	for _, tc := range []struct {
		name, code, message string
		build               func() (string, string, []byte, string)
	}{
		{"complexity_on_gpt_image_25", "invalid_value", "GPT Image 2.6", func() (string, string, []byte, string) {
			return jsonCase("gpt-image-2.5-sunburst", `,"complexity":"low"`)
		}},
		{"complexity_public_26_name_mapped_to_25", "invalid_value", "GPT Image 2.6", func() (string, string, []byte, string) {
			// The upstream contract follows the mapped model, not the public name.
			return "gpt-image-2.5-sunburst", "/v1/images/generations",
				[]byte(`{"model":"gpt-image-2.6-sunburst","prompt":"A lighthouse","complexity":"low"}`), "application/json"
		}},
		{"complexity_on_gpt_image_2", "invalid_value", "GPT Image 2.6", func() (string, string, []byte, string) {
			return jsonCase("gpt-image-2", `,"complexity":"high"`)
		}},
		{"complexity_on_dalle", "invalid_value", "GPT Image 2.6", func() (string, string, []byte, string) {
			return jsonCase("dall-e-3", `,"size":"1024x1024","complexity":"low"`)
		}},
		{"complexity_unknown_value", "invalid_value", "low, medium, high", func() (string, string, []byte, string) {
			return jsonCase("gpt-image-2.6-sunburst", `,"complexity":"extreme"`)
		}},
		{"complexity_blank_value", "invalid_value", "low, medium, high", func() (string, string, []byte, string) {
			return jsonCase("gpt-image-2.6-flare", `,"complexity":"   "`)
		}},
		{"safety_identifier_129_runes", "invalid_value", "safety_identifier", func() (string, string, []byte, string) {
			return jsonCase("gpt-image-2.6-sunburst", `,"safety_identifier":"`+strings.Repeat("é", 129)+`"`)
		}},
		{"svg_output_generation", "unsupported_output_format", "SVG", func() (string, string, []byte, string) {
			return jsonCase("gpt-image-2.6-sunburst", `,"output_format":"svg"`)
		}},
		{"gif_output_generation", "unsupported_output_format", "GIF", func() (string, string, []byte, string) {
			return jsonCase("gpt-image-2.6-flare", `,"output_format":"gif"`)
		}},
		{"svg_output_mixed_case_gpt_image_25", "unsupported_output_format", "SVG", func() (string, string, []byte, string) {
			return jsonCase("gpt-image-2.5-sunburst", `,"output_format":" SVG "`)
		}},
		{"case_folded_output_format_key", "invalid_image_request", "ambiguous", func() (string, string, []byte, string) {
			return jsonCase("gpt-image-2.6-sunburst", `,"Output_Format":"svg"`)
		}},
		{"duplicate_complexity_key", "invalid_image_request", "ambiguous", func() (string, string, []byte, string) {
			return jsonCase("gpt-image-2.6-sunburst", `,"complexity":"low","complexity":"high"`)
		}},
		{"multipart_edit_complexity_on_gpt_image_25", "invalid_value", "GPT Image 2.6", func() (string, string, []byte, string) {
			return editCase("gpt-image-2.5-flare", [2]string{"complexity", "low"})
		}},
		{"multipart_edit_svg_output", "unsupported_output_format", "SVG", func() (string, string, []byte, string) {
			return editCase("gpt-image-2.6-sunburst", [2]string{"output_format", "svg"})
		}},
		{"multipart_edit_gif_output", "unsupported_output_format", "GIF", func() (string, string, []byte, string) {
			return editCase("gpt-image-2.6-flare", [2]string{"output_format", "GIF"})
		}},
		{"multipart_edit_long_safety_identifier", "invalid_value", "safety_identifier", func() (string, string, []byte, string) {
			return editCase("gpt-image-2.6-flare", [2]string{"safety_identifier", strings.Repeat("x", 129)})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			modelName, path, body, contentType := tc.build()
			got := runImage26ControlRelay(t, channeltype.OpenAI, modelName, path, contentType, body)
			require.NotNil(t, got.Err, "unsupported controls must not reach the paid upstream")
			require.Equal(t, http.StatusBadRequest, got.Err.StatusCode)
			require.Equal(t, tc.code, got.Err.Code)
			require.Contains(t, got.Err.Message, tc.message)
			require.Zero(t, got.Calls, "rejected requests must never reach a paid upstream")
			require.Equal(t, image26ControlBalance, got.UserQuota, "rejection happens before quota reservation")
			require.Equal(t, image26ControlBalance, got.TokenQuota, "rejection happens before quota reservation")
		})
	}
}
