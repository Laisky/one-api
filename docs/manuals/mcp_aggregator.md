---
title: MCP Manual
version: 1.1
last_updated: 2026-01-17
---

# MCP Manual

This manual describes Model Context Protocol (MCP) support in one-api. It covers core concepts, administrator configuration, and downstream usage patterns for MCP tools and built-in tools.

## Menu

- [MCP Manual](#mcp-manual)
  - [Menu](#menu)
  - [1) MCP Concepts, Functions, and Domain Knowledge](#1-mcp-concepts-functions-and-domain-knowledge)
    - [What MCP is](#what-mcp-is)
    - [MCP entities in one-api](#mcp-entities-in-one-api)
    - [Tool ownership model](#tool-ownership-model)
    - [Tool schema and parameter signature](#tool-schema-and-parameter-signature)
    - [Tool registry and routing flow](#tool-registry-and-routing-flow)
    - [Priority and retry behavior](#priority-and-retry-behavior)
    - [Billing and logging](#billing-and-logging)
    - [Policy layers and allow/deny logic](#policy-layers-and-allowdeny-logic)
    - [Security and data boundaries](#security-and-data-boundaries)
  - [2) Administrator Guide: MCP Settings](#2-administrator-guide-mcp-settings)
    - [MCP Server configuration fields](#mcp-server-configuration-fields)
    - [Configuration examples](#configuration-examples)
      - [Example A: bearer auth + whitelist + pricing overrides](#example-a-bearer-auth--whitelist--pricing-overrides)
      - [Example B: custom headers + blacklist](#example-b-custom-headers--blacklist)
      - [Example C: API key auth + JSON headers](#example-c-api-key-auth--json-headers)
      - [Missing pricing behavior](#missing-pricing-behavior)
    - [Sync and test operations](#sync-and-test-operations)
    - [Policy resolution summary](#policy-resolution-summary)
  - [3) Downstream User Guide: MCP and Built-in Tools](#3-downstream-user-guide-mcp-and-built-in-tools)
    - [Using MCP tools as built-ins](#using-mcp-tools-as-built-ins)
    - [Tool selection rules](#tool-selection-rules)
    - [Using the MCP proxy endpoint](#using-the-mcp-proxy-endpoint)
    - [Best practices](#best-practices)
    - [OpenAI Response API (cURL)](#openai-response-api-curl)
      - [Server-qualified tool name](#server-qualified-tool-name)
    - [Claude Messages API (cURL)](#claude-messages-api-curl)

## 1) MCP Concepts, Functions, and Domain Knowledge

### What MCP is

Model Context Protocol (MCP) defines a standardized way for AI models and clients to discover and invoke tools hosted by remote servers. In one-api, MCP is used to aggregate tools from multiple MCP servers and expose them as built-in tools that behave like upstream provider tools.

### MCP entities in one-api

- **MCP Server**: An admin-managed registry entry that points to a remote MCP endpoint. It includes auth, policy rules, pricing, and sync settings.
- **MCP Tool**: A tool definition synchronized from an MCP server. It includes the tool name, description, and input schema.
- **Tool catalog**: The merged list of tools from all enabled MCP servers, filtered by policy.

### Tool ownership model

one-api distinguishes tool ownership to ensure correct routing and billing:

- `user_local`: tools provided directly by the client in the request. one-api never executes these tools.
- `channel_builtin`: tools provided by upstream providers (for example, web search). These are validated and billed with existing channel policy.
- `oneapi_builtin`: tools sourced from MCP servers. These are executed by one-api via MCP.

### Tool schema and parameter signature

Each MCP tool has an input schema (JSON Schema). one-api computes a **parameter signature** by canonicalizing this schema with stable key ordering. This signature is used to disambiguate tools when multiple MCP servers expose the same tool name.

### Tool registry and routing flow

For every request, one-api builds an internal tool registry and routes tool calls based on ownership. The high-level flow is:

1. **Intake**: Parse tools from the request payload.
2. **Classification**: Split tools into `user_local`, `channel_builtin`, and `oneapi_builtin`.
3. **Pre-dispatch conversion**:

- Keep `channel_builtin` tools as upstream built-ins (when allowed by channel/provider tooling policy).
- Convert `oneapi_builtin` tools into local tool definitions so upstream models can call them as standard tools.
- Keep `user_local` tools as local tools (but one-api never executes them).

1. **Upstream call**: Send the normalized request to the selected channel.
1. **Tool call handling**:

- If the model requests a tool call, one-api resolves it in the registry.
- `oneapi_builtin` → one-api invokes the MCP server and returns tool results to the model.
- `user_local` → one-api passes the tool call back to the client (existing local tool flow).

1. **Multi-round loop**: Continue until the model completes or the tool round limit is reached.

This registry is preserved across retries, ensuring idempotency and consistent billing. When MCP tools are executed, one-api switches to a non-streaming tool loop to guarantee tool execution ordering.

### Priority and retry behavior

When multiple MCP servers provide the same tool name (and signature), one-api prefers the server with the highest priority. If a tool invocation fails, one-api retries the next lower-priority server that matches the same name and signature. This mirrors channel priority and retry behavior.

### Billing and logging

Tool usage is billed per call according to per-server pricing rules. The billing pipeline records per-tool usage and costs in the existing tool usage metadata. Logs include MCP tool entries with server identifiers and costs.

### Policy layers and allow/deny logic

Tool availability is determined by the intersection of multiple policy layers. A tool is **denied** if any layer blocks it:

1. MCP server whitelist/blacklist
2. Channel MCP blacklist
3. User MCP blacklist
4. Request `allowed_tools` constraints (if present)

If the whitelist is empty, no MCP tools from that server are available until explicitly listed.

### Security and data boundaries

- MCP server credentials are stored encrypted and only attached to outbound MCP requests.
- MCP tools are executed by one-api, not by end users or upstream channels.
- one-api never executes `user_local` tools; those are handled by the client application.
- Tool results are sent back to the upstream model using the standard tool-result format to preserve compatibility.

## 2) Administrator Guide: MCP Settings

### MCP Server configuration fields

- **Name**: Unique server label. Used as the server identifier in tool naming.
- **Description**: Optional metadata for operators.
- **Status**: Enabled or disabled. Disabled servers are not used for tool selection.
- **Priority**: Integer priority (default 0). Higher values are preferred when multiple servers match the same tool.
- **Base URL**: The MCP server endpoint URL. Must be HTTP or HTTPS.
- **Protocol**: Currently `streamable_http`. Other protocols may be added later.
- **Auth type**:
  - `none`: No auth header
  - `bearer`: Adds `Authorization: Bearer <token>`
  - `api_key`: Adds `x-api-key: <token>`
  - `custom_headers`: Adds the provided headers
- **API key**: The secret used for bearer or API key auth. Stored encrypted.
- **Headers**: Custom headers (JSON map). Added to each MCP request.
- **Tool whitelist**: Only tools explicitly listed are exposed. Empty whitelist means no tools are allowed.
- **Tool blacklist**: Tools listed here are never exposed.
- **Tool pricing**: Per-tool pricing map. `usd_per_call` and `quota_per_call` are supported.
- **Auto sync enabled**: Whether to periodically sync the tool catalog.
- **Auto sync interval**: Minutes between syncs (default 60, bounded 5–1440).

### Configuration examples

Below are common MCP server configurations. These examples match the settings page fields and show the exact JSON shapes expected by the API.

#### Example A: bearer auth + whitelist + pricing overrides

```json
{
  "name": "acme-tools",
  "description": "Acme MCP server",
  "status": 1,
  "priority": 10,
  "base_url": "https://mcp.acme.ai",
  "protocol": "streamable_http",
  "auth_type": "bearer",
  "api_key": "${ACME_MCP_TOKEN}",
  "headers": {},
  "tool_whitelist": ["weather.get", "news.search"],
  "tool_blacklist": [],
  "tool_pricing": {
    "weather.get": { "usd_per_call": 0.002 },
    "news.search": { "usd_per_call": 0.004, "quota_per_call": 40 }
  },
  "auto_sync_enabled": true,
  "auto_sync_interval_minutes": 60
}
```

#### Example B: custom headers + blacklist

```json
{
  "name": "internal-mcp",
  "status": 1,
  "priority": 0,
  "base_url": "https://mcp.internal.example.com",
  "protocol": "streamable_http",
  "auth_type": "custom_headers",
  "api_key": "",
  "headers": {
    "x-tenant": "prod",
    "x-auth": "${INTERNAL_MCP_SECRET}"
  },
  "tool_whitelist": [],
  "tool_blacklist": ["experimental.tool"],
  "tool_pricing": {},
  "auto_sync_enabled": false,
  "auto_sync_interval_minutes": 60
}
```

#### Example C: API key auth + JSON headers

```json
{
  "name": "partner-mcp",
  "status": 1,
  "priority": 5,
  "base_url": "https://mcp.partner.io",
  "protocol": "streamable_http",
  "auth_type": "api_key",
  "api_key": "${PARTNER_API_KEY}",
  "headers": {
    "x-region": "us-east-1"
  },
  "tool_whitelist": ["calendar.list"],
  "tool_blacklist": [],
  "tool_pricing": {
    "calendar.list": { "usd_per_call": 0.001 }
  },
  "auto_sync_enabled": true,
  "auto_sync_interval_minutes": 120
}
```

#### Missing pricing behavior

If a tool is listed in `tool_whitelist` but no pricing exists in the server pricing map, the tool is free by default. The UI should highlight this state (for example, “No price set → will be free”).

### Sync and test operations

- **Sync**: Pulls tool metadata from the MCP server and updates the local catalog.
- **Test**: Validates connectivity and lists tools to confirm availability.

### Policy resolution summary

one-api applies layered policy rules in this order:

1. Server whitelist/blacklist
2. Channel MCP blacklist
3. User MCP blacklist
4. Request `allowed_tools` constraints (if present)

If a tool is denied by any layer, it is unavailable.

## 3) Downstream User Guide: MCP and Built-in Tools

### Using MCP tools as built-ins

Downstream users can include MCP tools in their requests as built-in tools. one-api converts MCP built-ins into local tool definitions before dispatching requests upstream, then executes MCP calls when the model requests them.

MCP tools are declared using `type: "mcp"` with `server_label` and `server_url`. Use `allowed_tools` to explicitly list the tools you want the model to see from that MCP server.

For OpenAI Responses and Claude Messages, the tool `type` field is treated as the tool name for MCP matching. If a configured MCP tool has the same name, one-api will convert it to a local function tool before sending the request upstream.

Implicit aliasing is also supported:

- If a tool type matches a known MCP tool name, one-api converts it to an MCP tool **even when** the upstream built-in is allowed.
- If no MCP tool matches, the request stays as an upstream built-in.

Use explicit `type: "mcp"` or a server-qualified tool name (e.g., `acme-tools.weather.get`) to force a specific MCP server when multiple servers expose the same tool.

### Tool selection rules

When a tool call is issued, one-api resolves the tool with these rules:

1. Match tool name.
2. If multiple servers expose the same name, match the parameter signature (canonicalized input schema).
3. If multiple matches remain, select the highest-priority server.
4. If the call fails, retry lower-priority servers that match the same name and signature.

If the tool name is ambiguous and no signature is available, one-api returns a disambiguation error and asks for a server label or signature match.

### Using the MCP proxy endpoint

The `/mcp` endpoint exposes a Streamable HTTP MCP server backed by one-api’s configured MCP tools. Downstream MCP clients can list tools and invoke them via `/mcp`. Tool calls should use the fully qualified name (`server_label.tool_name`) when possible to avoid ambiguity.

### Best practices

- Prefer explicit tool definitions with complete input schemas so one-api can compute signatures accurately.
- Use server-qualified tool names when you need a specific server.
- Keep tool parameters consistent with the published schema to avoid validation errors upstream.
- Review logs for tool usage and costs to confirm billing behavior.
- Expect streaming responses to be downgraded to non-streaming when MCP tools are executed.

### OpenAI Response API (cURL)

The following example calls one-api using the Responses API format and exposes MCP tools as built-ins. The model can choose whether to call the tools.

```bash
curl "https://oneapi.laisky.com/v1/responses" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ONEAPI_TOKEN" \
  -d '{
    "model": "gpt-5",
    "input": "Find the weather in Paris and summarize it.",
    "tools": [
      {
        "type": "mcp",
        "server_label": "acme-tools",
        "server_url": "https://mcp.acme.ai",
        "allowed_tools": ["weather.get"]
      }
    ],
    "tool_choice": "auto"
  }'
```

#### Server-qualified tool name

If multiple MCP servers expose the same tool name, qualify the tool name in your tool call as `server_label.tool_name` when available. This avoids ambiguity and ensures the correct MCP server is selected.

### Claude Messages API (cURL)

This example uses the Claude Messages API format and includes the same MCP tool declaration. one-api will normalize and route the tool call as needed.

```bash
curl "https://oneapi.laisky.com/v1/messages" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $ONEAPI_TOKEN" \
  -d '{
    "model": "claude-sonnet-4-5",
    "max_tokens": 512,
    "messages": [
      {
        "role": "user",
        "content": "Search news about renewable energy and give a short summary."
      }
    ],
    "tools": [
      {
        "type": "mcp",
        "server_label": "acme-tools",
        "server_url": "https://mcp.acme.ai",
        "allowed_tools": ["news.search"]
      }
    ]
  }'
```
