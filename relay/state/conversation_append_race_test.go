package state

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConcurrentConversationAppendsLoseNoItems reproduces a silent data-loss bug
// in the Conversations API.
//
// AppendConversationItems is a read-modify-write over an encrypted blob, so it
// cannot be a Redis CAS. CON04 says the per-conversation lease serializes it, but
// no caller outside this package ever took that lease, and the public API has no
// if-match semantics, so the controller passes AnyVersion. Two concurrent
// POST /v1/conversations/{id}/items therefore both read version N and both wrote
// N+1: the loser's items disappeared from the record even though the API returned
// 200 and echoed them back, and the rs:item:* index entries it had already written
// became unreachable by every cleanup path — permanent, uncapped Redis garbage.
func TestConcurrentConversationAppendsLoseNoItems(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	owner := OwnerScope{UserID: 11, TokenID: 5}

	store, _ := newRedisStoreForTest(t)
	conv := sampleConversation(t, owner)
	conv.Items = nil
	created, err := store.CreateConversation(ctx, conv, "")
	require.NoError(t, err)

	const writers = 8
	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			env := mustEnvelope(t, fmt.Sprintf(`{"type":"message","role":"user","content":"item-%d"}`, i))
			_, errs[i] = store.AppendConversationItems(ctx, owner, created.GatewayConversationID, AnyVersion,
				[]ItemEnvelope{env}, "")
		}(i)
	}
	wg.Wait()

	succeeded := 0
	for _, err := range errs {
		if err == nil {
			succeeded++
			continue
		}
		require.ErrorIs(t, err, ErrLeaseHeld, "the only acceptable failure is an explicit conflict")
	}
	require.Equal(t, writers, succeeded, "every concurrent append should have been serialized, not dropped")

	final, err := store.GetConversation(ctx, owner, created.GatewayConversationID)
	require.NoError(t, err)
	require.Len(t, final.Items, succeeded,
		"the record must contain every item an append reported as written")
}
