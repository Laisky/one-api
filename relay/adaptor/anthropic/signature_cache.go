package anthropic

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/one-api/relay/model"
)

// SignatureCacheBackend defines the interface for signature cache backends
type SignatureCacheBackend interface {
	Store(key, signature string, ttl time.Duration) error
	Get(key string) (*string, error)
	Delete(key string) error
}

// ThinkingSignatureCache manages caching of Claude thinking block signatures
type ThinkingSignatureCache struct {
	signatures map[string]string
	expiry     map[string]time.Time
	mutex      sync.RWMutex
	ttl        time.Duration
	backend    SignatureCacheBackend // Optional Redis backend
}

// SignatureCacheEntry represents a cached signature with metadata
type SignatureCacheEntry struct {
	Signature string
	ExpiresAt time.Time
}

// NewThinkingSignatureCache creates a new signature cache with specified TTL
func NewThinkingSignatureCache(ttl time.Duration) *ThinkingSignatureCache {
	cache := &ThinkingSignatureCache{
		signatures: make(map[string]string),
		expiry:     make(map[string]time.Time),
		ttl:        ttl,
	}

	// Start cleanup goroutine
	go cache.startCleanup()

	return cache
}

// SetBackend sets the cache backend (e.g., Redis)
func (tsc *ThinkingSignatureCache) SetBackend(backend SignatureCacheBackend) {
	tsc.backend = backend
}

// Store stores a signature in the cache with TTL
func (tsc *ThinkingSignatureCache) Store(key, signature string) {
	// Try backend first if available
	if tsc.backend != nil {
		if err := tsc.backend.Store(key, signature, tsc.ttl); err == nil {
			return // Successfully stored in backend
		}
		// Fall back to in-memory cache if backend fails
	}

	tsc.mutex.Lock()
	defer tsc.mutex.Unlock()

	tsc.signatures[key] = signature
	tsc.expiry[key] = time.Now().Add(tsc.ttl)
}

// Get retrieves a signature from the cache
func (tsc *ThinkingSignatureCache) Get(key string) *string {
	// Try backend first if available
	if tsc.backend != nil {
		if signature, err := tsc.backend.Get(key); err == nil && signature != nil {
			return signature
		}
		// Fall back to in-memory cache if backend fails or returns nil
	}

	tsc.mutex.RLock()
	defer tsc.mutex.RUnlock()

	// Check if key exists and hasn't expired
	if expiry, exists := tsc.expiry[key]; exists {
		if time.Now().Before(expiry) {
			if signature, sigExists := tsc.signatures[key]; sigExists {
				return &signature
			}
		}
	}

	return nil
}

// Delete removes a signature from the cache
func (tsc *ThinkingSignatureCache) Delete(key string) {
	tsc.mutex.Lock()
	defer tsc.mutex.Unlock()

	delete(tsc.signatures, key)
	delete(tsc.expiry, key)
}

// startCleanup runs periodic cleanup of expired entries
func (tsc *ThinkingSignatureCache) startCleanup() {
	ticker := time.NewTicker(time.Hour) // Cleanup every hour
	defer ticker.Stop()

	for range ticker.C {
		tsc.cleanupExpired()
	}
}

// cleanupExpired removes expired entries from the cache
func (tsc *ThinkingSignatureCache) cleanupExpired() {
	tsc.mutex.Lock()
	defer tsc.mutex.Unlock()

	now := time.Now()
	for key, expiry := range tsc.expiry {
		if now.After(expiry) {
			delete(tsc.signatures, key)
			delete(tsc.expiry, key)
		}
	}
}

// Size returns the current number of cached signatures
func (tsc *ThinkingSignatureCache) Size() int {
	tsc.mutex.RLock()
	defer tsc.mutex.RUnlock()

	return len(tsc.signatures)
}

// generateSignatureKey creates a unique cache key for a thinking block signature
func generateSignatureKey(tokenID, conversationID string, messageIndex, thinkingIndex int) string {
	return fmt.Sprintf("thinking_sig:%s:%s:%d:%d", tokenID, conversationID, messageIndex, thinkingIndex)
}

// generateConversationID creates a deterministic conversation ID from message history
func generateConversationID(messages []model.Message) string {
	var signature strings.Builder

	for i, msg := range messages {
		if msg.Role == "user" {
			// Use user message content for conversation identity
			content := truncateForHash(msg.StringContent(), 100)
			signature.WriteString(fmt.Sprintf("u%d:%s;", i, content))
		} else if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			// Include tool calls in conversation identity (but not thinking content)
			signature.WriteString(fmt.Sprintf("a%d:tools:%d;", i, len(msg.ToolCalls)))
		}
	}

	// If no signature content, use a default
	if signature.Len() == 0 {
		signature.WriteString("empty_conversation")
	}

	hash := sha256.Sum256([]byte(signature.String()))
	return fmt.Sprintf("conv_%x", hash[:8]) // Use first 8 bytes for shorter key
}

// truncateForHash truncates content to specified length for hash generation
func truncateForHash(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen]
}

// getTokenIDFromRequest extracts the token ID from the request context
func getTokenIDFromRequest(tokenID int) string {
	return fmt.Sprintf("token_%d", tokenID)
}

// Global signature cache instance
var globalSignatureCache *ThinkingSignatureCache

// InitSignatureCache initializes the global signature cache
func InitSignatureCache(ttl time.Duration) {
	if ttl == 0 {
		ttl = 24 * time.Hour // Default 24 hour TTL
	}
	globalSignatureCache = NewThinkingSignatureCache(ttl)
}

// GetSignatureCache returns the global signature cache instance
func GetSignatureCache() *ThinkingSignatureCache {
	if globalSignatureCache == nil {
		InitSignatureCache(24 * time.Hour)
	}
	return globalSignatureCache
}
