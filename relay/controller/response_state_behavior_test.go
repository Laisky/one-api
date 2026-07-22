package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/state"
)

// TestResponseStateBehaviorNativeRawForwardingPreservesSelectors verifies that
// the native Responses path retains state selectors from the raw request even
// though conversation is absent from the typed request DTO.
func TestResponseStateBehaviorNativeRawForwardingPreservesSelectors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		selector string
		key      string
	}{
		{name: "previous response", selector: `"previous_response_id":"resp_123"`, key: "previous_response_id"},
		{name: "conversation id", selector: `"conversation":"conv_123"`, key: "conversation"},
		{name: "conversation object", selector: `"conversation":{"id":"conv_123"}`, key: "conversation"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			raw := []byte(`{"model":"gpt-5",` + tt.selector + `,"input":"continue"}`)
			var request openai.ResponseAPIRequest
			require.NoError(t, json.Unmarshal(raw, &request))

			patched, _, _, err := normalizeResponseAPIRawBody(raw, &request, channeltype.OpenAI)
			require.NoError(t, err)

			var forwarded map[string]any
			require.NoError(t, json.Unmarshal(patched, &forwarded))
			require.Contains(t, forwarded, tt.key)
		})
	}
}

// enableStateForTest installs an in-memory state store and enables the feature
// for the duration of the test, resetting it afterwards. It returns the store so
// the test can seed parent responses/conversations. State-mutating tests must not
// run in parallel because the feature toggle is process-global.
func enableStateForTest(t *testing.T) *state.MemoryStore {
	t.Helper()
	store := state.NewMemoryStore(state.DefaultLimits())
	state.SetForTest(store)
	t.Cleanup(func() { state.SetForTest(nil) })
	return store
}

// TestResponseStateBehaviorDualSelectorsAreRejected verifies that with the state
// layer enabled, the mutually exclusive combination of conversation and
// previous_response_id is rejected at the one-api boundary before any upstream
// call. Closes B08.
func TestResponseStateBehaviorDualSelectorsAreRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableStateForTest(t)

	raw := []byte(`{
		"model":"gpt-5",
		"conversation":"conv_123",
		"previous_response_id":"resp_123",
		"input":"continue"
	}`)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")

	request, err := getAndValidateResponseAPIRequest(c)
	require.Nil(t, request)
	require.ErrorContains(t, err, "mutually exclusive")
}

// TestResponseStateBehaviorStateOnlyRequestsAreAccepted verifies that with the
// state layer enabled, a request carrying only a state selector (no input or
// prompt) is accepted, since the selector supplies the prior context. Closes B09.
func TestResponseStateBehaviorStateOnlyRequestsAreAccepted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableStateForTest(t)

	tests := []struct {
		name string
		body string
	}{
		{name: "previous response only", body: `{"model":"gpt-5","previous_response_id":"resp_123"}`},
		{name: "conversation id only", body: `{"model":"gpt-5","conversation":"conv_123"}`},
		{name: "conversation object only", body: `{"model":"gpt-5","conversation":{"id":"conv_123"}}`},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")

			request, err := getAndValidateResponseAPIRequest(c)
			require.NoError(t, err)
			require.NotNil(t, request)
		})
	}
}

// TestResponseStateBehaviorStateOnlyRejectedWhenDisabled verifies the disabled
// path preserves current behavior: a state-only request is rejected (row O01).
func TestResponseStateBehaviorStateOnlyRejectedWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.False(t, state.Enabled())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(`{"model":"gpt-5","previous_response_id":"resp_123"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	request, err := getAndValidateResponseAPIRequest(c)
	require.Nil(t, request)
	require.ErrorContains(t, err, "either input or prompt is required")
}

// TestResponseStateBehaviorFallbackReturnsUnresolvableSyntheticIDWhenDisabled
// verifies the disabled path preserves current behavior: a synthetic ID with no
// backing record, and no store/conversation on the response (row O01).
func TestResponseStateBehaviorFallbackReturnsUnresolvableSyntheticIDWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.False(t, state.Enabled())

	store := true
	firstRequest := &openai.ResponseAPIRequest{
		Model: "gpt-5",
		Input: openai.ResponseAPIInput{"remember alpha"},
		Store: &store,
	}
	firstResponse := &openai_compatible.SlimTextResponse{
		Choices: []openai_compatible.TextResponseChoice{
			{Message: relaymodel.Message{Role: "assistant", Content: "alpha"}, FinishReason: "stop"},
		},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(ctxkey.RequestId, "state-behavior-disabled")
	require.NoError(t, renderChatResponseAsResponseAPI(c, http.StatusOK, firstResponse, firstRequest,
		&metalib.Meta{OriginModelName: "gpt-5", ActualModelName: "gpt-5"}))

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	responseID, ok := response["id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, responseID)
	require.NotContains(t, response, "store")
	require.NotContains(t, response, "conversation")
}

// TestResponseStateBehaviorFallbackReturnsResolvableGatewayID verifies that with
// the state layer enabled, Chat fallback commits a gateway response node, returns
// a resolvable gateway ID, echoes store/conversation, and the ID hydrates prior
// context on the next request. Closes B10.
func TestResponseStateBehaviorFallbackReturnsResolvableGatewayID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := enableStateForTest(t)
	meta := testMeta()
	meta.OriginModelName = "gpt-5"
	meta.ActualModelName = "gpt-5"

	storeTrue := true
	firstRequest := &openai.ResponseAPIRequest{
		Model: "gpt-5",
		Input: openai.ResponseAPIInput{"remember alpha"},
		Store: &storeTrue,
	}
	firstResponse := &openai_compatible.SlimTextResponse{
		Choices: []openai_compatible.TextResponseChoice{
			{Message: relaymodel.Message{Role: "assistant", Content: "alpha"}, FinishReason: "stop"},
		},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(ctxkey.RequestId, "state-behavior-enabled")

	capturePendingStateCommit(c, meta, firstRequest)
	require.NoError(t, renderChatResponseAsResponseAPI(c, http.StatusOK, firstResponse, firstRequest, meta))

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	responseID, ok := response["id"].(string)
	require.True(t, ok)
	require.True(t, state.LooksLikeGatewayResponseID(responseID), "id %q must be a gateway id", responseID)
	require.Contains(t, response, "store")
	require.Equal(t, true, response["store"])

	// The committed record is resolvable and holds the transcript.
	rec, err := store.GetResponse(c, testOwner(), responseID)
	require.NoError(t, err)
	require.Len(t, rec.InputItems, 1)
	require.Len(t, rec.OutputItems, 1)

	// Continuing from that ID hydrates the prior context (not just current input).
	secondRequest := &openai.ResponseAPIRequest{
		Model:              "gpt-5",
		PreviousResponseId: &responseID,
		Input:              openai.ResponseAPIInput{"what did I ask you to remember?"},
	}
	hydrated, serr := hydrateResponseAPIRequestForFallback(c, meta, secondRequest, targetChatFallback)
	require.Nil(t, serr)
	converted, err := openai.ConvertResponseAPIToChatCompletionRequest(hydrated)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(converted.Messages), 3)
	require.Equal(t, "remember alpha", converted.Messages[0].StringContent())
	require.Equal(t, "alpha", converted.Messages[1].StringContent())
}

// TestGatewayResponseGetAndDelete verifies GET resolves a committed gateway
// response and DELETE tombstones it so a later GET is not found. Closes B14.
func TestGatewayResponseGetAndDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := enableStateForTest(t)
	meta := testMeta()

	id, err := state.NewResponseID()
	require.NoError(t, err)
	_, err = store.CreateResponse(context.Background(), &state.ResponseStateRecord{
		GatewayResponseID: id,
		Owner:             testOwner(),
		Status:            state.StatusCompleted,
		RequestedModel:    "gpt-5",
		StoreMode:         true,
		OutputItems:       []state.ItemEnvelope{mustEnv(t, `{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi"}]}`)},
	}, "")
	require.NoError(t, err)

	// GET resolves the gateway record.
	wGet := httptest.NewRecorder()
	cGet, _ := gin.CreateTestContext(wGet)
	cGet.Request = httptest.NewRequest(http.MethodGet, "/v1/responses/"+id, nil)
	cGet.Params = gin.Params{{Key: "response_id", Value: id}}
	handled, gwErr := serveGatewayResponseGet(cGet, meta, id)
	require.True(t, handled)
	require.Nil(t, gwErr)
	var got map[string]any
	require.NoError(t, json.Unmarshal(wGet.Body.Bytes(), &got))
	require.Equal(t, id, got["id"])

	// DELETE tombstones it.
	wDel := httptest.NewRecorder()
	cDel, _ := gin.CreateTestContext(wDel)
	cDel.Request = httptest.NewRequest(http.MethodDelete, "/v1/responses/"+id, nil)
	cDel.Params = gin.Params{{Key: "response_id", Value: id}}
	handled, gwErr = serveGatewayResponseDelete(cDel, meta, id)
	require.True(t, handled)
	require.Nil(t, gwErr)

	// A later GET returns not-found (feature enabled, passthrough off by default).
	wGet2 := httptest.NewRecorder()
	cGet2, _ := gin.CreateTestContext(wGet2)
	cGet2.Params = gin.Params{{Key: "response_id", Value: id}}
	handled, gwErr = serveGatewayResponseGet(cGet2, meta, id)
	require.True(t, handled)
	require.NotNil(t, gwErr)
	require.Equal(t, http.StatusNotFound, gwErr.StatusCode)
}
