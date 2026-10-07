package controller

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
	"gorm.io/gorm"

	"github.com/Laisky/one-api/model"
)

// TestChannelResponseTimePersistenceBudget verifies a stalled connection-pool
// acquisition ends at the worker budget and a successful worker releases its context.
// Parameters: t owns the isolated database and assertions. Returns: no values.
func TestChannelResponseTimePersistenceBudget(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		name := "completed write releases context"
		if blocked {
			name = "stalled connection pool expires"
		}
		t.Run(name, func(t *testing.T) {
			setupChannelSweepTestEnvironment(t)
			channel := &model.Channel{Name: "bounded-latency", Type: 1, ResponseTime: 91, TestTime: 1}
			require.NoError(t, model.DB.Create(channel).Error)
			core, logs := observer.New(zapcore.DebugLevel)
			lg, err := glog.NewWithName("bounded-latency", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
			require.NoError(t, err)
			parent, cancelParent := context.WithCancel(gmw.SetLogger(context.Background(), lg))
			cancelParent()
			captured := make(chan context.Context, 1)
			require.NoError(t, model.DB.Callback().Update().Before("gorm:begin_transaction").Register(
				"test:bounded_latency_context", func(tx *gorm.DB) { captured <- tx.Statement.Context }))
			sqlDB, err := model.DB.DB()
			require.NoError(t, err)
			if blocked {
				// Hold the fixture's sole SQL connection so database/sql must wait
				// for availability using the persistence context's actual deadline.
				conn, err := sqlDB.Conn(context.Background())
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, conn.Close()) })
			}
			started := time.Now()
			persistChannelTestResponseTime(parent, channel, 42)
			elapsed := time.Since(started)
			var ctx context.Context
			select {
			case ctx = <-captured:
			case <-time.After(time.Second):
				t.Fatal("persistence did not invoke the real update callback")
			}
			// Deadline expiry takes precedence over the helper's deferred cancellation.
			if blocked {
				require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
				require.GreaterOrEqual(t, elapsed, channelTestPersistenceTimeout)
				require.Less(t, elapsed, channelTestPersistenceTimeout+3*time.Second)
				failures := logs.FilterMessage("failed to update response time").All()
				require.Len(t, failures, 1)
				require.Contains(t, failures[0].ContextMap()["error"], "context deadline exceeded")
				return
			}
			require.ErrorIs(t, ctx.Err(), context.Canceled)
			require.Less(t, elapsed, channelTestPersistenceTimeout)
			var stored model.Channel
			require.NoError(t, model.DB.First(&stored, channel.Id).Error)
			require.Equal(t, 42, stored.ResponseTime)
			require.Greater(t, stored.TestTime, int64(1))
			require.Empty(t, logs.FilterMessage("failed to update response time").All())
		})
	}
}
