package streaming

import (
	"context"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	quotautil "github.com/songquanpeng/one-api/relay/quota"
)

// ErrQuotaExceeded indicates the user's quota was exhausted during streaming.
var ErrQuotaExceeded = errors.New("streaming quota exceeded")

// QuotaTrackerParams describes the immutable configuration required to
// initialize a streaming quota tracker.
type QuotaTrackerParams struct {
	UserID                 int
	TokenID                int
	ChannelID              int
	ModelName              string
	PromptTokens           int
	ModelRatio             float64
	ChannelModelRatio      map[string]float64
	GroupRatio             float64
	PreConsumedQuota       int64
	ChannelModelConfigs    map[string]model.ModelConfigLocal
	ChannelCompletionRatio map[string]float64
	PricingAdaptor         adaptor.Adaptor
	FlushInterval          time.Duration
	Logger                 *zap.Logger
	Ctx                    context.Context
}

// QuotaTracker incrementally accounts for completion tokens during streaming
// responses and proactively deducts quota when thresholds are reached. It keeps
// track of how much quota has already been charged beyond the initial
// pre-consumption so the final reconciliation can avoid double counting.
type QuotaTracker struct {
	params        QuotaTrackerParams
	mu            sync.Mutex
	chargedQuota  int64
	completionSum int
	lastFlush     time.Time
	finalUsage    *relaymodel.Usage
	abortErr      error
}

// NewQuotaTracker constructs a streaming quota tracker. If FlushInterval is not
// provided, it defaults to 3 seconds.
func NewQuotaTracker(params QuotaTrackerParams) *QuotaTracker {
	if params.FlushInterval <= 0 {
		params.FlushInterval = 3 * time.Second
	}
	if params.Ctx == nil {
		params.Ctx = context.Background()
	}
	tracker := &QuotaTracker{
		params:    params,
		lastFlush: time.Now().Add(-params.FlushInterval),
	}
	if tracker.params.Logger == nil {
		tracker.params.Logger = zap.NewNop()
	}
	return tracker
}

// StoreTracker attaches the tracker to the gin context for downstream access.
func StoreTracker(c *gin.Context, tracker *QuotaTracker) {
	if c == nil {
		return
	}
	c.Set(ctxkey.StreamingQuotaTracker, tracker)
}

// FromContext retrieves the tracker from the gin context, if present.
func FromContext(c *gin.Context) *QuotaTracker {
	if c == nil {
		return nil
	}
	if v, ok := c.Get(ctxkey.StreamingQuotaTracker); ok {
		if tracker, ok := v.(*QuotaTracker); ok {
			return tracker
		}
	}
	return nil
}

// RecordCompletionTokens registers newly generated completion tokens and
// performs incremental billing when the flush interval elapses. The delta should
// represent text tokens only; prompt tokens are handled via pre-consumption.
func (t *QuotaTracker) RecordCompletionTokens(delta int) error {
	if delta <= 0 {
		return t.maybeFlush(false)
	}

	t.mu.Lock()
	t.completionSum += delta
	err := t.flushLocked(false)
	t.mu.Unlock()
	if err != nil {
		return errors.Wrap(err, "flush streaming quota during record")
	}
	return nil
}

// UpdateFinalUsage stores the usage reported by the upstream provider. This is
// used during final reconciliation to prefer authoritative token counts.
func (t *QuotaTracker) UpdateFinalUsage(usage *relaymodel.Usage) {
	if usage == nil {
		return
	}
	t.mu.Lock()
	t.finalUsage = usage
	if usage.PromptTokens > 0 {
		t.params.PromptTokens = usage.PromptTokens
	}
	t.completionSum = usage.CompletionTokens
	t.mu.Unlock()
}

// Finalize flushes any remaining quota and returns the usage snapshot alongside
// the amount already charged during streaming.
func (t *QuotaTracker) Finalize(finalUsage *relaymodel.Usage) (*relaymodel.Usage, int64, error) {
	if finalUsage != nil {
		t.UpdateFinalUsage(finalUsage)
	}

	t.mu.Lock()
	err := t.flushLocked(true)
	snapshot := t.currentUsageLocked()
	charged := t.chargedQuota
	abortErr := t.abortErr
	t.mu.Unlock()

	if abortErr != nil {
		return snapshot, charged, abortErr
	}
	if err != nil {
		return snapshot, charged, errors.Wrap(err, "flush streaming quota during finalize")
	}
	return snapshot, charged, nil
}

// ChargedQuota returns the total quota charged during streaming (excluding the
// initial pre-consumption amount).
func (t *QuotaTracker) ChargedQuota() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.chargedQuota
}

// AbortError reports any terminal error encountered by the tracker (for
// example, insufficient quota).
func (t *QuotaTracker) AbortError() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.abortErr
}

func (t *QuotaTracker) maybeFlush(force bool) error {
	t.mu.Lock()
	err := t.flushLocked(force)
	t.mu.Unlock()
	if err != nil {
		return errors.Wrap(err, "flush streaming quota")
	}
	return nil
}

func (t *QuotaTracker) flushLocked(force bool) error {
	if t.abortErr != nil {
		return t.abortErr
	}

	now := time.Now()
	if !force && now.Sub(t.lastFlush) < t.params.FlushInterval {
		return nil
	}

	targetQuota := t.computeTargetQuotaLocked()
	delta := targetQuota - t.chargedQuota
	if delta <= 0 {
		t.lastFlush = now
		return nil
	}

	if err := t.ensureQuotaLocked(delta); err != nil {
		wrappedErr := errors.Wrap(err, "ensure quota during streaming flush")
		if errors.Is(err, ErrQuotaExceeded) {
			t.abortErr = wrappedErr
		}
		return wrappedErr
	}

	ctx := t.params.Ctx
	if err := model.PostConsumeTokenQuota(ctx, t.params.TokenID, delta); err != nil {
		err = errors.Wrap(err, "post consume token quota during streaming flush")
		t.abortErr = err
		return err
	}

	if err := model.CacheDecreaseUserQuota(ctx, t.params.UserID, delta); err != nil {
		t.params.Logger.Warn("streaming quota tracker failed to update user cache",
			zap.Int("user_id", t.params.UserID),
			zap.Error(err))
	}

	t.chargedQuota += delta
	t.lastFlush = now
	return nil
}

func (t *QuotaTracker) computeTargetQuotaLocked() int64 {
	usage := t.currentUsageLocked()
	result := quotautil.Compute(quotautil.ComputeInput{
		Usage:                  usage,
		ModelName:              t.params.ModelName,
		ModelRatio:             t.params.ModelRatio,
		ChannelModelRatio:      t.params.ChannelModelRatio,
		GroupRatio:             t.params.GroupRatio,
		ChannelModelConfigs:    t.params.ChannelModelConfigs,
		ChannelCompletionRatio: t.params.ChannelCompletionRatio,
		PricingAdaptor:         t.params.PricingAdaptor,
	})
	target := max(result.TotalQuota-t.params.PreConsumedQuota, 0)
	return target
}

func (t *QuotaTracker) ensureQuotaLocked(delta int64) error {
	if delta <= 0 {
		return nil
	}
	remaining, err := model.CacheGetUserQuota(t.params.Ctx, t.params.UserID)
	if err != nil {
		return errors.Wrap(err, "get user quota during streaming flush")
	}
	if remaining < delta {
		return ErrQuotaExceeded
	}
	return nil
}

func (t *QuotaTracker) currentUsageLocked() *relaymodel.Usage {
	if t.finalUsage != nil {
		// Return a defensive copy to avoid accidental mutation downstream.
		clone := *t.finalUsage
		return &clone
	}
	total := t.params.PromptTokens + t.completionSum
	return &relaymodel.Usage{
		PromptTokens:     t.params.PromptTokens,
		CompletionTokens: t.completionSum,
		TotalTokens:      total,
	}
}
