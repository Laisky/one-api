package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/metrics"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/state"
)

// ctxNativeGatewayParent stashes the original (client-supplied) gateway parent id
// for a native Responses continuation, so the committed child node records the
// correct gateway chain even after the outgoing previous_response_id is rewritten
// to the upstream handle.
const ctxNativeGatewayParent = "response_state_native_parent"

// resolveNativePreviousResponse translates a gateway previous_response_id for a
// request routed to a native Responses upstream (Section 5.6 step 4, ST-021).
//
// It returns divert=true when the referenced state cannot be honored on the
// currently selected native provider, so the caller must fall back to the
// hydrating Chat/Claude path (canonical replay). Behavior:
//   - previous_response_id is not a gateway record (a raw/legacy upstream id):
//     leave it untouched and forward verbatim exactly as today.
//   - gateway record bound to the SAME native provider with an upstream handle:
//     rewrite previous_response_id to that upstream handle so only the incremental
//     input is sent (rows M05, PERF02) — one bounded binding lookup, no full-chain
//     deserialization.
//   - gateway record bound to a DIFFERENT provider (or with no usable handle):
//     divert to the hydrating fallback so canonical items are replayed (row C08).
func resolveNativePreviousResponse(c *gin.Context, meta *metalib.Meta, req *openai.ResponseAPIRequest) (bool, *relaymodel.ErrorWithStatusCode) {
	if !responseStateActive(meta) || req == nil || req.PreviousResponseId == nil {
		return false, nil
	}
	prevID := strings.TrimSpace(*req.PreviousResponseId)
	if prevID == "" {
		return false, nil
	}
	owner := stateOwnerFromMeta(meta)
	if !owner.Valid() {
		return false, nil
	}

	binding, err := state.Store().GetResponseBinding(gmw.Ctx(c), owner, prevID)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			// Not a gateway record: a raw/legacy upstream id the client already holds.
			// Forward it verbatim as today (row B07).
			return false, nil
		}
		// A required state lookup failed: fail closed with a retryable 503 rather than
		// forwarding a possibly-gateway id upstream (row R05/E06).
		metrics.RecordStateEvent(metrics.StateCategoryMiss, metrics.StateOutcomeStoreError)
		return false, stateErrorf(codeStateStoreUnavailable, http.StatusServiceUnavailable, "resolve previous response binding: %v", err)
	}

	// The parent is a confirmed gateway record; remember its gateway id for chaining.
	c.Set(ctxNativeGatewayParent, prevID)

	if binding != nil && binding.ChannelID == meta.ChannelId && binding.APIType == meta.APIType && binding.UpstreamResponseID != "" {
		// Same native provider: reuse the upstream handle and send only incremental
		// input. The rewritten typed field is synced into the raw outgoing body by
		// normalizeResponseAPIRawBody.
		upstream := binding.UpstreamResponseID
		req.PreviousResponseId = &upstream
		metrics.RecordStateEvent(metrics.StateCategoryAffinity, metrics.StateOutcomePinned)
		return false, nil
	}

	// Gateway parent bound to a different provider or lacking a usable handle: the
	// native provider cannot continue from it, so hydrate + replay on fallback.
	metrics.RecordStateEvent(metrics.StateCategoryAffinity, metrics.StateOutcomeUnpinned)
	return true, nil
}

// commitNativeResponseState commits the result of a native Responses upstream call
// to gateway state so its upstream id is retrievable over HTTP and can back a
// same-provider continuation or a stateless-client checkpoint (ST-021: STR01, M05,
// PERF02). It mirrors commitWebSocketObservedResponses: the record is keyed by (and
// idempotent on) the raw upstream response id, and its binding carries that id as
// the upstream handle. It is a no-op when the feature is inactive, store=false, or
// the completed response object is unavailable. Commit failures are logged, never
// fatal to the already-billed request.
func commitNativeResponseState(c *gin.Context, meta *metalib.Meta) {
	commit := pendingCommitFromContext(c)
	if commit == nil || !commit.storeMode || !state.Enabled() || state.Store() == nil {
		return
	}
	respVal, ok := c.Get(ctxkey.ConvertedResponse)
	if !ok {
		return
	}
	resp, ok := respVal.(openai.ResponseAPIResponse)
	if !ok || resp.Id == "" {
		return
	}

	lg := gmw.GetLogger(c)
	store := state.Store()

	outEnvs := make([]state.ItemEnvelope, 0, len(resp.Output))
	for i := range resp.Output {
		raw, err := json.Marshal(resp.Output[i])
		if err != nil {
			continue
		}
		env, err := state.NewItemEnvelope(raw, "openai")
		if err != nil {
			continue
		}
		outEnvs = append(outEnvs, env)
	}

	var usageRaw json.RawMessage
	if resp.Usage != nil {
		if b, err := json.Marshal(resp.Usage); err == nil {
			usageRaw = b
		}
	}
	status := resp.Status
	if status == "" {
		status = state.StatusCompleted
	}

	rec := &state.ResponseStateRecord{
		GatewayResponseID: resp.Id,
		Owner:             commit.owner,
		CreatedAt:         time.Now().Unix(),
		Status:            status,
		ParentResponseID:  c.GetString(ctxNativeGatewayParent),
		ConversationID:    commit.conversationID,
		InputItems:        commit.inputItems,
		Instructions:      commit.instructions,
		RequestedModel:    commit.requestedModel,
		StoreMode:         true,
		OutputItems:       outEnvs,
		Usage:             usageRaw,
		CompletionStatus:  status,
		Binding: &state.ProviderBinding{
			ChannelID:          meta.ChannelId,
			APIType:            meta.APIType,
			ActualModel:        meta.ActualModelName,
			UpstreamResponseID: resp.Id,
		},
		ExpiresAt: time.Now().Add(state.ResponseTTLFromConfig()).Unix(),
	}

	// Bound the synchronous commit and detach it from request cancellation so a
	// client disconnect after upstream completion still commits (STR07). The upstream
	// id is the idempotency key, so a retry that re-observes the same response does
	// not double-write (S05).
	ctx, cancel := context.WithTimeout(detachedCommitContext(c), 5*time.Second)
	defer cancel()
	if _, err := store.CreateResponse(ctx, rec, resp.Id); err != nil {
		metrics.RecordStateEvent(metrics.StateCategoryCommit, metrics.StateOutcomeCommitFailed)
		lg.Warn("commit native response state failed", zap.Error(err))
		return
	}
	metrics.RecordStateEvent(metrics.StateCategoryCommit, metrics.StateOutcomeCommitted)

	if commit.conversationID != "" {
		appendItems := append(append([]state.ItemEnvelope{}, commit.inputItems...), outEnvs...)
		if _, err := store.AppendConversationItems(ctx, commit.owner, commit.conversationID, state.AnyVersion, appendItems, resp.Id); err != nil {
			lg.Warn("append conversation items to gateway conversation failed", zap.Error(err))
		}
	}
}
