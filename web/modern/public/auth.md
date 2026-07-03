# One API Authentication

## Relay API Keys

Inference endpoints under `/v1/*` and the MCP endpoint `/mcp` require a relay API key:

```http
Authorization: Bearer <relay-api-key>
```

Relay keys are distinct from web management access tokens. Use relay keys for model and MCP calls only.

## Management Authentication

Management endpoints under `/api` use either a browser session or a user management access token. Some endpoints require user, admin, or root privileges.

## Agent Rules

- Never put API keys in URLs.
- Never reveal API keys in logs, prompts, screenshots, or exported traces.
- Rotate keys if they are exposed.
- Use separate keys per agent or automation when possible.
