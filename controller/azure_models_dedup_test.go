package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestListAllModels_AzureAdaptorDoesNotDuplicateOrMislabel is a regression test for
// the Azure AI Foundry channel patch. Because channeltype.Azure gained its own
// apitype.Azure, the `for i := range apitype.Dummy` aggregation loop in init()
// started pulling azure.Adaptor.GetModelList() (= the entire OpenAI catalog PLUS
// the Foundry Claude models). That produced two defects visible on /v1/models:
//
//  1. Every OpenAI model was emitted a SECOND time (a duplicate "openai"-owned row),
//     because Azure's model list mirrors openai.ModelList.
//  2. Every Foundry Claude model was emitted with the WRONG owner "openai": the
//     aggregation loop builds a fresh, non-Init'd azure.Adaptor whose embedded
//     openai.Adaptor.ChannelType is 0, so GetChannelName() returns "openai" rather
//     than "azure", and the Claude ids collide with the OpenAI owner label.
//
// The other providers (openai-compatible channels, aws/vertex Claude) are already
// skipped or expected, so this test pins ONLY the Azure-attributable defect.
func TestListAllModels_AzureAdaptorDoesNotDuplicateOrMislabel(t *testing.T) {
	setupListModelsTestEnv(t)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/v1/models", ListAllModels)
	req, _ := http.NewRequest("GET", "/v1/models", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Object string `json:"object"`
		Data   []struct {
			Id      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "list", resp.Object)

	countByID := map[string]int{}
	claudeOwnedByOpenAI := make([]string, 0)
	for _, m := range resp.Data {
		countByID[m.Id]++
		if strings.HasPrefix(strings.ToLower(m.Id), "claude") && m.OwnedBy == "openai" {
			claudeOwnedByOpenAI = append(claudeOwnedByOpenAI, m.Id)
		}
	}

	// (1) OpenAI catalog must not be duplicated by the Azure adaptor. gpt-4o-mini is
	// exclusive to the OpenAI catalog, so its only legitimate source is apitype.OpenAI.
	require.Equal(t, 1, countByID["gpt-4o-mini"],
		"gpt-4o-mini must appear exactly once; a second copy means the Azure adaptor re-emitted the OpenAI catalog")

	// (2) A Foundry-only Claude model (not hosted by aws/vertex) must not be duplicated.
	// claude-mythos-5 is exclusive to the Anthropic catalog among the static apitypes.
	require.Equal(t, 1, countByID["claude-mythos-5"],
		"claude-mythos-5 must appear exactly once; a second copy means the Azure adaptor re-emitted the Foundry Claude catalog")

	// (3) No Claude model may be advertised as owned_by="openai" — only the non-Init'd
	// Azure adaptor mislabels Claude ids that way.
	require.Empty(t, claudeOwnedByOpenAI,
		"no Claude model should be owned_by=openai; the Azure adaptor mislabeled these Foundry Claude models: %v", claudeOwnedByOpenAI)
}
