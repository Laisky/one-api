package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/state"
)

// TestConversationCreatePerUserCapRejects covers ST-019 rows L07/V12 end-to-end
// through the Conversations API: creating beyond the per-user cap fails with
// state_limit_exceeded (413), and existing conversations are unaffected (no silent
// eviction).
func TestConversationCreatePerUserCapRejects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := state.NewMemoryStore(state.Limits{MaxConversationsPerUser: 1, MaxItemCount: 2048, MaxRecordBytes: 8 << 20})
	state.SetForTest(store)
	t.Cleanup(func() { state.SetForTest(nil) })

	newCtx := func() *gin.Context {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/conversations", strings.NewReader(`{}`))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(ctxkey.Id, 1)
		c.Set(ctxkey.TokenId, 1)
		return c
	}

	require.Nil(t, ConversationCreateHelper(newCtx()), "first conversation is under the cap")

	gwErr := ConversationCreateHelper(newCtx())
	require.NotNil(t, gwErr, "second conversation exceeds the per-user cap")
	require.Equal(t, http.StatusRequestEntityTooLarge, gwErr.StatusCode)
	require.Equal(t, codeStateLimitExceeded, gwErr.Code)
}

// TestResponseStateCancelGatewayResolution covers ST-017: the cancel handler now
// resolves gateway records first. Cancelling a fallback-generated (gateway)
// response returns the documented invalid-operation error (row C12), and an
// unknown id is not forwarded upstream when legacy passthrough is off (rows R08,
// SEC04).
func TestResponseStateCancelGatewayResolution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := enableStateForTest(t)
	meta := testMeta()

	id, err := state.NewResponseID()
	require.NoError(t, err)
	seedResponse(t, store, &state.ResponseStateRecord{
		GatewayResponseID: id,
		Owner:             testOwner(),
		Status:            state.StatusCompleted,
		RequestedModel:    "gpt-5",
		StoreMode:         true,
	})

	// C12: a gateway-committed response cannot be cancelled; documented 400.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "response_id", Value: id}}
	handled, gwErr := serveGatewayResponseCancel(c, meta, id)
	require.True(t, handled)
	require.NotNil(t, gwErr)
	require.Equal(t, http.StatusBadRequest, gwErr.StatusCode)

	// R08/SEC04: unknown id, passthrough OFF → handled locally as not-found, never
	// forwarded upstream.
	unknown := "resp_" + strings.Repeat("a", 32)
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Params = gin.Params{{Key: "response_id", Value: unknown}}
	handled, gwErr = serveGatewayResponseCancel(c2, meta, unknown)
	require.True(t, handled)
	require.NotNil(t, gwErr)
	require.Equal(t, http.StatusNotFound, gwErr.StatusCode)
}

// TestResponseStateCancelLegacyPassthrough covers the R08 passthrough-on half plus
// the ST-018 tombstone rule: an unknown id is forwarded upstream when passthrough
// is on, but a DELETED gateway id is tombstoned and must never be forwarded
// (row S06).
func TestResponseStateCancelLegacyPassthrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := state.NewMemoryStore(state.DefaultLimits())
	state.SetForTest(store, state.WithLegacyPassthrough(true))
	t.Cleanup(func() { state.SetForTest(nil) })
	meta := testMeta()

	// R08: unknown id, passthrough ON → fall through to the legacy upstream forward.
	unknown := "resp_" + strings.Repeat("b", 32)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "response_id", Value: unknown}}
	handled, gwErr := serveGatewayResponseCancel(c, meta, unknown)
	require.False(t, handled)
	require.Nil(t, gwErr)

	// S06/ST-018: a deleted gateway id is tombstoned; even with passthrough ON it is
	// answered locally as not-found and never forwarded upstream.
	id, err := state.NewResponseID()
	require.NoError(t, err)
	_, err = store.CreateResponse(context.Background(), &state.ResponseStateRecord{
		GatewayResponseID: id,
		Owner:             testOwner(),
		Status:            state.StatusCompleted,
		StoreMode:         true,
	}, "")
	require.NoError(t, err)
	require.NoError(t, store.DeleteResponse(context.Background(), testOwner(), id))

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Params = gin.Params{{Key: "response_id", Value: id}}
	handled, gwErr = serveGatewayResponseGet(c2, meta, id)
	require.True(t, handled)
	require.NotNil(t, gwErr)
	require.Equal(t, http.StatusNotFound, gwErr.StatusCode)

	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	c3.Params = gin.Params{{Key: "response_id", Value: id}}
	handled, gwErr = serveGatewayResponseCancel(c3, meta, id)
	require.True(t, handled)
	require.NotNil(t, gwErr)
	require.Equal(t, http.StatusNotFound, gwErr.StatusCode)
}

// TestResponseStateHydratedByteLimitEnforced covers ST-020 row L03: an oversized
// hydrated transcript is rejected with state_limit_exceeded before lowering or any
// upstream call.
func TestResponseStateHydratedByteLimitEnforced(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := enableStateForTest(t)
	meta := testMeta()

	orig := config.ResponseStateMaxHydratedBytes
	config.ResponseStateMaxHydratedBytes = 64
	t.Cleanup(func() { config.ResponseStateMaxHydratedBytes = orig })

	big := strings.Repeat("x", 500)
	parent := seedResponse(t, store, &state.ResponseStateRecord{
		GatewayResponseID: mustNewResponseID(t),
		Owner:             testOwner(),
		Status:            state.StatusCompleted,
		StoreMode:         true,
		InputItems:        []state.ItemEnvelope{mustEnv(t, `{"type":"message","role":"user","content":"`+big+`"}`)},
	})

	prevID := parent.GatewayResponseID
	req := &openai.ResponseAPIRequest{
		Model:              "gpt-5",
		PreviousResponseId: &prevID,
		Input:              openai.ResponseAPIInput{"hi"},
	}
	_, serr := hydrateResponseAPIRequestForFallback(context.Background(), meta, req, targetChatFallback)
	require.NotNil(t, serr)
	require.Equal(t, http.StatusRequestEntityTooLarge, serr.StatusCode)
}

func mustNewResponseID(t *testing.T) string {
	t.Helper()
	id, err := state.NewResponseID()
	require.NoError(t, err)
	return id
}

// countingItemStore wraps a store and counts GetItem calls so a test can prove
// how much work an unresolved request is able to drive.
type countingItemStore struct {
	state.ResponseStateStore
	getItemCalls int
}

// GetItem records the call and delegates.
//
// Parameters:
//   - ctx: request context.
//   - owner: the owner scope.
//   - itemID: the referenced item id.
//
// Return values:
//   - *state.ItemEnvelope: the delegated envelope.
//   - error: the delegated error.
func (s *countingItemStore) GetItem(ctx context.Context, owner state.OwnerScope, itemID string) (*state.ItemEnvelope, error) {
	s.getItemCalls++
	return s.ResponseStateStore.GetItem(ctx, owner, itemID)
}

// TestResponseStateItemCountLimitPrecedesReferenceResolution pins that the item
// count cap is enforced BEFORE item_reference fan-out.
//
// resolveItemReferences performs one store round-trip (Redis GET + AES-GCM
// decrypt + JSON decode) per input element, and the element list is taken
// verbatim from the request body. The cap used to run only after that loop, so a
// single request — which consumes no quota, because hydration happens before
// pre-consume — could drive an unbounded number of store reads and hold every
// decoded item in memory at once.
func TestResponseStateItemCountLimitPrecedesReferenceResolution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := enableStateForTest(t)
	meta := testMeta()

	orig := config.ResponseStateMaxItemCount
	config.ResponseStateMaxItemCount = 4
	t.Cleanup(func() { config.ResponseStateMaxItemCount = orig })

	counting := &countingItemStore{ResponseStateStore: store}
	state.SetForTest(counting)
	t.Cleanup(func() { state.SetForTest(nil) })

	input := make(openai.ResponseAPIInput, 0, 50)
	for i := 0; i < 50; i++ {
		input = append(input, map[string]any{"type": state.KindItemReference, "id": "item_does_not_exist"})
	}

	req := &openai.ResponseAPIRequest{Model: "gpt-5", Input: input}
	_, serr := hydrateResponseAPIRequestForFallback(context.Background(), meta, req, targetChatFallback)
	require.NotNil(t, serr)
	require.Equal(t, http.StatusRequestEntityTooLarge, serr.StatusCode)
	require.Zero(t, counting.getItemCalls,
		"the item cap must reject the request before any item_reference is resolved")
}
