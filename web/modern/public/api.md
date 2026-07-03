# One API API Reference

Base URL: `https://oneapi.laisky.com`

## Inference Endpoints

| Method | Path | Authentication | Purpose |
| --- | --- | --- | --- |
| `POST` | `/v1/chat/completions` | Relay API key | OpenAI Chat Completions-compatible generation. |
| `POST` | `/v1/responses` | Relay API key | OpenAI Responses-compatible generation. |
| `GET` | `/v1/responses/{response_id}` | Relay API key | Retrieve a stored Responses API response when supported. |
| `DELETE` | `/v1/responses/{response_id}` | Relay API key | Delete a Responses API response when supported. |
| `POST` | `/v1/messages` | Relay API key | Claude Messages-compatible generation. |
| `GET` | `/v1/models` | Relay API key | List models available to the relay key. |
| `POST` | `/v1/embeddings` | Relay API key | Create embedding vectors. |
| `POST` | `/v1/rerank` | Relay API key | Rerank documents. |
| `POST` | `/v1/images/generations` | Relay API key | Generate images. |
| `POST` | `/v1/audio/transcriptions` | Relay API key | Transcribe audio. |
| `GET` | `/v1/realtime` | Relay API key | Realtime websocket endpoint. |
| `POST` | `/mcp` | Relay API key | MCP Streamable HTTP JSON-RPC endpoint. |

## Public Discovery

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/status` | Public service status. |
| `GET` | `/api/models/display` | Public model catalog for UI and agents. |
| `GET` | `/api/tools/display` | Public MCP tool catalog for UI and agents. |
| `GET` | `/openapi.json` | OpenAPI description. |
| `GET` | `/.well-known/api-catalog` | API catalog linkset. |
| `GET` | `/.well-known/mcp/manifest.json` | MCP proxy manifest for agent and tool clients. |
| `GET` | `/.well-known/mcp/server-card.json` | MCP proxy server card. |

See `/openapi.json` for machine-readable schemas and examples.
