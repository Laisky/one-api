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

## OAuth Protected Resource Metadata {#oauth-protected-resource}

Protected-resource metadata is available at:

- [/.well-known/oauth-protected-resource](https://oneapi.laisky.com/.well-known/oauth-protected-resource)

The metadata identifies bearer-token usage for relay API keys and the scopes that map to model, message, response, and MCP access.

## OAuth Authorization Server Metadata {#oauth-authorization-server}

Authorization-server metadata is available at:

- [/.well-known/oauth-authorization-server](https://oneapi.laisky.com/.well-known/oauth-authorization-server)

Laisky One API does not currently expose dynamic agent registration endpoints. There is no `register_uri`, `claim_uri`, or `revocation_uri` for autonomous client onboarding. Operators provision relay API keys through the web management UI.

## Agent Auth {#agent_auth}

Agents should authenticate relay calls with `Authorization: Bearer <relay-api-key>`. Relay API keys are scoped by the operator-configured token, group, channel, model, and quota settings.
