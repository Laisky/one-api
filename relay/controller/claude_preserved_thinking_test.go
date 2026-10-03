package controller

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestRewriteAndSanitizeClaudeRequestBody_PreservesBoundThinkingBlocks verifies
// signed, redacted, and future thinking blocks survive request rewriting. It accepts
// the test context and reports all validation failures through that context.
func TestRewriteAndSanitizeClaudeRequestBody_PreservesBoundThinkingBlocks(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"model":"claude-fable-5-1",
		"max_tokens":4096,
		"thinking":{"type":"adaptive","block_binding":{"prefix_mismatch_behavior":"drop_block"}},
		"messages":[
			{"role":"user","content":[{"type":"text","text":"hello"}]},
			{"role":"assistant","content":[
				{"type":"thinking","thinking":"","signature":"sig==","future_field":{"kept":true}},
				{"type":"redacted_thinking","data":"opaque==","future_field":{"kept":true}},
				{"type":"future_thinking","payload":{"kept":true}},
				{"type":"text","text":"done"}
			]},
			{"role":"user","content":[{"type":"text","text":"continue"}]}
		]
	}`)

	result, stats, err := rewriteAndSanitizeClaudeRequestBody(raw, &ClaudeMessagesRequest{Model: "claude-fable-5-1"})
	require.NoError(t, err)
	require.Zero(t, stats.RemovedThinkingBlocks)
	require.Zero(t, stats.RemovedAssistantMessages)

	var root struct {
		Thinking struct {
			BlockBinding struct {
				PrefixMismatchBehavior string `json:"prefix_mismatch_behavior"`
			} `json:"block_binding"`
		} `json:"thinking"`
		Messages []struct {
			Role    string            `json:"role"`
			Content []json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(result, &root))
	require.Equal(t, "drop_block", root.Thinking.BlockBinding.PrefixMismatchBehavior)
	require.Len(t, root.Messages, 3)
	require.Len(t, root.Messages[1].Content, 4)

	wantTypes := []string{"thinking", "redacted_thinking", "future_thinking", "text"}
	for i, rawBlock := range root.Messages[1].Content {
		var block map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(rawBlock, &block))
		var blockType string
		require.NoError(t, json.Unmarshal(block["type"], &blockType))
		require.Equal(t, wantTypes[i], blockType)
		if i == 2 {
			require.Contains(t, block, "payload")
		} else if i < 2 {
			require.Contains(t, block, "future_field")
		}
		if blockType == "thinking" {
			require.Equal(t, `""`, string(block["thinking"]))
			require.Equal(t, `"sig=="`, string(block["signature"]))
		}
		if blockType == "redacted_thinking" {
			require.Equal(t, `"opaque=="`, string(block["data"]))
		}
	}
}

// TestRewriteAndSanitizeClaudeRequestBody_StripsOnlyUnsignedVisibleThinking verifies
// only unsigned visible thinking is removed while opaque redacted blocks remain. It
// accepts the test context and reports all validation failures through that context.
func TestRewriteAndSanitizeClaudeRequestBody_StripsOnlyUnsignedVisibleThinking(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"model":"claude-fable-5-1",
		"max_tokens":1024,
		"messages":[
			{"role":"assistant","content":[
				{"type":"thinking","thinking":"unsigned"},
				{"type":"redacted_thinking","data":"opaque=="},
				{"type":"thinking","thinking":"","signature":"sig=="},
				{"type":"text","text":"answer"}
			]}
		]
	}`)

	result, stats, err := rewriteAndSanitizeClaudeRequestBody(raw, &ClaudeMessagesRequest{Model: "claude-fable-5-1"})
	require.NoError(t, err)
	require.Equal(t, 1, stats.RemovedThinkingBlocks)
	require.Equal(t, []string{"messages[0].content[0]"}, stats.Locations)

	var root struct {
		Messages []struct {
			Content []struct {
				Type      string  `json:"type"`
				Signature *string `json:"signature,omitempty"`
				Data      *string `json:"data,omitempty"`
			} `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(result, &root))
	require.Len(t, root.Messages, 1)
	require.Equal(t, []string{"redacted_thinking", "thinking", "text"}, []string{
		root.Messages[0].Content[0].Type,
		root.Messages[0].Content[1].Type,
		root.Messages[0].Content[2].Type,
	})
	require.NotNil(t, root.Messages[0].Content[0].Data)
	require.Equal(t, "opaque==", *root.Messages[0].Content[0].Data)
	require.NotNil(t, root.Messages[0].Content[1].Signature)
	require.Equal(t, "sig==", *root.Messages[0].Content[1].Signature)
}

// TestRewriteClaudeRequestBody_AdaptiveThinkingPreservesBindingAndFutureFields verifies
// adaptive normalization changes only supported fields and preserves future siblings. It
// accepts the test context and reports all validation failures through that context.
func TestRewriteClaudeRequestBody_AdaptiveThinkingPreservesBindingAndFutureFields(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"model":"claude-fable-5-1",
		"max_tokens":1024,
		"thinking":{
			"type":"enabled",
			"budget_tokens":2048,
			"block_binding":{"prefix_mismatch_behavior":"error","future_option":true},
			"display":"summarized",
			"future_field":{"kept":true}
		},
		"messages":[{"role":"user","content":"hello"}]
	}`)

	result, err := rewriteClaudeRequestBody(raw, &ClaudeMessagesRequest{Model: "claude-fable-5-1"})
	require.NoError(t, err)

	var root map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result, &root))
	var thinking map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(root["thinking"], &thinking))
	require.Equal(t, `"adaptive"`, string(thinking["type"]))
	require.NotContains(t, thinking, "budget_tokens")
	require.Contains(t, thinking, "block_binding")
	require.Contains(t, thinking, "display")
	require.Contains(t, thinking, "future_field")

	var binding map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(thinking["block_binding"], &binding))
	require.Equal(t, `"error"`, string(binding["prefix_mismatch_behavior"]))
	require.Equal(t, "true", string(binding["future_option"]))
}

// TestRewriteClaudeRequestBody_AdaptiveThinkingNullDoesNotPanic verifies an explicit
// JSON null is treated as an absent optional thinking configuration and forwarded. It
// accepts the test context and reports all validation failures through that context.
func TestRewriteClaudeRequestBody_AdaptiveThinkingNullDoesNotPanic(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"model":"claude-fable-5-1",
		"max_tokens":1024,
		"thinking":null,
		"messages":[{"role":"user","content":"hello"}]
	}`)

	result, err := rewriteClaudeRequestBody(raw, &ClaudeMessagesRequest{Model: "claude-fable-5-1"})
	require.NoError(t, err)

	var root map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result, &root))
	require.Equal(t, "null", string(root["thinking"]))
}

// TestStripClaudeThinkingFromAssistantHistory_PreservedThinkingFallback verifies the
// compatibility retry removes all provider-bound thinking while preserving useful output.
// It accepts the test context and reports all validation failures through that context.
func TestStripClaudeThinkingFromAssistantHistory_PreservedThinkingFallback(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"messages":[{"role":"assistant","content":[
			{"type":"thinking","thinking":"","signature":"sig=="},
			{"type":"redacted_thinking","data":"opaque=="},
			{"type":"text","text":"answer"},
			{"type":"tool_use","id":"toolu_1","name":"read","input":{}}
		]}]
	}`)

	result, stats, err := stripClaudeThinkingFromAssistantHistory(raw)
	require.NoError(t, err)
	require.Equal(t, 2, stats.RemovedThinkingBlocks)
	require.Zero(t, stats.RemovedAssistantMessages)
	require.NotContains(t, string(result), `"type":"thinking"`)
	require.NotContains(t, string(result), `"type":"redacted_thinking"`)
	require.Contains(t, string(result), `"type":"text"`)
	require.Contains(t, string(result), `"type":"tool_use"`)
}

// TestShouldRetryClaudeThinkingReplay verifies legacy replay recovery remains enabled
// unless the validated caller explicitly selects strict prefix errors. It accepts the
// test context and reports all validation failures through that context.
func TestShouldRetryClaudeThinkingReplay(t *testing.T) {
	t.Parallel()

	newBindingError := []byte(`{
		"type":"error",
		"error":{
			"type":"invalid_request_error",
			"message":"messages.1.content.0: Invalid ` + "`signature`" + ` in ` + "`thinking`" + ` block. The block is bound to a different conversation."
		}
	}`)
	legacyRequest := &ClaudeMessagesRequest{
		Thinking: &relaymodel.Thinking{Type: "adaptive"},
	}
	explicitErrorRequest := &ClaudeMessagesRequest{
		Thinking: &relaymodel.Thinking{
			Type: "adaptive",
			BlockBinding: &relaymodel.ThinkingBlockBinding{
				PrefixMismatchBehavior: "error",
			},
		},
	}
	dropRequest := &ClaudeMessagesRequest{
		Thinking: &relaymodel.Thinking{
			Type: "adaptive",
			BlockBinding: &relaymodel.ThinkingBlockBinding{
				PrefixMismatchBehavior: "drop_block",
			},
		},
	}

	require.True(t, shouldRetryClaudeInvalidThinkingSignature(http.StatusBadRequest, newBindingError))
	require.True(t, shouldRetryClaudeThinkingReplay(http.StatusBadRequest, newBindingError, legacyRequest))
	require.False(t, shouldRetryClaudeThinkingReplay(http.StatusBadRequest, newBindingError, explicitErrorRequest))
	require.True(t, shouldRetryClaudeThinkingReplay(http.StatusBadRequest, newBindingError, dropRequest))
	require.True(t, shouldRetryClaudeThinkingReplay(http.StatusBadRequest, newBindingError, nil))
	require.False(t, shouldRetryClaudeThinkingReplay(http.StatusInternalServerError, newBindingError, legacyRequest))
}
