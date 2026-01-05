# GitHub Copilot Channel (Architecture + Ops)

This document describes how One-API integrates **GitHub Copilot** as a first-class channel type.

Copilot is not a standard OpenAI API key provider. Instead:

1. You provide a **GitHub access token** (PAT or OAuth token) as the channel key.
2. One-API exchanges it for a **short-lived Copilot API token** via GitHub.
3. One-API forwards OpenAI-style requests to `https://api.githubcopilot.com`.

## Channel Type

-   Channel Type ID: **53**
-   Name: **copilot**
-   Default Base URL: `https://api.githubcopilot.com`

## Credentials

### What to put in “Channel Key”

Set the channel key to a **GitHub access token** that has an active Copilot subscription entitlement.

Typical options:

-   **GitHub PAT (classic)** or fine-grained token (recommended for servers)
-   **GitHub OAuth device-flow access token** (good for manual setup / lab use)

One-API uses this GitHub token only to fetch the short-lived Copilot API token.

### Token Exchange Flow

For each Copilot channel, One-API calls:

-   `GET https://api.github.com/copilot_internal/v2/token`

with:

-   `Authorization: token <GITHUB_ACCESS_TOKEN>`

GitHub responds with a Copilot API token (typically valid for ~25–30 minutes). One-API caches the token per channel and refreshes it when it is close to expiring.

## Supported Endpoints

One-API accepts the normal OpenAI-style routes and rewrites to Copilot’s upstream paths:

-   `/v1/chat/completions` → `/chat/completions`
-   `/v1/embeddings` → `/embeddings`
-   `/v1/models` → `/models`
-   `/v1/responses` → `/v1/responses` (passed through)

Notes:

-   Image/audio/video endpoints are not implemented for Copilot in One-API.
-   Claude Messages API is not supported by the Copilot adaptor.

## Required Headers

Copilot upstreams often require editor-identifying headers.

The adaptor injects defaults when missing:

-   `editor-version` (default: `vscode/1.85.1`)
-   `editor-plugin-version` (default: `copilot/1.0.0`)
-   `Copilot-Integration-Id` (default: `vscode-chat`)
-   `User-Agent` (default: `GithubCopilot/1.0.0`)

### Overriding headers

If you need to override these, you can set them on the incoming request. One-API will forward them.

For convenience (and to avoid clobbering client headers), the adaptor also accepts the `X-` prefixed variants:

-   `X-editor-version`
-   `X-editor-plugin-version`
-   `X-Copilot-Integration-Id`
-   `X-User-Agent`

## Model Names and Mapping

Copilot’s available models can vary by account and GitHub feature flags.

You can:

-   Use One-API’s existing **model mapping** feature on the channel to translate your internal model names into Copilot’s upstream model IDs.
-   Leave mapping empty and send upstream model names directly.

## Troubleshooting

### 401 / 403 from token exchange

Symptoms:

-   Errors mentioning `copilot_internal/v2/token`

Common causes:

-   GitHub access token is invalid/expired/revoked.
-   The GitHub account does not have an active Copilot entitlement.

Fix:

-   Regenerate a GitHub token and update the channel key.

### 401 from `api.githubcopilot.com`

Symptoms:

-   Token exchange succeeds, but upstream requests fail.

Common causes:

-   Copilot API token expired and cache wasn’t refreshed (should auto-refresh).
-   Missing required editor headers.

Fix:

-   Retry after a minute.
-   Explicitly pass the required headers (or `X-...` variants).

### 404 upstream

Likely the upstream path is different for your Copilot account or request type.

Fix:

-   Verify you are calling supported endpoints.
-   Confirm the channel base URL is `https://api.githubcopilot.com`.

### 429 throttling

GitHub/Copilot may rate-limit heavily.

Fix:

-   Reduce concurrency.
-   Add additional channels and let One-API load-balance.

## Security Notes

-   Treat the GitHub access token as a high-value secret.
-   Prefer least-privilege tokens where possible.
-   Rotate tokens regularly.
-   This integration relies on GitHub/Copilot internal APIs; usage may be subject to GitHub terms and may change without notice.

## Implementation Pointers

-   Copilot adaptor: [relay/adaptor/copilot/adaptor.go](relay/adaptor/copilot/adaptor.go)
-   Token cache/exchange: [relay/adaptor/copilot/token.go](relay/adaptor/copilot/token.go)
