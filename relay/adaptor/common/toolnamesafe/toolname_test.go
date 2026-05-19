package toolnamesafe

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/model"
)

// validNamePattern mirrors DeepSeek/OpenAI's `^[a-zA-Z0-9_-]+$` allowlist used
// to assert that sanitized identifiers pass the upstream validator.
var validNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func newTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c
}

func mapOnContext(t *testing.T, c *gin.Context) map[string]string {
	t.Helper()
	raw, exists := c.Get(ctxkey.ToolNameSanitizeMap)
	require.True(t, exists, "expected ToolNameSanitizeMap to be set on context")
	mp, ok := raw.(map[string]string)
	require.True(t, ok, "expected map[string]string under ToolNameSanitizeMap")
	return mp
}

func TestSanitizeToolName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   string
		want    string
		changed bool
	}{
		{name: "empty", input: "", want: "", changed: false},
		{name: "valid alnum", input: "get_weather", want: "get_weather", changed: false},
		{name: "valid with dash", input: "get-weather-v2", want: "get-weather-v2", changed: false},
		{name: "single dot", input: "a.b", want: "a_b", changed: true},
		{name: "multiple dots", input: "mcp.server.tool", want: "mcp_server_tool", changed: true},
		{name: "forward slash", input: "tools/list", want: "tools_list", changed: true},
		{name: "mixed punctuation", input: "ns:tool@v1", want: "ns_tool_v1", changed: true},
		{name: "unicode", input: "工具", want: "__", changed: true}, // each rune becomes a single underscore
		{name: "exactly 64 bytes", input: strings.Repeat("a", 64), want: strings.Repeat("a", 64), changed: false},
		{name: "65 bytes clamps", input: strings.Repeat("a", 65), want: strings.Repeat("a", 64), changed: true},
		{name: "65 bytes with dots clamps", input: strings.Repeat("a.", 33), want: strings.Repeat("a_", 32), changed: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, changed := SanitizeToolName(tc.input)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.changed, changed)
			if tc.input != "" {
				require.LessOrEqual(t, len(got), MaxToolNameLen)
			}
		})
	}
}

// TestSanitizeToolName_Idempotent verifies sanitizing a sanitized name is a no-op.
func TestSanitizeToolName_Idempotent(t *testing.T) {
	t.Parallel()
	inputs := []string{
		"a.b.c",
		"mcp__server.something/list",
		strings.Repeat("x.", 40),
		"plain_name",
	}
	for _, in := range inputs {
		first, _ := SanitizeToolName(in)
		second, changed := SanitizeToolName(first)
		require.Equal(t, first, second, "sanitize should be idempotent for %q", in)
		require.False(t, changed, "second pass should not flag changes for %q", in)
	}
}

func TestSanitizeRequestToolNames_NilRequest(t *testing.T) {
	t.Parallel()
	c := newTestContext()
	got := SanitizeRequestToolNames(c, nil)
	require.Equal(t, 0, got)
	_, exists := c.Get(ctxkey.ToolNameSanitizeMap)
	require.False(t, exists, "no map should be set for nil request")
}

func TestSanitizeRequestToolNames_OnlyValidNames(t *testing.T) {
	t.Parallel()
	c := newTestContext()
	req := &model.GeneralOpenAIRequest{
		Tools: []model.Tool{
			{Type: "function", Function: &model.Function{Name: "get_weather"}},
			{Type: "function", Function: &model.Function{Name: "search-docs-v2"}},
		},
	}
	got := SanitizeRequestToolNames(c, req)
	require.Equal(t, 0, got)
	_, exists := c.Get(ctxkey.ToolNameSanitizeMap)
	require.False(t, exists, "valid-only requests should not stash a map")
	require.Equal(t, "get_weather", req.Tools[0].Function.Name)
	require.Equal(t, "search-docs-v2", req.Tools[1].Function.Name)
}

func TestSanitizeRequestToolNames_SingleDottedTool(t *testing.T) {
	t.Parallel()
	c := newTestContext()
	req := &model.GeneralOpenAIRequest{
		Tools: []model.Tool{
			{Type: "function", Function: &model.Function{Name: "mcp.server.search"}},
		},
	}
	got := SanitizeRequestToolNames(c, req)
	require.Equal(t, 1, got)
	require.Equal(t, "mcp_server_search", req.Tools[0].Function.Name)
	require.True(t, validNamePattern.MatchString(req.Tools[0].Function.Name))

	mp := mapOnContext(t, c)
	require.Equal(t, "mcp.server.search", mp["mcp_server_search"])
}

func TestSanitizeRequestToolNames_MixedToolList(t *testing.T) {
	t.Parallel()
	c := newTestContext()
	req := &model.GeneralOpenAIRequest{
		Tools: []model.Tool{
			{Type: "function", Function: &model.Function{Name: "valid_one"}},
			{Type: "function", Function: &model.Function{Name: "weird.one"}},
			{Type: "function", Function: &model.Function{Name: "ok2"}},
			{Type: "function", Function: &model.Function{Name: "another/bad"}},
		},
	}
	got := SanitizeRequestToolNames(c, req)
	require.Equal(t, 2, got)
	require.Equal(t, "valid_one", req.Tools[0].Function.Name)
	require.Equal(t, "weird_one", req.Tools[1].Function.Name)
	require.Equal(t, "ok2", req.Tools[2].Function.Name)
	require.Equal(t, "another_bad", req.Tools[3].Function.Name)

	mp := mapOnContext(t, c)
	require.Equal(t, "weird.one", mp["weird_one"])
	require.Equal(t, "another/bad", mp["another_bad"])
}

func TestSanitizeRequestToolNames_ToolChoiceObject(t *testing.T) {
	t.Parallel()
	c := newTestContext()
	req := &model.GeneralOpenAIRequest{
		ToolChoice: map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": "fs.read",
			},
		},
	}
	got := SanitizeRequestToolNames(c, req)
	require.Equal(t, 1, got)

	choiceMap, ok := req.ToolChoice.(map[string]any)
	require.True(t, ok)
	fnMap, ok := choiceMap["function"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "fs_read", fnMap["name"])

	mp := mapOnContext(t, c)
	require.Equal(t, "fs.read", mp["fs_read"])
}

func TestSanitizeRequestToolNames_ToolChoiceStringAuto(t *testing.T) {
	t.Parallel()
	c := newTestContext()
	req := &model.GeneralOpenAIRequest{
		ToolChoice: "auto",
	}
	got := SanitizeRequestToolNames(c, req)
	require.Equal(t, 0, got)
	require.Equal(t, "auto", req.ToolChoice)
	_, exists := c.Get(ctxkey.ToolNameSanitizeMap)
	require.False(t, exists)
}

func TestSanitizeRequestToolNames_AssistantReplayToolCalls(t *testing.T) {
	t.Parallel()
	c := newTestContext()
	req := &model.GeneralOpenAIRequest{
		Messages: []model.Message{
			{
				Role: "assistant",
				ToolCalls: []model.Tool{
					{
						Id:       "call_1",
						Type:     "function",
						Function: &model.Function{Name: "fs.read", Arguments: "{}"},
					},
					{
						Id:       "call_2",
						Type:     "function",
						Function: &model.Function{Name: "already_ok", Arguments: "{}"},
					},
				},
			},
		},
	}
	got := SanitizeRequestToolNames(c, req)
	require.Equal(t, 1, got)
	require.Equal(t, "fs_read", req.Messages[0].ToolCalls[0].Function.Name)
	require.Equal(t, "already_ok", req.Messages[0].ToolCalls[1].Function.Name)

	mp := mapOnContext(t, c)
	require.Equal(t, "fs.read", mp["fs_read"])
}

func TestSanitizeRequestToolNames_LegacyMessageName(t *testing.T) {
	t.Parallel()
	c := newTestContext()
	legacy := "tool.name"
	req := &model.GeneralOpenAIRequest{
		Messages: []model.Message{
			{Role: "function", Name: &legacy, Content: "{}"},
		},
	}
	got := SanitizeRequestToolNames(c, req)
	require.Equal(t, 1, got)
	require.NotNil(t, req.Messages[0].Name)
	require.Equal(t, "tool_name", *req.Messages[0].Name)

	mp := mapOnContext(t, c)
	require.Equal(t, "tool.name", mp["tool_name"])
}

func TestSanitizeRequestToolNames_CollisionSuffix(t *testing.T) {
	t.Parallel()
	c := newTestContext()
	req := &model.GeneralOpenAIRequest{
		Tools: []model.Tool{
			{Type: "function", Function: &model.Function{Name: "a.b"}},
			{Type: "function", Function: &model.Function{Name: "a-b"}}, // already valid, untouched
			{Type: "function", Function: &model.Function{Name: "a/b"}}, // collides with "a_b"
		},
	}
	got := SanitizeRequestToolNames(c, req)
	require.Equal(t, 2, got)

	// First dotted name takes the natural sanitized form.
	require.Equal(t, "a_b", req.Tools[0].Function.Name)
	require.Equal(t, "a-b", req.Tools[1].Function.Name)

	// Third collides with first; it must get a stable FNV-derived suffix.
	collided := req.Tools[2].Function.Name
	require.NotEqual(t, collided, "a_b")
	require.NotEqual(t, collided, "a-b")
	require.True(t, validNamePattern.MatchString(collided))
	require.LessOrEqual(t, len(collided), MaxToolNameLen)

	mp := mapOnContext(t, c)
	require.Equal(t, "a.b", mp["a_b"])
	require.Equal(t, "a/b", mp[collided])
}

func TestSanitizeRequestToolNames_Idempotent(t *testing.T) {
	t.Parallel()
	c := newTestContext()
	req := &model.GeneralOpenAIRequest{
		Tools: []model.Tool{
			{Type: "function", Function: &model.Function{Name: "mcp.tool"}},
		},
	}
	first := SanitizeRequestToolNames(c, req)
	require.Equal(t, 1, first)
	require.Equal(t, "mcp_tool", req.Tools[0].Function.Name)

	second := SanitizeRequestToolNames(c, req)
	require.Equal(t, 0, second, "already-sanitized request should not be re-renamed")
	require.Equal(t, "mcp_tool", req.Tools[0].Function.Name)

	mp := mapOnContext(t, c)
	require.Equal(t, "mcp.tool", mp["mcp_tool"])
	require.Len(t, mp, 1)
}

func TestRestoreToolCallNames_EmptySlice(t *testing.T) {
	t.Parallel()
	c := newTestContext()
	require.False(t, RestoreToolCallNames(c, nil))
	require.False(t, RestoreToolCallNames(c, []model.Tool{}))
}

func TestRestoreToolCallNames_NoMap(t *testing.T) {
	t.Parallel()
	c := newTestContext()
	tools := []model.Tool{{Function: &model.Function{Name: "anything"}}}
	require.False(t, RestoreToolCallNames(c, tools))
	require.Equal(t, "anything", tools[0].Function.Name)
}

func TestRestoreToolCallNames_RestoresKnownNames(t *testing.T) {
	t.Parallel()
	c := newTestContext()
	c.Set(ctxkey.ToolNameSanitizeMap, map[string]string{
		"fs_read":   "fs.read",
		"weird_one": "weird.one",
	})
	tools := []model.Tool{
		{Function: &model.Function{Name: "fs_read"}},
		{Function: &model.Function{Name: "weird_one"}},
	}
	require.True(t, RestoreToolCallNames(c, tools))
	require.Equal(t, "fs.read", tools[0].Function.Name)
	require.Equal(t, "weird.one", tools[1].Function.Name)
}

func TestRestoreToolCallNames_UnknownNameUnchanged(t *testing.T) {
	t.Parallel()
	c := newTestContext()
	c.Set(ctxkey.ToolNameSanitizeMap, map[string]string{"fs_read": "fs.read"})
	tools := []model.Tool{
		{Function: &model.Function{Name: "model_invented_name"}},
	}
	require.False(t, RestoreToolCallNames(c, tools))
	require.Equal(t, "model_invented_name", tools[0].Function.Name)
}

func TestRestoreToolCallNames_Mixed(t *testing.T) {
	t.Parallel()
	c := newTestContext()
	c.Set(ctxkey.ToolNameSanitizeMap, map[string]string{"fs_read": "fs.read"})
	tools := []model.Tool{
		{Function: &model.Function{Name: "fs_read"}},
		{Function: &model.Function{Name: "search_docs"}}, // not in map
		{Function: nil}, // nil function should be skipped
	}
	require.True(t, RestoreToolCallNames(c, tools))
	require.Equal(t, "fs.read", tools[0].Function.Name)
	require.Equal(t, "search_docs", tools[1].Function.Name)
	require.Nil(t, tools[2].Function)
}

func TestRestoreToolName(t *testing.T) {
	t.Parallel()

	t.Run("empty input", func(t *testing.T) {
		t.Parallel()
		c := newTestContext()
		require.Equal(t, "", RestoreToolName(c, ""))
	})

	t.Run("no map on context", func(t *testing.T) {
		t.Parallel()
		c := newTestContext()
		require.Equal(t, "anything_else", RestoreToolName(c, "anything_else"))
	})

	t.Run("hit in map", func(t *testing.T) {
		t.Parallel()
		c := newTestContext()
		c.Set(ctxkey.ToolNameSanitizeMap, map[string]string{"a_b": "a.b"})
		require.Equal(t, "a.b", RestoreToolName(c, "a_b"))
	})

	t.Run("miss in map", func(t *testing.T) {
		t.Parallel()
		c := newTestContext()
		c.Set(ctxkey.ToolNameSanitizeMap, map[string]string{"a_b": "a.b"})
		require.Equal(t, "x_y", RestoreToolName(c, "x_y"))
	})
}
