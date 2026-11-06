package controller

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
)

// codexFallbackStreamEvent is the subset of a Responses SSE event the stream
// assertions inspect.
type codexFallbackStreamEvent struct {
	Type      string          `json:"type"`
	ItemID    string          `json:"item_id"`
	Delta     json.RawMessage `json:"delta"`
	Arguments string          `json:"arguments"`
	Input     string          `json:"input"`
	Item      json.RawMessage `json:"item"`
	Response  json.RawMessage `json:"response"`
}

// codexFallbackItemHeader is the identity of an output item in an event.
type codexFallbackItemHeader struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Status string `json:"status"`
}

// codexFallbackCallTrace collects the events one tool call produced.
type codexFallbackCallTrace struct {
	sequence  []string
	added     json.RawMessage
	done      json.RawMessage
	deltas    strings.Builder
	arguments string
	input     string
	doneAt    int
}

// collectCodexFallbackStream decodes the SSE body and groups tool events by
// call ID. Parameters: t owns assertions and body is the SSE text. Returns the
// per-call traces, the response.completed position and its response payload.
func collectCodexFallbackStream(t *testing.T, body string) (map[string]*codexFallbackCallTrace, int, json.RawMessage) {
	t.Helper()
	traces := map[string]*codexFallbackCallTrace{}
	trace := func(id string) *codexFallbackCallTrace {
		if traces[id] == nil {
			traces[id] = &codexFallbackCallTrace{doneAt: -1}
		}
		return traces[id]
	}
	completedAt, completed := -1, json.RawMessage(nil)
	for position, raw := range parseSSEEvents(body) {
		if raw.data == "[DONE]" || raw.data == "" {
			continue
		}
		var event codexFallbackStreamEvent
		require.NoError(t, json.Unmarshal([]byte(raw.data), &event), "event %q", raw.event)
		switch event.Type {
		case "response.output_item.added", "response.output_item.done":
			var header codexFallbackItemHeader
			require.NoError(t, json.Unmarshal(event.Item, &header))
			if header.Type == "message" {
				continue
			}
			call := trace(header.ID)
			call.sequence = append(call.sequence, event.Type)
			if event.Type == "response.output_item.added" {
				call.added = event.Item
			} else {
				call.done, call.doneAt = event.Item, position
			}
		case "response.function_call_arguments.delta", "response.custom_tool_call_input.delta":
			call := trace(event.ItemID)
			call.sequence = append(call.sequence, event.Type)
			var text string
			require.NoError(t, json.Unmarshal(event.Delta, &text))
			call.deltas.WriteString(text)
		case "response.function_call_arguments.done":
			call := trace(event.ItemID)
			call.sequence = append(call.sequence, event.Type)
			call.arguments = event.Arguments
		case "response.custom_tool_call_input.done":
			call := trace(event.ItemID)
			call.sequence = append(call.sequence, event.Type)
			call.input = event.Input
		case "response.completed":
			require.Equal(t, -1, completedAt, "exactly one response.completed")
			completedAt, completed = position, event.Response
		}
	}
	require.NotEqual(t, -1, completedAt, "missing response.completed")
	return traces, completedAt, completed
}

// compactCodexFallbackSequence collapses repeated delta events so sequences
// compare independently of upstream chunking.
func compactCodexFallbackSequence(sequence []string) []string {
	compact := make([]string, 0, len(sequence))
	for _, name := range sequence {
		if len(compact) > 0 && compact[len(compact)-1] == name && strings.HasSuffix(name, ".delta") {
			continue
		}
		compact = append(compact, name)
	}
	return compact
}

// TestResponseFallbackCodexToolsHTTPStream relays the synthetic Codex request
// as SSE through the OpenAI-compatible Chat stream and the Anthropic Claude
// stream (content_block_start + input_json_delta) fallbacks. It proves the
// upstream receives the flattened catalog and lowered history, each tool call
// streams with its kind's event sequence (function arguments deltas; custom
// input buffered into one custom_tool_call_input.delta + .done; tool_search
// with no argument events), every output_item.done carries the final item
// before response.completed, and ordinary token billing settles.
func TestResponseFallbackCodexToolsHTTPStream(t *testing.T) {
	const balance = int64(1_000_000)
	for _, channel := range []int{channeltype.OpenAICompatible, channeltype.Anthropic} {
		t.Run(channeltype.IdToName(channel), func(t *testing.T) {
			c, recorder, requestID, got, _ := codexFallbackRun(t, channel, true, balance)
			requireCodexFallbackUpstream(t, channel, got)
			require.Contains(t, recorder.Header().Get("Content-Type"), "text/event-stream")

			traces, completedAt, completed := collectCodexFallbackStream(t, recorder.Body.String())
			upstream := codexFallbackUpstreamCalls(channel)
			expected := codexFallbackExpectedItems(channel)
			require.Len(t, traces, len(upstream), "one trace per upstream tool call")
			functionSequence := []string{"response.output_item.added", "response.function_call_arguments.delta", "response.function_call_arguments.done", "response.output_item.done"}
			for i, call := range upstream {
				id := codexFallbackRenderedCallID(call.id)
				trace := traces[id]
				require.NotNil(t, trace, "missing events for %s", id)
				var added, done codexFallbackItemHeader
				require.NoError(t, json.Unmarshal(trace.added, &added))
				require.NoError(t, json.Unmarshal(trace.done, &done))
				require.Equal(t, "in_progress", added.Status)
				require.Equal(t, done.Type, added.Type, "added and done agree on the item type")
				require.JSONEq(t, expected[i], string(trace.done), "output_item.done carries the complete final item")
				require.Less(t, trace.doneAt, completedAt, "output_item.done precedes response.completed")

				switch call.name {
				case "apply_patch":
					require.Equal(t, "custom_tool_call", added.Type)
					require.Equal(t, []string{"response.output_item.added", "response.custom_tool_call_input.delta", "response.custom_tool_call_input.done", "response.output_item.done"}, trace.sequence,
						"custom input is released once, without function_call_arguments events")
					require.Equal(t, codexFallbackPatch, trace.deltas.String())
					require.Equal(t, codexFallbackPatch, trace.input)
					require.Contains(t, string(trace.added), `"input":""`, "the added item carries the required, still-empty input field")
				case "tool_search":
					require.Equal(t, "tool_search_call", added.Type)
					require.Equal(t, []string{"response.output_item.added", "response.output_item.done"}, trace.sequence, "client tool_search has no argument events")
					require.Contains(t, string(trace.added), `"execution":"client"`)
					require.Contains(t, string(trace.added), `"arguments":{}`, "the added item carries the required, still-empty arguments object")
					require.Contains(t, string(trace.done), `"limit":8`)
					require.NotContains(t, string(trace.done), `8.0`)
				default:
					require.Equal(t, "function_call", added.Type)
					require.Equal(t, functionSequence, compactCodexFallbackSequence(trace.sequence))
					require.Equal(t, call.arguments, trace.deltas.String())
					require.Equal(t, call.arguments, trace.arguments)
					var addedItem struct {
						Name                  string    `json:"name"`
						Namespace             string    `json:"namespace"`
						EncryptedFunctionArgs *[]string `json:"encrypted_function_args"`
					}
					require.NoError(t, json.Unmarshal(trace.added, &addedItem))
					if call.name == "exec_command" {
						require.Equal(t, "exec_command", addedItem.Name)
						require.Empty(t, addedItem.Namespace)
						require.Nil(t, addedItem.EncryptedFunctionArgs, "root calls carry no encrypted_function_args")
					} else {
						require.NotEmpty(t, addedItem.Namespace, "the added item already splits namespace and name")
						require.NotContains(t, addedItem.Name, "__")
						require.NotNil(t, addedItem.EncryptedFunctionArgs)
						require.Empty(t, *addedItem.EncryptedFunctionArgs)
					}
				}
			}

			var response struct {
				Status string            `json:"status"`
				Output []json.RawMessage `json:"output"`
				Usage  struct {
					Input  int `json:"input_tokens"`
					Output int `json:"output_tokens"`
					Total  int `json:"total_tokens"`
				} `json:"usage"`
			}
			require.NoError(t, json.Unmarshal(completed, &response))
			require.Equal(t, "completed", response.Status)
			requireCodexFallbackOutput(t, channel, response.Output)
			require.Equal(t, 11, response.Usage.Input)
			require.Equal(t, 7, response.Usage.Output)
			require.Equal(t, 18, response.Usage.Total)
			requireFallbackTokenSettlement(t, c, requestID, balance, got.User, got.Token)
		})
	}
}
