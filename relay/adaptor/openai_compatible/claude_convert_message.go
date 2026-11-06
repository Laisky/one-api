package openai_compatible

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"regexp"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common/random"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// Claude Messages stop_reason values produced by converted upstreams.
const (
	ClaudeStopEndTurn   = "end_turn"
	ClaudeStopMaxTokens = "max_tokens"
	ClaudeStopToolUse   = "tool_use"
	ClaudeStopRefusal   = "refusal"
)

// claudeToolIDInvalidChars matches characters outside the tool_use id grammar
// ^[a-zA-Z0-9_-]+$ that the Messages API enforces when ids are echoed back.
var claudeToolIDInvalidChars = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// ClaudeMessageID returns upstreamID when present and otherwise a generated
// Claude-style message id, so a Message never carries an empty id.
func ClaudeMessageID(upstreamID string) string {
	if id := strings.TrimSpace(upstreamID); id != "" {
		return id
	}
	return "msg_" + random.GetRandomString(24)
}

// ClaudeStopReasonFromChatFinish maps an OpenAI Chat Completions finish_reason
// to a Claude stop_reason: stop → end_turn, length → max_tokens, tool_calls and
// function_call → tool_use, content_filter → refusal. Unknown or empty values
// map to end_turn.
func ClaudeStopReasonFromChatFinish(reason string) string {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "length", "max_tokens", "max_output_tokens":
		return ClaudeStopMaxTokens
	case "tool_calls", "function_call", "tool_use":
		return ClaudeStopToolUse
	case "content_filter", "refusal":
		return ClaudeStopRefusal
	default:
		return ClaudeStopEndTurn
	}
}

// ClaudeStopReasonFromResponseStatus maps an OpenAI Responses status and its
// incomplete_details.reason to a Claude stop_reason: an incomplete response
// stopped by max_output_tokens (or an unspecified reason) is max_tokens, one
// stopped by content_filter is refusal, and everything else is end_turn.
func ClaudeStopReasonFromResponseStatus(status, incompleteReason string) string {
	if !strings.EqualFold(strings.TrimSpace(status), "incomplete") {
		return ClaudeStopEndTurn
	}
	if strings.EqualFold(strings.TrimSpace(incompleteReason), "content_filter") {
		return ClaudeStopRefusal
	}
	return ClaudeStopMaxTokens
}

// FinalizeClaudeStopReason reconciles a mapped stop_reason with the rendered
// content: an empty reason becomes end_turn, an end_turn turn that requests
// tools becomes tool_use (some providers report "stop" for tool turns), and a
// tool_use reason without any tool_use block becomes end_turn.
func FinalizeClaudeStopReason(reason string, hasToolUse bool) string {
	switch {
	case reason == "":
		reason = ClaudeStopEndTurn
	case reason == ClaudeStopToolUse && !hasToolUse:
		return ClaudeStopEndTurn
	}
	if hasToolUse && reason == ClaudeStopEndTurn {
		return ClaudeStopToolUse
	}
	return reason
}

// ClaudeContentHasToolUse reports whether content contains a tool_use block.
func ClaudeContentHasToolUse(content []relaymodel.ClaudeContent) bool {
	for _, block := range content {
		if block.Type == claudeBlockToolUse {
			return true
		}
	}
	return false
}

// ClaudeToolInputFromArguments converts upstream tool-call arguments (a JSON
// string, raw JSON, or an already-decoded value) into a tool_use input object.
// Empty, null, or non-object arguments become {}; a JSON string that itself
// encodes an object (double-encoded arguments) is unwrapped once.
func ClaudeToolInputFromArguments(args any) json.RawMessage {
	var raw []byte
	switch v := args.(type) {
	case nil:
		return json.RawMessage(`{}`)
	case string:
		raw = []byte(v)
	case json.RawMessage:
		raw = v
	case []byte:
		raw = v
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return json.RawMessage(`{}`)
		}
		raw = encoded
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || !json.Valid(raw) {
		return json.RawMessage(`{}`)
	}
	switch raw[0] {
	case '{':
		return json.RawMessage(raw)
	case '"':
		var inner string
		if json.Unmarshal(raw, &inner) == nil {
			inner = strings.TrimSpace(inner)
			if strings.HasPrefix(inner, "{") && json.Valid([]byte(inner)) {
				return json.RawMessage(inner)
			}
		}
	}
	return json.RawMessage(`{}`)
}

// ClaudeToolIDAllocator assigns tool_use ids that satisfy the Messages API id
// grammar and are unique within one response. Valid unique upstream ids pass
// through unchanged so later tool_result echoes still match the upstream call.
type ClaudeToolIDAllocator struct {
	seen map[string]bool
}

// NewClaudeToolIDAllocator returns an allocator for one response.
func NewClaudeToolIDAllocator() *ClaudeToolIDAllocator {
	return &ClaudeToolIDAllocator{seen: map[string]bool{}}
}

// Allocate returns a valid, unique tool_use id derived from upstreamID: invalid
// characters become "_", an empty id is generated, and a repeated id gains a
// numeric suffix.
func (a *ClaudeToolIDAllocator) Allocate(upstreamID string) string {
	id := claudeToolIDInvalidChars.ReplaceAllString(strings.TrimSpace(upstreamID), "_")
	if id == "" {
		id = "toolu_" + random.GetRandomString(24)
	}
	candidate := id
	for n := 2; a.seen[candidate]; n++ {
		candidate = fmt.Sprintf("%s_%d", id, n)
	}
	a.seen[candidate] = true
	return candidate
}

// MarshalClaudeHTTPResponse renders payload as the JSON body of a new HTTP
// response that keeps orig's status and headers with a JSON content type and
// an exact Content-Length. It returns the response or a wrapped marshal error.
func MarshalClaudeHTTPResponse(orig *http.Response, payload relaymodel.ClaudeResponse) (*http.Response, *relaymodel.ErrorWithStatusCode) {
	if payload.Content == nil {
		payload.Content = []relaymodel.ClaudeContent{}
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, ErrorWrapper(errors.Wrap(err, "marshal_claude_response"), "marshal_claude_response_failed", http.StatusInternalServerError)
	}
	newResp := &http.Response{
		StatusCode: orig.StatusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(b)),
	}
	maps.Copy(newResp.Header, orig.Header)
	newResp.Header.Set("Content-Type", "application/json")
	newResp.Header.Set("Content-Length", fmt.Sprintf("%d", len(b)))
	return newResp, nil
}
