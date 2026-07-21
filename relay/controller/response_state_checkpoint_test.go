package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/state"
)

func checkpointMeta() *metalib.Meta {
	m := testMeta()
	m.OriginModelName = "gpt-5"
	m.ActualModelName = "gpt-5-2025"
	return m
}

// TestChatCheckpointMapperDeterminism proves the transcript mapper is stable and
// discriminating: identical visible transcripts hash equal, and a one-byte content
// change, a changed tool-call id, or a different signature each change the key
// (CP02-CP04).
func TestChatCheckpointMapperDeterminism(t *testing.T) {
	owner := state.OwnerScope{UserID: 1, TokenID: 1}
	binding := &state.ProviderBinding{ChannelID: 1, APIType: 0, ActualModel: "gpt-5-2025"}

	base := []relaymodel.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}
	keyOf := func(msgs []relaymodel.Message) string {
		cm := chatMessagesToCheckpoint(msgs)
		return state.CheckpointKeyAt(owner, "chat", "gpt-5", binding, cm, len(cm))
	}

	k1 := keyOf(base)
	k2 := keyOf([]relaymodel.Message{{Role: "user", Content: "hello"}, {Role: "assistant", Content: "hi there"}})
	require.Equal(t, k1, k2, "identical transcripts must hash equal")

	// CP03: one-byte content change.
	require.NotEqual(t, k1, keyOf([]relaymodel.Message{{Role: "user", Content: "hellO"}, {Role: "assistant", Content: "hi there"}}))

	// CP04: changed tool-call id.
	sig := "sig-abc"
	withTool := []relaymodel.Message{{Role: "assistant", ToolCallId: "call_1"}}
	withTool2 := []relaymodel.Message{{Role: "assistant", ToolCallId: "call_2"}}
	require.NotEqual(t, keyOf(withTool), keyOf(withTool2))

	// CP02: signature participates.
	withSig := []relaymodel.Message{{Role: "assistant", Content: "x", Signature: &sig}}
	require.NotEqual(t, keyOf([]relaymodel.Message{{Role: "assistant", Content: "x"}}), keyOf(withSig))
}

// TestChatCheckpointRecordThenMatch proves the end-to-end optimization: a recorded
// checkpoint for [user, assistant] lets the next request [user, assistant, user']
// continue from the upstream handle and send only the delta (M02, CP01).
func TestChatCheckpointRecordThenMatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableStateForTest(t)
	meta := checkpointMeta()

	// --- Turn 1: record the checkpoint for [user, assistant] -> upstream handle ---
	w1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(w1)
	c1.Set(ctxkey.ResponseAPIUpstreamID, "resp_upstream_turn1")
	c1.Set(ctxkey.ResponseAPIAssistantMessage, relaymodel.Message{Role: "assistant", Content: "hi there"})

	turn1 := &relaymodel.GeneralOpenAIRequest{
		Model:    "gpt-5",
		Messages: []relaymodel.Message{{Role: "user", Content: "hello"}},
	}
	recordChatCheckpoint(c1, meta, turn1)

	// --- Turn 2: [user, assistant, user'] should match the prefix and send delta ---
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	convReq := &openai.ResponseAPIRequest{
		Model: "gpt-5-2025",
		Input: openai.ResponseAPIInput{"hello", "hi there", "tell me more"},
	}
	c2.Set(ctxkey.ConvertedRequest, convReq)

	turn2 := &relaymodel.GeneralOpenAIRequest{
		Model: "gpt-5",
		Messages: []relaymodel.Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
			{Role: "user", Content: "tell me more"},
		},
	}

	body, matched := matchChatCheckpoint(c2, meta, turn2)
	require.True(t, matched, "an exact prefix must match and shortcut to the upstream handle")
	require.NotNil(t, body)
	require.NotNil(t, convReq.PreviousResponseId)
	require.Equal(t, "resp_upstream_turn1", *convReq.PreviousResponseId, "must continue from the bound upstream handle")
	require.NotEmpty(t, convReq.Input, "the delta (only the new user turn) must be sent")
	require.Less(t, len(convReq.Input), 3, "the matched prefix must be trimmed from the outgoing input")
}

// TestChatCheckpointNoMatchIsFailOpen proves a one-byte change or a disabled
// feature yields no match, so the caller performs a full explicit replay (CP03,
// CP10).
func TestChatCheckpointNoMatchIsFailOpen(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableStateForTest(t)
	meta := checkpointMeta()

	c1, _ := gin.CreateTestContext(httptest.NewRecorder())
	c1.Set(ctxkey.ResponseAPIUpstreamID, "resp_up")
	c1.Set(ctxkey.ResponseAPIAssistantMessage, relaymodel.Message{Role: "assistant", Content: "hi"})
	recordChatCheckpoint(c1, meta, &relaymodel.GeneralOpenAIRequest{Model: "gpt-5", Messages: []relaymodel.Message{{Role: "user", Content: "hello"}}})

	// A one-byte change in the prior turn breaks the prefix hash -> no match.
	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Set(ctxkey.ConvertedRequest, &openai.ResponseAPIRequest{Model: "gpt-5-2025", Input: openai.ResponseAPIInput{"x"}})
	_, matched := matchChatCheckpoint(c2, meta, &relaymodel.GeneralOpenAIRequest{
		Model: "gpt-5",
		Messages: []relaymodel.Message{
			{Role: "user", Content: "hellO"}, // changed
			{Role: "assistant", Content: "hi"},
			{Role: "user", Content: "next"},
		},
	})
	require.False(t, matched, "a changed prior turn must not match (fail open to full replay)")
}

// TestClaudeCheckpointRecordThenMatch proves the Claude path (M08) mirrors the
// Chat path: a recorded checkpoint for [user, assistant] lets the next Claude
// request [user, assistant, user'] continue from the upstream handle and send only
// the delta.
func TestClaudeCheckpointRecordThenMatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableStateForTest(t)
	meta := checkpointMeta()

	// --- Turn 1: record [user, assistant] -> upstream handle ---
	c1, _ := gin.CreateTestContext(httptest.NewRecorder())
	c1.Set(ctxkey.ResponseAPIUpstreamID, "resp_claude_turn1")
	c1.Set(ctxkey.ResponseAPIAssistantMessage, relaymodel.Message{Role: "assistant", Content: "hi there"})
	recordClaudeCheckpoint(c1, meta, &relaymodel.ClaudeRequest{
		Model:    "claude-sonnet-4-5",
		Messages: []relaymodel.ClaudeMessage{{Role: "user", Content: "hello"}},
	})

	// --- Turn 2: [user, assistant, user'] must match and send only the delta ---
	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	convReq := &openai.ResponseAPIRequest{
		Model: "gpt-5-2025",
		Input: openai.ResponseAPIInput{"hello", "hi there", "tell me more"},
	}
	c2.Set(ctxkey.ConvertedRequest, convReq)

	body, matched := matchClaudeCheckpoint(c2, meta, &relaymodel.ClaudeRequest{
		Model: "claude-sonnet-4-5",
		Messages: []relaymodel.ClaudeMessage{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
			{Role: "user", Content: "tell me more"},
		},
	})
	require.True(t, matched, "an exact prefix must match and shortcut to the upstream handle")
	require.NotNil(t, body)
	require.NotNil(t, convReq.PreviousResponseId)
	require.Equal(t, "resp_claude_turn1", *convReq.PreviousResponseId)
	require.NotEmpty(t, convReq.Input, "the delta (only the new user turn) must be sent")
	require.Less(t, len(convReq.Input), 3, "the matched prefix must be trimmed from the outgoing input")
}

// TestClaudeChatCheckpointsAreIsolated proves a Chat checkpoint never satisfies a
// Claude request and vice versa: the client family is part of the key (CP06).
func TestClaudeChatCheckpointsAreIsolated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enableStateForTest(t)
	meta := checkpointMeta()

	// Record a CHAT checkpoint.
	c1, _ := gin.CreateTestContext(httptest.NewRecorder())
	c1.Set(ctxkey.ResponseAPIUpstreamID, "resp_chat")
	c1.Set(ctxkey.ResponseAPIAssistantMessage, relaymodel.Message{Role: "assistant", Content: "hi there"})
	recordChatCheckpoint(c1, meta, &relaymodel.GeneralOpenAIRequest{Model: "gpt-5", Messages: []relaymodel.Message{{Role: "user", Content: "hello"}}})

	// A CLAUDE request with the same visible transcript must NOT match it.
	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Set(ctxkey.ConvertedRequest, &openai.ResponseAPIRequest{Model: "gpt-5-2025", Input: openai.ResponseAPIInput{"a", "b", "c"}})
	_, matched := matchClaudeCheckpoint(c2, meta, &relaymodel.ClaudeRequest{
		Model: "claude-sonnet-4-5",
		Messages: []relaymodel.ClaudeMessage{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
			{Role: "user", Content: "tell me more"},
		},
	})
	require.False(t, matched, "a Chat checkpoint must never satisfy a Claude request (CP06)")
}
