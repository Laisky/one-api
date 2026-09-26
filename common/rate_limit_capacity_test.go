package common

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// legacyLimiter is an independent copy of the pre-change timestamp state machine.
type legacyLimiter struct{ store map[string][]int64 }

// request returns the original Request decision at a supplied wall-clock second.
func (l *legacyLimiter) request(key string, limit int, duration, now int64) bool {
	q, ok := l.store[key]
	if !ok {
		l.store[key] = append(make([]int64, 0, limit), now)
		return true
	}
	if len(q) < limit {
		l.store[key] = append(q, now)
		return true
	}
	if now-q[0] >= duration {
		l.store[key] = append(q[1:], now)
		return true
	}
	return false
}

// record implements the original always-record transition, including a changing limit.
func (l *legacyLimiter) record(key string, limit int, now int64) {
	q, ok := l.store[key]
	if !ok {
		l.store[key] = append(make([]int64, 0, limit), now)
		return
	}
	q = append(q, now)
	if len(q) > limit {
		q = q[len(q)-limit:]
	}
	l.store[key] = q
}

// peek implements the original nonmutating budget probe at a supplied second.
func (l *legacyLimiter) peek(key string, limit int, duration, now int64) bool {
	q, ok := l.store[key]
	if !ok || len(q) < limit {
		return false
	}
	return now-q[0] < duration
}

// TestRateLimiterSparseCapacity verifies that a high ceiling does not allocate an unused full-size history.
func TestRateLimiterSparseCapacity(t *testing.T) {
	for _, record := range []bool{false, true} {
		t.Run(fmt.Sprint(record), func(t *testing.T) {
			var l InMemoryRateLimiter
			l.Init(0)
			for i := 0; i < 1000; i++ {
				if record {
					l.Record("sparse", 10000000)
				} else {
					require.True(t, l.Request("sparse", 10000000, 3600))
				}
				q := *l.store["sparse"]
				require.Len(t, q, i+1)
				require.LessOrEqual(t, cap(q), max(16, 2*len(q)), "capacity must follow recorded history, not the configured ceiling")
			}
		})
	}
}

// TestRateLimiterCapacityPreservesTransitions compares 20,000 deterministic operations with the legacy model.
func TestRateLimiterCapacityPreservesTransitions(t *testing.T) {
	var l InMemoryRateLimiter
	l.Init(0)
	legacy := legacyLimiter{store: make(map[string][]int64)}
	rng := rand.New(rand.NewSource(427))
	limits := []int{1, 2, 16, 17, 63, 1000}
	for i := 0; i < 20000; i++ {
		key := fmt.Sprint(rng.Intn(7))
		limit := limits[rng.Intn(len(limits))]
		duration := int64(3600)
		if rng.Intn(3) == 0 {
			duration = 0
		}
		now := time.Now().Unix()
		switch rng.Intn(3) {
		case 0:
			require.Equal(t, legacy.request(key, limit, duration, now), l.Request(key, limit, duration))
		case 1:
			legacy.record(key, limit, now)
			l.Record(key, limit)
		case 2:
			before := len(l.store)
			require.Equal(t, legacy.peek(key, limit, duration, now), l.PeekExceeded(key, limit, duration))
			require.Len(t, l.store, before, "peek must not create state")
		}
		for k, q := range legacy.store {
			require.Len(t, *l.store[k], len(q))
		}
	}
}

// TestRateLimiterCapacityExpiryAndInvalidLimits preserves expiry equality and legacy zero/negative inputs.
func TestRateLimiterCapacityExpiryAndInvalidLimits(t *testing.T) {
	var l InMemoryRateLimiter
	l.Init(0)
	old := []int64{time.Now().Unix() - 60, time.Now().Unix() - 60, time.Now().Unix() - 60}
	l.store["old"] = &old
	require.False(t, l.PeekExceeded("old", 3, 60))
	for i := 0; i < 3; i++ {
		require.True(t, l.Request("old", 3, 60))
	}
	require.False(t, l.Request("old", 3, 60))
	require.True(t, l.PeekExceeded("old", 3, 60))
	require.Panics(t, func() { l.Request("negative-request", -1, 60) })
	require.Panics(t, func() { l.Record("negative-record", -1) })
	require.NotContains(t, l.store, "negative-request")
	require.NotContains(t, l.store, "negative-record")
	require.True(t, l.Request("zero-request", 0, 3600))
	require.False(t, l.Request("zero-request", 0, 3600))
	l.Record("zero-record", 0)
	require.Len(t, *l.store["zero-record"], 1)
	l.Record("zero-record", 0)
	require.Empty(t, *l.store["zero-record"])
	require.Panics(t, func() { l.PeekExceeded("zero-record", 0, 3600) })
}

// TestRateLimiterCapacityConcurrentAdmission requires exact admission under concurrent requests and readonly probes.
func TestRateLimiterCapacityConcurrentAdmission(t *testing.T) {
	var l InMemoryRateLimiter
	l.Init(0)
	var admitted atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 32; j++ {
				l.PeekExceeded("shared", 37, 3600)
				if l.Request("shared", 37, 3600) {
					admitted.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int64(37), admitted.Load())
	require.Len(t, *l.store["shared"], 37)
}

// BenchmarkRateLimiterSparseKey measures cold-key allocation separately from steady streaming behavior.
func BenchmarkRateLimiterSparseKey(b *testing.B) {
	for _, limit := range []int{1000, 1000000} {
		b.Run(fmt.Sprint(limit), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var l InMemoryRateLimiter
				l.Init(0)
				l.Request("key", limit, 3600)
			}
		})
	}
}
