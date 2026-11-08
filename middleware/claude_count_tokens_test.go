package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
)

// claudeErrorEnvelope is the Anthropic error body shape asserted by these tests.
type claudeErrorEnvelope struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// decodeClaudeError decodes and sanity-checks an Anthropic error envelope.
func decodeClaudeError(t *testing.T, w *httptest.ResponseRecorder) claudeErrorEnvelope {
	t.Helper()
	var envelope claudeErrorEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope), w.Body.String())
	require.Equal(t, "error", envelope.Type, w.Body.String())
	require.NotEmpty(t, envelope.Error.Message, w.Body.String())
	return envelope
}

// TestClaudeCountTokensPathHelpers covers the path predicates.
func TestClaudeCountTokensPathHelpers(t *testing.T) {
	t.Parallel()
	require.True(t, IsClaudeCountTokensPath("/v1/messages/count_tokens"))
	require.True(t, IsClaudeCountTokensPath("/v1/messages/count_tokens/"))
	require.False(t, IsClaudeCountTokensPath("/v1/messages"))
	require.False(t, IsClaudeCountTokensPath("/v1/v1/messages/count_tokens"))
	require.False(t, IsClaudeCountTokensPath("/v1/messages/count_tokens/x"))
}

// TestClaudeErrorTypeForStatus covers the documented Anthropic error types.
func TestClaudeErrorTypeForStatus(t *testing.T) {
	t.Parallel()
	for status, want := range map[int]string{
		http.StatusBadRequest:            "invalid_request_error",
		http.StatusUnprocessableEntity:   "invalid_request_error",
		http.StatusUnauthorized:          "authentication_error",
		http.StatusForbidden:             "permission_error",
		http.StatusNotFound:              "not_found_error",
		http.StatusRequestEntityTooLarge: "request_too_large",
		http.StatusTooManyRequests:       "rate_limit_error",
		http.StatusInternalServerError:   "api_error",
		http.StatusBadGateway:            "api_error",
		529:                              "overloaded_error",
	} {
		require.Equal(t, want, ClaudeErrorTypeForStatus(status), status)
	}
}

// countTokensMiddlewareEngine wires the count_tokens body limit and model
// guard around a handler that echoes the cached body length.
func countTokensMiddlewareEngine(requestModel string) *gin.Engine {
	engine := gin.New()
	engine.Use(RequestBodyLimit())
	engine.POST(ClaudeCountTokensPath,
		ClaudeCountTokensBodyLimit(),
		func(c *gin.Context) {
			c.Set(ctxkey.RequestModel, requestModel)
			c.Next()
		},
		ClaudeCountTokensRequestGuard(),
		func(c *gin.Context) {
			body, err := common.GetRequestBody(c)
			if err != nil {
				c.String(http.StatusInternalServerError, err.Error())
				return
			}
			c.String(http.StatusOK, "%d", len(body))
		})
	return engine
}

// TestClaudeCountTokensBodyLimit proves the body is cached for later readers
// and an oversized body gets an Anthropic-format 413 before any handler runs.
// It mutates the global body cap, so it does not run in parallel.
func TestClaudeCountTokensBodyLimit(t *testing.T) {
	original := config.MaxRequestBodySizeMB
	config.MaxRequestBodySizeMB = 1
	t.Cleanup(func() { config.MaxRequestBodySizeMB = original })
	require.Equal(t, int64(1<<20), claudeCountTokensBodyLimitBytes(), "the smaller global cap wins")

	engine := countTokensMiddlewareEngine("claude-sonnet-4-5")
	body := `{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hi"}]}`
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, ClaudeCountTokensPath, strings.NewReader(body)))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, strconv.Itoa(len(body)), w.Body.String(), "later readers must see the cached body")

	oversized := `{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"` + strings.Repeat("a", 1<<20) + `"}]}`
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, ClaudeCountTokensPath, strings.NewReader(oversized)))
	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code, w.Body.String())
	require.Equal(t, "request_too_large", decodeClaudeError(t, w).Error.Type)

	config.MaxRequestBodySizeMB = 0
	require.Equal(t, ClaudeCountTokensMaxBodyBytes, claudeCountTokensBodyLimitBytes(), "the count_tokens cap applies without a global cap")
	config.MaxRequestBodySizeMB = 128
	require.Equal(t, ClaudeCountTokensMaxBodyBytes, claudeCountTokensBodyLimitBytes(), "the count_tokens cap is tighter than the default global cap")
}

// TestClaudeCountTokensRequestGuard proves a body without a usable model is
// rejected in Anthropic format, and a body with one passes.
func TestClaudeCountTokensRequestGuard(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		requestModel string
		body         string
		wantStatus   int
		wantMessage  string
	}{
		{"model_present", "claude-sonnet-4-5", `{"model":"claude-sonnet-4-5"}`, http.StatusOK, ""},
		{"invalid_json", "", `{"model":`, http.StatusBadRequest, "valid JSON"},
		{"missing_model", "", `{"messages":[]}`, http.StatusBadRequest, "model"},
		{"blank_model", "  ", `{"model":"  "}`, http.StatusBadRequest, "model"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			engine := countTokensMiddlewareEngine(tc.requestModel)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, ClaudeCountTokensPath, strings.NewReader(tc.body)))
			require.Equal(t, tc.wantStatus, w.Code, w.Body.String())
			if tc.wantStatus != http.StatusOK {
				envelope := decodeClaudeError(t, w)
				require.Equal(t, "invalid_request_error", envelope.Error.Type)
				require.Contains(t, envelope.Error.Message, tc.wantMessage)
			}
		})
	}
}

// TestAPIFormatAutoDetectSkipsClaudeMessagesSubPaths proves a count_tokens
// request whose body looks like a Responses request is not rerouted onto the
// billed Responses endpoint, while /v1/messages itself keeps auto-detection.
func TestAPIFormatAutoDetectSkipsClaudeMessagesSubPaths(t *testing.T) {
	originalEnabled := config.AutoDetectAPIFormat
	originalAction := config.AutoDetectAPIFormatAction
	config.AutoDetectAPIFormat = true
	config.AutoDetectAPIFormatAction = "transparent"
	t.Cleanup(func() {
		config.AutoDetectAPIFormat = originalEnabled
		config.AutoDetectAPIFormatAction = originalAction
	})

	engine := gin.New()
	engine.Use(APIFormatAutoDetect(engine))
	engine.POST("/v1/responses", func(c *gin.Context) { c.String(http.StatusOK, "responses") })
	engine.POST("/v1/messages", func(c *gin.Context) { c.String(http.StatusOK, "messages") })
	engine.POST(ClaudeCountTokensPath, func(c *gin.Context) { c.String(http.StatusOK, "count_tokens") })

	responsesBody := `{"model":"claude-sonnet-4-5","input":"hello"}`
	for path, want := range map[string]string{
		ClaudeCountTokensPath: "count_tokens",
		"/v1/messages":        "responses",
	} {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(responsesBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, path)
		require.Equal(t, want, w.Body.String(), path)
	}
}
