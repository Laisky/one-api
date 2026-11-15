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

// gemini4FormatPayload exercises Responses fallback and serializes the actual compatible request.
func gemini4FormatPayload(t *testing.T, raw string) (map[string]json.RawMessage, error) {
	t.Helper()
	var source ResponseAPIRequest
	require.NoError(t, json.Unmarshal([]byte(raw), &source))
	before, err := json.Marshal(source)
	require.NoError(t, err)
	chat, err := ConvertResponseAPIToChatCompletionRequest(&source)
	require.NoError(t, err)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	meta.Set2Context(ctx, &meta.Meta{ChannelType: channeltype.GeminiOpenAICompatible,
		ActualModelName: "gemini-4-hydrogen", Mode: relaymode.ChatCompletions})
	converted, conversionErr := (&Adaptor{}).ConvertRequest(ctx, relaymode.ChatCompletions, chat)
	after, err := json.Marshal(source)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after), "caller-owned Responses data must remain unchanged")
	if conversionErr != nil {
		require.Nil(t, converted)
		return nil, conversionErr
	}
	wire, err := json.Marshal(converted)
	require.NoError(t, err)
	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(wire, &payload))
	return payload, nil
}

// TestGemini4OutputConfigFormatWire retains constraints through the real fallback and final upstream payload.
func TestGemini4OutputConfigFormatWire(t *testing.T) {
	for _, carrier := range []string{"output_config", "text"} {
		t.Run(carrier, func(t *testing.T) {
			raw := `{"model":"gemini-4-hydrogen","input":"extract","reasoning":{"effort":"xhigh"},"` + carrier + `":{"format":{"type":"json_schema","name":"extract","strict":true,"schema":{"type":"object","properties":{"count":{"type":"integer","minimum":9007199254740993},"kind":{"const":"note"}},"required":["count","kind"],"additionalProperties":false}}}}`
			payload, err := gemini4FormatPayload(t, raw)
			require.NoError(t, err)
			require.Contains(t, payload, "response_format")
			var format struct {
				Type   string `json:"type"`
				Schema struct {
					Schema json.RawMessage `json:"schema"`
				} `json:"json_schema"`
			}
			require.NoError(t, json.Unmarshal(payload["response_format"], &format))
			require.Equal(t, "json_schema", format.Type)
			if carrier == "output_config" {
				require.Contains(t, string(format.Schema.Schema), "9007199254740993")
			}
			require.Contains(t, string(format.Schema.Schema), `"const":"note"`)
			require.JSONEq(t, `"xhigh"`, string(payload["reasoning_effort"]))
			require.NotContains(t, payload, "output_config")
		})
	}
}

// TestGemini4OutputConfigValidation rejects constraints that cannot be faithfully lowered.
func TestGemini4OutputConfigValidation(t *testing.T) {
	for _, config := range []string{
		`{"format":{"type":"xml","schema":{}}}`,
		`{"format":{"type":"json_schema"}}`,
		`{"format":{"type":"json_schema","schema":true}}`,
		`{"format":{"type":"json_schema","schema":{},"unknown":true}}`,
		`{"format":null}`, `{"unknown":true}`, `[]`,
	} {
		t.Run(config, func(t *testing.T) {
			_, err := gemini4FormatPayload(t, `{"model":"gemini-4-hydrogen","input":"extract","output_config":`+config+`}`)
			require.ErrorContains(t, err, "validation failed")
		})
	}
}

// TestGemini4OutputConfigConflicts rejects competing constraints instead of choosing one silently.
func TestGemini4OutputConfigConflicts(t *testing.T) {
	_, err := gemini4FormatPayload(t, `{"model":"gemini-4-hydrogen","input":"extract","output_config":{"format":{"type":"json_schema","schema":{"type":"object"}}},"text":{"format":{"type":"json_object"}}}`)
	require.ErrorContains(t, err, "conflicting")
}

// TestGemini4SummaryAndSamplingWire validates unsupported summary and fixed sampling controls.
func TestGemini4SummaryAndSamplingWire(t *testing.T) {
	for _, summary := range []string{"auto", "concise", "detailed"} {
		_, err := gemini4FormatPayload(t, `{"model":"gemini-4-hydrogen","input":"extract","reasoning":{"summary":"`+summary+`"}}`)
		require.ErrorContains(t, err, "reasoning.summary")
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	meta.Set2Context(ctx, &meta.Meta{ChannelType: channeltype.GeminiOpenAICompatible, ActualModelName: "gemini-4-carbon"})
	topK := 4
	request := &model.GeneralOpenAIRequest{Model: "gemini-4-carbon", TopK: &topK}
	converted, err := (&Adaptor{}).ConvertRequest(ctx, relaymode.ChatCompletions, request)
	require.NoError(t, err)
	wire, err := json.Marshal(converted)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(wire, &payload))
	require.NotContains(t, payload, "top_k")
	require.Equal(t, 4, topK)
}
