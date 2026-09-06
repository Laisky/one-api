package model

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestRetentionCleanersDoNotBlockStartup verifies an initial retention sweep is
// performed in the background, so a large expired-data backlog cannot delay
// process startup.
func TestRetentionCleanersDoNotBlockStartup(t *testing.T) {
	tests := []struct {
		name    string
		migrate any
		start   func(context.Context, int)
	}{
		{
			name:    "traces",
			migrate: &Trace{},
			start:   StartTraceRetentionCleaner,
		},
		{
			name:    "async task bindings",
			migrate: &AsyncTaskBinding{},
			start:   StartAsyncTaskRetentionCleaner,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(t.TempDir()+"/retention.db"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(tt.migrate))

			previousDB := DB
			DB = db
			t.Cleanup(func() { DB = previousDB })

			enteredSweep := make(chan struct{})
			completedSweep := make(chan struct{})
			var enteredOnce sync.Once
			var completedOnce sync.Once
			require.NoError(t, db.Callback().Raw().Before("gorm:raw").Register(
				"test:block-initial-retention-sweep", func(*gorm.DB) {
					enteredOnce.Do(func() { close(enteredSweep) })
					time.Sleep(300 * time.Millisecond)
				},
			))
			require.NoError(t, db.Callback().Raw().After("gorm:raw").Register(
				"test:complete-initial-retention-sweep", func(*gorm.DB) {
					completedOnce.Do(func() { close(completedSweep) })
				},
			))

			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			startedAt := time.Now()
			tt.start(ctx, 1)
			require.Less(t, time.Since(startedAt), 150*time.Millisecond,
				"starting the cleaner must not wait for its first sweep")

			select {
			case <-enteredSweep:
			case <-time.After(2 * time.Second):
				t.Fatal("initial retention sweep did not run")
			}

			select {
			case <-completedSweep:
			case <-time.After(2 * time.Second):
				t.Fatal("initial retention sweep did not complete")
			}
		})
	}
}
