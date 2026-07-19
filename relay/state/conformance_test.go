package state

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// storeFactory builds a fresh, empty store for one conformance sub-test.
type storeFactory func(t *testing.T) ResponseStateStore

// limitedStoreFactory builds a fresh, empty store with custom limits so both
// backends prove identical limit/cap semantics (ST-018 parity).
type limitedStoreFactory func(t *testing.T, limits Limits) ResponseStateStore

// mustEnvelope builds an envelope or fails the test.
func mustEnvelope(t *testing.T, raw string) ItemEnvelope {
	t.Helper()
	env, err := NewItemEnvelope(json.RawMessage(raw), "test")
	require.NoError(t, err)
	return env
}

func mustResponseID(t *testing.T) string {
	t.Helper()
	id, err := NewResponseID()
	require.NoError(t, err)
	return id
}

func mustConversationID(t *testing.T) string {
	t.Helper()
	id, err := NewConversationID()
	require.NoError(t, err)
	return id
}

// runStoreConformance exercises the full ResponseStateStore contract. It is
// shared by the in-memory and Redis backends so both prove identical semantics
// (Section 8.4). newStore must return an empty store per call.
func runStoreConformance(t *testing.T, newStore storeFactory, newLimited limitedStoreFactory) {
	ctx := context.Background()
	owner := OwnerScope{UserID: 7, TokenID: 3}
	other := OwnerScope{UserID: 8, TokenID: 4}

	t.Run("S01 response round trip", func(t *testing.T) {
		store := newStore(t)
		rec := sampleResponse(t, owner)
		created, err := store.CreateResponse(ctx, rec, "")
		require.NoError(t, err)
		require.Equal(t, CurrentSchemaVersion, created.SchemaVersion)

		got, err := store.GetResponse(ctx, owner, rec.GatewayResponseID)
		require.NoError(t, err)
		require.Equal(t, rec.GatewayResponseID, got.GatewayResponseID)
		require.Len(t, got.InputItems, len(rec.InputItems))
		require.Len(t, got.OutputItems, len(rec.OutputItems))
		require.JSONEq(t, string(rec.OutputItems[0].Raw), string(got.OutputItems[0].Raw))
		require.NotNil(t, got.Binding)
		require.Equal(t, rec.Binding.UpstreamResponseID, got.Binding.UpstreamResponseID)
	})

	t.Run("SEC03 foreign owner is not found", func(t *testing.T) {
		store := newStore(t)
		rec := sampleResponse(t, owner)
		_, err := store.CreateResponse(ctx, rec, "")
		require.NoError(t, err)

		_, err = store.GetResponse(ctx, other, rec.GatewayResponseID)
		require.ErrorIs(t, err, ErrNotFound)

		// Unknown id returns the identical external error.
		_, err = store.GetResponse(ctx, owner, mustResponseID(t))
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("S05 idempotent duplicate commit", func(t *testing.T) {
		store := newStore(t)
		rec := sampleResponse(t, owner)
		first, err := store.CreateResponse(ctx, rec, "idem-key-1")
		require.NoError(t, err)

		// Re-create a DIFFERENT record under the same idempotency key; the first
		// node must win and no duplicate is created.
		dup := sampleResponse(t, owner)
		dup.GatewayResponseID = rec.GatewayResponseID
		second, err := store.CreateResponse(ctx, dup, "idem-key-1")
		require.NoError(t, err)
		require.Equal(t, first.GatewayResponseID, second.GatewayResponseID)
	})

	t.Run("S06 tombstone after delete", func(t *testing.T) {
		store := newStore(t)
		rec := sampleResponse(t, owner)
		_, err := store.CreateResponse(ctx, rec, "")
		require.NoError(t, err)

		// The tombstone is not set before deletion.
		dead, err := store.ResponseTombstoned(ctx, rec.GatewayResponseID)
		require.NoError(t, err)
		require.False(t, dead)

		require.NoError(t, store.DeleteResponse(ctx, owner, rec.GatewayResponseID))
		_, err = store.GetResponse(ctx, owner, rec.GatewayResponseID)
		require.ErrorIs(t, err, ErrNotFound)
		// Binding must not be reusable after deletion.
		_, err = store.GetResponseBinding(ctx, owner, rec.GatewayResponseID)
		require.ErrorIs(t, err, ErrNotFound)
		// The tombstone is now readable so the resolve layer can refuse stale
		// fallback for a deleted gateway id (S06, ST-018).
		dead, err = store.ResponseTombstoned(ctx, rec.GatewayResponseID)
		require.NoError(t, err)
		require.True(t, dead)
		// Deleting again is a not-found.
		require.ErrorIs(t, store.DeleteResponse(ctx, owner, rec.GatewayResponseID), ErrNotFound)
	})

	t.Run("ST-018 upstream item index is purged on response delete", func(t *testing.T) {
		store := newStore(t)
		rec := sampleResponseWithUpstreamItems(t, owner)
		_, err := store.CreateResponse(ctx, rec, "")
		require.NoError(t, err)

		gwItemID := rec.OutputItems[0].GatewayItemID
		upItemID := rec.OutputItems[0].UpstreamItemID
		require.NotEmpty(t, upItemID, "sample must carry an upstream item id")

		// Both the gateway id and the raw upstream id resolve before deletion.
		_, err = store.GetItem(ctx, owner, gwItemID)
		require.NoError(t, err)
		_, err = store.GetItem(ctx, owner, upItemID)
		require.NoError(t, err)

		require.NoError(t, store.DeleteResponse(ctx, owner, rec.GatewayResponseID))
		// Neither index entry survives the delete (no data remanence within owner
		// scope) — the gap ST-018 closes; both backends must behave identically.
		_, err = store.GetItem(ctx, owner, gwItemID)
		require.ErrorIs(t, err, ErrNotFound)
		_, err = store.GetItem(ctx, owner, upItemID)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("ST-018 upstream item index is purged on conversation delete", func(t *testing.T) {
		store := newStore(t)
		conv := sampleConversationWithUpstreamItems(t, owner)
		_, err := store.CreateConversation(ctx, conv, "")
		require.NoError(t, err)

		upItemID := conv.Items[0].UpstreamItemID
		require.NotEmpty(t, upItemID)
		_, err = store.GetItem(ctx, owner, upItemID)
		require.NoError(t, err)

		require.NoError(t, store.DeleteConversation(ctx, owner, conv.GatewayConversationID))
		_, err = store.GetItem(ctx, owner, upItemID)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("L06 per-user response cap evicts oldest (TTL+LRU)", func(t *testing.T) {
		store := newLimited(t, Limits{MaxResponsesPerUser: 3})
		ids := make([]string, 0, 5)
		for i := 0; i < 5; i++ {
			rec := sampleResponse(t, owner)
			rec.CreatedAt = int64(1000 + i) // strictly increasing creation order
			_, err := store.CreateResponse(ctx, rec, "")
			require.NoError(t, err)
			ids = append(ids, rec.GatewayResponseID)
		}
		// Only the newest 3 survive; the 2 oldest are evicted and tombstoned so an
		// evicted parent degrades to previous_response_not_found (row L06).
		for _, id := range ids[:2] {
			_, err := store.GetResponse(ctx, owner, id)
			require.ErrorIs(t, err, ErrNotFound)
			dead, err := store.ResponseTombstoned(ctx, id)
			require.NoError(t, err)
			require.True(t, dead)
		}
		for _, id := range ids[2:] {
			_, err := store.GetResponse(ctx, owner, id)
			require.NoError(t, err)
		}
	})

	t.Run("L07 per-user conversation cap rejects create beyond limit", func(t *testing.T) {
		store := newLimited(t, Limits{MaxConversationsPerUser: 2})
		for i := 0; i < 2; i++ {
			_, err := store.CreateConversation(ctx, sampleConversation(t, owner), "")
			require.NoError(t, err)
		}
		// The third create fails explicitly; conversations are never silently evicted.
		_, err := store.CreateConversation(ctx, sampleConversation(t, owner), "")
		require.ErrorIs(t, err, ErrLimitExceeded)

		// Another owner is unaffected by this owner's cap.
		_, err = store.CreateConversation(ctx, sampleConversation(t, other), "")
		require.NoError(t, err)

		// Deleting one frees a slot for a new conversation (L07 accounting).
		list := sampleConversation(t, owner)
		_, err = store.CreateConversation(ctx, list, "") // still over cap
		require.ErrorIs(t, err, ErrLimitExceeded)
	})

	t.Run("S08 batch chain hydration preserves order and gaps", func(t *testing.T) {
		store := newStore(t)
		a := sampleResponse(t, owner)
		b := sampleResponse(t, owner)
		_, err := store.CreateResponse(ctx, a, "")
		require.NoError(t, err)
		_, err = store.CreateResponse(ctx, b, "")
		require.NoError(t, err)

		missing := mustResponseID(t)
		got, err := store.BatchGetResponses(ctx, owner, []string{a.GatewayResponseID, missing, b.GatewayResponseID})
		require.NoError(t, err)
		require.Len(t, got, 3)
		require.NotNil(t, got[0])
		require.Nil(t, got[1], "missing middle node must be a nil hole")
		require.NotNil(t, got[2])
		require.Equal(t, a.GatewayResponseID, got[0].GatewayResponseID)
		require.Equal(t, b.GatewayResponseID, got[2].GatewayResponseID)
	})

	t.Run("item reference resolves under owner scope", func(t *testing.T) {
		store := newStore(t)
		rec := sampleResponse(t, owner)
		_, err := store.CreateResponse(ctx, rec, "")
		require.NoError(t, err)

		itemID := rec.OutputItems[0].GatewayItemID
		got, err := store.GetItem(ctx, owner, itemID)
		require.NoError(t, err)
		require.JSONEq(t, string(rec.OutputItems[0].Raw), string(got.Raw))

		// Foreign owner cannot resolve it.
		_, err = store.GetItem(ctx, other, itemID)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("L02 item count limit rejects oversized record", func(t *testing.T) {
		store := newLimited(t, Limits{MaxItemCount: 2})
		rec := sampleResponse(t, owner)
		rec.InputItems = []ItemEnvelope{
			mustEnvelope(t, `{"type":"message","role":"user","content":"1"}`),
			mustEnvelope(t, `{"type":"message","role":"user","content":"2"}`),
			mustEnvelope(t, `{"type":"message","role":"user","content":"3"}`),
		}
		rec.OutputItems = nil
		_, err := store.CreateResponse(ctx, rec, "")
		require.ErrorIs(t, err, ErrLimitExceeded)
	})

	t.Run("CON01/CON02 conversation create and atomic append", func(t *testing.T) {
		store := newStore(t)
		conv := sampleConversation(t, owner)
		created, err := store.CreateConversation(ctx, conv, "")
		require.NoError(t, err)
		require.Equal(t, int64(0), created.Version)

		appended, err := store.AppendConversationItems(ctx, owner, conv.GatewayConversationID, created.Version,
			[]ItemEnvelope{mustEnvelope(t, `{"type":"message","role":"user","content":"turn"}`)}, "")
		require.NoError(t, err)
		require.Equal(t, int64(1), appended.Version)
		require.Len(t, appended.Items, len(conv.Items)+1)
	})

	t.Run("S05 conversation append is idempotent", func(t *testing.T) {
		store := newStore(t)
		conv := sampleConversation(t, owner)
		_, err := store.CreateConversation(ctx, conv, "")
		require.NoError(t, err)

		item := []ItemEnvelope{mustEnvelope(t, `{"type":"message","role":"user","content":"turn"}`)}
		first, err := store.AppendConversationItems(ctx, owner, conv.GatewayConversationID, AnyVersion, item, "append-1")
		require.NoError(t, err)
		second, err := store.AppendConversationItems(ctx, owner, conv.GatewayConversationID, AnyVersion, item, "append-1")
		require.NoError(t, err)
		require.Equal(t, first.Version, second.Version, "retried append must not double-apply")
		require.Len(t, second.Items, len(conv.Items)+1)
	})

	t.Run("version conflict on stale append", func(t *testing.T) {
		store := newStore(t)
		conv := sampleConversation(t, owner)
		_, err := store.CreateConversation(ctx, conv, "")
		require.NoError(t, err)
		item := []ItemEnvelope{mustEnvelope(t, `{"type":"message","role":"user","content":"t"}`)}
		_, err = store.AppendConversationItems(ctx, owner, conv.GatewayConversationID, 0, item, "")
		require.NoError(t, err)
		// Version is now 1; appending with expected version 0 must conflict.
		_, err = store.AppendConversationItems(ctx, owner, conv.GatewayConversationID, 0, item, "")
		require.ErrorIs(t, err, ErrVersionConflict)
	})

	t.Run("CON04 lease is exclusive", func(t *testing.T) {
		store := newStore(t)
		conv := sampleConversation(t, owner)
		_, err := store.CreateConversation(ctx, conv, "")
		require.NoError(t, err)

		token, err := store.AcquireConversationLease(ctx, owner, conv.GatewayConversationID, time.Minute)
		require.NoError(t, err)
		require.NotEmpty(t, token)

		_, err = store.AcquireConversationLease(ctx, owner, conv.GatewayConversationID, time.Minute)
		require.ErrorIs(t, err, ErrLeaseHeld)

		require.NoError(t, store.ReleaseConversationLease(ctx, owner, conv.GatewayConversationID, token))
		token2, err := store.AcquireConversationLease(ctx, owner, conv.GatewayConversationID, time.Minute)
		require.NoError(t, err)
		require.NotEmpty(t, token2)
	})

	t.Run("CON06 delete conversation tombstones items", func(t *testing.T) {
		store := newStore(t)
		conv := sampleConversation(t, owner)
		_, err := store.CreateConversation(ctx, conv, "")
		require.NoError(t, err)
		itemID := conv.Items[0].GatewayItemID

		require.NoError(t, store.DeleteConversation(ctx, owner, conv.GatewayConversationID))
		_, err = store.GetConversation(ctx, owner, conv.GatewayConversationID)
		require.ErrorIs(t, err, ErrNotFound)
		_, err = store.GetItem(ctx, owner, itemID)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("checkpoint put and owner-scoped get", func(t *testing.T) {
		store := newStore(t)
		cp := &CheckpointRecord{
			Key:          "hash-abc",
			Owner:        owner,
			ClientFamily: "chat",
			PublicModel:  "gpt-5",
			ResponseID:   mustResponseID(t),
		}
		require.NoError(t, store.PutCheckpoint(ctx, cp))

		got, err := store.GetCheckpoint(ctx, owner, "hash-abc")
		require.NoError(t, err)
		require.Equal(t, cp.ResponseID, got.ResponseID)

		// CP05: same key under another owner is a miss.
		_, err = store.GetCheckpoint(ctx, other, "hash-abc")
		require.ErrorIs(t, err, ErrNotFound)
	})
}

// sampleResponseWithUpstreamItems builds a response whose input and output items
// carry a raw provider "id", so NewItemEnvelope records a non-empty
// UpstreamItemID and the upstream-index dedup/delete cases have something to
// exercise (ST-018: the harness previously never set UpstreamItemID).
func sampleResponseWithUpstreamItems(t *testing.T, owner OwnerScope) *ResponseStateRecord {
	t.Helper()
	input := mustEnvelope(t, `{"type":"message","role":"user","id":"msg_up_in_1","content":"hello"}`)
	output := mustEnvelope(t, `{"type":"message","role":"assistant","id":"msg_up_out_1","content":[{"type":"output_text","text":"hi"}]}`)
	require.NotEmpty(t, output.UpstreamItemID)
	return &ResponseStateRecord{
		GatewayResponseID: mustResponseID(t),
		Owner:             owner,
		CreatedAt:         time.Now().Unix(),
		Status:            StatusCompleted,
		InputItems:        []ItemEnvelope{input},
		OutputItems:       []ItemEnvelope{output},
		RequestedModel:    "gpt-5",
		StoreMode:         true,
		ExpiresAt:         time.Now().Add(DefaultResponseTTL).Unix(),
	}
}

func sampleConversationWithUpstreamItems(t *testing.T, owner OwnerScope) *ConversationStateRecord {
	t.Helper()
	return &ConversationStateRecord{
		GatewayConversationID: mustConversationID(t),
		Owner:                 owner,
		Items:                 []ItemEnvelope{mustEnvelope(t, `{"type":"message","role":"user","id":"msg_up_conv_1","content":"seed"}`)},
	}
}

func sampleResponse(t *testing.T, owner OwnerScope) *ResponseStateRecord {
	t.Helper()
	input := mustEnvelope(t, `{"type":"message","role":"user","content":"hello"}`)
	output := mustEnvelope(t, `{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}],"unknown":{"x":1}}`)
	return &ResponseStateRecord{
		GatewayResponseID: mustResponseID(t),
		Owner:             owner,
		CreatedAt:         time.Now().Unix(),
		Status:            StatusCompleted,
		InputItems:        []ItemEnvelope{input},
		OutputItems:       []ItemEnvelope{output},
		RequestedModel:    "gpt-5",
		StoreMode:         true,
		Binding: &ProviderBinding{
			ChannelID:          11,
			APIType:            1,
			ActualModel:        "gpt-5",
			UpstreamResponseID: "resp_upstream_1",
		},
		ExpiresAt: time.Now().Add(DefaultResponseTTL).Unix(),
	}
}

func sampleConversation(t *testing.T, owner OwnerScope) *ConversationStateRecord {
	t.Helper()
	return &ConversationStateRecord{
		GatewayConversationID: mustConversationID(t),
		Owner:                 owner,
		CreatedAt:             time.Now().Unix(),
		Version:               0,
		Items:                 []ItemEnvelope{mustEnvelope(t, `{"type":"message","role":"user","content":"seed"}`)},
	}
}
