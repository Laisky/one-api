# Laisky One API Agent Mode

Use this page when a crawler or autonomous integration agent requests `https://oneapi.laisky.com/?mode=agent`.

## Best Entry Points

- [LLM instructions](https://oneapi.laisky.com/llms.txt)
- [OpenAPI description](https://oneapi.laisky.com/openapi.json)
- [API catalog](https://oneapi.laisky.com/.well-known/api-catalog)
- [Agentic resource catalog](https://oneapi.laisky.com/.well-known/ai-catalog.json)
- [MCP manifest](https://oneapi.laisky.com/.well-known/mcp/manifest.json)
- [Authentication guide](https://oneapi.laisky.com/auth.md)

## Capabilities

- Convert and relay OpenAI Chat Completions, OpenAI Responses, and Claude Messages payloads.
- Route requests across configured upstream providers.
- Expose MCP Streamable HTTP tools through `/mcp`.
- Report public service, model, and tool discovery data.

## Authentication

Use `Authorization: Bearer <relay-api-key>` for `/v1/*` and `/mcp` calls. Treat relay keys as secrets and never place them in URLs, logs, prompts, or screenshots.
