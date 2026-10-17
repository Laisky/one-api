# Client-executed Responses tool search

Native Responses routes can forward an explicit client-executed discovery tool:

```json
{
  "type": "tool_search",
  "execution": "client",
  "description": "Find local tools",
  "parameters": {
    "type": "object",
    "properties": {"query": {"type": "string"}},
    "required": ["query"],
    "additionalProperties": false
  }
}
```

The application executes discovery. The model emits a `tool_search_call` with
object-valued `arguments`, `execution: "client"`, and a `call_id`. Return a
`tool_search_output` carrying the same `call_id`, explicit client execution and
its `tools` array. OneAPI preserves call IDs, object arguments, namespace identity,
custom grammars, unknown catalog metadata and exact numeric literals through
native JSON/SSE forwarding and discovery state serialization. Ordinary function
arguments remain JSON strings; existing MCP representations are unchanged.

Loaded definitions may be named local `function` or `custom` tools, or a named
`namespace` containing those two kinds. Empty discovery results are valid.
Each catalog and namespace is bounded to 1,024 definitions; nested namespaces are
unsupported. A developer `additional_tools` input item can supply the same local
catalog and an explicit client-executed search definition at its root. Catalog
position in the conversation is retained. Noncanonical or duplicate reviewed
JSON keys are rejected before map decoding can hide a different tool definition.

These history catalogs cannot introduce hosted web/file/image/code tools,
remote MCP definitions, unknown paid tools or server-executed search. Missing
`execution` uses hosted API semantics and is not admitted as client execution.
Existing top-level metered web-search support and its policy remain unchanged.
No new hosted search billing path or free hosted execution is enabled. Local
search uses ordinary provider token settlement, including the existing usage
and cache accounting; there is no new tool-call surcharge. Exact provider prompt
estimation is not introduced by this change.

Chat/Claude fallback rejects discovery definitions or history explicitly with
HTTP 400/`tool_search_not_supported`, before caller-input state hydration and
quota reservation. Hydrated discovery history is checked again before fallback
execution. This avoids silently dropping a catalog or changing tool ownership.
A gateway MCP alias named `tool_search` cannot intercept provider discovery.
Native unsupported billing shapes return 400/`tool_billing_not_supported`;
malformed or ambiguous JSON can return 400/`invalid_response_api_request` earlier.

Regression coverage lives in `responseapi_tool_search_test.go`,
`response_client_tool_search_wire_test.go`, and
`response_client_tool_search_http_test.go`. Tests use loopback providers and
isolated test ledgers. They do not establish external provider support or actual
Codex acceptance. See [the versioned compatibility audit](codex_responses_compatibility.md)
for the current evidence and remaining capability gaps.

No database migration, saved-pricing rewrite, configuration migration or live
billing mutation is required. Release only after exact-head CI and a separately
approved production acceptance. Roll back the image through the existing
configuration-preserving deployment procedure; rolling back reintroduces the
prior client-search rejection and does not undo legitimate settled token usage.

Protocol sources: [OpenAI tool search](https://developers.openai.com/api/docs/guides/tools-tool-search)
and [Codex 0.163.0 client-search constructor](https://github.com/openai/codex/blob/83086188ae0614bd882818ff1aa23f3194962012/codex-rs/core/src/tools/handlers/tool_search_spec.rs#L97).
