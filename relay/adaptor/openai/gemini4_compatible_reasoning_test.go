package openai

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGemini4OpenAICompatibleReasoningForwarding verifies converted requests
// preserve supported reasoning controls and system instructions for mapped models.
func TestGemini4OpenAICompatibleReasoningForwarding(t *testing.T) {
	for _, surface := range []struct {
		name string
		mode int
	}{
		{"chat", relaymode.ChatCompletions},
		{"claude", relaymode.ClaudeMessages},
		{"responses", relaymode.ResponseAPI},
	} {
		for _, tc := range []struct {
			name, actualModel string
			requested         *string
			want              string
		}{
			{"argon-high", "gemini-4-argon", stringPtrReasoning("high"), "high"},
			{"carbon-low", "gemini-4-carbon", stringPtrReasoning("low"), "low"},
			{"carbon-medium", "gemini-4-carbon", stringPtrReasoning("medium"), "medium"},
			{"carbon-minimal-alias", "gemini-4-carbon", stringPtrReasoning("minimal"), "low"},
			{"hydrogen-xhigh", "gemini-4-hydrogen", stringPtrReasoning("xhigh"), "xhigh"},
			{"hydrogen-minimal-alias", "gemini-4-hydrogen", stringPtrReasoning("minimal"), "low"},
			{"hydrogen-normalized", " GEMINI-4-HYDROGEN ", stringPtrReasoning(" XHIGH "), "xhigh"},
			{"carbon-default", "gemini-4-carbon", stringPtrReasoning("default"), ""},
			{"hydrogen-default", "gemini-4-hydrogen", stringPtrReasoning("default"), ""},
			{"hydrogen-omitted", "gemini-4-hydrogen", nil, ""},
		} {
			t.Run(surface.name+"/"+tc.name, func(t *testing.T) {
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
				info := &meta.Meta{
					ChannelType:     channeltype.GeminiOpenAICompatible,
					ActualModelName: tc.actualModel, Mode: surface.mode,
				}
				meta.Set2Context(ctx, info)
				originalEffort := ""
				if tc.requested != nil {
					originalEffort = *tc.requested
				}
				temperature, topP := 0.0, 0.9
				messages := []model.Message{
					{Role: "system", Content: "Keep the user's instructions."},
					{Role: "user", Content: "Explain this code."},
				}
				request := &model.GeneralOpenAIRequest{
					Model: "tenant-alias", Messages: messages,
					ReasoningEffort: tc.requested, Temperature: &temperature, TopP: &topP,
				}
				converted, err := (&Adaptor{}).ConvertRequest(ctx, surface.mode, request)
				require.NoError(t, err)
				result, ok := converted.(*model.GeneralOpenAIRequest)
				require.True(t, ok, "Gemini must keep the Chat Completions payload")
				require.Equal(t, messages, result.Messages)
				require.Equal(t, tc.actualModel, info.ActualModelName, "the tenant alias must not replace the upstream model")
				if tc.requested != nil {
					require.Equal(t, originalEffort, *tc.requested, "normalization must not mutate the caller's string")
				}
				encoded, err := json.Marshal(result)
				require.NoError(t, err)
				var payload map[string]any
				require.NoError(t, json.Unmarshal(encoded, &payload))
				if tc.want == "" {
					require.Nil(t, result.ReasoningEffort)
					require.NotContains(t, payload, "reasoning_effort", "omission preserves the model's upstream default")
				} else {
					require.Equal(t, tc.want, payload["reasoning_effort"])
				}
				require.Nil(t, result.Temperature)
				require.Nil(t, result.TopP)
				require.NotContains(t, payload, "temperature")
				require.NotContains(t, payload, "top_p")
			})
		}
	}
}

// TestGemini4OpenAICompatibleRejectsUnsupportedEffort verifies invalid controls
// fail locally instead of being silently discarded or sent to Google.
func TestGemini4OpenAICompatibleRejectsUnsupportedEffort(t *testing.T) {
	for _, tc := range []struct{ model, effort string }{
		{"gemini-4-argon", "xhigh"},
		{"gemini-4-carbon", "xhigh"},
		{"gemini-4-carbon", "none"},
		{"gemini-4-hydrogen", "ultra"},
		{"gemini-4-hydrogen", "none"},
	} {
		t.Run(tc.model+"/"+tc.effort, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			meta.Set2Context(ctx, &meta.Meta{
				ChannelType:     channeltype.GeminiOpenAICompatible,
				ActualModelName: tc.model, Mode: relaymode.ChatCompletions,
			})
			request := &model.GeneralOpenAIRequest{
				Model: "tenant-alias", ReasoningEffort: &tc.effort,
				Messages: []model.Message{{Role: "user", Content: "Hello"}},
			}
			converted, err := (&Adaptor{}).ConvertRequest(ctx, relaymode.ChatCompletions, request)
			require.ErrorContains(t, err, "validation failed")
			require.ErrorContains(t, err, "reasoning_effort")
			require.ErrorContains(t, err, tc.model)
			require.Nil(t, converted)
		})
	}
}

// TestGemini4OpenAICompatibleMinimalAliasMatchesNestedLow verifies the documented
// Chat alias and native level compare consistently before conflict validation.
func TestGemini4OpenAICompatibleMinimalAliasMatchesNestedLow(t *testing.T) {
	for _, modelName := range []string{"gemini-4-carbon", "gemini-4-hydrogen"} {
		t.Run(modelName, func(t *testing.T) {
			minimal, low := "minimal", "low"
			request := &model.GeneralOpenAIRequest{
				ReasoningEffort: &minimal,
				Reasoning:       &model.OpenAIResponseReasoning{Effort: &low},
			}
			info := &meta.Meta{ChannelType: channeltype.GeminiOpenAICompatible, ActualModelName: modelName}
			require.NoError(t, (&Adaptor{}).applyRequestTransformations(info, request))
			require.Equal(t, &low, request.ReasoningEffort)
			require.Nil(t, request.Reasoning)
			require.Equal(t, "minimal", minimal)
			require.Equal(t, "low", low)
		})
	}
}

// TestGemini4OpenAICompatibleScopePreservesLegacyBehavior verifies other channels
// and unpublished IDs keep their existing reasoning and sampling transformations.
func TestGemini4OpenAICompatibleScopePreservesLegacyBehavior(t *testing.T) {
	for _, tc := range []struct {
		name, model string
		channel     int
	}{
		{"other-provider", "gemini-4-hydrogen", channeltype.OpenAICompatible},
		{"unknown-model", "gemini-4-hydrogen-custom", channeltype.GeminiOpenAICompatible},
		{"older-gemini", "gemini-3.8-flash", channeltype.GeminiOpenAICompatible},
	} {
		t.Run(tc.name, func(t *testing.T) {
			temperature, topP, effort := 0.0, 0.9, "high"
			messages := []model.Message{{Role: "system", Content: "Keep this instruction."}}
			request := &model.GeneralOpenAIRequest{
				Model: "tenant-alias", Messages: messages,
				ReasoningEffort: &effort, Temperature: &temperature, TopP: &topP,
			}
			info := &meta.Meta{ChannelType: tc.channel, ActualModelName: tc.model}
			require.NoError(t, (&Adaptor{}).applyRequestTransformations(info, request))
			require.Nil(t, request.ReasoningEffort)
			require.Equal(t, &temperature, request.Temperature)
			require.Equal(t, &topP, request.TopP)
			require.Equal(t, messages, request.Messages)
		})
	}
}

// TestGemini4OpenAICompatibleResponsesIngress verifies real Responses lowering
// carries nested effort to Google's Chat wire field without mutating the source.
func TestGemini4OpenAICompatibleResponsesIngress(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	meta.Set2Context(ctx, &meta.Meta{
		ChannelType: channeltype.GeminiOpenAICompatible, Mode: relaymode.ChatCompletions,
		ModelMapping: map[string]string{"tenant-alias": "gemini-4-hydrogen"},
	})
	effort, instruction := "xhigh", "Keep this instruction."
	source := &ResponseAPIRequest{
		Model: "tenant-alias", Instructions: &instruction,
		Reasoning: &model.OpenAIResponseReasoning{Effort: &effort},
		Input:     ResponseAPIInput{map[string]any{"role": "user", "content": "Explain this code."}},
	}
	chat, err := ConvertResponseAPIToChatCompletionRequest(source)
	require.NoError(t, err)
	converted, err := (&Adaptor{}).ConvertRequest(ctx, relaymode.ChatCompletions, chat)
	require.NoError(t, err)
	result, ok := converted.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, &effort, result.ReasoningEffort)
	require.Nil(t, result.Reasoning, "Responses-only controls must not reach Chat upstreams")
	require.Equal(t, model.Message{Role: "system", Content: instruction}, result.Messages[0])
	require.Equal(t, &model.OpenAIResponseReasoning{Effort: &effort}, source.Reasoning)
	require.Equal(t, "xhigh", effort)
}

// TestGemini4OpenAICompatibleClaudeIngress verifies actual Claude conversion
// preserves output_config effort and system messages while keeping source ownership.
func TestGemini4OpenAICompatibleClaudeIngress(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	meta.Set2Context(ctx, &meta.Meta{
		ChannelType: channeltype.GeminiOpenAICompatible, Mode: relaymode.ClaudeMessages,
		ModelMapping: map[string]string{"tenant-alias": "gemini-4-hydrogen"},
	})
	temperature, topP := 0.0, 0.9
	source := &model.ClaudeRequest{
		Model: "tenant-alias", MaxTokens: 1024,
		System:       "Keep this instruction.",
		Messages:     []model.ClaudeMessage{{Role: "user", Content: "Explain this code."}},
		OutputConfig: json.RawMessage(`{"effort":"xhigh"}`),
		Temperature:  &temperature, TopP: &topP,
	}
	converted, err := (&Adaptor{}).ConvertClaudeRequest(ctx, source)
	require.NoError(t, err)
	result, ok := converted.(*model.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, stringPtrReasoning("xhigh"), result.ReasoningEffort)
	require.Empty(t, result.OutputConfig)
	require.Nil(t, result.Temperature)
	require.Nil(t, result.TopP)
	require.Equal(t, model.Message{Role: "system", Content: source.System}, result.Messages[0])
	require.JSONEq(t, `{"effort":"xhigh"}`, string(source.OutputConfig))
	require.Equal(t, &temperature, source.Temperature)
	require.Equal(t, &topP, source.TopP)
}

// TestGemini4OpenAICompatibleRejectsAmbiguousThinking verifies conflicting
// controls and provider-specific thinking objects fail before upstream dispatch.
func TestGemini4OpenAICompatibleRejectsAmbiguousThinking(t *testing.T) {
	for _, tc := range []struct {
		name    string
		request *model.GeneralOpenAIRequest
	}{
		{"conflicting-efforts", &model.GeneralOpenAIRequest{
			ReasoningEffort: stringPtrReasoning("low"),
			Reasoning:       &model.OpenAIResponseReasoning{Effort: stringPtrReasoning("xhigh")},
		}},
		{"provider-thinking", &model.GeneralOpenAIRequest{Thinking: &model.Thinking{Type: "adaptive"}}},
		{"invalid-claude-config", &model.GeneralOpenAIRequest{OutputConfig: json.RawMessage(`{"effort":3}`)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &meta.Meta{ChannelType: channeltype.GeminiOpenAICompatible, ActualModelName: "gemini-4-hydrogen"}
			err := (&Adaptor{}).applyRequestTransformations(info, tc.request)
			require.ErrorContains(t, err, "validation failed")
		})
	}
}
