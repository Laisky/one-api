package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestMediaGenerationTariffDisplay checks that the public catalog displays the
// same per-generation USD amount used by admission and settlement. Parameters:
// t owns the fixture. Returns: none; token prices must not label paid music free.
func TestMediaGenerationTariffDisplay(t *testing.T) {
	setupModelsDisplayTestEnv(t)
	channel := &model.Channel{Name: "music-tariff-fixture", Type: channeltype.OpenRouter, Status: model.ChannelStatusEnabled, Models: "google/lyria-3-clip-preview,google/lyria-3-pro-preview", Group: "public"}
	require.NoError(t, model.DB.Create(channel).Error)
	router := gin.New()
	router.GET("/api/models/display", GetModelsDisplay)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/models/display", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var response ModelsDisplayResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.True(t, response.Success)
	channelKey := fmt.Sprintf("%s:%s", channeltype.IdToName(channel.Type), channel.Name)
	for name, price := range map[string]float64{"google/lyria-3-clip-preview": 0.04, "google/lyria-3-pro-preview": 0.08} {
		entry, exists := response.Data[channelKey].Models[name]
		require.True(t, exists)
		require.NotNil(t, entry.PerCallPricing)
		require.InDelta(t, price, entry.PerCallPricing.UsdPerCall, 1e-12)
		require.Zero(t, entry.InputPrice)
		require.Zero(t, entry.OutputPrice)
	}
}
