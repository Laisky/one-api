package controller

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/state"
)

// stateOwnerFromGin derives the owner scope from the authenticated token context.
func stateOwnerFromGin(c *gin.Context) state.OwnerScope {
	return state.OwnerScope{UserID: c.GetInt(ctxkey.Id), TokenID: c.GetInt(ctxkey.TokenId)}
}

// conversationsAvailable reports whether the Conversations API can serve this
// owner. It gates on the feature flag and the allowlist so a disabled deployment
// keeps current behavior (the endpoints report not-found).
func conversationsAvailable(c *gin.Context) (state.ResponseStateStore, state.OwnerScope, *relaymodel.ErrorWithStatusCode) {
	owner := stateOwnerFromGin(c)
	if !state.Enabled() || state.Store() == nil || !state.AllowedFor(owner.UserID, owner.TokenID, 0) {
		return nil, owner, stateErrorf("not_found", http.StatusNotFound, "conversations api is not enabled")
	}
	if !owner.Valid() {
		return nil, owner, stateErrorf("unauthorized", http.StatusUnauthorized, "authentication required")
	}
	return state.Store(), owner, nil
}

// mapConversationStoreError converts a store error into the documented API error.
func mapConversationStoreError(err error) *relaymodel.ErrorWithStatusCode {
	switch {
	case errors.Is(err, state.ErrNotFound):
		return stateErrorf(codeConversationNotFound, http.StatusNotFound, "conversation not found")
	case errors.Is(err, state.ErrVersionConflict):
		return stateErrorf(codeConversationConflict, http.StatusConflict, "conversation was modified concurrently")
	case errors.Is(err, state.ErrLeaseHeld):
		return stateErrorf(codeConversationConflict, http.StatusConflict, "conversation is locked by another request")
	case errors.Is(err, state.ErrLimitExceeded):
		return stateErrorf(codeStateLimitExceeded, http.StatusRequestEntityTooLarge, "conversation exceeds configured limits")
	case errors.Is(err, state.ErrStoreUnavailable):
		return stateErrorf(codeStateStoreUnavailable, http.StatusServiceUnavailable, "state store unavailable")
	default:
		return stateErrorf("state_error", http.StatusInternalServerError, "conversation operation failed: %v", err)
	}
}

// conversationCreateBody is the POST /v1/conversations request body.
type conversationCreateBody struct {
	Metadata json.RawMessage `json:"metadata,omitempty"`
	Items    []any           `json:"items,omitempty"`
}

// ConversationCreateHelper handles POST /v1/conversations (V01).
func ConversationCreateHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	store, owner, gwErr := conversationsAvailable(c)
	if gwErr != nil {
		return gwErr
	}

	var body conversationCreateBody
	if len(c.GetHeader("Content-Type")) > 0 {
		_ = json.NewDecoder(c.Request.Body).Decode(&body)
	}

	items, gwErr := itemsToEnvelopes(body.Items)
	if gwErr != nil {
		return gwErr
	}

	id, err := state.NewConversationID()
	if err != nil {
		return stateErrorf("state_error", http.StatusInternalServerError, "mint conversation id: %v", err)
	}
	rec := &state.ConversationStateRecord{
		GatewayConversationID: id,
		Owner:                 owner,
		Version:               0,
		Items:                 items,
		Metadata:              body.Metadata,
	}
	created, err := store.CreateConversation(gmw.Ctx(c), rec, "")
	if err != nil {
		return mapConversationStoreError(err)
	}
	return writeJSON(c, http.StatusOK, renderConversation(created))
}

// ConversationGetHelper handles GET /v1/conversations/{id} (V02).
func ConversationGetHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	store, owner, gwErr := conversationsAvailable(c)
	if gwErr != nil {
		return gwErr
	}
	rec, err := store.GetConversation(gmw.Ctx(c), owner, c.Param("conversation_id"))
	if err != nil {
		return mapConversationStoreError(err)
	}
	return writeJSON(c, http.StatusOK, renderConversation(rec))
}

// ConversationUpdateHelper handles POST /v1/conversations/{id} (V03).
func ConversationUpdateHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	store, owner, gwErr := conversationsAvailable(c)
	if gwErr != nil {
		return gwErr
	}
	var body struct {
		Metadata json.RawMessage `json:"metadata,omitempty"`
	}
	_ = json.NewDecoder(c.Request.Body).Decode(&body)
	rec, err := store.UpdateConversationMetadata(gmw.Ctx(c), owner, c.Param("conversation_id"), state.AnyVersion, body.Metadata)
	if err != nil {
		return mapConversationStoreError(err)
	}
	return writeJSON(c, http.StatusOK, renderConversation(rec))
}

// ConversationDeleteHelper handles DELETE /v1/conversations/{id} (V04).
func ConversationDeleteHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	store, owner, gwErr := conversationsAvailable(c)
	if gwErr != nil {
		return gwErr
	}
	id := c.Param("conversation_id")
	if err := store.DeleteConversation(gmw.Ctx(c), owner, id); err != nil {
		return mapConversationStoreError(err)
	}
	return writeJSON(c, http.StatusOK, map[string]any{"id": id, "object": "conversation.deleted", "deleted": true})
}

// ConversationItemsCreateHelper handles POST /v1/conversations/{id}/items (V05).
func ConversationItemsCreateHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	store, owner, gwErr := conversationsAvailable(c)
	if gwErr != nil {
		return gwErr
	}
	var body struct {
		Items []any `json:"items"`
	}
	_ = json.NewDecoder(c.Request.Body).Decode(&body)
	items, gwErr := itemsToEnvelopes(body.Items)
	if gwErr != nil {
		return gwErr
	}
	rec, err := store.AppendConversationItems(gmw.Ctx(c), owner, c.Param("conversation_id"), state.AnyVersion, items, "")
	if err != nil {
		return mapConversationStoreError(err)
	}
	// Return only the newly appended items in order.
	appended := rec.Items
	if len(appended) >= len(items) {
		appended = appended[len(appended)-len(items):]
	}
	return writeJSON(c, http.StatusOK, renderItemList(appended))
}

// ConversationItemsListHelper handles GET /v1/conversations/{id}/items (V06).
func ConversationItemsListHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	store, owner, gwErr := conversationsAvailable(c)
	if gwErr != nil {
		return gwErr
	}
	rec, err := store.GetConversation(gmw.Ctx(c), owner, c.Param("conversation_id"))
	if err != nil {
		return mapConversationStoreError(err)
	}

	items := rec.Items
	if order := c.Query("order"); order == "desc" {
		items = reverseEnvelopes(items)
	}
	if after := c.Query("after"); after != "" {
		items = itemsAfter(items, after)
	}
	limit := 20
	if l := c.Query("limit"); l != "" {
		if n, convErr := strconv.Atoi(l); convErr == nil && n > 0 {
			limit = n
		}
	}
	hasMore := false
	if len(items) > limit {
		items = items[:limit]
		hasMore = true
	}
	payload := renderItemList(items)
	payload["has_more"] = hasMore
	return writeJSON(c, http.StatusOK, payload)
}

// ConversationItemGetHelper handles GET /v1/conversations/{id}/items/{item_id} (V07).
func ConversationItemGetHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	store, owner, gwErr := conversationsAvailable(c)
	if gwErr != nil {
		return gwErr
	}
	rec, err := store.GetConversation(gmw.Ctx(c), owner, c.Param("conversation_id"))
	if err != nil {
		return mapConversationStoreError(err)
	}
	itemID := c.Param("item_id")
	for _, env := range rec.Items {
		if env.GatewayItemID == itemID || env.UpstreamItemID == itemID {
			return writeJSON(c, http.StatusOK, renderItem(env))
		}
	}
	return stateErrorf(codeConversationNotFound, http.StatusNotFound, "conversation item not found")
}

// ConversationItemDeleteHelper handles DELETE /v1/conversations/{id}/items/{item_id} (V08).
func ConversationItemDeleteHelper(c *gin.Context) *relaymodel.ErrorWithStatusCode {
	store, owner, gwErr := conversationsAvailable(c)
	if gwErr != nil {
		return gwErr
	}
	itemID := c.Param("item_id")
	if _, err := store.DeleteConversationItem(gmw.Ctx(c), owner, c.Param("conversation_id"), itemID, state.AnyVersion); err != nil {
		return mapConversationStoreError(err)
	}
	return writeJSON(c, http.StatusOK, map[string]any{"id": itemID, "object": "conversation.item.deleted", "deleted": true})
}

// --- rendering helpers ------------------------------------------------------

func renderConversation(rec *state.ConversationStateRecord) map[string]any {
	out := map[string]any{
		"id":         rec.GatewayConversationID,
		"object":     "conversation",
		"created_at": rec.CreatedAt,
	}
	if len(rec.Metadata) > 0 {
		var meta any
		if err := json.Unmarshal(rec.Metadata, &meta); err == nil {
			out["metadata"] = meta
		}
	}
	return out
}

func renderItem(env state.ItemEnvelope) map[string]any {
	var item map[string]any
	if err := json.Unmarshal(env.Raw, &item); err != nil || item == nil {
		item = map[string]any{}
	}
	item["id"] = env.GatewayItemID
	return item
}

func renderItemList(items []state.ItemEnvelope) map[string]any {
	data := make([]map[string]any, 0, len(items))
	for _, env := range items {
		data = append(data, renderItem(env))
	}
	out := map[string]any{"object": "list", "data": data}
	if len(items) > 0 {
		out["first_id"] = items[0].GatewayItemID
		out["last_id"] = items[len(items)-1].GatewayItemID
	}
	return out
}

func itemsToEnvelopes(items []any) ([]state.ItemEnvelope, *relaymodel.ErrorWithStatusCode) {
	out := make([]state.ItemEnvelope, 0, len(items))
	for _, raw := range items {
		env, err := inputItemToEnvelope(raw)
		if err != nil {
			return nil, stateErrorf(codeInvalidStateSelector, http.StatusBadRequest, "invalid conversation item")
		}
		out = append(out, env)
	}
	return out, nil
}

func reverseEnvelopes(items []state.ItemEnvelope) []state.ItemEnvelope {
	out := make([]state.ItemEnvelope, len(items))
	for i, env := range items {
		out[len(items)-1-i] = env
	}
	return out
}

func itemsAfter(items []state.ItemEnvelope, afterID string) []state.ItemEnvelope {
	for i, env := range items {
		if env.GatewayItemID == afterID || env.UpstreamItemID == afterID {
			if i+1 < len(items) {
				return items[i+1:]
			}
			return nil
		}
	}
	return items
}

// writeJSON writes a JSON body and returns nil so handlers can `return writeJSON(...)`.
func writeJSON(c *gin.Context, status int, body any) *relaymodel.ErrorWithStatusCode {
	c.JSON(status, body)
	return nil
}
