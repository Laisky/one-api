package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseChannelJSONFieldsMatchSavedChannelReaders proves the draft parsers
// the channel editor's readiness check uses decode exactly what the relay's
// saved-channel readers decode, including the empty, "{}", and malformed
// cases, so a draft cannot be judged under different rules than its saved row.
func TestParseChannelJSONFieldsMatchSavedChannelReaders(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	for _, tc := range []struct {
		name      string
		raw       *string
		malformed bool
		unset     bool
	}{
		{name: "nil", unset: true},
		{name: "empty", raw: strPtr(""), unset: true},
		{name: "empty_object", raw: strPtr("{}"), unset: true},
		{name: "spaced_empty_object", raw: strPtr("{ }")},
		{name: "null", raw: strPtr("null")},
		{name: "array", raw: strPtr("[]"), malformed: true},
		{name: "truncated", raw: strPtr(`{"a":`), malformed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			channel := &Channel{ModelMapping: tc.raw, ModelConfigs: tc.raw}
			mapping, mappingErr := ParseModelMapping(tc.raw)
			configs, configsErr := ParseModelPriceConfigs(tc.raw)
			if tc.malformed {
				require.Error(t, mappingErr)
				require.Error(t, configsErr)
				require.Nil(t, channel.GetModelMappingWithContext(context.Background()))
				require.Nil(t, channel.GetModelPriceConfigsWithContext(context.Background()))
				return
			}
			require.NoError(t, mappingErr)
			require.NoError(t, configsErr)
			if tc.unset {
				// An unset column reads as nil, the documented "no mapping/pricing" value.
				require.Nil(t, mapping)
				require.Nil(t, configs)
			}
			require.Equal(t, channel.GetModelMappingWithContext(context.Background()), mapping)
			require.Equal(t, channel.GetModelPriceConfigsWithContext(context.Background()), configs)
		})
	}

	mapping, err := ParseModelMapping(strPtr(`{"alias":"gpt-6-cyber"}`))
	require.NoError(t, err)
	require.Equal(t, map[string]string{"alias": "gpt-6-cyber"}, mapping)

	_, err = ParseModelMapping(strPtr(`{"alias":1}`))
	require.Error(t, err, "a mapping target must be a model name")

	configs, err := ParseModelPriceConfigs(strPtr(`{"gpt-6-cyber":{"ratio":2,"completion_ratio":6,"image":{"price_per_image_usd":0.04}}}`))
	require.NoError(t, err)
	require.Equal(t, 2.0, configs["gpt-6-cyber"].Ratio)
	require.Equal(t, 6.0, configs["gpt-6-cyber"].CompletionRatio)
	require.NotNil(t, configs["gpt-6-cyber"].Image)
	require.Equal(t, 0.04, configs["gpt-6-cyber"].Image.PricePerImageUsd)
}
