package controller

import (
	"encoding/json"
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/state"
)

// serveGatewayResponseGet attempts to satisfy a GET /v1/responses/:id from the
// gateway state store. It returns handled=true when it has fully answered the
// request (success or a definitive error); handled=false means the caller should
// fall through to the legacy upstream proxy (current behavior). It never forwards
// an unknown ID upstream when the feature is enabled and legacy passthrough is
// off (rows R08, SEC04).
func serveGatewayResponseGet(c *gin.Context, meta *metalib.Meta, responseID string) (bool, *relaymodel.ErrorWithStatusCode) {
	if !state.Enabled() || state.Store() == nil {
		return false, nil
	}
	owner := stateOwnerFromMeta(meta)
	if !owner.Valid() {
		return false, nil
	}
	rec, err := state.Store().GetResponse(gmw.Ctx(c), owner, responseID)
	if err == nil {
		if renderErr := renderStateRecordAsResponse(c, http.StatusOK, rec); renderErr != nil {
			return true, openai.ErrorWrapper(renderErr, "render_state_record_failed", http.StatusInternalServerError)
		}
		return true, nil
	}
	return handleGatewayLookupMiss(c, responseID, err)
}

// serveGatewayResponseDelete attempts to satisfy a DELETE from the gateway store.
func serveGatewayResponseDelete(c *gin.Context, meta *metalib.Meta, responseID string) (bool, *relaymodel.ErrorWithStatusCode) {
	if !state.Enabled() || state.Store() == nil {
		return false, nil
	}
	owner := stateOwnerFromMeta(meta)
	if !owner.Valid() {
		return false, nil
	}
	err := state.Store().DeleteResponse(gmw.Ctx(c), owner, responseID)
	if err == nil {
		body := map[string]any{"id": responseID, "object": "response.deleted", "deleted": true}
		c.JSON(http.StatusOK, body)
		return true, nil
	}
	return handleGatewayLookupMiss(c, responseID, err)
}

// serveGatewayResponseCancel resolves a cancel request against the gateway store
// before any upstream call (ST-017). A gateway-committed (fallback-generated)
// response is not a background upstream response, so it cannot be cancelled; the
// documented invalid-operation error is returned rather than forwarding a gateway
// ID upstream or pretending an upstream cancellation occurred (row C12). Unknown
// IDs follow the same passthrough/not-found policy as GET and DELETE, so a
// gateway-minted or deleted ID is never forwarded upstream when passthrough is off
// (rows R08, SEC04) — closing the hole where cancel skipped this check entirely.
func serveGatewayResponseCancel(c *gin.Context, meta *metalib.Meta, responseID string) (bool, *relaymodel.ErrorWithStatusCode) {
	if !state.Enabled() || state.Store() == nil {
		return false, nil
	}
	owner := stateOwnerFromMeta(meta)
	if !owner.Valid() {
		return false, nil
	}
	_, err := state.Store().GetResponse(gmw.Ctx(c), owner, responseID)
	if err == nil {
		return true, openai.ErrorWrapper(
			errors.New("this response cannot be cancelled: only a background response still owned by its upstream provider supports cancellation"),
			"invalid_operation", http.StatusBadRequest)
	}
	return handleGatewayLookupMiss(c, responseID, err)
}

// handleGatewayLookupMiss maps a store lookup error to the fall-through/not-found
// decision shared by GET, DELETE, and cancel.
func handleGatewayLookupMiss(c *gin.Context, responseID string, err error) (bool, *relaymodel.ErrorWithStatusCode) {
	if errors.Is(err, state.ErrNotFound) {
		// A tombstoned (deleted or LRU-evicted) gateway ID must never be forwarded
		// upstream, even in legacy passthrough mode: the tombstone prevents stale
		// fallback (row S06, ST-018).
		if store := state.Store(); store != nil {
			if dead, terr := store.ResponseTombstoned(gmw.Ctx(c), responseID); terr == nil && dead {
				return true, openai.ErrorWrapper(errors.New("response not found"), codeConversationNotFoundToResponse(), http.StatusNotFound)
			}
		}
		if state.LegacyPassthroughEnabled() {
			// Rollout compatibility: forward the unknown ID to the upstream exactly
			// as today (OpenAI-type channels only, enforced by the legacy handler).
			return false, nil
		}
		// Feature enabled, passthrough off: unknown/legacy IDs are not forwarded.
		return true, openai.ErrorWrapper(errors.New("response not found"), codeConversationNotFoundToResponse(), http.StatusNotFound)
	}
	if errors.Is(err, state.ErrStoreUnavailable) {
		return true, openai.ErrorWrapper(err, codeStateStoreUnavailable, http.StatusServiceUnavailable)
	}
	if errors.Is(err, state.ErrUnsupportedSchema) {
		return true, openai.ErrorWrapper(err, "state_schema_unsupported", http.StatusInternalServerError)
	}
	return true, openai.ErrorWrapper(err, "state_lookup_failed", http.StatusInternalServerError)
}

func codeConversationNotFoundToResponse() string { return "response_not_found" }

// renderStateRecordAsResponse reconstructs and writes a stored response node as a
// Responses API response object, with stable gateway IDs and the original output
// items (C10, I07).
func renderStateRecordAsResponse(c *gin.Context, status int, rec *state.ResponseStateRecord) error {
	lg := gmw.GetLogger(c)

	output := make([]openai.OutputItem, 0, len(rec.OutputItems))
	for _, env := range rec.OutputItems {
		var item openai.OutputItem
		if err := json.Unmarshal(env.Raw, &item); err != nil {
			lg.Warn("decode stored output item failed", zap.Error(err))
			continue
		}
		if item.Id == "" {
			item.Id = env.GatewayItemID
		}
		output = append(output, item)
	}

	response := openai.ResponseAPIResponse{
		Id:        rec.GatewayResponseID,
		Object:    "response",
		CreatedAt: rec.CreatedAt,
		Status:    rec.Status,
		Model:     rec.RequestedModel,
		Output:    output,
	}
	storeMode := rec.StoreMode
	response.Store = &storeMode
	if rec.Instructions != nil {
		response.Instructions = rec.Instructions
	}
	if rec.ConversationID != "" {
		response.Conversation = &openai.ResponseAPIConversation{Id: rec.ConversationID}
	}
	if len(rec.Usage) > 0 {
		var usage openai.ResponseAPIUsage
		if err := json.Unmarshal(rec.Usage, &usage); err == nil {
			response.Usage = &usage
		}
	}
	if rec.IncompleteReason != "" {
		response.IncompleteDetails = &openai.IncompleteDetails{Reason: rec.IncompleteReason}
	}
	if len(rec.ErrorMetadata) > 0 {
		var apiErr relaymodel.Error
		if err := json.Unmarshal(rec.ErrorMetadata, &apiErr); err == nil {
			response.Error = &apiErr
		}
	}

	data, err := json.Marshal(response)
	if err != nil {
		return errors.Wrap(err, "marshal state record response")
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(status)
	if _, err := c.Writer.Write(data); err != nil {
		return errors.Wrap(err, "write state record response")
	}
	return nil
}
