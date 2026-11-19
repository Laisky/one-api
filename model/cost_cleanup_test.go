package model

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common/helper"
)

// TestRemoveOldRequestCostUsesCapturedHandle proves the sampled cleanup works
// on the handle captured at scheduling time and tolerates a nil handle. The
// background job used to read the global DB when it ran, so a handle swapped
// to nil (test cleanup) crashed the process with a nil-pointer panic.
func TestRemoveOldRequestCostUsesCapturedHandle(t *testing.T) {
	require.NotPanics(t, func() { removeOldRequestCost(nil) })
	require.NotPanics(t, func() { scheduleOldRequestCostCleanup(nil) })

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&UserRequestCost{}))
	now := helper.GetTimestamp()
	require.NoError(t, db.Create(&UserRequestCost{RequestID: "old", CreatedTime: now - 8*24*3600, Quota: 1}).Error)
	require.NoError(t, db.Create(&UserRequestCost{RequestID: "fresh", CreatedTime: now, Quota: 1}).Error)

	removeOldRequestCost(db)

	var ids []string
	require.NoError(t, db.Model(&UserRequestCost{}).Pluck("request_id", &ids).Error)
	require.Equal(t, []string{"fresh"}, ids)
}
