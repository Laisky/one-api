package common

import (
	"testing"
	"time"
)

// TestInMemoryRateLimiter_PeekAndRecord verifies the "gate then record on
// failure" semantics used by the redeem-failure limiter: PeekExceeded is
// read-only and only reports limited once maxRequestNum failures have been
// recorded within the window.
func TestInMemoryRateLimiter_PeekAndRecord(t *testing.T) {
	var l InMemoryRateLimiter
	l.Init(0)

	const key = "rateLimit:RDMF:42"
	const maxNum = 3
	const duration int64 = 600

	// Peeking never records, so any number of peeks keeps the budget empty.
	for i := 0; i < 5; i++ {
		if l.PeekExceeded(key, maxNum, duration) {
			t.Fatalf("peek %d reported limited before any failure recorded", i)
		}
	}

	// Record up to (maxNum-1) failures: still under budget.
	for i := 0; i < maxNum-1; i++ {
		l.Record(key, maxNum)
		if l.PeekExceeded(key, maxNum, duration) {
			t.Fatalf("limited after only %d failures (max %d)", i+1, maxNum)
		}
	}

	// The maxNum-th failure fills the budget; the next peek must report limited.
	l.Record(key, maxNum)
	if !l.PeekExceeded(key, maxNum, duration) {
		t.Fatalf("expected limited after %d failures", maxNum)
	}

	// A different user shares no state.
	if l.PeekExceeded("rateLimit:RDMF:99", maxNum, duration) {
		t.Fatal("unrelated user should not be limited")
	}
}

// TestInMemoryRateLimiter_WindowExpiry verifies that recorded failures age out
// of the sliding window so a user is no longer blocked once the window passes.
func TestInMemoryRateLimiter_WindowExpiry(t *testing.T) {
	var l InMemoryRateLimiter
	l.Init(0)

	const key = "rateLimit:RDMF:7"
	const maxNum = 2
	// 1-second window so the test stays fast.
	const duration int64 = 1

	l.Record(key, maxNum)
	l.Record(key, maxNum)
	if !l.PeekExceeded(key, maxNum, duration) {
		t.Fatal("expected limited after filling the budget")
	}

	// After the window elapses the oldest entry has aged out.
	time.Sleep(1100 * time.Millisecond)
	if l.PeekExceeded(key, maxNum, duration) {
		t.Fatal("expected budget to free up after the window elapsed")
	}
}
