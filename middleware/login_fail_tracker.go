package middleware

import (
	"crypto/sha256"
	"encoding/hex"
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
	entries       map[string]*loginFailEntry
	overflowUntil time.Time
}{
	entries: make(map[string]*loginFailEntry),
}

const loginFailExpiry = 10 * time.Minute
const loginFailMaxEntries = 10000

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

// pruneLoginFailEntries removes expired bounded records.
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

// loginFailureKey hashes caller input to keep per-entry memory independent of input size.
func loginFailureKey(username string) string {
	digest := sha256.Sum256([]byte(username))
	return hex.EncodeToString(digest[:])
}

// RecordLoginFailure records a bounded failed login or enables conservative overflow verification.
func RecordLoginFailure(username string) {
	username = loginFailureKey(username)
	loginFailTracker.Lock()
	defer loginFailTracker.Unlock()
	now := time.Now()
	if _, exists := loginFailTracker.entries[username]; !exists && len(loginFailTracker.entries) >= loginFailMaxEntries {
		// Require verification globally while full rather than evicting recent failures.
		loginFailTracker.overflowUntil = now.Add(loginFailExpiry)
		return
	}
	loginFailTracker.entries[username] = &loginFailEntry{lastFailAt: now}
}

// ClearLoginFailure removes the failed login record for the given username (after successful login).
func ClearLoginFailure(username string) {
	loginFailTracker.Lock()
	defer loginFailTracker.Unlock()
	delete(loginFailTracker.entries, loginFailureKey(username))
}

// HasLoginFailure returns true if the given username has a recent failed login attempt.
func HasLoginFailure(username string) bool {
	loginFailTracker.RLock()
	defer loginFailTracker.RUnlock()
	if time.Now().Before(loginFailTracker.overflowUntil) {
		return true
	}
	entry, ok := loginFailTracker.entries[loginFailureKey(username)]
	if !ok {
		return false
	}
	return time.Since(entry.lastFailAt) <= loginFailExpiry
}
