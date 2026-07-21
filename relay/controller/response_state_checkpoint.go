package controller

import (
	"encoding/json"
	"net/http/httptest"
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/metrics"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/state"
)

// checkpoint client-family labels mixed into a checkpoint key so a Chat transcript
// never matches a Claude one, or vice versa (CP06).
const (
	checkpointClientFamilyChat   = "chat"
	checkpointClientFamilyClaude = "claude"
)

// checkpointBindingFromMeta builds the provider-binding identity that scopes a
// stateless-client checkpoint to one route (Section 5.7): channel, api type, and
// actual model. A checkpoint recorded on one route never matches another (CP06).
func checkpointBindingFromMeta(meta *metalib.Meta) *state.ProviderBinding {
	return &state.ProviderBinding{
		ChannelID:   meta.ChannelId,
		APIType:     meta.APIType,
		ActualModel: meta.ActualModelName,
	}
}

// canonicalContentString renders arbitrary Chat message content to a stable
// string. A plain string passes through; structured content is encoded with
// encoding/json, whose object keys are emitted in sorted order, so identical
// visible content always produces identical bytes. Determinism is required for a
// match to succeed; any nondeterminism merely defeats a match (fail open) and can
// never cause a wrong match.
func canonicalContentString(content any) string {
	if content == nil {
		return ""
	}
	if s, ok := content.(string); ok {
		return s
	}
	if b, err := json.Marshal(content); err == nil {
		return string(b)
	}
	return ""
}

// chatMessageToCheckpoint maps one Chat message to the canonical checkpoint form.
// Tool-call ids/names/arguments and the tool_call_id link are folded in so a
// changed tool call yields a different key (CP04); an Anthropic signature (when a
// client echoes one) participates as well (CP02).
func chatMessageToCheckpoint(m relaymodel.Message) state.CheckpointMessage {
	cm := state.CheckpointMessage{
		Role:       m.Role,
		Content:    canonicalContentString(m.Content),
		ToolCallID: m.ToolCallId,
	}
	if m.Name != nil {
		cm.Name = *m.Name
	}
	if m.Signature != nil {
		cm.Signature = *m.Signature
	}
	if len(m.ToolCalls) > 0 {
		if b, err := json.Marshal(m.ToolCalls); err == nil {
			cm.Content = cm.Content + "\x00tool_calls:" + string(b)
		}
	}
	return cm
}

// chatMessagesToCheckpoint deterministically maps Chat Completions messages into
// the format-agnostic checkpoint transcript.
func chatMessagesToCheckpoint(messages []relaymodel.Message) []state.CheckpointMessage {
	out := make([]state.CheckpointMessage, 0, len(messages))
	for i := range messages {
		out = append(out, chatMessageToCheckpoint(messages[i]))
	}
	return out
}

// convertedResponseAPIRequest returns the converted *ResponseAPIRequest that the
// adaptor stashes when the selected upstream is Responses-format, or nil otherwise.
// Its presence is exactly how the render path itself decides Chat<-Responses, so it
// is the correct discriminator for whether a checkpoint is even relevant.
func convertedResponseAPIRequest(c *gin.Context) *openai.ResponseAPIRequest {
	if v, ok := c.Get(ctxkey.ConvertedRequest); ok {
		if req, ok := v.(*openai.ResponseAPIRequest); ok {
			return req
		}
	}
	return nil
}

// chatCheckpointTranscript builds the full downstream-visible transcript to record:
// the request messages plus the rendered assistant turn the client will echo back
// on its next stateless request. Keying on the assistant turn is what makes the
// next request's longest-prefix match end after it, so only the genuinely new user
// delta is sent.
func chatCheckpointTranscript(c *gin.Context, messages []relaymodel.Message) []state.CheckpointMessage {
	msgs := chatMessagesToCheckpoint(messages)
	if v, ok := c.Get(ctxkey.ResponseAPIAssistantMessage); ok {
		if am, ok := v.(relaymodel.Message); ok {
			if am.Role == "" {
				am.Role = "assistant"
			}
			msgs = append(msgs, chatMessageToCheckpoint(am))
		}
	}
	return msgs
}

// matchChatCheckpoint attempts to shortcut a Chat request that is being served by a
// Responses upstream: it finds the longest exact checkpoint prefix of the request
// transcript and, on a hit, rewrites the converted request to continue from the
// bound upstream handle while sending only the unmatched delta. It returns the
// re-serialized request body and true on a hit; false leaves the request untouched
// for an ordinary full replay. Gated on the feature and a converted
// *ResponseAPIRequest; it fails open on any doubt (CP03, CP07, CP08, CP10).
func matchChatCheckpoint(c *gin.Context, meta *metalib.Meta, textRequest *relaymodel.GeneralOpenAIRequest) ([]byte, bool) {
	if !responseStateActive(meta) || textRequest == nil {
		return nil, false
	}
	convReq := convertedResponseAPIRequest(c)
	if convReq == nil {
		return nil, false
	}
	// Never override an explicit prior handle — explicit selectors fail closed, not
	// through this optimization.
	if convReq.PreviousResponseId != nil && strings.TrimSpace(*convReq.PreviousResponseId) != "" {
		return nil, false
	}
	owner := stateOwnerFromMeta(meta)
	if !owner.Valid() {
		return nil, false
	}
	msgs := chatMessagesToCheckpoint(textRequest.Messages)
	if len(msgs) == 0 {
		return nil, false
	}
	binding := checkpointBindingFromMeta(meta)
	rec, prefixLen, hit := state.LongestCheckpointMatch(gmw.Ctx(c), state.Store(), owner, checkpointClientFamilyChat, meta.OriginModelName, binding, msgs)
	if !hit || rec == nil || rec.ResponseID == "" {
		return nil, false
	}
	// The delta is the messages beyond the matched prefix. If nothing new remains,
	// the client resent an already-processed transcript verbatim; replay normally.
	if prefixLen >= len(textRequest.Messages) {
		return nil, false
	}
	deltaMessages := textRequest.Messages[prefixLen:]
	deltaReq := openai.ConvertChatCompletionToResponseAPI(&relaymodel.GeneralOpenAIRequest{
		Model:    textRequest.Model,
		Messages: deltaMessages,
	})
	upstream := rec.ResponseID
	convReq.PreviousResponseId = &upstream
	convReq.Input = deltaReq.Input
	body, err := json.Marshal(convReq)
	if err != nil {
		return nil, false
	}
	metrics.RecordStateEvent(metrics.StateCategoryPath, metrics.StateOutcomeHydrated)
	return body, true
}

// recordChatCheckpoint records a continuation checkpoint after a Chat request has
// been served by a Responses upstream, mapping the full downstream-visible
// transcript (request messages plus the rendered assistant turn) to the upstream
// response handle. No-op when the feature is inactive or the upstream was not
// Responses-format (no upstream id was surfaced). Ambiguity is handled by the
// algorithm (CP07); failures are logged, never fatal.
func recordChatCheckpoint(c *gin.Context, meta *metalib.Meta, textRequest *relaymodel.GeneralOpenAIRequest) {
	if !responseStateActive(meta) || textRequest == nil {
		return
	}
	upstreamID := c.GetString(ctxkey.ResponseAPIUpstreamID)
	if upstreamID == "" {
		return
	}
	owner := stateOwnerFromMeta(meta)
	if !owner.Valid() {
		return
	}
	msgs := chatCheckpointTranscript(c, textRequest.Messages)
	binding := checkpointBindingFromMeta(meta)
	if err := state.RecordCheckpoint(gmw.Ctx(c), state.Store(), owner, checkpointClientFamilyChat, meta.OriginModelName, binding, msgs, upstreamID, state.ResponseTTLFromConfig()); err != nil {
		gmw.GetLogger(c).Warn("record chat checkpoint failed", zap.Error(err))
	}
}

// --- Claude Messages checkpoint (M08) --------------------------------------

// claudeMessagesToCheckpoint deterministically maps Claude Messages into the
// canonical checkpoint transcript. Claude tool_use/tool_result blocks and signed
// thinking live inside the content array, so a JSON-canonical rendering of the
// content already folds them into the key (CP02, CP04).
func claudeMessagesToCheckpoint(messages []relaymodel.ClaudeMessage) []state.CheckpointMessage {
	out := make([]state.CheckpointMessage, 0, len(messages))
	for i := range messages {
		out = append(out, state.CheckpointMessage{
			Role:    messages[i].Role,
			Content: canonicalContentString(messages[i].Content),
		})
	}
	return out
}

// claudeCheckpointTranscript appends the rendered assistant turn (surfaced by the
// Claude<-Responses render handler) to the request transcript, so the next
// request's longest-prefix match ends after the assistant turn.
func claudeCheckpointTranscript(c *gin.Context, messages []relaymodel.ClaudeMessage) []state.CheckpointMessage {
	msgs := claudeMessagesToCheckpoint(messages)
	if v, ok := c.Get(ctxkey.ResponseAPIAssistantMessage); ok {
		if am, ok := v.(relaymodel.Message); ok {
			role := am.Role
			if role == "" {
				role = "assistant"
			}
			msgs = append(msgs, state.CheckpointMessage{Role: role, Content: canonicalContentString(am.Content)})
		}
	}
	return msgs
}

// convertClaudeDeltaToResponsesInput converts only the delta Claude messages
// (messages[prefixLen:]) into a Responses input, so a matched checkpoint sends only
// the new turn. The conversion runs on a THROWAWAY gin context so none of the live
// request's ctxkeys (ConvertedRequest, ClaudeMessagesConversion, tool-name maps)
// are mutated. System/instructions are cleared because a previous_response_id
// continuation does not re-inherit them (R4).
func convertClaudeDeltaToResponsesInput(meta *metalib.Meta, claudeRequest *relaymodel.ClaudeRequest, prefixLen int) (openai.ResponseAPIInput, bool) {
	if prefixLen < 0 || prefixLen >= len(claudeRequest.Messages) {
		return nil, false
	}
	delta := *claudeRequest
	delta.System = nil
	delta.Messages = claudeRequest.Messages[prefixLen:]

	throwaway, _ := gin.CreateTestContext(httptest.NewRecorder())
	if meta != nil {
		throwaway.Set(ctxkey.Meta, meta)
	}
	converted, err := openai_compatible.ConvertClaudeRequest(throwaway, &delta)
	if err != nil {
		return nil, false
	}
	openaiReq, ok := converted.(*relaymodel.GeneralOpenAIRequest)
	if !ok {
		return nil, false
	}
	responsesReq := openai.ConvertChatCompletionToResponseAPI(openaiReq)
	if len(responsesReq.Input) == 0 {
		return nil, false
	}
	return responsesReq.Input, true
}

// matchClaudeCheckpoint mirrors matchChatCheckpoint for Claude Messages clients
// served by a Responses upstream (M08). It fails open on any doubt.
func matchClaudeCheckpoint(c *gin.Context, meta *metalib.Meta, claudeRequest *relaymodel.ClaudeRequest) ([]byte, bool) {
	if !responseStateActive(meta) || claudeRequest == nil {
		return nil, false
	}
	convReq := convertedResponseAPIRequest(c)
	if convReq == nil {
		return nil, false
	}
	if convReq.PreviousResponseId != nil && strings.TrimSpace(*convReq.PreviousResponseId) != "" {
		return nil, false
	}
	owner := stateOwnerFromMeta(meta)
	if !owner.Valid() {
		return nil, false
	}
	msgs := claudeMessagesToCheckpoint(claudeRequest.Messages)
	if len(msgs) == 0 {
		return nil, false
	}
	binding := checkpointBindingFromMeta(meta)
	rec, prefixLen, hit := state.LongestCheckpointMatch(gmw.Ctx(c), state.Store(), owner, checkpointClientFamilyClaude, meta.OriginModelName, binding, msgs)
	if !hit || rec == nil || rec.ResponseID == "" || prefixLen >= len(claudeRequest.Messages) {
		return nil, false
	}
	deltaInput, ok := convertClaudeDeltaToResponsesInput(meta, claudeRequest, prefixLen)
	if !ok {
		return nil, false
	}
	upstream := rec.ResponseID
	convReq.PreviousResponseId = &upstream
	convReq.Input = deltaInput
	body, err := json.Marshal(convReq)
	if err != nil {
		return nil, false
	}
	metrics.RecordStateEvent(metrics.StateCategoryPath, metrics.StateOutcomeHydrated)
	return body, true
}

// recordClaudeCheckpoint records a continuation checkpoint after a Claude Messages
// request has been served by a Responses upstream (M08). No-op when the feature is
// inactive or the upstream was not Responses-format.
func recordClaudeCheckpoint(c *gin.Context, meta *metalib.Meta, claudeRequest *relaymodel.ClaudeRequest) {
	if !responseStateActive(meta) || claudeRequest == nil {
		return
	}
	upstreamID := c.GetString(ctxkey.ResponseAPIUpstreamID)
	if upstreamID == "" {
		return
	}
	owner := stateOwnerFromMeta(meta)
	if !owner.Valid() {
		return
	}
	msgs := claudeCheckpointTranscript(c, claudeRequest.Messages)
	binding := checkpointBindingFromMeta(meta)
	if err := state.RecordCheckpoint(gmw.Ctx(c), state.Store(), owner, checkpointClientFamilyClaude, meta.OriginModelName, binding, msgs, upstreamID, state.ResponseTTLFromConfig()); err != nil {
		gmw.GetLogger(c).Warn("record claude checkpoint failed", zap.Error(err))
	}
}
