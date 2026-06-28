package openai_compatible

import (
	"encoding/json"

	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/render"
	"github.com/Laisky/one-api/relay/model"
)

// StreamRewriterFromContext returns the Response API stream-rewrite bridge that
// the chat-completions fallback (relayResponseAPIThroughChat) installs in the
// gin context, or nil when the request is a plain Chat Completions stream.
//
// Channel adaptors that emit their own SSE stream (instead of delegating to the
// unified openai_compatible stream handler) MUST consult this so that a
// /v1/responses streaming request routed through the chat fallback receives
// Responses API events (response.output_text.delta / response.completed) rather
// than raw Chat Completions chunks. Skipping it is the class of bug first found
// in the replicate adaptor, where the gptchat UI rendered "(no output)".
func StreamRewriterFromContext(c *gin.Context) StreamRewriteHandler {
	if c == nil {
		return nil
	}
	if v, ok := c.Get(ctxkey.ResponseStreamRewriteHandler); ok {
		if rw, ok := v.(StreamRewriteHandler); ok {
			return rw
		}
	}
	return nil
}

// RenderStreamChunkWithBridge writes a single OpenAI chat-completion stream
// chunk to the client. When a Response API rewrite bridge is active the chunk is
// routed through it (converted into Responses API SSE events); otherwise it is
// emitted verbatim as a `data: {chunk}` SSE line, identical to a direct
// render.ObjectData call.
//
// chunk may be any value that marshals to an OpenAI chat.completion.chunk JSON
// object (e.g. *openai.ChatCompletionsStreamResponse), so adaptors keep using
// their existing chunk type without conversion. The returned error mirrors
// render.ObjectData so existing call sites can keep logging it.
func RenderStreamChunkWithBridge(c *gin.Context, chunk any) error {
	if rw := StreamRewriterFromContext(c); rw != nil {
		if normalized, ok := normalizeStreamChunk(chunk); ok {
			if handled, _ := rw.HandleChunk(c, normalized); handled {
				return nil
			}
		}
	}
	return render.ObjectData(c, chunk)
}

// FinalizeStreamWithBridge emits the terminal SSE events for a stream. With a
// rewrite bridge active it finalizes usage and emits the Responses API
// completion events; otherwise it writes the chat-completions `data: [DONE]`
// sentinel. usage may be nil.
func FinalizeStreamWithBridge(c *gin.Context, usage *model.Usage) {
	if rw := StreamRewriterFromContext(c); rw != nil {
		rw.FinalizeUsage(usage)
		if handled, _ := rw.HandleDone(c); handled {
			return
		}
	}
	render.Done(c)
}

// normalizeStreamChunk converts an arbitrary OpenAI chat-completion chunk value
// into the *ChatCompletionsStreamResponse shape the bridge consumes. It returns
// ok=false when the value cannot be represented as a chat chunk so the caller
// falls back to verbatim rendering.
func normalizeStreamChunk(chunk any) (*ChatCompletionsStreamResponse, bool) {
	switch v := chunk.(type) {
	case nil:
		return nil, false
	case *ChatCompletionsStreamResponse:
		if v == nil {
			return nil, false
		}
		return v, true
	case ChatCompletionsStreamResponse:
		return &v, true
	}

	raw, err := json.Marshal(chunk)
	if err != nil {
		return nil, false
	}
	var out ChatCompletionsStreamResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, false
	}
	return &out, true
}
