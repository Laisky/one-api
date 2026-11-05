package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPricingUpdatePreservesConcurrentFieldsMultiDB verifies stale pricing saves
// preserve a concurrent name and configuration update on each supported database.
// Parameters: t owns the test. Returns: no values.
func TestPricingUpdatePreservesConcurrentFieldsMultiDB(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := useTestingModelMatrixBackend(t, dialect)
			channel := insertTestingModelMatrixChannel(t, dialect, "pricing", nil)
			var stale Channel
			require.NoError(t, db.First(&stale, channel.Id).Error)
			require.NoError(t, stale.SetModelPriceConfigs(map[string]ModelConfigLocal{"gpt-4o": {Ratio: 2}}))
			require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{
				"name": "concurrent-name", "config": `{"region":"concurrent"}`,
				"testing_model": ChannelTestingModelSkip,
			}).Error)
			tooling := &ChannelToolingConfig{Whitelist: []string{"code_interpreter"}}
			require.NoError(t, stale.UpdatePricingWithContext(context.Background(), true, tooling, true))
			var saved Channel
			require.NoError(t, db.First(&saved, channel.Id).Error)
			require.Equal(t, "concurrent-name", saved.Name)
			require.Contains(t, saved.Config, `"region":"concurrent"`)
			require.NotNil(t, saved.TestingModel)
			require.Equal(t, ChannelTestingModelSkip, *saved.TestingModel)
			require.Equal(t, float64(2), saved.GetModelPriceConfigs()["gpt-4o"].Ratio)
			require.Equal(t, []string{"code_interpreter"}, saved.GetToolingConfig().Whitelist)

			require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).
				Update("model_configs", `{"gpt-4o":{"ratio":3}}`).Error)
			require.NoError(t, stale.UpdatePricingWithContext(context.Background(), false,
				&ChannelToolingConfig{Whitelist: []string{"web_search"}}, true))
			require.NoError(t, db.First(&saved, channel.Id).Error)
			require.Equal(t, float64(3), saved.GetModelPriceConfigs()["gpt-4o"].Ratio)
			require.Equal(t, []string{"web_search"}, saved.GetToolingConfig().Whitelist)
			require.Contains(t, saved.Config, `"region":"concurrent"`)
		})
	}
}
