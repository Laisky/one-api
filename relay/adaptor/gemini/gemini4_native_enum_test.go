package gemini

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestGemini4NativeWireThinkingEnum checks actual mapped-model payloads against the native enum contract.
func TestGemini4NativeWireThinkingEnum(t *testing.T) {
	for _, spec := range []struct {
		name, fallback string
		levels         []string
	}{
		{"gemini-4-argon", "MEDIUM", []string{"low", "medium", "high"}},
		{"gemini-4-carbon", "MEDIUM", []string{"low", "medium", "high"}},
		{"gemini-4-hydrogen", "HIGH", []string{"low", "medium", "high", "xhigh"}},
	} {
		for _, form := range []string{"omitted", "default", "empty", "lower", "upper", "spaced", "nested"} {
			for _, level := range spec.levels {
				t.Run(spec.name+"/"+form+"/"+level, func(t *testing.T) {
					req := &model.GeneralOpenAIRequest{Model: "caller-alias", Messages: []model.Message{{Role: "user", Content: "hello"}}}
					want := strings.ToUpper(level)
					requested := level
					switch form {
					case "omitted":
						want = spec.fallback
					case "default":
						requested = "default"
						req.ReasoningEffort = &requested
						want = spec.fallback
					case "empty":
						requested = ""
						req.ReasoningEffort = &requested
						want = spec.fallback
					case "lower":
						req.ReasoningEffort = &requested
					case "upper":
						requested = strings.ToUpper(level)
						req.ReasoningEffort = &requested
					case "spaced":
						requested = " " + strings.ToUpper(level) + " "
						req.ReasoningEffort = &requested
					case "nested":
						req.Reasoning = &model.OpenAIResponseReasoning{Effort: &requested}
					}
					before, err := json.Marshal(req)
					require.NoError(t, err)
					converted, err := (&Adaptor{}).ConvertRequest(gemini4TestContext(spec.name), relaymode.ChatCompletions, req)
					require.NoError(t, err)
					wire, err := json.Marshal(converted)
					require.NoError(t, err)
					var payload struct {
						GenerationConfig struct {
							ThinkingConfig struct {
								Level string `json:"thinkingLevel"`
							} `json:"thinkingConfig"`
						} `json:"generation_config"`
					}
					require.NoError(t, json.Unmarshal(wire, &payload))
					require.Equal(t, want, payload.GenerationConfig.ThinkingConfig.Level, "GenerateContent ThinkingLevel must use its documented uppercase enum")
					require.Contains(t, []string{"LOW", "MEDIUM", "HIGH", "XHIGH"}, payload.GenerationConfig.ThinkingConfig.Level)
					after, err := json.Marshal(req)
					require.NoError(t, err)
					require.JSONEq(t, string(before), string(after), "native conversion must preserve caller input")
				})
			}
		}
	}
}

// TestGemini4NativeEnumCompatibleControl retains lowercase compatible reasoning_effort.
func TestGemini4NativeEnumCompatibleControl(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	meta.Set2Context(ctx, &meta.Meta{ChannelType: channeltype.GeminiOpenAICompatible, ActualModelName: "gemini-4-hydrogen"})
	effort := " XHIGH "
	converted, err := (&openai.Adaptor{}).ConvertRequest(ctx, relaymode.ChatCompletions, &model.GeneralOpenAIRequest{
		Model: "caller-alias", ReasoningEffort: &effort, Messages: []model.Message{{Role: "user", Content: "hello"}},
	})
	require.NoError(t, err)
	wire, err := json.Marshal(converted)
	require.NoError(t, err)
	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(wire, &payload))
	require.JSONEq(t, `"xhigh"`, string(payload["reasoning_effort"]))
	require.NotContains(t, payload, "generation_config")
	require.Equal(t, " XHIGH ", effort)
}

// TestGemini4NativeEnumLegacyControl leaves unrelated native model generation unchanged.
func TestGemini4NativeEnumLegacyControl(t *testing.T) {
	for _, name := range []string{"gemini-2.5-flash", "gemini-4-unknown", "gemini-4-carbon-preview"} {
		t.Run(name, func(t *testing.T) {
			effort, temperature := "HIGH", 0.3
			converted, err := (&Adaptor{}).ConvertRequest(gemini4TestContext(name), relaymode.ChatCompletions, &model.GeneralOpenAIRequest{
				Model: name, ReasoningEffort: &effort, Temperature: &temperature, Messages: []model.Message{{Role: "user", Content: "hello"}},
			})
			require.NoError(t, err)
			wire, err := json.Marshal(converted)
			require.NoError(t, err)
			require.NotContains(t, string(wire), "thinkingLevel")
			require.Contains(t, string(wire), `"temperature":0.3`)
		})
	}
}
