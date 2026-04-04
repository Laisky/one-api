package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestLogDB creates an in-memory SQLite DB for log tests and
// auto-migrates the Log and User tables. It saves/restores the global LOG_DB and DB.
func setupTestLogDB(t *testing.T) {
	t.Helper()
	origLogDB := LOG_DB
	origDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}, &User{}))
	LOG_DB = db
	DB = db
	t.Cleanup(func() {
		LOG_DB = origLogDB
		DB = origDB
	})
}

// TestRecordProvisionalConsumeLog verifies that a provisional log entry is
// created with the provisional metadata flag and estimated quota.
func TestRecordProvisionalConsumeLog(t *testing.T) {
	setupTestLogDB(t)

	entry := &Log{
		UserId:    42,
		ChannelId: 5,
		ModelName: "gpt-5.4",
		TokenName: "test-token",
		RequestId: "req_123",
		TraceId:   "trace_abc",
	}

	logID := RecordProvisionalConsumeLog(context.Background(), entry, 5000)
	require.Greater(t, logID, 0, "should return positive log ID")

	// Verify the record in the database
	var saved Log
	err := LOG_DB.First(&saved, logID).Error
	require.NoError(t, err)

	require.Equal(t, 42, saved.UserId)
	require.Equal(t, 5, saved.ChannelId)
	require.Equal(t, "gpt-5.4", saved.ModelName)
	require.Equal(t, 5000, saved.Quota)
	require.Equal(t, LogTypeProvisional, saved.Type)
	require.Equal(t, "req_123", saved.RequestId)
	require.Contains(t, saved.Content, "[provisional]")

	// Verify provisional metadata flag
	require.NotNil(t, saved.Metadata)
	provisional, ok := saved.Metadata[LogMetadataKeyProvisional]
	require.True(t, ok, "metadata should contain provisional flag")
	require.Equal(t, true, provisional)
}

// TestReconcileConsumeLog verifies that a provisional log entry can be
// reconciled with final billing data, removing the provisional flag.
func TestReconcileConsumeLog(t *testing.T) {
	setupTestLogDB(t)

	// Create provisional entry first
	entry := &Log{
		UserId:    42,
		ChannelId: 5,
		ModelName: "gpt-5.4",
		TokenName: "test-token",
		RequestId: "req_456",
	}
	logID := RecordProvisionalConsumeLog(context.Background(), entry, 10000)
	require.Greater(t, logID, 0)

	// Reconcile with final billing data
	finalMetadata := LogMetadata{"extra": "data"}
	err := ReconcileConsumeLog(context.Background(), logID, 7500,
		"model rate 1.25, group rate 1.00, completion rate 6.00",
		42875, 4000, 12345, finalMetadata)
	require.NoError(t, err)

	// Verify the reconciled record
	var saved Log
	err = LOG_DB.First(&saved, logID).Error
	require.NoError(t, err)

	require.Equal(t, 7500, saved.Quota, "quota should be updated to final value")
	require.Contains(t, saved.Content, "model rate 1.25")
	require.NotContains(t, saved.Content, "[provisional]")
	require.Equal(t, 42875, saved.PromptTokens)
	require.Equal(t, 4000, saved.CompletionTokens)
	require.Zero(t, saved.CachedPromptTokens)
	require.Zero(t, saved.CachedCompletionTokens)
	require.Equal(t, int64(12345), saved.ElapsedTime)

	// Verify provisional flag is removed
	require.NotNil(t, saved.Metadata)
	_, hasProvisional := saved.Metadata[LogMetadataKeyProvisional]
	require.False(t, hasProvisional, "provisional flag should be removed after reconciliation")

	// Verify extra metadata is preserved
	extra, ok := saved.Metadata["extra"]
	require.True(t, ok)
	require.Equal(t, "data", extra)
}

// TestReconcileConsumeLog_ZeroLogID is a no-op when logID is 0
// (e.g., when logging was disabled at pre-consume time).
func TestReconcileConsumeLog_ZeroLogID(t *testing.T) {
	err := ReconcileConsumeLog(context.Background(), 0, 5000, "content", 100, 50, 1000, nil)
	require.NoError(t, err)
}

// TestRecordProvisionalConsumeLog_ZeroQuota returns 0 when estimated quota is 0.
func TestRecordProvisionalConsumeLog_ZeroQuota(t *testing.T) {
	logID := RecordProvisionalConsumeLog(context.Background(), &Log{UserId: 1, ChannelId: 1, ModelName: "m"}, 0)
	require.Equal(t, 0, logID)
}

// TestProvisionalLogFullLifecycle tests the complete lifecycle:
// provisional create -> reconcile -> verify audit trail integrity.
func TestProvisionalLogFullLifecycle(t *testing.T) {
	setupTestLogDB(t)

	// Step 1: Pre-consume creates provisional log
	entry := &Log{
		UserId:    100,
		ChannelId: 10,
		ModelName: "gpt-5.4",
		TokenName: "user-token",
		IsStream:  true,
		RequestId: "req_lifecycle",
		TraceId:   "trace_lifecycle",
	}
	logID := RecordProvisionalConsumeLog(context.Background(), entry, 20000)
	require.Greater(t, logID, 0)

	// Verify provisional state
	var provisional Log
	require.NoError(t, LOG_DB.First(&provisional, logID).Error)
	require.Equal(t, 20000, provisional.Quota)
	require.Equal(t, LogTypeProvisional, provisional.Type)
	require.Equal(t, true, provisional.Metadata[LogMetadataKeyProvisional])

	// Step 2: Post-billing reconciles with actual usage (less than estimated)
	err := ReconcileConsumeLog(context.Background(), logID, 15000,
		"model rate 1.25, group rate 1.00, completion rate 4.00, cached_prompt 0, cached_completion 0, cache_write_5m 0, cache_write_1h 0",
		42875, 4000, 72000, nil)
	require.NoError(t, err)

	// Step 3: Verify final state - should be a normal consume log now
	var final Log
	require.NoError(t, LOG_DB.First(&final, logID).Error)
	require.Equal(t, 15000, final.Quota)
	require.Equal(t, 42875, final.PromptTokens)
	require.Equal(t, 4000, final.CompletionTokens)
	require.Equal(t, int64(72000), final.ElapsedTime)
	require.Equal(t, "req_lifecycle", final.RequestId)
	require.Equal(t, "trace_lifecycle", final.TraceId)
	require.Equal(t, true, final.IsStream)
	require.Equal(t, LogTypeConsume, final.Type)
	require.NotContains(t, final.Content, "[provisional]")

	// Provisional flag should be gone
	_, hasProvisional := final.Metadata[LogMetadataKeyProvisional]
	require.False(t, hasProvisional)
}

// TestReconcileConsumeLogDetailed verifies that detailed reconciliation updates
// cached token fields in addition to the existing prompt/completion columns.
func TestReconcileConsumeLogDetailed(t *testing.T) {
	setupTestLogDB(t)

	entry := &Log{
		UserId:    42,
		ChannelId: 5,
		ModelName: "gpt-5.4",
		TokenName: "test-token",
		RequestId: "req_cached",
		TraceId:   "trace_cached",
	}
	logID := RecordProvisionalConsumeLog(context.Background(), entry, 10000)
	require.Greater(t, logID, 0)

	metadata := AppendCacheWriteTokensMetadata(LogMetadata{"extra": "data"}, 128, 256)
	err := ReconcileConsumeLogDetailed(context.Background(), logID, ConsumeLogReconcileDetail{
		FinalQuota:             6184,
		Content:                "model rate 1.00, group rate 1.00, completion rate 1.00, cached_prompt 9088, cached_completion 0, cache_write_5m 128, cache_write_1h 256",
		PromptTokens:           9208,
		CompletionTokens:       653,
		CachedPromptTokens:     9088,
		CachedCompletionTokens: 0,
		ElapsedTime:            14000,
		Metadata:               metadata,
	})
	require.NoError(t, err)

	var saved Log
	require.NoError(t, LOG_DB.First(&saved, logID).Error)
	require.Equal(t, 6184, saved.Quota)
	require.Equal(t, 9208, saved.PromptTokens)
	require.Equal(t, 653, saved.CompletionTokens)
	require.Equal(t, 9088, saved.CachedPromptTokens)
	require.Zero(t, saved.CachedCompletionTokens)
	require.Equal(t, int64(14000), saved.ElapsedTime)

	cacheWriteAny, ok := saved.Metadata[LogMetadataKeyCacheWriteTokens]
	require.True(t, ok)
	cacheWrite, ok := cacheWriteAny.(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(128), cacheWrite[LogMetadataKeyCacheWrite5m])
	require.Equal(t, float64(256), cacheWrite[LogMetadataKeyCacheWrite1h])
	require.Equal(t, "data", saved.Metadata["extra"])

	_, hasProvisional := saved.Metadata[LogMetadataKeyProvisional]
	require.False(t, hasProvisional)
}
