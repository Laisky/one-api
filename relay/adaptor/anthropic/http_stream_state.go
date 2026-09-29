package anthropic

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/model"
	"github.com/gin-gonic/gin"
)

type httpContentBlock struct {
	kind      string
	toolIndex int
	arguments bool
	closed    bool
}

type httpStreamState struct {
	native    bool
	model     string
	receipt   httpUsageReceipt
	started   bool
	stopped   bool
	finished  bool
	stopEvent []byte
	terminal  *openai.ChatCompletionsStreamResponse
	blocks    map[int]*httpContentBlock
	tools     int
	id        string
	created   int64
}

// consume validates one Anthropic event, merges receipts, and forwards content.
// Unknown event types remain forward-compatible without bypassing framing checks.
func (s *httpStreamState) consume(c *gin.Context, raw []byte) error {
	var envelope struct {
		Type    string          `json:"type"`
		Message json.RawMessage `json:"message"`
		Usage   json.RawMessage `json:"usage"`
	}
	if err := decodeClaudeJSON(raw, &envelope); err != nil {
		return err
	}
	if envelope.Type == "" || strings.ContainsAny(envelope.Type, "\r\n") {
		return errors.New("invalid Claude event type")
	}
	if envelope.Type == "error" {
		if s.native {
			if err := writeHTTPSSE(c, envelope.Type, raw); err != nil {
				return err
			}
		} else {
			if _, err := c.Writer.Write(append(append([]byte("data: "), raw...), '\n', '\n')); err != nil {
				return errors.Wrap(err, "write Claude stream error")
			}
			c.Writer.Flush()
		}
		return errors.New("Claude returned an in-stream error")
	}
	if s.stopped {
		return errors.New("Claude event received after message_stop")
	}
	native := s.native
	var event StreamResponse
	// The minimal usage envelope is decoded independently from content, so
	// unmodeled native blocks do not force a lossy typed round trip.
	switch envelope.Type {
	case "message_start":
		if s.started {
			return errors.New("duplicate Claude message_start")
		}
		var message struct {
			ID    string          `json:"id"`
			Model string          `json:"model"`
			Usage json.RawMessage `json:"usage"`
		}
		if err := decodeClaudeJSON(envelope.Message, &message); err != nil {
			return err
		}
		if message.ID == "" {
			return errors.New("Claude message_start has no message ID")
		}
		if err := s.receipt.apply(message.Usage); err != nil {
			return err
		}
		s.started = true
		s.model = message.Model
		if native {
			return writeHTTPSSE(c, envelope.Type, raw)
		}
		// Commit the stream after the first verified receipt. A later failure must
		// not look like an unstarted request eligible for transparent replay.
		chunk := &openai.ChatCompletionsStreamResponse{Choices: []openai.ChatCompletionsStreamResponseChoice{{}}}
		chunk.Choices[0].Delta.Role = "assistant"
		return s.writeChat(c, chunk)
	case "message_delta":
		if !s.started {
			return errors.New("Claude delta precedes message_start")
		}
		if err := s.receipt.apply(envelope.Usage); err != nil {
			return err
		}
		if err := decodeClaudeJSON(raw, &event); err != nil {
			return err
		}
		if event.Delta == nil {
			return errors.New("Claude message_delta has no delta")
		}
		if event.Delta.StopReason != nil && *event.Delta.StopReason != "" {
			s.finished = true
		}
		if native {
			return writeHTTPSSE(c, envelope.Type, raw)
		}
		if event.Delta.StopReason != nil && *event.Delta.StopReason != "" {
			s.terminal, _ = StreamResponseClaude2OpenAI(c, &event)
		}
		return nil
	case "message_stop":
		if !s.started || !s.finished {
			return errors.New("Claude message_stop without a final delta")
		}
		for _, block := range s.blocks {
			if !block.closed {
				return errors.New("Claude stopped with an open content block")
			}
		}
		s.stopped = true
		s.stopEvent = bytes.Clone(raw)
		return nil
	case "content_block_start", "content_block_delta", "content_block_stop":
		if !s.started {
			return errors.New("Claude content precedes message_start")
		}
		var content struct {
			Index *int `json:"index"`
			Block *struct {
				Type string `json:"type"`
			} `json:"content_block"`
			Delta *struct {
				Type        string `json:"type"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
		}
		if err := decodeClaudeJSON(raw, &content); err != nil {
			return err
		}
		if content.Index == nil || *content.Index < 0 {
			return errors.New("invalid Claude content index")
		}
		index := *content.Index
		block := s.blocks[index]
		switch envelope.Type {
		case "content_block_start":
			if block != nil || content.Block == nil || content.Block.Type == "" {
				return errors.New("invalid Claude content_block_start")
			}
			block = &httpContentBlock{kind: content.Block.Type, toolIndex: s.tools}
			if block.kind == "tool_use" {
				s.tools++
			}
			s.blocks[index] = block
		case "content_block_delta":
			if block == nil || block.closed || content.Delta == nil {
				return errors.New("Claude delta has no open content block")
			}
			if content.Delta.Type == "input_json_delta" && content.Delta.PartialJSON != "" {
				block.arguments = true
			}
		case "content_block_stop":
			if block == nil || block.closed {
				return errors.New("Claude content stop has no open block")
			}
			block.closed = true
		}
		if native {
			return writeHTTPSSE(c, envelope.Type, raw)
		}
		if envelope.Type == "content_block_stop" {
			if block.kind == "tool_use" && !block.arguments {
				index := block.toolIndex
				chunk := &openai.ChatCompletionsStreamResponse{Choices: []openai.ChatCompletionsStreamResponseChoice{{}}}
				chunk.Choices[0].Delta.ToolCalls = []model.Tool{{Index: &index, Function: &model.Function{Arguments: "{}"}}}
				return s.writeChat(c, chunk)
			}
			return nil
		}
		if err := decodeClaudeJSON(raw, &event); err != nil {
			return err
		}
		chunk, _ := StreamResponseClaude2OpenAI(c, &event)
		if chunk == nil {
			return nil
		}
		for i := range chunk.Choices {
			chunk.Choices[i].FinishReason = nil
			for j := range chunk.Choices[i].Delta.ToolCalls {
				index := block.toolIndex
				call := &chunk.Choices[i].Delta.ToolCalls[j]
				call.Index = &index
				if envelope.Type == "content_block_start" && event.ContentBlock.Input != nil {
					initial, err := json.Marshal(event.ContentBlock.Input)
					if err != nil {
						return errors.Wrap(err, "encode initial Claude tool arguments")
					}
					if string(initial) != "{}" && string(initial) != "null" {
						call.Function.Arguments = string(initial)
						block.arguments = true
					}
				}
			}
		}
		return s.writeChat(c, chunk)
	default:
		if native {
			return writeHTTPSSE(c, envelope.Type, raw)
		}
		return nil
	}
}

// writeChat sends a converted chunk through the existing Responses API bridge.
// It supplies stable metadata and reports downstream errors to the caller.
func (s *httpStreamState) writeChat(c *gin.Context, chunk *openai.ChatCompletionsStreamResponse) error {
	chunk.Id, chunk.Model, chunk.Created = s.id, s.model, s.created
	chunk.Object = "chat.completion.chunk"
	if s.stopped {
		chunk.Usage = s.receipt.snapshot()
	}
	if err := openai_compatible.RenderStreamChunkWithBridge(c, chunk); err != nil {
		return errors.Wrap(err, "write converted Claude stream")
	}
	if writer, ok := c.Writer.(*httpResponseWriter); ok && writer.err != nil {
		return writer.err
	}
	return nil
}

// writeHTTPSSE emits one native Claude event without losing unknown fields.
// Compact removes JSON whitespace that would otherwise break SSE framing.
func writeHTTPSSE(c *gin.Context, kind string, raw []byte) error {
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return errors.Wrap(err, "compact Claude SSE data")
	}
	payload := append([]byte("event: "+kind+"\ndata: "), compact.Bytes()...)
	payload = append(payload, '\n', '\n')
	n, err := c.Writer.Write(payload)
	if err != nil {
		return errors.Wrap(err, "write native Claude SSE")
	}
	if n != len(payload) {
		return errors.WithStack(io.ErrShortWrite)
	}
	c.Writer.Flush()
	return nil
}
