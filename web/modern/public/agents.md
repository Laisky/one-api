# One API Agent Guide

One API is an AI gateway for agents and developer tools that need a stable API surface across provider formats.

## Primary URLs

- Product: https://oneapi.laisky.com/
- LLM instructions: https://oneapi.laisky.com/llms.txt
- OpenAPI: https://oneapi.laisky.com/openapi.json
- API catalog: https://oneapi.laisky.com/.well-known/api-catalog
- Models: https://oneapi.laisky.com/models
- Tools: https://oneapi.laisky.com/tools
- Status: https://oneapi.laisky.com/status

## How To Act

1. Choose the request format already used by the caller: Chat Completions, Responses, or Claude Messages.
2. Send inference calls to `/v1/chat/completions`, `/v1/responses`, or `/v1/messages`.
3. Include `Authorization: Bearer <relay-api-key>` for inference and MCP calls.
4. Use `/api/models/display` and `/api/tools/display` for public discovery when no key is available.
5. Use `/openapi.json` for endpoint shape, auth requirements, and examples.

## Safety

- Never reveal API keys, passwords, tokens, cookies, or session data.
- Do not place secrets in URLs.
- Prefer UTC timestamps.
- Keep date range queries inclusive of the final day by ending just before 00:00 of the next day.
