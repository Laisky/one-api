package middleware

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestSecurityLoginTrackerBudget verifies a flood cannot exceed the fixed cardinality or retain oversized keys.
func TestSecurityLoginTrackerBudget(t *testing.T) {
	prefix := "security-login-budget-"
	for i := 0; i < 11000; i++ {
		RecordLoginFailure(fmt.Sprintf("%s%d", prefix, i))
	}
	t.Cleanup(func() {
		for i := 0; i < 11000; i++ {
			ClearLoginFailure(fmt.Sprintf("%s%d", prefix, i))
		}
	})
	loginFailTracker.RLock()
	size := len(loginFailTracker.entries)
	loginFailTracker.RUnlock()
	require.LessOrEqual(t, size, 10000)
	require.False(t, HasLoginFailure(prefix+"0"), "oldest failures must be evicted predictably")
	require.True(t, HasLoginFailure(prefix+"10999"))
	for _, bad := range []string{"", strings.Repeat("x", 255), string([]byte{0xff}), "   "} {
		RecordLoginFailure(bad)
		require.False(t, HasLoginFailure(bad))
		ClearLoginFailure(bad)
	}
}

// TestSecurityLoginTrackerConcurrentLifecycle preserves clearing, expiry and thread safety.
func TestSecurityLoginTrackerConcurrentLifecycle(t *testing.T) {
	var workers sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			for i := 0; i < 200; i++ {
				key := fmt.Sprintf("security-concurrent-%d-%d", worker, i)
				RecordLoginFailure(key)
				HasLoginFailure(key)
				ClearLoginFailure(key)
			}
		}(worker)
	}
	workers.Wait()
	key := "security-expiry"
	RecordLoginFailure(key)
	require.True(t, HasLoginFailure(key))
	loginFailTracker.Lock()
	loginFailTracker.entries[key].lastFailAt = time.Now().Add(-11 * time.Minute)
	loginFailTracker.Unlock()
	require.False(t, HasLoginFailure(key))
	pruneLoginFailEntries()
	ClearLoginFailure(key)
	RecordLoginFailure(key)
	require.True(t, HasLoginFailure(key))
	ClearLoginFailure(key)
	require.False(t, HasLoginFailure(key))
}
