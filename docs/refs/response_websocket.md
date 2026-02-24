# WebSocket Mode

> https://developers.openai.com/api/docs/guides/websocket-mode/

The Responses API supports a WebSocket mode for long-running, tool-call-heavy workflows. In this mode, you keep a persistent connection to `/v1/responses` and continue each turn by sending only new input items plus `previous_response_id`.

WebSocket mode is compatible with both Zero Data Retention (ZDR) and `store=false`.

## Why use WebSocket mode

WebSocket mode is most useful when a workflow involves many model-tool round trips (for example, agentic coding or orchestration loops with repeated tool calls).

Because the connection stays open and each turn sends only incremental input, WebSocket mode reduces per-turn continuation overhead and improves end-to-end latency across long chains. For rollouts with 20+ tool calls, we have seen up to roughly 40% faster end-to-end execution.

## Connect and create responses

In WebSocket mode, start each turn by sending a `response.create` event from the client. The payload mirrors the normal [Responses create body](https://developers.openai.com/api/reference/resources/responses/methods/create), except that transport-specific fields like `stream` and `background` are not used.

```python
from websocket import create_connection
import json
import os

ws = create_connection(
    "wss://api.openai.com/v1/responses",
    header=[
        f"Authorization: Bearer {os.environ['OPENAI_API_KEY']}",
    ],
)

ws.send(
    json.dumps(
        {
            "type": "response.create",
            "model": "gpt-5.2",
            "store": False,
            "input": [
                {
                    "type": "message",
                    "role": "user",
                    "content": [{"type": "input_text", "text": "Find fizz_buzz()"}],
                }
            ],
            "tools": [],
        }
    )
)
```

Clients can optionally warm up request state by sending `response.create` with `generate: false`. This is useful when you already know the tools, instructions, and/or custom messages you plan to send with an upcoming turn. `generate: false` does not return a model output, but prepares request state so the next generated turn can start faster. The warmup request returns a response ID that you can chain from with `previous_response_id`, including on later turns in a response chain. The next section explains how to continue a session using `previous_response_id` and incremental inputs.

## Continue with incremental inputs

To continue a run, send another `response.create` with:

- `previous_response_id` set to the prior response ID.
- `input` containing only new items (for example, tool outputs and the next user message).

```python
ws.send(
    json.dumps(
        {
            "type": "response.create",
            "model": "gpt-5.2",
            "store": False,
            "previous_response_id": "resp_123",
            "input": [
                {
                    "type": "function_call_output",
                    "call_id": "call_123",
                    "output": "tool result",
                },
                {
                    "type": "message",
                    "role": "user",
                    "content": [{"type": "input_text", "text": "Now optimize it."}],
                },
            ],
            "tools": [],
        }
    )
)
```

## How continuation works

WebSocket mode uses the same `previous_response_id` chaining semantics as HTTP mode, but it adds a lower-latency continuation path on the active socket.

On an active WebSocket connection, the service keeps one previous-response state in a connection-local in-memory cache (the most recent response). Continuing from that most recent response is fast because the service can reuse connection-local state. Because the previous-response state is retained only in memory and is not written to disk, you can use WebSocket mode in a way that is compatible with `store=false` and Zero Data Retention (ZDR).

If a `previous_response_id` is not in the in-memory cache, behavior depends on whether you store responses:

- With `store=true`, the service may hydrate older response IDs from persisted state when available. Continuation can still work, but it usually loses the in-memory latency benefit.
- With `store=false` (including ZDR), there is no persisted fallback. If the ID is uncached, the request returns `previous_response_not_found`.

If a turn fails (`4xx` or `5xx`), the service evicts the referenced `previous_response_id` from the connection-local cache. This prevents reusing stale cached state for that failed continuation.

## Compaction and creating new responses

If you are using compaction, there are two different continuation patterns:

### Server-side compaction (`context_management`)

When you enable server-side compaction (`context_management` with `compact_threshold`), compaction happens during normal `/responses` generation. In WebSocket mode, you continue the same way you normally do: send the next `response.create` with the latest `previous_response_id` and only new input items.

### Standalone `/responses/compact`

The standalone [`/responses/compact` endpoint](https://developers.openai.com/api/docs/api-reference/responses/compact) returns a new compacted input window, not a response ID. After compaction, create a new response on your WebSocket connection using the compacted window as `input` (plus the next user/tool items).

Start a new chain by omitting `previous_response_id` or setting it to `null`. Pass the compacted output as-is; do not prune the returned window.

```python
# Compact your current window (HTTP call)
compacted = client.responses.compact(
    model="gpt-5.2",
    input=long_input_items_array,
)

# Start a new response on the WebSocket using the compacted window
ws.send(
    json.dumps(
        {
            "type": "response.create",
            "model": "gpt-5.2",
            "store": False,
            "input": [
                *compacted.output,
                {
                    "type": "message",
                    "role": "user",
                    "content": [{"type": "input_text", "text": "Continue from here."}],
                },
            ],
            "tools": [],
        }
    )
)
```

## Connection behavior and limits

- Server events and ordering match the existing Responses streaming event model.
- A single WebSocket connection can receive multiple `response.create` messages, but it runs them sequentially (one in-flight response at a time).
- No multiplexing support today. Use multiple connections if you need parallel runs.
- Connection duration is limited to 60 minutes. Reconnect when the limit is reached.

## Reconnect and recover

When a connection closes (or hits the 60-minute limit), open a new WebSocket connection and continue with one of these patterns:

1. If your prior response is persisted (`store=true`) and you have a valid response ID, continue with `previous_response_id` and new input items.
2. If you cannot continue the chain (for example, `store=false`/ZDR or `previous_response_not_found`), start a new response by setting `previous_response_id` to `null` (or omitting it) and send the full input context for the next turn.
3. If you compacted context with `/responses/compact`, use the returned compacted window as the base `input` for that new response, then append the latest user/tool items.

## Errors to handle

`previous_response_not_found`

```json
{
  "type": "error",
  "status": 400,
  "error": {
    "code": "previous_response_not_found",
    "message": "Previous response with id 'resp_abc' not found.",
    "param": "previous_response_id"
  }
}
```

`websocket_connection_limit_reached`

```json
{
  "type": "error",
  "error": {
    "type": "invalid_request_error",
    "code": "websocket_connection_limit_reached",
    "message": "Responses websocket connection limit reached (60 minutes). Create a new websocket connection to continue."
  },
  "status": 400
}
```

## Related guides

- [Conversation state](https://developers.openai.com/api/docs/guides/conversation-state)
- [Streaming API responses](https://developers.openai.com/api/docs/guides/streaming-responses)
- [Responses streaming events reference](https://developers.openai.com/api/docs/api-reference/responses-streaming)
