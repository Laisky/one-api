package adaptor

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestModelBillingPolicyZeroValueIsUnrestricted keeps undeclared models on
// their historical billing behavior. Parameter t reports failures.
func TestModelBillingPolicyZeroValueIsUnrestricted(t *testing.T) {
	t.Parallel()
	var policy ModelBillingPolicy
	require.False(t, policy.RestrictsServiceTier())
	require.Empty(t, policy.PinnedServiceTier())
	for _, tier := range []string{"", "default", "fast", "flex", "priority"} {
		require.True(t, policy.SupportsServiceTier(tier), tier)
	}
	require.True(t, policy.PermitsWebSocket())
	for _, tokens := range []int{-1, 0, 272_000, 272_001, 10_000_000} {
		require.True(t, policy.HasCacheWriteTariff(tokens), tokens)
	}
	require.False(t, policy.NativeTariffOnly)
	require.False(t, policy.CaptureRawUsageReceipt)
}

// TestModelBillingPolicyRestrictions checks each declared restriction and the
// inclusive long-context threshold. Parameter t reports failures.
func TestModelBillingPolicyRestrictions(t *testing.T) {
	t.Parallel()
	policy := ModelBillingPolicy{
		SupportedServiceTiers:             []string{"default", "flex"},
		HTTPOnly:                          true,
		CacheWriteUnpricedFromInputTokens: 272_001,
	}
	require.True(t, policy.RestrictsServiceTier())
	require.Equal(t, "default", policy.PinnedServiceTier())
	for _, tier := range []string{"default", " Default\t", "FLEX"} {
		require.True(t, policy.SupportsServiceTier(tier), tier)
	}
	for _, tier := range []string{"", "  ", "fast", "auto", "priority", "default-fast"} {
		require.False(t, policy.SupportsServiceTier(tier), tier)
	}
	require.False(t, policy.PermitsWebSocket())
	require.True(t, policy.HasCacheWriteTariff(0))
	require.True(t, policy.HasCacheWriteTariff(272_000))
	require.False(t, policy.HasCacheWriteTariff(272_001))
	require.False(t, policy.HasCacheWriteTariff(1_050_000))
}

// TestModelBillingPolicyCloneIsIndependent ensures callers cannot mutate a
// shared catalog slice through a copy. Parameter t reports failures.
func TestModelBillingPolicyCloneIsIndependent(t *testing.T) {
	t.Parallel()
	shared := []string{"default"}
	original := ModelBillingPolicy{SupportedServiceTiers: shared, HTTPOnly: true}
	clone := original.Clone()
	require.Equal(t, original, clone)
	clone.SupportedServiceTiers[0] = "fast"
	require.Equal(t, []string{"default"}, shared)
}

// TestModelBillingPolicyLookupInstallation checks that the installed catalog
// lookup is consulted and that clearing it declares no policy. The test swaps
// process-wide state, so it is sequential and restores the previous lookup.
// Parameter t reports failures.
func TestModelBillingPolicyLookupInstallation(t *testing.T) {
	previous := modelBillingPolicyLookup.Load()
	t.Cleanup(func() { modelBillingPolicyLookup.Store(previous) })

	SetModelBillingPolicyLookup(func(modelName string) (ModelBillingPolicy, bool) {
		if strings.EqualFold(strings.TrimSpace(modelName), "policy-model") {
			return ModelBillingPolicy{HTTPOnly: true}, true
		}
		return ModelBillingPolicy{}, false
	})
	policy, declared := LookupModelBillingPolicy(" POLICY-MODEL ")
	require.True(t, declared)
	require.True(t, policy.HTTPOnly)
	_, declared = LookupModelBillingPolicy("other-model")
	require.False(t, declared)

	SetModelBillingPolicyLookup(nil)
	_, declared = LookupModelBillingPolicy("policy-model")
	require.False(t, declared)
}
