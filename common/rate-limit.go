package common

import (
	"sync"
	"time"
)

// InMemoryRateLimiter keeps per-key request timestamps to enforce simple in-memory quotas.
// It is safe for concurrent use within a single process.
type InMemoryRateLimiter struct {
	store              map[string]*[]int64
	mutex              sync.Mutex
	expirationDuration time.Duration
}

// Init prepares the rate limiter with an optional expiration window for idle keys.
// When expirationDuration is greater than zero, a background goroutine periodically prunes stale entries.
func (l *InMemoryRateLimiter) Init(expirationDuration time.Duration) {
	if l.store == nil {
		l.mutex.Lock()
		if l.store == nil {
			l.store = make(map[string]*[]int64)
			l.expirationDuration = expirationDuration
			if expirationDuration > 0 {
				go l.clearExpiredItems()
			}
		}
		l.mutex.Unlock()
	}
}

func (l *InMemoryRateLimiter) clearExpiredItems() {
	for {
		time.Sleep(l.expirationDuration)
		l.mutex.Lock()
		now := time.Now().Unix()
		for key := range l.store {
			queue := l.store[key]
			size := len(*queue)
			if size == 0 || now-(*queue)[size-1] > int64(l.expirationDuration.Seconds()) {
				delete(l.store, key)
			}
		}
		l.mutex.Unlock()
	}
}

// Request evaluates whether the key can issue another request under the provided constraints.
// maxRequestNum limits the number of requests recorded within the sliding duration window, whose unit is seconds.
func (l *InMemoryRateLimiter) Request(key string, maxRequestNum int, duration int64) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	// [old <-- new]
	queue, ok := l.store[key]
	now := time.Now().Unix()
	if ok {
		if len(*queue) < maxRequestNum {
			*queue = append(*queue, now)
			return true
		} else {
			if now-(*queue)[0] >= duration {
				*queue = (*queue)[1:]
				*queue = append(*queue, now)
				return true
			} else {
				return false
			}
		}
	} else {
		s := make([]int64, 0, maxRequestNum)
		l.store[key] = &s
		*(l.store[key]) = append(*(l.store[key]), now)
	}
	return true
}

// PeekExceeded reports whether the key already has at least maxRequestNum
// timestamps recorded within the sliding window, WITHOUT recording a new one.
// This is the read-only companion to Record, used to gate an action before the
// work is attempted so that probe checks never consume the budget themselves.
func (l *InMemoryRateLimiter) PeekExceeded(key string, maxRequestNum int, duration int64) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	queue, ok := l.store[key]
	if !ok || len(*queue) < maxRequestNum {
		return false
	}
	// The oldest retained timestamp is at the front. If it has aged out of the
	// window the budget is no longer full, so the caller is not limited yet.
	now := time.Now().Unix()
	return now-(*queue)[0] < duration
}

// Record appends a timestamp for the key, bounding the stored slice to the most
// recent maxRequestNum entries. Unlike Request it never rejects; it only logs
// the event. Pair it with PeekExceeded to build a "gate then record on failure"
// limiter where only failures consume the budget.
func (l *InMemoryRateLimiter) Record(key string, maxRequestNum int) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	now := time.Now().Unix()
	queue, ok := l.store[key]
	if !ok {
		s := make([]int64, 0, maxRequestNum)
		s = append(s, now)
		l.store[key] = &s
		return
	}
	*queue = append(*queue, now)
	if len(*queue) > maxRequestNum {
		*queue = (*queue)[len(*queue)-maxRequestNum:]
	}
}
