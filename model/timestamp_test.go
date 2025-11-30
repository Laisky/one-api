package model

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCreatedAtUpdatedAtFields tests that all models have CreatedAt and UpdatedAt fields
// that are properly set when creating and updating records
func TestCreatedAtUpdatedAtFields(t *testing.T) {
	setupTestDatabase(t)

	t.Run("User timestamps", func(t *testing.T) {
		now := time.Now().UnixNano()
		nowStr := strconv.FormatInt(now, 10)
		user := User{
			Username:    "testuser_" + time.Now().Format("150405") + "_" + nowStr,
			Password:    "testpassword123",
			AccessToken: "testtoken_" + time.Now().Format("150405") + "_" + nowStr,
			AffCode:     "testaffcode_" + nowStr,
		}

		// Test creation
		before := time.Now().UnixMilli()
		err := DB.Create(&user).Error
		require.NoError(t, err)
		after := time.Now().UnixMilli()

		assert.True(t, user.CreatedAt >= before && user.CreatedAt <= after, "CreatedAt should be set automatically")
		assert.True(t, user.UpdatedAt >= before && user.UpdatedAt <= after, "UpdatedAt should be set automatically on creation")
		originalCreatedAt := user.CreatedAt
		originalUpdatedAt := user.UpdatedAt

		// Test update
		time.Sleep(10 * time.Millisecond) // Ensure some time passes
		before = time.Now().UnixMilli()
		user.Username = "updateduser_" + time.Now().Format("150405") + "_" + nowStr
		err = DB.Save(&user).Error
		require.NoError(t, err)
		after = time.Now().UnixMilli()

		assert.Equal(t, originalCreatedAt, user.CreatedAt, "CreatedAt should not change on update")
		assert.True(t, user.UpdatedAt >= before && user.UpdatedAt <= after, "UpdatedAt should be updated automatically")
		assert.True(t, user.UpdatedAt > originalUpdatedAt, "UpdatedAt should be newer after update")
	})

	t.Run("Token timestamps", func(t *testing.T) {
		token := Token{
			UserId: 1,
			Key:    "test-token-key-123456789012345678901234567890",
			Name:   "test-token",
		}

		// Test creation
		before := time.Now().UnixMilli()
		err := DB.Create(&token).Error
		require.NoError(t, err)
		after := time.Now().UnixMilli()

		assert.True(t, token.CreatedAt >= before && token.CreatedAt <= after, "CreatedAt should be set automatically")
		assert.True(t, token.UpdatedAt >= before && token.UpdatedAt <= after, "UpdatedAt should be set automatically on creation")
	})

	t.Run("Channel timestamps", func(t *testing.T) {
		channel := Channel{
			Name: "test-channel",
			Type: 1,
			Key:  "test-key",
		}

		// Test creation
		before := time.Now().UnixMilli()
		err := DB.Create(&channel).Error
		require.NoError(t, err)
		after := time.Now().UnixMilli()

		assert.True(t, channel.CreatedAt >= before && channel.CreatedAt <= after, "CreatedAt should be set automatically")
		assert.True(t, channel.UpdatedAt >= before && channel.UpdatedAt <= after, "UpdatedAt should be set automatically on creation")
	})

	t.Run("Redemption timestamps", func(t *testing.T) {
		redemption := Redemption{
			UserId: 1,
			Key:    "test-redemption-key-123456789012",
			Name:   "test-redemption",
			Quota:  100,
		}

		// Test creation
		before := time.Now().UnixMilli()
		err := DB.Create(&redemption).Error
		require.NoError(t, err)
		after := time.Now().UnixMilli()

		assert.True(t, redemption.CreatedAt >= before && redemption.CreatedAt <= after, "CreatedAt should be set automatically")
		assert.True(t, redemption.UpdatedAt >= before && redemption.UpdatedAt <= after, "UpdatedAt should be set automatically on creation")
	})

	t.Run("Option timestamps", func(t *testing.T) {
		option := Option{
			Key:   "test-option-key",
			Value: "test-value",
		}

		// Test creation
		before := time.Now().UnixMilli()
		err := DB.Create(&option).Error
		require.NoError(t, err)
		after := time.Now().UnixMilli()

		assert.True(t, option.CreatedAt >= before && option.CreatedAt <= after, "CreatedAt should be set automatically")
		assert.True(t, option.UpdatedAt >= before && option.UpdatedAt <= after, "UpdatedAt should be set automatically on creation")
	})

	t.Run("UserRequestCost timestamps", func(t *testing.T) {
		cost := UserRequestCost{
			UserID:    1,
			RequestID: "test-request-123",
			Quota:     50,
		}

		// Test creation
		before := time.Now().UnixMilli()
		err := DB.Create(&cost).Error
		require.NoError(t, err)
		after := time.Now().UnixMilli()

		assert.True(t, cost.CreatedAt >= before && cost.CreatedAt <= after, "CreatedAt should be set automatically")
		assert.True(t, cost.UpdatedAt >= before && cost.UpdatedAt <= after, "UpdatedAt should be set automatically on creation")
	})

	t.Run("Log timestamps", func(t *testing.T) {
		log := Log{
			UserId:    1,
			CreatedAt: time.Now().UnixMilli(),
			Type:      1,
			Content:   "test log",
		}

		// Test creation
		before := time.Now().UnixMilli()
		err := DB.Create(&log).Error
		require.NoError(t, err)
		after := time.Now().UnixMilli()

		// Log has its own CreatedAt logic, but UpdatedAt should still work
		assert.True(t, log.UpdatedAt >= before && log.UpdatedAt <= after, "UpdatedAt should be set automatically on creation")
	})

	t.Run("Ability timestamps", func(t *testing.T) {
		now := time.Now().UnixNano()
		nowStr := strconv.FormatInt(now, 10)
		ability := Ability{
			Group:     "test-group-" + time.Now().Format("150405") + "-" + nowStr,
			Model:     "test-model-" + time.Now().Format("150405") + "-" + nowStr,
			ChannelId: 1,
			Enabled:   true,
		}

		// Test creation
		before := time.Now().UnixMilli()
		err := DB.Create(&ability).Error
		require.NoError(t, err)
		after := time.Now().UnixMilli()

		assert.True(t, ability.CreatedAt >= before && ability.CreatedAt <= after, "CreatedAt should be set automatically")
		assert.True(t, ability.UpdatedAt >= before && ability.UpdatedAt <= after, "UpdatedAt should be set automatically on creation")
	})
}

// setupTestDatabase ensures a clean test database is available for testing
func setupTestDatabase(t *testing.T) {
	if DB == nil {
		// Initialize primary and log databases for tests
		InitDB()
		InitLogDB()
	}
	require.NotNil(t, DB, "Database connection not available for testing after InitDB")

	// Clean up test data
	DB.Exec("DELETE FROM users WHERE username LIKE 'test%' OR access_token LIKE 'test%'")
	DB.Exec("DELETE FROM tokens WHERE name LIKE 'test%'")
	DB.Exec("DELETE FROM token_transactions WHERE transaction_id LIKE 'test%'")
	DB.Exec("DELETE FROM channels WHERE name LIKE 'test%'")
	DB.Exec("DELETE FROM redemptions WHERE name LIKE 'test%'")
	DB.Exec("DELETE FROM options WHERE key LIKE 'test%'")
	DB.Exec("DELETE FROM user_request_costs WHERE request_id LIKE 'test%'")
	DB.Exec("DELETE FROM logs WHERE content LIKE 'test%'")
	DB.Exec("DELETE FROM abilities WHERE `group` LIKE 'test%' OR model LIKE 'test%'")
}
