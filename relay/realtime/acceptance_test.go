package realtime

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReviewCacheLowerBoundMatchesEnumeration independently enumerates every
// admissible cache allocation, including preallocated buckets and cache premiums.
// It verifies the minimum calculation without using Cost as its own oracle.
func TestReviewCacheLowerBoundMatchesEnumeration(t *testing.T) {
	for _, rates := range []Rates{
		{Text: 4, Audio: 32, Image: 5, CachedText: .4, CachedAudio: .4, CachedImage: .5},
		{Text: 10, Audio: 1, Image: 2, CachedText: 12, CachedAudio: 4, CachedImage: 8},
		{Text: 0, Audio: 32, Image: 5, CachedText: 0, CachedAudio: .4, CachedImage: .5},
	} {
		for unknown := int64(0); unknown <= 7; unknown++ {
			record := Record{Tokens: Tokens{Input: 9, Text: 3, Audio: 3, Image: 3,
				CachedText: 1, CachedImage: 1, CachedUnallocated: unknown}}
			before := record
			want := math.Inf(1)
			for text := int64(1); text <= 3; text++ {
				for audio := int64(0); audio <= 3; audio++ {
					for image := int64(1); image <= 3; image++ {
						if text+audio+image-2 != unknown {
							continue
						}
						candidate := float64(3-text)*rates.Text + float64(text)*rates.CachedText +
							float64(3-audio)*rates.Audio + float64(audio)*rates.CachedAudio +
							float64(3-image)*rates.Image + float64(image)*rates.CachedImage
						want = math.Min(want, candidate)
					}
				}
			}
			got, err := Cost(record, rates)
			require.NoError(t, err)
			require.InDelta(t, want, got, 1e-9)
			require.Equal(t, before, record, "do not overwrite authoritative evidence with the chosen allocation")
		}
	}
}

// TestReviewErrorClassification verifies errors.Is through wrapped sentinels,
// and distinguishes a retained uncertain receipt from a missing usage receipt.
func TestReviewErrorClassification(t *testing.T) {
	l := NewLedger()
	err := l.Observe([]byte(`{"type":"response.done","response":{"id":"r","usage":{"input_tokens":2,"output_tokens":0,"input_token_details":{"text_tokens":1,"audio_tokens":1,"cached_tokens":1}}}}`))
	require.ErrorIs(t, err, ErrAmbiguousCache)
	require.Len(t, l.Records, 1)
	require.Equal(t, int64(1), l.Records[0].Tokens.CachedUnallocated)
	require.False(t, l.HasUsageGap())
	err = l.Observe([]byte(`{"type":"response.done","response":{"id":"missing"}}`))
	require.ErrorIs(t, err, ErrIncompleteUsage)
	require.True(t, l.HasUsageGap())
	require.ErrorIs(t, (Tokens{Input: -1}).Validate(), ErrInvalidUsage)
	_, err = Cost(Record{}, Rates{Text: -1})
	require.ErrorIs(t, err, ErrInvalidPrice)
	_, err = RoundQuota(1<<53, 1, 0)
	require.ErrorIs(t, err, ErrQuotaOverflow)
}

// TestReviewPendingAndIdentifierBounds checks collections that can grow without
// completed receipts and bounds variable-sized keys/models, not merely counts.
func TestReviewPendingAndIdentifierBounds(t *testing.T) {
	l := NewLedger()
	for i := 0; i < MaxPendingResponses; i++ {
		err := l.Observe([]byte(fmt.Sprintf(`{"type":"response.created","response":{"id":"r%d"}}`, i)))
		if i+1 == MaxPendingResponses {
			require.ErrorIs(t, err, ErrLedgerLimit)
		} else {
			require.NoError(t, err)
		}
	}
	require.Len(t, l.pendingResponses, MaxPendingResponses)
	require.True(t, l.HasUsageGap())
	require.ErrorIs(t, l.Observe([]byte(`{"type":"response.created","response":{"id":"extra"}}`)), ErrLedgerLimit)
	require.Len(t, l.pendingResponses, MaxPendingResponses)
	for _, frame := range []string{
		fmt.Sprintf(`{"type":"input_audio_buffer.committed","item_id":%q}`, strings.Repeat("x", MaxIdentifierBytes+1)),
		fmt.Sprintf(`{"type":"session.updated","session":{"input_audio_transcription":{"model":%q}}}`, strings.Repeat("m", MaxModelBytes+1)),
	} {
		l := NewLedger()
		require.ErrorIs(t, l.Observe([]byte(frame)), ErrLedgerLimit)
		require.Empty(t, l.Records)
		require.Empty(t, l.itemModels)
		require.True(t, l.Audit().CapacityReached)
	}
}
