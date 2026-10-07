package model

import (
	"context"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common"
)

// TestChannelResponseTimeContextCompatibility verifies explicit cancellation and
// deadlines still prevent model writes, while the legacy method persists latency.
// Parameters: t owns the isolated database and assertions. Returns: no values.
func TestChannelResponseTimeContextCompatibility(t *testing.T) {
	cases := []struct {
		name     string
		canceled bool
		deadline bool
		legacy   bool
	}{
		{name: "canceled caller", canceled: true},
		{name: "expired caller deadline", deadline: true},
		{name: "live caller"},
		{name: "legacy caller", legacy: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(t.TempDir()+"/channel.db"), &gorm.Config{Logger: glogger.Discard})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			require.NoError(t, db.AutoMigrate(&Channel{}))
			originalDB := DB
			DB = db
			t.Cleanup(func() { DB = originalDB })
			oldSQLite := common.UsingSQLite.Load()
			common.UsingSQLite.Store(true)
			t.Cleanup(func() { common.UsingSQLite.Store(oldSQLite) })
			channel := &Channel{Name: "latency-control", Type: 1, ResponseTime: 91, TestTime: 1}
			require.NoError(t, db.Create(channel).Error)
			core, logs := observer.New(zapcore.DebugLevel)
			lg, err := glog.NewWithName("latency-control", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
			require.NoError(t, err)
			ctx := gmw.SetLogger(context.Background(), lg)
			if tc.canceled {
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = canceled
			}
			if tc.deadline {
				expired, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
				defer cancel()
				ctx = expired
			}
			if tc.legacy {
				channel.UpdateResponseTime(42)
			} else {
				channel.UpdateResponseTimeWithContext(ctx, 42)
			}
			var got Channel
			require.NoError(t, db.First(&got, "id = ?", channel.Id).Error)
			failures := logs.FilterMessage("failed to update response time").All()
			if tc.canceled || tc.deadline {
				require.Equal(t, 91, got.ResponseTime)
				require.EqualValues(t, 1, got.TestTime)
				require.Len(t, failures, 1, "model cancellation must remain visible exactly once")
				fields := failures[0].ContextMap()
				require.EqualValues(t, channel.Id, fields["channel_id"])
				require.Equal(t, channel.UUID, fields["channel_uuid"])
				require.Contains(t, fields["error"], ctx.Err().Error())
			} else {
				require.Equal(t, 42, got.ResponseTime)
				require.Greater(t, got.TestTime, int64(1))
				require.Empty(t, failures)
			}
		})
	}
}
