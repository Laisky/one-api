package model

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/logger"
)

// fillCacheBestEffort stores a value that was just read from the authoritative
// database. Redis is a cache-aside optimization here, never a correctness
// dependency: a failed fill is reported once, as a stackless WARN, and is never
// returned, so the caller keeps the authoritative value and its request
// proceeds. The cache key is never logged because some keys carry credentials.
//
// Parameters:
//   - ctx carries the request-scoped logger and bounds the Redis write.
//   - key, value and ttl describe the cache entry.
//   - message names the entry in the WARN record.
//   - fields identify the entity without exposing secrets.
func fillCacheBestEffort(ctx context.Context, key, value string, ttl time.Duration, message string, fields ...zap.Field) {
	if err := common.RedisSet(ctx, key, value, ttl); err != nil {
		logger.FromContext(ctx).Warn(message, append(fields, zap.String("error", err.Error()))...)
	}
}

// getTokenByKey reads the token identified by key from the database. It
// returns the token, or a wrapped error when the database is unavailable or
// the lookup fails.
func getTokenByKey(key string) (*Token, error) {
	if DB == nil {
		return nil, errors.New("database not initialized")
	}
	keyCol := "`key`"
	if common.UsingPostgreSQL.Load() {
		keyCol = `"key"`
	}
	var token Token
	if err := DB.Where(keyCol+" = ?", key).First(&token).Error; err != nil {
		return nil, errors.Wrap(err, "get token by key")
	}
	return &token, nil
}
