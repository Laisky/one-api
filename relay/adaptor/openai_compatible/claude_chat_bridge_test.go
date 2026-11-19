package openai_compatible

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// bridgeTestContext returns a request context and meta for a Claude Messages
// attempt; converted selects whether ConvertClaudeRequest marked it.
func bridgeTestContext(t *testing.T, stream, converted bool) (*gin.Context, *flushRecorder, *meta.Meta) {
	t.Helper()
	c, w := newGinTestContext()
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Set(ctxkey.ClaudeMessagesConversion, converted)
	m := &meta.Meta{Mode: relaymode.ClaudeMessages, IsStream: stream, ActualModelName: "legacy-model", PromptTokens: 7}
	return c, w, m
}

// writeChatJSON is a legacy non-stream handler body writing one chat completion.
func writeChatJSON(c *gin.Context, body string) {
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write([]byte(body))
}

// streamChatChunks is a legacy stream handler body rendering text deltas and
// an optional final chunk through the stream bridge.
func streamChatChunks(c *gin.Context, texts []string, finish bool) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	for _, text := range texts {
		chunk := ChatCompletionsStreamResponse{Object: "chat.completion.chunk", Choices: []ChatCompletionsStreamResponseChoice{{Delta: relaymodel.Message{Content: text}}}}
		_ = RenderStreamChunkWithBridge(c, chunk)
	}
	if finish {
		FinalizeStreamWithBridge(c, nil)
	}
}

// sseEventNames returns the SSE event names of raw in order.
func sseEventNames(raw string) []string {
	var names []string
	for _, line := range strings.Split(raw, "\n") {
		if name, ok := strings.CutPrefix(line, "event: "); ok {
			names = append(names, name)
		}
	}
	return names
}

// TestRenderChatHandlerAsClaudeInactiveIsTransparent verifies the bridge never
// touches a request that is not a converted Claude Messages attempt.
func TestRenderChatHandlerAsClaudeInactiveIsTransparent(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		mode      int
		converted bool
	}{
		{name: "chat completions", mode: relaymode.ChatCompletions, converted: false},
		{name: "claude without conversion flag", mode: relaymode.ClaudeMessages, converted: false},
		{name: "internal chat round keeps flag cleared", mode: relaymode.ChatCompletions, converted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, w, m := bridgeTestContext(t, false, tc.converted)
			m.Mode = tc.mode
			want := &relaymodel.Usage{PromptTokens: 3, CompletionTokens: 1, TotalTokens: 4}
			usage, err := RenderChatHandlerAsClaude(c, m, func() (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
				writeChatJSON(c, `{"object":"chat.completion","choices":[{"message":{"role":"assistant","content":"hi"}}]}`)
				return want, nil
			})
			require.Nil(t, err)
			require.Same(t, want, usage)
			require.JSONEq(t, `{"object":"chat.completion","choices":[{"message":{"role":"assistant","content":"hi"}}]}`, w.Body.String())
		})
	}
}

// TestRenderChatHandlerAsClaudeJSON verifies a captured chat completion becomes
// one Claude message carrying the handler's usage, which is returned as is.
func TestRenderChatHandlerAsClaudeJSON(t *testing.T) {
	t.Parallel()
	c, w, m := bridgeTestContext(t, false, true)
	want := &relaymodel.Usage{PromptTokens: 11, CompletionTokens: 2, TotalTokens: 13}
	usage, err := RenderChatHandlerAsClaude(c, m, func() (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
		writeChatJSON(c, `{"id":"chatcmpl-1","object":"chat.completion","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":"ack","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"q\":1}"}}]}}],"usage":{"prompt_tokens":99,"completion_tokens":99}}`)
		return want, nil
	})
	require.Nil(t, err)
	require.Same(t, want, usage, "billing keeps the handler's usage")
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))
	var reply map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &reply))
	require.Equal(t, "message", reply["type"])
	require.Equal(t, "assistant", reply["role"])
	require.Equal(t, "legacy-model", reply["model"], "an empty upstream model falls back to the mapped model")
	require.Equal(t, "tool_use", reply["stop_reason"])
	content := reply["content"].([]any)
	require.Len(t, content, 2)
	require.Equal(t, map[string]any{"type": "text", "text": "ack"}, content[0])
	tool := content[1].(map[string]any)
	require.Equal(t, "tool_use", tool["type"])
	require.Equal(t, "lookup", tool["name"])
	require.Equal(t, map[string]any{"q": float64(1)}, tool["input"])
	claudeUsage := reply["usage"].(map[string]any)
	require.EqualValues(t, 11, claudeUsage["input_tokens"], "the body presents the billed usage, not the chat body's")
	require.EqualValues(t, 2, claudeUsage["output_tokens"])
	require.NotContains(t, w.Body.String(), "chat.completion")
	_, recorded := c.Get(ctxkey.ConvertedResponseUsage)
	require.False(t, recorded, "no converted receipt may compete with the handler's usage")
}

// TestRenderChatHandlerAsClaudeJSONHandlerError verifies a failed handler
// leaves the client untouched so the controller can render the error.
func TestRenderChatHandlerAsClaudeJSONHandlerError(t *testing.T) {
	t.Parallel()
	c, w, m := bridgeTestContext(t, false, true)
	failure := ErrorWrapper(errors.New("upstream said no"), "upstream_error", http.StatusBadGateway)
	partial := &relaymodel.Usage{PromptTokens: 5}
	usage, err := RenderChatHandlerAsClaude(c, m, func() (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
		writeChatJSON(c, `{"partial":true}`)
		return partial, failure
	})
	require.Same(t, failure, err)
	require.Same(t, partial, usage)
	require.Zero(t, w.Body.Len(), "captured bytes of a failed handler never reach the client")
	require.False(t, c.Writer.Written())
}

// TestRenderChatHandlerAsClaudeJSONUnknownBody verifies output that is not a
// chat completion is forwarded unchanged instead of fabricating a message.
func TestRenderChatHandlerAsClaudeJSONUnknownBody(t *testing.T) {
	t.Parallel()
	c, w, m := bridgeTestContext(t, false, true)
	usage, err := RenderChatHandlerAsClaude(c, m, func() (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
		c.Writer.Header().Set("Content-Type", "text/plain")
		c.Writer.Header().Set("Content-Length", "999")
		c.Writer.WriteHeader(http.StatusAccepted)
		_, _ = c.Writer.WriteString("not json")
		return &relaymodel.Usage{}, nil
	})
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, http.StatusAccepted, w.Code)
	require.Equal(t, "text/plain", w.Header().Get("Content-Type"))
	require.Empty(t, w.Header().Get("Content-Length"), "framing headers of the captured body are not published")
	require.Equal(t, "not json", w.Body.String())
}

// TestRenderChatHandlerAsClaudeStream verifies chat chunks become an ordered
// Claude stream whose terminal events carry the handler's final usage, and
// that the previous stream rewriter is restored.
func TestRenderChatHandlerAsClaudeStream(t *testing.T) {
	t.Parallel()
	c, w, m := bridgeTestContext(t, true, true)
	previous := StreamRewriteHandler(newClaudeChatStreamBridge(c, m))
	c.Set(ctxkey.ResponseStreamRewriteHandler, previous)
	final := &relaymodel.Usage{PromptTokens: 7, CompletionTokens: 3, TotalTokens: 10}
	usage, err := RenderChatHandlerAsClaude(c, m, func() (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
		require.NotSame(t, previous, StreamRewriterFromContext(c), "the Claude bridge replaces any rewriter while streaming")
		streamChatChunks(c, []string{"a", "ck"}, true)
		// Adaptors such as PaLM compute usage only after streaming.
		return final, nil
	})
	require.Nil(t, err)
	require.Same(t, final, usage)
	require.Same(t, previous, StreamRewriterFromContext(c), "the previous rewriter is restored")
	raw := w.Body.String()
	require.NotContains(t, raw, "[DONE]")
	require.NotContains(t, raw, "chat.completion.chunk")
	require.Equal(t, []string{"message_start", "content_block_start", "content_block_delta", "content_block_delta", "content_block_stop", "message_delta", "message_stop"}, sseEventNames(raw))
	events := helperParseSSEDataLines(raw)
	start := events[0]["message"].(map[string]any)
	require.Equal(t, "legacy-model", start["model"])
	require.EqualValues(t, 7, start["usage"].(map[string]any)["input_tokens"])
	delta := events[5]
	require.Equal(t, "end_turn", delta["delta"].(map[string]any)["stop_reason"])
	require.EqualValues(t, 3, delta["usage"].(map[string]any)["output_tokens"], "message_delta carries the final usage")
}

// TestRenderChatHandlerAsClaudeStreamIncomplete verifies an interrupted or
// failed stream is never terminated as a complete Claude message: the client
// gets a Claude error event, and the controller an error result.
func TestRenderChatHandlerAsClaudeStreamIncomplete(t *testing.T) {
	t.Parallel()
	t.Run("handler never finalized", func(t *testing.T) {
		t.Parallel()
		c, w, m := bridgeTestContext(t, true, true)
		_, err := RenderChatHandlerAsClaude(c, m, func() (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
			streamChatChunks(c, []string{"partial"}, false)
			return &relaymodel.Usage{}, nil
		})
		requireIncompleteStream(t, err, w.Body.String())
	})
	t.Run("handler error after content", func(t *testing.T) {
		t.Parallel()
		c, w, m := bridgeTestContext(t, true, true)
		failure := ErrorWrapper(errors.New("read failed"), "read_stream_failed", http.StatusInternalServerError)
		_, err := RenderChatHandlerAsClaude(c, m, func() (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
			streamChatChunks(c, []string{"partial"}, true)
			return &relaymodel.Usage{}, failure
		})
		require.Same(t, failure, err)
		names := sseEventNames(w.Body.String())
		require.Equal(t, "error", names[len(names)-1], "a failed stream ends with a Claude error event")
		require.NotContains(t, names, "message_stop")
	})
	t.Run("handler error before content", func(t *testing.T) {
		t.Parallel()
		c, w, m := bridgeTestContext(t, true, true)
		failure := ErrorWrapper(errors.New("upstream error"), "upstream_error", http.StatusBadGateway)
		_, err := RenderChatHandlerAsClaude(c, m, func() (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
			return nil, failure
		})
		require.Same(t, failure, err)
		require.Zero(t, w.Body.Len(), "a handler failing before streaming leaves the client for the error renderer")
	})
}

// requireIncompleteStream asserts the Claude stream in raw was reported as
// incomplete: an error result, a trailing Claude error event, and no
// message_delta or message_stop.
func requireIncompleteStream(t *testing.T, err *relaymodel.ErrorWithStatusCode, raw string) {
	t.Helper()
	require.NotNil(t, err, "an incomplete stream must be an error result")
	require.Equal(t, "upstream_stream_incomplete", err.Code)
	require.Equal(t, http.StatusBadGateway, err.StatusCode)
	names := sseEventNames(raw)
	require.NotEmpty(t, names)
	require.Equal(t, "message_start", names[0])
	require.Equal(t, "error", names[len(names)-1], "the client must see a Claude error event: %s", raw)
	require.NotContains(t, names, "message_delta")
	require.NotContains(t, names, "message_stop")
}

// TestRenderChatHandlerAsClaudeStreamCompletionRule pins the completion rule:
// complete only without a reported interruption and with either a
// finish_reason or a finalized stream that delivered content.
func TestRenderChatHandlerAsClaudeStreamCompletionRule(t *testing.T) {
	t.Parallel()
	stop := "stop"
	finishChunk := ChatCompletionsStreamResponse{Choices: []ChatCompletionsStreamResponseChoice{{FinishReason: &stop}}}
	t.Run("finish_reason without finalize is complete", func(t *testing.T) {
		t.Parallel()
		c, w, m := bridgeTestContext(t, true, true)
		_, err := RenderChatHandlerAsClaude(c, m, func() (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
			streamChatChunks(c, []string{"ack"}, false)
			_ = RenderStreamChunkWithBridge(c, finishChunk)
			return &relaymodel.Usage{PromptTokens: 7, CompletionTokens: 1}, nil
		})
		require.Nil(t, err)
		require.Contains(t, sseEventNames(w.Body.String()), "message_stop")
	})
	t.Run("finalized after a reported read error is incomplete with usage kept", func(t *testing.T) {
		t.Parallel()
		c, w, m := bridgeTestContext(t, true, true)
		receipt := &relaymodel.Usage{PromptTokens: 7, CompletionTokens: 4}
		usage, err := RenderChatHandlerAsClaude(c, m, func() (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
			streamChatChunks(c, []string{"partial"}, false)
			MarkStreamInterrupted(c)
			FinalizeStreamWithBridge(c, receipt)
			return receipt, nil
		})
		requireIncompleteStream(t, err, w.Body.String())
		require.Same(t, receipt, usage, "the delivered work stays billable")
	})
	t.Run("interruption overrides a synthesized finish_reason", func(t *testing.T) {
		t.Parallel()
		c, w, m := bridgeTestContext(t, true, true)
		_, err := RenderChatHandlerAsClaude(c, m, func() (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
			streamChatChunks(c, []string{"partial"}, false)
			MarkStreamInterrupted(c)
			_ = RenderStreamChunkWithBridge(c, finishChunk)
			FinalizeStreamWithBridge(c, nil)
			return &relaymodel.Usage{}, nil
		})
		requireIncompleteStream(t, err, w.Body.String())
	})
	t.Run("finalized without content is incomplete and unbilled", func(t *testing.T) {
		t.Parallel()
		c, w, m := bridgeTestContext(t, true, true)
		usage, err := RenderChatHandlerAsClaude(c, m, func() (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
			// A JSON error body parses into no chunk at all.
			FinalizeStreamWithBridge(c, nil)
			return &relaymodel.Usage{PromptTokens: 7}, nil
		})
		require.NotNil(t, err)
		require.Equal(t, "upstream_stream_incomplete", err.Code)
		require.Nil(t, usage, "an estimate without delivered output must not settle as success")
		require.Zero(t, w.Body.Len(), "nothing is written, so the controller renders the error")
	})
	t.Run("output receipt without content is still billed", func(t *testing.T) {
		t.Parallel()
		c, _, m := bridgeTestContext(t, true, true)
		receipt := &relaymodel.Usage{PromptTokens: 7, CompletionTokens: 3}
		usage, err := RenderChatHandlerAsClaude(c, m, func() (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
			FinalizeStreamWithBridge(c, nil)
			return receipt, nil
		})
		require.NotNil(t, err)
		require.Same(t, receipt, usage, "a provider-reported output count is never discarded")
	})
	t.Run("chat clients ignore interruption reports", func(t *testing.T) {
		t.Parallel()
		c, w, m := bridgeTestContext(t, true, false)
		_, err := RenderChatHandlerAsClaude(c, m, func() (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
			streamChatChunks(c, []string{"partial"}, false)
			MarkStreamInterrupted(c)
			FinalizeStreamWithBridge(c, nil)
			return &relaymodel.Usage{}, nil
		})
		require.Nil(t, err)
		require.Contains(t, w.Body.String(), "chat.completion.chunk")
		require.Contains(t, w.Body.String(), "data: [DONE]")
	})
}

// TestRenderChatHandlerAsClaudeStreamMissingUsage verifies a complete stream
// whose handler reports no usage settles a flagged estimate of the prompt and
// the streamed output instead of nothing.
func TestRenderChatHandlerAsClaudeStreamMissingUsage(t *testing.T) {
	t.Parallel()
	c, w, m := bridgeTestContext(t, true, true)
	usage, err := RenderChatHandlerAsClaude(c, m, func() (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
		streamChatChunks(c, []string{"hello", " world"}, true)
		return nil, nil
	})
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, "legacy_chat_stream_missing_usage", usage.BillingEstimateReason)
	require.Equal(t, 7, usage.PromptTokens)
	require.Equal(t, CountTokenText("hello world", "legacy-model"), usage.CompletionTokens)
	require.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
	require.Contains(t, sseEventNames(w.Body.String()), "message_stop")
}

// TestRenderChatHandlerAsClaudeRestoresOnPanic verifies a panicking handler
// cannot leave the client writer captured or the Claude rewriter installed,
// so panic recovery reaches the real client.
func TestRenderChatHandlerAsClaudeRestoresOnPanic(t *testing.T) {
	t.Parallel()
	t.Run("json", func(t *testing.T) {
		t.Parallel()
		c, _, m := bridgeTestContext(t, false, true)
		original := c.Writer
		require.Panics(t, func() {
			_, _ = RenderChatHandlerAsClaude(c, m, func() (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
				require.NotSame(t, original, c.Writer, "the handler writes into the capture")
				panic("handler bug")
			})
		})
		require.Same(t, original, c.Writer, "the client writer is restored after a panic")
	})
	t.Run("stream", func(t *testing.T) {
		t.Parallel()
		c, _, m := bridgeTestContext(t, true, true)
		require.Panics(t, func() {
			_, _ = RenderChatHandlerAsClaude(c, m, func() (*relaymodel.Usage, *relaymodel.ErrorWithStatusCode) {
				require.NotNil(t, StreamRewriterFromContext(c))
				panic("handler bug")
			})
		})
		require.Nil(t, StreamRewriterFromContext(c), "the Claude rewriter is removed after a panic")
	})
}

// TestConvertClaudeRequestThroughChat verifies the Claude request reaches the
// provider conversion as a chat request and the attempt is marked converted.
func TestConvertClaudeRequestThroughChat(t *testing.T) {
	t.Parallel()
	c, _, _ := bridgeTestContext(t, false, false)
	request := &relaymodel.ClaudeRequest{Model: "legacy-model", MaxTokens: 32, System: "Be brief", Messages: []relaymodel.ClaudeMessage{{Role: "user", Content: "hello"}}}
	native, err := ConvertClaudeRequestThroughChat(c, request, func(chat *relaymodel.GeneralOpenAIRequest) (any, error) {
		require.Equal(t, "legacy-model", chat.Model)
		require.Len(t, chat.Messages, 2)
		require.Equal(t, "system", chat.Messages[0].Role)
		require.Equal(t, "hello", chat.Messages[1].StringContent())
		return map[string]any{"query": chat.Messages[len(chat.Messages)-1].StringContent()}, nil
	})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"query": "hello"}, native)
	require.True(t, c.GetBool(ctxkey.ClaudeMessagesConversion))

	_, err = ConvertClaudeRequestThroughChat(c, request, func(*relaymodel.GeneralOpenAIRequest) (any, error) {
		return nil, errors.New("provider rejects")
	})
	require.ErrorContains(t, err, "provider rejects")
	_, err = ConvertClaudeRequestThroughChat(c, nil, func(*relaymodel.GeneralOpenAIRequest) (any, error) {
		return nil, nil
	})
	require.Error(t, err)
}

// TestConvertClaudeToolsAndSystemToChat verifies the exported converters keep
// schema-less tools valid and ignore an absent system prompt.
func TestConvertClaudeToolsAndSystemToChat(t *testing.T) {
	t.Parallel()
	tools := ConvertClaudeToolsToChat([]relaymodel.ClaudeTool{
		{Name: "lookup"},
		{Name: "search", InputSchema: map[string]any{"type": "object"}},
		{Type: "web_search_20250305", Name: "web_search"},
	})
	require.Len(t, tools, 3)
	require.Equal(t, map[string]any{}, tools[0].Function.Parameters)
	require.Equal(t, map[string]any{"type": "object"}, tools[1].Function.Parameters)
	require.Nil(t, tools[2].Function)
	require.Nil(t, ConvertClaudeToolsToChat(nil))

	require.Empty(t, ClaudeSystemText(nil))
	require.Equal(t, "Be brief", ClaudeSystemText("Be brief"))
	require.Equal(t, "a\nb", ClaudeSystemText([]any{
		map[string]any{"type": "text", "text": "a"},
		map[string]any{"type": "image"},
		map[string]any{"type": "text", "text": "b"},
	}))
}
