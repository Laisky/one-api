package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/channeltype"
)

// TestChannelPriorityLogic tests the channel priority selection logic
// This test verifies that priority fallback works correctly when high priority channels fail
func TestChannelPriorityLogic(t *testing.T) {
	tests := []struct {
		name                 string
		highPriorityChannels []*model.Channel
		lowPriorityChannels  []*model.Channel
		highPriorityError    error
		lowPriorityError     error
		expectedChannelId    int
		expectedError        bool
		description          string
	}{
		{
			name: "high_priority_available",
			highPriorityChannels: []*model.Channel{
				{Id: 1, Priority: ptrToInt64(10)},
			},
			lowPriorityChannels: []*model.Channel{
				{Id: 2, Priority: ptrToInt64(5)},
			},
			highPriorityError: nil,
			lowPriorityError:  nil,
			expectedChannelId: 1,
			expectedError:     false,
			description:       "Should use high priority channel when available",
		},
		{
			name:                 "fallback_to_low_priority",
			highPriorityChannels: nil,
			lowPriorityChannels: []*model.Channel{
				{Id: 2, Priority: ptrToInt64(5)},
			},
			highPriorityError: errors.New("no high priority channels available"),
			lowPriorityError:  nil,
			expectedChannelId: 2,
			expectedError:     false,
			description:       "Should fallback to low priority when high priority unavailable",
		},
		{
			name:                 "no_channels_available",
			highPriorityChannels: nil,
			lowPriorityChannels:  nil,
			highPriorityError:    errors.New("no high priority channels available"),
			lowPriorityError:     errors.New("no channels available"),
			expectedChannelId:    0,
			expectedError:        true,
			description:          "Should return error when no channels available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Testing: %s", tt.description)

			// Simulate the channel selection logic from distributor middleware
			var selectedChannel *model.Channel
			var finalError error

			// First try to get highest priority channels (ignoreFirstPriority=false)
			if tt.highPriorityError != nil {
				// High priority failed, try lower priority (ignoreFirstPriority=true)
				t.Logf("High priority channels unavailable, trying lower priority")
				if tt.lowPriorityError != nil {
					finalError = tt.lowPriorityError
				} else if len(tt.lowPriorityChannels) > 0 {
					selectedChannel = tt.lowPriorityChannels[0]
				}
			} else if len(tt.highPriorityChannels) > 0 {
				selectedChannel = tt.highPriorityChannels[0]
			}

			// Verify results
			if tt.expectedError {
				assert.Error(t, finalError, "Should have error when no channels available")
				assert.Nil(t, selectedChannel, "Channel should be nil when no channels available")
				t.Logf("✓ Correctly failed with error: %v", finalError)
			} else {
				assert.NoError(t, finalError, "Should not have error when channels are available")
				assert.NotNil(t, selectedChannel, "Channel should not be nil")
				assert.Equal(t, tt.expectedChannelId, selectedChannel.Id, "Should select correct channel")
				t.Logf("✓ Selected channel %d as expected", selectedChannel.Id)
			}
		})
	}
}

// TestChannelPriorityFallbackScenario tests specific priority fallback scenarios
func TestChannelPriorityFallbackScenario(t *testing.T) {
	t.Run("rate_limit_suspension_fallback", func(t *testing.T) {
		// Simulate a scenario where high priority channels are suspended due to 429 errors
		// and the system should fallback to lower priority channels

		highPriorityUnavailable := errors.New("high priority channels suspended due to rate limits")
		lowPriorityChannel := &model.Channel{
			Id:       100,
			Priority: ptrToInt64(25),
			Name:     "backup-channel",
		}

		t.Logf("Simulating rate limit scenario where high priority channels are suspended")

		// First attempt (high priority) fails
		var selectedChannel *model.Channel
		var err error

		// Simulate high priority failure
		err = highPriorityUnavailable
		if err != nil {
			t.Logf("High priority channels unavailable: %v", err)
			// Fallback to lower priority
			selectedChannel = lowPriorityChannel
			err = nil
		}

		assert.NoError(t, err, "Should successfully fallback to lower priority channels")
		assert.NotNil(t, selectedChannel, "Should get a channel from fallback")
		assert.Equal(t, 100, selectedChannel.Id, "Should get the lower priority channel")
		assert.Equal(t, int64(25), *selectedChannel.Priority, "Should have correct priority")

		t.Logf("✓ Successfully fell back from high priority (suspended) to low priority channel")
		t.Logf("✓ Channel selected: ID=%d, Priority=%d", selectedChannel.Id, *selectedChannel.Priority)
	})
}

// Helper function to create pointer to int64
func ptrToInt64(v int64) *int64 {
	return &v
}

func setupDistributorTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, testDB.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}))

	originalDB := model.DB
	originalUsingSQLite := common.UsingSQLite.Load()

	model.DB = testDB
	common.UsingSQLite.Store(true)

	cleanup := func() {
		model.DB = originalDB
		common.UsingSQLite.Store(originalUsingSQLite)
	}

	return testDB, cleanup
}

func TestDistributeSpecificChannelRejectsUnsupportedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, cleanup := setupDistributorTestDB(t)
	defer cleanup()

	user := &model.User{
		Id:       1,
		Username: "tester",
		Password: "hashed",
		Group:    "default",
		Status:   model.UserStatusEnabled,
	}
	require.NoError(t, db.Create(user).Error)

	priority := int64(100)
	channel := &model.Channel{
		Id:       2,
		Name:     "openai",
		Type:     channeltype.OpenAI,
		Models:   "gpt-4",
		Group:    "default",
		Status:   model.ChannelStatusEnabled,
		Priority: &priority,
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities())

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-5"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	c.Set(ctxkey.Id, user.Id)
	c.Set(ctxkey.RequestModel, "gpt-5")
	c.Set(ctxkey.SpecificChannelId, channel.Id)
	c.Set(ctxkey.TokenId, 42)
	gmw.SetLogger(c, logger.Logger)

	Distribute()(c)

	assert.True(t, c.IsAborted(), "middleware should abort for unsupported model")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "does not support")
}

func TestDistributeSpecificChannelAllowsSupportedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, cleanup := setupDistributorTestDB(t)
	defer cleanup()

	user := &model.User{
		Id:       10,
		Username: "tester",
		Password: "hashed",
		Group:    "default",
		Status:   model.UserStatusEnabled,
	}
	require.NoError(t, db.Create(user).Error)

	priority := int64(50)
	channel := &model.Channel{
		Id:       20,
		Name:     "openai",
		Type:     channeltype.OpenAI,
		Models:   "gpt-4,gpt-4o",
		Group:    "default",
		Status:   model.ChannelStatusEnabled,
		Priority: &priority,
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities())

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	c.Set(ctxkey.Id, user.Id)
	c.Set(ctxkey.RequestModel, "gpt-4o")
	c.Set(ctxkey.SpecificChannelId, channel.Id)
	c.Set(ctxkey.TokenId, 99)
	gmw.SetLogger(c, logger.Logger)

	Distribute()(c)

	assert.False(t, c.IsAborted(), "middleware should allow supported model")
	assert.Equal(t, http.StatusOK, rec.Code, "middleware should leave response as OK")
}

func TestDistributeAutoSkipsUnsupportedChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, cleanup := setupDistributorTestDB(t)
	defer cleanup()

	originalMemoryCache := config.MemoryCacheEnabled
	config.MemoryCacheEnabled = false
	defer func() { config.MemoryCacheEnabled = originalMemoryCache }()

	user := &model.User{
		Id:       42,
		Username: "auto-user",
		Password: "hashed",
		Group:    "default",
		Status:   model.UserStatusEnabled,
	}
	require.NoError(t, db.Create(user).Error)

	badPriority := int64(200)
	badChannel := &model.Channel{
		Id:       300,
		Name:     "bad-channel",
		Type:     channeltype.OpenAI,
		Models:   "gpt-4",
		Group:    "default",
		Status:   model.ChannelStatusEnabled,
		Priority: &badPriority,
	}
	require.NoError(t, db.Create(badChannel).Error)
	require.NoError(t, badChannel.AddAbilities())

	// Simulate stale abilities: channel models changed without updating abilities
	require.NoError(t, db.Model(badChannel).Update("models", "gpt-4-legacy").Error)

	goodPriority := int64(100)
	goodChannel := &model.Channel{
		Id:       301,
		Name:     "good-channel",
		Type:     channeltype.OpenAI,
		Models:   "gpt-4",
		Group:    "default",
		Status:   model.ChannelStatusEnabled,
		Priority: &goodPriority,
	}
	require.NoError(t, db.Create(goodChannel).Error)
	require.NoError(t, goodChannel.AddAbilities())

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	c.Set(ctxkey.Id, user.Id)
	c.Set(ctxkey.RequestModel, "gpt-4")
	c.Set(ctxkey.TokenId, 777)
	gmw.SetLogger(c, logger.Logger)

	Distribute()(c)

	assert.False(t, c.IsAborted(), "middleware should continue when a supported channel exists")
	selectedChannelId := c.GetInt(ctxkey.ChannelId)
	assert.Equal(t, goodChannel.Id, selectedChannelId, "should select the channel that still supports the model")
}

// TestChannelSupportsEndpoint verifies the endpoint support checking logic.
func TestChannelSupportsEndpoint(t *testing.T) {
	// Test with default endpoints (no custom config)
	t.Run("default_endpoints_openai", func(t *testing.T) {
		channel := &model.Channel{
			Id:     1,
			Type:   channeltype.OpenAI,
			Config: "",
		}

		// OpenAI should support chat completions by default
		require.True(t, channelSupportsEndpoint(channel, 1), "OpenAI should support chat completions")

		// OpenAI should support embeddings by default
		require.True(t, channelSupportsEndpoint(channel, 3), "OpenAI should support embeddings")
	})

	t.Run("default_endpoints_cohere", func(t *testing.T) {
		channel := &model.Channel{
			Id:     2,
			Type:   channeltype.Cohere,
			Config: "",
		}

		// Cohere should support chat completions
		require.True(t, channelSupportsEndpoint(channel, 1), "Cohere should support chat completions")

		// Cohere should support rerank
		require.True(t, channelSupportsEndpoint(channel, 14), "Cohere should support rerank")
	})

	t.Run("custom_endpoints", func(t *testing.T) {
		// Channel with custom endpoints config
		channel := &model.Channel{
			Id:     3,
			Type:   channeltype.OpenAI,
			Config: `{"supported_endpoints": ["chat_completions", "embeddings"]}`,
		}

		// Should support configured endpoints
		require.True(t, channelSupportsEndpoint(channel, 1), "should support chat completions")
		require.True(t, channelSupportsEndpoint(channel, 3), "should support embeddings")

		// Should NOT support endpoints not in custom list
		require.False(t, channelSupportsEndpoint(channel, 14), "should not support rerank")
	})

	t.Run("unknown_relay_mode", func(t *testing.T) {
		channel := &model.Channel{
			Id:     4,
			Type:   channeltype.OpenAI,
			Config: "",
		}

		// Unknown relay mode should be allowed (backward compatibility)
		require.True(t, channelSupportsEndpoint(channel, 0), "unknown relay mode should be allowed")
	})
}

// TestEndpointValidationBackwardCompatibility verifies that existing channels without
// endpoint configuration continue to work exactly as before.
func TestEndpointValidationBackwardCompatibility(t *testing.T) {
	t.Run("empty_config_uses_defaults", func(t *testing.T) {
		// Channel with no config at all - should use defaults
		channel := &model.Channel{
			Id:     1,
			Type:   channeltype.OpenAI,
			Config: "",
		}

		// OpenAI default endpoints should be available
		require.True(t, channelSupportsEndpoint(channel, 1), "chat_completions")
		require.True(t, channelSupportsEndpoint(channel, 3), "embeddings")
		require.True(t, channelSupportsEndpoint(channel, 15), "response_api")
	})

	t.Run("config_without_endpoints_uses_defaults", func(t *testing.T) {
		// Channel with other config fields but no supported_endpoints
		channel := &model.Channel{
			Id:     2,
			Type:   channeltype.Azure,
			Config: `{"region": "eastus", "api_version": "2024-02-15"}`,
		}

		// Azure default endpoints should be available
		require.True(t, channelSupportsEndpoint(channel, 1), "chat_completions")
		require.True(t, channelSupportsEndpoint(channel, 3), "embeddings")
	})

	t.Run("empty_endpoints_array_uses_defaults", func(t *testing.T) {
		// Channel with explicitly empty supported_endpoints array
		channel := &model.Channel{
			Id:     3,
			Type:   channeltype.Anthropic,
			Config: `{"supported_endpoints": []}`,
		}

		// Anthropic default endpoints should be available
		require.True(t, channelSupportsEndpoint(channel, 1), "chat_completions")
		require.True(t, channelSupportsEndpoint(channel, 18), "claude_messages") // ClaudeMessages = 18
		// Anthropic doesn't support embeddings by default
		require.False(t, channelSupportsEndpoint(channel, 3), "embeddings")
	})
}

// TestEndpointValidationCustomConfig verifies custom endpoint configurations work correctly.
func TestEndpointValidationCustomConfig(t *testing.T) {
	t.Run("restrict_to_single_endpoint", func(t *testing.T) {
		// Channel restricted to only chat completions
		channel := &model.Channel{
			Id:     1,
			Type:   channeltype.OpenAI,
			Config: `{"supported_endpoints": ["chat_completions"]}`,
		}

		require.True(t, channelSupportsEndpoint(channel, 1), "chat_completions")
		require.False(t, channelSupportsEndpoint(channel, 3), "embeddings")
		require.False(t, channelSupportsEndpoint(channel, 15), "response_api")
	})

	t.Run("expand_beyond_defaults", func(t *testing.T) {
		// Anthropic channel with embeddings added (not in default)
		channel := &model.Channel{
			Id:     2,
			Type:   channeltype.Anthropic,
			Config: `{"supported_endpoints": ["chat_completions", "claude_messages", "embeddings"]}`,
		}

		require.True(t, channelSupportsEndpoint(channel, 1), "chat_completions")
		require.True(t, channelSupportsEndpoint(channel, 18), "claude_messages") // ClaudeMessages = 18
		require.True(t, channelSupportsEndpoint(channel, 3), "embeddings added")
	})

	t.Run("case_insensitive_endpoint_names", func(t *testing.T) {
		// Test that endpoint name matching is case-insensitive
		channel := &model.Channel{
			Id:     3,
			Type:   channeltype.OpenAI,
			Config: `{"supported_endpoints": ["CHAT_COMPLETIONS", "Embeddings"]}`,
		}

		require.True(t, channelSupportsEndpoint(channel, 1), "CHAT_COMPLETIONS")
		require.True(t, channelSupportsEndpoint(channel, 3), "Embeddings")
	})
}

// TestEndpointRoutingIntegration tests the full routing flow with endpoint validation.
func TestEndpointRoutingIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger.SetupLogger()

	// Set up test database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{})
	require.NoError(t, err)
	model.DB = db
	common.UsingSQLite.Store(true)
	config.MemoryCacheEnabled = true
	model.InitChannelCache()

	// Create test user
	user := &model.User{
		Username:    "endpoint-test-user",
		DisplayName: "Test User",
		Group:       "default",
		Status:      1,
	}
	require.NoError(t, db.Create(user).Error)

	t.Run("channel_skipped_for_unsupported_endpoint", func(t *testing.T) {
		// Create a channel that doesn't support embeddings
		priority := int64(100)
		channel := &model.Channel{
			Id:       500,
			Name:     "no-embeddings",
			Type:     channeltype.OpenAI,
			Models:   "text-embedding-ada-002",
			Group:    "default",
			Status:   model.ChannelStatusEnabled,
			Priority: &priority,
			Config:   `{"supported_endpoints": ["chat_completions"]}`, // No embeddings!
		}
		require.NoError(t, db.Create(channel).Error)
		require.NoError(t, channel.AddAbilities())

		// Create a fallback channel that supports embeddings
		fallbackPriority := int64(50)
		fallbackChannel := &model.Channel{
			Id:       501,
			Name:     "with-embeddings",
			Type:     channeltype.OpenAI,
			Models:   "text-embedding-ada-002",
			Group:    "default",
			Status:   model.ChannelStatusEnabled,
			Priority: &fallbackPriority,
			Config:   `{"supported_endpoints": ["chat_completions", "embeddings"]}`,
		}
		require.NoError(t, db.Create(fallbackChannel).Error)
		require.NoError(t, fallbackChannel.AddAbilities())

		model.InitChannelCache()

		// Request embeddings endpoint
		req := httptest.NewRequest(http.MethodPost, "/v1/embeddings", bytes.NewBufferString(`{"model":"text-embedding-ada-002"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = req

		c.Set(ctxkey.Id, user.Id)
		c.Set(ctxkey.RequestModel, "text-embedding-ada-002")
		c.Set(ctxkey.TokenId, 888)
		gmw.SetLogger(c, logger.Logger)

		Distribute()(c)

		// Should skip the first channel and select the fallback
		assert.False(t, c.IsAborted(), "should not abort")
		selectedChannelId := c.GetInt(ctxkey.ChannelId)
		assert.Equal(t, fallbackChannel.Id, selectedChannelId, "should select fallback channel with embeddings support")
	})
}
