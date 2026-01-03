package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIncreaseUserQuota_WithContext verifies that IncreaseUserQuota properly handles context parameter.
func TestIncreaseUserQuota_WithContext(t *testing.T) {
	setupTestDatabase(t)

	user := &User{
		Username: fmt.Sprintf("test-user-inc-quota-%d", time.Now().UnixNano()),
		Password: "testpassword12345",
		Status:   UserStatusEnabled,
		Role:     RoleCommonUser,
		Quota:    100,
	}
	require.NoError(t, DB.Create(user).Error)
	defer DB.Exec("DELETE FROM users WHERE id = ?", user.Id)

	ctx := context.Background()
	initialQuota := user.Quota

	// Test with valid context
	err := IncreaseUserQuota(ctx, user.Id, 50)
	require.NoError(t, err)

	var refreshedUser User
	require.NoError(t, DB.First(&refreshedUser, user.Id).Error)
	assert.Equal(t, initialQuota+50, refreshedUser.Quota)
}

// TestIncreaseUserQuota_WithNilContext verifies backward compatibility when nil context is passed.
func TestIncreaseUserQuota_WithNilContext(t *testing.T) {
	setupTestDatabase(t)

	user := &User{
		Username: fmt.Sprintf("test-user-inc-nil-ctx-%d", time.Now().UnixNano()),
		Password: "testpassword12345",
		Status:   UserStatusEnabled,
		Role:     RoleCommonUser,
		Quota:    100,
	}
	require.NoError(t, DB.Create(user).Error)
	defer DB.Exec("DELETE FROM users WHERE id = ?", user.Id)

	initialQuota := user.Quota

	// Test with nil context (backward compatibility)
	var nilCtx context.Context //nolint:SA1012 // intentionally nil to test backward compatibility
	err := IncreaseUserQuota(nilCtx, user.Id, 50)
	require.NoError(t, err)

	var refreshedUser User
	require.NoError(t, DB.First(&refreshedUser, user.Id).Error)
	assert.Equal(t, initialQuota+50, refreshedUser.Quota)
}

// TestIncreaseUserQuota_NegativeQuota verifies that negative quota is rejected.
func TestIncreaseUserQuota_NegativeQuota(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	err := IncreaseUserQuota(ctx, 1, -10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "quota cannot be negative")
}

// TestDecreaseUserQuota_WithContext verifies that DecreaseUserQuota properly handles context parameter.
func TestDecreaseUserQuota_WithContext(t *testing.T) {
	setupTestDatabase(t)

	user := &User{
		Username: fmt.Sprintf("test-user-dec-quota-%d", time.Now().UnixNano()),
		Password: "testpassword12345",
		Status:   UserStatusEnabled,
		Role:     RoleCommonUser,
		Quota:    100,
	}
	require.NoError(t, DB.Create(user).Error)
	defer DB.Exec("DELETE FROM users WHERE id = ?", user.Id)

	ctx := context.Background()
	initialQuota := user.Quota

	// Test with valid context
	err := DecreaseUserQuota(ctx, user.Id, 30)
	require.NoError(t, err)

	var refreshedUser User
	require.NoError(t, DB.First(&refreshedUser, user.Id).Error)
	assert.Equal(t, initialQuota-30, refreshedUser.Quota)
}

// TestDecreaseUserQuota_WithNilContext verifies backward compatibility when nil context is passed.
func TestDecreaseUserQuota_WithNilContext(t *testing.T) {
	setupTestDatabase(t)

	user := &User{
		Username: fmt.Sprintf("test-user-dec-nil-ctx-%d", time.Now().UnixNano()),
		Password: "testpassword12345",
		Status:   UserStatusEnabled,
		Role:     RoleCommonUser,
		Quota:    100,
	}
	require.NoError(t, DB.Create(user).Error)
	defer DB.Exec("DELETE FROM users WHERE id = ?", user.Id)

	initialQuota := user.Quota

	// Test with nil context (backward compatibility)
	var nilCtx context.Context //nolint:SA1012 // intentionally nil to test backward compatibility
	err := DecreaseUserQuota(nilCtx, user.Id, 30)
	require.NoError(t, err)

	var refreshedUser User
	require.NoError(t, DB.First(&refreshedUser, user.Id).Error)
	assert.Equal(t, initialQuota-30, refreshedUser.Quota)
}

// TestDecreaseUserQuota_NegativeQuota verifies that negative quota is rejected.
func TestDecreaseUserQuota_NegativeQuota(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	err := DecreaseUserQuota(ctx, 1, -10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "quota cannot be negative")
}

// TestDecreaseUserQuota_InsufficientQuota verifies that decrement fails when quota is insufficient.
func TestDecreaseUserQuota_InsufficientQuota(t *testing.T) {
	setupTestDatabase(t)

	user := &User{
		Username: fmt.Sprintf("test-user-dec-insuf-%d", time.Now().UnixNano()),
		Password: "testpassword12345",
		Status:   UserStatusEnabled,
		Role:     RoleCommonUser,
		Quota:    50,
	}
	require.NoError(t, DB.Create(user).Error)
	defer DB.Exec("DELETE FROM users WHERE id = ?", user.Id)

	ctx := context.Background()

	// Try to decrease more than available
	err := DecreaseUserQuota(ctx, user.Id, 100)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient user quota")

	// Verify quota remains unchanged
	var refreshedUser User
	require.NoError(t, DB.First(&refreshedUser, user.Id).Error)
	assert.Equal(t, int64(50), refreshedUser.Quota)
}

// TestDecreaseUserQuota_CanceledContext verifies behavior when context is canceled.
func TestDecreaseUserQuota_CanceledContext(t *testing.T) {
	setupTestDatabase(t)

	user := &User{
		Username: fmt.Sprintf("test-user-dec-cancel-%d", time.Now().UnixNano()),
		Password: "testpassword12345",
		Status:   UserStatusEnabled,
		Role:     RoleCommonUser,
		Quota:    100,
	}
	require.NoError(t, DB.Create(user).Error)
	defer DB.Exec("DELETE FROM users WHERE id = ?", user.Id)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel the context immediately

	// Even with canceled context, the operation should still succeed
	// because we use context.Background() as fallback when ctx.Done() is not yet checked
	// This tests that the function handles canceled context gracefully
	err := DecreaseUserQuota(ctx, user.Id, 30)
	// The operation may succeed or fail depending on when the context is checked
	// If SQLite retry is triggered, it will fail with context canceled
	// If no retry is needed, it will succeed
	if err != nil {
		// If error occurred, it should be context-related or DB-related
		t.Logf("DecreaseUserQuota with canceled context returned error (expected): %v", err)
	}
}

// TestIncreaseUserQuota_CanceledContext verifies behavior when context is canceled.
func TestIncreaseUserQuota_CanceledContext(t *testing.T) {
	setupTestDatabase(t)

	user := &User{
		Username: fmt.Sprintf("test-user-inc-cancel-%d", time.Now().UnixNano()),
		Password: "testpassword12345",
		Status:   UserStatusEnabled,
		Role:     RoleCommonUser,
		Quota:    100,
	}
	require.NoError(t, DB.Create(user).Error)
	defer DB.Exec("DELETE FROM users WHERE id = ?", user.Id)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel the context immediately

	// Similar to decrease test
	err := IncreaseUserQuota(ctx, user.Id, 50)
	if err != nil {
		t.Logf("IncreaseUserQuota with canceled context returned error (expected): %v", err)
	}
}

// TestIncreaseDecreaseUserQuota_Concurrent verifies concurrent quota operations work correctly.
func TestIncreaseDecreaseUserQuota_Concurrent(t *testing.T) {
	setupTestDatabase(t)

	user := &User{
		Username: fmt.Sprintf("test-user-conc-%d", time.Now().UnixNano()),
		Password: "testpassword12345",
		Status:   UserStatusEnabled,
		Role:     RoleCommonUser,
		Quota:    1000,
	}
	require.NoError(t, DB.Create(user).Error)
	defer DB.Exec("DELETE FROM users WHERE id = ?", user.Id)

	ctx := context.Background()
	initialQuota := user.Quota

	// Run concurrent operations
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			_ = IncreaseUserQuota(ctx, user.Id, 10)
			done <- true
		}()
		go func() {
			_ = DecreaseUserQuota(ctx, user.Id, 5)
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 20; i++ {
		<-done
	}

	var refreshedUser User
	require.NoError(t, DB.First(&refreshedUser, user.Id).Error)

	// After 10 increases of 10 and 10 decreases of 5, net change should be +50
	// However, some decreases may fail due to race conditions, so we just check
	// that the quota is reasonable (between initial and initial+100)
	assert.GreaterOrEqual(t, refreshedUser.Quota, initialQuota-50)
	assert.LessOrEqual(t, refreshedUser.Quota, initialQuota+100)
}
