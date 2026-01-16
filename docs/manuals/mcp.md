---
title: MCP Manual
version: 1.0
last_updated: 2026-01-16
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
    - [Priority and retry behavior](#priority-and-retry-behavior)
    - [Billing and logging](#billing-and-logging)
  - [2) Administrator Guide: MCP Settings](#2-administrator-guide-mcp-settings)
    - [MCP Server configuration fields](#mcp-server-configuration-fields)
    - [Sync and test operations](#sync-and-test-operations)
    - [Policy resolution summary](#policy-resolution-summary)
  - [3) Downstream User Guide: MCP and Built-in Tools](#3-downstream-user-guide-mcp-and-built-in-tools)
    - [Using MCP tools as built-ins](#using-mcp-tools-as-built-ins)
    - [Tool selection rules](#tool-selection-rules)
    - [Using the MCP proxy endpoint](#using-the-mcp-proxy-endpoint)
    - [Best practices](#best-practices)

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

### Priority and retry behavior

When multiple MCP servers provide the same tool name (and signature), one-api prefers the server with the highest priority. If a tool invocation fails, one-api retries the next lower-priority server that matches the same name and signature. This mirrors channel priority and retry behavior.

### Billing and logging

Tool usage is billed per call according to per-server pricing rules. The billing pipeline records per-tool usage and costs in the existing tool usage metadata. Logs include MCP tool entries with server identifiers and costs.

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
