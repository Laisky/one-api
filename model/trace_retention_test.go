package model

import (
	"context"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/logger"
)

func TestCleanExpiredTraces(t *testing.T) {
	setupTestDatabase(t)

	require.NoError(t, DB.Exec("DELETE FROM traces WHERE trace_id LIKE 'test-retention-%'").Error)

	ctx := gmw.SetLogger(context.Background(), logger.Logger)

	retentionDays := 30
	oldID := "test-retention-old"
	newID := "test-retention-new"

	_, err := CreateTrace(ctx, oldID, "/api/old", "GET", 0)
	require.NoError(t, err)

	_, err = CreateTrace(ctx, newID, "/api/new", "GET", 0)
	require.NoError(t, err)

	oldTimestamp := time.Now().UTC().Add(-time.Duration(retentionDays+1) * 24 * time.Hour).UnixMilli()
	freshTimestamp := time.Now().UTC().Add(-time.Duration(retentionDays-1) * 24 * time.Hour).UnixMilli()

	require.NoError(t, DB.Model(&Trace{}).Where("trace_id = ?", oldID).Update("created_at", oldTimestamp).Error)
	require.NoError(t, DB.Model(&Trace{}).Where("trace_id = ?", newID).Update("created_at", freshTimestamp).Error)

	deleted, err := CleanExpiredTraces(retentionDays)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)

	var count int64
	require.NoError(t, DB.Model(&Trace{}).Where("trace_id = ?", oldID).Count(&count).Error)
	require.Equal(t, int64(0), count)

	require.NoError(t, DB.Model(&Trace{}).Where("trace_id = ?", newID).Count(&count).Error)
	require.Equal(t, int64(1), count)
}
