package format_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/gemini"
	vertexgemini "github.com/Laisky/one-api/relay/adaptor/vertexai/gemini"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// This temporary placement runs the actual native adaptor contracts through the
// existing essential unit gate. After RED/GREEN verification, move these tests
// to the Gemini package; do not permanently expand the normal CI compilation set.

// TestReview607NativeThinkingWireContract verifies both native transports,
// mapped IDs, model-specific defaults, explicit levels and invalid-level errors.
func TestReview607NativeThinkingWireContract(t *testing.T) {
	for _, provider := range []string{"gemini", "vertex"} {
		for _, level := range []string{"absent", "default", "minimal", "high", "low", "medium", "none", "xhigh"} {
			t.Run(provider+"/"+level, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
				c.Set(ctxkey.Meta, &meta.Meta{ActualModelName: "gemini-3.1-flash-lite-image"})
				request := &model.GeneralOpenAIRequest{
					Model: "public-alias", Messages: []model.Message{{Role: "user", Content: "Draw a bird"}},
				}
				if level != "absent" {
					request.ReasoningEffort = &level
				}
				var converted any
				var err error
				if provider == "gemini" {
					converted, err = (&gemini.Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, request)
				} else {
					converted, err = (&vertexgemini.Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, request)
				}
				require.Equal(t, "public-alias", request.Model, "conversion must not mutate the caller")
				switch level {
				case "absent", "default", "minimal", "high":
					require.NoError(t, err)
					encoded, err := json.Marshal(converted)
					require.NoError(t, err)
					want := "minimal"
					if level == "high" {
						want = "high"
					}
					require.Contains(t, string(encoded), `"thinkingLevel":"`+want+`"`)
				default:
					require.ErrorContains(t, err, "reasoning effort")
					require.Nil(t, converted)
				}
			})
		}
	}
}

// TestReview607NativeMediaWireContract preserves interleaved text/video/PDF
// through the real Gemini and Vertex converters, for unary and streaming calls.
func TestReview607NativeMediaWireContract(t *testing.T) {
	for _, provider := range []string{"gemini", "vertex"} {
		for _, modelName := range []string{"gemini-3.1-flash-image", "gemini-3.1-flash-lite-image"} {
			for _, stream := range []bool{false, true} {
				t.Run(provider+"/"+modelName+map[bool]string{false: "/unary", true: "/stream"}[stream], func(t *testing.T) {
					var request model.GeneralOpenAIRequest
					require.NoError(t, json.Unmarshal([]byte(`{"model":"public-alias","messages":[{"role":"user","content":[{"type":"text","text":"before"},{"type":"video_url","video_url":{"url":"data:video/mp4;base64,YQ=="}},{"type":"file","file":{"file_url":"https://example.com/document.pdf"}},{"type":"text","text":"after"}]}]}`), &request))
					request.Stream = stream
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
					c.Set(ctxkey.Meta, &meta.Meta{ActualModelName: modelName})
					var converted any
					var err error
					if provider == "gemini" {
						converted, err = (&gemini.Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, &request)
					} else {
						converted, err = (&vertexgemini.Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, &request)
					}
					require.NoError(t, err)
					require.Equal(t, "public-alias", request.Model)
					wire, err := json.Marshal(converted)
					require.NoError(t, err)
					var payload struct {
						Contents []gemini.ChatContent `json:"contents"`
					}
					require.NoError(t, json.Unmarshal(wire, &payload))
					require.Len(t, payload.Contents, 1)
					parts := payload.Contents[0].Parts
					require.Len(t, parts, 4, "do not drop media or duplicate concatenated text")
					require.Equal(t, "before", parts[0].Text)
					require.NotNil(t, parts[1].InlineData)
					require.Equal(t, "video/mp4", parts[1].InlineData.MimeType)
					require.Equal(t, "YQ==", parts[1].InlineData.Data)
					require.NotNil(t, parts[2].FileData)
					require.Equal(t, "https://example.com/document.pdf", parts[2].FileData.FileURI)
					require.Equal(t, "after", parts[3].Text)
				})
			}
		}
	}
}

// TestReview607NativeMediaErrors rejects malformed and unsupported media before
// dispatch rather than dropping it and sending an empty fallback text prompt.
func TestReview607NativeMediaErrors(t *testing.T) {
	for _, tc := range []struct{ name, model, content string }{
		{"malformed-video", "gemini-3.1-flash-image", `[{"type":"video_url","video_url":{}}]`},
		{"bad-base64", "gemini-3.1-flash-image", `[{"type":"video_url","video_url":{"url":"data:video/mp4;base64,!"}}]`},
		{"unsupported-video", "gemini-3-pro-image", `[{"type":"video_url","video_url":{"url":"data:video/mp4;base64,YQ=="}}]`},
		{"disguised-audio", "gemini-3.1-flash-lite-image", `[{"type":"file","file_data":"data:audio/wav;base64,YQ=="}]`},
		{"unknown-content", "gemini-3.1-flash-image", `[{"type":"silently_drop_me"}]`},
		{"unresolved-file-id", "gemini-3.1-flash-image", `[{"type":"file","file_id":"another-tenants-file"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var content any
			require.NoError(t, json.Unmarshal([]byte(tc.content), &content))
			request := &model.GeneralOpenAIRequest{Model: tc.model, Messages: []model.Message{{Role: "user", Content: content}}}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			converted, err := (&gemini.Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, request)
			require.Error(t, err)
			require.Nil(t, converted)
		})
	}
}
