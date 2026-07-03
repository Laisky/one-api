# One API Developers

Developers and agents can integrate with One API using OpenAI-compatible, Claude-compatible, and MCP-compatible clients.

## Quick Start

1. Create or obtain a relay API key from the web UI.
2. Choose `/v1/chat/completions`, `/v1/responses`, or `/v1/messages`.
3. Send `Authorization: Bearer <relay-api-key>`.
4. Use `/v1/models` to inspect models available to that key.

## Machine-Readable Files

- `/openapi.json`
- `/llms.txt`
- `/.well-known/api-catalog`
- `/.well-known/agent-card.json`
