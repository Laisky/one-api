package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGPT6CyberNativeResponseTariff checks final HTTP admission after model
// mapping, preserving compatible-provider controls. Parameter t asserts; none return.
func TestGPT6CyberNativeResponseTariff(t *testing.T) {
	for _, tc := range []struct {
		name, controls, errorText string
	}{
		{"default", "", ""},
		{"explicit_standard", `,"service_tier":"default"`, ""},
		{"fast", `,"service_tier":"fast"`, "Standard"},
		{"auto", `,"service_tier":"auto"`, "Standard"},
		{"flex", `,"service_tier":"flex"`, "Standard"},
		{"short_cache", `,"prompt_cache_options":{"ttl":"30m"}`, ""},
		{"long_cache", `,"prompt_cache_options":{"ttl":"12h"}`, "12h"},
		{"tool_cache", `,"tools":[{"type":"function","name":"lookup","parameters":{},"prompt_cache_breakpoint":{"ttl":"12h"}}]`, "12h"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(`{"model":"customer-alias","input":"hello"` + tc.controls + `}`)
			var request openai.ResponseAPIRequest
			require.NoError(t, json.Unmarshal(raw, &request))
			request.Model = "gpt-6-cyber"
			wire, _, _, err := normalizeResponseAPIRawBody(raw, &request, channeltype.OpenAI)
			if tc.errorText != "" {
				require.ErrorContains(t, err, tc.errorText)
				return
			}
			require.NoError(t, err)
			var payload map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(wire, &payload))
			require.Equal(t, `"gpt-6-cyber"`, string(payload["model"]))
			require.Equal(t, `"default"`, string(payload["service_tier"]))
		})
	}
	var other map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(`{"service_tier":"fast"}`), &other))
	changed, err := normalizeOpenAITerraStandardPricing(other, "gpt-6-cyber", channeltype.OpenAICompatible)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, `"fast"`, string(other["service_tier"]))
}

// TestGPT6CyberWebSocketRequiresQualifiedBilling rejects an upgrade that would
// bypass HTTP tariff admission. Parameter t reports failures; nothing returns.
func TestGPT6CyberWebSocketRequiresQualifiedBilling(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses?model=gpt-6-cyber", nil)
	c.Request.Header.Set("Upgrade", "websocket")
	c.Request.Header.Set("Connection", "Upgrade")
	info := &metalib.Meta{ChannelType: channeltype.OpenAI, ActualModelName: "gpt-6-cyber",
		BaseURL: "https://api.openai.com", Mode: relaymode.ResponseAPI}
	handled, err := maybeHandleResponseAPIWebSocket(c, info)
	require.True(t, handled)
	require.NotNil(t, err)
	require.Equal(t, http.StatusBadRequest, err.StatusCode)
	require.Equal(t, "response_websocket_model_billing_not_supported", err.Code)
}
