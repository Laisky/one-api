package asyncvideo

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/meta"
)

// Resolver obtains current credentials for a persisted routing identity. It
// must reject channel identity/type/host changes rather than poll another tenant.
type Resolver func(context.Context, *model.AsyncTask) (Provider, *meta.Meta, error)

var taskWake = make(chan struct{}, 1)

// Notify wakes an idle worker after a committed job. This is only a latency
// optimization; database scanning makes missed notifications harmless.
func Notify() {
	select {
	case taskWake <- struct{}{}:
	default:
	}
}

// StartWorkers starts a bounded, joinable pool. Each replica uses fenced DB
// leases; shutdown waits through model.WaitForBackgroundWorkers, not detached
// per-request goroutines. The poller and log outbox recover on every startup.
func StartWorkers(ctx context.Context, resolve Resolver, concurrency int) {
	concurrency = max(1, min(concurrency, 32))
	for range concurrency {
		model.StartBackgroundWorker(ctx, func(ctx context.Context) {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for ctx.Err() == nil {
				operation, cancel := context.WithTimeout(ctx, 45*time.Second)
				worked, err := ProcessOne(operation, resolve, time.Now().UTC())
				cancel()
				if err != nil && ctx.Err() == nil {
					logger.FromContext(ctx).Warn("async video worker operation failed", zap.Error(err))
				}
				if worked && err == nil {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				case <-taskWake:
				}
			}
		})
	}
	model.StartBackgroundWorker(ctx, func(ctx context.Context) {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for ctx.Err() == nil {
			operation, cancel := context.WithTimeout(ctx, 8*time.Second)
			if err := model.FlushAsyncTaskLogs(operation); err != nil && ctx.Err() == nil {
				logger.FromContext(ctx).Warn("async video log outbox failed", zap.Error(err))
			}
			if err := model.CleanSettledAsyncTasks(operation, time.Now().UTC()); err != nil && ctx.Err() == nil {
				logger.FromContext(ctx).Warn("async video receipt retention failed", zap.Error(err))
			}
			if err := model.CleanAsyncTaskLogReceipts(operation, time.Now().UTC()); err != nil && ctx.Err() == nil {
				logger.FromContext(ctx).Warn("async video log receipt retention failed", zap.Error(err))
			}
			cancel()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	})
}

// ProcessOne advances at most one due job. An accepted task is NEVER submitted
// again, including after a crash. Poll errors use capped backoff; expired jobs
// require reconciliation without inventing a failed outcome or refund.
func ProcessOne(ctx context.Context, resolve Resolver, now time.Time) (bool, error) {
	task, err := model.ClaimAsyncTask(ctx, now)
	if err != nil || task == nil {
		return false, err
	}
	update := model.AsyncTaskUpdate{State: task.State, NextPollAt: now.Add(5 * time.Second).UnixMilli()}
	if task.State == model.AsyncTaskUnknown {
		// Only outstanding financial evidence makes an anonymous task eligible.
		// Reconcile its known charge without issuing any provider POST or GET.
		update.ErrorCode = "submission_unknown"
		return true, persistObservation(ctx, task, update)
	}
	age := now.UnixMilli() - task.CreatedAt
	if (task.State == model.AsyncTaskSubmitting && age > (5*time.Minute).Milliseconds()) || age > (24*time.Hour).Milliseconds() {
		if task.State == model.AsyncTaskSubmitting && task.UpstreamID == "" {
			update.State, update.Refund, update.ErrorCode = model.AsyncTaskFailed, true, "submission_expired_before_dispatch"
		} else {
			update.State, update.ErrorCode = model.AsyncTaskReconciliation, "task_reconciliation_required"
		}
		return true, persistObservation(ctx, task, update)
	}
	provider, info, resolveErr := resolve(ctx, task)
	if resolveErr != nil || provider == nil || info == nil {
		// Missing credentials are not evidence that an already accepted job failed.
		if task.State == model.AsyncTaskSubmitting {
			update.State, update.Refund = model.AsyncTaskFailed, true
		} else {
			// A database/credential lookup failure is not a permanent provider
			// outcome. Retry resolution before safe GET polling after recovery.
			update.PollFailures = min(task.PollFailures+1, 8)
			update.NextPollAt = now.Add(time.Duration(min(300, 5<<update.PollFailures)) * time.Second).UnixMilli()
		}
		update.ErrorCode = "task_channel_unavailable"
		return true, persistObservation(ctx, task, update)
	}
	if task.State == model.AsyncTaskSubmitting {
		receipt, submitErr := provider.SubmitVideo(ctx, info, []byte(task.RequestBody))
		update.CostUSD = receipt.CostUSD
		switch {
		case receipt.ID != "":
			update.State, update.UpstreamID = model.AsyncTaskQueued, receipt.ID
		case receipt.Rejected:
			update.State, update.Refund, update.ErrorCode = model.AsyncTaskFailed, true, "upstream_submission_rejected"
		default:
			update.State, update.ErrorCode = model.AsyncTaskUnknown, "submission_unknown"
		}
		if submitErr != nil {
			// The error may contain provider payload/URL details. Public errors use a
			// fixed code. Only task identity/outcome is logged, never input or API keys.
			logger.FromContext(ctx).Warn("async video submission did not produce a normal receipt", zap.String("task_id", task.ID), zap.String("outcome", update.State))
		}
	} else {
		observation, pollErr := provider.PollVideo(ctx, info, task.UpstreamID)
		// Financial evidence survives malformed output/status. It may increase
		// a debit, but it can never authorize a refund on an invalid observation.
		update.CostUSD = observation.CostUSD
		if pollErr == nil {
			pollErr = ValidateObservation(task.State, observation)
		}
		if pollErr != nil {
			update.ErrorCode, update.PollFailures = "upstream_poll_unavailable", min(task.PollFailures+1, 8)
			if task.State == model.AsyncTaskFailed || task.State == model.AsyncTaskCancelled {
				update.ErrorCode = "upstream_generation_failed"
			}
			update.NextPollAt = now.Add(time.Duration(min(300, 5<<update.PollFailures)) * time.Second).UnixMilli()
		} else {
			update.State, update.Refund = observation.State, observation.Refunded
			update.CostUSD = observation.CostUSD
			// A stale provider replica must not regress a running/failed job to queued.
			if update.State == model.AsyncTaskQueued && task.State != model.AsyncTaskQueued {
				update.State = task.State
			}
			if update.State == model.AsyncTaskCompleted && observation.Result != nil {
				data, marshalErr := json.Marshal(observation.Result)
				if marshalErr != nil {
					return true, errors.Wrap(marshalErr, "encode async video result")
				}
				update.ResultJSON = string(data)
			}
			if update.State == model.AsyncTaskFailed || update.State == model.AsyncTaskCancelled {
				update.ErrorCode = "upstream_generation_failed"
			}
		}
	}
	if err := persistObservation(ctx, task, update); err != nil {
		// A provider ID is safe diagnostic metadata and is critical when DB failed
		// after upstream acceptance. No in-memory success is advertised as durable.
		logger.FromContext(ctx).Warn("async video observation persistence failed", zap.String("task_id", task.ID), zap.String("upstream_task_id", update.UpstreamID))
		return true, errors.Wrap(err, "persist async video worker outcome")
	}
	return true, nil
}

// persistObservation gives already-observed provider work a bounded durable write
// even when shutdown cancels I/O. This call remains inside the joined worker;
// it never starts an untracked goroutine or extends the provider operation.
func persistObservation(ctx context.Context, task *model.AsyncTask, update model.AsyncTaskUpdate) error {
	durable, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := model.RecordAsyncTaskEvidence(durable, task, update); err != nil {
		return err
	}
	return model.ApplyAsyncTaskUpdate(durable, task, update)
}
