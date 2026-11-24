package model

import (
	"context"
	"slices"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
)

// retentionTestCatalog returns a catalog with defaults Foo and Bar and the
// non-default catalog models Priced, Unpriced, and Mapped. Servable reports
// the names in servable, records every call in calls, and never mutates the
// channel. Parameters: servable is the servable set and calls receives each
// candidate list. Returns: the catalog.
func retentionTestCatalog(servable map[string]bool, calls *[][]string) ChannelResetCatalog {
	return ChannelResetCatalog{
		Defaults: []string{"Foo", "Bar"},
		Known:    []string{" Priced ", "Foo", "Unpriced", "", "Bad,Name", "Mapped", "Bar", "Priced"},
		Servable: func(_ context.Context, _ *Channel, candidates []string) (map[string]bool, error) {
			if calls != nil {
				*calls = append(*calls, candidates)
			}
			result := map[string]bool{"Injected": true}
			for _, name := range candidates {
				result[name] = servable[name]
			}
			return result, nil
		},
	}
}

// TestPlanChannelModelResetRetainsExplicitlyServableModels verifies retention:
// defaults first, then every listed or explicitly configured non-default
// catalog model the channel can serve, in catalog order, with overrides intact.
func TestPlanChannelModelResetRetainsExplicitlyServableModels(t *testing.T) {
	channel := Channel{
		Type:                   channeltype.AwsClaude,
		Models:                 "Custom,Unpriced,Foo,Priced",
		HiddenModels:           resetTestString(`["Priced"]`),
		ModelMapping:           resetTestString(`{"Mapped":"Foo","Foo":"Unpriced"}`),
		ModelConfigs:           resetTestString(`{"Priced":{"ratio":1,"completion_ratio":2}}`),
		InferenceProfileArnMap: resetTestString(`{"Priced":"private-arn"}`),
		TestingModel:           resetTestString("Priced"),
	}
	before := channel
	var calls [][]string
	planned, retained, err := planChannelModelReset(context.Background(), &channel,
		retentionTestCatalog(map[string]bool{"Priced": true, "Mapped": true}, &calls))
	require.NoError(t, err)
	require.Equal(t, [][]string{{"Priced", "Unpriced", "Mapped"}}, calls, "candidates are referenced non-defaults in catalog order")
	require.Equal(t, []string{"Priced", "Mapped"}, retained)
	require.Equal(t, "Foo,Bar,Priced,Mapped", planned.Models, "an injected name outside the candidates is ignored")
	require.Nil(t, planned.HiddenModels)
	require.Equal(t, resetTestString("Priced"), planned.TestingModel, "a retained testing model survives")
	require.Equal(t, channel.ModelMapping, planned.ModelMapping)
	require.Equal(t, channel.ModelConfigs, planned.ModelConfigs)
	require.Equal(t, channel.InferenceProfileArnMap, planned.InferenceProfileArnMap)
	require.Equal(t, before, channel, "planning never mutates the locked row")
}

// TestPlanChannelModelResetRetainsListedServableModel verifies a listed
// non-default catalog model with no override of its own is still a retention
// candidate, and is kept when the channel can serve it.
func TestPlanChannelModelResetRetainsListedServableModel(t *testing.T) {
	channel := Channel{Type: channeltype.OpenAI, Models: "Unpriced,Foo"}
	var calls [][]string
	planned, retained, err := planChannelModelReset(context.Background(), &channel,
		retentionTestCatalog(map[string]bool{"Unpriced": true}, &calls))
	require.NoError(t, err)
	require.Equal(t, [][]string{{"Unpriced"}}, calls)
	require.Equal(t, []string{"Unpriced"}, retained)
	require.Equal(t, "Foo,Bar,Unpriced", planned.Models)
}

// TestPlanChannelModelResetRetentionConflicts verifies that unsupported keys
// keep their historical codes, that recognized but unservable keys report
// tariff_incomplete for the field that references them, and that a mapping
// target only needs to be recognized by the provider.
func TestPlanChannelModelResetRetentionConflicts(t *testing.T) {
	servable := map[string]bool{"Priced": true}
	tests := []struct {
		name    string
		channel Channel
		code    string
		field   string
		models  []string
	}{
		{"incomplete tariff", Channel{ModelConfigs: resetTestString(`{"Unpriced":{"ratio":1}}`)}, "tariff_incomplete", "model_configs", []string{"Unpriced"}},
		{"unsupported pricing wins over incomplete", Channel{ModelConfigs: resetTestString(`{"Unpriced":{"ratio":1},"Retired":{"ratio":1}}`)}, "pricing_conflict", "model_configs", []string{"Retired"}},
		{"legacy input ratio", Channel{ModelRatio: resetTestString(`{"Unpriced":1}`)}, "tariff_incomplete", "model_ratio", []string{"Unpriced"}},
		{"legacy output ratio", Channel{CompletionRatio: resetTestString(`{"Unpriced":1}`)}, "tariff_incomplete", "completion_ratio", []string{"Unpriced"}},
		{"legacy unsupported", Channel{ModelRatio: resetTestString(`{"Retired":1}`)}, "pricing_conflict", "model_ratio", []string{"Retired"}},
		{"mapping alias unservable", Channel{ModelMapping: resetTestString(`{"Unpriced":"Unpriced"}`)}, "tariff_incomplete", "model_mapping", []string{"Unpriced"}},
		{"mapping alias unsupported", Channel{ModelMapping: resetTestString(`{"Alias":"Foo"}`)}, "mapping_conflict", "model_mapping", []string{"Alias"}},
		{"mapping target unsupported", Channel{ModelMapping: resetTestString(`{"Foo":"Retired"}`)}, "mapping_conflict", "model_mapping", []string{"Retired"}},
		{"mapping alias and target unsupported", Channel{ModelMapping: resetTestString(`{"Alias":"Retired","Unpriced":"Foo"}`)}, "mapping_conflict", "model_mapping", []string{"Alias", "Retired"}},
		{"profile unservable", Channel{InferenceProfileArnMap: resetTestString(`{"Unpriced":"private-arn"}`)}, "tariff_incomplete", "inference_profile_arn_map", []string{"Unpriced"}},
		{"profile unsupported", Channel{InferenceProfileArnMap: resetTestString(`{"Retired":"private-arn"}`)}, "inference_profile_conflict", "inference_profile_arn_map", []string{"Retired"}},
		{"malformed pricing before servability", Channel{ModelConfigs: resetTestString(`{"Priced":{"ratio":-1}}`)}, "invalid_configuration", "model_configs", nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := test.channel
			channel.Type = channeltype.AwsClaude
			channel.Models = "Foo"
			before := channel
			planned, retained, err := planChannelModelReset(context.Background(), &channel, retentionTestCatalog(servable, nil))
			require.Error(t, err)
			require.Nil(t, planned)
			require.Nil(t, retained)
			var conflict *ChannelModelResetConflict
			require.True(t, errors.As(err, &conflict))
			require.Equal(t, test.code, conflict.Code)
			require.Equal(t, test.field, conflict.Field)
			require.Equal(t, test.models, conflict.Models)
			require.NotContains(t, err.Error(), "private-arn")
			require.Equal(t, before, channel)
		})
	}

	t.Run("mapping target outside the kept set is allowed", func(t *testing.T) {
		channel := Channel{Type: channeltype.AwsClaude, Models: "Foo", ModelMapping: resetTestString(`{"Foo":"Unpriced"}`)}
		planned, retained, err := planChannelModelReset(context.Background(), &channel, retentionTestCatalog(servable, nil))
		require.NoError(t, err)
		require.Empty(t, retained)
		require.Equal(t, "Foo,Bar", planned.Models)
	})
}

// TestPlanChannelModelResetWithoutServability verifies a catalog without a
// servability callback retains nothing, so an explicitly configured
// non-default model is refused instead of silently dropped or kept.
func TestPlanChannelModelResetWithoutServability(t *testing.T) {
	catalog := retentionTestCatalog(nil, nil)
	catalog.Servable = nil
	channel := Channel{Type: channeltype.OpenAI, Models: "Foo,Priced"}
	planned, retained, err := planChannelModelReset(context.Background(), &channel, catalog)
	require.NoError(t, err)
	require.Empty(t, retained)
	require.Equal(t, "Foo,Bar", planned.Models, "a listed non-default model without overrides is dropped")

	channel.ModelConfigs = resetTestString(`{"Priced":{"ratio":1,"completion_ratio":2}}`)
	_, _, err = planChannelModelReset(context.Background(), &channel, catalog)
	var conflict *ChannelModelResetConflict
	require.True(t, errors.As(err, &conflict))
	require.Equal(t, "tariff_incomplete", conflict.Code)
}

// TestPlanChannelModelResetServabilityFailure verifies a servability error is
// an operational failure rather than a policy conflict.
func TestPlanChannelModelResetServabilityFailure(t *testing.T) {
	catalog := retentionTestCatalog(nil, nil)
	catalog.Servable = func(context.Context, *Channel, []string) (map[string]bool, error) {
		return nil, errors.New("readiness unavailable")
	}
	channel := Channel{Type: channeltype.OpenAI, Models: "Foo,Priced"}
	planned, retained, err := planChannelModelReset(context.Background(), &channel, catalog)
	require.Error(t, err)
	require.Nil(t, planned)
	require.Nil(t, retained)
	var conflict *ChannelModelResetConflict
	require.False(t, errors.As(err, &conflict))
	require.ErrorContains(t, err, "readiness unavailable")
}

// TestResetChannelModelsToDefaultsPersistsRetainedModels verifies the stored
// list and routing abilities include retained models for every group, that
// overrides are untouched, and that a servability failure rolls back.
func TestResetChannelModelsToDefaultsPersistsRetainedModels(t *testing.T) {
	db := useChannelPersistenceTestDB(t)
	channel := &Channel{
		Type: channeltype.OpenAI, Name: "retention", Status: ChannelStatusEnabled,
		Models: "Unpriced,Foo", Group: "default,vip",
		ModelConfigs: resetTestString(`{"Priced":{"ratio":1,"completion_ratio":2}}`),
	}
	require.NoError(t, channel.Insert())
	catalog := func(int) ChannelResetCatalog { return retentionTestCatalog(map[string]bool{"Priced": true}, nil) }
	_, err := ResetChannelModelsToDefaults(context.Background(), channel.Id, catalog)
	require.NoError(t, err)
	var stored Channel
	require.NoError(t, db.First(&stored, "id = ?", channel.Id).Error)
	require.Equal(t, "Foo,Bar,Priced", stored.Models)
	require.Equal(t, channel.ModelConfigs, stored.ModelConfigs)
	var abilities []Ability
	require.NoError(t, db.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
	var keys []string
	for _, ability := range abilities {
		require.True(t, ability.Enabled)
		keys = append(keys, ability.Group+"/"+ability.Model)
	}
	slices.Sort(keys)
	require.Equal(t, []string{"default/Bar", "default/Foo", "default/Priced", "vip/Bar", "vip/Foo", "vip/Priced"}, keys)

	failing := func(int) ChannelResetCatalog {
		value := retentionTestCatalog(nil, nil)
		value.Servable = func(context.Context, *Channel, []string) (map[string]bool, error) {
			return nil, errors.New("readiness unavailable")
		}
		return value
	}
	require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Update("models", "Priced").Error)
	var before Channel
	require.NoError(t, db.First(&before, "id = ?", channel.Id).Error)
	_, err = ResetChannelModelsToDefaults(context.Background(), channel.Id, failing)
	require.Error(t, err)
	var after Channel
	require.NoError(t, db.First(&after, "id = ?", channel.Id).Error)
	require.Equal(t, before, after)
}
