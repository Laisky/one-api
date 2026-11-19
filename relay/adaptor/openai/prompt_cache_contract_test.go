package openai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
)

// TestInspectPromptCacheWriteIntent pins how request controls map to
// OpenAI's GPT-5.6+ cache-write contract, including caller data that must
// never be interpreted as a provider cache control.
func TestInspectPromptCacheWriteIntent(t *testing.T) {
	for _, tc := range []struct {
		name          string
		body          string
		writes        bool
		clientWrites  bool
		breakpoints   int
		optionsPresnt bool
	}{
		{name: "no_controls_is_implicit", body: `{"input":"hi"}`, writes: true},
		{name: "null_options_is_implicit", body: `{"prompt_cache_options":null,"input":"hi"}`, writes: true},
		{name: "explicit_mode_disables_caching", body: `{"prompt_cache_options":{"mode":"explicit"},"input":"hi"}`, optionsPresnt: true},
		{name: "explicit_mode_casefold", body: `{"prompt_cache_options":{"mode":" EXPLICIT "},"input":"hi"}`, optionsPresnt: true},
		{name: "explicit_mode_with_breakpoint", body: `{"prompt_cache_options":{"mode":"explicit"},"input":[{"role":"user","content":[{"type":"input_text","text":"a","prompt_cache_breakpoint":{}}]}]}`,
			writes: true, clientWrites: true, breakpoints: 1, optionsPresnt: true},
		{name: "requested_implicit_mode", body: `{"prompt_cache_options":{"mode":"implicit"},"input":"hi"}`, writes: true, clientWrites: true, optionsPresnt: true},
		{name: "prewarm", body: `{"prompt_cache_options":{"mode":"explicit","prewarm":true},"input":"hi"}`, clientWrites: true, optionsPresnt: true},
		{name: "ttl_only_keeps_implicit_default", body: `{"prompt_cache_options":{"ttl":"30m"},"input":"hi"}`, writes: true, optionsPresnt: true},
		{name: "function_call_output_parts", body: `{"input":[{"type":"function_call_output","call_id":"c","output":[{"type":"input_text","text":"r","prompt_cache_breakpoint":{"ttl":"30m"}}]}]}`,
			writes: true, clientWrites: true, breakpoints: 1},
		{name: "string_output_is_data", body: `{"input":[{"type":"function_call_output","call_id":"c","output":"{\"prompt_cache_breakpoint\":{\"ttl\":\"12h\"}}"}]}`, writes: true},
		{name: "tool_schema_is_data", body: `{"input":"hi","tools":[{"type":"function","name":"f","parameters":{"type":"object","properties":{"x":{"prompt_cache_breakpoint":{"ttl":"12h"}}}}}]}`, writes: true},
		{name: "prompt_variables_are_data", body: `{"prompt":{"id":"pmpt_1","variables":{"x":{"prompt_cache_breakpoint":{"ttl":"12h"}}}}}`, writes: true},
		{name: "literal_text_is_data", body: `{"input":"prompt_cache_breakpoint ttl 12h"}`, writes: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			intent, err := InspectPromptCacheWriteIntent([]byte(tc.body))
			require.NoError(t, err)
			require.Equal(t, tc.writes, intent.Writes())
			require.Equal(t, tc.clientWrites, intent.ClientRequestsWrites())
			require.Equal(t, tc.breakpoints, intent.Breakpoints)
			require.Equal(t, tc.optionsPresnt, intent.OptionsPresent)
		})
	}
}

// TestInspectPromptCacheWriteIntentRejectsUnmeteredControls proves malformed
// and unmetered controls are refused as client control errors.
func TestInspectPromptCacheWriteIntentRejectsUnmeteredControls(t *testing.T) {
	for name, body := range map[string]string{
		"options_ttl_12h":       `{"prompt_cache_options":{"ttl":"12h"}}`,
		"options_ttl_null":      `{"prompt_cache_options":{"ttl":null}}`,
		"options_ttl_number":    `{"prompt_cache_options":{"ttl":1800}}`,
		"options_not_object":    `{"prompt_cache_options":"explicit"}`,
		"options_unknown_mode":  `{"prompt_cache_options":{"mode":"always"}}`,
		"options_mode_null":     `{"prompt_cache_options":{"mode":null}}`,
		"options_prewarm_text":  `{"prompt_cache_options":{"prewarm":"yes"}}`,
		"breakpoint_ttl_12h":    `{"input":[{"role":"user","content":[{"type":"input_text","text":"a","prompt_cache_breakpoint":{"ttl":"12h"}}]}]}`,
		"breakpoint_not_object": `{"input":[{"role":"user","content":[{"type":"input_text","text":"a","prompt_cache_breakpoint":true}]}]}`,
		"breakpoint_ttl_null":   `{"input":[{"role":"user","content":[{"type":"input_text","text":"a","prompt_cache_breakpoint":{"ttl":null}}]}]}`,
		"deep_nesting":          `{"input":` + strings.Repeat("[", 80) + strings.Repeat("]", 80) + `}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := InspectPromptCacheWriteIntent([]byte(body))
			require.Error(t, err)
			require.True(t, adaptor.IsUnbillableRequestControl(err), "client controls must be answered as HTTP 400: %v", err)
		})
	}
}

// TestSanitizePromptCacheOptions proves only metered controls are forwarded.
func TestSanitizePromptCacheOptions(t *testing.T) {
	for _, tc := range []struct {
		name, raw, want string
		keep            bool
	}{
		{name: "normalizes_and_drops_diagnostics", raw: `{"mode":" Explicit ","ttl":"30M","prewarm":false,"comparison_response_id":"resp_x","future":1}`,
			want: `{"mode":"explicit","prewarm":false,"ttl":"30m"}`, keep: true},
		{name: "diagnostics_only", raw: `{"comparison_response_id":"resp_x"}`},
		{name: "empty_object", raw: `{}`},
		{name: "null", raw: `null`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, keep, err := SanitizePromptCacheOptions(json.RawMessage(tc.raw))
			require.NoError(t, err)
			require.Equal(t, tc.keep, keep)
			if tc.keep {
				require.JSONEq(t, tc.want, string(got))
			}
		})
	}
	_, _, err := SanitizePromptCacheOptions(json.RawMessage(`{"ttl":"12h"}`))
	require.True(t, adaptor.IsUnbillableRequestControl(err))
}

// TestPromptCacheExplicitPinIsAttemptScoped proves the admission pin rewrites
// only the attempt it was decided for, merges existing options, is cleared by
// the next admission, and refuses a pinned body it cannot rewrite.
func TestPromptCacheExplicitPinIsAttemptScoped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newContext := func() *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
		return c
	}
	pinned := &meta.Meta{ChannelType: channeltype.OpenAI, ChannelId: 7, ActualModelName: "gpt-6-cyber"}
	read := func(t *testing.T, c *gin.Context, m *meta.Meta, body string) map[string]any {
		t.Helper()
		out, err := applyPromptCacheExplicitPin(c, m, bytes.NewReader([]byte(body)))
		require.NoError(t, err)
		raw, err := io.ReadAll(out)
		require.NoError(t, err)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(raw, &decoded))
		return decoded
	}

	c := newContext()
	SetPromptCacheExplicitPin(c, pinned, true)
	got := read(t, c, pinned, `{"model":"gpt-6-cyber","prompt_cache_options":{"ttl":"30m"},"input":"hi"}`)
	require.Equal(t, map[string]any{"mode": "explicit", "ttl": "30m"}, got["prompt_cache_options"])

	for name, other := range map[string]*meta.Meta{
		"other_channel": {ChannelType: channeltype.OpenAI, ChannelId: 8, ActualModelName: "gpt-6-cyber"},
		"other_model":   {ChannelType: channeltype.OpenAI, ChannelId: 7, ActualModelName: "gpt-6.1-terra"},
		"other_type":    {ChannelType: channeltype.Azure, ChannelId: 7, ActualModelName: "gpt-6-cyber"},
	} {
		t.Run(name, func(t *testing.T) {
			got := read(t, c, other, `{"input":"hi"}`)
			require.NotContains(t, got, "prompt_cache_options", "a retry on another attempt must not inherit the pin")
		})
	}

	SetPromptCacheExplicitPin(c, pinned, false)
	require.NotContains(t, read(t, c, pinned, `{"input":"hi"}`), "prompt_cache_options", "the next admission clears the pin")

	SetPromptCacheExplicitPin(c, pinned, true)
	_, err := applyPromptCacheExplicitPin(c, pinned, strings.NewReader(`not json`))
	require.Error(t, err, "a pinned body that cannot be rewritten must not be dispatched")
}
