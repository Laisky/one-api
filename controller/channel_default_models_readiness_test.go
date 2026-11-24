package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/channeltype"
	relaycontroller "github.com/Laisky/one-api/relay/controller"
	"github.com/Laisky/one-api/relay/meta"
)

// TestDefaultChannelModelsNeedNoOperatorPricing pins the upgrade contract for
// unconfigured admins: every model a channel type lists by default (the
// /api/models/defaults list the channel editor fills a new channel from and a
// reset writes) must be servable with empty model_configs, so saving a default
// channel never raises a pricing warning and admission never refuses a default
// model for missing operator pricing. A model whose host has no qualified
// tariff belongs off the default list but stays in the full /api/models
// catalog, so operators may still add and price it. Parameters: t is the test
// handle. Returns: none.
func TestDefaultChannelModelsNeedNoOperatorPricing(t *testing.T) {
	require.NotEmpty(t, channelId2DefaultModels, "the default model lists must be initialized")
	types := make([]int, 0, len(channelId2DefaultModels))
	for channelType := range channelId2DefaultModels {
		types = append(types, channelType)
	}
	slices.Sort(types)
	at := time.Now().UTC()
	var gaps []string
	for _, channelType := range types {
		provider := relaycontroller.ResolveChannelPricingAdaptor(channelType)
		if provider == nil {
			continue
		}
		for _, gap := range relaycontroller.EvaluateChannelPricingReadiness(provider, channelId2DefaultModels[channelType], nil, nil, at) {
			gaps = append(gaps, fmt.Sprintf("%s(%d) %s: %s", channeltype.IdToName(channelType), channelType, gap.Model, gap.Requirement))
		}
	}
	require.Empty(t, gaps, "default channel models must not need operator pricing")

	// The full catalog is every adaptor's raw list (some adaptors build it
	// from a map, so only membership is stable across calls), and each default
	// list is exactly the zero-config servable subsequence of that catalog.
	require.Len(t, channelId2Models, len(channelId2DefaultModels))
	for _, channelType := range types {
		catalog := channelId2Models[channelType]
		require.ElementsMatch(t, rawAdaptorModelList(t, channelType), catalog, "%s(%d) catalog must be the raw adaptor list", channeltype.IdToName(channelType), channelType)
		provider := relaycontroller.ResolveChannelPricingAdaptor(channelType)
		refused := map[string]bool{}
		if provider != nil {
			for _, gap := range relaycontroller.EvaluateChannelPricingReadiness(provider, catalog, nil, nil, at) {
				refused[gap.Model] = true
			}
		}
		want := slices.DeleteFunc(slices.Clone(catalog), func(name string) bool { return refused[name] })
		require.Equal(t, want, channelId2DefaultModels[channelType], "%s(%d) defaults must equal the zero-config servable catalog", channeltype.IdToName(channelType), channelType)
	}

	// Control: the filter is exercised. Known operator-priced models stay in
	// the full catalog (operators can still add and price them) but are kept
	// off the default lists.
	for channelType, name := range map[int]string{
		channeltype.Ali:       "qwen-audio-turbo",
		channeltype.Groq:      "minimaxai/minimax-m2.7",
		channeltype.VertextAI: "claude-fable-5-2",
		channeltype.Gemini:    "gemini-3.5-transcribe-live",
	} {
		require.Contains(t, channelId2Models[channelType], name, "%s must remain in the full catalog", name)
		require.NotContains(t, channelId2DefaultModels[channelType], name, "%s must not be a default model", name)
	}
}

// TestDashboardModelCatalogEndpoints drives both admin handlers: /api/models
// serves the full catalog (operator-priced models included, the documented
// contract) and /api/models/defaults serves the zero-config lists in the same
// shape. Parameters: t is the test handle. Returns: none.
func TestDashboardModelCatalogEndpoints(t *testing.T) {
	// fetch runs one handler. Parameters: path is the route and handler its
	// controller. Returns: the decoded data map.
	fetch := func(path string, handler gin.HandlerFunc) map[string][]string {
		t.Helper()
		router := gin.New()
		router.GET(path, handler)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, recorder.Code)
		var body struct {
			Success bool                `json:"success"`
			Message *string             `json:"message"`
			Data    map[string][]string `json:"data"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
		require.True(t, body.Success)
		require.NotNil(t, body.Message)
		require.Empty(t, *body.Message)
		require.NotEmpty(t, body.Data)
		return body.Data
	}
	gin.SetMode(gin.TestMode)
	catalog := fetch("/api/models", DashboardListModels)
	defaults := fetch("/api/models/defaults", DashboardListDefaultModels)
	require.Equal(t, len(catalog), len(defaults), "both endpoints cover the same channel types")
	for key, models := range defaults {
		full, ok := catalog[key]
		require.True(t, ok, "channel type %s must be in the full catalog", key)
		require.Subset(t, full, models, "channel type %s defaults must come from its catalog", key)
	}
	groq := strconv.Itoa(channeltype.Groq)
	require.Contains(t, catalog[groq], "minimaxai/minimax-m2.7")
	require.NotContains(t, defaults[groq], "minimaxai/minimax-m2.7")
	require.ElementsMatch(t, rawAdaptorModelList(t, channeltype.Groq), catalog[groq])
}

// rawAdaptorModelList returns a channel type's unfiltered adaptor model list,
// built the way the catalog init builds it. Parameters: t is the test handle
// and channelType a channel type ID. Returns: the adaptor's model list.
func rawAdaptorModelList(t *testing.T, channelType int) []string {
	t.Helper()
	adaptor := relay.GetAdaptor(channeltype.ToAPIType(channelType))
	require.NotNil(t, adaptor)
	adaptor.Init(&meta.Meta{ChannelType: channelType})
	return adaptor.GetModelList()
}
