package relay_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestOctoberCatalogueReviewCoverage requires one explicit disposition for every
// adaptor directory, including helpers, using t. It returns no values and checks
// review coverage rather than claiming current upstream availability.
func TestOctoberCatalogueReviewCoverage(t *testing.T) {
	t.Parallel()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Dir(filepath.Dir(file))
	entries, err := os.ReadDir(filepath.Join(root, "relay", "adaptor"))
	require.NoError(t, err)
	var directories []string
	for _, entry := range entries {
		if entry.IsDir() {
			directories = append(directories, entry.Name())
		}
	}
	text, err := os.ReadFile(filepath.Join(root, "docs", "adaptor-model-refresh-20261012.md"))
	require.NoError(t, err)
	pattern := regexp.MustCompile("(?m)^\\| `([A-Za-z0-9_]+)` \\| ([a-z]+) \\|")
	allowed := map[string]bool{"updated": true, "shared": true, "retained": true, "configured": true, "limited": true, "helper": true}
	var reviewed []string
	for _, match := range pattern.FindAllSubmatch(text, -1) {
		require.True(t, allowed[string(match[2])], "unknown review disposition")
		reviewed = append(reviewed, string(match[1]))
	}
	require.ElementsMatch(t, directories, reviewed, "every directory needs exactly one disposition, including evidence limitations")
	for _, match := range regexp.MustCompile(`\]\(([a-z0-9-]+-audit-20261012\.md)\)`).FindAllSubmatch(text, -1) {
		_, err := os.Stat(filepath.Join(root, "docs", string(match[1])))
		require.NoError(t, err, "linked provider evidence must exist")
	}
	require.Contains(t, string(text), "Directory coverage is not a claim")
}

// TestOctoberExplicitFreeModelsDoNotUseUnknownFallback verifies free-model
// discovery and billing separately from paid and unknown models using t. It
// returns no values and does not relax the positive-price paid-model smoke test.
func TestOctoberExplicitFreeModelsDoNotUseUnknownFallback(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, free, paid string
		provider         adaptor.Adaptor
	}{
		{"Spark WebSocket", "Spark-Lite", "Spark-Pro", relay.GetAdaptor(apitype.Xunfei)},
		{"Spark HTTP", "lite", "generalv3", &openai.Adaptor{ChannelType: channeltype.XunfeiV2}},
		{"Cohere rate-limited API", "command-a-vision-07-2025", "command", relay.GetAdaptor(apitype.Cohere)},
		{"NVIDIA hosted trial", "z-ai/glm-5.3", "", relay.GetAdaptor(apitype.NVIDIA)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.NotNil(t, tc.provider)
			cfg, exists := tc.provider.GetDefaultModelPricing()[tc.free]
			require.True(t, exists, "zero is explicit provider data, not a missing map entry")
			require.Contains(t, tc.provider.GetModelList(), tc.free)
			require.Zero(t, cfg.Ratio)
			require.Zero(t, tc.provider.GetModelRatio(tc.free))
			require.Zero(t, tc.provider.GetModelRatio(tc.free)*tc.provider.GetCompletionRatio(tc.free))
			const unknown = "__unpublished_catalogue_regression_model__"
			require.NotContains(t, tc.provider.GetDefaultModelPricing(), unknown)
			require.Greater(t, tc.provider.GetModelRatio(unknown), 0.0, "unknown pricing must not silently become free")
			if tc.paid != "" {
				require.Contains(t, tc.provider.GetDefaultModelPricing(), tc.paid)
				require.Greater(t, tc.provider.GetModelRatio(tc.paid), 0.0)
				require.Greater(t, tc.provider.GetModelRatio(tc.paid)*tc.provider.GetCompletionRatio(tc.paid), 0.0)
			}
		})
	}
}
