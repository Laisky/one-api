package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common/metrics"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/apitype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/state"
)

// hydrationTarget names the format a resolved turn is being lowered to. It
// determines which provider-bound items degrade versus fail (Section 5.8).
type hydrationTarget int

const (
	targetChatFallback hydrationTarget = iota
	targetClaudeFallback
)

// label returns a stable, content-free name for the target used in portability
// error messages and metrics.
func (t hydrationTarget) label() string {
	if t == targetClaudeFallback {
		return "claude"
	}
	return "chat"
}

// Stable machine-readable state error codes (Section 6).
const (
	codeInvalidStateSelector    = "invalid_state_selector"
	codePreviousResponseMissing = "previous_response_not_found"
	codeConversationNotFound    = "conversation_not_found"
	codeConversationConflict    = "conversation_conflict"
	codeStateNotPortable        = "state_not_portable"
	codeStateLimitExceeded      = "state_limit_exceeded"
	codeStateStoreUnavailable   = "state_store_unavailable"
)

// stateErrorf builds a typed error-with-status for a state failure. 4xx codes log
// at WARN without a stack trace; 5xx at ERROR (handled by the caller); the error
// is always wrapped with github.com/Laisky/errors/v2 per repository convention.
func stateErrorf(code string, status int, format string, args ...any) *relaymodel.ErrorWithStatusCode {
	return openai.ErrorWrapper(errors.Errorf(format, args...), code, status)
}

// stateOwnerFromMeta derives the owner scope used for every state lookup.
func stateOwnerFromMeta(meta *metalib.Meta) state.OwnerScope {
	return state.OwnerScope{UserID: meta.UserId, TokenID: meta.TokenId}
}

// responseFallbackTarget reports whether a Responses request routed through the
// Chat fallback actually lowers to Claude Messages (an Anthropic-family upstream),
// so the hydrator applies the Claude column of the portability table (ST-008). The
// enforcement is identical for both targets today; the distinction drives the
// error label and reserves a seam for a future native Responses-to-Claude path.
func responseFallbackTarget(meta *metalib.Meta) hydrationTarget {
	if meta == nil {
		return targetChatFallback
	}
	switch meta.APIType {
	case apitype.Anthropic, apitype.AwsClaude:
		return targetClaudeFallback
	case apitype.VertexAI:
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(meta.ActualModelName)), "claude") {
			return targetClaudeFallback
		}
	}
	return targetChatFallback
}

// responseStateActive reports whether gateway state should be applied for this
// request: the feature is enabled and the owner/channel is in the allowlist.
func responseStateActive(meta *metalib.Meta) bool {
	if !state.Enabled() || state.Store() == nil {
		return false
	}
	return state.AllowedFor(meta.UserId, meta.TokenId, meta.ChannelId)
}

// hydrateResponseAPIRequestForFallback resolves state selectors and lowers the
// referenced state for a Chat/Claude fallback route. It returns a clone of req
// whose Input holds the fully resolved effective transcript (prior items followed
// by the current input, with item_reference items resolved). Instructions are NOT
// inherited from a parent (R4). When no state applies it returns req unchanged.
func hydrateResponseAPIRequestForFallback(ctx context.Context, meta *metalib.Meta, req *openai.ResponseAPIRequest, target hydrationTarget) (*openai.ResponseAPIRequest, *relaymodel.ErrorWithStatusCode) {
	if req == nil {
		return nil, stateErrorf(codeInvalidStateSelector, http.StatusBadRequest, "nil response api request")
	}
	if !responseStateActive(meta) {
		// Feature disabled or out of allowlist: current behavior, no hydration.
		return req, nil
	}

	store := state.Store()
	owner := stateOwnerFromMeta(meta)
	if !owner.Valid() {
		// Without an owner we cannot safely resolve state; leave the request as-is.
		return req, nil
	}
	limits := state.LimitsFromConfig()

	prevID := ""
	if req.PreviousResponseId != nil {
		prevID = strings.TrimSpace(*req.PreviousResponseId)
	}
	convID := req.Conversation.ConversationID()

	if prevID != "" && convID != "" {
		return nil, stateErrorf(codeInvalidStateSelector, http.StatusBadRequest,
			"conversation and previous_response_id are mutually exclusive")
	}

	// Resolve prior context.
	var priorItems []any
	switch {
	case prevID != "":
		items, serr := hydratePreviousResponseChain(ctx, store, owner, limits, prevID)
		if serr != nil {
			return nil, serr
		}
		priorItems = items
		metrics.RecordStateEvent(metrics.StateCategoryPath, metrics.StateOutcomeHydrated)
	case convID != "":
		conv, err := store.GetConversation(ctx, owner, convID)
		if err != nil {
			if errors.Is(err, state.ErrNotFound) {
				metrics.RecordStateEvent(metrics.StateCategoryMiss, metrics.StateOutcomeNotFound)
				return nil, stateErrorf(codeConversationNotFound, http.StatusNotFound, "conversation not found")
			}
			metrics.RecordStateEvent(metrics.StateCategoryMiss, metrics.StateOutcomeStoreError)
			return nil, stateErrorf(codeStateStoreUnavailable, http.StatusServiceUnavailable, "load conversation: %v", err)
		}
		priorItems = envelopesToItems(conv.Items)
		metrics.RecordStateEvent(metrics.StateCategoryPath, metrics.StateOutcomeConversation)
	default:
		metrics.RecordStateEvent(metrics.StateCategoryPath, metrics.StateOutcomeStateless)
	}

	combined := make([]any, 0, len(priorItems)+len(req.Input))
	combined = append(combined, priorItems...)
	combined = append(combined, []any(req.Input)...)

	// Resolve item_reference items anywhere in the effective turn — the hydrated
	// prior context AND the current input — under owner scope. Prior items are the
	// turn's incremental input stored pre-resolution, so a stored reference must be
	// resolved here too, otherwise it reaches lowering unresolved and degrades into
	// an empty message (review finding). An unresolvable reference fails closed with
	// invalid_state_selector (P05).
	combined, serr := resolveItemReferences(ctx, store, owner, combined)
	if serr != nil {
		return nil, serr
	}

	if limits.MaxItemCount > 0 && len(combined) > limits.MaxItemCount {
		return nil, stateErrorf(codeStateLimitExceeded, http.StatusRequestEntityTooLarge,
			"hydrated item count %d exceeds limit %d", len(combined), limits.MaxItemCount)
	}

	// Lower the resolved items for the target format. Portable items pass through;
	// reasoning/thinking degrade to a display-only sidecar (dropped rather than
	// becoming empty messages — the B05 fix); hosted/built-in tool-call state and
	// unknown item types fail closed with state_not_portable (ST-008: P04, I05).
	lowered, lowErr := lowerItemsForTarget(combined, target)
	if lowErr != nil {
		return nil, lowErr
	}

	clone := *req
	clone.Input = openai.ResponseAPIInput(lowered)
	return &clone, nil
}

// hydratePreviousResponseChain walks the parent chain oldest-first and returns the
// reconstructed prior transcript items. Each node stores only its incremental
// input plus its output, so concatenating input+output across the chain rebuilds
// the full history without duplication (R5).
func hydratePreviousResponseChain(ctx context.Context, store state.ResponseStateStore, owner state.OwnerScope, limits state.Limits, headID string) ([]any, *relaymodel.ErrorWithStatusCode) {
	var chain []*state.ResponseStateRecord
	seen := map[string]struct{}{}
	cur := headID
	depth := 0
	for cur != "" {
		depth++
		if limits.MaxChainDepth > 0 && depth > limits.MaxChainDepth {
			return nil, stateErrorf(codeStateLimitExceeded, http.StatusRequestEntityTooLarge,
				"response chain depth exceeds limit %d", limits.MaxChainDepth)
		}
		if _, dup := seen[cur]; dup {
			break
		}
		seen[cur] = struct{}{}

		rec, err := store.GetResponse(ctx, owner, cur)
		if err != nil {
			if errors.Is(err, state.ErrNotFound) {
				// Absent, expired, deleted, or foreign-owned parents all return the
				// same external error (E03).
				return nil, stateErrorf(codePreviousResponseMissing, http.StatusBadRequest,
					"previous response not found")
			}
			return nil, stateErrorf(codeStateStoreUnavailable, http.StatusServiceUnavailable, "load response chain: %v", err)
		}
		chain = append(chain, rec)
		cur = rec.ParentResponseID
	}

	// chain is head-first (newest-first); emit oldest-first.
	var items []any
	for i := len(chain) - 1; i >= 0; i-- {
		rec := chain[i]
		items = append(items, envelopesToItems(rec.InputItems)...)
		items = append(items, envelopesToItems(rec.OutputItems)...)
	}
	return items, nil
}

// resolveItemReferences replaces item_reference items with the referenced stored
// item's raw payload. A reference that cannot be resolved under owner scope is an
// invalid_state_selector with an external shape identical for unknown and
// foreign-owned item IDs (P05).
func resolveItemReferences(ctx context.Context, store state.ResponseStateStore, owner state.OwnerScope, input []any) ([]any, *relaymodel.ErrorWithStatusCode) {
	out := make([]any, 0, len(input))
	for _, item := range input {
		m, ok := item.(map[string]any)
		if !ok {
			out = append(out, item)
			continue
		}
		typeVal, _ := m["type"].(string)
		if !strings.EqualFold(strings.TrimSpace(typeVal), state.KindItemReference) {
			out = append(out, item)
			continue
		}
		refID, _ := m["id"].(string)
		if refID == "" {
			return nil, stateErrorf(codeInvalidStateSelector, http.StatusBadRequest, "item_reference missing id")
		}
		env, err := store.GetItem(ctx, owner, refID)
		if err != nil {
			if errors.Is(err, state.ErrNotFound) {
				return nil, stateErrorf(codeInvalidStateSelector, http.StatusBadRequest, "referenced item not found")
			}
			return nil, stateErrorf(codeStateStoreUnavailable, http.StatusServiceUnavailable, "resolve item reference: %v", err)
		}
		resolved, decodeErr := rawToAny(env.Raw)
		if decodeErr != nil {
			return nil, stateErrorf(codeInvalidStateSelector, http.StatusBadRequest, "decode referenced item")
		}
		out = append(out, resolved)
	}
	return out, nil
}

// lowerItemsForTarget lowers resolved canonical items onto a stateless Chat/Claude
// fallback route using the authoritative portability decision in
// state.FallbackLowering (Section 5.8): portable items pass through; reasoning and
// thinking degrade to a display-only sidecar (dropped so they never become empty
// messages — the B05 fix); hosted/built-in tool-call state and unknown item types
// fail closed with state_not_portable, naming the blocking item kind without
// exposing content (E05). The target only affects the error label; both fallback
// families apply the same table because neither Chat nor Claude Messages can
// faithfully carry OpenAI-hosted tool state or an unknown Responses item type.
func lowerItemsForTarget(items []any, target hydrationTarget) ([]any, *relaymodel.ErrorWithStatusCode) {
	out := make([]any, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			// Bare strings are user messages: always portable.
			out = append(out, item)
			continue
		}
		raw, err := json.Marshal(m)
		if err != nil {
			// A value we cannot even re-encode is treated as portable; the downstream
			// converter will surface any real problem.
			out = append(out, item)
			continue
		}
		action, kind := state.FallbackLowering(raw)
		switch action {
		case state.FallbackActionCarry:
			out = append(out, item)
		case state.FallbackActionDrop:
			// Sidecar: opaque provider-bound state dropped; the request proceeds.
			metrics.RecordStateEvent(metrics.StateCategoryPortability, metrics.StateOutcomeSidecarDropped)
		default: // state.FallbackActionFail
			metrics.RecordStateEvent(metrics.StateCategoryPortability, metrics.StateOutcomeNotPortable)
			return nil, stateErrorf(codeStateNotPortable, http.StatusConflict,
				"state item %q cannot be represented on the %s fallback route", kind, target.label())
		}
	}
	return out, nil
}

// envelopesToItems converts stored item envelopes back into raw input items.
func envelopesToItems(envelopes []state.ItemEnvelope) []any {
	out := make([]any, 0, len(envelopes))
	for _, env := range envelopes {
		v, err := rawToAny(env.Raw)
		if err != nil || v == nil {
			continue
		}
		out = append(out, v)
	}
	return out
}

func rawToAny(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, errors.Wrap(err, "decode raw item")
	}
	return v, nil
}
