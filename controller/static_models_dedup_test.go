package controller

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStaticModelsAreDeterministicallyDeduped pins the contract that the
// aggregated static catalog carries exactly one row per model id, with the winner
// chosen by byte order of the owning adaptor name.
//
// Before dedupeStaticModelsByOwner existed, /v1/models emitted a row per provider
// for every shared id -- 309 ids were duplicated, and modelsMap resolved the owner
// by whichever apitype happened to be swept last, so the label moved whenever the
// enum grew.
func TestStaticModelsAreDeterministicallyDeduped(t *testing.T) {
	t.Parallel()

	byID := make(map[string][]string, len(allModels))
	for _, m := range allModels {
		byID[m.Id] = append(byID[m.Id], m.OwnedBy)
	}

	dupes := make([]string, 0)
	for id, owners := range byID {
		if len(owners) > 1 {
			dupes = append(dupes, id)
		}
	}
	sort.Strings(dupes)
	require.Empty(t, dupes, "every model id must appear exactly once in the static catalog")
	require.Len(t, byID, len(allModels))
}

// TestSharedGLMIdOwnedDeterministically pins the FALLBACK tie-break for the Zhipu
// (open.bigmodel.cn) and Zai (api.z.ai) channels, which are two brands of the
// same company advertising the same GLM model ids.
//
// allModels is the compiled-in catalog, built at init() before any channel is
// readable, so byte order is all there is to rank by and "zai" sorts before
// "zhipu". This label is NOT what /v1/models reports: once channels exist the
// owner is resolved from the channel backing the ability (see
// TestResolveUserAvailableModelsOwnerFollowsChannelPriority), so a deployment
// running only a Zhipu channel reports "zhipu".
//
// Either way it is a display concern: billing resolves per request through the
// channel's own apitype, so glm-4.7 bills at CNY tiers on a Zhipu channel and at
// flat USD on a Zai channel regardless of the owner shown in any listing.
func TestSharedGLMIdOwnedDeterministically(t *testing.T) {
	t.Parallel()

	owners := make(map[string]string, len(allModels))
	for _, m := range allModels {
		owners[m.Id] = m.OwnedBy
	}

	require.Equal(t, "zai", owners["glm-4.7"])
	require.Equal(t, "zai", owners["glm-5.3"])

	// Z.AI-exclusive ids survive the dedup rather than being dropped with the
	// rest of the overlapping catalog.
	require.Equal(t, "zai", owners["glm-4-32b-0414-128k"])

	// BigModel-exclusive ids keep their own owner.
	require.Equal(t, "zhipu", owners["embedding-3"])
	require.Equal(t, "zhipu", owners["glm-realtime-flash"])
}

// TestDedupeStaticModelsByOwnerRule exercises the helper directly, including the
// stable-position tie-break for two rows with the same owner.
func TestDedupeStaticModelsByOwnerRule(t *testing.T) {
	t.Parallel()

	got := dedupeStaticModelsByOwner([]OpenAIModels{
		{Id: "m1", OwnedBy: "zhipu", Root: "first"},
		{Id: "m1", OwnedBy: "zai", Root: "second"},
		{Id: "m2", OwnedBy: "aws", Root: "third"},
		{Id: "m1", OwnedBy: "anthropic", Root: "fourth"},
		{Id: "m2", OwnedBy: "aws", Root: "fifth"},
	})

	require.Len(t, got, 2)

	byID := map[string]OpenAIModels{}
	for _, m := range got {
		byID[m.Id] = m
	}
	// Smallest owner in byte order wins, regardless of position.
	require.Equal(t, "anthropic", byID["m1"].OwnedBy)
	require.Equal(t, "fourth", byID["m1"].Root)
	// Equal owners: first occurrence wins.
	require.Equal(t, "third", byID["m2"].Root)
}
