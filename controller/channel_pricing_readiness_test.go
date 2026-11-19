package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
	relaycontroller "github.com/Laisky/one-api/relay/controller"
)

// channelPricingReadinessResponse is the decoded readiness response body.
type channelPricingReadinessResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Gaps []relaycontroller.ChannelPricingGap `json:"gaps"`
	} `json:"data"`
}

// postChannelPricingReadiness drives the production handler. Parameters: t
// owns assertions and body is the raw request body. Returns: the recorder
// and its decoded body.
func postChannelPricingReadiness(t *testing.T, body string) (*httptest.ResponseRecorder, channelPricingReadinessResponse) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/channel/pricing-readiness", CheckChannelPricingReadiness)
	w := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/channel/pricing-readiness", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, request)
	var response channelPricingReadinessResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response), w.Body.String())
	return w, response
}

// TestCheckChannelPricingReadinessReportsGaps verifies the API contract the
// channel editor consumes: the draft is encoded like the channel save payload,
// gaps name the supported model, its mapped upstream model, and the
// requirement, and a fully priced draft returns an empty list.
func TestCheckChannelPricingReadinessReportsGaps(t *testing.T) {
	body := fmt.Sprintf(`{"type":%d,"models":" gpt-4o,cyber ,gpt-image-2.6-flare,cyber","model_mapping":"{\"cyber\":\"gpt-6-cyber\"}","model_configs":null}`, channeltype.Azure)
	w, response := postChannelPricingReadiness(t, body)
	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, response.Success)
	got := make([]string, 0, len(response.Data.Gaps))
	for _, gap := range response.Data.Gaps {
		require.NotEmpty(t, gap.Message)
		got = append(got, gap.Model+">"+gap.UpstreamModel+":"+gap.Requirement)
	}
	require.Equal(t, []string{
		"cyber>gpt-6-cyber:" + relaycontroller.PricingRequirementTokenTariff,
		"gpt-image-2.6-flare>gpt-image-2.6-flare:" + relaycontroller.PricingRequirementImageFallback,
		"gpt-image-2.6-flare>gpt-image-2.6-flare:" + relaycontroller.PricingRequirementTokenTariff,
	}, got, "models are trimmed and deduplicated exactly like a saved channel's")

	configs, err := json.Marshal(map[string]any{
		"gpt-6-cyber":         map[string]any{"ratio": 2, "completion_ratio": 6},
		"gpt-image-2.6-flare": map[string]any{"ratio": 2, "completion_ratio": 10, "image": map[string]any{"price_per_image_usd": 0.04}},
	})
	require.NoError(t, err)
	priced, err := json.Marshal(map[string]any{
		"type": channeltype.Azure, "models": "gpt-4o,cyber,gpt-image-2.6-flare",
		"model_mapping": `{"cyber":"gpt-6-cyber"}`, "model_configs": string(configs),
	})
	require.NoError(t, err)
	w, response = postChannelPricingReadiness(t, string(priced))
	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, response.Success)
	require.NotNil(t, response.Data.Gaps, "a ready channel returns [] so the editor can tell it from a failure")
	require.Empty(t, response.Data.Gaps)
	require.Contains(t, w.Body.String(), `"gaps":[]`)
}

// TestCheckChannelPricingReadinessRejectsInvalidDrafts verifies bad or
// unbounded input is refused with HTTP 400 before any evaluation, so the
// editor fails open instead of warning on garbage.
func TestCheckChannelPricingReadinessRejectsInvalidDrafts(t *testing.T) {
	tooManyModels := strings.Repeat("m,", maxChannelPricingReadinessModels) + "m"
	oversized := fmt.Sprintf(`{"type":1,"models":"gpt-4o","model_configs":%q}`, strings.Repeat(" ", maxChannelPricingReadinessBodyBytes))
	for _, tc := range []struct {
		name, body, message string
	}{
		{name: "malformed_json", body: `{"type":1,`, message: "valid channel pricing readiness request"},
		{name: "empty_body", body: ``, message: "valid channel pricing readiness request"},
		{name: "unknown_field", body: `{"type":1,"models":"gpt-4o","key":"sk-secret"}`, message: "valid channel pricing readiness request"},
		{name: "wrong_type", body: `{"type":"azure","models":"gpt-4o"}`, message: "valid channel pricing readiness request"},
		{name: "trailing_data", body: `{"type":1,"models":"gpt-4o"}{}`, message: "Only one"},
		{name: "unknown_channel_type", body: `{"type":0,"models":"gpt-4o"}`, message: "Unsupported channel type"},
		{name: "out_of_range_channel_type", body: fmt.Sprintf(`{"type":%d,"models":"gpt-4o"}`, channeltype.Dummy), message: "Unsupported channel type"},
		{name: "too_many_models", body: fmt.Sprintf(`{"type":1,"models":%q}`, tooManyModels), message: "At most"},
		{name: "malformed_model_mapping", body: `{"type":1,"models":"gpt-4o","model_mapping":"{\"a\":1}"}`, message: "Model Mapping"},
		{name: "malformed_model_configs", body: `{"type":1,"models":"gpt-4o","model_configs":"[1]"}`, message: "Model Configs"},
		{name: "oversized_body", body: oversized, message: "must not exceed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, response := postChannelPricingReadiness(t, tc.body)
			require.Equal(t, http.StatusBadRequest, w.Code)
			require.False(t, response.Success)
			require.Contains(t, response.Message, tc.message)
			require.NotContains(t, w.Body.String(), "sk-secret", "the response must not echo request values")
		})
	}

	// The model cap is inclusive: exactly the limit is evaluated.
	atLimit := strings.Repeat("m,", maxChannelPricingReadinessModels-1) + "m"
	var buf bytes.Buffer
	require.NoError(t, json.NewEncoder(&buf).Encode(map[string]any{"type": channeltype.OpenAI, "models": atLimit}))
	w, response := postChannelPricingReadiness(t, buf.String())
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.True(t, response.Success)
}
