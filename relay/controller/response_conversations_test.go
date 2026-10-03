package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/state"
)

// newConvContext builds a gin context authenticated as (userID, tokenID) with an
// optional JSON body and route params.
func newConvContext(t *testing.T, method, body string, userID, tokenID int, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var reader *bytes.Reader
	if body != "" {
		reader = bytes.NewReader([]byte(body))
	} else {
		reader = bytes.NewReader(nil)
	}
	c.Request = httptest.NewRequest(method, "/v1/conversations", reader)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(ctxkey.Id, userID)
	c.Set(ctxkey.TokenId, tokenID)
	c.Params = params
	return c, w
}

// TestConversationsCRUDLifecycle exercises create → get → append → list → item get
// → item delete → conversation delete (rows V01-V08).
func TestConversationsCRUDLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableStateForTest(t)

	// V01 create with an initial item and metadata.
	c, w := newConvContext(t, http.MethodPost,
		`{"metadata":{"topic":"weather"},"items":[{"type":"message","role":"user","content":"hi"}]}`, 1, 1, nil)
	require.Nil(t, ConversationCreateHelper(c))
	var created map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	convID, _ := created["id"].(string)
	require.True(t, state.LooksLikeGatewayConversationID(convID))
	require.Equal(t, "conversation", created["object"])

	params := gin.Params{{Key: "conversation_id", Value: convID}}

	// V02 get.
	c, w = newConvContext(t, http.MethodGet, "", 1, 1, params)
	require.Nil(t, ConversationGetHelper(c))
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Equal(t, convID, got["id"])

	// V05 append items.
	c, w = newConvContext(t, http.MethodPost,
		`{"items":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"sunny"}]}]}`, 1, 1, params)
	require.Nil(t, ConversationItemsCreateHelper(c))
	var appended map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &appended))
	data, _ := appended["data"].([]any)
	require.Len(t, data, 1)
	appendedItem, _ := data[0].(map[string]any)
	appendedItemID, _ := appendedItem["id"].(string)
	require.NotEmpty(t, appendedItemID)

	// V06 list — should now have 2 items.
	c, w = newConvContext(t, http.MethodGet, "", 1, 1, params)
	require.Nil(t, ConversationItemsListHelper(c))
	var listed map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listed))
	listData, _ := listed["data"].([]any)
	require.Len(t, listData, 2)

	// V07 get single item.
	c, w = newConvContext(t, http.MethodGet, "", 1, 1, gin.Params{
		{Key: "conversation_id", Value: convID}, {Key: "item_id", Value: appendedItemID},
	})
	require.Nil(t, ConversationItemGetHelper(c))
	var item map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &item))
	require.Equal(t, appendedItemID, item["id"])

	// V08 delete item.
	c, w = newConvContext(t, http.MethodDelete, "", 1, 1, gin.Params{
		{Key: "conversation_id", Value: convID}, {Key: "item_id", Value: appendedItemID},
	})
	require.Nil(t, ConversationItemDeleteHelper(c))

	// V04 delete conversation.
	c, w = newConvContext(t, http.MethodDelete, "", 1, 1, params)
	require.Nil(t, ConversationDeleteHelper(c))

	// Later get → conversation_not_found (V04 tombstone).
	c, w = newConvContext(t, http.MethodGet, "", 1, 1, params)
	gwErr := ConversationGetHelper(c)
	require.NotNil(t, gwErr)
	require.Equal(t, http.StatusNotFound, gwErr.StatusCode)
}

// TestConversationForeignOwnerIsNotFound verifies a conversation owned by another
// tenant is not found without existence disclosure (V10).
func TestConversationForeignOwnerIsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := enableStateForTest(t)

	id, err := state.NewConversationID()
	require.NoError(t, err)
	_, err = store.CreateConversation(nil, &state.ConversationStateRecord{
		GatewayConversationID: id,
		Owner:                 state.OwnerScope{UserID: 99, TokenID: 99},
	}, "")
	require.NoError(t, err)

	c, _ := newConvContext(t, http.MethodGet, "", 1, 1, gin.Params{{Key: "conversation_id", Value: id}})
	gwErr := ConversationGetHelper(c)
	require.NotNil(t, gwErr)
	require.Equal(t, http.StatusNotFound, gwErr.StatusCode)
}

// TestConversationsDisabledIsNotFound verifies the endpoints report not-found when
// the feature is disabled (row O01, V11 gating).
func TestConversationsDisabledIsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.False(t, state.Enabled())

	c, _ := newConvContext(t, http.MethodPost, `{}`, 1, 1, nil)
	gwErr := ConversationCreateHelper(c)
	require.NotNil(t, gwErr)
	require.Equal(t, http.StatusNotFound, gwErr.StatusCode)
}

// TestConversationCreateOversizedIsRejected verifies an oversized create is
// rejected with state_limit_exceeded before any store write (V12).
func TestConversationCreateOversizedIsRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := state.NewMemoryStore(state.Limits{MaxItemCount: 1})
	state.SetForTest(store)
	t.Cleanup(func() { state.SetForTest(nil) })

	c, _ := newConvContext(t, http.MethodPost,
		`{"items":[{"type":"message","role":"user","content":"a"},{"type":"message","role":"user","content":"b"}]}`, 1, 1, nil)
	gwErr := ConversationCreateHelper(c)
	require.NotNil(t, gwErr)
	require.Equal(t, http.StatusRequestEntityTooLarge, gwErr.StatusCode)
}
