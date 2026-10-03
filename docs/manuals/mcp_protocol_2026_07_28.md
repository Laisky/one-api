---
title: MCP 2026-07-28 Protocol Compatibility
version: 1.2
last_updated: 2026-09-29
---

# MCP 2026-07-28 Protocol Compatibility

one-api supports the stable MCP `2026-07-28` protocol on both sides of the gateway while retaining the existing initialization-based Streamable HTTP lifecycle for legacy clients and upstream servers.

## Native implementation release 1.2.0

The latest stable protocol was rechecked on **2026-09-29** and remains
`2026-07-28`. Both the native client and gateway now identify their implementation
as `1.2.0`. This project implements MCP directly; it does not depend on `mcp-go`
or the official Go SDK, so there is no unrelated SDK dependency bump.

This is a **tools gateway**, not an implementation of every optional MCP feature.
It serves `server/discover`, `tools/list`, and `tools/call`. It does not advertise
resources, prompts, subscriptions/list-change delivery, task lifecycle methods,
Apps, Skills, OAuth discovery, or a full JSON Schema validator. Schemas and
extension descriptor fields are preserved; existing best-effort routing checks
are not a claim of full JSON Schema 2020-12 validation. External schema references
are never automatically fetched.

### Per-request capabilities and multi-round-trip interactions

The gateway forwards declared `elicitation` (form/URL), `sampling`, and `roots`
capabilities for request-scoped MRTR interactions. It neither performs these
interactions nor approves them: the caller must present the appropriate UI,
obtain user consent, and explicitly resend `inputResponses` and `requestState`
with the same qualified tool name. URL elicitation links are never fetched or
opened automatically by one-api. Capabilities for unimplemented extensions are
not negotiated upstream. Sampling and roots remain compatibility features;
new callers should prefer the non-deprecated protocol features.

Caller metadata, including trace context and opaque vendor fields, is forwarded
per request. The outbound protocol version and client implementation identity
belong to one-api. Metadata is not an authenticated user identity and cannot
override the gateway's user, blacklist, catalog, or configured upstream credentials.
No capability or continuation state is saved on a shared client. `input_required`
results are not billed or audited as completed tool calls; the final successful
result uses the existing accounting path.

### Live progress and cancellation

A caller can include `_meta.progressToken` and accept `text/event-stream` to
receive live `notifications/progress` from the selected upstream. Only matching
request tokens are forwarded. Unrelated notifications, logging notifications,
and subscription events are not delivered by this tools-only stream.
JSON-only callers still receive a final JSON response. The first progress event
lazily starts SSE; the final result or error then uses the same SSE encoding.
`X-Accel-Buffering: no` disables common reverse-proxy buffering.

The client incrementally parses LF, CRLF, and CR frames, supports multiline data
and comments, and returns as soon as the correlated final envelope arrives,
without waiting for EOF. The existing **32 MiB total response limit** also bounds
stream consumption. Callback errors and cancellation close the upstream response
and stop automatic tool replay. Modern HTTP streams are not resumable.

### Exact JSON and retry safety

Large integer IDs, arguments, schemas, structured results, MRTR inputs, and opaque
metadata are decoded without float64 rounding. Explicit `structuredContent: null`
is preserved. Fractional request IDs and unknown result variants are rejected;
a missing `resultType` still means `complete` for backward compatibility.
Header-mirrored integers retain their separate JavaScript-safe bounds.

Read-only catalog discovery retains broad legacy detection. A modern **tool call**
is replayed through legacy initialization only after an explicit initialization,
session, or supported-version rejection and only when its request has no modern-only
fields. Ambiguous HTTP/transport errors, malformed results, and failed streams are
not automatically replayed against another upstream: their side effects are unknown.
This is deliberately more conservative than legacy catalog discovery. Callers
connecting to an opaque legacy endpoint can discover its era with `ListToolsLatest`
first and reuse that client, or explicitly use `Initialize`/`CallTool`.

Diagnostic logs retain method, endpoint, sanitized headers, status and body size,
but omit request/response payloads so arbitrary MRTR responses and continuation
state cannot appear in debug logs.

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

`server/discover` reports supported protocol versions, capabilities, cache metadata, and server identity. Successful results include `resultType` and `result._meta["io.modelcontextprotocol/serverInfo"]`. `tools/list` responses use deterministic ordering, a private cache scope, and a cache TTL.

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
8. retains legacy discovery and explicit initialization compatibility while applying the conservative tool-call replay policy above.

Authentication failures and recognized modern errors such as `HeaderMismatch`, `MissingRequiredClientCapability`, and `UnsupportedProtocolVersion` are returned to the caller rather than being misclassified as legacy-server failures.

## Credential transport policy

Configured remote MCP endpoints that receive an API key, an authorization or cookie header, custom authentication headers, or URL user information must use HTTPS. The same rule is enforced both when server configuration is validated and immediately before outbound network I/O, so previously persisted or directly constructed clients cannot bypass it.

Plaintext HTTP remains available for unauthenticated endpoints. Credentialed HTTP is allowed only for `localhost`, `127.0.0.0/8`, and `::1` loopback endpoints used by local development and integration tests. The exception is host-bound: a credentialed redirect must remain on the exact original origin. HTTPS-to-HTTP redirects are always rejected, and credentialed redirects must preserve the original scheme, hostname, and effective port.

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
- `2025-11-25`, `2025-06-18`, and `2025-03-26` Streamable HTTP compatibility;
- legacy tool-result aliases such as `is_error` and `structured_content`.

The original `ListTools` and `CallTool` methods remain available for code that deliberately requires the legacy lifecycle. one-api's active synchronization and proxy execution paths use the modern-first methods.

## Error and status behavior

Modern transport-level validation uses HTTP status codes in addition to JSON-RPC errors:

| Condition | HTTP status | JSON-RPC code |
| --- | ---: | ---: |
| Missing required request metadata | 400 | `-32602` |
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
- legacy `initialize` delegation through the same `/mcp` endpoint;
- configuration-time and runtime rejection of credentialed remote plaintext HTTP;
- loopback-only credentialed HTTP compatibility and redirect downgrade protection.

The 2026-09-29 regressions additionally exercise real HTTP gateway/client/upstream
round trips with SQLite-backed policy and accounting, capability propagation,
URL elicitation continuation, live progress that gates the upstream's final
response, downstream cancellation, truncated-stream terminal errors, concurrent
metadata isolation, exact-number boundaries, malformed results, and empty catalogs.
Run the affected packages with:

```sh
go test -race -count=1 ./relay/mcp ./controller
go vet ./...
```

## Specification references

- Current stable protocol: <https://modelcontextprotocol.io/specification/latest>
- Per-request metadata and JSON results: <https://modelcontextprotocol.io/specification/2026-07-28/basic>
- MRTR: <https://modelcontextprotocol.io/specification/2026-07-28/basic/patterns/mrtr>
- Elicitation: <https://modelcontextprotocol.io/specification/2026-07-28/client/elicitation>


- MCP changelog: <https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/docs/specification/2026-07-28/changelog.mdx>
- Versioning: <https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/docs/specification/2026-07-28/basic/versioning.mdx>
- Streamable HTTP: <https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/docs/specification/2026-07-28/basic/transports/streamable-http.mdx>
- Server discovery: <https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/docs/specification/2026-07-28/server/discover.mdx>
- Tools: <https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/docs/specification/2026-07-28/server/tools.mdx>
