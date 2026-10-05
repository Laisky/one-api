package common

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/logger"
)

const (
	// TotpCodeCacheDuration is the duration for which a TOTP code is cached to prevent replay
	TotpCodeCacheDuration = 30 * time.Second
	// TotpCodeCacheKeyPrefix is the prefix for TOTP code cache keys
	TotpCodeCacheKeyPrefix = "totp_used"
)

// totpCodeKey returns the replay-cache key for one user's TOTP code.
func totpCodeKey(userId int, totpCode string) string {
	return fmt.Sprintf("%s:%d:%s", TotpCodeCacheKeyPrefix, userId, totpCode)
}

// IsTotpCodeUsed checks if a TOTP code has been used recently (within 30 seconds)
func IsTotpCodeUsed(ctx context.Context, userId int, totpCode string) bool {
	if totpCode == "" {
		return false
	}

	key := totpCodeKey(userId, totpCode)

	if IsRedisEnabled() {
		return isRedisKeyExists(ctx, key)
	} else {
		// For memory cache, we'll use a simple in-memory map
		return isMemoryKeyExists(key)
	}
}

// MarkTotpCodeAsUsed marks a TOTP code as used to prevent replay attacks
func MarkTotpCodeAsUsed(ctx context.Context, userId int, totpCode string) error {
	if totpCode == "" {
		return nil
	}

	key := totpCodeKey(userId, totpCode)

	if IsRedisEnabled() {
		return RedisSet(ctx, key, "1", TotpCodeCacheDuration)
	} else {
		// For memory cache, we'll use a simple in-memory map
		return setMemoryKey(key, TotpCodeCacheDuration)
	}
}

// ConsumeTotpCode atomically records totpCode as used by userId. It returns
// true only for the first caller within TotpCodeCacheDuration, so concurrent
// logins cannot accept the same code twice. Redis uses SET NX; the in-memory
// fallback checks and records under one lock. An empty code is never accepted.
// A Redis failure is returned with false; the caller decides how to degrade.
func ConsumeTotpCode(ctx context.Context, userId int, totpCode string) (bool, error) {
	if totpCode == "" {
		return false, nil
	}
	key := totpCodeKey(userId, totpCode)
	if IsRedisEnabled() {
		if RDB == nil {
			return false, errors.New("redis not initialized")
		}
		first, err := RDB.SetNX(ctx, key, "1", TotpCodeCacheDuration).Result()
		if err != nil {
			return false, errors.Wrap(err, "record TOTP code use in redis")
		}
		return first, nil
	}
	return consumeMemoryKey(key, TotpCodeCacheDuration), nil
}

// isRedisKeyExists checks if a key exists in Redis
func isRedisKeyExists(ctx context.Context, key string) bool {
	exists, err := RDB.Exists(ctx, key).Result()
	if err != nil {
		logger.FromContext(ctx).Error("Redis exists check failed", zap.Error(err))
		return false
	}
	return exists > 0
}

// Memory cache for TOTP codes when Redis is not available. Every access holds
// totpMemoryMu: concurrent logins otherwise crash the process with a
// concurrent map write and can accept one code more than once.
var (
	totpMemoryMu    sync.Mutex
	totpMemoryCache = make(map[string]time.Time)
)

// isMemoryKeyExists checks if a key exists in memory cache
func isMemoryKeyExists(key string) bool {
	totpMemoryMu.Lock()
	defer totpMemoryMu.Unlock()
	expireTime, exists := totpMemoryCache[key]
	if !exists {
		return false
	}

	// Check if expired
	if time.Now().UTC().After(expireTime) {
		delete(totpMemoryCache, key)
		return false
	}

	return true
}

// setMemoryKey sets a key in memory cache with expiration
func setMemoryKey(key string, duration time.Duration) error {
	now := time.Now().UTC()
	totpMemoryMu.Lock()
	defer totpMemoryMu.Unlock()
	pruneExpiredTotpCodesLocked(now)
	totpMemoryCache[key] = now.Add(duration)
	return nil
}

// consumeMemoryKey records key for duration and reports whether it was absent
// or expired, checking and writing under one lock so exactly one caller wins.
func consumeMemoryKey(key string, duration time.Duration) bool {
	now := time.Now().UTC()
	totpMemoryMu.Lock()
	defer totpMemoryMu.Unlock()
	if expireTime, exists := totpMemoryCache[key]; exists && !now.After(expireTime) {
		return false
	}
	pruneExpiredTotpCodesLocked(now)
	totpMemoryCache[key] = now.Add(duration)
	return true
}

// pruneExpiredTotpCodesLocked removes entries that expired before now. Only
// verified codes are recorded, so the map stays small; the caller holds
// totpMemoryMu.
func pruneExpiredTotpCodesLocked(now time.Time) {
	for key, expireTime := range totpMemoryCache {
		if now.After(expireTime) {
			delete(totpMemoryCache, key)
		}
	}
}

// CleanupExpiredTotpCodes removes expired TOTP codes from memory cache
// This function should be called periodically when Redis is not available
func CleanupExpiredTotpCodes() {
	totpMemoryMu.Lock()
	defer totpMemoryMu.Unlock()
	pruneExpiredTotpCodesLocked(time.Now().UTC())
}
