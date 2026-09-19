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
// dashboard handler, not merely the adaptor method. Parameters: t owns the
// test. Returns: none. Router AdminAuth remains unchanged; no IAM probe occurs.
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
	require.Contains(t, models, "gemini-3.8-live")
	require.Contains(t, models, "gemini-3.8-live-extended-thinking")
	require.Contains(t, models, "gemini-live-2.5-flash-native-audio")
	require.Contains(t, models, "gemini-3.8-flash")
}
