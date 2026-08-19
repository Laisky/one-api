package state

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// checkpointTestOwner is the default tenant used by the checkpoint tests.
func checkpointTestOwner() OwnerScope { return OwnerScope{UserID: 1, TokenID: 2} }

// checkpointTestBinding is the default provider-binding identity used by the tests.
func checkpointTestBinding() *ProviderBinding {
	return &ProviderBinding{ChannelID: 7, APIType: 1, ActualModel: "gpt-4o-2024-11-20"}
}

// checkpointTestMessages returns a stable four-turn transcript.
func checkpointTestMessages() []CheckpointMessage {
	return []CheckpointMessage{
		{Role: "user", Content: "what is the weather in paris?"},
		{Role: "assistant", Content: "", ToolCallID: "call_1", Name: "get_weather"},
		{Role: "tool", Content: `{"temp_c":21}`, ToolCallID: "call_1", Name: "get_weather"},
		{Role: "assistant", Content: "It is 21C in Paris.", Signature: "sig-abc"},
	}
}

const checkpointTestFamily = "chat"
const checkpointTestModel = "gpt-4o"

// TestCheckpointKeyAtDeterministic verifies the key is reproducible: identical
// inputs always hash to the identical key, with no time or randomness folded in.
func TestCheckpointKeyAtDeterministic(t *testing.T) {
	t.Parallel()
	owner := checkpointTestOwner()
	msgs := checkpointTestMessages()
	k1 := CheckpointKeyAt(owner, checkpointTestFamily, checkpointTestModel, checkpointTestBinding(), msgs, len(msgs))
	k2 := CheckpointKeyAt(owner, checkpointTestFamily, checkpointTestModel, checkpointTestBinding(), msgs, len(msgs))
	require.Equal(t, k1, k2)
	require.Len(t, k1, 64) // hex-encoded sha256
}

// TestCheckpointCP01LongestPrefixSelected records checkpoints at two different
// prefix lengths and asserts the longest live match wins (CP01).
func TestCheckpointCP01LongestPrefixSelected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore(DefaultLimits())
	owner := checkpointTestOwner()
	binding := checkpointTestBinding()
	msgs := checkpointTestMessages()

	// Two checkpoints: one at prefix length 2, a longer one at prefix length 3.
	require.NoError(t, RecordCheckpoint(ctx, store, owner, checkpointTestFamily, checkpointTestModel, binding, msgs[:2], "resp-prefix-2", time.Hour))
	require.NoError(t, RecordCheckpoint(ctx, store, owner, checkpointTestFamily, checkpointTestModel, binding, msgs[:3], "resp-prefix-3", time.Hour))

	matched, prefixLen, ok := LongestCheckpointMatch(ctx, store, owner, checkpointTestFamily, checkpointTestModel, binding, msgs)
	require.True(t, ok)
	require.Equal(t, 3, prefixLen)
	require.NotNil(t, matched)
	require.Equal(t, "resp-prefix-3", matched.ResponseID)
	require.False(t, matched.Ambiguous)
}

// TestCheckpointCP02SignatureInHash proves the thinking signature is part of the
// hash: same visible text with a different signature does not match (CP02).
func TestCheckpointCP02SignatureInHash(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore(DefaultLimits())
	owner := checkpointTestOwner()
	binding := checkpointTestBinding()

	base := []CheckpointMessage{
		{Role: "user", Content: "prove it"},
		{Role: "assistant", Content: "here", Signature: "sig-A"},
	}
	altered := []CheckpointMessage{
		{Role: "user", Content: "prove it"},
		{Role: "assistant", Content: "here", Signature: "sig-B"},
	}

	// Keys must differ solely because the signature differs.
	require.NotEqual(t,
		CheckpointKeyAt(owner, checkpointTestFamily, checkpointTestModel, binding, base, len(base)),
		CheckpointKeyAt(owner, checkpointTestFamily, checkpointTestModel, binding, altered, len(altered)),
	)

	require.NoError(t, RecordCheckpoint(ctx, store, owner, checkpointTestFamily, checkpointTestModel, binding, base, "resp-sig", time.Hour))

	_, _, ok := LongestCheckpointMatch(ctx, store, owner, checkpointTestFamily, checkpointTestModel, binding, altered)
	require.False(t, ok)
}

// TestCheckpointCP03OneByteContentChange verifies a one-byte content edit yields
// no match (CP03).
func TestCheckpointCP03OneByteContentChange(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore(DefaultLimits())
	owner := checkpointTestOwner()
	binding := checkpointTestBinding()
	msgs := checkpointTestMessages()

	require.NoError(t, RecordCheckpoint(ctx, store, owner, checkpointTestFamily, checkpointTestModel, binding, msgs, "resp-exact", time.Hour))

	altered := checkpointTestMessages()
	altered[0].Content = "what is the weather in parit?" // one byte changed: s -> t

	require.NotEqual(t,
		CheckpointKeyAt(owner, checkpointTestFamily, checkpointTestModel, binding, msgs, len(msgs)),
		CheckpointKeyAt(owner, checkpointTestFamily, checkpointTestModel, binding, altered, len(altered)),
	)

	_, _, ok := LongestCheckpointMatch(ctx, store, owner, checkpointTestFamily, checkpointTestModel, binding, altered)
	require.False(t, ok)
}

// TestCheckpointCP04ToolCallIDChange verifies a changed tool-call id yields no
// match even when every other field is identical (CP04).
func TestCheckpointCP04ToolCallIDChange(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore(DefaultLimits())
	owner := checkpointTestOwner()
	binding := checkpointTestBinding()
	msgs := checkpointTestMessages()

	require.NoError(t, RecordCheckpoint(ctx, store, owner, checkpointTestFamily, checkpointTestModel, binding, msgs, "resp-tool", time.Hour))

	altered := checkpointTestMessages()
	altered[1].ToolCallID = "call_2"
	altered[2].ToolCallID = "call_2"

	require.NotEqual(t,
		CheckpointKeyAt(owner, checkpointTestFamily, checkpointTestModel, binding, msgs, len(msgs)),
		CheckpointKeyAt(owner, checkpointTestFamily, checkpointTestModel, binding, altered, len(altered)),
	)

	_, _, ok := LongestCheckpointMatch(ctx, store, owner, checkpointTestFamily, checkpointTestModel, binding, altered)
	require.False(t, ok)
}

// TestCheckpointCP05DifferentOwner verifies the same transcript under a different
// owner does not match: owner scope is in the hash and enforced by the store
// (CP05).
func TestCheckpointCP05DifferentOwner(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore(DefaultLimits())
	owner := checkpointTestOwner()
	other := OwnerScope{UserID: 99, TokenID: 2}
	binding := checkpointTestBinding()
	msgs := checkpointTestMessages()

	require.NoError(t, RecordCheckpoint(ctx, store, owner, checkpointTestFamily, checkpointTestModel, binding, msgs, "resp-owner", time.Hour))

	// The key itself differs by owner...
	require.NotEqual(t,
		CheckpointKeyAt(owner, checkpointTestFamily, checkpointTestModel, binding, msgs, len(msgs)),
		CheckpointKeyAt(other, checkpointTestFamily, checkpointTestModel, binding, msgs, len(msgs)),
	)

	// ...and the other owner sees no match.
	_, _, ok := LongestCheckpointMatch(ctx, store, other, checkpointTestFamily, checkpointTestModel, binding, msgs)
	require.False(t, ok)

	// A per-token owner variant (same user, different token) also does not match.
	otherToken := OwnerScope{UserID: 1, TokenID: 999}
	_, _, ok = LongestCheckpointMatch(ctx, store, otherToken, checkpointTestFamily, checkpointTestModel, binding, msgs)
	require.False(t, ok)
}

// TestCheckpointCP06DifferentModelOrBinding verifies that a different public model
// or a different provider binding produces no incompatible match (CP06).
func TestCheckpointCP06DifferentModelOrBinding(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore(DefaultLimits())
	owner := checkpointTestOwner()
	binding := checkpointTestBinding()
	msgs := checkpointTestMessages()

	require.NoError(t, RecordCheckpoint(ctx, store, owner, checkpointTestFamily, checkpointTestModel, binding, msgs, "resp-route", time.Hour))

	// Different public model -> no match.
	_, _, ok := LongestCheckpointMatch(ctx, store, owner, checkpointTestFamily, "gpt-4o-mini", binding, msgs)
	require.False(t, ok)

	// Different channel id -> no match.
	altChannel := &ProviderBinding{ChannelID: 8, APIType: 1, ActualModel: "gpt-4o-2024-11-20"}
	_, _, ok = LongestCheckpointMatch(ctx, store, owner, checkpointTestFamily, checkpointTestModel, altChannel, msgs)
	require.False(t, ok)

	// Different api type -> no match.
	altAPIType := &ProviderBinding{ChannelID: 7, APIType: 99, ActualModel: "gpt-4o-2024-11-20"}
	_, _, ok = LongestCheckpointMatch(ctx, store, owner, checkpointTestFamily, checkpointTestModel, altAPIType, msgs)
	require.False(t, ok)

	// Different actual model -> no match.
	altModel := &ProviderBinding{ChannelID: 7, APIType: 1, ActualModel: "gpt-4o-2024-08-06"}
	_, _, ok = LongestCheckpointMatch(ctx, store, owner, checkpointTestFamily, checkpointTestModel, altModel, msgs)
	require.False(t, ok)

	// Sanity: the exact original still matches.
	_, _, ok = LongestCheckpointMatch(ctx, store, owner, checkpointTestFamily, checkpointTestModel, binding, msgs)
	require.True(t, ok)
}

// TestCheckpointCP07Ambiguous verifies that two distinct responseIDs colliding on
// one key mark the stored checkpoint ambiguous and that an ambiguous checkpoint is
// treated as a non-match (CP07).
func TestCheckpointCP07Ambiguous(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore(DefaultLimits())
	owner := checkpointTestOwner()
	binding := checkpointTestBinding()
	msgs := checkpointTestMessages()

	// First continuation: a clean, matchable checkpoint.
	require.NoError(t, RecordCheckpoint(ctx, store, owner, checkpointTestFamily, checkpointTestModel, binding, msgs, "resp-one", time.Hour))
	_, _, ok := LongestCheckpointMatch(ctx, store, owner, checkpointTestFamily, checkpointTestModel, binding, msgs)
	require.True(t, ok)

	// Second continuation under the identical visible transcript -> ambiguity.
	require.NoError(t, RecordCheckpoint(ctx, store, owner, checkpointTestFamily, checkpointTestModel, binding, msgs, "resp-two", time.Hour))

	// The stored record is now flagged ambiguous.
	key := CheckpointKeyAt(owner, checkpointTestFamily, checkpointTestModel, binding, msgs, len(msgs))
	rec, err := store.GetCheckpoint(ctx, owner, key)
	require.NoError(t, err)
	require.True(t, rec.Ambiguous)

	// And matching now fails open.
	_, _, ok = LongestCheckpointMatch(ctx, store, owner, checkpointTestFamily, checkpointTestModel, binding, msgs)
	require.False(t, ok)

	// A further record with yet another id keeps it disabled, not resurrected.
	require.NoError(t, RecordCheckpoint(ctx, store, owner, checkpointTestFamily, checkpointTestModel, binding, msgs, "resp-three", time.Hour))
	rec, err = store.GetCheckpoint(ctx, owner, key)
	require.NoError(t, err)
	require.True(t, rec.Ambiguous)
}

// TestCheckpointCP07IdempotentRefresh verifies that re-recording the same
// responseID is an idempotent refresh and never marks the checkpoint ambiguous.
func TestCheckpointCP07IdempotentRefresh(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore(DefaultLimits())
	owner := checkpointTestOwner()
	binding := checkpointTestBinding()
	msgs := checkpointTestMessages()

	require.NoError(t, RecordCheckpoint(ctx, store, owner, checkpointTestFamily, checkpointTestModel, binding, msgs, "resp-same", time.Hour))
	require.NoError(t, RecordCheckpoint(ctx, store, owner, checkpointTestFamily, checkpointTestModel, binding, msgs, "resp-same", time.Hour))

	matched, _, ok := LongestCheckpointMatch(ctx, store, owner, checkpointTestFamily, checkpointTestModel, binding, msgs)
	require.True(t, ok)
	require.False(t, matched.Ambiguous)
	require.Equal(t, "resp-same", matched.ResponseID)
}

// TestCheckpointCP08ExpiredTarget verifies an expired checkpoint is a silent
// non-match, not an error, for stateless clients (CP08).
func TestCheckpointCP08ExpiredTarget(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore(DefaultLimits())
	owner := checkpointTestOwner()
	binding := checkpointTestBinding()
	msgs := checkpointTestMessages()

	key := CheckpointKeyAt(owner, checkpointTestFamily, checkpointTestModel, binding, msgs, len(msgs))

	// Put a checkpoint that already expired an hour ago.
	require.NoError(t, store.PutCheckpoint(ctx, &CheckpointRecord{
		SchemaVersion: CurrentSchemaVersion,
		Key:           key,
		Owner:         owner,
		ClientFamily:  checkpointTestFamily,
		PublicModel:   checkpointTestModel,
		Binding:       binding,
		ResponseID:    "resp-expired",
		ExpiresAt:     time.Now().Add(-time.Hour).Unix(),
	}))

	// The store treats it as gone.
	_, err := store.GetCheckpoint(ctx, owner, key)
	require.ErrorIs(t, err, ErrNotFound)

	// The matcher fails open with no error surface.
	matched, prefixLen, ok := LongestCheckpointMatch(ctx, store, owner, checkpointTestFamily, checkpointTestModel, binding, msgs)
	require.False(t, ok)
	require.Nil(t, matched)
	require.Zero(t, prefixLen)
}

// TestCheckpointNoMatchOnEmptyStore verifies the clean cold-cache path: nothing
// stored means no match and no error (CP03/CP08 fail-open baseline).
func TestCheckpointNoMatchOnEmptyStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore(DefaultLimits())
	owner := checkpointTestOwner()
	binding := checkpointTestBinding()
	msgs := checkpointTestMessages()

	matched, prefixLen, ok := LongestCheckpointMatch(ctx, store, owner, checkpointTestFamily, checkpointTestModel, binding, msgs)
	require.False(t, ok)
	require.Nil(t, matched)
	require.Zero(t, prefixLen)
}

// TestCheckpointMatchGuards verifies the defensive guards: a nil store, an invalid
// owner, or an empty transcript never match and never panic.
func TestCheckpointMatchGuards(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := NewMemoryStore(DefaultLimits())
	owner := checkpointTestOwner()
	binding := checkpointTestBinding()
	msgs := checkpointTestMessages()

	_, _, ok := LongestCheckpointMatch(ctx, nil, owner, checkpointTestFamily, checkpointTestModel, binding, msgs)
	require.False(t, ok)

	_, _, ok = LongestCheckpointMatch(ctx, store, OwnerScope{}, checkpointTestFamily, checkpointTestModel, binding, msgs)
	require.False(t, ok)

	_, _, ok = LongestCheckpointMatch(ctx, store, owner, checkpointTestFamily, checkpointTestModel, binding, nil)
	require.False(t, ok)

	// RecordCheckpoint rejects a nil store and an invalid owner.
	require.Error(t, RecordCheckpoint(ctx, nil, owner, checkpointTestFamily, checkpointTestModel, binding, msgs, "resp", time.Hour))
	require.Error(t, RecordCheckpoint(ctx, store, OwnerScope{}, checkpointTestFamily, checkpointTestModel, binding, msgs, "resp", time.Hour))
}
