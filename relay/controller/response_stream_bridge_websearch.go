package controller

import (
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
)

// streamWebSearchState tracks one provider-executed web search rendered as a
// Responses web_search_call item.
type streamWebSearchState struct {
	id     string
	index  int
	query  string
	status string
	done   bool
}

// handleWebSearchCalls renders carried web searches: the first sighting emits
// output_item.added plus the in_progress and searching events, and a terminal
// status emits completed (on success) and output_item.done. Parameters: c is
// the request context and calls the searches carried by one Chat delta.
// Returns: nothing; write failures are retained in h.writeErr.
func (h *chatToResponseStreamBridge) handleWebSearchCalls(c *gin.Context, calls []model.WebSearchCall) {
	for _, call := range calls {
		if h.writeErr != nil {
			return
		}
		state := h.webSearchState(c, call)
		if call.Query != "" {
			state.query = call.Query
		}
		if call.Status == model.WebSearchCallCompleted || call.Status == model.WebSearchCallFailed {
			h.finishWebSearch(c, state, call.Status)
		}
	}
}

// webSearchState returns the state for call, creating it and emitting its
// opening events on first sight. Parameters: c is the request context and call
// the carried search. Returns: the state.
func (h *chatToResponseStreamBridge) webSearchState(c *gin.Context, call model.WebSearchCall) *streamWebSearchState {
	if call.ID != "" {
		if state, ok := h.webSearches[call.ID]; ok {
			return state
		}
	}
	state := &streamWebSearchState{
		id:     responseWebSearchCallID(call.ID),
		index:  h.nextOutputIndex(),
		query:  call.Query,
		status: model.WebSearchCallInProgress,
	}
	if h.webSearches == nil {
		h.webSearches = make(map[string]*streamWebSearchState)
	}
	if call.ID != "" {
		h.webSearches[call.ID] = state
	}
	h.webSearchOrder = append(h.webSearchOrder, state)

	item := responseWebSearchCallItem(state.id, model.WebSearchCallInProgress, state.query)
	if !h.emitEvent(c, "response.output_item.added", openai.ResponseAPIStreamEvent{
		OutputIndex: state.index,
		Item:        &item,
	}) {
		return state
	}
	for _, eventType := range []string{"response.web_search_call.in_progress", "response.web_search_call.searching"} {
		if !h.emitEvent(c, eventType, openai.ResponseAPIStreamEvent{ItemId: state.id, OutputIndex: state.index}) {
			return state
		}
	}
	return state
}

// finishWebSearch emits a search's terminal events exactly once. Parameters: c
// is the request context, state the search and status its terminal status.
// Returns: nothing.
func (h *chatToResponseStreamBridge) finishWebSearch(c *gin.Context, state *streamWebSearchState, status string) {
	if state.done {
		return
	}
	state.done = true
	state.status = finalWebSearchCallStatus(status)
	if state.status == model.WebSearchCallCompleted {
		if !h.emitEvent(c, "response.web_search_call.completed", openai.ResponseAPIStreamEvent{ItemId: state.id, OutputIndex: state.index}) {
			return
		}
	}
	item := h.webSearchItem(state)
	h.emitEvent(c, "response.output_item.done", openai.ResponseAPIStreamEvent{
		OutputIndex: state.index,
		Item:        &item,
	})
}

// finishPendingWebSearches closes searches that never produced a result before
// the upstream message ended; they are reported as failed. Parameters: c is
// the request context. Returns: nothing.
func (h *chatToResponseStreamBridge) finishPendingWebSearches(c *gin.Context) {
	for _, state := range h.webSearchOrder {
		h.finishWebSearch(c, state, model.WebSearchCallFailed)
	}
}

// webSearchItem renders the final web_search_call item for state.
func (h *chatToResponseStreamBridge) webSearchItem(state *streamWebSearchState) openai.OutputItem {
	return responseWebSearchCallItem(state.id, state.status, state.query)
}
