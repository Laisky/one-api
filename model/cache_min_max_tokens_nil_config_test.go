package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// minMaxTokensChannel builds a cache-test channel for model "target" at
// priority whose model configs are configs (empty for no model config at all).
//
// Parameters:
//   - id, priority: the channel identity and routing tier.
//   - configs: the raw model_configs JSON, or "" for none.
//
// Return values:
//   - *Channel: the unsaved channel.
func minMaxTokensChannel(id int, priority int64, configs string) *Channel {
	channel := abilityCacheChannel(id, priority)
	if configs != "" {
		channel.ModelConfigs = &configs
	}
	return channel
}

// minMaxTokensPicks runs the first-pass cached selection rounds times with
// DEFAULT_USE_MIN_MAX_TOKENS_MODEL enabled by the caller and returns the set of
// selected channel IDs, failing the test on a panic or an error.
//
// Parameters:
//   - t: the test handle.
//   - rounds: how many selections to run.
//
// Return values:
//   - map[int]bool: the IDs that were selected at least once.
func minMaxTokensPicks(t *testing.T, rounds int) map[int]bool {
	t.Helper()
	seen := map[int]bool{}
	for range rounds {
		var channel *Channel
		var err error
		require.NotPanics(t, func() {
			channel, err = CacheGetRandomSatisfiedChannelWithContext(context.Background(), "default", "target", false)
		})
		require.NoError(t, err)
		seen[channel.Id] = true
	}
	return seen
}

// TestMinMaxTokensSelectionToleratesNilModelConfig reproduces the review
// finding that DEFAULT_USE_MIN_MAX_TOKENS_MODEL dereferenced a nil model config:
// when a top-tier channel had no model config for the requested model, the
// smallest-limit preference read MaxTokens through nil and panicked the request.
// The selection must prefer the smallest declared limit and treat undeclared
// limits as unbounded.
func TestMinMaxTokensSelectionToleratesNilModelConfig(t *testing.T) {
	original := config.DefaultUseMinMaxTokensModel
	config.DefaultUseMinMaxTokensModel = true
	t.Cleanup(func() { config.DefaultUseMinMaxTokensModel = original })

	t.Run("all declared", func(t *testing.T) {
		setupAbilityCacheTest(t,
			minMaxTokensChannel(1, 10, `{"target": {"max_tokens": 8192}}`),
			minMaxTokensChannel(2, 10, `{"target": {"max_tokens": 4096}}`),
		)
		require.Equal(t, map[int]bool{2: true}, minMaxTokensPicks(t, 30), "control: the smallest declared limit is preferred")
	})

	t.Run("declared and missing", func(t *testing.T) {
		setupAbilityCacheTest(t,
			minMaxTokensChannel(1, 10, `{"target": {"max_tokens": 4096}}`),
			minMaxTokensChannel(2, 10, ""),
		)
		require.Equal(t, map[int]bool{1: true}, minMaxTokensPicks(t, 30), "the declared limit is preferred over a missing config")
	})

	t.Run("mixed declared, unset and missing", func(t *testing.T) {
		setupAbilityCacheTest(t,
			minMaxTokensChannel(1, 10, ""),
			minMaxTokensChannel(2, 10, `{"target": {"max_tokens": 8192}}`),
			minMaxTokensChannel(3, 10, `{"target": {"max_tokens": 4096}}`),
			minMaxTokensChannel(4, 10, `{"target": {"ratio": 1}}`),
		)
		require.Equal(t, map[int]bool{3: true}, minMaxTokensPicks(t, 30), "the smallest declared limit is preferred")
	})

	t.Run("single missing", func(t *testing.T) {
		setupAbilityCacheTest(t, minMaxTokensChannel(1, 10, ""))
		require.Equal(t, map[int]bool{1: true}, minMaxTokensPicks(t, 10))
	})

	t.Run("all undeclared", func(t *testing.T) {
		setupAbilityCacheTest(t,
			minMaxTokensChannel(1, 10, ""),
			minMaxTokensChannel(2, 10, `{"target": {"ratio": 1}}`),
		)
		require.Equal(t, map[int]bool{1: true, 2: true}, minMaxTokensPicks(t, 60), "without declared limits the whole tier stays selectable")
	})
}
