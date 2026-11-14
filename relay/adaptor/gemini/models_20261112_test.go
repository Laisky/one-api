package gemini

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGemini4NativeGenerationContracts checks actual native request conversion
// for mapped model aliases, valid defaults, explicit levels, and ignored sampling.
// Parameter t coordinates offline assertions; this test returns no values.
func TestGemini4NativeGenerationContracts(t *testing.T) {
	for _, spec := range []struct {
		name, defaultLevel string
		levels             []string
	}{
		{"gemini-4-argon", "medium", []string{"low", "medium", "high"}},
		{"gemini-4-carbon", "medium", []string{"low", "medium", "high"}},
		{"gemini-4-hydrogen", "high", []string{"low", "medium", "high", "xhigh"}},
	} {
		for _, level := range append([]string{"absent", "default", ""}, spec.levels...) {
			t.Run(spec.name+"/"+level, func(t *testing.T) {
				c := gemini4TestContext(spec.name)
				temperature, topP := 0.2, 0.3
				request := model.GeneralOpenAIRequest{
					Model: "public-alias", MaxTokens: 1024, Temperature: &temperature, TopP: &topP,
					Messages: []model.Message{{Role: "system", Content: "Follow the operating policy."}, {Role: "user", Content: "Review this change."}},
				}
				if level != "absent" {
					request.ReasoningEffort = &level
				}
				before, err := json.Marshal(request)
				require.NoError(t, err)
				converted, err := (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, &request)
				require.NoError(t, err)
				native, ok := converted.(*ChatRequest)
				require.True(t, ok)
				require.NotNil(t, native.GenerationConfig.ThinkingConfig)
				want := level
				if level == "absent" || level == "default" || level == "" {
					want = spec.defaultLevel
				}
				require.Equal(t, strings.ToUpper(want), native.GenerationConfig.ThinkingConfig.ThinkingLevel)
				require.Nil(t, native.GenerationConfig.Temperature)
				require.Nil(t, native.GenerationConfig.TopP)
				require.Zero(t, native.GenerationConfig.TopK)
				require.Equal(t, 1024, native.GenerationConfig.MaxOutputTokens)
				require.Nil(t, native.GenerationConfig.ResponseModalities)
				require.NotNil(t, native.SystemInstruction)
				require.Equal(t, "Follow the operating policy.", native.SystemInstruction.Parts[0].Text)
				require.Len(t, native.Contents, 1)
				require.Equal(t, "Review this change.", native.Contents[0].Parts[0].Text)
				wire, err := json.Marshal(native)
				require.NoError(t, err)
				require.Contains(t, string(wire), `"thinkingLevel":"`+strings.ToUpper(want)+`"`)
				require.NotContains(t, string(wire), `"temperature"`)
				require.NotContains(t, string(wire), `"topP"`)
				after, err := json.Marshal(request)
				require.NoError(t, err)
				require.JSONEq(t, string(before), string(after), "conversion must not mutate caller input")
			})
		}
	}
}

// TestGemini4NativeEffortAdmission checks invalid levels, nested Responses
// controls, normalized equivalent fields, and model-specific xhigh acceptance.
// Parameter t coordinates offline assertions; this test returns no values.
func TestGemini4NativeEffortAdmission(t *testing.T) {
	for _, tc := range []struct {
		name, upstream, fields, want, wantError string
	}{
		{"hydrogen-xhigh", "Gemini-4-Hydrogen", `"reasoning_effort":" XHIGH "`, "xhigh", ""},
		{"nested-effort", "gemini-4-hydrogen", `"reasoning":{"effort":"xhigh"}`, "xhigh", ""},
		{"matching-fields", "gemini-4-hydrogen", `"reasoning_effort":" XHIGH ","reasoning":{"effort":"xhigh"}`, "xhigh", ""},
		{"conflicting-fields", "gemini-4-hydrogen", `"reasoning_effort":"high","reasoning":{"effort":"low"}`, "", "conflicting reasoning effort"},
		{"argon-xhigh", "gemini-4-argon", `"reasoning_effort":"xhigh"`, "", "reasoning effort"},
		{"carbon-xhigh", "gemini-4-carbon", `"reasoning_effort":"xhigh"`, "", "reasoning effort"},
		{"hydrogen-minimal", "gemini-4-hydrogen", `"reasoning_effort":"minimal"`, "", "reasoning effort"},
		{"disabled-thinking", "gemini-4-hydrogen", `"reasoning_effort":"none"`, "", "reasoning effort"},
		{"unknown-effort", "gemini-4-carbon", `"reasoning_effort":"ultra"`, "", "reasoning effort"},
		{"integer-budget", "gemini-4-hydrogen", `"thinking":{"type":"enabled","budget_tokens":4096}`, "", "thinking budgets"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var request model.GeneralOpenAIRequest
			require.NoError(t, json.Unmarshal([]byte(`{"model":"public-alias","messages":[{"role":"user","content":"Hello"}],`+tc.fields+`}`), &request))
			converted, err := (&Adaptor{}).ConvertRequest(gemini4TestContext(tc.upstream), relaymode.ChatCompletions, &request)
			require.Equal(t, "public-alias", request.Model)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				require.Nil(t, converted)
				return
			}
			require.NoError(t, err)
			native, ok := converted.(*ChatRequest)
			require.True(t, ok)
			require.NotNil(t, native.GenerationConfig.ThinkingConfig)
			require.Equal(t, strings.ToUpper(tc.want), native.GenerationConfig.ThinkingConfig.ThinkingLevel)
		})
	}
}

// TestGemini4NativeClientFormats verifies Gemini controls survive Claude Messages
// and Responses ingress lowering while preserving system instructions and aliases.
// Parameter t coordinates offline assertions; this test returns no values.
func TestGemini4NativeClientFormats(t *testing.T) {
	for _, format := range []string{"claude", "responses"} {
		t.Run(format, func(t *testing.T) {
			c := gemini4TestContext("gemini-4-hydrogen")
			var converted any
			var err error
			switch format {
			case "claude":
				request := &model.ClaudeRequest{
					Model: "public-alias", System: "Follow the operating policy.", MaxTokens: 2048,
					OutputConfig: json.RawMessage(`{"effort":"xhigh"}`),
					Messages:     []model.ClaudeMessage{{Role: "user", Content: "Review this change."}},
				}
				converted, err = (&Adaptor{}).ConvertClaudeRequest(c, request)
				require.Equal(t, "public-alias", request.Model)
			case "responses":
				var request openai.ResponseAPIRequest
				require.NoError(t, json.Unmarshal([]byte(`{"model":"public-alias","instructions":"Follow the operating policy.","input":"Review this change.","reasoning":{"effort":"xhigh"},"max_output_tokens":2048}`), &request))
				chat, convertErr := openai.ConvertResponseAPIToChatCompletionRequest(&request, true)
				require.NoError(t, convertErr)
				converted, err = (&Adaptor{}).ConvertRequest(c, relaymode.ChatCompletions, chat)
				require.Equal(t, "public-alias", request.Model)
			}
			require.NoError(t, err)
			native, ok := converted.(*ChatRequest)
			require.True(t, ok)
			require.NotNil(t, native.GenerationConfig.ThinkingConfig)
			require.Equal(t, "XHIGH", native.GenerationConfig.ThinkingConfig.ThinkingLevel)
			require.Equal(t, 2048, native.GenerationConfig.MaxOutputTokens)
			require.NotNil(t, native.SystemInstruction)
			require.Equal(t, "Follow the operating policy.", native.SystemInstruction.Parts[0].Text)
		})
	}
}

// TestGemini4NativeURLsAndLegacyControl checks both ordinary REST transports,
// explicit version overrides, and unchanged conversion of unrelated model IDs.
// Parameter t coordinates offline assertions; this test returns no values.
func TestGemini4NativeURLsAndLegacyControl(t *testing.T) {
	for _, name := range []string{"gemini-4-carbon", "gemini-4-hydrogen"} {
		for _, stream := range []bool{false, true} {
			requestMeta := &meta.Meta{ActualModelName: name, Mode: relaymode.ChatCompletions, BaseURL: "https://generativelanguage.googleapis.com", IsStream: stream}
			action := "generateContent"
			if stream {
				action = "streamGenerateContent?alt=sse"
			}
			for _, version := range []string{"", "v1"} {
				requestMeta.Config.APIVersion = version
				url, err := (&Adaptor{}).GetRequestURL(requestMeta)
				require.NoError(t, err)
				if version == "" {
					version = "v1beta"
				}
				require.Equal(t, "https://generativelanguage.googleapis.com/"+version+"/models/"+name+":"+action, url)
			}
		}
	}
	for _, name := range []string{"gemini-2.5-flash", "gemini-4-unknown", "gemini-4-carbon-preview"} {
		t.Run(name, func(t *testing.T) {
			temperature, topP, effort := 0.2, 0.3, "xhigh"
			request := &model.GeneralOpenAIRequest{Model: name, ReasoningEffort: &effort, Temperature: &temperature, TopP: &topP, MaxTokens: 2048,
				Messages: []model.Message{{Role: "user", Content: "Hello"}}}
			converted, err := (&Adaptor{}).ConvertRequest(gemini4TestContext(name), relaymode.ChatCompletions, request)
			require.NoError(t, err)
			native := converted.(*ChatRequest)
			require.Nil(t, native.GenerationConfig.ThinkingConfig)
			require.Equal(t, &temperature, native.GenerationConfig.Temperature)
			require.Equal(t, &topP, native.GenerationConfig.TopP)
			if strings.HasPrefix(name, "gemini-4-") {
				require.False(t, IsModelSupportSystemInstruction(name))
			}
		})
	}
}

// gemini4TestContext creates a request context bound to the upstream model.
// Parameter name is the mapped provider ID; the returned context owns no secrets.
func gemini4TestContext(name string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Set(ctxkey.Meta, &meta.Meta{ActualModelName: name})
	return c
}
