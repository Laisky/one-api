package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	"github.com/Laisky/one-api/relay/state"
)

// ctxPendingStateCommit stashes the pre-hydration snapshot needed to commit a
// gateway response node at render time. It must be captured BEFORE hydration so
// the stored input items are the current turn's incremental input, not the
// hydrated full transcript (which would double-count on the next chain walk).
const ctxPendingStateCommit = "response_state_pending_commit"

// pendingStateCommit is the snapshot captured before hydration.
type pendingStateCommit struct {
	owner          state.OwnerScope
	inputItems     []state.ItemEnvelope
	instructions   *string
	parentID       string
	conversationID string
	storeMode      bool
	requestedModel string
	binding        state.ProviderBinding
	requestID      string
}

// capturePendingStateCommit records the information needed to commit a response
// node later. It is a no-op when the feature is inactive. store defaults to true
// when omitted (R6/A05).
func capturePendingStateCommit(c *gin.Context, meta *metalib.Meta, req *openai.ResponseAPIRequest) {
	if !responseStateActive(meta) || req == nil {
		return
	}
	owner := stateOwnerFromMeta(meta)
	if !owner.Valid() {
		return
	}

	items := make([]state.ItemEnvelope, 0, len(req.Input))
	for _, raw := range req.Input {
		env, err := inputItemToEnvelope(raw)
		if err != nil {
			continue
		}
		items = append(items, env)
	}

	prevID := ""
	if req.PreviousResponseId != nil {
		prevID = strings.TrimSpace(*req.PreviousResponseId)
	}

	c.Set(ctxPendingStateCommit, &pendingStateCommit{
		owner:          owner,
		inputItems:     items,
		instructions:   req.Instructions,
		parentID:       prevID,
		conversationID: req.Conversation.ConversationID(),
		storeMode:      req.Store == nil || *req.Store,
		requestedModel: req.Model,
		binding: state.ProviderBinding{
			ChannelID:   meta.ChannelId,
			APIType:     meta.APIType,
			ActualModel: meta.ActualModelName,
		},
		requestID: c.GetString(ctxkey.RequestId),
	})
}

// pendingCommitFromContext returns the captured commit snapshot, if any.
func pendingCommitFromContext(c *gin.Context) *pendingStateCommit {
	if v, ok := c.Get(ctxPendingStateCommit); ok {
		if commit, ok := v.(*pendingStateCommit); ok {
			return commit
		}
	}
	return nil
}

// inputItemToEnvelope builds a lossless envelope from a Response API input item,
// which is either a bare string (a user message) or a typed object.
func inputItemToEnvelope(item any) (state.ItemEnvelope, error) {
	switch v := item.(type) {
	case string:
		return state.NewStringInputEnvelope(v)
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return state.ItemEnvelope{}, errors.Wrap(err, "marshal input item")
		}
		return state.NewItemEnvelope(raw, "client")
	}
}

// commitResult carries the outcome of a state commit for the renderer.
type commitResult struct {
	gatewayID    string
	output       []openai.OutputItem
	storeMode    bool
	conversation string
	committed    bool
}

// commitFallbackResponseNode commits a response node for a fallback-generated
// response and returns the gateway ID plus the output items stamped with their
// gateway item IDs. On any store failure after a completed upstream it returns
// committed=false so the caller falls back to a synthetic ID and still returns
// the response; the model usage is billed regardless and the failure is logged
// (BIL06). When store=false no shared record is written (A06/SEC06).
func commitFallbackResponseNode(c *gin.Context, commit *pendingStateCommit, output []openai.OutputItem, usage *openai.ResponseAPIUsage, status string) commitResult {
	result := commitResult{output: output, storeMode: commit.storeMode, conversation: commit.conversationID}
	if !state.Enabled() || state.Store() == nil {
		return result
	}
	lg := gmw.GetLogger(c)
	if !commit.storeMode {
		// store=false: honor the explicit no-persistence contract.
		metrics.RecordStateEvent(metrics.StateCategoryCommit, metrics.StateOutcomeNoStore)
		return result
	}

	store := state.Store()

	// Stamp each output item with a stable gateway item ID and build envelopes.
	stampedOutput := make([]openai.OutputItem, len(output))
	copy(stampedOutput, output)
	outEnvs := make([]state.ItemEnvelope, 0, len(output))
	for i := range stampedOutput {
		raw, err := json.Marshal(stampedOutput[i])
		if err != nil {
			continue
		}
		env, err := state.NewItemEnvelope(raw, "openai")
		if err != nil {
			continue
		}
		stampedOutput[i].Id = env.GatewayItemID
		// Re-marshal so the stored raw carries the gateway item ID too.
		if reRaw, err := json.Marshal(stampedOutput[i]); err == nil {
			env.Raw = reRaw
		}
		outEnvs = append(outEnvs, env)
	}

	gwID, err := state.NewResponseID()
	if err != nil {
		lg.Error("mint gateway response id failed", zap.Error(err))
		return result
	}

	parentID := ""
	if state.LooksLikeGatewayResponseID(commit.parentID) {
		parentID = commit.parentID
	}

	var usageRaw json.RawMessage
	if usage != nil {
		if b, err := json.Marshal(usage); err == nil {
			usageRaw = b
		}
	}

	rec := &state.ResponseStateRecord{
		GatewayResponseID: gwID,
		Owner:             commit.owner,
		CreatedAt:         time.Now().Unix(),
		Status:            status,
		ParentResponseID:  parentID,
		ConversationID:    commit.conversationID,
		InputItems:        commit.inputItems,
		Instructions:      commit.instructions,
		RequestedModel:    commit.requestedModel,
		Tools:             nil,
		StoreMode:         true,
		OutputItems:       outEnvs,
		Usage:             usageRaw,
		CompletionStatus:  status,
		Binding:           &commit.binding,
		ExpiresAt:         time.Now().Add(state.ResponseTTLFromConfig()).Unix(),
	}

	// The one-api request ID makes the commit idempotent under retries (S05/F02).
	ctx := detachedCommitContext(c)
	committed, err := store.CreateResponse(ctx, rec, commit.requestID)
	if err != nil {
		metrics.RecordStateEvent(metrics.StateCategoryCommit, metrics.StateOutcomeCommitFailed)
		lg.Error("commit fallback response state failed; returning synthetic id",
			zap.Error(err), zap.String("request_id", commit.requestID))
		return result
	}
	metrics.RecordStateEvent(metrics.StateCategoryCommit, metrics.StateOutcomeCommitted)

	// Attach input+output to a conversation when one was selected (CON02).
	if commit.conversationID != "" {
		appendItems := append(append([]state.ItemEnvelope{}, commit.inputItems...), outEnvs...)
		if _, err := store.AppendConversationItems(ctx, commit.owner, commit.conversationID, state.AnyVersion, appendItems, commit.requestID); err != nil {
			lg.Warn("append conversation items failed", zap.Error(err), zap.String("request_id", commit.requestID))
		}
	}

	result.gatewayID = committed.GatewayResponseID
	result.output = stampedOutput
	result.committed = true
	return result
}

// detachedCommitContext returns a non-cancelled context for a state commit so a
// client disconnect after upstream completion does not abort the commit (STR07).
//
// Safe ONLY while state commits run synchronously inside the request handler (as
// they do today): it derives from the live request context. Before ever moving a
// commit into a goroutine that outlives the handler, switch to the relayctx.Detach
// helpers (as the deferred billing path does), so the goroutine never retains a
// recycled *gin.Context (ST-023).
func detachedCommitContext(c *gin.Context) context.Context {
	return context.WithoutCancel(gmw.Ctx(c))
}

// fingerprintID returns a short, non-reversible fingerprint of an identifier for
// logs and traces. Raw upstream provider IDs must never appear in logs (OBS07);
// the fingerprint keeps entries correlatable without exposing the provider id.
func fingerprintID(id string) string {
	if id == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:6])
}

// commitWebSocketObservedResponses records the store!=false completed responses
// observed on a native Responses websocket so their upstream IDs are retrievable
// over HTTP afterwards (proposal ST-011, Section 5.9). The native passthrough
// leaves connection-local store=false state to the upstream; only store=true
// responses reach this function. Each record is keyed by (and idempotent on) the
// raw upstream response ID, so a reconnect that re-observes the same response does
// not create a duplicate node. Commit failures are logged, never fatal to the
// already-completed socket session.
func commitWebSocketObservedResponses(c *gin.Context, meta *metalib.Meta, responses []*openai.ResponseAPIResponse) {
	if len(responses) == 0 || !responseStateActive(meta) {
		return
	}
	owner := stateOwnerFromMeta(meta)
	if !owner.Valid() {
		return
	}
	store := state.Store()
	if store == nil {
		return
	}
	lg := gmw.GetLogger(c)
	// Bound the synchronous commit so a slow/hung shared store cannot block the
	// post-websocket billing path indefinitely (review finding). The context is
	// detached from request cancellation so a client disconnect after upstream
	// completion does not abort the commit (STR07).
	ctx, cancel := context.WithTimeout(detachedCommitContext(c), 5*time.Second)
	defer cancel()

	for _, resp := range responses {
		if resp == nil || resp.Id == "" {
			continue
		}

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
			Owner:             owner,
			CreatedAt:         time.Now().Unix(),
			Status:            status,
			RequestedModel:    resp.Model,
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
		// Idempotent by the upstream response ID so a socket reconnect that
		// re-observes the same completed response does not double-write (WS04, S05).
		if _, err := store.CreateResponse(ctx, rec, resp.Id); err != nil {
			metrics.RecordStateEvent(metrics.StateCategoryCommit, metrics.StateOutcomeCommitFailed)
			// Log a short hash of the upstream id, never the raw provider id (OBS07).
			lg.Warn("commit websocket observed response failed",
				zap.Error(err), zap.String("upstream_response_fp", fingerprintID(resp.Id)))
			continue
		}
		metrics.RecordStateEvent(metrics.StateCategoryCommit, metrics.StateOutcomeCommitted)
	}
}

// commitResponseNodeWithID commits a response node under a caller-supplied gateway
// response ID. The streaming bridge uses this because it must pre-mint the ID at
// stream start so every SSE event carries the same gateway ID that the committed
// record and the final response.completed object use (STR01). The commit is
// idempotent by the one-api request ID, so a duplicate terminal event does not
// create a second node or double-append a conversation (STR05). Returns true on a
// successful commit.
func commitResponseNodeWithID(c *gin.Context, commit *pendingStateCommit, gatewayID string, output []openai.OutputItem, usage *openai.ResponseAPIUsage, status string) bool {
	if commit == nil || !commit.storeMode || !state.Enabled() || state.Store() == nil {
		return false
	}
	lg := gmw.GetLogger(c)
	store := state.Store()

	outEnvs := make([]state.ItemEnvelope, 0, len(output))
	for i := range output {
		raw, err := json.Marshal(output[i])
		if err != nil {
			continue
		}
		env, err := state.NewItemEnvelope(raw, "openai")
		if err != nil {
			continue
		}
		outEnvs = append(outEnvs, env)
	}

	parentID := ""
	if state.LooksLikeGatewayResponseID(commit.parentID) {
		parentID = commit.parentID
	}
	var usageRaw json.RawMessage
	if usage != nil {
		if b, err := json.Marshal(usage); err == nil {
			usageRaw = b
		}
	}

	rec := &state.ResponseStateRecord{
		GatewayResponseID: gatewayID,
		Owner:             commit.owner,
		CreatedAt:         time.Now().Unix(),
		Status:            status,
		ParentResponseID:  parentID,
		ConversationID:    commit.conversationID,
		InputItems:        commit.inputItems,
		Instructions:      commit.instructions,
		RequestedModel:    commit.requestedModel,
		StoreMode:         true,
		OutputItems:       outEnvs,
		Usage:             usageRaw,
		CompletionStatus:  status,
		Binding:           &commit.binding,
		ExpiresAt:         time.Now().Add(state.ResponseTTLFromConfig()).Unix(),
	}

	ctx := detachedCommitContext(c)
	if _, err := store.CreateResponse(ctx, rec, commit.requestID); err != nil {
		metrics.RecordStateEvent(metrics.StateCategoryCommit, metrics.StateOutcomeCommitFailed)
		lg.Error("commit streamed response state failed", zap.Error(err), zap.String("request_id", commit.requestID))
		return false
	}
	metrics.RecordStateEvent(metrics.StateCategoryCommit, metrics.StateOutcomeCommitted)
	if commit.conversationID != "" {
		appendItems := append(append([]state.ItemEnvelope{}, commit.inputItems...), outEnvs...)
		if _, err := store.AppendConversationItems(ctx, commit.owner, commit.conversationID, state.AnyVersion, appendItems, commit.requestID); err != nil {
			lg.Warn("append conversation items failed", zap.Error(err), zap.String("request_id", commit.requestID))
		}
	}
	return true
}
