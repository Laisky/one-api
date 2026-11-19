package openai

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
)

// catalogFromFamilies assembles complete family tables before aliases and lifecycle.
// It takes no parameters and returns a fresh map without changing global maps.
func catalogFromFamilies() map[string]adaptor.ModelConfig {
	return adaptor.JoinModelCatalogs(
		gpt35ModelRatios, gpt4ModelRatios, gpt4oModelRatios, realtimeModelRatios,
		gpt45ModelRatios, gpt41ModelRatios, gpt5ModelRatios, gpt56ModelRatios, gpt6ModelRatios,
		oSeriesModelRatios, specializedModelRatios, embeddingModelRatios,
		audioModelRatios, imageModelRatios, videoModelRatios,
	)
}

// TestOpenAIOctoberPreservesCatalogPricing verifies every pre-existing entry is
// retained with identical tariffs and limits. Parameter t owns assertions; no
// values return. The metadata changes are independently pinned by behavior tests.
func TestOpenAIOctoberPreservesCatalogPricing(t *testing.T) {
	before := catalogFromFamilies()
	after := completeOpenAIModelCatalog(before)
	require.Len(t, after, len(before)+4)
	for name, old := range before {
		t.Run(name, func(t *testing.T) {
			updated, ok := after[name]
			require.True(t, ok, "historical model removed")
			updated.Description = old.Description
			require.Equal(t, old, updated, "only lifecycle descriptions may change")
		})
	}
	require.Equal(t, after, completeOpenAIModelCatalog(after), "refresh must be idempotent")
	for _, provider := range []int{channeltype.Groq, channeltype.Mistral, channeltype.XAI} {
		a := &Adaptor{ChannelType: provider}
		for snapshot := range openAIPublishedSnapshots {
			require.NotContains(t, a.GetModelList(), snapshot)
			require.NotContains(t, a.GetDefaultModelPricing(), snapshot)
		}
	}
	for _, invented := range []string{"gpt-6.1-sol-pro", "gpt-6.1-sol-ultrafast", "gpt-6.1-sol-2026-09-29", "gpt-5.3-codex-2026-02-05"} {
		require.NotContains(t, after, invented)
	}
}

// TestOpenAIOctoberSnapshotOwnership verifies snapshot mutations cannot leak into
// their base or another snapshot. Parameter t owns assertions; no values return.
func TestOpenAIOctoberSnapshotOwnership(t *testing.T) {
	before := catalogFromFamilies()
	originalPro := before["gpt-5.5-pro"].Clone()
	after := completeOpenAIModelCatalog(before)
	pro := after["gpt-5.5-pro-2026-04-23"]
	pro.SupportedReasoningEfforts[0] = "corrupted"
	pro.SupportedFeatures[0] = "corrupted"
	pro.InputModalities[0] = "corrupted"
	pro.Tiers[0].Ratio = -42
	require.Equal(t, originalPro, before["gpt-5.5-pro"])
	require.Equal(t, "medium", after["gpt-5.5-pro"].SupportedReasoningEfforts[0])
	require.Greater(t, after["gpt-5.5-pro"].Tiers[0].Ratio, 0.0)

	oldAudio := before["gpt-4o-mini-transcribe"].Clone()
	newAudio := after["gpt-4o-mini-transcribe-2025-12-15"]
	require.NotNil(t, newAudio.Audio)
	newAudio.Audio.PromptRatio = -42
	newAudio.InputModalities[0] = "corrupted"
	require.Equal(t, oldAudio, before["gpt-4o-mini-transcribe"])
	require.Equal(t, oldAudio.Audio, after["gpt-4o-mini-transcribe"].Audio)
	require.Equal(t, oldAudio.Audio, after["gpt-4o-mini-transcribe-2025-03-20"].Audio)
}

// TestOpenAIOctoberLifecycleIntegrity verifies each note targets a real model
// exactly once without creating phantom IDs. Parameter t owns assertions; no
// values return, and absence of a note does not imply upstream availability.
func TestOpenAIOctoberLifecycleIntegrity(t *testing.T) {
	seen := make(map[string]bool)
	for _, notice := range openAILifecycleNotices {
		for _, name := range strings.Fields(notice.models) {
			require.False(t, seen[name], "duplicate lifecycle note for %s", name)
			seen[name] = true
			cfg, ok := ModelRatios[name]
			require.True(t, ok, "unknown lifecycle model %s", name)
			require.Contains(t, cfg.Description, notice.shutdown)
			require.Contains(t, cfg.Description, notice.replacement)
			require.Equal(t, 1, strings.Count(cfg.Description, "[OpenAI lifecycle:"))
		}
	}
	require.Empty(t, completeOpenAIModelCatalog(nil))
	unknown := map[string]adaptor.ModelConfig{"tenant-alias": {Ratio: 42, Description: "customer-owned"}}
	require.Equal(t, unknown, completeOpenAIModelCatalog(unknown))
}
