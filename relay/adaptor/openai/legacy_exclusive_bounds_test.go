package openai

import (
	"encoding/json"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/model"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestLegacyExclusiveBoundsTranslation adapts valid Draft 4 pairs without converting booleans into numeric one.
func TestLegacyExclusiveBoundsTranslation(t *testing.T) {
	for _, tc := range []struct{ name, raw, want string }{
		{"min-true", `{"type":"number","minimum":9007199254740993.125,"exclusiveMinimum":true}`, `{"type":"number","minimum":9007199254740993.125,"exclusiveMinimum":9007199254740993.125}`},
		{"max-true", `{"type":"number","maximum":9007199254740995,"exclusiveMaximum":true}`, `{"type":"number","maximum":9007199254740995,"exclusiveMaximum":9007199254740995}`},
		{"both-false", `{"type":"number","minimum":0,"maximum":7,"exclusiveMinimum":false,"exclusiveMaximum":false}`, `{"type":"number","minimum":0,"maximum":7}`},
		{"numeric", `{"type":"number","exclusiveMinimum":9007199254740993,"exclusiveMaximum":9007199254740993.125}`, `{"type":"number","exclusiveMinimum":9007199254740993,"exclusiveMaximum":9007199254740993.125}`},
		{"unpaired-invalid-legacy", `{"type":"number","exclusiveMinimum":true,"exclusiveMaximum":false}`, `{"type":"number"}`},
		{"invalid-threshold", `{"type":"number","minimum":"not-a-number","exclusiveMinimum":true}`, `{"type":"number","minimum":"not-a-number"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var schema, want map[string]any
			require.NoError(t, decodeExactSchema([]byte(tc.raw), &schema))
			require.NoError(t, decodeExactSchema([]byte(tc.want), &want))
			before, err := json.Marshal(schema)
			require.NoError(t, err)
			got, _ := NormalizeStructuredJSONSchemaForModel(schema, channeltype.OpenAI, "gpt-6.1-sol")
			require.Equal(t, want, got)
			after, err := json.Marshal(schema)
			require.NoError(t, err)
			require.Equal(t, string(before), string(after))
			again, changed := NormalizeStructuredJSONSchemaForModel(got, channeltype.OpenAI, "gpt-6.1-sol")
			require.False(t, changed, "modern normalization must be idempotent")
			require.Equal(t, got, again)
		})
	}
}

// decodeExactSchema decodes schema JSON without float64 rounding.
func decodeExactSchema(raw []byte, target any) error {
	return model.DecodeSchemaJSON(raw, target)
}

// normalizeLegacyExclusiveFixture checks source ownership and idempotence around one provider policy conversion.
func normalizeLegacyExclusiveFixture(t *testing.T, source map[string]any, channel int, name string) map[string]any {
	t.Helper()
	before, err := json.Marshal(source)
	require.NoError(t, err)
	got, _ := NormalizeStructuredJSONSchemaForModel(source, channel, name)
	after, err := json.Marshal(source)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after), "normalization must not mutate caller schemas")
	again, changed := NormalizeStructuredJSONSchemaForModel(got, channel, name)
	require.False(t, changed, "the resulting schema must be stable on repeated conversion")
	require.Equal(t, got, again)
	return got
}

// TestLegacyExclusiveBoundsSchemaContexts translates nested schema nodes while retaining property names and literal instance data.
func TestLegacyExclusiveBoundsSchemaContexts(t *testing.T) {
	const raw = `{"type":"object","properties":{"exclusiveMinimum":{"type":"number","minimum":-2,"exclusiveMinimum":true},"literal":{"const":{"minimum":-7,"exclusiveMinimum":true},"enum":[{"exclusiveMaximum":false}],"default":{"exclusiveMinimum":true},"examples":[{"exclusiveMaximum":true}]}},"$defs":{"exclusiveMaximum":{"type":"number","maximum":0,"exclusiveMaximum":true}},"allOf":[{"properties":{"fraction":{"minimum":-0.123456789012345678901234567890,"exclusiveMinimum":false}}}],"items":{"anyOf":[{"maximum":9007199254740997,"exclusiveMaximum":true}]}}`
	const wantRaw = `{"type":"object","properties":{"exclusiveMinimum":{"type":"number","minimum":-2,"exclusiveMinimum":-2},"literal":{"const":{"minimum":-7,"exclusiveMinimum":true},"enum":[{"exclusiveMaximum":false}],"default":{"exclusiveMinimum":true},"examples":[{"exclusiveMaximum":true}]}},"$defs":{"exclusiveMaximum":{"type":"number","maximum":0,"exclusiveMaximum":0}},"allOf":[{"properties":{"fraction":{"minimum":-0.123456789012345678901234567890}}}],"items":{"anyOf":[{"maximum":9007199254740997,"exclusiveMaximum":9007199254740997}]}}`
	var source, want map[string]any
	require.NoError(t, decodeExactSchema([]byte(raw), &source))
	require.NoError(t, decodeExactSchema([]byte(wantRaw), &want))
	got := normalizeLegacyExclusiveFixture(t, source, channeltype.OpenAI, "gpt-6.1-sol")
	require.Equal(t, want, got)
	properties := got["properties"].(map[string]any)
	require.Contains(t, properties, "exclusiveMinimum")
	definitions := got["$defs"].(map[string]any)
	require.Contains(t, definitions, "exclusiveMaximum")
	// A changed descendant owns its new map; later retry-local edits cannot reach the source.
	properties["exclusiveMinimum"].(map[string]any)["minimum"] = json.Number("-999")
	sourceProperty := source["properties"].(map[string]any)["exclusiveMinimum"].(map[string]any)
	require.Equal(t, json.Number("-2"), sourceProperty["minimum"])
	require.Equal(t, true, sourceProperty["exclusiveMinimum"])
}

// TestLegacyExclusiveBoundsProviderIsolation retains conservative policies for unsupported destinations without altering literal data.
func TestLegacyExclusiveBoundsProviderIsolation(t *testing.T) {
	for _, tc := range []struct {
		name, model string
		channel     int
		inclusive   bool
	}{
		{"google-compatible", "gemini-4-hydrogen", channeltype.GeminiOpenAICompatible, true},
		{"google-native", "gemini-4-hydrogen", channeltype.Gemini, true},
		{"vertex-google", "gemini-4-hydrogen", channeltype.VertextAI, true},
		{"azure", "gpt-6.1-sol", channeltype.Azure, false},
		{"unknown-provider", "gpt-6.1-sol", channeltype.OpenAICompatible, false},
		{"unknown-model", "unknown-model", channeltype.OpenAI, false},
		{"fine-tuned", "ft:gpt-4o:fixture", channeltype.OpenAI, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var source map[string]any
			require.NoError(t, decodeExactSchema([]byte(`{"type":"object","properties":{"exclusiveMinimum":{"type":"number","minimum":-2,"maximum":0,"exclusiveMinimum":true,"exclusiveMaximum":false},"literal":{"const":{"exclusiveMinimum":true,"exclusiveMaximum":false}}},"$defs":{"exclusiveMaximum":{"type":"number","minimum":0,"exclusiveMinimum":true}}}`), &source))
			got := normalizeLegacyExclusiveFixture(t, source, tc.channel, tc.model)
			properties := got["properties"].(map[string]any)
			require.Contains(t, properties, "exclusiveMinimum")
			node := properties["exclusiveMinimum"].(map[string]any)
			require.NotContains(t, node, "exclusiveMinimum")
			require.NotContains(t, node, "exclusiveMaximum")
			if tc.inclusive {
				require.Equal(t, json.Number("-2"), node["minimum"])
				require.Equal(t, json.Number("0"), node["maximum"])
			} else {
				require.NotContains(t, node, "minimum")
				require.NotContains(t, node, "maximum")
			}
			require.Equal(t, map[string]any{"exclusiveMinimum": true, "exclusiveMaximum": false}, properties["literal"].(map[string]any)["const"])
			definitions := got["$defs"].(map[string]any)
			require.Contains(t, definitions, "exclusiveMaximum")
			require.NotContains(t, definitions["exclusiveMaximum"], "exclusiveMinimum")
		})
	}
}

// TestLegacyExclusiveBoundsProgrammaticNumbers retains zero, negative, and exact numeric threshold types without coercion.
func TestLegacyExclusiveBoundsProgrammaticNumbers(t *testing.T) {
	for _, tc := range []struct {
		name  string
		bound any
	}{
		{"int-zero", int(0)}, {"int-negative", int(-3)}, {"int8-negative", int8(-2)},
		{"int64-large-negative", int64(-9007199254740993)},
		{"uint-zero", uint(0)}, {"uint64-large", uint64(9007199254740997)},
		{"float32-negative", float32(-0.5)}, {"float64-zero", float64(0)}, {"float64-negative", float64(-0.125)},
		{"number-zero", json.Number("0")}, {"number-large", json.Number("9007199254740993")},
		{"number-negative-fraction", json.Number("-0.123456789012345678901234567890")},
	} {
		for _, side := range []struct{ inclusive, exclusive string }{
			{"minimum", "exclusiveMinimum"}, {"maximum", "exclusiveMaximum"},
		} {
			for _, legacy := range []struct {
				name    string
				enabled bool
			}{{"true", true}, {"false", false}} {
				t.Run(tc.name+"/"+side.exclusive+"/"+legacy.name, func(t *testing.T) {
					source := map[string]any{"type": "number", side.inclusive: tc.bound, side.exclusive: legacy.enabled}
					got := normalizeLegacyExclusiveFixture(t, source, channeltype.OpenAI, "gpt-6.1-sol")
					require.Equal(t, tc.bound, got[side.inclusive], "inclusive thresholds keep their exact Go representation")
					if legacy.enabled {
						require.Equal(t, tc.bound, got[side.exclusive], "true qualifies the exact bound rather than numeric one")
					} else {
						require.NotContains(t, got, side.exclusive)
					}
				})
			}
		}
	}
}
