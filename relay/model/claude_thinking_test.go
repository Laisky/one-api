package model

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// rawClaudeEnvelope builds an envelope around an arbitrary JSON payload.
func rawClaudeEnvelope(payload string) string {
	return ClaudeThinkingEnvelopePrefix + base64.RawURLEncoding.EncodeToString([]byte(payload))
}

// TestClaudeThinkingEnvelopeRoundTrip verifies signed (including empty omitted
// display text) and redacted blocks survive the envelope byte-for-byte.
func TestClaudeThinkingEnvelopeRoundTrip(t *testing.T) {
	t.Parallel()
	for _, block := range []ClaudeThinkingBlock{
		{Type: ClaudeThinkingBlockTypeThinking, Signature: "sig1"},
		{Type: ClaudeThinkingBlockTypeThinking, Thinking: "  exact\n text ", Signature: "sig2", ToolCallsBefore: 2, AfterText: true},
		{Type: ClaudeThinkingBlockTypeRedacted, Data: "opaque-redacted", ToolCallsBefore: 1},
	} {
		envelope, err := EncodeClaudeThinkingEnvelope(block)
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(envelope, "oneapi.claude.v1."))
		require.True(t, IsClaudeThinkingEnvelope(envelope))
		decoded, err := DecodeClaudeThinkingEnvelope(envelope)
		require.NoError(t, err)
		require.Equal(t, block, decoded)
	}
}

// TestClaudeThinkingEnvelopeRejectsUnreplayable verifies the encoder never
// produces an envelope without verifiable provider state.
func TestClaudeThinkingEnvelopeRejectsUnreplayable(t *testing.T) {
	t.Parallel()
	for _, block := range []ClaudeThinkingBlock{
		{Type: ClaudeThinkingBlockTypeThinking, Thinking: "unsigned"},
		{Type: ClaudeThinkingBlockTypeRedacted},
		{Type: "text", Thinking: "x", Signature: "s"},
	} {
		_, err := EncodeClaudeThinkingEnvelope(block)
		require.Error(t, err)
		require.False(t, block.Replayable())
	}
}

// TestDecodeClaudeThinkingEnvelopeRejectsForeignAndMalformed verifies every
// invalid envelope is rejected so callers fall back safely.
func TestDecodeClaudeThinkingEnvelopeRejectsForeignAndMalformed(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"openai_encrypted":    "gAAAAABn-opaque-openai-reasoning",
		"other_version":       "oneapi.claude.v2." + base64.RawURLEncoding.EncodeToString([]byte(`{"s":"x"}`)),
		"bad_base64":          ClaudeThinkingEnvelopePrefix + "!!!not-base64!!!",
		"padded_base64":       ClaudeThinkingEnvelopePrefix + base64.URLEncoding.EncodeToString([]byte(`{"s":"xy"}`)),
		"not_object":          rawClaudeEnvelope(`["s","x"]`),
		"null_payload":        rawClaudeEnvelope(`null`),
		"unknown_field":       rawClaudeEnvelope(`{"s":"x","extra":1}`),
		"case_folded_key":     rawClaudeEnvelope(`{"S":"x"}`),
		"trailing_value":      rawClaudeEnvelope(`{"s":"x"}{"s":"y"}`),
		"no_state":            rawClaudeEnvelope(`{"t":"text only"}`),
		"mixed_state":         rawClaudeEnvelope(`{"s":"x","r":"y"}`),
		"redacted_with_text":  rawClaudeEnvelope(`{"t":"x","r":"y"}`),
		"negative_anchor":     rawClaudeEnvelope(`{"s":"x","k":-1}`),
		"huge_anchor":         rawClaudeEnvelope(`{"s":"x","k":100000000}`),
		"wrong_type":          rawClaudeEnvelope(`{"s":7}`),
		"fractional_anchor":   rawClaudeEnvelope(`{"s":"x","k":1.5}`),
		"prefix_only":         ClaudeThinkingEnvelopePrefix,
		"oversized_envelope":  ClaudeThinkingEnvelopePrefix + strings.Repeat("A", MaxClaudeThinkingEnvelopeBytes),
		"empty":               "",
		"prefix_case_changed": strings.ToUpper(ClaudeThinkingEnvelopePrefix) + "e30",
	}
	for name, envelope := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeClaudeThinkingEnvelope(envelope)
			require.Error(t, err)
		})
	}
}

// TestHasReplayableClaudeThinkingBlocks verifies the replayability summary.
func TestHasReplayableClaudeThinkingBlocks(t *testing.T) {
	t.Parallel()
	require.False(t, HasReplayableClaudeThinkingBlocks(nil))
	require.False(t, HasReplayableClaudeThinkingBlocks([]ClaudeThinkingBlock{{Type: ClaudeThinkingBlockTypeThinking}}))
	require.True(t, HasReplayableClaudeThinkingBlocks([]ClaudeThinkingBlock{{Type: ClaudeThinkingBlockTypeRedacted, Data: "d"}}))
}
