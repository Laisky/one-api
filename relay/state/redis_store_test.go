package state

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func testKeyRing(t *testing.T) *KeyRing {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	spec := "1:" + base64.StdEncoding.EncodeToString(key)
	ring, err := ParseKeyRing(spec)
	require.NoError(t, err)
	return ring
}

func newRedisStoreForTest(t *testing.T) (*RedisStore, *miniredis.Miniredis) {
	t.Helper()
	return newRedisStoreForTestWithLimits(t, DefaultLimits())
}

func newRedisStoreForTestWithLimits(t *testing.T, limits Limits) (*RedisStore, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, err := NewRedisStore(client, testKeyRing(t), limits, DefaultResponseTTL)
	require.NoError(t, err)
	return store, mr
}

// TestRedisStoreConformance runs the shared store contract against the Redis
// backend via miniredis, proving it matches the in-memory backend (Section 8.4).
// The limits-aware factory ensures the L02/L06/L07 limit rows also run against
// Redis, not only the in-memory backend (ST-018 parity).
func TestRedisStoreConformance(t *testing.T) {
	runStoreConformance(t, func(t *testing.T) ResponseStateStore {
		store, _ := newRedisStoreForTest(t)
		return store
	}, func(t *testing.T, limits Limits) ResponseStateStore {
		store, _ := newRedisStoreForTestWithLimits(t, limits)
		return store
	})
}

// TestRedisStorePayloadIsEncrypted verifies stored values are ciphertext and the
// Redis key names contain no user content (SEC01).
func TestRedisStorePayloadIsEncrypted(t *testing.T) {
	ctx := context.Background()
	store, mr := newRedisStoreForTest(t)
	owner := OwnerScope{UserID: 1, TokenID: 1}

	rec := sampleResponse(t, owner)
	// Embed a recognizable secret so we can prove it never appears in plaintext.
	rec.OutputItems[0] = mustEnvelope(t, `{"type":"message","role":"assistant","content":[{"type":"output_text","text":"SECRET-CANARY-VALUE"}]}`)
	_, err := store.CreateResponse(ctx, rec, "")
	require.NoError(t, err)

	raw, err := mr.Get(store.respKey(rec.GatewayResponseID))
	require.NoError(t, err)
	require.NotContains(t, raw, "SECRET-CANARY-VALUE")
	require.NotContains(t, raw, "output_text")

	// Key names carry only the random gateway ID, no content.
	for _, key := range mr.Keys() {
		require.NotContains(t, key, "SECRET-CANARY-VALUE")
	}
}

// TestRedisStoreLeaseExpiry verifies an abandoned lease frees the conversation
// after its TTL (CON05), using miniredis fast-forward.
func TestRedisStoreLeaseExpiry(t *testing.T) {
	ctx := context.Background()
	store, mr := newRedisStoreForTest(t)
	owner := OwnerScope{UserID: 1, TokenID: 1}

	conv := sampleConversation(t, owner)
	_, err := store.CreateConversation(ctx, conv, "")
	require.NoError(t, err)

	_, err = store.AcquireConversationLease(ctx, owner, conv.GatewayConversationID, 30*time.Second)
	require.NoError(t, err)
	_, err = store.AcquireConversationLease(ctx, owner, conv.GatewayConversationID, 30*time.Second)
	require.ErrorIs(t, err, ErrLeaseHeld)

	mr.FastForward(31 * time.Second)
	token, err := store.AcquireConversationLease(ctx, owner, conv.GatewayConversationID, 30*time.Second)
	require.NoError(t, err)
	require.NotEmpty(t, token)
}

// TestRedisStoreConversationIdleTTL verifies the Redis backend expires an idle
// conversation via its key TTL and that a read slides it forward (row L08),
// using miniredis fast-forward.
func TestRedisStoreConversationIdleTTL(t *testing.T) {
	ctx := context.Background()
	store, mr := newRedisStoreForTest(t)
	store.SetConversationIdleTTL(30 * time.Second)
	owner := OwnerScope{UserID: 1, TokenID: 1}

	conv := sampleConversation(t, owner)
	conv.ExpiresAt = 0
	_, err := store.CreateConversation(ctx, conv, "")
	require.NoError(t, err)

	// A read within the window slides the idle TTL forward.
	mr.FastForward(25 * time.Second)
	_, err = store.GetConversation(ctx, owner, conv.GatewayConversationID)
	require.NoError(t, err)

	// 20s after the slide (< 30s): still alive.
	mr.FastForward(20 * time.Second)
	_, err = store.GetConversation(ctx, owner, conv.GatewayConversationID)
	require.NoError(t, err)

	// Idle past the window: gone.
	mr.FastForward(31 * time.Second)
	_, err = store.GetConversation(ctx, owner, conv.GatewayConversationID)
	require.ErrorIs(t, err, ErrNotFound)
}

// TestKeyRingRotation verifies old ciphertext stays readable after a new primary
// key is added, and new writes use the new key (SEC02).
func TestKeyRingRotation(t *testing.T) {
	t.Parallel()

	key1 := make([]byte, 32)
	for i := range key1 {
		key1[i] = byte(i + 1)
	}
	key2 := make([]byte, 32)
	for i := range key2 {
		key2[i] = byte(i + 100)
	}
	enc := base64.StdEncoding.EncodeToString

	oldRing, err := ParseKeyRing("1:" + enc(key1))
	require.NoError(t, err)
	token, err := oldRing.Encrypt([]byte("payload-v1"))
	require.NoError(t, err)

	// Rotated ring: new key "2" is primary; old key "1" still present for reads.
	newRing, err := ParseKeyRing("2:" + enc(key2) + ",1:" + enc(key1))
	require.NoError(t, err)
	require.Equal(t, "2", newRing.PrimaryVersion())

	plain, err := newRing.Decrypt(token)
	require.NoError(t, err)
	require.Equal(t, "payload-v1", string(plain))

	newToken, err := newRing.Encrypt([]byte("payload-v2"))
	require.NoError(t, err)
	require.Equal(t, "2", newToken[:1], "new writes must use the primary key version")
}

// TestParseKeyRingRejectsBadSpecs verifies configuration validation.
func TestParseKeyRingRejectsBadSpecs(t *testing.T) {
	t.Parallel()

	_, err := ParseKeyRing("")
	require.Error(t, err)

	_, err = ParseKeyRing("1:not-base64!!")
	require.Error(t, err)

	// Wrong key length.
	_, err = ParseKeyRing("1:" + base64.StdEncoding.EncodeToString([]byte("short")))
	require.Error(t, err)

	// Missing version.
	_, err = ParseKeyRing(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	require.Error(t, err)
}

// TestRedisStoreSchemaGuard verifies a record from a newer schema version fails
// with a typed migration error instead of silently dropping fields (S07).
func TestRedisStoreSchemaGuard(t *testing.T) {
	ctx := context.Background()
	store, _ := newRedisStoreForTest(t)
	owner := OwnerScope{UserID: 1, TokenID: 1}

	rec := sampleResponse(t, owner)
	rec.SchemaVersion = CurrentSchemaVersion + 1
	_, err := store.CreateResponse(ctx, rec, "")
	require.NoError(t, err)

	_, err = store.GetResponse(ctx, owner, rec.GatewayResponseID)
	require.ErrorIs(t, err, ErrUnsupportedSchema)
}
