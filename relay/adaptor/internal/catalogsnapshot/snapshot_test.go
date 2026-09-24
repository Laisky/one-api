package catalogsnapshot

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestSnapshotCurrencyAndIsolation verifies explicit zero prices, both native
// currencies, cache sentinels, tiers, and deep-copy preservation with t.
func TestSnapshotCurrencyAndIsolation(t *testing.T) {
	t.Parallel()
	for currency, factor := range map[string]float64{"USD": ratio.MilliTokensUsd, "CNY": ratio.MilliTokensRmb} {
		t.Run(currency, func(t *testing.T) {
			base := map[string]adaptor.ModelConfig{"existing": {Ratio: 19, CompletionRatio: 3, InputModalities: []string{"text"}, Audio: &adaptor.AudioPricingConfig{UsdPerSecond: 0.25}}, "legacy": {Ratio: 7}}
			raw := []byte(fmt.Sprintf(`{"version":1,"currency":%q,"sources":[{"url":"https://example.com/models","sha256":"fixture"}],"models":{"existing":{"ratio":2,"completion_ratio":4,"cached_input_ratio":-1,"tiers":[{"ratio":4,"completion_ratio":5,"cached_input_ratio":1,"input_token_threshold":128000}]},"free":{"ratio":0,"completion_ratio":1},"free-output":{"ratio":1,"completion_ratio":0}}}`, currency))
			out, err := Decode(base, raw)
			require.NoError(t, err)
			require.InDelta(t, 2*factor, out["existing"].Ratio, 1e-12)
			require.Equal(t, 4.0, out["existing"].CompletionRatio)
			require.Equal(t, -1.0, out["existing"].CachedInputRatio)
			require.InDelta(t, 4*factor, out["existing"].Tiers[0].Ratio, 1e-12)
			require.Zero(t, out["free"].Ratio)
			require.Zero(t, out["free-output"].CompletionRatio)
			require.Equal(t, 7.0, out["legacy"].Ratio)
			out["existing"].InputModalities[0] = "mutated"
			out["existing"].Audio.UsdPerSecond = 99
			require.Equal(t, []string{"text"}, base["existing"].InputModalities)
			require.Equal(t, 0.25, base["existing"].Audio.UsdPerSecond)
			require.Equal(t, 19.0, base["existing"].Ratio)
		})
	}
}

// TestSnapshotRejectsInvalidData checks schema errors and unquoted new models
// fail without changing the original catalog using t.
func TestSnapshotRejectsInvalidData(t *testing.T) {
	t.Parallel()
	for _, patch := range []string{`{"x":{"ratio":-1}}`, `{"x":{"context_length":10}}`, `{"x":{"ratio":1,"max_tokens":10}}`, `{"x":{"ratio":1,"typo":2}}`, `{" x":{"ratio":1}}`, `{"x":null}`} {
		base := map[string]adaptor.ModelConfig{"old": {Ratio: 5}}
		_, err := Decode(base, []byte(`{"version":1,"currency":"USD","sources":[{"url":"fixture"}],"models":`+patch+`}`))
		require.Error(t, err)
		require.Len(t, base, 1)
		require.Equal(t, 5.0, base["old"].Ratio)
	}
	require.Panics(t, func() { Apply(nil, []byte(`{}`)) })
}

// TestPartialEmbeddingPreservesNormalizedImagePrice catches double conversion
// when a provider publishes a new text rate but leaves image pricing unspecified.
func TestPartialEmbeddingPreservesNormalizedImagePrice(t *testing.T) {
	t.Parallel()
	base := map[string]adaptor.ModelConfig{"embed": {Ratio: 2, Embedding: &adaptor.EmbeddingPricingConfig{TextTokenRatio: 2, ImageTokenRatio: 9}}}
	raw := []byte(`{"version":1,"currency":"CNY","sources":[{"url":"fixture"}],"models":{"embed":{"embedding":{"text_token_ratio":7}}}}`)
	out, err := Decode(base, raw)
	require.NoError(t, err)
	require.InDelta(t, 7*ratio.MilliTokensRmb, out["embed"].Embedding.TextTokenRatio, 1e-12)
	require.Equal(t, 9.0, out["embed"].Embedding.ImageTokenRatio)
	require.Equal(t, 2.0, base["embed"].Embedding.TextTokenRatio)
}

// TestAffirmativeCapabilitiesPreserveUnspecifiedMetadata verifies an additive
// source table cannot silently remove reasoning while adding tools, using t.
func TestAffirmativeCapabilitiesPreserveUnspecifiedMetadata(t *testing.T) {
	t.Parallel()
	base := map[string]adaptor.ModelConfig{"model": {Ratio: 2, CompletionRatio: 1, SupportedFeatures: []string{"reasoning", "tools"}}}
	raw := []byte(`{"version":1,"currency":"USD","sources":[{"url":"fixture"}],"models":{"model":{"add_supported_features":["tools","structured_outputs"]}}}`)
	got, err := Decode(base, raw)
	require.NoError(t, err)
	require.Equal(t, []string{"reasoning", "tools", "structured_outputs"}, got["model"].SupportedFeatures)
	require.Equal(t, []string{"reasoning", "tools"}, base["model"].SupportedFeatures)
	_, err = Decode(nil, []byte(`{"version":1,"currency":"USD","sources":[{"url":"fixture"}],"models":{"new":{"ratio":1}}}`))
	require.Error(t, err, "a missing output quote must not imply free output")
}
