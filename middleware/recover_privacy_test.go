package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestRelayPanicRecoverSuppressesContent verifies panic diagnostics preserve
// status and timing metadata without reading or exporting request content.
func TestRelayPanicRecoverSuppressesContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, observed := observer.New(zapcore.DebugLevel)
	lg, err := glog.NewConsoleWithName("privacy", glog.LevelDebug,
		zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	router := gin.New()
	router.Use(func(c *gin.Context) { gmw.SetLogger(c, lg); c.Next() })
	router.Use(RelayPanicRecover())
	router.POST("/v1/:resource", func(*gin.Context) { panic("sentinel-panic-secret") })
	body := `{"messages":[{"content":"sentinel-prompt-secret"}],"api_key":"sentinel-body-key"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/sentinel-path-secret", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer sentinel-header-key")
	req.Header.Set("Cookie", "session=sentinel-cookie-secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.Len(t, observed.All(), 1)
	entry := observed.All()[0]
	encoded, err := json.Marshal(entry.ContextMap())
	require.NoError(t, err)
	for _, sentinel := range []string{"sentinel-panic-secret", "sentinel-prompt-secret", "sentinel-body-key", "sentinel-path-secret", "sentinel-header-key", "sentinel-cookie-secret"} {
		require.NotContains(t, string(encoded), sentinel)
	}
	require.Equal(t, int64(len(body)), entry.ContextMap()["body_bytes"])
	require.Equal(t, "/v1/:resource", entry.ContextMap()["route"])
	require.Equal(t, true, entry.ContextMap()["body_logging_suppressed"])
	require.NotContains(t, entry.ContextMap(), "request_body")
}
