package controller

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestResponseFallbackRendersWithAdmissionCatalog proves the renderers reuse
// the catalog admission built from the hydrated request instead of rebuilding
// one: a stored catalog wins over the request passed to the renderer, and a
// full fallback run leaves the admission catalog in the context.
func TestResponseFallbackRendersWithAdmissionCatalog(t *testing.T) {
	var admitted, other openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"m","tools":[{"type":"namespace","name":"mcp__docs","tools":[{"type":"function","name":"read_doc"}]}]}`), &admitted))
	require.NoError(t, json.Unmarshal([]byte(`{"model":"m","tools":[{"type":"function","name":"read_doc"}]}`), &other))
	catalog, err := openai.BuildResponseFallbackToolCatalog(&admitted)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(ctxkey.ResponseFallbackToolCatalog, catalog)
	require.Same(t, catalog, responseFallbackCatalogForRender(c, &other), "the stored admission catalog is authoritative")
	item := responseFallbackCatalogForRender(c, &other).ResponseFallbackToolCallItem("call_1", "mcp__docs__read_doc", `{}`)
	require.Equal(t, "mcp__docs", item.Namespace)
	require.Equal(t, "read_doc", item.Name)

	fresh, _ := gin.CreateTestContext(httptest.NewRecorder())
	rebuilt := responseFallbackCatalogForRender(fresh, &other)
	require.NotNil(t, rebuilt, "callers that bypass admission still get a catalog")
	require.NotSame(t, catalog, rebuilt)

	run, _, _, _, _ := codexFallbackRun(t, channeltype.OpenAICompatible, false, 1_000_000)
	stored, ok := run.Get(ctxkey.ResponseFallbackToolCatalog)
	require.True(t, ok, "admission stores its catalog for conversion and rendering")
	require.NotNil(t, stored.(*openai.ResponseFallbackToolCatalog))
}
