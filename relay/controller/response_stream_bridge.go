package controller

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v6"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/random"
	"github.com/songquanpeng/one-api/common/render"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/adaptor/openai_compatible"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
)

type chatToResponseStreamBridge struct {
	meta     *metalib.Meta
	original *openai.ResponseAPIRequest

	responseID string
	createdAt  int64
	model      string

	messageItemID      string
	messageOutputIndex int
	outputIndexCounter int

	textBuilder      strings.Builder
	reasoningBuilder strings.Builder

	reasoningInitialized bool
	usage                *openai.ResponseAPIUsage

	toolCalls map[string]*streamToolCallState
	toolOrder []string

	lastFinishReason string
	incomplete       *openai.IncompleteDetails

	streamDone    bool
	upstreamDone  bool
	streamStarted bool
}

type streamToolCallState struct {
	id        string
	name      string
	index     int
	arguments strings.Builder
}

func newChatToResponseStreamBridge(c *gin.Context, meta *metalib.Meta, request *openai.ResponseAPIRequest) openai_compatible.StreamRewriteHandler {
	handler := &chatToResponseStreamBridge{
		meta:       meta,
		original:   request,
		responseID: generateResponseAPIID(c, nil),
		createdAt:  time.Now().Unix(),
		model:      meta.ActualModelName,
		toolCalls:  make(map[string]*streamToolCallState),
	}

	if handler.model == "" {
		handler.model = request.Model
	}

	handler.messageItemID = fmt.Sprintf("msg_%s", random.GetRandomString(16))
	handler.messageOutputIndex = handler.nextOutputIndex()

	return handler
}

func (h *chatToResponseStreamBridge) HandleChunk(c *gin.Context, chunk *openai_compatible.ChatCompletionsStreamResponse) (bool, bool) {
	if h.streamDone {
		return true, true
	}

	if !h.streamStarted {
		h.ensureInitialized(c, chunk)
	}

	if chunk.Model != "" {
		h.model = chunk.Model
	}
	if chunk.Created != 0 {
		h.createdAt = chunk.Created
	}

	for _, choice := range chunk.Choices {
		if delta := choice.Delta.StringContent(); delta != "" {
			h.appendTextDelta(c, delta)
		}

		if choice.Delta.Reasoning != nil && *choice.Delta.Reasoning != "" {
			h.appendReasoningDelta(c, *choice.Delta.Reasoning)
		}
		if choice.Delta.ReasoningContent != nil && *choice.Delta.ReasoningContent != "" {
			h.appendReasoningDelta(c, *choice.Delta.ReasoningContent)
		}
		if choice.Delta.Thinking != nil && *choice.Delta.Thinking != "" {
			h.appendReasoningDelta(c, *choice.Delta.Thinking)
		}

		if len(choice.Delta.ToolCalls) > 0 {
			h.handleToolCalls(c, choice.Delta.ToolCalls)
		}

		if choice.FinishReason != nil {
			h.lastFinishReason = *choice.FinishReason
		}
	}

	if chunk.Usage != nil {
		h.usage = (&openai.ResponseAPIUsage{}).FromModelUsage(chunk.Usage)
	}

	return true, false
}

func (h *chatToResponseStreamBridge) HandleUpstreamDone(_ *gin.Context) (bool, bool) {
	h.upstreamDone = true
	return true, false
}

func (h *chatToResponseStreamBridge) HandleDone(c *gin.Context) (bool, bool) {
	if h.streamDone {
		return true, true
	}

	h.streamDone = true

	if !h.streamStarted {
		// No chunks were processed; initialize so we can emit terminal events.
		h.ensureInitialized(c, &openai_compatible.ChatCompletionsStreamResponse{})
	}

	text := h.textBuilder.String()

	h.emitEvent(c, "response.output_text.done", openai.ResponseAPIStreamEvent{
		Type:         "response.output_text.done",
		ItemId:       h.messageItemID,
		OutputIndex:  h.messageOutputIndex,
		ContentIndex: 0,
		Text:         text,
	})

	h.emitEvent(c, "response.content_part.done", openai.ResponseAPIStreamEvent{
		Type:         "response.content_part.done",
		ItemId:       h.messageItemID,
		OutputIndex:  h.messageOutputIndex,
		ContentIndex: 0,
		Part: &openai.OutputContent{
			Type: "output_text",
			Text: text,
		},
	})

	messageItem := openai.OutputItem{
		Id:      h.messageItemID,
		Type:    "message",
		Status:  "completed",
		Role:    "assistant",
		Content: []openai.OutputContent{{Type: "output_text", Text: text}},
	}

	if reasoning := strings.TrimSpace(h.reasoningBuilder.String()); reasoning != "" {
		h.emitEvent(c, "response.reasoning_summary_text.done", openai.ResponseAPIStreamEvent{
			Type: "response.reasoning_summary_text.done",
			Part: &openai.OutputContent{Type: "summary_text", Text: reasoning},
		})
	}

	h.emitEvent(c, "response.output_item.done", openai.ResponseAPIStreamEvent{
		Type:        "response.output_item.done",
		OutputIndex: h.messageOutputIndex,
		Item:        &messageItem,
	})

	finalOutputs := make([]openai.OutputItem, 0, 1+len(h.toolOrder)+1)
	finalOutputs = append(finalOutputs, messageItem)

	if reasoning := strings.TrimSpace(h.reasoningBuilder.String()); reasoning != "" {
		finalOutputs = append(finalOutputs, openai.OutputItem{
			Type:   "reasoning",
			Status: "completed",
			Summary: []openai.OutputContent{
				{Type: "summary_text", Text: reasoning},
			},
		})
	}

	for _, id := range h.toolOrder {
		state := h.toolCalls[id]
		args := state.arguments.String()

		h.emitEvent(c, "response.function_call_arguments.done", openai.ResponseAPIStreamEvent{
			Type:        "response.function_call_arguments.done",
			ItemId:      state.id,
			OutputIndex: state.index,
			Arguments:   args,
		})

		toolItem := openai.OutputItem{
			Id:        state.id,
			Type:      "function_call",
			Status:    "completed",
			CallId:    state.id,
			Name:      state.name,
			Arguments: args,
		}

		h.emitEvent(c, "response.output_item.done", openai.ResponseAPIStreamEvent{
			Type:        "response.output_item.done",
			OutputIndex: state.index,
			Item:        &toolItem,
		})

		finalOutputs = append(finalOutputs, toolItem)
	}

	status, incomplete := h.finalStatus()
	if incomplete != nil {
		h.incomplete = incomplete
	}

	response := h.buildFinalResponse(status, finalOutputs)
	if h.incomplete != nil {
		response.IncompleteDetails = h.incomplete
	}

	h.emitEvent(c, "response.completed", openai.ResponseAPIStreamEvent{
		Type:     "response.completed",
		Response: response,
	})

	render.Done(c)

	return true, true
}

func (h *chatToResponseStreamBridge) FinalizeUsage(usage *model.Usage) {
	if usage == nil {
		return
	}
	h.usage = (&openai.ResponseAPIUsage{}).FromModelUsage(usage)
}

func (h *chatToResponseStreamBridge) ensureInitialized(c *gin.Context, chunk *openai_compatible.ChatCompletionsStreamResponse) {
	h.streamStarted = true

	if chunk != nil {
		if chunk.Model != "" {
			h.model = chunk.Model
		}
		if chunk.Created != 0 {
			h.createdAt = chunk.Created
		}
	}

	response := &openai.ResponseAPIResponse{
		Id:                 h.responseID,
		Object:             "response",
		CreatedAt:          h.createdAt,
		Status:             "in_progress",
		Model:              h.model,
		Instructions:       h.original.Instructions,
		MaxOutputTokens:    h.original.MaxOutputTokens,
		Metadata:           h.original.Metadata,
		ParallelToolCalls:  h.original.ParallelToolCalls != nil && *h.original.ParallelToolCalls,
		PreviousResponseId: h.original.PreviousResponseId,
		Reasoning:          h.original.Reasoning,
		ServiceTier:        h.original.ServiceTier,
		Temperature:        h.original.Temperature,
		Text:               h.original.Text,
		ToolChoice:         h.original.ToolChoice,
		Tools:              convertResponseAPITools(h.original.Tools),
		TopP:               h.original.TopP,
		Truncation:         h.original.Truncation,
		User:               h.original.User,
	}

	if response.Model == "" {
		response.Model = h.original.Model
	}

	h.emitEvent(c, "response.created", openai.ResponseAPIStreamEvent{
		Type:     "response.created",
		Response: response,
	})

	messageItem := openai.OutputItem{
		Id:     h.messageItemID,
		Type:   "message",
		Status: "in_progress",
		Role:   "assistant",
	}

	h.emitEvent(c, "response.output_item.added", openai.ResponseAPIStreamEvent{
		Type:        "response.output_item.added",
		OutputIndex: h.messageOutputIndex,
		Item:        &messageItem,
	})

	h.emitEvent(c, "response.content_part.added", openai.ResponseAPIStreamEvent{
		Type:         "response.content_part.added",
		ItemId:       h.messageItemID,
		OutputIndex:  h.messageOutputIndex,
		ContentIndex: 0,
		Part: &openai.OutputContent{
			Type: "output_text",
			Text: "",
		},
	})
}

func (h *chatToResponseStreamBridge) appendTextDelta(c *gin.Context, delta string) {
	if delta == "" {
		return
	}

	h.textBuilder.WriteString(delta)

	h.emitEvent(c, "response.output_text.delta", openai.ResponseAPIStreamEvent{
		Type:         "response.output_text.delta",
		ItemId:       h.messageItemID,
		OutputIndex:  h.messageOutputIndex,
		ContentIndex: 0,
		Delta:        delta,
	})
}

func (h *chatToResponseStreamBridge) appendReasoningDelta(c *gin.Context, delta string) {
	trimmed := strings.TrimSpace(delta)
	if trimmed == "" {
		return
	}

	if !h.reasoningInitialized {
		h.emitEvent(c, "response.reasoning_summary_part.added", openai.ResponseAPIStreamEvent{
			Type: "response.reasoning_summary_part.added",
			Part: &openai.OutputContent{Type: "summary_text", Text: ""},
		})
		h.reasoningInitialized = true
	}

	h.reasoningBuilder.WriteString(trimmed)
	h.emitEvent(c, "response.reasoning_summary_text.delta", openai.ResponseAPIStreamEvent{
		Type:  "response.reasoning_summary_text.delta",
		Delta: trimmed,
	})
}

func (h *chatToResponseStreamBridge) handleToolCalls(c *gin.Context, tools []model.Tool) {
	for _, tool := range tools {
		state := h.ensureToolCallState(c, &tool)

		if tool.Function == nil || tool.Function.Arguments == nil {
			continue
		}

		args := h.stringifyArguments(tool.Function.Arguments)
		if args == "" {
			continue
		}

		state.arguments.WriteString(args)
		h.emitEvent(c, "response.function_call_arguments.delta", openai.ResponseAPIStreamEvent{
			Type:        "response.function_call_arguments.delta",
			ItemId:      state.id,
			OutputIndex: state.index,
			Delta:       args,
		})
	}
}

func (h *chatToResponseStreamBridge) ensureToolCallState(c *gin.Context, tool *model.Tool) *streamToolCallState {
	id := tool.Id
	if id == "" {
		id = fmt.Sprintf("call_%s", random.GetRandomString(16))
	}

	state, exists := h.toolCalls[id]
	if !exists {
		state = &streamToolCallState{
			id:    id,
			index: h.nextOutputIndex(),
		}
		h.toolCalls[id] = state
		h.toolOrder = append(h.toolOrder, id)

		if tool.Function != nil {
			state.name = tool.Function.Name
		}

		item := openai.OutputItem{
			Id:     state.id,
			Type:   "function_call",
			Status: "in_progress",
			Name:   state.name,
		}

		h.emitEvent(c, "response.output_item.added", openai.ResponseAPIStreamEvent{
			Type:        "response.output_item.added",
			OutputIndex: state.index,
			Item:        &item,
		})
	} else if tool.Function != nil && tool.Function.Name != "" {
		state.name = tool.Function.Name
	}

	return state
}

func (h *chatToResponseStreamBridge) stringifyArguments(args any) string {
	switch v := args.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case nil:
		return ""
	default:
		if data, err := json.Marshal(v); err == nil {
			return string(data)
		}
		return ""
	}
}

func (h *chatToResponseStreamBridge) buildFinalResponse(status string, outputs []openai.OutputItem) *openai.ResponseAPIResponse {
	response := &openai.ResponseAPIResponse{
		Id:                 h.responseID,
		Object:             "response",
		CreatedAt:          h.createdAt,
		Status:             status,
		Model:              h.model,
		Output:             outputs,
		Usage:              h.usage,
		Instructions:       h.original.Instructions,
		MaxOutputTokens:    h.original.MaxOutputTokens,
		Metadata:           h.original.Metadata,
		ParallelToolCalls:  h.original.ParallelToolCalls != nil && *h.original.ParallelToolCalls,
		PreviousResponseId: h.original.PreviousResponseId,
		Reasoning:          h.original.Reasoning,
		ServiceTier:        h.original.ServiceTier,
		Temperature:        h.original.Temperature,
		Text:               h.original.Text,
		ToolChoice:         h.original.ToolChoice,
		Tools:              convertResponseAPITools(h.original.Tools),
		TopP:               h.original.TopP,
		Truncation:         h.original.Truncation,
		User:               h.original.User,
	}

	if response.Model == "" {
		response.Model = h.meta.ActualModelName
	}
	if response.Model == "" {
		response.Model = h.original.Model
	}

	return response
}

func (h *chatToResponseStreamBridge) finalStatus() (string, *openai.IncompleteDetails) {
	switch strings.ToLower(strings.TrimSpace(h.lastFinishReason)) {
	case "length":
		return "incomplete", &openai.IncompleteDetails{Reason: "max_output_tokens"}
	case "cancelled":
		return "cancelled", nil
	default:
		return "completed", nil
	}
}

func (h *chatToResponseStreamBridge) emitEvent(c *gin.Context, eventType string, event openai.ResponseAPIStreamEvent) {
	event.Type = eventType
	if event.Id == "" {
		event.Id = h.responseID
	}

	payload, err := json.Marshal(event)
	if err != nil {
		gmw.GetLogger(c).Warn("failed to marshal response stream event", zap.String("event_type", eventType), zap.Error(err))
		return
	}

	var builder strings.Builder
	if eventType != "" {
		builder.WriteString("event: ")
		builder.WriteString(eventType)
		builder.WriteByte('\n')
	}
	builder.WriteString("data: ")
	builder.Write(payload)
	builder.WriteString("\n\n")

	if _, err := c.Writer.Write([]byte(builder.String())); err != nil {
		gmw.GetLogger(c).Warn("failed to write response stream event", zap.String("event_type", eventType), zap.Error(err))
	}
	c.Writer.Flush()
}

func (h *chatToResponseStreamBridge) nextOutputIndex() int {
	idx := h.outputIndexCounter
	h.outputIndexCounter++
	return idx
}
