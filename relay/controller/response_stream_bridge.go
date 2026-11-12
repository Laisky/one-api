package controller

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/random"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/state"
)

type chatToResponseStreamBridge struct {
	meta     *metalib.Meta
	original *openai.ResponseAPIRequest

	responseID string
	createdAt  int64
	model      string

	messageItemID      string
	messageOutputIndex int
	// messageOpened reports whether the primary message item was announced.
	// It opens lazily with its first text, so it follows the items (reasoning,
	// thinking, searches) that preceded the text and clients that track one
	// active item (Codex) show every text delta; a tool-only turn has none.
	messageOpened bool
	// sealedMessages are primary message items completed before the stream
	// ended because another item followed them, in output order.
	sealedMessages     []sealedReasoningItem
	outputIndexCounter int
	// sequenceNumber is the sequence_number of the next emitted event.
	sequenceNumber int

	textBuilder      strings.Builder
	reasoningBuilder strings.Builder

	reasoningInitialized bool
	usage                *openai.ResponseAPIUsage
	primaryChoiceBound   bool
	primaryChoiceIndex   int
	activeChoiceIndex    int
	choices              map[int]*responseChoiceState
	choiceOrder          []*responseChoiceState
	writeErr             error
	deliveredImages      int

	toolCalls       map[string]*streamToolCallState
	toolIndex       map[string]*streamToolCallState
	toolOrder       []string
	choiceToolOrder map[int][]string

	// webSearches renders provider-executed web searches as web_search_call
	// items, keyed by upstream invocation ID, in arrival order.
	webSearches    map[string]*streamWebSearchState
	webSearchOrder []*streamWebSearchState

	lastFinishReason string
	incomplete       *openai.IncompleteDetails

	streamDone                bool
	upstreamDone              bool
	streamStarted             bool
	requiredActionInitialized bool

	// toolsEcho is the request's tools in the Response object's spec shape,
	// encoded once for response.created and the terminal response.
	toolsEcho json.RawMessage

	// toolCatalog restores namespaced, custom and tool_search identities that
	// fallback flattened into Chat function tools. Nil renders plain calls.
	toolCatalog *openai.ResponseFallbackToolCatalog

	// includeEncrypted reports whether the client asked for
	// reasoning.encrypted_content; gateway Claude envelopes are always persisted
	// with committed state but reach the client only when it opted in.
	includeEncrypted bool

	// pendingCommit, when set, causes the terminal event to commit a gateway
	// response node. The gateway response ID is pre-minted at construction so it is
	// the ID carried by every emitted event (STR01).
	pendingCommit *pendingStateCommit
}

type streamToolCallState struct {
	extraContent *model.ToolExtraContent
	id           string
	name         string
	index        int
	arguments    strings.Builder
	streamIndex  *int
}

type responseChoiceState struct {
	contentParts         []openai.OutputContent
	contentDone          int
	index                int
	messageID            string
	messageIndex         int
	text                 strings.Builder
	reasoning            strings.Builder
	reasoningID          string
	reasoningIndex       int
	reasoningInitialized bool
	finishReason         string
	// sealedReasoning holds reasoning items already completed by verbatim Claude
	// thinking blocks, with their envelopes, in output order.
	sealedReasoning []sealedReasoningItem
}

// sealedReasoningItem is a completed reasoning item and its output index.
type sealedReasoningItem struct {
	index int
	item  openai.OutputItem
}

// rawMessageFromString encodes value as a JSON string literal for a delta
// field. It returns nil for an empty value or when encoding fails.
func rawMessageFromString(value string) json.RawMessage {
	if value == "" {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return json.RawMessage(encoded)
}

// newChatToResponseStreamBridge builds the handler that rewrites an upstream
// Chat Completions stream into Responses API SSE events for a fallback request.
// Parameters: c is the request context, meta the mapped request metadata and
// request the hydrated Responses request (its tools drive call rendering).
// Returns: the stream rewrite handler; when gateway state is active it carries
// a pre-minted gateway response ID and the pending state commit.
func newChatToResponseStreamBridge(c *gin.Context, meta *metalib.Meta, request *openai.ResponseAPIRequest) openai_compatible.StreamRewriteHandler {
	handler := &chatToResponseStreamBridge{
		meta:       meta,
		original:   request,
		responseID: generateResponseAPIID(c, nil),
		createdAt:  time.Now().Unix(),
		model:      meta.ActualModelName,
	}

	if handler.model == "" {
		handler.model = request.Model
	}
	handler.toolCatalog = responseFallbackCatalogForRender(c, request)
	handler.toolsEcho = responseToolsEcho(gmw.GetLogger(c), request.Tools)
	handler.includeEncrypted = responseIncludesReasoningEncryptedContent(request)

	// If gateway state is active for this request, pre-mint a gateway response ID
	// and remember the pre-hydration commit snapshot so the terminal event can
	// commit a backing record and every emitted event carries the gateway ID
	// (closes B13). No-op when the feature is inactive or store=false.
	if commit := pendingCommitFromContext(c); commit != nil && commit.storeMode && state.Enabled() && state.Store() != nil {
		if gwID, err := state.NewResponseID(); err == nil {
			handler.responseID = gwID
			handler.pendingCommit = commit
		}
	}

	handler.messageItemID = "msg_" + random.GetRandomString(16)
	handler.messageOutputIndex = -1

	return handler
}

// commitStreamedResponse commits the accumulated streamed output as a gateway
// response node (using the pre-minted gateway ID) and stamps store/conversation
// onto the final response object. It is a no-op unless a commit is pending.
func (h *chatToResponseStreamBridge) commitStreamedResponse(c *gin.Context, response *openai.ResponseAPIResponse, status string) {
	if h.pendingCommit == nil || response == nil {
		return
	}
	if commitResponseNodeWithID(c, h.pendingCommit, h.responseID, response.Output, h.usage, status) {
		storeMode := true
		response.Store = &storeMode
		if h.pendingCommit.conversationID != "" {
			response.Conversation = &openai.ResponseAPIConversation{Id: h.pendingCommit.conversationID}
		}
	}
}

// HandleUpstreamDone records that the upstream sent its [DONE] marker; the
// terminal events are emitted later by HandleDone. It returns handled=true and
// stop=false.
func (h *chatToResponseStreamBridge) HandleUpstreamDone(_ *gin.Context) (bool, bool) {
	h.upstreamDone = true
	return true, false
}

// HandleDone completes the bridged stream once the upstream signalled [DONE]:
// it flushes every tool call's completion events and output_item.done, closes
// the message and reasoning items, commits gateway state when pending, and
// emits the terminal response event. Parameters: c is the request context.
// Returns: whether the done marker was handled and whether streaming must stop.
func (h *chatToResponseStreamBridge) HandleDone(c *gin.Context) (bool, bool) {
	if h.streamDone {
		return true, true
	}

	h.streamDone = true

	if !h.streamStarted {
		// No chunks were processed; initialize so we can emit terminal events.
		h.ensureInitialized(c, &openai_compatible.ChatCompletionsStreamResponse{})
	}

	if !h.messageOpened && h.outputIndexCounter == 0 {
		// A response with no output at all still carries one (empty)
		// assistant message, as it always has.
		h.openPrimaryMessage(c)
	}
	messageItem := h.primaryMessage("completed")
	if h.messageOpened {
		h.emitChoiceContentDone(c, h.choices[h.primaryChoiceIndex])
	}

	// Emit tool-call terminals BEFORE the message item's output_item.done so that
	// downstream agent loops never observe an output_item.done event ahead of the
	// matching function_call_arguments.done for the same call_id. The per-call
	// invariant we guarantee here is:
	//
	//   response.output_item.added (function_call, arguments empty)
	//     -> response.function_call_arguments.delta * N
	//     -> response.function_call_arguments.done   (final arguments)
	//     -> response.output_item.done               (final arguments, identical)
	//
	// Some consumers (notably go-ramjet's agentx oneAPI adapter) accumulate
	// args from deltas and fire tool dispatch on output_item.done; emitting any
	// output_item.done — even for an unrelated message item — before the
	// args.done events for queued tool calls used to ship empty/partial args
	// downstream. Tool-call events are flushed first to preserve the spec
	// ordering for every function_call regardless of whether the response also
	// carried a text message.
	h.finishPendingWebSearches(c)
	toolItems := make([]openai.OutputItem, 0, len(h.toolOrder))
	for _, id := range h.toolOrder {
		state := h.toolCalls[id]
		toolItem := h.toolItem(state, "completed", true)
		h.emitToolArgumentsDone(c, state, toolItem)

		// output_item.done MUST come after function_call_arguments.done for the
		// same call_id and MUST carry the full buffered arguments string (not a
		// snapshot of empty state). See bug report on go-ramjet agent loop
		// observing args="{" because output_item.done arrived first.
		h.emitEvent(c, "response.output_item.done", openai.ResponseAPIStreamEvent{
			Type:        "response.output_item.done",
			OutputIndex: state.index,
			Item:        &toolItem,
		})

		toolItems = append(toolItems, toolItem)
	}

	if h.messageOpened {
		h.emitEvent(c, "response.output_item.done", openai.ResponseAPIStreamEvent{
			Type:        "response.output_item.done",
			OutputIndex: h.messageOutputIndex,
			Item:        &messageItem,
		})
	}
	for _, choice := range h.choiceOrder {
		h.finishChoice(c, choice)
	}

	type indexedItem struct {
		index int
		item  openai.OutputItem
	}
	var indexed []indexedItem
	for _, sealed := range h.sealedMessages {
		indexed = append(indexed, indexedItem{sealed.index, sealed.item})
	}
	if h.messageOpened {
		indexed = append(indexed, indexedItem{h.messageOutputIndex, messageItem})
	}
	for _, choice := range h.choiceOrder {
		if choice.index != h.primaryChoiceIndex {
			indexed = append(indexed, indexedItem{choice.messageIndex, h.choiceMessage(choice, "completed")})
		}
		if reasoning := h.choiceReasoning(choice, "completed"); reasoning != nil {
			indexed = append(indexed, indexedItem{choice.reasoningIndex, *reasoning})
		}
		for _, sealed := range choice.sealedReasoning {
			indexed = append(indexed, indexedItem{sealed.index, sealed.item})
		}
	}
	for _, search := range h.webSearchOrder {
		indexed = append(indexed, indexedItem{search.index, h.webSearchItem(search)})
	}
	for i, key := range h.toolOrder {
		indexed = append(indexed, indexedItem{h.toolCalls[key].index, toolItems[i]})
	}
	sort.Slice(indexed, func(i, j int) bool { return indexed[i].index < indexed[j].index })
	finalOutputs := make([]openai.OutputItem, 0, len(indexed))
	for _, item := range indexed {
		finalOutputs = append(finalOutputs, item.item)
	}

	requiredAction := h.buildRequiredActionSnapshot()
	if requiredAction != nil {
		h.emitRequiredActionDelta(c, true)
	}

	status, incomplete := h.finalStatus()
	if incomplete != nil {
		h.incomplete = incomplete
	}

	response := h.buildFinalResponse(status, finalOutputs, requiredAction)
	if h.incomplete != nil {
		response.IncompleteDetails = h.incomplete
	}
	if h.writeErr != nil {
		return true, true
	}

	// Commit the gateway response node exactly once at the terminal event so the
	// streamed fallback ID is backed by a retrievable record (closes B13).
	h.commitStreamedResponse(c, response, status)
	response.Output = clientVisibleResponseOutput(response.Output, h.includeEncrypted)

	// The spec ends an incomplete response (e.g. max_output_tokens) with
	// response.incomplete; clients such as Codex and the OpenAI SDKs act on the
	// event type, not only on response.status.
	terminalEvent := "response.completed"
	if status == "incomplete" {
		terminalEvent = "response.incomplete"
	}
	h.emitEvent(c, terminalEvent, openai.ResponseAPIStreamEvent{
		Type:     terminalEvent,
		Response: response,
	})
	if h.writeErr != nil {
		return true, true
	}

	h.emitDone(c)

	return true, true
}

// FinalizeUsage stores the settled upstream usage for the terminal response
// event. A nil usage leaves the previous value unchanged.
func (h *chatToResponseStreamBridge) FinalizeUsage(usage *model.Usage) {
	if usage == nil {
		return
	}
	h.usage = (&openai.ResponseAPIUsage{}).FromModelUsage(usage)
}

// ensureInitialized starts the Responses stream on the first chunk: it emits
// response.created plus the primary message item and its output_text part.
// Parameters: c is the request context and chunk supplies the upstream model
// and creation time when present.
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
		Model:              userVisibleModelName(h.meta, ""),
		Output:             make([]openai.OutputItem, 0),
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
		ToolsEcho:          h.toolsEcho,
		TopP:               h.original.TopP,
		Truncation:         h.original.Truncation,
		User:               h.original.User,
	}

	if response.Model == "" {
		response.Model = h.model
	}
	if response.Model == "" {
		response.Model = h.original.Model
	}

	h.emitEvent(c, "response.created", openai.ResponseAPIStreamEvent{
		Type:     "response.created",
		Response: response,
	})
}

// openPrimaryMessage announces the primary message item and its output_text
// part the first time text arrives (or at completion when the response has no
// output at all), allocating its output_index then so the item follows every
// item that preceded the text. It is a no-op once the message is open.
func (h *chatToResponseStreamBridge) openPrimaryMessage(c *gin.Context) {
	if h.messageOpened {
		return
	}
	h.beforeItemAdded(c, nil, itemKindMessage)
	h.messageOpened = true
	h.messageOutputIndex = h.nextOutputIndex()
	if choice := h.choices[h.primaryChoiceIndex]; choice != nil && h.primaryChoiceBound {
		choice.messageID, choice.messageIndex = h.messageItemID, h.messageOutputIndex
	}
	messageItem := openai.OutputItem{
		Id:      h.messageItemID,
		Type:    "message",
		Status:  "in_progress",
		Role:    "assistant",
		Content: make([]openai.OutputContent, 0),
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
		Part:         &openai.OutputContent{Type: "output_text"},
	})
}

// appendTextDelta streams one text delta of the primary message and records
// it once the event was written. Empty deltas are ignored.
func (h *chatToResponseStreamBridge) appendTextDelta(c *gin.Context, delta string) {
	if delta == "" {
		return
	}
	h.openPrimaryMessage(c)

	if h.emitEvent(c, "response.output_text.delta", openai.ResponseAPIStreamEvent{
		Type:         "response.output_text.delta",
		ItemId:       h.messageItemID,
		OutputIndex:  h.messageOutputIndex,
		ContentIndex: 0,
		Delta:        rawMessageFromString(delta),
	}) {
		h.textBuilder.WriteString(delta)
	}
}

// appendReasoningDelta forwards one streamed reasoning delta verbatim so word
// boundaries that fall on chunk edges survive. Leading whitespace is dropped
// only before any reasoning has accumulated, so a whitespace-only first delta
// never opens an empty reasoning item.
func (h *chatToResponseStreamBridge) appendReasoningDelta(c *gin.Context, delta string) {
	choice := h.choices[h.activeChoiceIndex]
	if choice == nil {
		return
	}
	if choice.reasoning.Len() == 0 {
		delta = strings.TrimLeftFunc(delta, unicode.IsSpace)
	}
	if delta == "" {
		return
	}
	h.appendChoiceReasoning(c, choice, delta)
}

// handleToolCalls applies one chunk's tool-call deltas: it opens each call's
// output item on first sight and forwards function argument deltas, while
// custom-tool input and tool_search arguments are buffered until completion.
// Parameters: c is the request context, tools the chunk's tool-call deltas.
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

		if h.toolKind(state) != openai.ResponseFallbackToolFunction {
			// Custom input and tool_search arguments are JSON envelopes upstream;
			// buffer them and release the decoded value when the call completes.
			state.arguments.WriteString(args)
			h.emitRequiredActionDelta(c, false)
			continue
		}

		if !h.emitEvent(c, "response.function_call_arguments.delta", openai.ResponseAPIStreamEvent{
			Type:        "response.function_call_arguments.delta",
			ItemId:      state.id,
			OutputIndex: state.index,
			Delta:       rawMessageFromString(args),
		}) {
			return
		}
		state.arguments.WriteString(args)
		h.emitRequiredActionDelta(c, false)
	}
}

// ensureToolCallState returns the accumulated state for one upstream tool call,
// matching deltas by stream index or ID, and emits response.output_item.added
// with the kind-specific item when the call is first seen. Parameters: c is the
// request context, tool the tool-call delta. Returns: the call's stream state.
func (h *chatToResponseStreamBridge) ensureToolCallState(c *gin.Context, tool *model.Tool) *streamToolCallState {
	// Text-only streams never use tool state, so allocate these maps only when
	// the first tool delta arrives instead of on every bridged response.
	if h.toolCalls == nil {
		h.toolCalls = make(map[string]*streamToolCallState)
	}
	if tool.Index != nil && h.toolIndex == nil {
		h.toolIndex = make(map[string]*streamToolCallState)
	}
	if h.choiceToolOrder == nil {
		h.choiceToolOrder = make(map[int][]string)
	}
	choice := h.activeChoiceIndex
	key := func(id string) string { return strconv.Itoa(choice) + ":" + id }

	normalizedID := ""
	if trimmed := strings.TrimSpace(tool.Id); trimmed != "" {
		normalizedID = ensureResponseAPICallID(trimmed)
	}

	var state *streamToolCallState
	if normalizedID != "" {
		state = h.toolCalls[key(normalizedID)]
	}
	if state == nil && tool.Index != nil {
		idx := *tool.Index
		if existing, ok := h.toolIndex[key(strconv.Itoa(idx))]; ok {
			state = existing
		} else if idx >= 0 && idx < len(h.choiceToolOrder[choice]) {
			if id := h.choiceToolOrder[choice][idx]; id != "" {
				if candidate, ok := h.toolCalls[id]; ok {
					state = candidate
				}
			}
		}
	}

	created := false
	if state == nil {
		id := normalizedID
		if id == "" {
			id = "call_" + random.GetRandomString(16)
		}
		for _, existingKey := range h.toolOrder {
			if h.toolCalls[existingKey].id == id {
				id = "call_" + random.GetRandomString(16)
				break
			}
		}
		h.beforeItemAdded(c, h.choices[choice], itemKindOther)
		state = &streamToolCallState{
			id:    id,
			index: h.nextOutputIndex(),
		}
		if tool.Index != nil {
			idx := *tool.Index
			state.streamIndex = new(int)
			*state.streamIndex = idx
			h.toolIndex[key(strconv.Itoa(idx))] = state
		}
		storageKey := key(state.id)
		h.toolCalls[storageKey] = state
		h.toolOrder = append(h.toolOrder, storageKey)
		h.choiceToolOrder[choice] = append(h.choiceToolOrder[choice], storageKey)
		created = true
	}

	if normalizedID != "" && normalizedID != state.id {
		// Reaching here means the state already existed, so response.output_item.added
		// has already published state.id as this call's id and call_id. Clients that
		// build the tool call identity from the added event never revisit it, so an
		// upstream that reveals the real tool-call ID only in a later delta must not
		// rename the call underneath them — doing so made output_item.done contradict
		// output_item.added. Keep the published ID and register the upstream ID as an
		// alias so later deltas still resolve to this state. The ID only has to be
		// self-consistent within the conversation: it is converted back by
		// convertResponseAPIIDToToolCall on replay and never has to match what the
		// upstream originally issued.
		h.toolCalls[key(normalizedID)] = state
	}

	if tool.Index != nil {
		idx := *tool.Index
		if state.streamIndex == nil {
			state.streamIndex = new(int)
		}
		*state.streamIndex = idx
		h.toolIndex[key(strconv.Itoa(idx))] = state
	}

	if tool.Function != nil && tool.Function.Name != "" {
		state.name = tool.Function.Name
	}

	if tool.ExtraContent != nil {
		state.extraContent = tool.ExtraContent
	}

	if created {
		// Clients such as pi derive the tool call identity from call_id on the added
		// event and never revisit it, so the added item must already carry call_id.
		item := h.toolItem(state, "in_progress", false)

		h.emitEvent(c, "response.output_item.added", openai.ResponseAPIStreamEvent{
			Type:        "response.output_item.added",
			OutputIndex: state.index,
			Item:        &item,
		})
	}

	h.emitRequiredActionDelta(c, false)

	return state
}

// stringifyArguments returns upstream tool-call arguments as a JSON string:
// strings and byte slices verbatim, other values JSON-encoded, nil as "".
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

// buildFinalResponse assembles the terminal Response object. Parameters:
// status is the final status, outputs the items in output_index order and
// requiredAction the optional tool-call snapshot. Returns the response.
func (h *chatToResponseStreamBridge) buildFinalResponse(status string, outputs []openai.OutputItem, requiredAction *openai.ResponseAPIRequiredAction) *openai.ResponseAPIResponse {
	if outputs == nil {
		outputs = make([]openai.OutputItem, 0)
	}
	response := &openai.ResponseAPIResponse{
		Id:                 h.responseID,
		Object:             "response",
		CreatedAt:          h.createdAt,
		Status:             status,
		Model:              userVisibleModelName(h.meta, ""),
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
		ToolsEcho:          h.toolsEcho,
		TopP:               h.original.TopP,
		Truncation:         h.original.Truncation,
		User:               h.original.User,
		RequiredAction:     requiredAction,
	}

	if response.Model == "" {
		response.Model = h.model
	}
	if response.Model == "" {
		response.Model = h.meta.ActualModelName
	}
	if response.Model == "" {
		response.Model = h.original.Model
	}

	return response
}

// finalStatus derives the terminal status from the choices' finish reasons:
// cancelled, incomplete (max_output_tokens) for a length finish, or completed.
// It returns the status and the incomplete details when applicable.
func (h *chatToResponseStreamBridge) finalStatus() (string, *openai.IncompleteDetails) {
	for _, choice := range h.choiceOrder {
		if strings.EqualFold(strings.TrimSpace(choice.finishReason), "cancelled") {
			return "cancelled", nil
		}
	}
	for _, choice := range h.choiceOrder {
		if strings.EqualFold(strings.TrimSpace(choice.finishReason), "length") {
			return "incomplete", &openai.IncompleteDetails{Reason: "max_output_tokens"}
		}
	}
	switch strings.ToLower(strings.TrimSpace(h.lastFinishReason)) {
	case "length":
		return "incomplete", &openai.IncompleteDetails{Reason: "max_output_tokens"}
	case "cancelled":
		return "cancelled", nil
	default:
		return "completed", nil
	}
}

// HandleError preserves partial output as failed, never authorizing complete tool
// calls or publishing response.completed after quota or transport exhaustion.
func (h *chatToResponseStreamBridge) HandleError(c *gin.Context, failure *model.ErrorWithStatusCode) (bool, bool) {
	if h.streamDone {
		return true, true
	}
	h.streamDone = true
	if !h.streamStarted {
		h.ensureInitialized(c, &openai_compatible.ChatCompletionsStreamResponse{})
	}
	type indexedItem struct {
		index int
		item  openai.OutputItem
	}
	items := []indexedItem{}
	for _, sealed := range h.sealedMessages {
		items = append(items, indexedItem{sealed.index, sealed.item})
	}
	for _, choice := range h.choiceOrder {
		item := h.choiceMessage(choice, "incomplete")
		if item.Content[0].Text != "" {
			items = append(items, indexedItem{choice.messageIndex, item})
		}
		if reasoning := h.choiceReasoning(choice, "incomplete"); reasoning != nil {
			items = append(items, indexedItem{choice.reasoningIndex, *reasoning})
		}
		for _, sealed := range choice.sealedReasoning {
			items = append(items, indexedItem{sealed.index, sealed.item})
		}
	}
	for _, id := range h.toolOrder {
		call := h.toolCalls[id]
		items = append(items, indexedItem{call.index, h.toolItem(call, "incomplete", true)})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].index < items[j].index })
	output := make([]openai.OutputItem, 0, len(items))
	for _, item := range items {
		output = append(output, item.item)
	}
	response := h.buildFinalResponse("failed", output, nil)
	response.Error = &failure.Error
	h.commitStreamedResponse(c, response, "failed")
	response.Output = clientVisibleResponseOutput(response.Output, h.includeEncrypted)
	h.emitEvent(c, "response.failed", openai.ResponseAPIStreamEvent{Type: "response.failed", Response: response})
	return true, true
}
