package common

import (
	"context"
	"fmt"
	"time"

	"github.com/Laisky/zap"

	"github.com/songquanpeng/one-api/common/logger"
)

const (
	// TotpCodeCacheDuration is the duration for which a TOTP code is cached to prevent replay
	TotpCodeCacheDuration = 30 * time.Second
	// TotpCodeCacheKeyPrefix is the prefix for TOTP code cache keys
	TotpCodeCacheKeyPrefix = "totp_used"
)

// IsTotpCodeUsed checks if a TOTP code has been used recently (within 30 seconds)
func IsTotpCodeUsed(ctx context.Context, userId int, totpCode string) bool {
	if totpCode == "" {
		return false
	}

	key := fmt.Sprintf("%s:%d:%s", TotpCodeCacheKeyPrefix, userId, totpCode)

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

	key := fmt.Sprintf("%s:%d:%s", TotpCodeCacheKeyPrefix, userId, totpCode)

	if IsRedisEnabled() {
		return RedisSet(ctx, key, "1", TotpCodeCacheDuration)
	} else {
		// For memory cache, we'll use a simple in-memory map
		return setMemoryKey(key, TotpCodeCacheDuration)
	}
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

// Memory cache for TOTP codes when Redis is not available
var totpMemoryCache = make(map[string]time.Time)

// isMemoryKeyExists checks if a key exists in memory cache
func isMemoryKeyExists(key string) bool {
	expireTime, exists := totpMemoryCache[key]
	if !exists {
		return false
	}

	// Check if expired
	if time.Now().After(expireTime) {
		delete(totpMemoryCache, key)
		return false
	}

	return true
}

// setMemoryKey sets a key in memory cache with expiration
func setMemoryKey(key string, duration time.Duration) error {
	totpMemoryCache[key] = time.Now().Add(duration)

	// Clean up expired keys periodically (simple cleanup)
	go func() {
		time.Sleep(duration + time.Minute) // Clean up after expiration + buffer
		if expireTime, exists := totpMemoryCache[key]; exists {
			if time.Now().After(expireTime) {
				delete(totpMemoryCache, key)
			}
		}
	}()

	return nil
}

// CleanupExpiredTotpCodes removes expired TOTP codes from memory cache
// This function should be called periodically when Redis is not available
func CleanupExpiredTotpCodes() {
	now := time.Now()
	for key, expireTime := range totpMemoryCache {
		if now.After(expireTime) {
			delete(totpMemoryCache, key)
		}
	}
}
