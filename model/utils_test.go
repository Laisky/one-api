package model

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/random"
)

// TestBatchUpdaterGracefulShutdown verifies that the batch updater properly
// flushes pending changes during graceful shutdown.
func TestBatchUpdaterGracefulShutdown(t *testing.T) {
	setupTestDatabase(t)

	// Create a test user to accumulate quota changes
	user := &User{
		Username:    fmt.Sprintf("test-batch-shutdown-%d", time.Now().UnixNano()),
		Password:    "testpassword12345",
		Status:      UserStatusEnabled,
		Role:        RoleCommonUser,
		Quota:       1000,
		AccessToken: random.GetUUID(),
		AffCode:     random.GetRandomString(8),
	}
	require.NoError(t, DB.Create(user).Error)
	defer DB.Exec("DELETE FROM users WHERE id = ?", user.Id)

	// Save original config and restore after test
	originalInterval := config.BatchUpdateInterval
	originalTimeout := config.BatchUpdateTimeoutSec
	config.BatchUpdateInterval = 60 // Long interval so we can control when flush happens
	config.BatchUpdateTimeoutSec = 30
	defer func() {
		config.BatchUpdateInterval = originalInterval
		config.BatchUpdateTimeoutSec = originalTimeout
	}()

	// Initialize the batch updater
	InitBatchUpdater()

	// Add some quota changes that will be batched
	addNewRecord(BatchUpdateTypeUserQuota, user.Id, 100)
	addNewRecord(BatchUpdateTypeUserQuota, user.Id, 50)

	// Verify the changes are in the store (not yet flushed)
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	pendingValue := batchUpdateStores[BatchUpdateTypeUserQuota][user.Id]
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
	assert.Equal(t, int64(150), pendingValue, "pending changes should be accumulated")

	// Verify quota hasn't changed in DB yet
	var userBefore User
	require.NoError(t, DB.First(&userBefore, user.Id).Error)
	assert.Equal(t, int64(1000), userBefore.Quota, "quota should not be updated yet")

	// Trigger graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	StopBatchUpdater(ctx)

	// Wait a bit for the GoCritical task to be registered and complete
	time.Sleep(500 * time.Millisecond)

	// Verify the changes were flushed
	var userAfter User
	require.NoError(t, DB.First(&userAfter, user.Id).Error)
	assert.Equal(t, int64(1150), userAfter.Quota, "quota should be updated after shutdown flush")

}

// TestBatchUpdaterContextCancellation verifies that batch update respects context cancellation.
func TestBatchUpdaterContextCancellation(t *testing.T) {
	setupTestDatabase(t)

	// Create test users with unique identifiers
	timestamp := time.Now().UnixNano()
	users := make([]*User, 3)
	for i := range users {
		users[i] = &User{
			Username:    fmt.Sprintf("test-batch-cancel-%d-%d", timestamp, i),
			Password:    "testpassword12345",
			Status:      UserStatusEnabled,
			Role:        RoleCommonUser,
			Quota:       1000,
			AccessToken: random.GetUUID(),
			AffCode:     random.GetRandomString(8),
		}
		require.NoError(t, DB.Create(users[i]).Error)
	}
	defer func() {
		for _, u := range users {
			DB.Exec("DELETE FROM users WHERE id = ?", u.Id)
		}
	}()

	// Add pending changes for all users
	for _, u := range users {
		batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
		batchUpdateStores[BatchUpdateTypeUserQuota][u.Id] = 100
		batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
	}

	// Create an already-canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Run batch update with canceled context
	batchUpdate(ctx)

	// When context is already canceled, batchUpdate checks ctx.Err() at the start
	// of the loop and returns early WITHOUT swapping the store for that type.
	// This means the pending changes remain in the store for the next batch cycle.
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	storeLen := len(batchUpdateStores[BatchUpdateTypeUserQuota])
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()

	// The store should still have our pending changes since context was canceled
	// before processing BatchUpdateTypeUserQuota (which is first in the enum)
	assert.Equal(t, 3, storeLen, "store should retain pending changes when context is canceled early")

	// Clean up the store for future tests
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	batchUpdateStores[BatchUpdateTypeUserQuota] = make(map[int]int64)
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()
}

// TestAddNewRecord verifies that addNewRecord properly accumulates values.
func TestAddNewRecord(t *testing.T) {
	// Clear any existing state
	for i := range BatchUpdateTypeCount {
		batchUpdateLocks[i].Lock()
		batchUpdateStores[i] = make(map[int]int64)
		batchUpdateLocks[i].Unlock()
	}

	// Test adding new records
	addNewRecord(BatchUpdateTypeUserQuota, 1, 100)
	addNewRecord(BatchUpdateTypeUserQuota, 1, 50)
	addNewRecord(BatchUpdateTypeUserQuota, 2, 200)

	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	value1 := batchUpdateStores[BatchUpdateTypeUserQuota][1]
	value2 := batchUpdateStores[BatchUpdateTypeUserQuota][2]
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()

	assert.Equal(t, int64(150), value1, "values for same ID should be accumulated")
	assert.Equal(t, int64(200), value2, "different ID should have separate value")

	// Test negative values (decrements)
	addNewRecord(BatchUpdateTypeUserQuota, 1, -30)

	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	value1After := batchUpdateStores[BatchUpdateTypeUserQuota][1]
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()

	assert.Equal(t, int64(120), value1After, "negative values should be subtracted")

	// Clean up
	for i := range BatchUpdateTypeCount {
		batchUpdateLocks[i].Lock()
		batchUpdateStores[i] = make(map[int]int64)
		batchUpdateLocks[i].Unlock()
	}
}

// TestAddNewRecordConcurrent verifies thread safety of addNewRecord.
func TestAddNewRecordConcurrent(t *testing.T) {
	// Clear any existing state
	for i := range BatchUpdateTypeCount {
		batchUpdateLocks[i].Lock()
		batchUpdateStores[i] = make(map[int]int64)
		batchUpdateLocks[i].Unlock()
	}

	const numGoroutines = 100
	const numIterations = 100
	const testID = 999

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				addNewRecord(BatchUpdateTypeUserQuota, testID, 1)
			}
		}()
	}

	wg.Wait()

	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	finalValue := batchUpdateStores[BatchUpdateTypeUserQuota][testID]
	batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()

	expectedValue := int64(numGoroutines * numIterations)
	assert.Equal(t, expectedValue, finalValue, "concurrent adds should be properly accumulated")

	// Clean up
	for i := range BatchUpdateTypeCount {
		batchUpdateLocks[i].Lock()
		batchUpdateStores[i] = make(map[int]int64)
		batchUpdateLocks[i].Unlock()
	}
}

// TestBatchUpdateFlushesAllTypes verifies that batchUpdate processes all update types.
func TestBatchUpdateFlushesAllTypes(t *testing.T) {
	setupTestDatabase(t)

	// Create test user and token
	user := &User{
		Username:    fmt.Sprintf("test-batch-all-types-%d", time.Now().UnixNano()),
		Password:    "testpassword12345",
		Status:      UserStatusEnabled,
		Role:        RoleCommonUser,
		Quota:       1000,
		AccessToken: random.GetUUID(),
		AffCode:     random.GetRandomString(8),
	}
	require.NoError(t, DB.Create(user).Error)
	defer DB.Exec("DELETE FROM users WHERE id = ?", user.Id)

	// Clear stores and add test data
	for i := range BatchUpdateTypeCount {
		batchUpdateLocks[i].Lock()
		batchUpdateStores[i] = make(map[int]int64)
		batchUpdateLocks[i].Unlock()
	}

	// Add pending changes
	addNewRecord(BatchUpdateTypeUserQuota, user.Id, 100)
	addNewRecord(BatchUpdateTypeUsedQuota, user.Id, 50)
	addNewRecord(BatchUpdateTypeRequestCount, user.Id, 5)

	// Run batch update
	ctx := context.Background()
	batchUpdate(ctx)

	// Verify changes were applied
	var updatedUser User
	require.NoError(t, DB.First(&updatedUser, user.Id).Error)

	assert.Equal(t, int64(1100), updatedUser.Quota, "user quota should be increased")
	assert.Equal(t, int64(50), updatedUser.UsedQuota, "used quota should be updated")
	assert.Equal(t, 5, updatedUser.RequestCount, "request count should be updated")
}

// TestValidateOrderClause verifies order clause normalization and whitelisting.
func TestValidateOrderClause(t *testing.T) {
	allowed := map[string]string{
		"id":           "id",
		"name":         "name",
		"created_time": "created_at",
	}

	require.Equal(t, "name asc", ValidateOrderClause(" Name ", "ASC", allowed, "id desc"))
	require.Equal(t, "created_at desc", ValidateOrderClause("created_time", "desc", allowed, "id desc"))
	require.Equal(t, "id desc", ValidateOrderClause("invalid", "asc", allowed, "id desc"))
	require.Equal(t, "id desc", ValidateOrderClause("", "", allowed, "id desc"))
}
