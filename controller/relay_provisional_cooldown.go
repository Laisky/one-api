package controller

import (
	"context"
	"net/http"
	"os"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"

	dbmodel "github.com/Laisky/one-api/model"
)

// relayFailureUsesAdaptiveCooldown reports whether processChannelRelayError will
// record p's failure on the adaptive cooldown ladder: it walks the same branch
// order as processChannelRelayError up to its suspendRelayAbility calls (429, or
// a 5xx the upstream did not mark as retryable, after one-api's own and
// caller-originated failures are exempted) and then applies suspendRelayAbility's
// adaptive eligibility (proven provider provenance, no legacy duration override,
// not a read-only call, dispatch timestamps present). Client cancellations are
// exempted by isUserOriginatedRelayError, before the 5xx branch. The drift guard
// TestRelayProvisionalCooldownEligibilityMatchesProcessor runs the real
// processor for every branch.
//
// Parameters:
//   - p: the snapshotted failure with its routing identity and timestamps.
//
// Returns:
//   - bool: true when the failure will advance the adaptive ladder.
func relayFailureUsesAdaptiveCooldown(p processChannelRelayErrorParams) bool {
	e := &p.Err
	if !e.FromUpstream || p.ReadOnly || p.StartedAt.IsZero() || p.CompletedAt.IsZero() {
		return false
	}
	if isInternalInfraError(e.RawError) || isAdaptorInternalError(e) || isUserOriginatedRelayError(e) {
		return false
	}
	var override string
	switch {
	case e.StatusCode == http.StatusTooManyRequests:
		override = "CHANNEL_SUSPEND_SECONDS_FOR_429"
	case e.StatusCode >= http.StatusInternalServerError && e.StatusCode <= 599 && !upstreamSuggestsRetry(e):
		override = "CHANNEL_SUSPEND_SECONDS_FOR_5XX"
	default:
		return false
	}
	_, configured := os.LookupEnv(override)
	return !configured
}

// publishRelayProvisionalCooldown hides the failed ability from this replica's
// cached routing for the first adaptive rung before the asynchronous processor
// persists the failure, for exactly the failures that processor records on the
// adaptive ladder. It runs synchronously on the request goroutine. The caller
// must invoke the returned release after processChannelRelayError finished, so
// the persisted outcome (applied, discarded, or failed) replaces the
// provisional rung.
//
// Parameters:
//   - ctx: the detached request context carrying the request-scoped logger.
//   - params: the snapshotted failure with its routing identity.
//
// Returns:
//   - func(): the idempotent release; a no-op when nothing was published.
func publishRelayProvisionalCooldown(ctx context.Context, params processChannelRelayErrorParams) func() {
	if !relayFailureUsesAdaptiveCooldown(params) {
		return func() {}
	}
	release, duration := dbmodel.PublishProvisionalAbilityCooldown(params.Group, params.OriginalModel, params.ChannelId)
	if duration > 0 {
		// The detached context's logger already carries the attempt's channel identity.
		lg := gmw.GetLogger(ctx)
		lg.Debug("published provisional ability cooldown ahead of persistence",
			zap.String("group", params.Group),
			zap.String("original_model", params.OriginalModel),
			zap.Int("status_code", params.Err.StatusCode),
			zap.Duration("provisional_cooldown", duration))
	}
	return release
}
