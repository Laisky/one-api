---
title: MCP 2026-07-28 Protocol Compatibility
version: 1.0
last_updated: 2026-08-30
---

# MCP 2026-07-28 Protocol Compatibility

one-api supports the stable MCP `2026-07-28` protocol on both sides of the gateway while retaining the existing initialization-based Streamable HTTP lifecycle for legacy clients and upstream servers.

## Modern server behavior

The authenticated `/mcp` endpoint accepts modern requests without an `initialize` exchange.

Every modern request must include these values in `params._meta`:

- `io.modelcontextprotocol/protocolVersion`;
- `io.modelcontextprotocol/clientCapabilities`;
- `io.modelcontextprotocol/clientInfo` should also identify the client.

Every HTTP POST must also include:

- `MCP-Protocol-Version`, matching `io.modelcontextprotocol/protocolVersion`;
- `Mcp-Method`, matching the JSON-RPC method;
- `Mcp-Name` for `tools/call`, matching `params.name` after protocol decoding;
- any schema-driven `Mcp-Param-*` headers declared through `x-mcp-header`.

`server/discover` reports supported protocol versions, capabilities, cache metadata, and server identity. Successful results include `resultType` and `params._meta["io.modelcontextprotocol/serverInfo"]`. `tools/list` responses use deterministic ordering, a private cache scope, and a cache TTL.

The HTTP endpoint validates `Origin` whenever it is present. The Origin host must match the MCP endpoint host, preventing DNS-rebinding access from an unrelated browser origin.

## Modern client behavior

The production synchronization and tool-call paths send `2026-07-28` requests directly. They do not initialize a protocol session before a modern request.

The client:

1. attaches namespaced modern `_meta` fields to every request;
2. mirrors the protocol method and tool name into HTTP headers;
3. derives `Mcp-Param-*` values from statically reachable `x-mcp-header` annotations;
4. supports JSON and Server-Sent Events responses;
5. accepts `resultType`, `structuredContent`, `isError`, `inputRequests`, and `requestState`;
6. preserves `inputResponses` and `requestState` when retrying a multi-round-trip tool call;
7. excludes malformed `x-mcp-header` tool definitions without hiding valid tools;
8. retries through the legacy initialize/session lifecycle only when the remote endpoint does not return a recognized modern protocol error.

Authentication failures and recognized modern errors such as `HeaderMismatch`, `MissingRequiredClientCapability`, and `UnsupportedProtocolVersion` are returned to the caller rather than being misclassified as legacy-server failures.

## Schema-driven parameter headers

An input-schema property may define an `x-mcp-header` annotation when its type is `string`, `integer`, or `boolean`.

```json
{
  "type": "object",
  "properties": {
    "tenant": {
      "type": "string",
      "x-mcp-header": "Tenant-ID"
    }
  }
}
```

For `{"tenant":"acme"}`, the client sends:

```text
Mcp-Param-Tenant-ID: acme
```

Header annotations must be unique case-insensitively and reachable from the schema root through `properties` only. Annotated integers must remain within the JavaScript-safe integer range. A missing or `null` parameter produces no header.

Values that are not safe plain HTTP field values, or that already resemble the sentinel, use this exact encoding:

```text
=?base64?<standard-base64-encoded-UTF-8>?=
```

The same encoding applies to `Mcp-Name`. The server decodes mirrored values, independently derives the expected parameter values from the JSON body, and rejects missing, repeated, malformed, or mismatched headers with error code `-32020`.

## Legacy compatibility

Requests without modern protocol metadata continue through the original handler. Existing legacy behavior remains available, including:

- `initialize` and `notifications/initialized`;
- `Mcp-Session-Id`;
- the existing `2025-06-18` Streamable HTTP compatibility path;
- legacy tool-result aliases such as `is_error` and `structured_content`.

The original `ListTools` and `CallTool` methods remain available for code that deliberately requires the legacy lifecycle. one-api's active synchronization and proxy execution paths use the modern-first methods.

## Error and status behavior

Modern transport-level validation uses HTTP status codes in addition to JSON-RPC errors:

| Condition | HTTP status | JSON-RPC code |
| --- | ---: | ---: |
| Header/body mismatch | 400 | `-32020` |
| Missing required client capability | 400 | `-32021` |
| Unsupported protocol version | 400 | `-32022` |
| Invalid Origin | 403 | `-32600` |
| Unknown modern method | 404 | `-32601` |

Tool execution failures that occur after a valid request remain JSON-RPC errors with HTTP 200, preserving normal RPC semantics.

## Validation coverage

Regression tests cover:

- handshake-free modern tool listing;
- modern-to-legacy client fallback;
- namespaced request and result metadata;
- protocol, method, encoded tool-name, and schema-driven parameter headers;
- nested extraction, null omission, safe-integer enforcement, and exact Base64 sentinel encoding;
- exclusion of invalid tool header schemas;
- modern and legacy result-field aliases;
- multi-round-trip request fields;
- `server/discover`, cache metadata, Origin validation, and header mismatch rejection;
- legacy `initialize` delegation through the same `/mcp` endpoint.

## Specification references

- MCP changelog: <https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/docs/specification/2026-07-28/changelog.mdx>
- Versioning: <https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/docs/specification/2026-07-28/basic/versioning.mdx>
- Streamable HTTP: <https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/docs/specification/2026-07-28/basic/transports/streamable-http.mdx>
- Server discovery: <https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/docs/specification/2026-07-28/server/discover.mdx>
- Tools: <https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/docs/specification/2026-07-28/server/tools.mdx>
