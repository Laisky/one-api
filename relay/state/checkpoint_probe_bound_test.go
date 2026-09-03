package state

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// countingCheckpointStore counts checkpoint lookups so a test can measure how
// much store work one request is able to drive.
type countingCheckpointStore struct {
	ResponseStateStore
	lookups int
}

// GetCheckpoint records the call and reports a miss.
//
// Parameters:
//   - ctx: request context.
//   - owner: the owner scope.
//   - key: the checkpoint key.
//
// Return values:
//   - *CheckpointRecord: always nil.
//   - error: always ErrNotFound.
func (s *countingCheckpointStore) GetCheckpoint(ctx context.Context, owner OwnerScope, key string) (*CheckpointRecord, error) {
	s.lookups++
	return nil, ErrNotFound
}

// TestLongestCheckpointMatchBoundsItsProbes pins that a request cannot drive an
// unbounded number of checkpoint lookups.
//
// The probe loop ran from len(msgs) down to 1, performing one store round-trip
// (Redis GET + AES-GCM decrypt) per message and re-hashing the whole n-prefix each
// iteration — O(N) round-trips and O(N^2) hashing. The message list comes straight
// from the request body via chatMessagesToCheckpoint, with no cap anywhere on the
// path, so a single Chat request carrying 50k near-empty messages held a Redis
// connection for 50k sequential lookups.
//
// Bounding is safe by this function's own contract: it fails open, and a miss
// simply means the caller performs an ordinary explicit replay.
func TestLongestCheckpointMatchBoundsItsProbes(t *testing.T) {
	t.Parallel()

	owner := OwnerScope{UserID: 3, TokenID: 1}
	store := &countingCheckpointStore{ResponseStateStore: NewMemoryStore(DefaultLimits())}

	msgs := make([]CheckpointMessage, 0, 5000)
	for i := 0; i < 5000; i++ {
		msgs = append(msgs, CheckpointMessage{Role: "user", Content: fmt.Sprintf("m%d", i)})
	}

	_, _, ok := LongestCheckpointMatch(context.Background(), store, owner, "chat", "gpt-5", nil, msgs)
	require.False(t, ok, "no checkpoint was seeded, so this must miss and fail open")
	require.LessOrEqualf(t, store.lookups, maxCheckpointProbes,
		"a %d-message request drove %d checkpoint lookups; the probe count must be bounded",
		len(msgs), store.lookups)
}

// TestLongestCheckpointMatchStillFindsARecentPrefix keeps the bound from breaking
// the optimization it protects: the match a real client needs is at or near the
// full message list, because a turn appends one or two messages to what the
// gateway already stored.
func TestLongestCheckpointMatchStillFindsARecentPrefix(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	owner := OwnerScope{UserID: 4, TokenID: 1}
	store := NewMemoryStore(DefaultLimits())

	msgs := make([]CheckpointMessage, 0, 40)
	for i := 0; i < 40; i++ {
		msgs = append(msgs, CheckpointMessage{Role: "user", Content: fmt.Sprintf("m%d", i)})
	}

	// Seed a checkpoint one turn behind the request, the normal continuation case.
	const seededPrefix = 39
	key := CheckpointKeyAt(owner, "chat", "gpt-5", nil, msgs, seededPrefix)
	require.NoError(t, store.PutCheckpoint(ctx, &CheckpointRecord{
		Key:        key,
		Owner:      owner,
		ResponseID: "resp_seeded",
	}))

	matched, prefixLen, ok := LongestCheckpointMatch(ctx, store, owner, "chat", "gpt-5", nil, msgs)
	require.True(t, ok, "a checkpoint one turn behind must still be found")
	require.Equal(t, seededPrefix, prefixLen)
	require.Equal(t, "resp_seeded", matched.ResponseID)
}
