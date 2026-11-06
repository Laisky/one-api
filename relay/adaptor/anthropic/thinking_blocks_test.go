package anthropic

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/model"
)

// newThinkingBlocksTestContext returns a context that optionally behaves like a
// Responses fallback request (the only consumer of thinking blocks).
func newThinkingBlocksTestContext(t *testing.T, responsesFallback bool) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	gmw.SetLogger(c, glog.Shared.Named("anthropic-thinking-blocks-test"))
	if responsesFallback {
		c.Set(ctxkey.ResponseAPIRequestOriginal, struct{}{})
	}
	return c, recorder
}

// strPtr returns a pointer to value for building Claude content fixtures.
func strPtr(value string) *string {
	return &value
}

// TestResponseClaude2OpenAIAttachesThinkingBlocks verifies a non-streaming
// Claude message hands every thinking/redacted block, with its position, to
// the Responses bridge, and that plain Chat clients never see the field.
func TestResponseClaude2OpenAIAttachesThinkingBlocks(t *testing.T) {
	t.Parallel()
	response := &Response{Model: "claude-opus-5-5", Content: []Content{
		{Type: "thinking", Thinking: strPtr(""), Signature: strPtr("sig1")},
		{Type: "text", Text: "Checking."},
		{Type: "tool_use", Id: "toolu_1", Name: "shell", Input: map[string]any{"cmd": "ls"}},
		{Type: "redacted_thinking", Data: strPtr("redacted-data")},
		{Type: "thinking", Thinking: strPtr("unsigned is dropped")},
		{Type: "tool_use", Id: "toolu_2", Name: "shell", Input: map[string]any{"cmd": "pwd"}},
	}}

	c, _ := newThinkingBlocksTestContext(t, true)
	converted := ResponseClaude2OpenAI(c, response)
	require.Len(t, converted.Choices, 1)
	require.Equal(t, []model.ClaudeThinkingBlock{
		{Type: "thinking", Signature: "sig1"},
		{Type: "redacted_thinking", Data: "redacted-data", ToolCallsBefore: 1, AfterText: true},
	}, converted.Choices[0].Message.ClaudeThinkingBlocks)

	chat, _ := newThinkingBlocksTestContext(t, false)
	plain := ResponseClaude2OpenAI(chat, response)
	require.Empty(t, plain.Choices[0].Message.ClaudeThinkingBlocks)
	encoded, err := json.Marshal(plain)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "oneapi_claude_thinking_blocks")
}

// TestStreamHandlerEmitsCompletedThinkingBlocks verifies streamed thinking is
// rebuilt exactly (empty omitted-display text plus signature, and redacted
// data) and emitted once per block at content_block_stop, only for Responses.
func TestStreamHandlerEmitsCompletedThinkingBlocks(t *testing.T) {
	t.Parallel()
	sse := strings.Join([]string{
		`data: {"type":"message_start","message":{"id":"msg_think","type":"message","role":"assistant","model":"claude-opus-5-5","usage":{"input_tokens":7,"output_tokens":0}}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig1"}}`,
		`data: {"type":"content_block_stop","index":0}`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"redacted_thinking","data":"opaque"}}`,
		`data: {"type":"content_block_stop","index":1}`,
		`data: {"type":"content_block_start","index":2,"content_block":{"type":"thinking","thinking":""}}`,
		`data: {"type":"content_block_delta","index":2,"delta":{"type":"thinking_delta","thinking":"Plan "}}`,
		`data: {"type":"content_block_delta","index":2,"delta":{"type":"thinking_delta","thinking":"carefully."}}`,
		`data: {"type":"content_block_delta","index":2,"delta":{"type":"signature_delta","signature":"sig2"}}`,
		`data: {"type":"content_block_stop","index":2}`,
		`data: {"type":"content_block_start","index":3,"content_block":{"type":"tool_use","id":"toolu_1","name":"shell","input":{}}}`,
		`data: {"type":"content_block_delta","index":3,"delta":{"type":"input_json_delta","partial_json":"{}"}}`,
		`data: {"type":"content_block_stop","index":3}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":9}}`,
		`data: {"type":"message_stop"}`,
	}, "\n\n") + "\n\n"

	run := func(responsesFallback bool) []model.ClaudeThinkingBlock {
		c, recorder := newThinkingBlocksTestContext(t, responsesFallback)
		resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(sse)), Header: http.Header{"Content-Type": {"text/event-stream"}}}
		errResp, usage := StreamHandler(c, resp)
		require.Nil(t, errResp)
		require.NotNil(t, usage)
		require.Equal(t, 9, usage.CompletionTokens, "thinking plumbing must not change usage")
		var blocks []model.ClaudeThinkingBlock
		for _, line := range strings.Split(recorder.Body.String(), "\n") {
			payload, ok := strings.CutPrefix(line, "data: ")
			if !ok || payload == "[DONE]" {
				continue
			}
			var chunk struct {
				Choices []struct {
					Delta model.Message `json:"delta"`
				} `json:"choices"`
			}
			require.NoError(t, json.Unmarshal([]byte(payload), &chunk))
			for _, choice := range chunk.Choices {
				blocks = append(blocks, choice.Delta.ClaudeThinkingBlocks...)
			}
		}
		return blocks
	}

	require.Equal(t, []model.ClaudeThinkingBlock{
		{Type: "thinking", Signature: "sig1"},
		{Type: "redacted_thinking", Data: "opaque"},
		{Type: "thinking", Thinking: "Plan carefully.", Signature: "sig2"},
	}, run(true))
	require.Empty(t, run(false), "plain Chat streams must not carry gateway thinking blocks")
}

// TestThinkingBlockTrackerDropsOversizedBlock verifies the stream accumulator
// bounds memory and never emits a block that cannot fit in an envelope.
func TestThinkingBlockTrackerDropsOversizedBlock(t *testing.T) {
	t.Parallel()
	c, _ := newThinkingBlocksTestContext(t, true)
	tracker := NewThinkingBlockTracker(c)
	require.NotNil(t, tracker)
	tracker.Observe(0, &StreamResponse{Type: "content_block_start", ContentBlock: &Content{Type: "thinking", Thinking: strPtr("")}})
	huge := strings.Repeat("x", model.MaxClaudeThinkingEnvelopeBytes+1)
	tracker.Observe(0, &StreamResponse{Type: "content_block_delta", Delta: &Delta{Type: "thinking_delta", Thinking: &huge}})
	tracker.Observe(0, &StreamResponse{Type: "content_block_delta", Delta: &Delta{Type: "signature_delta", Signature: strPtr("sig")}})
	require.Nil(t, tracker.Complete(0))

	var nilTracker *ThinkingBlockTracker
	nilTracker.Observe(0, &StreamResponse{Type: "content_block_start"})
	require.Nil(t, nilTracker.Complete(0))
}

// TestInsertClaudeThinkingBlocksRestoresPositions verifies replayed blocks are
// placed before the text or tool_use they preceded without reordering blocks.
func TestInsertClaudeThinkingBlocksRestoresPositions(t *testing.T) {
	t.Parallel()
	contents := []Content{{Type: "text", Text: "a"}, {Type: "tool_use", Id: "t1"}, {Type: "tool_use", Id: "t2"}}
	blocks := []model.ClaudeThinkingBlock{
		{Type: "thinking", Signature: "before-text"},
		{Type: "thinking", Signature: "after-text", AfterText: true},
		{Type: "redacted_thinking", Data: "before-t2", ToolCallsBefore: 1, AfterText: true},
		{Type: "thinking", Signature: "trailing", ToolCallsBefore: 2, AfterText: true},
	}
	out := insertClaudeThinkingBlocks(contents, blocks)
	order := make([]string, 0, len(out))
	for _, content := range out {
		switch {
		case content.Signature != nil:
			order = append(order, *content.Signature)
		case content.Data != nil:
			order = append(order, *content.Data)
		case content.Id != "":
			order = append(order, content.Id)
		default:
			order = append(order, content.Type)
		}
	}
	require.Equal(t, []string{"before-text", "text", "after-text", "t1", "before-t2", "t2", "trailing"}, order)
	require.Equal(t, contents, insertClaudeThinkingBlocks(contents, nil))
}

// TestConvertRequestReplaysGatewayThinkingBlocks verifies verbatim blocks are
// replayed in place (thinking before the tool_use it led to), even when the
// request carries no thinking config (always-thinking Opus), and supersede
// both the signature cache and the <think> text fallback.
func TestConvertRequestReplaysGatewayThinkingBlocks(t *testing.T) {
	t.Parallel()
	c, _ := newThinkingBlocksTestContext(t, true)
	summary := "summarized thinking"
	request := model.GeneralOpenAIRequest{
		Model:     "claude-opus-5-5",
		MaxTokens: 4096,
		Messages: []model.Message{
			{Role: "user", Content: "run it"},
			{
				Role:             "assistant",
				Content:          "",
				ReasoningContent: &summary,
				ToolCalls: []model.Tool{
					{Id: "toolu_1", Type: "function", Function: &model.Function{Name: "shell", Arguments: `{"cmd":"ls"}`}},
					{Id: "toolu_2", Type: "function", Function: &model.Function{Name: "shell", Arguments: `{"cmd":"pwd"}`}},
				},
				ClaudeThinkingBlocks: []model.ClaudeThinkingBlock{
					{Type: "thinking", Signature: "sig1"},
					{Type: "redacted_thinking", Data: "opaque", ToolCallsBefore: 1},
				},
			},
			{Role: "tool", ToolCallId: "toolu_1", Content: "a"},
			{Role: "tool", ToolCallId: "toolu_2", Content: "b"},
		},
	}
	converted, err := ConvertRequest(c, request)
	require.NoError(t, err)
	require.Len(t, converted.Messages, 4)
	assistant := converted.Messages[1].Content
	require.Len(t, assistant, 4)
	require.Equal(t, "thinking", assistant[0].Type)
	require.Equal(t, "sig1", *assistant[0].Signature)
	require.Empty(t, *assistant[0].Thinking)
	require.Equal(t, "tool_use", assistant[1].Type)
	require.Equal(t, "redacted_thinking", assistant[2].Type)
	require.Equal(t, "opaque", *assistant[2].Data)
	require.Equal(t, "tool_use", assistant[3].Type)

	encoded, err := json.Marshal(converted.Messages[1])
	require.NoError(t, err)
	require.Contains(t, string(encoded), `{"type":"thinking","thinking":"","signature":"sig1"}`)
	require.NotContains(t, string(encoded), "<think>")
	require.NotContains(t, string(encoded), "oneapi_claude_thinking_blocks")
}

// TestConvertRequestSkipsGatewayThinkingWhenUnsupported verifies blocks are not
// sent to models without thinking or when thinking is explicitly disabled.
func TestConvertRequestSkipsGatewayThinkingWhenUnsupported(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		model    string
		thinking *model.Thinking
	}{
		{name: "no_thinking_model", model: "claude-3-5-haiku"},
		{name: "explicitly_disabled", model: "claude-sonnet-4-5", thinking: &model.Thinking{Type: "disabled"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newThinkingBlocksTestContext(t, true)
			request := model.GeneralOpenAIRequest{
				Model:     tc.model,
				MaxTokens: 4096,
				Thinking:  tc.thinking,
				Messages: []model.Message{
					{Role: "user", Content: "hi"},
					{Role: "assistant", Content: "hello", ClaudeThinkingBlocks: []model.ClaudeThinkingBlock{{Type: "thinking", Signature: "sig1"}}},
					{Role: "user", Content: "again"},
				},
			}
			converted, err := ConvertRequest(c, request)
			require.NoError(t, err)
			for _, content := range converted.Messages[1].Content {
				require.NotEqual(t, "thinking", content.Type)
			}
		})
	}
}
