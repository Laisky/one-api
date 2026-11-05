package model

import (
	"context"
	stderrors "errors"
	"fmt"
	"testing"
	"time"

	stackerrors "github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// requireAsyncOriginFrame verifies the chain retains a stack from the operation that detected the error.
func requireAsyncOriginFrame(t *testing.T, err error, frame string) {
	t.Helper()
	var frames string
	for e := err; e != nil; e = stderrors.Unwrap(e) {
		if trace, ok := e.(interface{ StackTrace() stackerrors.StackTrace }); ok {
			frames += fmt.Sprintf("%+v\n", trace.StackTrace())
		}
	}
	require.Contains(t, frames, frame)
}

// TestAsyncTransactionFailuresKeepOriginStacks verifies admission and both lease-fencing paths retain their causes and origin frames.
func TestAsyncTransactionFailuresKeepOriginStacks(t *testing.T) {
	t.Run("quota", func(t *testing.T) {
		_, input := durableJobTestStore(t)
		require.NoError(t, DB.Model(&User{}).Where("id = ?", input.UserID).Update("quota", 0).Error)
		_, _, err := ReserveAsyncTask(context.Background(), input)
		require.ErrorIs(t, err, ErrAsyncQuota)
		require.EqualError(t, err, "reserve asynchronous task: insufficient quota for asynchronous task")
		requireAsyncOriginFrame(t, err, "ReserveAsyncTask.func1.1")
	})
	for _, upstream := range []string{"", "job"} {
		t.Run("lost lease/"+upstream, func(t *testing.T) {
			_, input := durableJobTestStore(t)
			saved := reserveDurableJob(t, input)
			lease, err := ClaimAsyncTask(context.Background(), time.Now().UTC())
			require.NoError(t, err)
			require.Equal(t, saved.ID, lease.ID)
			require.NoError(t, DB.Model(&AsyncTask{}).Where("id = ?", lease.ID).Update("lease_owner", "another-owner").Error)
			err = ApplyAsyncTaskUpdate(context.Background(), lease, AsyncTaskUpdate{State: AsyncTaskQueued, UpstreamID: upstream})
			require.ErrorIs(t, err, ErrAsyncLeaseLost)
			require.EqualError(t, err, "commit async task observation: asynchronous task lease is no longer owned")
			requireAsyncOriginFrame(t, err, "ApplyAsyncTaskUpdate.func1.1")
		})
	}
}

// TestAsyncPublicMarkersKeepOriginStacks verifies directly exposed markers retain their cause, text and originating frame.
func TestAsyncPublicMarkersKeepOriginStacks(t *testing.T) {
	_, input := durableJobTestStore(t)
	saved := reserveDurableJob(t, input)
	_, err := FindAsyncTaskByDedup(context.Background(), saved.DedupKey, "different", saved.UserID, saved.UserUUID)
	require.ErrorIs(t, err, ErrAsyncIdempotencyConflict)
	require.EqualError(t, err, ErrAsyncIdempotencyConflict.Error())
	t.Run("dedup frame", func(t *testing.T) { requireAsyncOriginFrame(t, err, "FindAsyncTaskByDedup") })
	lease, err := ClaimAsyncTask(context.Background(), time.Now().UTC())
	require.NoError(t, err)
	require.NotNil(t, lease)
	require.NoError(t, DB.Model(&AsyncTask{}).Where("id = ?", lease.ID).Update("lease_owner", "another-owner").Error)
	err = RecordAsyncTaskEvidence(context.Background(), lease, AsyncTaskUpdate{UpstreamID: "job"})
	require.ErrorIs(t, err, ErrAsyncLeaseLost)
	require.EqualError(t, err, ErrAsyncLeaseLost.Error())
	requireAsyncOriginFrame(t, err, "RecordAsyncTaskEvidence")
}
