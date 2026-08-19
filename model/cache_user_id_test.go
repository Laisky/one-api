package model

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
)

// setupUserObjCacheTest wires an in-memory database and a miniredis-backed Redis
// client, then returns the ready context. It reproduces the production runtime
// where CacheGetUserById reads/writes the "user_obj:<id>" Redis key.
func setupUserObjCacheTest(t *testing.T) context.Context {
	t.Helper()
	setupTestDatabase(t)

	redisServer, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(redisServer.Close)

	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { require.NoError(t, redisClient.Close()) })

	originalRedisEnabled := common.IsRedisEnabled()
	originalRedisClient := common.RDB
	common.SetRedisEnabled(true)
	common.RDB = redisClient
	t.Cleanup(func() {
		common.SetRedisEnabled(originalRedisEnabled)
		common.RDB = originalRedisClient
	})

	// Make the cache TTL deterministic and positive so the key survives between calls.
	originalTTL := UserId2UserCacheSeconds
	UserId2UserCacheSeconds = 300
	t.Cleanup(func() { UserId2UserCacheSeconds = originalTTL })

	return context.Background()
}

// TestCacheGetUserByIdPreservesId_Issue353 reproduces GitHub issue #353:
// once a User object is written to the Redis "user_obj:*" cache, subsequent
// cache hits must return the correct integer Id. Before the fix the cache
// serialization went through the API-facing User.MarshalJSON, which drops the
// internal integer Id, so a cache hit returned Id == 0. That zero then flowed
// into ctxkey.Id and eventually produced a 500 ("user id is empty").
func TestCacheGetUserByIdPreservesId_Issue353(t *testing.T) {
	ctx := setupUserObjCacheTest(t)

	user := &User{
		Username:   fmt.Sprintf("test-issue353-%d", time.Now().UnixNano()),
		Password:   "testpassword12345",
		Status:     UserStatusEnabled,
		Role:       RoleCommonUser,
		Group:      "vip",
		Quota:      98765,
		TotpSecret: "JBSWY3DPEHPK3PXP", // exercise the secret-scrub path
	}
	require.NoError(t, DB.Create(user).Error)
	require.NotZero(t, user.Id, "sanity: DB must assign a non-zero primary key")
	t.Cleanup(func() { DB.Exec("DELETE FROM users WHERE id = ?", user.Id) })

	// First call: cache miss -> served from DB -> populates the Redis cache.
	firstHit, err := CacheGetUserById(ctx, user.Id)
	require.NoError(t, err)
	require.Equal(t, user.Id, firstHit.Id, "DB read must return the correct Id")

	// Inspect the raw cached payload: this is the precise bug locus. The cache
	// entry must contain the integer id, otherwise the round-trip loses it.
	cacheKey := fmt.Sprintf("user_obj:%d", user.Id)
	rawPayload, err := common.RedisGet(ctx, cacheKey)
	require.NoError(t, err, "cache entry must exist after the first (miss) call")
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(rawPayload), &decoded))
	require.Contains(t, decoded, "id", "cached user_obj payload must carry the integer id field")
	require.EqualValues(t, user.Id, decoded["id"], "cached id must equal the real user id")

	// Security posture: secrets must not be persisted to Redis (unchanged from the
	// pre-fix behavior, which also excluded them). The fix restores only Id/InviterId.
	require.Empty(t, decoded["password"], "cached payload must not contain a password")
	require.Empty(t, decoded["access_token"], "cached payload must not contain an access_token")
	require.Empty(t, decoded["totp_secret"], "cached payload must not contain a TOTP secret")

	// Second call: cache hit -> served from Redis. GetUserById can never return
	// Id == 0 (it seeds User{Id: id} and rejects id == 0), so an Id of 0 here
	// can ONLY come from the cache deserialization path -> proves the bug.
	secondHit, err := CacheGetUserById(ctx, user.Id)
	require.NoError(t, err)
	require.Equal(t, user.Id, secondHit.Id,
		"cache hit must preserve the integer Id (issue #353: it was dropped -> 0)")

	// Sanity: other fields must survive the round-trip too (rules out a false
	// positive where everything is zeroed, which would hide the specific Id bug).
	require.Equal(t, user.Username, secondHit.Username)
	require.Equal(t, user.Status, secondHit.Status)
	require.Equal(t, "vip", secondHit.Group)
	require.EqualValues(t, 98765, secondHit.Quota)
}

// TestCacheGetUserByIdServesFromCacheWithCorrectId proves the second read is
// genuinely served from Redis (not a silent DB re-read) AND still has the right
// Id: after populating the cache we delete the DB row, so a correct non-zero Id
// on the next call can only originate from a faithful cache entry.
func TestCacheGetUserByIdServesFromCacheWithCorrectId(t *testing.T) {
	ctx := setupUserObjCacheTest(t)

	user := &User{
		Username: fmt.Sprintf("test-issue353-cacheonly-%d", time.Now().UnixNano()),
		Password: "testpassword12345",
		Status:   UserStatusEnabled,
		Role:     RoleCommonUser,
		Quota:    111,
	}
	require.NoError(t, DB.Create(user).Error)
	wantId := user.Id
	require.NotZero(t, wantId)
	t.Cleanup(func() { DB.Exec("DELETE FROM users WHERE id = ?", wantId) })

	// Populate the cache.
	_, err := CacheGetUserById(ctx, wantId)
	require.NoError(t, err)

	// Remove the DB row so any subsequent success must come from the cache.
	require.NoError(t, DB.Exec("DELETE FROM users WHERE id = ?", wantId).Error)

	cached, err := CacheGetUserById(ctx, wantId)
	require.NoError(t, err, "must be served from cache after DB row is gone")
	require.Equal(t, wantId, cached.Id, "cache-served user must keep its integer Id")
}

// TestUserDefaultMarshalIsHonestAndSecretFree replaces the retired
// TestUserMarshalJSON_StillHidesInternalIntId. The boundary-DTO refactor
// INVERTS the old design intent: model.User no longer has a whitelist
// MarshalJSON, so a default json.Marshal(User) is now HONEST (it emits the
// internal integer "id" — that is exactly what the Redis object cache relies on,
// issue #353). The external contract (uuid-only, no int id) is instead enforced
// by dto.UserResponse (see TestResponseGoldens/user).
//
// What must remain true forever: the three secret fields are json:"-", so NO
// serialization path — cache, log, or a future queue — can emit them, even by
// accident (G3). This is the security net that took over from the marshaler
// (T4/T16).
func TestUserDefaultMarshalIsHonestAndSecretFree(t *testing.T) {
	inviterUUID := "018f0000-0000-7000-8000-000000000009"
	u := User{
		Id:               42,
		InviterId:        7,
		UUID:             "018f0000-0000-7000-8000-000000000001",
		Username:         "alice",
		InviterUUID:      &inviterUUID,
		Password:         "$2a$10$bcrypthashsecret",
		AccessToken:      "access-token-secret-000000000000",
		TotpSecret:       "JBSWY3DPEHPK3PXP",
		VerificationCode: "verif-code",
	}
	b, err := json.Marshal(u)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))

	// Honest default serialization: the internal integer id is present (this is
	// what fixed #353 — the cache round-trip preserves it).
	require.Contains(t, got, "id", "default marshal must be honest and carry the internal integer id")
	require.EqualValues(t, 42, got["id"])

	// Secrets are unrepresentable in any serialization (json:"-").
	require.NotContains(t, got, "password", "password must never be serialized")
	require.NotContains(t, got, "access_token", "access_token must never be serialized")
	require.NotContains(t, got, "totp_secret", "totp_secret must never be serialized")
	require.NotContains(t, got, "verification_code", "verification_code must never be serialized")
}
