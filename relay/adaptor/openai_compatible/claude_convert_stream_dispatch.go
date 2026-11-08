package openai_compatible

import (
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"

	relaymodel "github.com/Laisky/one-api/relay/model"
)

// claudeOpenAIStreamTranslator routes each upstream SSE payload to the Chat
// Completions or Responses translator. Both share one emitter and one tool id
// allocator, so a stream mixing the two shapes (e.g. Responses deltas followed
// by a Chat usage chunk) still yields one consistent Claude message.
type claudeOpenAIStreamTranslator struct {
	em        *ClaudeStreamEmitter
	chat      *claudeChatStreamTranslator
	responses *claudeResponsesStreamTranslator
	lastChat  bool
}

// newClaudeOpenAIStreamTranslator returns a translator writing to em.
func newClaudeOpenAIStreamTranslator(c *gin.Context, em *ClaudeStreamEmitter) *claudeOpenAIStreamTranslator {
	ids := NewClaudeToolIDAllocator()
	return &claudeOpenAIStreamTranslator{
		em:        em,
		chat:      newClaudeChatStreamTranslator(c, em, ids),
		responses: newClaudeResponsesStreamTranslator(c, em, ids),
	}
}

// claudeStreamPayloadProbe holds the discriminating fields of a payload.
type claudeStreamPayloadProbe struct {
	Type    string          `json:"type"`
	Object  string          `json:"object"`
	Choices json.RawMessage `json:"choices"`
}

// consume translates one SSE data payload; malformed payloads are skipped.
func (t *claudeOpenAIStreamTranslator) consume(payload string) {
	var probe claudeStreamPayloadProbe
	if err := json.Unmarshal([]byte(payload), &probe); err != nil {
		return
	}
	switch {
	case strings.HasPrefix(probe.Type, "response.") || probe.Type == "error":
		var event responseAPIStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return
		}
		t.lastChat = false
		t.responses.consume(&event)
	case probe.Object == "response" && len(probe.Choices) == 0:
		// A bare terminal Responses object without an event envelope.
		var resp responseAPIResponse
		if err := json.Unmarshal([]byte(payload), &resp); err != nil {
			return
		}
		t.lastChat = false
		t.responses.consumeTerminal(&resp, "")
	case probe.Type == "":
		var chunk ChatCompletionsStreamResponse
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return
		}
		if len(chunk.Choices) > 0 {
			t.lastChat = true
		}
		t.chat.consume(&chunk)
	}
}

// completed reports whether the upstream signalled the end of the message.
func (t *claudeOpenAIStreamTranslator) completed() bool {
	return t.chat.finishSeen || t.responses.terminal
}

// settledByReceipt reports whether the upstream ended with a terminal
// Responses event carrying its final usage receipt. Such a stream is complete
// for billing even though Responses never sends [DONE].
func (t *claudeOpenAIStreamTranslator) settledByReceipt() bool {
	return t.responses.receipt
}

// finish flushes pending tool calls and terminates the Claude message with the
// stop_reason of the protocol that ended the stream and the given usage.
func (t *claudeOpenAIStreamTranslator) finish(usage relaymodel.ClaudeUsage) {
	t.chat.flushPendingTools()
	t.responses.flushPendingTools()
	stopReason := t.chat.finalStopReason()
	if t.responses.terminal && !(t.lastChat && t.chat.finishSeen) {
		stopReason = t.responses.finalStopReason()
	}
	t.em.Finish(stopReason, usage)
}
