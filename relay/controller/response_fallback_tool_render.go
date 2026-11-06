package controller

import (
	"encoding/json"

	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	gmw "github.com/Laisky/gin-middlewares/v7"

	"github.com/Laisky/one-api/relay/adaptor/openai"
)

// responseFallbackCatalogForRender rebuilds the fallback tool catalog used to
// render upstream tool calls back into Responses items. Admission already
// validated the same request, so a failure here is unexpected; it is logged and
// rendering degrades to plain function_call items rather than failing a
// response whose upstream work already completed.
// Parameters: c is the request context, request the hydrated Responses request.
// Returns the catalog, or nil when it cannot be built.
func responseFallbackCatalogForRender(c *gin.Context, request *openai.ResponseAPIRequest) *openai.ResponseFallbackToolCatalog {
	if request == nil {
		return nil
	}
	catalog, err := openai.BuildResponseFallbackToolCatalog(request)
	if err != nil {
		lg := gmw.GetLogger(c)
		lg.Warn("rebuild response fallback tool catalog for rendering", zap.Error(err))
		return nil
	}
	return catalog
}

// toolKind reports how the bridge must render the tool call held in state.
func (h *chatToResponseStreamBridge) toolKind(state *streamToolCallState) openai.ResponseFallbackToolKind {
	if binding, ok := h.toolCatalog.Binding(state.name); ok {
		return binding.Kind
	}
	return openai.ResponseFallbackToolFunction
}

// toolItem renders the Responses output item for state with the given status.
// In-progress items carry no arguments or input yet, matching the added event.
func (h *chatToResponseStreamBridge) toolItem(state *streamToolCallState, status string, final bool) openai.OutputItem {
	arguments := ""
	if final {
		arguments = state.arguments.String()
	}
	item := h.toolCatalog.ResponseFallbackToolCallItem(state.id, state.name, arguments)
	item.Status = status
	if !final {
		item.Input = ""
		item.ToolSearchArguments = nil
		if item.Type == "tool_search_call" {
			// arguments is required on tool_search_call; an in-progress item
			// carries an empty object so strict clients can parse the event.
			item.ToolSearchArguments = json.RawMessage("{}")
		}
	}
	return item
}

// emitToolArgumentsDone emits the kind-specific completion events for one
// tool call before its output_item.done: function_call_arguments.done for
// functions, and the buffered custom_tool_call_input delta plus done for
// custom tools. Client tool_search calls have no argument events.
// It returns false when the client stream is gone.
func (h *chatToResponseStreamBridge) emitToolArgumentsDone(c *gin.Context, state *streamToolCallState, item openai.OutputItem) bool {
	switch h.toolKind(state) {
	case openai.ResponseFallbackToolCustom:
		// Custom input is wrapped in JSON upstream, so it is released in one
		// delta once the envelope is complete; Codex uses it for progress.
		if !h.emitEvent(c, "response.custom_tool_call_input.delta", openai.ResponseAPIStreamEvent{
			Type:        "response.custom_tool_call_input.delta",
			ItemId:      state.id,
			OutputIndex: state.index,
			Delta:       rawMessageFromString(item.Input),
		}) {
			return false
		}
		return h.emitEvent(c, "response.custom_tool_call_input.done", openai.ResponseAPIStreamEvent{
			Type:        "response.custom_tool_call_input.done",
			ItemId:      state.id,
			OutputIndex: state.index,
			Input:       item.Input,
		})
	case openai.ResponseFallbackToolSearch:
		return true
	default:
		return h.emitEvent(c, "response.function_call_arguments.done", openai.ResponseAPIStreamEvent{
			Type:        "response.function_call_arguments.done",
			ItemId:      state.id,
			OutputIndex: state.index,
			Arguments:   item.Arguments,
		})
	}
}
