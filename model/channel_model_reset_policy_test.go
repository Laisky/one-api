package model

import (
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
)

// resetTestString returns a pointer to the supplied test configuration string.
func resetTestString(value string) *string { return &value }

// TestPlanChannelModelResetRejectsUnsafeConfigurations verifies fail-closed
// preflight for incompatible catalogs, mapping/pricing conflicts, and invalid JSON.
func TestPlanChannelModelResetRejectsUnsafeConfigurations(t *testing.T) {
	tests := []struct {
		name     string
		channel  Channel
		defaults []string
		code     string
		field    string
	}{
		{"openai compatible", Channel{Type: channeltype.OpenAICompatible}, []string{"Foo"}, "unsupported_channel", "type"},
		{"claude compatible", Channel{Type: channeltype.ClaudeCompatible}, []string{"Foo"}, "unsupported_channel", "type"},
		{"legacy custom", Channel{Type: channeltype.Custom}, []string{"Foo"}, "unsupported_channel", "type"},
		{"unknown type", Channel{Type: channeltype.Dummy}, []string{"Foo"}, "no_default_models", "type"},
		{"no catalog", Channel{Type: channeltype.OpenAI}, nil, "no_default_models", "models"},
		{"empty catalog", Channel{Type: channeltype.OpenAI}, []string{"", " "}, "no_default_models", "models"},
		{"invalid catalog", Channel{Type: channeltype.OpenAI}, []string{"Foo,Bar"}, "no_default_models", "models"},
		{"mapping alias removed", Channel{Type: channeltype.OpenAI, ModelMapping: resetTestString(`{"Alias":"Foo"}`)}, []string{"Foo"}, "mapping_conflict", "model_mapping"},
		{"mapping target removed", Channel{Type: channeltype.OpenAI, ModelMapping: resetTestString(`{"Foo":"Retired"}`)}, []string{"Foo"}, "mapping_conflict", "model_mapping"},
		{"mapping case mismatch", Channel{Type: channeltype.OpenAI, ModelMapping: resetTestString(`{"foo":"Foo"}`)}, []string{"Foo"}, "mapping_conflict", "model_mapping"},
		{"pricing removed", Channel{Type: channeltype.OpenAI, ModelConfigs: resetTestString(`{"Retired":{"ratio":2}}`)}, []string{"Foo"}, "pricing_conflict", "model_configs"},
		{"legacy input removed", Channel{Type: channeltype.OpenAI, ModelRatio: resetTestString(`{"Retired":2}`)}, []string{"Foo"}, "pricing_conflict", "model_ratio"},
		{"legacy output removed", Channel{Type: channeltype.OpenAI, CompletionRatio: resetTestString(`{"Retired":2}`)}, []string{"Foo"}, "pricing_conflict", "completion_ratio"},
		{"profile removed", Channel{Type: channeltype.AwsClaude, InferenceProfileArnMap: resetTestString(`{"Retired":"private-arn"}`)}, []string{"Foo"}, "inference_profile_conflict", "inference_profile_arn_map"},
		{"malformed mapping", Channel{Type: channeltype.OpenAI, ModelMapping: resetTestString(`{bad-secret`)}, []string{"Foo"}, "invalid_configuration", "model_mapping"},
		{"mapping wrong type", Channel{Type: channeltype.OpenAI, ModelMapping: resetTestString(`{"Foo":42}`)}, []string{"Foo"}, "invalid_configuration", "model_mapping"},
		{"mapping null entry", Channel{Type: channeltype.OpenAI, ModelMapping: resetTestString(`{"Foo":null}`)}, []string{"Foo"}, "invalid_configuration", "model_mapping"},
		{"pricing array", Channel{Type: channeltype.OpenAI, ModelConfigs: resetTestString(`[]`)}, []string{"Foo"}, "invalid_configuration", "model_configs"},
		{"pricing wrong type", Channel{Type: channeltype.OpenAI, ModelConfigs: resetTestString(`{"Foo":{"ratio":"secret"}}`)}, []string{"Foo"}, "invalid_configuration", "model_configs"},
		{"pricing null entry", Channel{Type: channeltype.OpenAI, ModelConfigs: resetTestString(`{"Foo":null}`)}, []string{"Foo"}, "invalid_configuration", "model_configs"},
		{"config malformed", Channel{Type: channeltype.OpenAI, Config: "secret-not-json"}, []string{"Foo"}, "invalid_configuration", "config"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			original := test.channel
			original.Models = "Old"
			before := original
			planned, err := planChannelModelReset(&original, test.defaults)
			require.Error(t, err)
			require.Nil(t, planned)
			var conflict *ChannelModelResetConflict
			require.True(t, errors.As(err, &conflict))
			require.Equal(t, test.code, conflict.Code)
			require.Equal(t, test.field, conflict.Field)
			require.NotContains(t, err.Error(), "secret")
			require.NotContains(t, err.Error(), "private-arn")
			require.Equal(t, before, original)
		})
	}
}

// TestPlanChannelModelResetPreservesCompatibleOverrides verifies that known
// providers using an OpenAI protocol are not confused with custom compatible types.
func TestPlanChannelModelResetPreservesCompatibleOverrides(t *testing.T) {
	channel := Channel{
		Type:                  channeltype.GeminiOpenAICompatible,
		Models:                "Retired,Foo",
		HiddenModels:          resetTestString(`["Foo"]`),
		ModelMapping:          resetTestString(`{"Foo":"Bar"}`),
		ModelConfigs:          resetTestString(`{"Foo":{"ratio":2,"completion_ratio":3}}`),
		ModelRatio:            resetTestString(`{"Foo":2}`),
		CompletionRatio:       resetTestString(`{"Bar":3}`),
		InferenceProfileArnMap: resetTestString(`{"Foo":"private-arn"}`),
	}
	defaults := []string{" Foo ", "Bar", "foo", "Foo", ""}
	planned, err := planChannelModelReset(&channel, defaults)
	require.NoError(t, err)
	require.Equal(t, "Foo,Bar,foo", planned.Models)
	require.Nil(t, planned.HiddenModels)
	require.Equal(t, channel.ModelMapping, planned.ModelMapping)
	require.Equal(t, channel.ModelConfigs, planned.ModelConfigs)
	require.Equal(t, channel.ModelRatio, planned.ModelRatio)
	require.Equal(t, channel.CompletionRatio, planned.CompletionRatio)
	require.Equal(t, channel.InferenceProfileArnMap, planned.InferenceProfileArnMap)
	require.Equal(t, "Retired,Foo", channel.Models)
	require.Equal(t, []string{" Foo ", "Bar", "foo", "Foo", ""}, defaults)
}

// TestPlanChannelModelResetTestingModel verifies valid selections and the explicit
// skip sentinel survive, while removed testing models revert to automatic choice.
func TestPlanChannelModelResetTestingModel(t *testing.T) {
	for _, name := range []string{"Foo", ChannelTestingModelSkip, "Retired", ""} {
		t.Run(name, func(t *testing.T) {
			channel := Channel{Type: channeltype.OpenAI, TestingModel: resetTestString(name)}
			planned, err := planChannelModelReset(&channel, []string{"Foo"})
			require.NoError(t, err)
			if name == "Foo" || name == ChannelTestingModelSkip {
				require.Equal(t, channel.TestingModel, planned.TestingModel)
			} else {
				require.Nil(t, planned.TestingModel)
			}
		})
	}
}
