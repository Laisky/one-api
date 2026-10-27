package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestGPT61TerraRejectsUnpricedCacheLifetimeVariants covers provider-owned
// cache controls that could select an unsupported paid tariff without 12h text.
func TestGPT61TerraRejectsUnpricedCacheLifetimeVariants(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"top_level_1h", `{"model":"gpt-6.1-terra","input":"hello","prompt_cache_options":{"ttl":"1h"}}`},
		{"top_level_6h", `{"model":"gpt-6.1-terra","input":"hello","prompt_cache_options":{"ttl":"6h"}}`},
		{"nested_1h", `{"model":"gpt-6.1-terra","input":[{"role":"user","content":[{"type":"input_text","text":"hello","prompt_cache_breakpoint":{"ttl":"1h"}}]}]}`},
		{"nested_numeric", `{"model":"gpt-6.1-terra","input":[{"role":"user","content":[{"type":"input_text","text":"hello","prompt_cache_breakpoint":{"ttl":3600}}]}]}`},
		{"nested_malformed", `{"model":"gpt-6.1-terra","input":[{"role":"user","content":[{"type":"input_text","text":"hello","prompt_cache_breakpoint":"12h"}]}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(tc.body)
			var typed openai.ResponseAPIRequest
			require.NoError(t, json.Unmarshal(raw, &typed))
			_, _, _, err := normalizeResponseAPIRawBody(raw, &typed, channeltype.OpenAI)
			require.Error(t, err, "unpriced/malformed cache controls must fail before provider dispatch")
		})
	}
}

// TestGPT61TerraCacheLifetimePolicyAcrossPublishedModels proves that the
// billing admission protection applies to other GPT-6 and GPT-5.6 models
// with the same unmetered 12h cache-write category, without altering tiers.
func TestGPT61TerraCacheLifetimePolicyAcrossPublishedModels(t *testing.T) {
	for _, modelName := range []string{"gpt-6-astra", "gpt-6-luna", "gpt-6.1-sol", "gpt-5.6-sol"} {
		t.Run(modelName, func(t *testing.T) {
			raw := []byte(`{"model":"` + modelName + `","input":"hello","prompt_cache_options":{"ttl":"12h"}}`)
			var typed openai.ResponseAPIRequest
			require.NoError(t, json.Unmarshal(raw, &typed))
			_, _, _, err := normalizeResponseAPIRawBody(raw, &typed, channeltype.OpenAI)
			require.Error(t, err, "native OpenAI 12h tariff must never use the 30m quota rate")
		})
	}
}

// TestGPT61TerraCacheLifetimeControlsPreserveSupportedRequests ensures
// ordinary text and default/30m cache choices do not become false positives.
func TestGPT61TerraCacheLifetimeControlsPreserveSupportedRequests(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte(`{"model":"gpt-6.1-terra","input":"The literal text ttl=12h is not a cache control."}`),
		[]byte(`{"model":"gpt-6.1-terra","input":"hello","prompt_cache_options":{"ttl":"30m"}}`),
		[]byte(`{"model":"gpt-6.1-terra","input":[{"role":"user","content":[{"type":"input_text","text":"hello","prompt_cache_breakpoint":{"ttl":"30m"}}]}]}`),
		[]byte(`{"model":"gpt-6-luna","input":"hello","service_tier":"fast"}`),
	} {
		var typed openai.ResponseAPIRequest
		require.NoError(t, json.Unmarshal(raw, &typed))
		_, _, _, err := normalizeResponseAPIRawBody(raw, &typed, channeltype.OpenAI)
		require.NoError(t, err)
	}
}
