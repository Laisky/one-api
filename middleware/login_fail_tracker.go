package middleware

import (
	"sync"
	"time"
)

// loginFailEntry records a failed login attempt timestamp for a username.
type loginFailEntry struct {
	lastFailAt time.Time
}

// loginFailTracker tracks usernames that have had recent failed login attempts.
// After a failed login, the username is recorded. Subsequent login attempts for
// the same username will require Turnstile verification until the entry expires
// or the user logs in successfully.
var loginFailTracker = struct {
	sync.RWMutex
	entries map[string]*loginFailEntry
}{
	entries: make(map[string]*loginFailEntry),
}

const loginFailExpiry = 10 * time.Minute

func init() {
	// Background goroutine to prune expired entries every 5 minutes.
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			pruneLoginFailEntries()
		}
	}()
}

func pruneLoginFailEntries() {
	now := time.Now()
	loginFailTracker.Lock()
	defer loginFailTracker.Unlock()
	for username, entry := range loginFailTracker.entries {
		if now.Sub(entry.lastFailAt) > loginFailExpiry {
			delete(loginFailTracker.entries, username)
		}
	}
}

// RecordLoginFailure marks the given username as having a failed login attempt.
func RecordLoginFailure(username string) {
	loginFailTracker.Lock()
	defer loginFailTracker.Unlock()
	loginFailTracker.entries[username] = &loginFailEntry{lastFailAt: time.Now()}
}

// ClearLoginFailure removes the failed login record for the given username (after successful login).
func ClearLoginFailure(username string) {
	loginFailTracker.Lock()
	defer loginFailTracker.Unlock()
	delete(loginFailTracker.entries, username)
}

// HasLoginFailure returns true if the given username has a recent failed login attempt.
func HasLoginFailure(username string) bool {
	loginFailTracker.RLock()
	defer loginFailTracker.RUnlock()
	entry, ok := loginFailTracker.entries[username]
	if !ok {
		return false
	}
	return time.Since(entry.lastFailAt) <= loginFailExpiry
}
