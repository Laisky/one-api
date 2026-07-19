package state

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestMemoryStoreConformance runs the shared store contract against the in-memory
// backend.
func TestMemoryStoreConformance(t *testing.T) {
	t.Parallel()
	runStoreConformance(t, func(t *testing.T) ResponseStateStore {
		return NewMemoryStore(DefaultLimits())
	})
}

// TestMemoryStoreResponseTTL verifies the default 30-day response retention and
// that an expired node reads as not-found (S02).
func TestMemoryStoreResponseTTL(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	owner := OwnerScope{UserID: 1, TokenID: 1}

	base := time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)
	store := NewMemoryStore(DefaultLimits())
	store.SetClock(func() time.Time { return base })

	rec := sampleResponse(t, owner)
	rec.ExpiresAt = base.Add(DefaultResponseTTL).Unix()
	_, err := store.CreateResponse(ctx, rec, "")
	require.NoError(t, err)

	// Just before expiry: still readable.
	store.SetClock(func() time.Time { return base.Add(DefaultResponseTTL - time.Hour) })
	_, err = store.GetResponse(ctx, owner, rec.GatewayResponseID)
	require.NoError(t, err)

	// After expiry: not found.
	store.SetClock(func() time.Time { return base.Add(DefaultResponseTTL + time.Hour) })
	_, err = store.GetResponse(ctx, owner, rec.GatewayResponseID)
	require.ErrorIs(t, err, ErrNotFound)
}

// TestMemoryStoreConversationHasNoTTL verifies a conversation without ExpiresAt
// never expires under the response TTL (S03).
func TestMemoryStoreConversationHasNoTTL(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	owner := OwnerScope{UserID: 1, TokenID: 1}

	base := time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)
	store := NewMemoryStore(DefaultLimits())
	store.SetClock(func() time.Time { return base })

	conv := sampleConversation(t, owner)
	conv.ExpiresAt = 0
	_, err := store.CreateConversation(ctx, conv, "")
	require.NoError(t, err)

	store.SetClock(func() time.Time { return base.Add(365 * 24 * time.Hour) })
	_, err = store.GetConversation(ctx, owner, conv.GatewayConversationID)
	require.NoError(t, err, "conversation must not inherit the response TTL")
}

// TestMemoryStoreLeaseTimeout verifies an expired lease frees the conversation
// for a later writer (CON05).
func TestMemoryStoreLeaseTimeout(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	owner := OwnerScope{UserID: 1, TokenID: 1}

	base := time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)
	store := NewMemoryStore(DefaultLimits())
	store.SetClock(func() time.Time { return base })

	conv := sampleConversation(t, owner)
	_, err := store.CreateConversation(ctx, conv, "")
	require.NoError(t, err)

	_, err = store.AcquireConversationLease(ctx, owner, conv.GatewayConversationID, 30*time.Second)
	require.NoError(t, err)

	// Before expiry: still held.
	store.SetClock(func() time.Time { return base.Add(10 * time.Second) })
	_, err = store.AcquireConversationLease(ctx, owner, conv.GatewayConversationID, 30*time.Second)
	require.ErrorIs(t, err, ErrLeaseHeld)

	// After expiry: a new writer may acquire.
	store.SetClock(func() time.Time { return base.Add(31 * time.Second) })
	token, err := store.AcquireConversationLease(ctx, owner, conv.GatewayConversationID, 30*time.Second)
	require.NoError(t, err)
	require.NotEmpty(t, token)
}

// TestMemoryStoreImmutableParentUnderConcurrency verifies concurrent forks off a
// shared parent never mutate the immutable parent record and never race (F07).
func TestMemoryStoreImmutableParentUnderConcurrency(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	owner := OwnerScope{UserID: 1, TokenID: 1}
	store := NewMemoryStore(DefaultLimits())

	parent := sampleResponse(t, owner)
	_, err := store.CreateResponse(ctx, parent, "")
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			child := sampleResponse(t, owner)
			child.ParentResponseID = parent.GatewayResponseID
			_, cerr := store.CreateResponse(ctx, child, "")
			require.NoError(t, cerr)
			// Reading the parent concurrently and mutating the returned copy must not
			// affect stored state.
			got, gerr := store.GetResponse(ctx, owner, parent.GatewayResponseID)
			require.NoError(t, gerr)
			got.InputItems = nil
		}()
	}
	wg.Wait()

	final, err := store.GetResponse(ctx, owner, parent.GatewayResponseID)
	require.NoError(t, err)
	require.Len(t, final.InputItems, len(parent.InputItems), "parent must remain immutable")
}
