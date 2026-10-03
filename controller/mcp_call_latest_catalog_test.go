package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/model"
)

func TestExecuteModernMCPToolReusesPreparedCatalog(t *testing.T) {
	cleanup, fixture := setupMCPProxyTest(t)
	defer cleanup()

	toolQueries := 0
	const callbackName = "test:modern-mcp-tool-call-reuses-prepared-catalog"
	err := model.DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx != nil && tx.Statement != nil && tx.Statement.Table == "mcp_tools" {
			toolQueries++
		}
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, model.DB.Callback().Query().Remove(callbackName))
	}()

	c, _ := newMCPCallContext(t, fixture.user.Id, "modern-catalog-reuse")
	result, err := executeModernMCPTool(context.Background(), c, modernMCPCallParams{
		Name:      "fake-mcp.echo",
		Arguments: map[string]any{"message": "hello"},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, fixture.upstreamHits)
	require.Equal(t, 1, toolQueries, "modern tools/call must reuse one prepared tool-catalog snapshot")
}
