package meta

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/model"
)

func TestGetByContext_ChannelRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a test context
	c, _ := gin.CreateTestContext(nil)
	c.Request = &http.Request{
		URL:    &url.URL{Path: "/v1/chat/completions"},
		Header: make(http.Header),
	}
	c.Request.Header.Set("Authorization", "Bearer test-key")

	// Set initial channel information
	c.Set(ctxkey.Channel, 1)
	c.Set(ctxkey.ChannelId, 100)
	c.Set(ctxkey.TokenId, 1)
	c.Set(ctxkey.TokenName, "test-token")
	c.Set(ctxkey.Id, 1)
	c.Set(ctxkey.Group, "default")
	c.Set(ctxkey.RequestModel, "gpt-3.5-turbo")
	c.Set(ctxkey.BaseURL, "https://api.openai.com")
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.SystemPrompt, "")
	c.Set(ctxkey.ModelMapping, map[string]string{})
	c.Set(ctxkey.Config, model.ChannelConfig{})

	// First call - should create new meta
	meta1 := GetByContext(c)
	assert.Equal(t, 100, meta1.ChannelId)
	assert.Equal(t, 1, meta1.ChannelType)
	assert.Equal(t, "https://api.openai.com", meta1.BaseURL)
	assert.Equal(t, 1.0, meta1.ChannelRatio)

	// Second call with same channel - should return cached meta
	meta2 := GetByContext(c)
	assert.Same(t, meta1, meta2) // Should be the same object
	assert.Equal(t, 100, meta2.ChannelId)

	// Simulate channel change during retry (like what happens in controller/relay.go)
	c.Set(ctxkey.Channel, 2)
	c.Set(ctxkey.ChannelId, 200)
	c.Set(ctxkey.BaseURL, "https://api.anthropic.com")
	c.Set(ctxkey.ChannelRatio, 2.0)
	c.Set(ctxkey.Config, model.ChannelConfig{APIVersion: "2023-06-01"})

	// Third call with different channel - should update cached meta
	meta3 := GetByContext(c)
	assert.Same(t, meta1, meta3)          // Should still be the same object (updated in place)
	assert.Equal(t, 200, meta3.ChannelId) // But with updated channel info
	assert.Equal(t, 2, meta3.ChannelType)
	assert.Equal(t, "https://api.anthropic.com", meta3.BaseURL)
	assert.Equal(t, 2.0, meta3.ChannelRatio)
	assert.Equal(t, "2023-06-01", meta3.Config.APIVersion)

	// Verify that other fields remain unchanged
	assert.Equal(t, "test-token", meta3.TokenName)
	assert.Equal(t, 1, meta3.UserId)
	assert.Equal(t, "default", meta3.Group)
	assert.Equal(t, "gpt-3.5-turbo", meta3.OriginModelName)
}

func TestGetByContext_NoChannelChange(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a test context
	c, _ := gin.CreateTestContext(nil)
	c.Request = &http.Request{
		URL:    &url.URL{Path: "/v1/chat/completions"},
		Header: make(http.Header),
	}
	c.Request.Header.Set("Authorization", "Bearer test-key")

	// Set channel information
	c.Set(ctxkey.Channel, 1)
	c.Set(ctxkey.ChannelId, 100)
	c.Set(ctxkey.TokenId, 1)
	c.Set(ctxkey.TokenName, "test-token")
	c.Set(ctxkey.Id, 1)
	c.Set(ctxkey.Group, "default")
	c.Set(ctxkey.RequestModel, "gpt-3.5-turbo")
	c.Set(ctxkey.BaseURL, "https://api.openai.com")
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.SystemPrompt, "")
	c.Set(ctxkey.ModelMapping, map[string]string{})
	c.Set(ctxkey.Config, model.ChannelConfig{})

	// First call
	meta1 := GetByContext(c)
	originalChannelId := meta1.ChannelId

	// Change some other context values but keep channel ID the same
	c.Set(ctxkey.BaseURL, "https://different.url.com")
	c.Set(ctxkey.ChannelRatio, 5.0)

	// Second call - should return cached meta without updates since channel ID didn't change
	meta2 := GetByContext(c)
	assert.Same(t, meta1, meta2)
	assert.Equal(t, originalChannelId, meta2.ChannelId)
	// These should NOT be updated since channel ID didn't change
	assert.Equal(t, "https://api.openai.com", meta2.BaseURL)
	assert.Equal(t, 1.0, meta2.ChannelRatio)
}

func TestEnsureActualModelName(t *testing.T) {
	meta := &Meta{
		ModelMapping: map[string]string{"alias": "mapped"},
	}

	meta.EnsureActualModelName("alias")

	require.Equal(t, "alias", meta.OriginModelName, "expected OriginModelName to be backfilled")
	require.Equal(t, "mapped", meta.ActualModelName, "expected ActualModelName to be mapped value")

	// Calling again with blank fallback should keep existing values
	meta.EnsureActualModelName("")
	require.Equal(t, "mapped", meta.ActualModelName, "expected ActualModelName to remain unchanged")
}
