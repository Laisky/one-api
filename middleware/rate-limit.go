package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
)

var timeFormat = "2006-01-02T15:04:05.000Z"

var inMemoryRateLimiter common.InMemoryRateLimiter

func redisRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string) {
	ctx := gmw.Ctx(c)

	key := fmt.Sprintf("rateLimit:%s:%s", mark, c.ClientIP())

	switch mark {
	case "GR":
		hashedToken := sha256.Sum256([]byte(GetTokenKeyParts(c)[0]))
		key = fmt.Sprintf("rateLimit:%s:%s", mark, hex.EncodeToString(hashedToken[:8]))
	case "CR":
		maxRequestNum = c.GetInt(ctxkey.RateLimit)
		if maxRequestNum <= 0 {
			return
		}
		hashedToken := sha256.Sum256([]byte(GetTokenKeyParts(c)[0]))
		key = fmt.Sprintf("rateLimit:%s:%s:%d", mark, hex.EncodeToString(hashedToken[:8]), c.GetInt(ctxkey.ChannelId))
	}

	rdb := common.RDB
	listLength, err := rdb.LLen(ctx, key).Result()
	if err != nil {
		AbortWithError(c, http.StatusInternalServerError, errors.Wrap(err, "failed to get list length"))
		return
	}

	if listLength < int64(maxRequestNum) {
		rdb.LPush(ctx, key, time.Now().Format(timeFormat))
		rdb.Expire(ctx, key, config.RateLimitKeyExpirationDuration)
	} else {
		oldTimeStr, err := rdb.LIndex(ctx, key, -1).Result()
		if err != nil {
			AbortWithError(c, http.StatusInternalServerError, errors.Wrap(err, "failed to get old time"))
			return
		}

		oldTime, err := time.Parse(timeFormat, oldTimeStr)
		if err != nil {
			AbortWithError(c, http.StatusInternalServerError, errors.Wrap(err, "failed to parse old time"))
			return
		}

		nowTimeStr := time.Now().Format(timeFormat)
		nowTime, err := time.Parse(timeFormat, nowTimeStr)
		if err != nil {
			AbortWithError(c, http.StatusInternalServerError, errors.Wrap(err, "failed to parse now time"))
			return
		}

		// time.Since will return negative number!
		// See: https://stackoverflow.com/questions/50970900/why-is-time-since-returning-negative-durations-on-windows
		if int64(nowTime.Sub(oldTime).Seconds()) < duration {
			rdb.Expire(ctx, key, config.RateLimitKeyExpirationDuration)
			AbortWithError(c, http.StatusTooManyRequests, errors.New("rate limit exceeded"))
		} else {
			rdb.LPush(ctx, key, time.Now().Format(timeFormat))
			rdb.LTrim(ctx, key, 0, int64(maxRequestNum-1))
			rdb.Expire(ctx, key, config.RateLimitKeyExpirationDuration)
		}
	}
}

func memoryRateLimiter(c *gin.Context, maxRequestNum int, duration int64, mark string) {
	key := fmt.Sprintf("rateLimit:%s:%s", mark, c.ClientIP())

	switch mark {
	case "GR":
		hashedToken := sha256.Sum256([]byte(GetTokenKeyParts(c)[0]))
		key = fmt.Sprintf("rateLimit:%s:%s", mark, hex.EncodeToString(hashedToken[:8]))
	case "CR":
		maxRequestNum = c.GetInt(ctxkey.RateLimit)
		if maxRequestNum <= 0 {
			return
		}
		hashedToken := sha256.Sum256([]byte(GetTokenKeyParts(c)[0]))
		key = fmt.Sprintf("rateLimit:%s:%s:%d", mark, hex.EncodeToString(hashedToken[:8]), c.GetInt(ctxkey.ChannelId))
	}

	if !inMemoryRateLimiter.Request(key, maxRequestNum, duration) {
		AbortWithError(c, http.StatusTooManyRequests, errors.New("rate limit exceeded"))
		return
	}
}

func rateLimitFactory(maxRequestNum int, duration int64, mark string) func(c *gin.Context) {
	if maxRequestNum <= 0 || config.DebugEnabled {
		return func(c *gin.Context) {
			c.Next()
		}
	}
	if common.IsRedisEnabled() {
		return func(c *gin.Context) {
			redisRateLimiter(c, maxRequestNum, duration, mark)
		}
	} else {
		// It's safe to call multi times.
		inMemoryRateLimiter.Init(config.RateLimitKeyExpirationDuration)
		return func(c *gin.Context) {
			memoryRateLimiter(c, maxRequestNum, duration, mark)
		}
	}
}

func GlobalWebRateLimit() func(c *gin.Context) {
	return rateLimitFactory(config.GlobalWebRateLimitNum, config.GlobalWebRateLimitDuration, "GW")
}

func GlobalAPIRateLimit() func(c *gin.Context) {
	return rateLimitFactory(config.GlobalApiRateLimitNum, config.GlobalApiRateLimitDuration, "GA")
}

func CriticalRateLimit() func(c *gin.Context) {
	return rateLimitFactory(config.CriticalRateLimitNum, config.CriticalRateLimitDuration, "CT")
}

func DownloadRateLimit() func(c *gin.Context) {
	return rateLimitFactory(config.DownloadRateLimitNum, config.DownloadRateLimitDuration, "DW")
}

func UploadRateLimit() func(c *gin.Context) {
	return rateLimitFactory(config.UploadRateLimitNum, config.UploadRateLimitDuration, "UP")
}

func GlobalRelayRateLimit() func(c *gin.Context) {
	return rateLimitFactory(config.GlobalRelayRateLimitNum, config.GlobalRelayRateLimitDuration, "GR")
}

func ChannelRateLimit() func(c *gin.Context) {
	maxRequestNum := 0
	if config.ChannelRateLimitEnabled {
		maxRequestNum = 1
	}
	return rateLimitFactory(maxRequestNum, config.ChannelRateLimitDuration, "CR")
}

// TotpRateLimit limits TOTP verification attempts to 1 per second per user
func TotpRateLimit() func(c *gin.Context) {
	return rateLimitFactory(1, 1, "TOTP")
}

// CheckTotpRateLimit checks if user can make TOTP verification request
func CheckTotpRateLimit(c *gin.Context, userId int) bool {
	if config.DebugEnabled {
		return true
	}

	key := fmt.Sprintf("rateLimit:TOTP:%d", userId)

	if common.IsRedisEnabled() {
		return checkRedisRateLimit(c, key, 1, 1)
	} else {
		inMemoryRateLimiter.Init(config.RateLimitKeyExpirationDuration)
		return inMemoryRateLimiter.Request(key, 1, 1)
	}
}

// checkRedisRateLimit checks rate limit using Redis
func checkRedisRateLimit(c *gin.Context, key string, maxRequestNum int, duration int64) bool {
	ctx := gmw.Ctx(c)
	rdb := common.RDB
	lg := gmw.GetLogger(c)

	listLength, err := rdb.LLen(ctx, key).Result()
	if err != nil {
		// If Redis fails, allow the request but log the error
		lg.Warn("Redis rate limit check failed, allowing request", zap.String("key", key), zap.Error(err))
		return true
	}

	if listLength < int64(maxRequestNum) {
		rdb.LPush(ctx, key, time.Now().Format(timeFormat))
		rdb.Expire(ctx, key, config.RateLimitKeyExpirationDuration)
		return true
	} else {
		oldTimeStr, err := rdb.LIndex(ctx, key, -1).Result()
		if err != nil {
			lg.Warn("Redis rate limit get old time failed, allowing request", zap.String("key", key), zap.Error(err))
			return true
		}

		oldTime, err := time.Parse(timeFormat, oldTimeStr)
		if err != nil {
			lg.Warn("Redis rate limit parse old time failed, allowing request", zap.String("key", key), zap.String("time_str", oldTimeStr), zap.Error(err))
			return true
		}

		nowTime := time.Now()
		if int64(nowTime.Sub(oldTime).Seconds()) < duration {
			rdb.Expire(ctx, key, config.RateLimitKeyExpirationDuration)
			return false
		} else {
			rdb.LPush(ctx, key, nowTime.Format(timeFormat))
			rdb.LTrim(ctx, key, 0, int64(maxRequestNum-1))
			rdb.Expire(ctx, key, config.RateLimitKeyExpirationDuration)
			return true
		}
	}
}
