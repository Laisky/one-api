package middleware

import (
	"container/list"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	loginFailExpiry         = 10 * time.Minute
	maxLoginFailEntries     = 10000
	maxLoginIdentifierBytes = 254
)

// loginFailEntry records a failure timestamp and its position in the eviction queue.
type loginFailEntry struct {
	lastFailAt time.Time
	position   *list.Element
}

// loginFailTracker bounds retained identifiers and maintains oldest-failure-first eviction.
var loginFailTracker = struct {
	sync.RWMutex
	entries map[string]*loginFailEntry
	order   list.List
}{entries: make(map[string]*loginFailEntry)}

// ValidLoginIdentifier reports whether an identifier fits the bounded login contract without rewriting it.
func ValidLoginIdentifier(username string) bool {
	return len(username) > 0 && len(username) <= maxLoginIdentifierBytes && utf8.ValidString(username) && strings.TrimSpace(username) != ""
}

// init starts the existing periodic expiry cleanup for the bounded tracker.
func init() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			pruneLoginFailEntries()
		}
	}()
}

// pruneLoginFailEntries removes expired records and their queue nodes under the tracker lock.
func pruneLoginFailEntries() {
	now := time.Now()
	loginFailTracker.Lock()
	defer loginFailTracker.Unlock()
	for username, entry := range loginFailTracker.entries {
		if now.Sub(entry.lastFailAt) > loginFailExpiry {
			loginFailTracker.order.Remove(entry.position)
			delete(loginFailTracker.entries, username)
		}
	}
}

// RecordLoginFailure retains a bounded identifier and evicts the oldest failure in constant time when full.
func RecordLoginFailure(username string) {
	if !ValidLoginIdentifier(username) {
		return
	}
	loginFailTracker.Lock()
	defer loginFailTracker.Unlock()
	if entry, ok := loginFailTracker.entries[username]; ok {
		entry.lastFailAt = time.Now()
		loginFailTracker.order.MoveToBack(entry.position)
		return
	}
	if len(loginFailTracker.entries) >= maxLoginFailEntries {
		oldest := loginFailTracker.order.Front()
		delete(loginFailTracker.entries, oldest.Value.(string))
		loginFailTracker.order.Remove(oldest)
	}
	// Clone so a short identifier cannot retain a much larger caller-owned backing string.
	username = strings.Clone(username)
	loginFailTracker.entries[username] = &loginFailEntry{lastFailAt: time.Now(), position: loginFailTracker.order.PushBack(username)}
}

// ClearLoginFailure removes both the identifier and its eviction node after a successful login.
func ClearLoginFailure(username string) {
	loginFailTracker.Lock()
	defer loginFailTracker.Unlock()
	if entry, ok := loginFailTracker.entries[username]; ok {
		loginFailTracker.order.Remove(entry.position)
		delete(loginFailTracker.entries, username)
	}
}

// HasLoginFailure reports whether a valid identifier has a recent retained failure.
func HasLoginFailure(username string) bool {
	if !ValidLoginIdentifier(username) {
		return false
	}
	loginFailTracker.RLock()
	defer loginFailTracker.RUnlock()
	entry, ok := loginFailTracker.entries[username]
	return ok && time.Since(entry.lastFailAt) <= loginFailExpiry
}
