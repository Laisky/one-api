package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
)

// TestVertexLiveDashboardModelCatalog exercises the actual administrative
// dashboard handler, not merely the adaptor method. Vertex Live models require
// operator realtime prices, so they stay in the adaptor catalog (operators may
// add and price them) but are not default suggestions: a channel filled from
// the defaults must be servable without operator pricing. Parameters: t owns
// the test. Returns: none. Router AdminAuth remains unchanged; no IAM probe occurs.
func TestVertexLiveDashboardModelCatalog(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/models", nil)
	DashboardListModels(c)
	var body struct {
		Success bool                `json:"success"`
		Data    map[string][]string `json:"data"`
	}
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.True(t, body.Success)
	models := body.Data[fmt.Sprint(channeltype.VertextAI)]
	raw := rawAdaptorModelList(t, channeltype.VertextAI)
	for _, name := range []string{"gemini-3.8-live", "gemini-live-2.5-flash-native-audio"} {
		require.Contains(t, raw, name, "%s stays addable from the adaptor catalog", name)
		require.NotContains(t, models, name, "%s needs operator realtime prices, so it is not a default suggestion", name)
	}
	for _, name := range vertexLegacyLiveModelIDs() {
		require.NotContains(t, models, name, "dashboard suggestions do not determine operator-configured eligibility")
	}
	require.Contains(t, models, "gemini-3.8-flash")
}

// TestVertexLiveDefaultPricingDoesNotBorrowDeveloperRates exercises the actual
// admin default-pricing handler. Parameters: t owns the test. Returns: none;
// model suggestions remain available without manufacturing backend prices.
func TestVertexLiveDefaultPricingDoesNotBorrowDeveloperRates(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/channel/default-pricing?type=%d", channeltype.VertextAI), nil)
	GetChannelDefaultPricing(c)
	var body struct {
		Success bool              `json:"success"`
		Data    map[string]string `json:"data"`
	}
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.True(t, body.Success)
	var prices map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(body.Data["model_configs"]), &prices))
	require.NotContains(t, prices, "gemini-3.8-live")
	require.NotContains(t, prices, "gemini-3.8-live-extended-thinking")
	require.Contains(t, prices, "gemini-3.8-flash")
}
