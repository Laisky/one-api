# Laisky One API Sandbox

Laisky One API provides a free, unauthenticated sandbox for agent integration tests. Sandbox endpoints return static mock responses and never call upstream model providers.

## No-Key Sandbox Endpoints

- `POST https://oneapi.laisky.com/sandbox/v1/chat/completions`
- `POST https://oneapi.laisky.com/sandbox/v1/responses`
- `POST https://oneapi.laisky.com/sandbox/v1/messages`
- `GET https://oneapi.laisky.com/ask`
- `POST https://oneapi.laisky.com/.well-known/mcp`

Use the sandbox to verify request routing, response parsing, MCP discovery, and documentation flows before provisioning a relay API key for real model calls.

## Production Calls

Real provider calls require `Authorization: Bearer <relay-api-key>` and use `/v1/chat/completions`, `/v1/responses`, `/v1/messages`, or `/mcp`.
