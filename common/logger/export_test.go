package logger

import (
	"context"
	"sync"
	"time"
)

// ResetSetupLogOnceForTests resets the setupLogOnce guard so tests can re-run SetupLogger without touching
// unexported state in production code. It must only be used from test files.
func ResetSetupLogOnceForTests() {
	setupLogOnce = sync.Once{}
}

// WaitForLogRetentionCleanerForTests blocks until all retention cleaner goroutines finish.
// Background has no cancellation or deadline, so the shared join cannot return a timeout.
func WaitForLogRetentionCleanerForTests() {
	_ = WaitForRetentionWorkers(context.Background())
}

// SetRotationNowFuncForTests overrides the rotation clock to make time deterministic in tests.
func SetRotationNowFuncForTests(f func() time.Time) {
	rotationNow = f
}

// ResetRotationNowFuncForTests restores the default wall clock used by rotation writers.
func ResetRotationNowFuncForTests() {
	rotationNow = defaultRotationNow
}
