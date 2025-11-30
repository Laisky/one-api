package anthropic

import (
	"testing"
	"time"

	"github.com/songquanpeng/one-api/relay/model"
	"github.com/stretchr/testify/require"
)

func TestThinkingSignatureCache(t *testing.T) {
	// Test basic cache operations
	cache := NewThinkingSignatureCache(time.Hour)

	// Test Store and Get
	key := "test_key"
	signature := "test_signature_value"

	cache.Store(key, signature)

	result := cache.Get(key)
	require.NotNil(t, result, "Expected signature to be found, got nil")
	require.Equal(t, signature, *result)

	// Test non-existent key
	nonExistentResult := cache.Get("non_existent_key")
	require.Nil(t, nonExistentResult, "Expected nil for non-existent key")

	// Test Delete
	cache.Delete(key)
	deletedResult := cache.Get(key)
	require.Nil(t, deletedResult, "Expected nil after deletion")
}

func TestSignatureCacheTTL(t *testing.T) {
	// Test TTL functionality
	cache := NewThinkingSignatureCache(100 * time.Millisecond)

	key := "ttl_test_key"
	signature := "ttl_test_signature"

	cache.Store(key, signature)

	// Should be available immediately
	result := cache.Get(key)
	require.NotNil(t, result, "Signature should be available immediately after storage")
	require.Equal(t, signature, *result)

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Should be expired now
	expiredResult := cache.Get(key)
	require.Nil(t, expiredResult, "Signature should be expired after TTL")
}

func TestGenerateSignatureKey(t *testing.T) {
	tokenID := "token_123"
	conversationID := "conv_abc"
	messageIndex := 5
	thinkingIndex := 2

	expected := "thinking_sig:token_123:conv_abc:5:2"
	result := generateSignatureKey(tokenID, conversationID, messageIndex, thinkingIndex)

	require.Equal(t, expected, result)
}

func TestGenerateConversationID(t *testing.T) {
	// Test with empty messages
	emptyMessages := []model.Message{}
	emptyResult := generateConversationID(emptyMessages)
	require.NotEmpty(t, emptyResult, "Conversation ID should not be empty for empty messages")

	// Test with user messages
	messages := []model.Message{
		{Role: "user", Content: "Hello, how are you?"},
		{Role: "assistant", Content: "I'm doing well, thank you!"},
		{Role: "user", Content: "What's the weather like?"},
	}

	result1 := generateConversationID(messages)
	require.NotEmpty(t, result1, "Conversation ID should not be empty")

	// Same messages should produce same ID
	result2 := generateConversationID(messages)
	require.Equal(t, result1, result2, "Same messages should produce same conversation ID")

	// Different messages should produce different ID
	differentMessages := []model.Message{
		{Role: "user", Content: "Different message"},
	}
	result3 := generateConversationID(differentMessages)
	require.NotEqual(t, result1, result3, "Different messages should produce different conversation IDs")
}

func TestTruncateForHash(t *testing.T) {
	// Test normal case
	content := "This is a test message"
	result := truncateForHash(content, 10)
	expected := "This is a "
	require.Equal(t, expected, result)

	// Test case where content is shorter than maxLen
	shortContent := "Short"
	result2 := truncateForHash(shortContent, 10)
	require.Equal(t, shortContent, result2)

	// Test empty content
	emptyResult := truncateForHash("", 10)
	require.Empty(t, emptyResult, "Expected empty string for empty input")
}

func TestGetTokenIDFromRequest(t *testing.T) {
	tokenID := 12345
	expected := "token_12345"
	result := getTokenIDFromRequest(tokenID)

	require.Equal(t, expected, result)
}

func TestCacheSize(t *testing.T) {
	cache := NewThinkingSignatureCache(time.Hour)

	// Initially empty
	require.Equal(t, 0, cache.Size(), "Cache should be empty initially")

	// Add some entries
	cache.Store("key1", "sig1")
	cache.Store("key2", "sig2")
	cache.Store("key3", "sig3")

	require.Equal(t, 3, cache.Size())

	// Delete one entry
	cache.Delete("key2")

	require.Equal(t, 2, cache.Size())
}

func TestCacheCleanup(t *testing.T) {
	// Test that cleanup removes expired entries
	cache := NewThinkingSignatureCache(50 * time.Millisecond)

	// Add some entries
	cache.Store("key1", "sig1")
	cache.Store("key2", "sig2")

	// Verify they exist
	require.Equal(t, 2, cache.Size(), "Expected 2 entries before expiration")

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Manually trigger cleanup
	cache.cleanupExpired()

	// Should be empty now
	require.Equal(t, 0, cache.Size())
}

// Mock backend for testing
type mockBackend struct {
	data map[string]string
	fail bool
}

func (m *mockBackend) Store(key, signature string, ttl time.Duration) error {
	if m.fail {
		return &mockError{}
	}
	if m.data == nil {
		m.data = make(map[string]string)
	}
	m.data[key] = signature
	return nil
}

func (m *mockBackend) Get(key string) (*string, error) {
	if m.fail {
		return nil, &mockError{}
	}
	if value, exists := m.data[key]; exists {
		return &value, nil
	}
	return nil, nil
}

func (m *mockBackend) Delete(key string) error {
	if m.fail {
		return &mockError{}
	}
	delete(m.data, key)
	return nil
}

type mockError struct{}

func (e *mockError) Error() string {
	return "mock error"
}

func TestCacheWithBackend(t *testing.T) {
	cache := NewThinkingSignatureCache(time.Hour)
	backend := &mockBackend{}
	cache.SetBackend(backend)

	// Test Store and Get with backend
	key := "backend_test_key"
	signature := "backend_test_signature"

	cache.Store(key, signature)

	// Should retrieve from backend
	result := cache.Get(key)
	require.NotNil(t, result, "Should retrieve signature from backend")
	require.Equal(t, signature, *result)

	// Verify it's actually in the backend
	backendValue, exists := backend.data[key]
	require.True(t, exists, "Signature should exist in backend")
	require.Equal(t, signature, backendValue, "Signature should be stored in backend")
}

func TestCacheBackendFallback(t *testing.T) {
	cache := NewThinkingSignatureCache(time.Hour)
	backend := &mockBackend{fail: true} // Backend that always fails
	cache.SetBackend(backend)

	// Test Store fallback to in-memory
	key := "fallback_test_key"
	signature := "fallback_test_signature"

	cache.Store(key, signature)

	// Should fallback to in-memory cache
	result := cache.Get(key)
	require.NotNil(t, result, "Should fallback to in-memory cache when backend fails")
	require.Equal(t, signature, *result)
}
