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
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
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

// TestResponseStateBehaviorDualSelectorsReachNativeUpstream verifies that the
// controller accepts and forwards the invalid combination of conversation and
// previous_response_id instead of rejecting it at the one-api boundary.
func TestResponseStateBehaviorDualSelectorsReachNativeUpstream(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

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
	require.NoError(t, err)
	require.NotNil(t, request.PreviousResponseId)

	patched, _, _, err := normalizeResponseAPIRawBody(raw, request, channeltype.OpenAI)
	require.NoError(t, err)
	var forwarded map[string]any
	require.NoError(t, json.Unmarshal(patched, &forwarded))
	require.Contains(t, forwarded, "conversation")
	require.Contains(t, forwarded, "previous_response_id")
}

// TestResponseStateBehaviorStateOnlyRequestsAreRejected verifies that the
// controller requires explicit input or a prompt even when a Responses state
// selector is present and could provide the prior context.
func TestResponseStateBehaviorStateOnlyRequestsAreRejected(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		body string
	}{
		{name: "previous response only", body: `{"model":"gpt-5","previous_response_id":"resp_123"}`},
		{name: "conversation only", body: `{"model":"gpt-5","conversation":"conv_123"}`},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")

			request, err := getAndValidateResponseAPIRequest(c)
			require.Nil(t, request)
			require.ErrorContains(t, err, "either input or prompt is required")
		})
	}
}

// TestResponseStateBehaviorFallbackReturnsUnresolvableSyntheticID verifies that
// Chat fallback returns a Responses-shaped identifier without retaining the
// output transcript needed to resolve that identifier on the next request.
func TestResponseStateBehaviorFallbackReturnsUnresolvableSyntheticID(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	store := true
	firstRequest := &openai.ResponseAPIRequest{
		Model: "gpt-5",
		Input: openai.ResponseAPIInput{"remember alpha"},
		Store: &store,
	}
	firstResponse := &openai_compatible.SlimTextResponse{
		Choices: []openai_compatible.TextResponseChoice{
			{
				Message:      relaymodel.Message{Role: "assistant", Content: "alpha"},
				FinishReason: "stop",
			},
		},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(ctxkey.RequestId, "state-behavior")
	require.NoError(t, renderChatResponseAsResponseAPI(
		c,
		http.StatusOK,
		firstResponse,
		firstRequest,
		&metalib.Meta{OriginModelName: "gpt-5", ActualModelName: "gpt-5"},
	))

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	responseID, ok := response["id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, responseID)
	require.NotContains(t, response, "store")
	require.NotContains(t, response, "conversation")

	secondRequest := &openai.ResponseAPIRequest{
		Model:              "gpt-5",
		PreviousResponseId: &responseID,
		Input:              openai.ResponseAPIInput{"what did I ask you to remember?"},
	}
	converted, err := openai.ConvertResponseAPIToChatCompletionRequest(secondRequest)
	require.NoError(t, err)
	require.Len(t, converted.Messages, 1)
	require.Equal(t, "what did I ask you to remember?", converted.Messages[0].StringContent())
}
