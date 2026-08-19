package openai

import (
	"bytes"
	"encoding/json"
	"io"
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

	"github.com/Laisky/one-api/relay/model"
)

// TestImageHandler_UpstreamErrorSurfacesStructuredError verifies that a JSON error
// returned by the upstream image API is returned to the relay error pipeline instead
// of being treated as a successful relay after the body is copied to the client.
func TestImageHandler_UpstreamErrorSurfacesStructuredError(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"error":{"message":"request rejected by the safety system","type":"image_generation_user_error","param":"prompt","code":"moderation_blocked"}}`)
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}

	errResp, usage := ImageHandler(ctx, resp)

	require.Nil(t, usage, "upstream image errors must not produce usage")
	require.NotNil(t, errResp, "upstream image errors must reach relay error handling")
	require.Equal(t, http.StatusBadRequest, errResp.StatusCode)
	require.Equal(t, model.ErrorType("image_generation_user_error"), errResp.Type)
	require.Equal(t, "moderation_blocked", errResp.Code)
	require.Equal(t, "prompt", errResp.Param)
	require.Equal(t, "request rejected by the safety system", errResp.Message)
	require.Empty(t, recorder.Body.Bytes(), "outer relay must write the normalized error response")
}

// TestImageHandler_AcceptsValid202Response verifies the response layer treats
// every valid HTTP 2xx image response as successful, matching billing's status
// classification.
func TestImageHandler_AcceptsValid202Response(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body := `{"created":1700000000,"data":[{"url":"https://example.com/image.png"}]}`
	resp := &http.Response{
		StatusCode: http.StatusAccepted,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	errResp, usage := ImageHandler(ctx, resp)

	require.Nil(t, errResp)
	require.NotNil(t, usage)
	require.Equal(t, http.StatusAccepted, recorder.Code)
	require.JSONEq(t, body, recorder.Body.String())
}

// TestImageHandler_UpstreamErrorPreservesSafeHeaders verifies that retry and
// request-correlation metadata survives normalized upstream image errors while
// sensitive provider headers are not copied to the client.
func TestImageHandler_UpstreamErrorPreservesSafeHeaders(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header: http.Header{
			"Retry-After":                  []string{"17"},
			"X-Request-Id":                 []string{"req_upstream_123"},
			"X-Ratelimit-Remaining-Images": []string{"0"},
			"Set-Cookie":                   []string{"provider_session=secret"},
		},
		Body: io.NopCloser(strings.NewReader(
			`{"error":{"message":"rate limited","type":"rate_limit_error","code":"rate_limit"}}`,
		)),
	}

	errResp, usage := ImageHandler(ctx, resp)

	require.NotNil(t, errResp)
	require.Nil(t, usage)
	require.Equal(t, "17", recorder.Header().Get("Retry-After"))
	require.Equal(t, "req_upstream_123", recorder.Header().Get("X-Request-Id"))
	require.Equal(t, "0", recorder.Header().Get("X-Ratelimit-Remaining-Images"))
	require.Empty(t, recorder.Header().Get("Set-Cookie"))
}

// TestImageHandler_ZhipuErrorEnvelopePreservesDiagnostics verifies the flat
// error envelope returned by the Zhipu image endpoint is normalized without
// discarding its safe message and machine-readable code.
func TestImageHandler_ZhipuErrorEnvelopePreservesDiagnostics(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     make(http.Header),
		Body: io.NopCloser(strings.NewReader(
			`{"code":1214,"msg":"model invocation parameters are invalid","success":false}`,
		)),
	}

	errResp, usage := ImageHandler(ctx, resp)

	require.NotNil(t, errResp)
	require.Nil(t, usage)
	require.Equal(t, "model invocation parameters are invalid", errResp.Message)
	require.EqualValues(t, float64(1214), errResp.Code)
	require.Equal(t, model.ErrorTypeUpstream, errResp.Type)
}

// TestImageHandler_BoundsProviderControlledLogFields verifies that an upstream
// cannot amplify or inject arbitrarily large param, code, or request-id values
// into the gateway's structured debug logs.
func TestImageHandler_BoundsProviderControlledLogFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, observed := observer.New(zapcore.DebugLevel)
	lg, err := glog.NewConsoleWithName("test", glog.LevelDebug,
		zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	gmw.SetLogger(ctx, lg)

	const sentinel = "must-not-appear-after-log-boundary"
	longValue := strings.Repeat("x", 4096) + sentinel
	body, err := json.Marshal(map[string]any{
		"error": map[string]any{
			"message": longValue,
			"type":    "invalid_request_error",
			"param":   longValue,
			"code":    longValue,
		},
	})
	require.NoError(t, err)
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"X-Request-Id": []string{longValue}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}

	errResp, usage := ImageHandler(ctx, resp)
	require.NotNil(t, errResp)
	require.Nil(t, usage)
	require.NotContains(t, errResp.Message, sentinel)
	require.NotContains(t, errResp.Param, sentinel)
	require.NotContains(t, errResp.Code, sentinel)

	entries := observed.FilterMessage("upstream image request returned a structured error").All()
	require.Len(t, entries, 1)
	encoded, err := json.Marshal(entries[0].ContextMap())
	require.NoError(t, err)
	require.NotContains(t, string(encoded), sentinel)
	require.Less(t, len(encoded), 2048, "provider-controlled log fields must be bounded")
}
