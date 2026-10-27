package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestGPT61TerraStandardServiceTierAdmission checks the final native Responses
// provider body before upstream dispatch. Terra's audited tariff is Standard-only.
func TestGPT61TerraStandardServiceTierAdmission(t *testing.T) {
	for _, tier := range []string{"fast", "priority", "flex", "auto", "ultrafast"} {
		t.Run(tier, func(t *testing.T) {
			raw := []byte(`{"model":"gpt-6.1-terra","input":"hello","service_tier":"` + tier + `"}`)
			var request openai.ResponseAPIRequest
			require.NoError(t, json.Unmarshal(raw, &request))
			_, _, _, err := normalizeResponseAPIRawBody(raw, &request, channeltype.OpenAI)
			require.ErrorContains(t, err, "Standard")
		})
	}
	for _, raw := range [][]byte{
		[]byte(`{"model":"gpt-6.1-terra","input":"hello"}`),
		[]byte(`{"model":"gpt-6.1-terra","input":"hello","service_tier":"default"}`),
	} {
		var request openai.ResponseAPIRequest
		require.NoError(t, json.Unmarshal(raw, &request))
		wire, _, _, err := normalizeResponseAPIRawBody(raw, &request, channeltype.OpenAI)
		require.NoError(t, err)
		var payload map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(wire, &payload))
		require.Equal(t, `"default"`, string(payload["service_tier"]))
	}
}

// TestGPT61TerraRejectsUnmeteredTwelveHourCache ensures Standard-only Terra
// requests cannot forward a more expensive cache lifetime without a meter.
func TestGPT61TerraRejectsUnmeteredTwelveHourCache(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte(`{"model":"gpt-6.1-terra","input":"hello","prompt_cache_options":{"ttl":"12h"}}`),
		[]byte(`{"model":"gpt-6.1-terra","input":[{"role":"user","content":[{"type":"input_text","text":"hello","prompt_cache_breakpoint":{"ttl":"12h"}}]}]}`),
		[]byte(`{"model":"gpt-6.1-terra","input":"hello","tools":[{"type":"function","name":"search","description":"Look up a string","parameters":{"type":"object","properties":{}},"prompt_cache_breakpoint":{"ttl":"12h"}}]}`),
	} {
		var request openai.ResponseAPIRequest
		require.NoError(t, json.Unmarshal(raw, &request))
		_, _, _, err := normalizeResponseAPIRawBody(raw, &request, channeltype.OpenAI)
		require.ErrorContains(t, err, "12h")
	}
}

// TestGPT61TerraStandardPolicyPreservesOtherModels asserts non-Terra models
// keep their established service tier and cache passthrough behavior.
func TestGPT61TerraStandardPolicyPreservesOtherModels(t *testing.T) {
	raw := []byte(`{"model":"gpt-6-luna","input":"hello","service_tier":"fast"}`)
	var request openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal(raw, &request))
	wire, _, _, err := normalizeResponseAPIRawBody(raw, &request, channeltype.OpenAI)
	require.NoError(t, err)
	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(wire, &payload))
	require.Equal(t, `"fast"`, string(payload["service_tier"]))
}
