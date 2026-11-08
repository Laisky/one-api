package controller

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// requireProviderMatrixUpstream asserts assertion (2): the provider received
// the flattened tool catalog and the lowered Codex history. Chat and Claude
// platforms reuse the shared Codex upstream assertions on their own wire
// format (the provider route was already asserted, so the observation is
// re-labelled with the family path those assertions expect); native Gemini
// has its own decoder because it carries neither call IDs nor tool messages.
// Parameters: t owns assertions, p is the provider and got the observation.
func requireProviderMatrixUpstream(t *testing.T, p providerMatrixProvider, got providerMatrixObservation) {
	t.Helper()
	if p.family == providerMatrixGemini {
		requireProviderMatrixGeminiUpstream(t, got.Body)
		return
	}
	wire := providerMatrixWireChannel(p)
	normalized := got.codexFallbackObservation
	normalized.Path = fallbackUpstreamPath(wire)
	requireCodexFallbackUpstream(t, wire, normalized)
}

// providerMatrixGeminiPart is one native Gemini content part.
type providerMatrixGeminiPart struct {
	Text       string `json:"text"`
	InlineData *struct {
		MimeType string `json:"mimeType"`
		Data     string `json:"data"`
	} `json:"inlineData"`
	FunctionCall *struct {
		Name string          `json:"name"`
		Args json.RawMessage `json:"args"`
	} `json:"functionCall"`
}

// providerMatrixGeminiRequest is the subset of a native Gemini generateContent
// request the matrix inspects.
type providerMatrixGeminiRequest struct {
	Contents []struct {
		Role  string                     `json:"role"`
		Parts []providerMatrixGeminiPart `json:"parts"`
	} `json:"contents"`
	Tools []struct {
		FunctionDeclarations []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Parameters  json.RawMessage `json:"parameters"`
		} `json:"function_declarations"`
	} `json:"tools"`
}

// decodeProviderMatrixGeminiWire normalizes a native Gemini request into the
// shared fallbackWire shape: function declarations become tools, "model"
// contents become assistant turns, text parts concatenate and functionCall
// parts become calls without IDs. Parameters: t owns assertions and body is
// the upstream request. Returns the normalized wire and the raw request.
func decodeProviderMatrixGeminiWire(t *testing.T, body []byte) (fallbackWire, providerMatrixGeminiRequest) {
	t.Helper()
	var request providerMatrixGeminiRequest
	require.NoError(t, json.Unmarshal(body, &request))
	var wire fallbackWire
	for _, group := range request.Tools {
		for _, declaration := range group.FunctionDeclarations {
			wire.Tools = append(wire.Tools, fallbackWireTool{Name: declaration.Name, Description: declaration.Description, Schema: declaration.Parameters})
		}
	}
	for _, content := range request.Contents {
		turn := fallbackWireTurn{Role: content.Role}
		if turn.Role == "model" {
			turn.Role = "assistant"
		}
		for _, part := range content.Parts {
			turn.Text += part.Text
			if part.FunctionCall != nil {
				turn.Calls = append(turn.Calls, fallbackWireCall{Name: part.FunctionCall.Name, Arguments: string(part.FunctionCall.Args)})
			}
		}
		wire.Turns = append(wire.Turns, turn)
	}
	return wire, request
}

// providerMatrixGeminiExpectedTurns derives the documented Gemini lowering of
// codexFallbackExpectedTurns: Gemini functionCall parts carry no call IDs and
// the native adaptor sends each tool result as a user text turn directly after
// its call (positional pairing), so tool turns become user text and calls lose
// their IDs. Returns the expected turns.
func providerMatrixGeminiExpectedTurns() []fallbackWireTurn {
	turns := codexFallbackExpectedTurns()
	out := make([]fallbackWireTurn, 0, len(turns))
	for _, turn := range turns {
		if turn.Role == "tool" {
			out = append(out, fallbackWireTurn{Role: "user", Text: turn.Results[0].Content})
			continue
		}
		var calls []fallbackWireCall
		for _, call := range turn.Calls {
			calls = append(calls, fallbackWireCall{Name: call.Name, Arguments: call.Arguments})
		}
		out = append(out, fallbackWireTurn{Role: turn.Role, Text: turn.Text, Calls: calls})
	}
	return out
}

// requireProviderMatrixGeminiUpstream asserts the native Gemini request for
// the synthetic Codex transcript: flattened function declarations in catalog
// order with namespace description prefixes, apply_patch reduced to one
// required string parameter "input" carrying the grammar in its description,
// tool_search with its own schema, and the lowered history. Gemini's
// functionCall.args is a protobuf Struct whose numbers are doubles, so the
// discovery cursor 9007199254740993 cannot survive as an exact literal; the
// turn comparison is JSON-semantic for that reason. Parameters: t owns
// assertions and body is the upstream request.
func requireProviderMatrixGeminiUpstream(t *testing.T, body []byte) {
	t.Helper()
	wire, _ := decodeProviderMatrixGeminiWire(t, body)
	require.Equal(t, codexFallbackFlatTools, fallbackWireToolNames(wire))
	require.Equal(t, "Run a shell command", fallbackWireToolByName(t, wire, "exec_command").Description)
	require.Equal(t, "[Multi-agent collaboration] Spawn a worker agent", fallbackWireToolByName(t, wire, "collaboration__spawn_agent").Description)
	require.Equal(t, "[GitHub connector] Create an issue", fallbackWireToolByName(t, wire, "mcp__codex_apps__github_create_issue").Description)
	require.Equal(t, "[Docs server] Read one document", fallbackWireToolByName(t, wire, "mcp__docs__read_doc").Description)

	patch := fallbackWireToolByName(t, wire, "apply_patch")
	requireProviderMatrixSchema(t, patch.Schema, map[string]string{"input": "string"}, []string{"input"})
	require.Contains(t, patch.Description, "as the string argument `input`")
	require.Contains(t, patch.Description, "The input must match this lark grammar exactly:\nstart: begin_patch hunk+ end_patch")

	search := fallbackWireToolByName(t, wire, "tool_search")
	require.Equal(t, "# Tool discovery\nSearch deferred tools.", search.Description)
	requireProviderMatrixSchema(t, search.Schema, map[string]string{"query": "string", "limit": "number"}, []string{"query"})

	requireFallbackWireTurns(t, providerMatrixGeminiExpectedTurns(), wire.Turns)
}

// requireProviderMatrixSchema asserts an object schema with exactly the given
// property types and required list. Parameters: t owns assertions, raw is the
// schema, types maps property names to JSON types and required is the list.
func requireProviderMatrixSchema(t *testing.T, raw json.RawMessage, types map[string]string, required []string) {
	t.Helper()
	var schema struct {
		Type       string `json:"type"`
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	require.NoError(t, json.Unmarshal(raw, &schema), "schema %s", raw)
	require.True(t, strings.EqualFold("object", schema.Type), "schema type %q", schema.Type)
	require.Len(t, schema.Properties, len(types), "schema %s", raw)
	for name, kind := range types {
		property, ok := schema.Properties[name]
		require.True(t, ok, "missing property %q in %s", name, raw)
		require.True(t, strings.EqualFold(kind, property.Type), "property %q type %q", name, property.Type)
	}
	require.Equal(t, required, schema.Required)
}

// requireProviderMatrixReplyEnvelope asserts the client reply envelope (JSON
// content type or SSE with one response.completed) is a completed response
// with usage 11/7/18, and returns its output items. Parameters: t owns
// assertions, recorder holds the reply and stream selects SSE parsing.
func requireProviderMatrixReplyEnvelope(t *testing.T, recorder *httptest.ResponseRecorder, stream bool) []json.RawMessage {
	t.Helper()
	raw := json.RawMessage(recorder.Body.Bytes())
	if stream {
		require.Contains(t, recorder.Header().Get("Content-Type"), "text/event-stream")
		_, _, raw = collectCodexFallbackStream(t, recorder.Body.String())
	} else {
		require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
	}
	var reply struct {
		Status string            `json:"status"`
		Output []json.RawMessage `json:"output"`
		Usage  struct {
			Input  int `json:"input_tokens"`
			Output int `json:"output_tokens"`
			Total  int `json:"total_tokens"`
		} `json:"usage"`
	}
	require.NoError(t, json.Unmarshal(raw, &reply))
	require.Equal(t, "completed", reply.Status)
	require.Equal(t, 11, reply.Usage.Input)
	require.Equal(t, 7, reply.Usage.Output)
	require.Equal(t, 18, reply.Usage.Total)
	return reply.Output
}

// requireProviderMatrixItems asserts assertion (4): the rendered output holds
// the "Working." message, then each upstream tool call restored to its Codex
// identity (namespace + name with encrypted_function_args [], custom_tool_call
// apply_patch with raw input, client tool_search_call with object arguments).
// Chat and Claude platforms must echo the provider call IDs exactly; Gemini
// calls carry no provider IDs, so each item must instead carry one minted,
// unique call_ ID shared by id and call_id. Parameters: t owns assertions, p
// is the provider and output the response output array.
func requireProviderMatrixItems(t *testing.T, p providerMatrixProvider, output []json.RawMessage) {
	t.Helper()
	wire := providerMatrixWireChannel(p)
	if p.family != providerMatrixGemini {
		requireCodexFallbackOutput(t, wire, output)
		return
	}
	expected := codexFallbackExpectedItems(wire)
	require.Len(t, output, len(expected)+1)
	var message struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	require.NoError(t, json.Unmarshal(output[0], &message))
	require.Equal(t, "message", message.Type)
	require.Equal(t, "assistant", message.Role)
	require.Len(t, message.Content, 1)
	require.Equal(t, "Working.", message.Content[0].Text)
	seen := map[string]bool{}
	for i, want := range expected {
		got := output[i+1]
		var ids struct {
			ID     string `json:"id"`
			CallID string `json:"call_id"`
		}
		require.NoError(t, json.Unmarshal(got, &ids))
		require.True(t, strings.HasPrefix(ids.ID, "call_"), "output item %d id %q", i+1, ids.ID)
		require.Equal(t, ids.ID, ids.CallID, "output item %d", i+1)
		require.False(t, seen[ids.ID], "output item %d reuses call ID %q", i+1, ids.ID)
		seen[ids.ID] = true
		require.JSONEq(t, providerMatrixWithoutIDs(t, json.RawMessage(want)), providerMatrixWithoutIDs(t, got), "output item %d", i+1)
	}
}

// providerMatrixWithoutIDs returns item re-encoded without its id and call_id
// fields so items compare independently of minted call IDs. Parameters: t owns
// assertions and item is one output item. Returns the reduced JSON text.
func providerMatrixWithoutIDs(t *testing.T, item json.RawMessage) string {
	t.Helper()
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(item, &fields))
	delete(fields, "id")
	delete(fields, "call_id")
	encoded, err := json.Marshal(fields)
	require.NoError(t, err)
	return string(encoded)
}

// requireProviderMatrixStreamEvents asserts the per-call SSE event sequences:
// a function call streams added, argument deltas, arguments.done and done; the
// custom apply_patch call releases its raw input once through one
// custom_tool_call_input.delta and .done without function argument events;
// tool_search has only added and done. Every output_item.done carries the
// final item that response.completed lists and precedes it. Parameters: t
// owns assertions, p is the provider and body the SSE text.
func requireProviderMatrixStreamEvents(t *testing.T, p providerMatrixProvider, body string) {
	t.Helper()
	traces, completedAt, completed := collectCodexFallbackStream(t, body)
	var response struct {
		Output []json.RawMessage `json:"output"`
	}
	require.NoError(t, json.Unmarshal(completed, &response))
	upstream := codexFallbackUpstreamCalls(providerMatrixWireChannel(p))
	require.Len(t, traces, len(upstream), "one trace per upstream tool call")
	require.Len(t, response.Output, len(upstream)+1)
	functionSequence := []string{"response.output_item.added", "response.function_call_arguments.delta", "response.function_call_arguments.done", "response.output_item.done"}
	for i, call := range upstream {
		item := response.Output[i+1]
		var header codexFallbackItemHeader
		require.NoError(t, json.Unmarshal(item, &header))
		trace := traces[header.ID]
		require.NotNil(t, trace, "missing events for %s (%s)", header.ID, call.name)
		require.JSONEq(t, string(item), string(trace.done), "output_item.done carries the final item of %s", call.name)
		require.Less(t, trace.doneAt, completedAt, "output_item.done precedes response.completed")
		var added codexFallbackItemHeader
		require.NoError(t, json.Unmarshal(trace.added, &added))
		require.Equal(t, "in_progress", added.Status)
		require.Equal(t, header.Type, added.Type, "added and done agree on the item type")
		switch call.name {
		case "apply_patch":
			require.Equal(t, "custom_tool_call", added.Type)
			require.Equal(t, []string{"response.output_item.added", "response.custom_tool_call_input.delta", "response.custom_tool_call_input.done", "response.output_item.done"}, trace.sequence)
			require.Equal(t, codexFallbackPatch, trace.deltas.String())
			require.Equal(t, codexFallbackPatch, trace.input)
		case "tool_search":
			require.Equal(t, "tool_search_call", added.Type)
			require.Equal(t, []string{"response.output_item.added", "response.output_item.done"}, trace.sequence, "client tool_search has no argument events")
			require.Contains(t, string(trace.added), `"execution":"client"`)
		default:
			require.Equal(t, "function_call", added.Type)
			require.Equal(t, functionSequence, compactCodexFallbackSequence(trace.sequence))
			require.JSONEq(t, call.arguments, trace.deltas.String())
			require.JSONEq(t, call.arguments, trace.arguments)
		}
	}
}

// requireProviderMatrixSettlement asserts assertion (5): the shared token-only
// settlement (25 quota held before dispatch, settled once on the user, token,
// request cost and consume log) plus exactly one consume log row, so nothing
// was charged twice. Parameters: t owns assertions and result is the outcome.
func requireProviderMatrixSettlement(t *testing.T, result providerMatrixResult) {
	t.Helper()
	requireFallbackTokenSettlement(t, result.c, result.requestID, providerMatrixBalance, result.got.User, result.got.Token)
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", result.requestID, model.LogTypeConsume).Find(&logs).Error)
	require.Len(t, logs, 1, "exactly one final consume row")
}
