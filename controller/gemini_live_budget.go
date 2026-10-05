package controller

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/relayctx"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
	"github.com/Laisky/one-api/relay/realtime"
)

// liveSpendTopUpTimeout bounds one durable reservation increment, so a stalled
// database can neither hang a socket reader nor extend a session indefinitely.
const liveSpendTopUpTimeout = 10 * time.Second

// liveEstimateTokenLimit caps any single estimate bucket at 2^40 tokens.
const liveEstimateTokenLimit = 1 << 40

// realtimeReservation is the durable funding state handed to settlement.
// Reserved is the quota debited before and during the session; EstimateFloor
// is the charge kept when receipts are missing; Metadata is bounded audit data.
type realtimeReservation struct {
	Reserved      int64
	EstimateFloor int64
	Metadata      model.LogMetadata
}

// liveSpendGate is the controller-owned realtime.SpendGate of one Gemini Live
// session. It prices estimates and receipts with the same quota.Compute path as
// settlement, and extends the reservation with the same atomic user-and-token
// debit as admission. It never retains the gin context.
type liveSpendGate struct {
	mu          sync.Mutex
	ctx         context.Context
	lg          glog.Logger
	tokenID     int
	userID      int
	pricing     quota.ComputeInput
	worstInput  int
	worstOutput int
	chunk       int64
	initial     int64
	reserved    int64
	committed   int64
	evidence    int64
	unfunded    int64
	topUps      int
	exhausted   bool
}

// newLiveSpendGate prepares the budget for an admitted session. Parameters: c
// supplies the request logger and identity, m the billed user and token,
// pricing the settlement inputs without usage, and reserved the admission
// reservation already debited. Returns: the gate, or an error wrapping
// realtime.ErrInvalidPrice when a modality cannot be priced before work starts.
func newLiveSpendGate(c *gin.Context, m *meta.Meta, pricing quota.ComputeInput, reserved int64) (*liveSpendGate, error) {
	g := &liveSpendGate{ctx: relayctx.Detach(c), lg: gmw.GetLogger(c), tokenID: m.TokenId, userID: m.UserId,
		pricing: pricing, chunk: reserved, initial: reserved, reserved: reserved}
	g.pricing.Usage = nil
	const probe = 1_000_000
	inputs := []realtime.Tokens{{Input: probe, Text: probe}, {Input: probe, Audio: probe},
		{Input: probe, Image: probe}, {Input: probe, Video: probe}}
	outputs := []realtime.Tokens{{Output: probe, OutputText: probe}, {Output: probe, OutputAudio: probe}}
	var err error
	if g.worstInput, err = g.mostExpensive(inputs); err != nil {
		return nil, err
	}
	if g.worstOutput, err = g.mostExpensive(outputs); err != nil {
		return nil, err
	}
	return g, nil
}

// mostExpensive selects the modality whose probe costs most at unit group
// ratio. Parameters: probes are single-modality token sets. Returns: the index
// of the most expensive probe, or an error when any probe is unpriceable.
func (g *liveSpendGate) mostExpensive(probes []realtime.Tokens) (int, error) {
	best, bestCost := 0, int64(-1)
	for i, tokens := range probes {
		cost, unpriced := g.priceRecord(realtime.Record{Tokens: tokens}, 1)
		if unpriced {
			return 0, errors.Wrap(realtime.ErrInvalidPrice, "unpriceable Gemini Live modality")
		}
		if cost > bestCost {
			best, bestCost = i, cost
		}
	}
	return best, nil
}

// priceRecord prices one record with the settlement resolver. Parameters:
// record is validated token usage and group the group multiplier. Returns: the
// rounded quota and whether any bucket was unpriceable (a lower bound).
func (g *liveSpendGate) priceRecord(record realtime.Record, group float64) (int64, bool) {
	ledger := realtime.NewLedger()
	ledger.Records = []realtime.Record{record}
	ledger.InputTokens, ledger.OutputTokens = record.Tokens.Input, record.Tokens.Output
	input := g.pricing
	input.GroupRatio = group
	input.Usage = &relaymodel.Usage{Realtime: ledger}
	result := quota.Compute(input)
	return result.TotalQuota, result.UnpricedUsage
}

// priceEstimate converts a conservative estimate into a validated record and
// prices it. Context and output allowances use the session's most expensive
// input and output modalities. Parameters: e is the estimate. Returns: quota,
// or an error when the estimate cannot be priced; callers then fail closed.
func (g *liveSpendGate) priceEstimate(e realtime.Estimate) (int64, error) {
	if e.IsZero() {
		return 0, nil
	}
	// Reject absurd bounds before any arithmetic; the cap is far above the
	// tokens a 15-minute session can carry, so it only fails closed.
	for _, value := range []int64{e.Text, e.Audio, e.Image, e.Video, e.OutputText, e.OutputAudio, e.Context, e.Output} {
		if value < 0 || value > liveEstimateTokenLimit {
			return 0, errors.Wrap(realtime.ErrQuotaOverflow, "Gemini Live estimate exceeds the quota range")
		}
	}
	t := realtime.Tokens{Text: e.Text, Audio: e.Audio, Image: e.Image, Video: e.Video,
		OutputText: e.OutputText, OutputAudio: e.OutputAudio}
	inputs := []*int64{&t.Text, &t.Audio, &t.Image, &t.Video}
	*inputs[g.worstInput] += e.Context
	outputs := []*int64{&t.OutputText, &t.OutputAudio}
	*outputs[g.worstOutput] += e.Output
	t.Input = t.Text + t.Audio + t.Image + t.Video
	t.Output = t.OutputText + t.OutputAudio
	cost, unpriced := g.priceRecord(realtime.Record{Tokens: t}, g.pricing.GroupRatio)
	if unpriced {
		return 0, errors.Wrap(realtime.ErrUnpriceableInput, "Gemini Live estimate is unpriceable")
	}
	return cost, nil
}

// Commit adds one authoritative receipt to observed spend. Parameters: record
// is a committed provider receipt. Returns: none. Per-receipt rounding can only
// overstate the session total that settlement rounds once.
func (g *liveSpendGate) Commit(record realtime.Record) {
	cost, unpriced := g.priceRecord(record, g.pricing.GroupRatio)
	g.mu.Lock()
	defer g.mu.Unlock()
	if unpriced {
		g.lg.Debug("Gemini Live receipt is a pricing lower bound", zap.Int64("receipt_quota", cost))
	}
	g.committed = min(g.committed+cost, math.MaxInt64/2)
}

// Ensure keeps the durable reservation at or above observed spend plus
// pending. Parameters: pending bounds all unreceipted work. Returns: nil when
// funded, or an error wrapping realtime.ErrBudgetExhausted after which every
// later call also fails, so a refused session can never resume.
func (g *liveSpendGate) Ensure(pending realtime.Estimate) error {
	cost, priceErr := g.priceEstimate(pending)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.exhausted {
		return errors.WithStack(realtime.ErrBudgetExhausted)
	}
	if priceErr != nil {
		g.exhausted = true
		g.lg.Warn("Gemini Live session budget exhausted", zap.Error(priceErr), zap.Int64("reserved_quota", g.reserved))
		return errors.Wrap(realtime.ErrBudgetExhausted, "unpriceable Live work")
	}
	need := min(g.committed+cost, math.MaxInt64/2)
	if need <= g.reserved {
		return nil
	}
	if err := g.topUpLocked(need - g.reserved); err != nil {
		g.exhausted, g.unfunded = true, need-g.reserved
		g.lg.Warn("Gemini Live session budget exhausted", zap.Error(err), zap.Int64("reserved_quota", g.reserved),
			zap.Int64("committed_quota", g.committed), zap.Int64("required_quota", need))
		return errors.Wrap(realtime.ErrBudgetExhausted, "insufficient quota for Live work")
	}
	return nil
}

// topUpLocked atomically debits at least delta from the user and finite token.
// It first tries one admission-sized chunk to limit database writes, then the
// exact shortfall. Parameters: delta is the positive shortfall. Returns: the
// final reservation error; the caller holds g.mu.
func (g *liveSpendGate) topUpLocked(delta int64) error {
	ctx, cancel := context.WithTimeout(g.ctx, liveSpendTopUpTimeout)
	defer cancel()
	amount := max(delta, g.chunk)
	err := model.PreConsumeTokenQuota(ctx, g.tokenID, amount)
	if err != nil && amount > delta {
		amount = delta
		err = model.PreConsumeTokenQuota(ctx, g.tokenID, amount)
	}
	if err != nil {
		return errors.Wrap(err, "extend Gemini Live reservation")
	}
	g.reserved += amount
	g.topUps++
	if cacheErr := model.CacheDecreaseUserQuota(ctx, g.userID, amount); cacheErr != nil {
		g.lg.Warn("failed to sync user quota cache after Live reservation", zap.Error(cacheErr))
	}
	g.lg.Debug("Gemini Live session budget extended", zap.Int64("increment_quota", amount),
		zap.Int64("reserved_quota", g.reserved), zap.Int64("committed_quota", g.committed))
	return nil
}

// Finish records demonstrably received unreceipted work. Parameters: evidence
// excludes allowances of turns that never started. Returns: none; unpriceable
// evidence conservatively keeps the whole reservation.
func (g *liveSpendGate) Finish(evidence realtime.Estimate) {
	cost, err := g.priceEstimate(evidence)
	g.mu.Lock()
	defer g.mu.Unlock()
	if err != nil {
		g.lg.Warn("Gemini Live unreceipted work is unpriceable; keeping the reservation", zap.Error(err))
		g.evidence = g.reserved
		return
	}
	g.evidence = cost
}

// settlement returns the funding state for the final ledger. Parameters: none.
// Returns: the total reservation and the estimate floor for missing receipts:
// at least the admission reservation and the priced evidence, but never more
// than the prepaid reservation, so estimates never create debt.
func (g *liveSpendGate) settlement() realtimeReservation {
	g.mu.Lock()
	defer g.mu.Unlock()
	floor := min(max(g.initial, min(g.committed+g.evidence, math.MaxInt64/2)), g.reserved)
	metadata := model.LogMetadata{"realtime_budget_reserved": g.reserved, "realtime_budget_top_ups": g.topUps}
	if g.exhausted {
		metadata["realtime_budget_exhausted"] = true
		if g.unfunded > 0 {
			metadata["realtime_budget_unfunded_estimate"] = g.unfunded
		}
	}
	return realtimeReservation{Reserved: g.reserved, EstimateFloor: floor, Metadata: metadata}
}
