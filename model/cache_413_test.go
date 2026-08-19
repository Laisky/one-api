package model

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// This test ensures the 413 path's tryLargerMaxTokens filtering logic does not panic and
// returns an error when no candidates exist after filtering.
func TestCacheGetRandomSatisfiedChannelExcluding_413Filtering_NoCandidates(t *testing.T) {
	// Force memory cache path so we do not hit DB (which is not initialized in this unit test).
	original := config.MemoryCacheEnabled
	config.MemoryCacheEnabled = true
	defer func() { config.MemoryCacheEnabled = original }()

	// Ensure the in-memory structure is initialized but empty for target group/model.
	if group2model2channels == nil {
		group2model2channels = make(map[string]map[string][]*Channel)
	}
	if group2model2channels["default"] == nil {
		group2model2channels["default"] = make(map[string][]*Channel)
	}
	group2model2channels["default"]["gpt-3.5-turbo"] = []*Channel{} // explicitly empty

	_, err := CacheGetRandomSatisfiedChannelExcluding("default", "gpt-3.5-turbo", false, map[int]bool{1: true}, true)
	require.Error(t, err)
}
