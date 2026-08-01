package common

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

	"github.com/Laisky/one-api/common/ctxkey"
)

// TestLogClientRequestPayload_OnceAndReusable verifies payload logging deduplicates per request and keeps body reusable.
func TestLogClientRequestPayload_OnceAndReusable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	payload := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	gmw.SetLogger(c, glog.Shared.Named("test"))

	err := LogClientRequestPayload(c, "chat_completions", 16)
	require.NoError(t, err)

	logged, ok := c.Get(ctxkey.ClientRequestPayloadLogged)
	require.True(t, ok)
	require.Equal(t, true, logged)

	type chatReq struct {
		Model string `json:"model"`
	}
	var req chatReq
	err = UnmarshalBodyReusable(c, &req)
	require.NoError(t, err)
	require.Equal(t, "gpt-4o", req.Model)

	err = LogClientRequestPayload(c, "chat_completions", 4)
	require.NoError(t, err)
}

// TestLogClientRequestPayloadDoesNotExposeContent verifies that request logs
// remain shape-only even when prompts, tool outputs, or credentials are present.
func TestLogClientRequestPayloadDoesNotExposeContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, observed := observer.New(zapcore.DebugLevel)
	lg, err := glog.NewConsoleWithName("test", glog.LevelDebug,
		zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	payload := `{"input":"sentinel-user-text","reasoning":"sentinel-reasoning","tool_output":"sentinel-file-content","headers":{"Authorization":"sentinel-credential"}}`
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(payload))
	gmw.SetLogger(c, lg)

	require.NoError(t, LogClientRequestPayload(c, "responses", DefaultLogBodyLimit))
	require.Len(t, observed.All(), 1)
	encoded, err := json.Marshal(observed.All()[0].ContextMap())
	require.NoError(t, err)
	logged := string(encoded)
	require.NotContains(t, logged, "sentinel-user-text")
	require.NotContains(t, logged, "sentinel-reasoning")
	require.NotContains(t, logged, "sentinel-file-content")
	require.NotContains(t, logged, "sentinel-credential")
	require.Contains(t, logged, "body_bytes")
}

// TestUnmarshalBodyReusable_ImplicitRequestPayloadLog verifies unmarshal path triggers the unified request logging flag.
func TestUnmarshalBodyReusable_ImplicitRequestPayloadLog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	payload := `{"model":"gpt-4.1-mini"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	gmw.SetLogger(c, glog.Shared.Named("test"))

	type responseReq struct {
		Model string `json:"model"`
	}
	var req responseReq
	err := UnmarshalBodyReusable(c, &req)
	require.NoError(t, err)
	require.Equal(t, "gpt-4.1-mini", req.Model)

	logged, ok := c.Get(ctxkey.ClientRequestPayloadLogged)
	require.True(t, ok)
	require.Equal(t, true, logged)
}
