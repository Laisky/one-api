package aws

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/tracing"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/adaptor/aws/utils"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

type invokeContentBlock struct {
	kind      string
	toolIndex int
	arguments bool
	closed    bool
}

type invokeStreamState struct {
	receipt   invokeUsageReceipt
	started   bool
	stopped   bool
	finished  bool
	stopEvent []byte
	terminal  *openai.ChatCompletionsStreamResponse
	blocks    map[int]*invokeContentBlock
	tools     int
	id        string
	created   int64
}

// StreamHandler reads AWS EventStream frames synchronously with cancellation.
// It returns SDK/protocol/write failures with any verified receipt. Successful
// terminal markers are withheld until message_stop AND a clean SDK EOF.
func StreamHandler(c *gin.Context, client *bedrockruntime.Client) (result *relaymodel.ErrorWithStatusCode, usage *relaymodel.Usage) {
	target, err := resolveClaudeInvokeModel(c, client)
	if err != nil {
		return invokeError(err), nil
	}
	body, err := claudeInvokeBody(c)
	if err != nil {
		return invokeError(err), nil
	}
	started := time.Now()
	response, err := client.InvokeModelWithResponseStream(gmw.Ctx(c), &bedrockruntime.InvokeModelWithResponseStreamInput{
		ModelId: aws.String(target), Accept: aws.String("application/json"), ContentType: aws.String("application/json"), Body: body,
	})
	utils.UpdateRegionHealthMetrics(client.Options().Region, err == nil, time.Since(started), err)
	if err != nil {
		return invokeError(err), nil
	}
	// An accepted stream with no readable receipt is unknown cost, not a
	// confirmed free rejection. A non-nil estimated receipt makes existing
	// controller settlement retain the reservation and prevent replay.
	defer func() {
		if result != nil && usage == nil {
			usage = &relaymodel.Usage{BillingEstimateReason: "missing_usage_after_accepted_aws_claude_invoke"}
		}
	}()
	if response == nil || response.GetStream() == nil {
		return invokeError(errors.New("missing Claude EventStream")), nil
	}
	stream := response.GetStream()
	closed := false
	defer func() {
		if !closed {
			if err := stream.Close(); err != nil && result == nil {
				result = invokeError(errors.Wrap(err, "close Claude EventStream"))
			}
		}
	}()
	original := c.Writer
	writer := &invokeResponseWriter{ResponseWriter: original}
	c.Writer = writer
	defer func() { c.Writer = original }()
	common.SetEventStreamHeaders(c)
	state := &invokeStreamState{blocks: map[int]*invokeContentBlock{}, id: tracing.GenerateChatCompletionID(c), created: helper.GetTimestamp()}
	for {
		if err := gmw.Ctx(c).Err(); err != nil {
			return invokeError(errors.WithStack(err)), state.receipt.snapshot()
		}
		select {
		case <-gmw.Ctx(c).Done():
			return invokeError(errors.WithStack(gmw.Ctx(c).Err())), state.receipt.snapshot()
		case event, ok := <-stream.Events():
			if !ok {
				if err := stream.Err(); err != nil {
					return invokeError(errors.Wrap(err, "read Claude EventStream")), state.receipt.snapshot()
				}
				if !state.stopped || !state.finished || state.receipt.input == nil || state.receipt.output == nil {
					return invokeError(errors.Wrap(io.ErrUnexpectedEOF, "incomplete Claude stream")), state.receipt.snapshot()
				}
				err := stream.Close()
				closed = true
				if err != nil {
					return invokeError(errors.Wrap(err, "close Claude EventStream")), state.receipt.snapshot()
				}
				if err := gmw.Ctx(c).Err(); err != nil {
					return invokeError(errors.WithStack(err)), state.receipt.snapshot()
				}
				usage = state.receipt.snapshot()
				if c.GetBool(ctxkey.ClaudeMessagesNative) {
					if err := writeInvokeSSE(c, "message_stop", state.stopEvent); err != nil {
						return invokeError(err), usage
					}
				} else {
					if state.terminal != nil {
						if err := state.writeChat(c, state.terminal); err != nil {
							return invokeError(err), usage
						}
					}
					openai_compatible.FinalizeStreamWithBridge(c, usage)
					if writer.err != nil {
						return invokeError(writer.err), usage
					}
				}
				return nil, usage
			}
			chunk, ok := event.(*types.ResponseStreamMemberChunk)
			if !ok || chunk == nil {
				return invokeError(errors.New("unsupported Claude EventStream frame")), state.receipt.snapshot()
			}
			if err := state.consume(c, chunk.Value.Bytes); err != nil {
				return invokeError(err), state.receipt.snapshot()
			}
			if writer.err != nil {
				return invokeError(writer.err), state.receipt.snapshot()
			}
		}
	}
}

// consume validates one Anthropic event, merges receipts, and forwards content.
// Unknown event types remain forward-compatible without bypassing framing checks.
func (s *invokeStreamState) consume(c *gin.Context, raw []byte) error {
	var envelope struct {
		Type    string          `json:"type"`
		Message json.RawMessage `json:"message"`
		Usage   json.RawMessage `json:"usage"`
	}
	if err := decodeInvokeJSON(raw, &envelope); err != nil {
		return err
	}
	if envelope.Type == "" || strings.ContainsAny(envelope.Type, "\r\n") {
		return errors.New("invalid Claude event type")
	}
	if envelope.Type == "error" {
		return errors.New("Claude returned an in-stream error")
	}
	if s.stopped {
		return errors.New("Claude event received after message_stop")
	}
	native := c.GetBool(ctxkey.ClaudeMessagesNative)
	var event anthropic.StreamResponse
	// The minimal usage envelope is decoded independently from content, so
	// unmodeled native blocks do not force a lossy typed round trip.
	switch envelope.Type {
	case "message_start":
		if s.started {
			return errors.New("duplicate Claude message_start")
		}
		var message struct {
			ID    string          `json:"id"`
			Usage json.RawMessage `json:"usage"`
		}
		if err := decodeInvokeJSON(envelope.Message, &message); err != nil {
			return err
		}
		if message.ID == "" {
			return errors.New("Claude message_start has no message ID")
		}
		if err := s.receipt.apply(message.Usage); err != nil {
			return err
		}
		s.started = true
		if native {
			return writeInvokeSSE(c, envelope.Type, raw)
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
		if err := decodeInvokeJSON(raw, &event); err != nil {
			return err
		}
		if event.Delta == nil {
			return errors.New("Claude message_delta has no delta")
		}
		if event.Delta.StopReason != nil && *event.Delta.StopReason != "" {
			s.finished = true
		}
		if native {
			return writeInvokeSSE(c, envelope.Type, raw)
		}
		if event.Delta.StopReason != nil && *event.Delta.StopReason != "" {
			s.terminal, _ = anthropic.StreamResponseClaude2OpenAI(c, &event)
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
		if err := decodeInvokeJSON(raw, &content); err != nil {
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
			block = &invokeContentBlock{kind: content.Block.Type, toolIndex: s.tools}
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
			return writeInvokeSSE(c, envelope.Type, raw)
		}
		if envelope.Type == "content_block_stop" {
			if block.kind == "tool_use" && !block.arguments {
				index := block.toolIndex
				chunk := &openai.ChatCompletionsStreamResponse{Choices: []openai.ChatCompletionsStreamResponseChoice{{}}}
				chunk.Choices[0].Delta.ToolCalls = []relaymodel.Tool{{Index: &index, Function: &relaymodel.Function{Arguments: "{}"}}}
				return s.writeChat(c, chunk)
			}
			return nil
		}
		if err := decodeInvokeJSON(raw, &event); err != nil {
			return err
		}
		chunk, _ := anthropic.StreamResponseClaude2OpenAI(c, &event)
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
			return writeInvokeSSE(c, envelope.Type, raw)
		}
		return nil
	}
}

// writeChat sends a converted chunk through the existing Responses API bridge.
// It supplies stable metadata and reports downstream errors to the caller.
func (s *invokeStreamState) writeChat(c *gin.Context, chunk *openai.ChatCompletionsStreamResponse) error {
	chunk.Id, chunk.Model, chunk.Created = s.id, c.GetString(ctxkey.RequestModel), s.created
	chunk.Object = "chat.completion.chunk"
	if err := openai_compatible.RenderStreamChunkWithBridge(c, chunk); err != nil {
		return errors.Wrap(err, "write converted Claude stream")
	}
	if writer, ok := c.Writer.(*invokeResponseWriter); ok && writer.err != nil {
		return writer.err
	}
	return nil
}

// writeInvokeSSE emits one native Claude event without losing unknown fields.
// Compact removes JSON whitespace that would otherwise break SSE framing.
func writeInvokeSSE(c *gin.Context, kind string, raw []byte) error {
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
