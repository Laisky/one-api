package openai_compatible

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestHandlerPreservesNestedCachedTokens verifies that an OpenAI-shaped response
// carrying prompt_tokens_details.cached_tokens (and no top-level cached_tokens)
// is left UNCHANGED by the shared usage normalization. This guards against the
// StepFun top-level promotion accidentally clobbering OpenAI-style usage.
func TestHandlerPreservesNestedCachedTokens(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := map[string]any{
		"choices": []map[string]any{
			{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "hi"},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     591,
			"completion_tokens": 120,
			"total_tokens":      711,
			"prompt_tokens_details": map[string]any{
				"cached_tokens": 256,
			},
		},
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(raw)),
	}

	errResp, usage := Handler(c, resp, 591, "gpt-4o")
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	require.NotNil(t, usage.PromptTokensDetails)
	require.Equal(t, 256, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 591, usage.PromptTokens)

	var rendered map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rendered))
	renderedUsage, ok := rendered["usage"].(map[string]any)
	require.True(t, ok, "rendered response should include usage")
	require.NotContains(t, renderedUsage, "cached_tokens", "OpenAI-shaped usage should not include top-level cached_tokens")
}

// TestHandlerPromotesTopLevelCachedTokens verifies that a StepFun-shaped response
// (top-level usage.cached_tokens, no prompt_tokens_details) has the cached count
// promoted into the nested prompt_tokens_details.cached_tokens field so downstream
// billing applies the cache-hit ratio. StepFun JSON shape per
// https://platform.stepfun.com/docs/zh/guides/developer/prompt-cache.
func TestHandlerPromotesTopLevelCachedTokens(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := map[string]any{
		"choices": []map[string]any{
			{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "hi"},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"cached_tokens":     512,
			"prompt_tokens":     591,
			"completion_tokens": 120,
			"total_tokens":      711,
		},
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(raw)),
	}

	errResp, usage := Handler(c, resp, 591, "step-3.5-flash")
	require.Nil(t, errResp)
	require.NotNil(t, usage)
	require.NotNil(t, usage.PromptTokensDetails, "top-level cached_tokens should populate nested details")
	require.Equal(t, 512, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 591, usage.PromptTokens)
	require.Zero(t, usage.CachedTokens)

	var rendered map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rendered))
	renderedUsage, ok := rendered["usage"].(map[string]any)
	require.True(t, ok, "rendered response should include usage")
	require.NotContains(t, renderedUsage, "cached_tokens", "top-level cached_tokens should be internal-only after normalization")
	renderedDetails, ok := renderedUsage["prompt_tokens_details"].(map[string]any)
	require.True(t, ok, "rendered usage should include prompt token details")
	require.EqualValues(t, 512, renderedDetails["cached_tokens"])
}
