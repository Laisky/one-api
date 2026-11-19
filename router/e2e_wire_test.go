package router

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	// e2eChat, e2eResponses and e2eMessages name the three client protocols.
	e2eChat      = "chat"
	e2eResponses = "responses"
	e2eMessages  = "messages"
	// e2eMaxTokens is the output limit every client request carries.
	e2eMaxTokens = 256
)

// e2eProtocols lists every client protocol.
var e2eProtocols = []string{e2eChat, e2eResponses, e2eMessages}

// e2eClientRequest returns the endpoint and JSON body of a client request in
// protocol for modelName. Parameters: stream selects SSE delivery and extra is
// appended to the root object (for example `,"service_tier":"priority"`).
func e2eClientRequest(protocol, modelName string, stream bool, extra string) (string, string) {
	streamField := ""
	if stream {
		streamField = `,"stream":true`
	}
	limit := strconv.Itoa(e2eMaxTokens)
	switch protocol {
	case e2eResponses:
		return "/v1/responses", `{"model":"` + modelName + `","input":"hello","max_output_tokens":` + limit + streamField + extra + `}`
	case e2eMessages:
		return "/v1/messages", `{"model":"` + modelName + `","messages":[{"role":"user","content":"hello"}],"max_tokens":` + limit + streamField + extra + `}`
	default:
		return "/v1/chat/completions", `{"model":"` + modelName + `","messages":[{"role":"user","content":"hello"}],"max_tokens":` + limit + streamField + extra + `}`
	}
}

// e2eSSEEvent is one parsed server-sent event.
type e2eSSEEvent struct {
	Event string
	Data  string
}

// e2eParseSSE splits an SSE body into events. Parameters: t owns assertions and
// body is the raw stream. Returns: the events in order.
func e2eParseSSE(t *testing.T, body string) []e2eSSEEvent {
	t.Helper()
	var events []e2eSSEEvent
	for _, block := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n\n") {
		var event e2eSSEEvent
		var data []string
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event:"):
				event.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		if event.Event == "" && len(data) == 0 {
			continue
		}
		event.Data = strings.Join(data, "\n")
		events = append(events, event)
	}
	require.NotEmpty(t, events, "an SSE response must carry events: %q", body)
	return events
}

// e2eDecode decodes a JSON object. Parameters: t owns assertions and raw is
// the JSON text. Returns: the decoded object.
func e2eDecode(t *testing.T, raw string) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &out), "invalid JSON: %q", raw)
	return out
}

// e2eRequireSuccessWire asserts the client received a well-formed successful
// response in its own protocol and delivery mode, answering "ack".
// Parameters: t owns assertions, protocol and stream describe the request and
// res is the client result.
func e2eRequireSuccessWire(t *testing.T, protocol string, stream bool, res e2eResult) {
	t.Helper()
	require.Equal(t, 200, res.Code, "body=%s", res.Body)
	if stream {
		require.Contains(t, res.Header.Get("Content-Type"), "text/event-stream", "body=%s", res.Body)
		e2eRequireSuccessStream(t, protocol, e2eParseSSE(t, res.Body))
		return
	}
	require.Contains(t, res.Header.Get("Content-Type"), "application/json")
	root := e2eDecode(t, res.Body)
	switch protocol {
	case e2eResponses:
		require.Equal(t, "response", root["object"], "body=%s", res.Body)
		require.Equal(t, "completed", root["status"], "body=%s", res.Body)
		require.Contains(t, res.Body, `"output_text"`)
		require.Contains(t, res.Body, `"ack"`)
		require.NotNil(t, root["usage"], "body=%s", res.Body)
	case e2eMessages:
		require.Equal(t, "message", root["type"], "body=%s", res.Body)
		require.Equal(t, "assistant", root["role"], "body=%s", res.Body)
		content, _ := root["content"].([]any)
		require.NotEmpty(t, content, "body=%s", res.Body)
		block, _ := content[len(content)-1].(map[string]any)
		require.Equal(t, "text", block["type"], "body=%s", res.Body)
		require.Equal(t, "ack", block["text"], "body=%s", res.Body)
		require.NotNil(t, root["usage"], "body=%s", res.Body)
	default:
		// The response identity (id, object, created, model) is asserted by
		// e2eRequireChatIdentity where the route preserves it.
		choices, _ := root["choices"].([]any)
		require.Len(t, choices, 1, "body=%s", res.Body)
		message, _ := choices[0].(map[string]any)["message"].(map[string]any)
		require.Equal(t, "ack", message["content"], "body=%s", res.Body)
		require.NotNil(t, root["usage"], "body=%s", res.Body)
	}
}

// e2eRequireChatIdentity asserts that a non-streaming Chat Completions body
// carries its response identity: id, object "chat.completion", created and
// model. Parameters: t owns assertions, res is the client result and modelName
// the expected model.
func e2eRequireChatIdentity(t *testing.T, res e2eResult, modelName string) {
	t.Helper()
	root := e2eDecode(t, res.Body)
	require.Equal(t, "chat.completion", root["object"], "body=%s", res.Body)
	require.NotEmpty(t, root["id"], "body=%s", res.Body)
	require.Equal(t, modelName, root["model"], "body=%s", res.Body)
	created, _ := root["created"].(float64)
	require.Positive(t, created, "body=%s", res.Body)
}

// e2eRequireSuccessStream asserts one protocol's successful SSE grammar and
// that the streamed text is "ack". Parameters: t owns assertions, protocol
// names the client protocol and events are the parsed events.
func e2eRequireSuccessStream(t *testing.T, protocol string, events []e2eSSEEvent) {
	t.Helper()
	var text strings.Builder
	switch protocol {
	case e2eResponses:
		var completed bool
		for _, event := range events {
			if event.Data == "[DONE]" {
				continue
			}
			root := e2eDecode(t, event.Data)
			kind, _ := root["type"].(string)
			if event.Event != "" {
				require.Equal(t, event.Event, kind, "the SSE event name must match the payload type")
			}
			switch kind {
			case "response.output_text.delta":
				delta, _ := root["delta"].(string)
				text.WriteString(delta)
			case "response.completed":
				completed = true
				response, _ := root["response"].(map[string]any)
				require.NotNil(t, response["usage"], "response.completed must carry usage")
			}
		}
		require.True(t, completed, "a Responses stream must end with response.completed: %+v", events)
	case e2eMessages:
		var names []string
		for _, event := range events {
			root := e2eDecode(t, event.Data)
			kind, _ := root["type"].(string)
			require.Equal(t, event.Event, kind, "the SSE event name must match the payload type")
			names = append(names, kind)
			if kind == "content_block_delta" {
				delta, _ := root["delta"].(map[string]any)
				if delta["type"] == "text_delta" {
					value, _ := delta["text"].(string)
					text.WriteString(value)
				}
			}
		}
		require.NotEmpty(t, names)
		require.Equal(t, "message_start", names[0], "events=%v", names)
		require.Equal(t, "message_stop", names[len(names)-1], "events=%v", names)
		require.Contains(t, names, "message_delta")
	default:
		require.Equal(t, "[DONE]", events[len(events)-1].Data, "a Chat stream must end with [DONE]")
		for _, event := range events[:len(events)-1] {
			root := e2eDecode(t, event.Data)
			require.Equal(t, "chat.completion.chunk", root["object"], "data=%s", event.Data)
			// Every chunk carries the stream identity, including chunks converted
			// from Responses delta events that have no response object.
			require.NotEmpty(t, root["id"], "data=%s", event.Data)
			require.NotEmpty(t, root["model"], "data=%s", event.Data)
			created, _ := root["created"].(float64)
			require.Positive(t, created, "data=%s", event.Data)
			choices, _ := root["choices"].([]any)
			for _, choice := range choices {
				delta, _ := choice.(map[string]any)["delta"].(map[string]any)
				content, _ := delta["content"].(string)
				text.WriteString(content)
			}
		}
	}
	require.Equal(t, "ack", text.String(), "streamed text")
}

// e2eRequireErrorWire asserts the client received status with an error body
// in its protocol's shape. Parameters: t owns assertions, protocol names the
// client protocol, res is the client result and status the expected HTTP
// status. Returns: the decoded error object.
func e2eRequireErrorWire(t *testing.T, protocol string, res e2eResult, status int) map[string]any {
	t.Helper()
	require.Equal(t, status, res.Code, "body=%s", res.Body)
	root := e2eDecode(t, res.Body)
	errObject, ok := root["error"].(map[string]any)
	require.True(t, ok, "an error response must carry an error object: %s", res.Body)
	message, _ := errObject["message"].(string)
	require.NotEmpty(t, message, "body=%s", res.Body)
	require.NotEmpty(t, errObject["type"], "body=%s", res.Body)
	if protocol == e2eMessages {
		// Only a native Anthropic error body is replayed in Anthropic shape
		// ({"type":"error",...}); converted and local errors keep the gateway's
		// {"error":{...}} shape, as controller/client_compatibility_claude_error_test.go pins.
		if kind, present := root["type"]; present {
			require.Equal(t, "error", kind, "body=%s", res.Body)
		}
	}
	return errObject
}

// e2eRequirePaidErrorWire asserts how a provider error carrying a paid
// receipt reaches the client: the provider status with an error body, or, for
// a streaming Responses client served through the Chat fallback, a
// protocol-native HTTP 200 stream ending in response.failed that carries the
// error and the receipt. Parameters: t owns assertions, protocol/stream
// describe the request, res is the client result and status the provider status.
func e2eRequirePaidErrorWire(t *testing.T, protocol string, stream bool, res e2eResult, status int) {
	t.Helper()
	if protocol != e2eResponses || !stream || res.Code != 200 {
		e2eRequireErrorWire(t, protocol, res, status)
		return
	}
	events := e2eParseSSE(t, res.Body)
	last := events[len(events)-1]
	require.Equal(t, "response.failed", last.Event, "a streamed paid failure must end in response.failed: %s", res.Body)
	response, _ := e2eDecode(t, last.Data)["response"].(map[string]any)
	require.Equal(t, "failed", response["status"], "body=%s", res.Body)
	require.NotNil(t, response["error"], "response.failed must carry the provider error")
	require.NotNil(t, response["usage"], "response.failed must carry the paid receipt")
}
