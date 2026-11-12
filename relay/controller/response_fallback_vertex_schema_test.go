package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
)

// TestResponseFallbackVertexSchemaFidelity verifies real Responses dispatch preserves Vertex schemas while other providers retain their sanitizer.
func TestResponseFallbackVertexSchemaFidelity(t *testing.T) {
	var providers []providerMatrixProvider
	for _, p := range providerMatrixProviders() {
		if p.name == "gemini" || p.name == "openai_compatible" {
			providers = append(providers, p)
		}
	}
	vertex := providers[1]
	if vertex.name != "gemini" {
		t.Fatal("missing native Gemini fixture")
	}
	vertex.name = "vertex_gemini"
	vertex.channel = channeltype.VertextAI
	vertex.config = model.ChannelConfig{VertexAIProjectID: providerMatrixVertexProject, Region: providerMatrixVertexRegion}
	vertex.vertexToken = providerMatrixVertexToken
	providers = append(providers, vertex)
	for _, p := range providers {
		t.Run(p.name, func(t *testing.T) {
			const schema = `{"type":"object","description":"Keep this schema","additionalProperties":false,"properties":{"kind":{"type":"string","const":"note"},"entries":{"type":"array","items":{"$ref":"#/$defs/entry"}}},"$defs":{"entry":{"anyOf":[{"type":"string"},{"type":"null"}]}},"required":["kind"]}`
			body := `{"model":"` + p.actual + `","input":"Save note","tools":[{"type":"function","name":"store_note","parameters":` + schema + `}],"text":{"format":{"type":"json_schema","name":"result","schema":` + schema + `}}}`
			result := runProviderMatrix(t, p, false, body, providerMatrixOptions{})
			requireProviderMatrixDispatched(t, result)
			var wire map[string]any
			require.NoError(t, json.Unmarshal(result.got.Body, &wire))
			var toolSchema, outputSchema map[string]any
			tools := wire["tools"].([]any)
			if p.family == providerMatrixGemini {
				declaration := tools[0].(map[string]any)["function_declarations"].([]any)[0].(map[string]any)
				key := "parameters"
				if p.name == "vertex_gemini" {
					key = "parametersJsonSchema"
				}
				toolSchema = declaration[key].(map[string]any)
				generation := wire["generation_config"].(map[string]any)
				key = "responseSchema"
				if p.name == "vertex_gemini" {
					key = "responseJsonSchema"
				}
				outputSchema = generation[key].(map[string]any)
			} else {
				toolSchema = tools[0].(map[string]any)["function"].(map[string]any)["parameters"].(map[string]any)
				outputSchema = wire["response_format"].(map[string]any)["json_schema"].(map[string]any)["schema"].(map[string]any)
			}
			if p.name == "vertex_gemini" {
				var expected map[string]any
				require.NoError(t, json.Unmarshal([]byte(schema), &expected))
				require.Equal(t, expected, toolSchema)
				require.Equal(t, expected, outputSchema)
			} else {
				require.NotContains(t, toolSchema, "additionalProperties")
				require.NotContains(t, outputSchema, "additionalProperties")
			}
		})
	}
}

// TestResponseFallbackSchemaPolicyUsesMappedProvider verifies schema fidelity follows trusted routing rather than the public model alias.
func TestResponseFallbackSchemaPolicyUsesMappedProvider(t *testing.T) {
	for _, tc := range []struct {
		name     string
		meta     *metalib.Meta
		preserve bool
	}{
		{"nil", nil, false},
		{"vertex_gemini_alias", &metalib.Meta{APIType: apitype.VertexAI, OriginModelName: "friendly", ActualModelName: "gemini-3.8-flash"}, true},
		{"vertex_claude", &metalib.Meta{APIType: apitype.VertexAI, ActualModelName: "claude-opus-4-6"}, false},
		{"developer_gemini", &metalib.Meta{APIType: apitype.Gemini, ActualModelName: "gemini-3.8-flash"}, false},
		{"compatible_gemini", &metalib.Meta{APIType: apitype.OpenAI, ActualModelName: "gemini-3.8-flash"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.preserve, responseFallbackConversionOptions(tc.meta).PreserveJSONSchema)
			require.Equal(t, tc.preserve, responseFallbackConversionOptions(tc.meta).RequireToolResultPairing)
		})
	}
}
