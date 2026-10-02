package controller

// Bounded in-process cache for log counts (W2.4 items 5 and 6).
//
// Counting is the expensive part of a list response, so it is worth reusing.
// What must NOT be reused is the claim: a cached exact count is exact as of the
// instant it ran, not as of now, so every cached entry carries that instant and
// is republished with cached=true.
//
// The cache is in-process and bounded. It works without Redis, which the
// zero-dependency default deployment requires, and it cannot grow without limit
// because a caller can vary the filter freely.

import (
	"sync"
	"time"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
)

// logCountCacheMaxEntries bounds the cache so varied filters cannot grow it
// without limit.
const logCountCacheMaxEntries = 512

type logCountCacheEntry struct {
	count     model.LogCount
	expiresAt time.Time
}

var (
	logCountCacheMu sync.Mutex
	logCountCache   = make(map[string]logCountCacheEntry, logCountCacheMaxEntries)
	// logCountCacheNow is the clock, replaceable in tests.
	logCountCacheNow = time.Now
)

// logCountCacheKey composes the cache key for a request.
//
// The key includes the caller's authorization scope and the normalized query,
// both through the filter digest, plus the counting mode. A cached count can
// therefore never be served to a caller whose scope differs from the one that
// produced it.
//
// Parameters:
//   - req: the validated cursor request.
//
// Return values:
//   - string: the cache key.
func logCountCacheKey(req logCursorRequest) string {
	mode := "probe"
	if req.wantsExact {
		mode = "exact"
	}
	return req.endpoint + "|" + mode + "|" + req.filter.Digest(req.scope, req.endpoint)
}

// lookupCachedLogCount returns a cached count when one is still valid.
//
// Parameters:
//   - req: the validated cursor request.
//
// Return values:
//   - model.LogCount: the cached count, marked cached, with its original AsOf.
//   - bool: whether a valid entry was found.
func lookupCachedLogCount(req logCursorRequest) (model.LogCount, bool) {
	if config.LogCountCacheTTL() <= 0 {
		return model.LogCount{}, false
	}

	key := logCountCacheKey(req)

	logCountCacheMu.Lock()
	defer logCountCacheMu.Unlock()

	entry, ok := logCountCache[key]
	if !ok || logCountCacheNow().After(entry.expiresAt) {
		return model.LogCount{}, false
	}

	count := entry.count
	count.Cached = true
	return count, true
}

// storeCachedLogCount records a computed count.
//
// An unavailable count is not cached: it says nothing worth remembering, and
// caching it would extend a transient budget exhaustion across the TTL.
//
// Parameters:
//   - req: the validated cursor request.
//   - count: the freshly computed count.
//
// Return values: none.
func storeCachedLogCount(req logCursorRequest, count model.LogCount) {
	ttl := config.LogCountCacheTTL()
	if ttl <= 0 || count.Quality == model.LogCountUnavailable {
		return
	}

	logCountCacheMu.Lock()
	defer logCountCacheMu.Unlock()

	if len(logCountCache) >= logCountCacheMaxEntries {
		// Bounded and simple: drop everything rather than track recency. The
		// cost is a brief recomputation, and the bound is what matters.
		clear(logCountCache)
	}

	logCountCache[logCountCacheKey(req)] = logCountCacheEntry{
		count:     count,
		expiresAt: logCountCacheNow().Add(ttl),
	}
}

// ResetLogCountCache clears the cache.
//
// Parameters: none.
//
// Return values: none.
func ResetLogCountCache() {
	logCountCacheMu.Lock()
	defer logCountCacheMu.Unlock()
	clear(logCountCache)
}
