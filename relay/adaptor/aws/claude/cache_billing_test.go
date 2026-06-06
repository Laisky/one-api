package aws

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestBuildClaudeUsageMapsCacheBuckets verifies that the non-streaming AWS Bedrock
// Claude usage builder captures the cache-read and cache-creation buckets that
// Bedrock returns separately from input_tokens. Bedrock's input_tokens EXCLUDES
// cache, so dropping these buckets under-bills the cached/created tokens.
func TestBuildClaudeUsageMapsCacheBuckets(t *testing.T) {
	t.Parallel()

	// Bedrock Claude response shape (Anthropic JSON) with cache buckets and the
	// modern cache_creation ephemeral_5m split.
	body := `{
		"id": "msg_bdrk_1",
		"type": "message",
		"role": "assistant",
		"model": "claude-sonnet-4-5",
		"content": [{"type": "text", "text": "ok"}],
		"stop_reason": "end_turn",
		"usage": {
			"input_tokens": 21,
			"output_tokens": 50,
			"cache_read_input_tokens": 180000,
			"cache_creation_input_tokens": 8000,
			"cache_creation": {"ephemeral_5m_input_tokens": 8000}
		}
	}`

	claudeResponse := new(anthropic.Response)
	require.NoError(t, json.Unmarshal([]byte(body), claudeResponse))

	usage := buildClaudeUsage(claudeResponse)

	require.Equal(t, 21, usage.PromptTokens, "PromptTokens must equal Bedrock input_tokens (excludes cache)")
	require.Equal(t, 50, usage.CompletionTokens)
	require.NotNil(t, usage.PromptTokensDetails, "cache-read tokens must populate PromptTokensDetails")
	require.Equal(t, 180000, usage.PromptTokensDetails.CachedTokens, "cache_read_input_tokens must be billed")
	require.Equal(t, 8000, usage.CacheWrite5mTokens, "cache_creation ephemeral_5m must map to CacheWrite5mTokens")
	require.Equal(t, 0, usage.CacheWrite1hTokens)
}

// TestBuildClaudeUsageLegacyCacheCreationFallback verifies the legacy
// cache_creation_input_tokens field (no ephemeral split) maps to the 5m bucket,
// matching the native Anthropic handler's fallback behavior.
func TestBuildClaudeUsageLegacyCacheCreationFallback(t *testing.T) {
	t.Parallel()

	body := `{
		"id": "msg_bdrk_2",
		"type": "message",
		"role": "assistant",
		"model": "claude-sonnet-4-5",
		"content": [{"type": "text", "text": "ok"}],
		"usage": {
			"input_tokens": 10,
			"output_tokens": 5,
			"cache_read_input_tokens": 1234,
			"cache_creation_input_tokens": 4321
		}
	}`

	claudeResponse := new(anthropic.Response)
	require.NoError(t, json.Unmarshal([]byte(body), claudeResponse))

	usage := buildClaudeUsage(claudeResponse)

	require.NotNil(t, usage.PromptTokensDetails, "cache-read tokens must populate PromptTokensDetails")
	require.Equal(t, 1234, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 4321, usage.CacheWrite5mTokens, "legacy cache_creation_input_tokens must fall back to 5m bucket")
	require.Equal(t, 0, usage.CacheWrite1hTokens)
}

// TestAccumulateClaudeStreamUsageMapsCacheBuckets verifies the streaming AWS Bedrock
// Claude accumulation captures cache buckets from message_start and message_delta
// events, mirroring the native Anthropic stream handler. It drives the real
// StreamResponseClaude2OpenAI transform to obtain the per-event meta that the
// production StreamHandler accumulates, then feeds that meta to the accumulator.
func TestAccumulateClaudeStreamUsageMapsCacheBuckets(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	gmw.SetLogger(c, glog.Shared.Named("aws-claude-stream-test"))

	// message_start carries the cache-read + cache-creation buckets plus the
	// initial input_tokens; message_delta carries the output_tokens.
	startBody := `{
		"type": "message_start",
		"message": {
			"id": "msg_bdrk_3",
			"type": "message",
			"role": "assistant",
			"model": "claude-sonnet-4-5",
			"usage": {
				"input_tokens": 21,
				"output_tokens": 0,
				"cache_read_input_tokens": 180000,
				"cache_creation": {"ephemeral_5m_input_tokens": 8000}
			}
		}
	}`
	deltaBody := `{
		"type": "message_delta",
		"delta": {"stop_reason": "end_turn"},
		"usage": {"output_tokens": 50}
	}`

	var usage relaymodel.Usage

	for _, body := range []string{startBody, deltaBody} {
		streamResp := new(anthropic.StreamResponse)
		require.NoError(t, json.Unmarshal([]byte(body), streamResp))
		_, meta := anthropic.StreamResponseClaude2OpenAI(c, streamResp)
		require.NotNil(t, meta, "expected usage meta for event")
		accumulateClaudeStreamUsage(&usage, meta)
	}

	require.Equal(t, 21, usage.PromptTokens, "PromptTokens must equal Bedrock input_tokens (excludes cache)")
	require.Equal(t, 50, usage.CompletionTokens)
	require.NotNil(t, usage.PromptTokensDetails, "cache-read tokens must populate PromptTokensDetails")
	require.Equal(t, 180000, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 8000, usage.CacheWrite5mTokens)
	require.Equal(t, 0, usage.CacheWrite1hTokens)
}
