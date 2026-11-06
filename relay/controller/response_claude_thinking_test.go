package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// claudeThinkingFixtureBlocks models one Claude turn: summarized thinking,
// redacted thinking, then (after the first tool_use) omitted-display thinking.
func claudeThinkingFixtureBlocks() []relaymodel.ClaudeThinkingBlock {
	return []relaymodel.ClaudeThinkingBlock{
		{Type: "thinking", Thinking: "Plan ahead", Signature: "sig1"},
		{Type: "redacted_thinking", Data: "opaque", AfterText: false},
		{Type: "thinking", Signature: "sig3", ToolCallsBefore: 1, AfterText: true},
	}
}

// requireClaudeEnvelope decodes item's envelope and compares it with want.
func requireClaudeEnvelope(t *testing.T, item openai.OutputItem, want relaymodel.ClaudeThinkingBlock) {
	t.Helper()
	require.Equal(t, "reasoning", item.Type)
	require.True(t, strings.HasPrefix(item.EncryptedContent, relaymodel.ClaudeThinkingEnvelopePrefix), "missing envelope")
	got, err := relaymodel.DecodeClaudeThinkingEnvelope(item.EncryptedContent)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

// TestBuildResponseOutputEmitsClaudeThinkingItems verifies the JSON path emits
// one reasoning item per Claude block, ordered among the function calls, with
// envelopes and a summary only for blocks that carry display text.
func TestBuildResponseOutputEmitsClaudeThinkingItems(t *testing.T) {
	t.Parallel()
	blocks := claudeThinkingFixtureBlocks()
	joined := "Plan ahead"
	choices := []openai_compatible.TextResponseChoice{{
		Message: relaymodel.Message{
			Role:      "assistant",
			Content:   "Running.",
			Reasoning: &joined,
			ToolCalls: []relaymodel.Tool{
				{Id: "toolu_1", Type: "function", Function: &relaymodel.Function{Name: "shell", Arguments: "{}"}},
				{Id: "toolu_2", Type: "function", Function: &relaymodel.Function{Name: "shell", Arguments: "{}"}},
			},
			ClaudeThinkingBlocks: blocks,
		},
		FinishReason: "tool_calls",
	}}
	out := buildResponseOutput(logger.Logger, choices, nil)
	kinds := make([]string, 0, len(out))
	for _, item := range out {
		kinds = append(kinds, item.Type)
	}
	require.Equal(t, []string{"message", "reasoning", "reasoning", "function_call", "reasoning", "function_call"}, kinds)
	requireClaudeEnvelope(t, out[1], blocks[0])
	require.Equal(t, []openai.OutputContent{{Type: "summary_text", Text: "Plan ahead"}}, out[1].Summary)
	require.True(t, strings.HasPrefix(out[1].Id, "rs_"))
	requireClaudeEnvelope(t, out[2], blocks[1])
	require.Empty(t, out[2].Summary)
	requireClaudeEnvelope(t, out[4], blocks[2])
	require.Empty(t, out[4].Summary)
	require.Equal(t, "call_toolu_1", out[3].CallId)
	require.Equal(t, "call_toolu_2", out[5].CallId)
}

// TestRenderChatResponseAsResponseAPIGatesClaudeEnvelopesOnInclude verifies the
// client sees envelopes only after asking for reasoning.encrypted_content,
// while reasoning items themselves are always emitted.
func TestRenderChatResponseAsResponseAPIGatesClaudeEnvelopesOnInclude(t *testing.T) {
	t.Parallel()
	for _, include := range []bool{true, false} {
		c, recorder := newBridgeTestContext(t)
		request := &openai.ResponseAPIRequest{Model: "claude-opus-5-5"}
		if include {
			request.Include = []string{"reasoning.encrypted_content"}
		}
		textResp := &openai_compatible.SlimTextResponse{Choices: []openai_compatible.TextResponseChoice{{
			Message: relaymodel.Message{Role: "assistant", Content: "done", ClaudeThinkingBlocks: []relaymodel.ClaudeThinkingBlock{{Type: "thinking", Signature: "sig1"}}},
		}}}
		require.NoError(t, renderChatResponseAsResponseAPI(c, http.StatusOK, textResp, request, &metalib.Meta{ActualModelName: "claude-opus-5-5"}))
		var response openai.ResponseAPIResponse
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		require.Len(t, response.Output, 2)
		require.Equal(t, "reasoning", response.Output[1].Type)
		require.Contains(t, recorder.Body.String(), `"summary":[]`)
		if include {
			requireClaudeEnvelope(t, response.Output[1], relaymodel.ClaudeThinkingBlock{Type: "thinking", Signature: "sig1"})
		} else {
			require.NotContains(t, recorder.Body.String(), "encrypted_content")
		}
	}
}

// bridgeClaudeBlocksChunk wraps verbatim blocks the way the Claude stream does.
func bridgeClaudeBlocksChunk(blocks ...relaymodel.ClaudeThinkingBlock) *openai_compatible.ChatCompletionsStreamResponse {
	return &openai_compatible.ChatCompletionsStreamResponse{Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{
		{Delta: relaymodel.Message{Role: "assistant", ClaudeThinkingBlocks: blocks}},
	}}
}

// bridgeToolStartChunk starts one streamed tool call.
func bridgeToolStartChunk(index int, id string) *openai_compatible.ChatCompletionsStreamResponse {
	return &openai_compatible.ChatCompletionsStreamResponse{Choices: []openai_compatible.ChatCompletionsStreamResponseChoice{
		{Delta: relaymodel.Message{ToolCalls: []relaymodel.Tool{{Id: id, Type: "function", Index: &index, Function: &relaymodel.Function{Name: "shell", Arguments: "{}"}}}}},
	}}
}

// TestStreamBridgeEmitsClaudeThinkingItems verifies the SSE path emits a
// reasoning output_item.added/done pair per Claude block as soon as the block
// completes (before any function_call is done), with output indexes in Claude
// order and envelopes gated on include but always kept for state.
func TestStreamBridgeEmitsClaudeThinkingItems(t *testing.T) {
	t.Parallel()
	blocks := claudeThinkingFixtureBlocks()
	for _, include := range []bool{true, false} {
		c, recorder := newBridgeTestContext(t)
		request := &openai.ResponseAPIRequest{Model: "claude-opus-5-5"}
		if include {
			request.Include = []string{"reasoning.encrypted_content"}
		}
		bridge := newChatToResponseStreamBridge(c, &metalib.Meta{ActualModelName: "claude-opus-5-5"}, request).(*chatToResponseStreamBridge)
		for _, chunk := range []*openai_compatible.ChatCompletionsStreamResponse{
			bridgeReasoningChunk("Plan "),
			bridgeReasoningChunk("ahead"),
			bridgeClaudeBlocksChunk(blocks[0]),
			bridgeClaudeBlocksChunk(blocks[1]),
			bridgeTextChunk("Running."),
			bridgeToolStartChunk(0, "toolu_1"),
			bridgeClaudeBlocksChunk(blocks[2]),
			bridgeToolStartChunk(1, "toolu_2"),
			bridgeFinishChunk("tool_calls"),
		} {
			bridge.HandleChunk(c, chunk)
		}
		bridge.HandleDone(c)
		require.NoError(t, bridge.StreamWriteError())

		var doneOrder []openai.OutputItem
		var doneIndexes []int
		var completed *openai.ResponseAPIResponse
		for _, event := range parseSSEEvents(recorder.Body.String()) {
			var decoded openai.ResponseAPIStreamEvent
			if event.data == "[DONE]" || json.Unmarshal([]byte(event.data), &decoded) != nil {
				continue
			}
			switch event.event {
			case "response.output_item.done":
				doneOrder = append(doneOrder, *decoded.Item)
				doneIndexes = append(doneIndexes, decoded.OutputIndex)
			case "response.completed":
				completed = decoded.Response
			}
		}
		kinds := make([]string, 0, len(doneOrder))
		for _, item := range doneOrder {
			kinds = append(kinds, item.Type)
		}
		require.Equal(t, []string{"reasoning", "reasoning", "reasoning", "function_call", "function_call", "message"}, kinds,
			"reasoning must be done before the calls it preceded")
		require.Equal(t, []int{1, 2, 4, 3, 5, 0}, doneIndexes)
		require.Equal(t, []openai.OutputContent{{Type: "summary_text", Text: "Plan ahead"}}, doneOrder[0].Summary)
		require.Empty(t, doneOrder[1].Summary)

		require.NotNil(t, completed)
		finalKinds := make([]string, 0, len(completed.Output))
		for _, item := range completed.Output {
			finalKinds = append(finalKinds, item.Type)
		}
		require.Equal(t, []string{"message", "reasoning", "reasoning", "function_call", "reasoning", "function_call"}, finalKinds)

		if include {
			for i, item := range []openai.OutputItem{doneOrder[0], doneOrder[1], doneOrder[2]} {
				requireClaudeEnvelope(t, item, blocks[i])
			}
			requireClaudeEnvelope(t, completed.Output[4], blocks[2])
		} else {
			require.NotContains(t, recorder.Body.String(), "encrypted_content")
		}
		// State persistence always keeps the envelopes.
		sealed := bridge.choices[0].sealedReasoning
		require.Len(t, sealed, 3)
		for i, item := range sealed {
			requireClaudeEnvelope(t, item.item, blocks[i])
		}
	}
}

// TestStreamBridgeRendersWholeClaudeMessageInOrder verifies the non-streaming
// upstream rendered as SSE (one chunk) interleaves reasoning and calls too.
func TestStreamBridgeRendersWholeClaudeMessageInOrder(t *testing.T) {
	t.Parallel()
	c, recorder := newBridgeTestContext(t)
	request := &openai.ResponseAPIRequest{Model: "claude-opus-5-5", Include: []string{"reasoning.encrypted_content"}}
	textResp := &openai_compatible.SlimTextResponse{Choices: []openai_compatible.TextResponseChoice{{
		Message: relaymodel.Message{
			Role:    "assistant",
			Content: "Running.",
			ToolCalls: []relaymodel.Tool{
				{Id: "toolu_1", Type: "function", Function: &relaymodel.Function{Name: "shell", Arguments: "{}"}},
				{Id: "toolu_2", Type: "function", Function: &relaymodel.Function{Name: "shell", Arguments: "{}"}},
			},
			ClaudeThinkingBlocks: claudeThinkingFixtureBlocks(),
		},
		FinishReason: "tool_calls",
	}}}
	require.NoError(t, renderChatResponseAsResponseAPIStream(c, http.StatusOK, textResp, request, &metalib.Meta{ActualModelName: "claude-opus-5-5"}))
	var completed *openai.ResponseAPIResponse
	for _, event := range parseSSEEvents(recorder.Body.String()) {
		if event.event != "response.completed" {
			continue
		}
		var decoded openai.ResponseAPIStreamEvent
		require.NoError(t, json.Unmarshal([]byte(event.data), &decoded))
		completed = decoded.Response
	}
	require.NotNil(t, completed)
	kinds := make([]string, 0, len(completed.Output))
	for _, item := range completed.Output {
		kinds = append(kinds, item.Type)
	}
	require.Equal(t, []string{"message", "reasoning", "reasoning", "function_call", "reasoning", "function_call"}, kinds)
}

// TestClaudeThinkingReplayPolicy verifies only Claude-converting upstreams keep
// gateway blocks, stripping copies messages instead of mutating them, and the
// shared Chat sanitizer removes blocks before any OpenAI-compatible wire.
func TestClaudeThinkingReplayPolicy(t *testing.T) {
	t.Parallel()
	require.True(t, responseFallbackReplaysClaudeThinking(&metalib.Meta{APIType: apitype.Anthropic}))
	require.True(t, responseFallbackReplaysClaudeThinking(&metalib.Meta{APIType: apitype.AwsClaude, ActualModelName: "claude-opus-5-5"}))
	require.False(t, responseFallbackReplaysClaudeThinking(&metalib.Meta{APIType: apitype.AwsClaude, ActualModelName: "llama3-70b"}))
	require.True(t, responseFallbackReplaysClaudeThinking(&metalib.Meta{APIType: apitype.VertexAI, ActualModelName: "claude-sonnet-5"}))
	require.False(t, responseFallbackReplaysClaudeThinking(&metalib.Meta{APIType: apitype.VertexAI, ActualModelName: "gemini-2.5-pro"}))
	require.True(t, responseFallbackReplaysClaudeThinking(&metalib.Meta{APIType: apitype.Azure, ChannelType: channeltype.Azure, ActualModelName: "claude-opus-5-5"}))
	require.False(t, responseFallbackReplaysClaudeThinking(&metalib.Meta{APIType: apitype.Azure, ChannelType: channeltype.Azure, ActualModelName: "gpt-5"}))
	require.False(t, responseFallbackReplaysClaudeThinking(&metalib.Meta{APIType: apitype.DeepSeek}))
	require.False(t, responseFallbackReplaysClaudeThinking(&metalib.Meta{APIType: apitype.OpenAI}))
	require.False(t, responseFallbackReplaysClaudeThinking(nil))

	blocks := []relaymodel.ClaudeThinkingBlock{{Type: "thinking", Signature: "sig1"}}
	original := []relaymodel.Message{{Role: "user", Content: "q"}, {Role: "assistant", Content: "a", ClaudeThinkingBlocks: blocks}}
	stripped, removed := stripClaudeThinkingBlocks(original)
	require.Equal(t, 1, removed)
	require.Empty(t, stripped[1].ClaudeThinkingBlocks)
	require.Equal(t, blocks, original[1].ClaudeThinkingBlocks, "caller-owned messages must not change")

	c, _ := newBridgeTestContext(t)
	request := &relaymodel.GeneralOpenAIRequest{Model: "deepseek-v4", Messages: original}
	applyClaudeThinkingReplayPolicy(c, &metalib.Meta{APIType: apitype.DeepSeek}, nil, request)
	require.Empty(t, request.Messages[1].ClaudeThinkingBlocks)

	sanitized := sanitizeConvertedChatFields(&relaymodel.GeneralOpenAIRequest{Messages: original}).(*relaymodel.GeneralOpenAIRequest)
	encoded, err := json.Marshal(sanitized)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "oneapi_claude_thinking_blocks")
	require.Equal(t, blocks, original[1].ClaudeThinkingBlocks)
}

// TestHydrateCarriesClaudeEnvelopesOnlyToClaude verifies stored or replayed
// reasoning with a gateway envelope survives lowering for a Claude target and
// is still dropped for a Chat target.
func TestHydrateCarriesClaudeEnvelopesOnlyToClaude(t *testing.T) {
	enableStateForTest(t)
	envelope, err := relaymodel.EncodeClaudeThinkingEnvelope(relaymodel.ClaudeThinkingBlock{Type: "thinking", Signature: "sig1"})
	require.NoError(t, err)
	input := func() openai.ResponseAPIInput {
		return openai.ResponseAPIInput{
			map[string]any{"type": "reasoning", "id": "rs_1", "summary": []any{}, "encrypted_content": envelope},
			map[string]any{"type": "reasoning", "id": "rs_2", "summary": []any{}, "encrypted_content": "opaque-openai"},
			"follow up",
		}
	}
	claude, serr := hydrateResponseAPIRequestForFallback(context.Background(), testMeta(), &openai.ResponseAPIRequest{Model: "claude-opus-5-5", Input: input()}, targetClaudeFallback)
	require.Nil(t, serr)
	require.Len(t, claude.Input, 2)
	require.Equal(t, envelope, claude.Input[0].(map[string]any)["encrypted_content"])

	chat, serr := hydrateResponseAPIRequestForFallback(context.Background(), testMeta(), &openai.ResponseAPIRequest{Model: "gpt-5", Input: input()}, targetChatFallback)
	require.Nil(t, serr)
	require.Len(t, chat.Input, 1)
}

// TestResponseFallbackTargetIncludesAzureClaude verifies Azure Foundry Claude
// uses the Claude lowering column.
func TestResponseFallbackTargetIncludesAzureClaude(t *testing.T) {
	t.Parallel()
	require.Equal(t, targetClaudeFallback, responseFallbackTarget(&metalib.Meta{APIType: apitype.Azure, ChannelType: channeltype.Azure, ActualModelName: "claude-opus-5-5"}))
	require.Equal(t, targetChatFallback, responseFallbackTarget(&metalib.Meta{APIType: apitype.Azure, ChannelType: channeltype.Azure, ActualModelName: "gpt-5"}))
}
