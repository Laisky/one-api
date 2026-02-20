package common

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/ctxkey"
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
