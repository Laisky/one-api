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
// gateway state store. It always returns handled=true with a local response or
// error; missing authorization storage never permits provider passthrough.
func serveGatewayResponseGet(c *gin.Context, meta *metalib.Meta, responseID string) (bool, *relaymodel.ErrorWithStatusCode) {
	if !state.Enabled() || state.Store() == nil {
		return true, stateErrorf(codeStateStoreUnavailable, http.StatusServiceUnavailable, "response ownership storage is unavailable")
	}
	owner := stateOwnerFromMeta(meta)
	if !owner.Valid() {
		return true, stateErrorf("response_not_found", http.StatusNotFound, "response not found")
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
		return true, stateErrorf(codeStateStoreUnavailable, http.StatusServiceUnavailable, "response ownership storage is unavailable")
	}
	owner := stateOwnerFromMeta(meta)
	if !owner.Valid() {
		return true, stateErrorf("response_not_found", http.StatusNotFound, "response not found")
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
// IDs return a non-disclosing not-found error regardless of legacy configuration.
func serveGatewayResponseCancel(c *gin.Context, meta *metalib.Meta, responseID string) (bool, *relaymodel.ErrorWithStatusCode) {
	if !state.Enabled() || state.Store() == nil {
		return true, stateErrorf(codeStateStoreUnavailable, http.StatusServiceUnavailable, "response ownership storage is unavailable")
	}
	owner := stateOwnerFromMeta(meta)
	if !owner.Valid() {
		return true, stateErrorf("response_not_found", http.StatusNotFound, "response not found")
	}
	_, err := state.Store().GetResponse(gmw.Ctx(c), owner, responseID)
	if err == nil {
		return true, openai.ErrorWrapper(
			errors.New("this response cannot be cancelled: only a background response still owned by its upstream provider supports cancellation"),
			"invalid_operation", http.StatusBadRequest)
	}
	return handleGatewayLookupMiss(c, responseID, err)
}

// handleGatewayLookupMiss maps a store lookup error to a definitive local error
// for the supplied request and identifier; it never permits upstream dispatch.
func handleGatewayLookupMiss(c *gin.Context, responseID string, err error) (bool, *relaymodel.ErrorWithStatusCode) {
	if errors.Is(err, state.ErrNotFound) {
		// An owner-scoped miss never authorizes raw provider passthrough.
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

// codeConversationNotFoundToResponse returns the stable response lookup error code.
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
