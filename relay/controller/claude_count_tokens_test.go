package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// newCountTokensTestContext returns a gin context whose request body is body.
func newCountTokensTestContext(t *testing.T, body string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}

// TestClaudeCountTokensURL covers count_tokens URL derivation, including the
// guarantee that an endpoint override is never reused unless it is a Messages URL.
func TestClaudeCountTokensURL(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		baseURL  string
		override string
		want     string
		ok       bool
	}{
		{"default_base", "https://api.anthropic.com", "", "https://api.anthropic.com/v1/messages/count_tokens", true},
		{"base_with_trailing_slash", "https://proxy.example/anthropic/", "", "https://proxy.example/anthropic/v1/messages/count_tokens", true},
		{"empty_base", "", "", "", false},
		{"override_messages", "https://ignored.example", "https://gw.example/v1/messages?x=1", "https://gw.example/v1/messages/count_tokens?x=1", true},
		{"override_prefixed_messages", "https://ignored.example", "https://foundry.example/anthropic/v1/messages/", "https://foundry.example/anthropic/v1/messages/count_tokens", true},
		{"override_not_messages", "https://ignored.example", "https://gw.example/generate", "", false},
		{"override_relative", "https://ignored.example", "/v1/messages", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			meta := &metalib.Meta{Mode: relaymode.ClaudeMessages, BaseURL: tc.baseURL}
			if tc.override != "" {
				meta.Config = dbmodel.ChannelConfig{EndpointURLs: map[string]string{
					channeltype.RelayModeToEndpointName(relaymode.ClaudeMessages): tc.override,
				}}
			}
			got, ok := claudeCountTokensURL(meta)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestClaudeCountTokensUpstreamRejectsRequest pins which upstream statuses are
// returned to the client instead of falling back to the local estimate.
func TestClaudeCountTokensUpstreamRejectsRequest(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity} {
		require.True(t, claudeCountTokensUpstreamRejectsRequest(status), status)
	}
	for _, status := range []int{
		http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusMethodNotAllowed,
		http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, 529,
	} {
		require.False(t, claudeCountTokensUpstreamRejectsRequest(status), status)
	}
}

// TestGetAndValidateClaudeCountTokensRequest proves count_tokens accepts a
// Messages body without max_tokens and still applies the shared validation.
func TestGetAndValidateClaudeCountTokensRequest(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"no_max_tokens", `{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hi"}]}`, 0},
		{"with_max_tokens", `{"model":"claude-sonnet-4-5","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`, 0},
		{"missing_model", `{"messages":[{"role":"user","content":"hi"}]}`, http.StatusBadRequest},
		{"empty_messages", `{"model":"claude-sonnet-4-5","messages":[]}`, http.StatusBadRequest},
		{"bad_role", `{"model":"claude-sonnet-4-5","messages":[{"role":"tool","content":"hi"}]}`, http.StatusBadRequest},
		{"empty_content", `{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":""}]}`, http.StatusBadRequest},
		{"invalid_json", `{"model":`, http.StatusBadRequest},
		{"ambiguous_model_key", `{"model":"claude-sonnet-4-5","Model":"claude-opus-4-1","messages":[{"role":"user","content":"hi"}]}`, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := newCountTokensTestContext(t, tc.body)
			request, raw, failure := getAndValidateClaudeCountTokensRequest(c)
			if tc.wantStatus == 0 {
				require.Nil(t, failure)
				require.Equal(t, "claude-sonnet-4-5", request.Model)
				require.Equal(t, tc.body, string(raw))
				return
			}
			require.NotNil(t, failure)
			require.Equal(t, tc.wantStatus, failure.status)
			require.Error(t, failure.err)
		})
	}
}

// TestValidateClaudeCountTokensLocalInput mirrors Anthropic's count_tokens
// input contract for the local estimate and proves no remote source survives.
func TestValidateClaudeCountTokensLocalInput(t *testing.T) {
	t.Parallel()
	const head = `{"model":"claude-sonnet-4-5",`
	const userText = `"messages":[{"role":"user","content":"hi"}]`
	for _, tc := range []struct {
		name    string
		body    string
		wantErr string
	}{
		{"plain_text", head + userText + `}`, ""},
		{"custom_tool", head + userText + `,"tools":[{"name":"Read","input_schema":{"type":"object"}}]}`, ""},
		{"client_tool", head + userText + `,"tools":[{"type":"bash_20250124","name":"bash"}]}`, ""},
		{"advisor_tool_is_counted", head + userText + `,"tools":[{"type":"advisor_20260301","name":"advisor"}]}`, ""},
		{"web_search_tool", head + userText + `,"tools":[{"type":"web_search_20250305","name":"web_search"}]}`, "web_search"},
		{"code_execution_tool", head + userText + `,"tools":[{"type":"code_execution_20250825","name":"code_execution"}]}`, "code_execution"},
		{"mcp_servers", head + userText + `,"mcp_servers":[{"type":"url","url":"https://mcp.example.invalid","name":"x"}]}`, "mcp_connector"},
		{"base64_image", head + `"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}}]}]}`, ""},
		{"url_image", head + `"messages":[{"role":"user","content":[{"type":"image","source":{"type":"url","url":"https://example.invalid/a.png"}}]}]}`, "messages[0].content[0]"},
		{"file_image", head + `"messages":[{"role":"user","content":[{"type":"image","source":{"type":"file","file_id":"file_1"}}]}]}`, "base64 image"},
		{"base64_typed_image_with_url_only", head + `"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","url":"https://example.invalid/a.png"}}]}]}`, "base64 image"},
		{"nested_tool_result_url_image", head + `"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"t","content":[{"type":"image","source":{"type":"url","url":"https://example.invalid/a.png"}}]}]}]}`, "messages[0].content[0].content[0]"},
		{"system_url_image", head + `"system":[{"type":"image","source":{"type":"url","url":"https://example.invalid/a.png"}}],` + userText + `}`, "system[0]"},
		{"text_document", head + `"messages":[{"role":"user","content":[{"type":"document","source":{"type":"text","media_type":"text/plain","data":"doc"}}]}]}`, ""},
		{"url_document", head + `"messages":[{"role":"user","content":[{"type":"document","source":{"type":"url","url":"https://example.invalid/a.pdf"}}]}]}`, "document"},
		{"file_document", head + `"messages":[{"role":"user","content":[{"type":"document","source":{"type":"file","file_id":"file_1"}}]}]}`, "document"},
		{"content_document_url_image", head + `"messages":[{"role":"user","content":[{"type":"document","source":{"type":"content","content":[{"type":"image","source":{"type":"url","url":"https://example.invalid/a.png"}}]}}]}]}`, "source.content[0]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var request ClaudeMessagesRequest
			require.NoError(t, json.Unmarshal([]byte(tc.body), &request))
			err := validateClaudeCountTokensLocalInput(&request, []byte(tc.body))
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// TestEstimateClaudeCountTokensMatchesMessagesEstimator proves the local
// count_tokens answer is the Messages admission estimator's prompt count.
func TestEstimateClaudeCountTokensMatchesMessagesEstimator(t *testing.T) {
	t.Parallel()
	body := `{"model":"claude-sonnet-4-5","system":"You are Claude Code.","messages":[{"role":"user","content":[{"type":"text","text":"hello there"},{"type":"tool_result","tool_use_id":"t","content":"file contents"}]}],"tools":[{"name":"Read","description":"Read a file","input_schema":{"type":"object","properties":{"path":{"type":"string"}}}}]}`
	c := newCountTokensTestContext(t, body)
	c.Set(ctxkey.Meta, &metalib.Meta{ChannelType: channeltype.OpenAICompatible})
	request, raw, failure := getAndValidateClaudeCountTokensRequest(c)
	require.Nil(t, failure)

	tokens, failure := estimateClaudeCountTokens(c, request, raw)
	require.Nil(t, failure)
	want, err := getClaudeMessagesPromptTokens(c, request)
	require.NoError(t, err)
	require.Equal(t, want, tokens)
	require.Positive(t, tokens)

	withoutTools := *request
	withoutTools.Tools = nil
	fewer, err := getClaudeMessagesPromptTokens(c, &withoutTools)
	require.NoError(t, err)
	require.Greater(t, tokens, fewer, "tool definitions must be counted")
}
