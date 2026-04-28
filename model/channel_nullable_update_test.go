package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
)

// TestChannelUpdateNullableFieldsClearedWhenProvided verifies that Channel.Update()
// honors the NullableFieldsProvided map for the four nullable text columns
// (model_mapping, model_configs, system_prompt, inference_profile_arn_map):
//
//   - When the column is flagged in NullableFieldsProvided and the field on the
//     struct is nil, the DB column must be cleared (NULL).
//   - When the column is flagged and the field is a pointer to "", the DB column
//     must be persisted as the empty string.
//   - When the column is NOT flagged, GORM's zero-value-skip behavior must be
//     preserved so existing values stay intact (backward compatibility).
func TestChannelUpdateNullableFieldsClearedWhenProvided(t *testing.T) {
	testDB := setupTestDB(t)
	originalDB := DB
	DB = testDB
	defer func() { DB = originalDB }()

	originalUsingSQLite := common.UsingSQLite.Load()
	common.UsingSQLite.Store(true)
	defer func() { common.UsingSQLite.Store(originalUsingSQLite) }()

	// insertSeededChannel creates a fresh channel with all four nullable text
	// fields populated with a non-empty value. Returns the inserted channel
	// reloaded from the DB so callers see the persisted state.
	insertSeededChannel := func(t *testing.T, label string) *Channel {
		t.Helper()
		group := fmt.Sprintf("group-%s-%d", label, time.Now().UnixNano())
		ch := &Channel{
			Name:                   fmt.Sprintf("nullable-%s-%d", label, time.Now().UnixNano()),
			Type:                   1,
			Status:                 ChannelStatusEnabled,
			Models:                 "gpt-3.5-turbo",
			Group:                  group,
			ModelMapping:           stringPtr(`{"foo":"bar"}`),
			ModelConfigs:           stringPtr(`{"gpt-3.5-turbo":{"ratio":1}}`),
			SystemPrompt:           stringPtr("you are a helpful assistant"),
			InferenceProfileArnMap: stringPtr(`{"gpt-3.5-turbo":"arn:aws:bedrock:us-east-1:123:inference-profile/foo"}`),
		}
		require.NoError(t, ch.Insert())

		var reloaded Channel
		require.NoError(t, DB.First(&reloaded, "id = ?", ch.Id).Error)
		require.NotNil(t, reloaded.ModelMapping)
		require.NotNil(t, reloaded.ModelConfigs)
		require.NotNil(t, reloaded.SystemPrompt)
		require.NotNil(t, reloaded.InferenceProfileArnMap)
		return &reloaded
	}

	t.Run("nil-pointer with provided flag clears DB column", func(t *testing.T) {
		channel := insertSeededChannel(t, "clear")

		channel.ModelMapping = nil
		channel.ModelConfigs = nil
		channel.SystemPrompt = nil
		channel.InferenceProfileArnMap = nil
		channel.NullableFieldsProvided = map[string]bool{
			"model_mapping":             true,
			"model_configs":             true,
			"system_prompt":             true,
			"inference_profile_arn_map": true,
		}
		require.NoError(t, channel.Update())

		var got Channel
		require.NoError(t, DB.First(&got, "id = ?", channel.Id).Error)

		// SQLite stores NULL for *string set to nil; round-trip should yield a
		// nil pointer. Be permissive in case any layer normalizes NULL to "".
		assertClearedOrEmpty := func(t *testing.T, label string, p *string) {
			t.Helper()
			if p == nil {
				return
			}
			assert.Equal(t, "", *p, "%s should be nil or empty after clear", label)
		}
		assertClearedOrEmpty(t, "ModelMapping", got.ModelMapping)
		assertClearedOrEmpty(t, "ModelConfigs", got.ModelConfigs)
		assertClearedOrEmpty(t, "SystemPrompt", got.SystemPrompt)
		assertClearedOrEmpty(t, "InferenceProfileArnMap", got.InferenceProfileArnMap)
	})

	t.Run("empty-string pointer with provided flag persists empty string", func(t *testing.T) {
		channel := insertSeededChannel(t, "empty")

		empty := ""
		channel.ModelMapping = &empty
		channel.ModelConfigs = stringPtr("")
		channel.SystemPrompt = stringPtr("")
		channel.InferenceProfileArnMap = stringPtr("")
		channel.NullableFieldsProvided = map[string]bool{
			"model_mapping":             true,
			"model_configs":             true,
			"system_prompt":             true,
			"inference_profile_arn_map": true,
		}
		require.NoError(t, channel.Update())

		var got Channel
		require.NoError(t, DB.First(&got, "id = ?", channel.Id).Error)

		require.NotNil(t, got.ModelMapping, "ModelMapping should not be nil after persisting empty string")
		assert.Equal(t, "", *got.ModelMapping)
		require.NotNil(t, got.ModelConfigs)
		assert.Equal(t, "", *got.ModelConfigs)
		require.NotNil(t, got.SystemPrompt)
		assert.Equal(t, "", *got.SystemPrompt)
		require.NotNil(t, got.InferenceProfileArnMap)
		assert.Equal(t, "", *got.InferenceProfileArnMap)
	})

	t.Run("nil pointer WITHOUT provided flag preserves previous value", func(t *testing.T) {
		channel := insertSeededChannel(t, "preserve")

		// snapshot the originals for later comparison
		origModelMapping := *channel.ModelMapping
		origModelConfigs := *channel.ModelConfigs
		origSystemPrompt := *channel.SystemPrompt
		origInferenceArn := *channel.InferenceProfileArnMap

		// Frontend did not include these fields in the payload at all -> nil
		// struct values + empty NullableFieldsProvided map. Backward-compat
		// path: GORM should skip nil pointers and the existing values must
		// remain.
		channel.ModelMapping = nil
		channel.ModelConfigs = nil
		channel.SystemPrompt = nil
		channel.InferenceProfileArnMap = nil
		channel.NullableFieldsProvided = map[string]bool{}
		require.NoError(t, channel.Update())

		var got Channel
		require.NoError(t, DB.First(&got, "id = ?", channel.Id).Error)

		require.NotNil(t, got.ModelMapping, "ModelMapping must be preserved when not provided")
		assert.Equal(t, origModelMapping, *got.ModelMapping)
		require.NotNil(t, got.ModelConfigs, "ModelConfigs must be preserved when not provided")
		assert.Equal(t, origModelConfigs, *got.ModelConfigs)
		require.NotNil(t, got.SystemPrompt, "SystemPrompt must be preserved when not provided")
		assert.Equal(t, origSystemPrompt, *got.SystemPrompt)
		require.NotNil(t, got.InferenceProfileArnMap, "InferenceProfileArnMap must be preserved when not provided")
		assert.Equal(t, origInferenceArn, *got.InferenceProfileArnMap)
	})
}
