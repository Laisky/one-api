# One API

One API is an agent-friendly AI gateway for OpenAI Chat Completions, OpenAI Responses, Claude Messages, MCP tools, and multi-provider model routing.

## What It Does

- Accepts Chat Completions requests at `/v1/chat/completions`.
- Accepts Responses requests at `/v1/responses`.
- Accepts Claude Messages requests at `/v1/messages`.
- Converts compatible request and response formats across configured upstream providers.
- Provides MCP Streamable HTTP proxy access at `/mcp`.
- Exposes public model and tool discovery through `/api/models/display` and `/api/tools/display`.
- Tracks usage, quota, billing, logs, and channel health for operators.

## Agent Resources

- `/llms.txt`
- `/agents.md`
- `/openapi.json`
- `/.well-known/api-catalog`
- `/.well-known/agent-card.json`
- `/.well-known/agent-skills/index.json`
