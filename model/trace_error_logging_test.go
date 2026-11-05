package model

import (
	"context"
	"net/http/httptest"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// TestTraceFailuresAreReportedByCaller verifies trace errors retain diagnostic
// context and are logged once by the caller. t owns the fixtures and assertions.
func TestTraceFailuresAreReportedByCaller(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/trace.db"), &gorm.Config{Logger: gormlogger.Discard})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	oldDB := DB
	DB = db
	t.Cleanup(func() { DB = oldDB })

	core, logs := observer.New(zapcore.DebugLevel)
	lg, err := glog.NewWithName("trace-test", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	ctx := gmw.SetLogger(context.Background(), lg)
	ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginCtx.Request = httptest.NewRequest("GET", "/", nil).WithContext(ctx)
	gmw.SetLogger(ginCtx, lg)

	cases := []struct {
		name string
		run  func() error
		want string
	}{
		{"create", func() error { _, e := CreateTrace(ctx, "trace-id", "/", "GET", 0); return e }, "failed to create trace record"},
		{"timestamp", func() error { return UpdateTraceTimestamp(ginCtx, "trace-id", TimestampRequestCompleted) }, "failed to query trace record for timestamp update"},
		{"external call", func() error { return AppendTraceExternalCall(ginCtx, "trace-id", TraceExternalCall{}) }, "failed to query trace record for external call"},
		{"status", func() error { return UpdateTraceStatus(ctx, "trace-id", 500) }, "failed to update trace status"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logs.TakeAll()
			err := tc.run()
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
			require.Contains(t, err.Error(), "trace-id")
			lg.Error("trace operation failed", zap.Error(err))
			require.Len(t, logs.FilterLevelExact(zapcore.ErrorLevel).All(), 1)
		})
	}
}
