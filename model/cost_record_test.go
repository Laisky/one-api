package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
)

func setupCostTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&UserRequestCost{})
	require.NoError(t, err)

	return db
}

func TestUpdateUserRequestCostQuotaByRequestID_AllowsZeroQuotaRecords(t *testing.T) {
	testDB := setupCostTestDB(t)
	originalDB := DB
	DB = testDB
	defer func() { DB = originalDB }()

	originalUsingSQLite := common.UsingSQLite.Load()
	common.UsingSQLite.Store(true)
	defer func() { common.UsingSQLite.Store(originalUsingSQLite) }()

	userID := 42
	reqID := "test-request-0"

	require.NoError(t, UpdateUserRequestCostQuotaByRequestID(userID, reqID, 0))

	cost, err := GetCostByRequestId(reqID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), cost.Quota)
	assert.Equal(t, userID, cost.UserID)

	// Update to a non-zero quota then back to zero to ensure overrides are applied.
	require.NoError(t, UpdateUserRequestCostQuotaByRequestID(userID, reqID, 123))
	require.NoError(t, UpdateUserRequestCostQuotaByRequestID(userID, reqID, 0))

	cost, err = GetCostByRequestId(reqID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), cost.Quota)
	assert.Equal(t, userID, cost.UserID)
}
