package model

import (
	"context"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// pinnedStartupOutput captures child output and signals completed database/server startup.
// The pinned artifacts log server startup only after database bootstrap and root-account creation.
// This is a startup barrier, not a replacement for any sustained-traffic observation window.
type pinnedStartupOutput struct {
	mu       sync.Mutex
	text     strings.Builder
	ready    chan struct{}
	signaled bool
}

// newPinnedStartupOutput returns a concurrency-safe startup output collector.
func newPinnedStartupOutput() *pinnedStartupOutput {
	return &pinnedStartupOutput{ready: make(chan struct{})}
}

// Write appends child output and recognizes startup messages even across write boundaries.
func (output *pinnedStartupOutput) Write(data []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()
	n, err := output.text.Write(data)
	if !output.signaled {
		text := output.text.String()
		if strings.Contains(text, "database schema migrated") && strings.Contains(text, "server started") {
			output.signaled = true
			close(output.ready)
		}
	}
	return n, err
}

// String returns a snapshot without racing with the child process's output-copy goroutine.
func (output *pinnedStartupOutput) String() string {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.text.String()
}

// waitForPinnedStartup waits for both startup milestones or returns the context failure.
func waitForPinnedStartup(ctx context.Context, output *pinnedStartupOutput) error {
	select {
	case <-output.ready:
		return nil
	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), "wait for pinned database and server startup")
	}
}

// TestPinnedStartupOutputRequiresBothMilestones verifies the barrier and chunked output.
func TestPinnedStartupOutputRequiresBothMilestones(t *testing.T) {
	output := newPinnedStartupOutput()
	for _, text := range []string{"database schema mig", "rated\n", "server sta"} {
		_, err := output.Write([]byte(text))
		require.NoError(t, err)
		select {
		case <-output.ready:
			t.Fatal("startup must not complete before both full milestones")
		default:
		}
	}
	_, err := output.Write([]byte("rted\n"))
	require.NoError(t, err)
	require.NoError(t, waitForPinnedStartup(t.Context(), output))
	_, err = output.Write([]byte("server started again\n"))
	require.NoError(t, err, "a repeated milestone must not close the channel twice")
	require.Contains(t, output.String(), "database schema migrated\nserver started\n")
}

// TestPinnedStartupDeadline verifies a missing milestone fails without real wall-clock sleep.
func TestPinnedStartupDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		output := newPinnedStartupOutput()
		_, err := output.Write([]byte("database schema migrated\n"))
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(t.Context(), 22*time.Second)
		defer cancel()
		start := time.Now()
		require.ErrorIs(t, waitForPinnedStartup(ctx, output), context.DeadlineExceeded)
		require.Equal(t, 22*time.Second, time.Since(start))
	})
}

// TestPinnedStartupReturnsAtReadiness verifies the maximum budget is not a fixed delay.
func TestPinnedStartupReturnsAtReadiness(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		output := newPinnedStartupOutput()
		ctx, cancel := context.WithTimeout(t.Context(), 22*time.Second)
		defer cancel()
		start := time.Now()
		go func() {
			time.Sleep(2 * time.Second)
			if _, err := output.Write([]byte("database schema migrated\nserver started\n")); err != nil {
				t.Errorf("write startup output: %v", err)
			}
		}()
		require.NoError(t, waitForPinnedStartup(ctx, output))
		require.Equal(t, 2*time.Second, time.Since(start))
	})
}

// TestPinnedStartupOutputConcurrentAccess exercises output collection under the race detector.
func TestPinnedStartupOutputConcurrentAccess(t *testing.T) {
	output := newPinnedStartupOutput()
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			for range 100 {
				if _, err := output.Write([]byte("diagnostic\n")); err != nil {
					t.Errorf("write diagnostic output: %v", err)
				}
				_ = output.String()
			}
		})
	}
	workers.Wait()
	require.Equal(t, 400, strings.Count(output.String(), "diagnostic\n"))
}
