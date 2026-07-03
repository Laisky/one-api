# One API Agent Skill

Use this skill when an agent needs to call One API as an AI gateway.

## Steps

1. Read `https://oneapi.laisky.com/llms.txt`.
2. Inspect `https://oneapi.laisky.com/openapi.json` for endpoint schemas.
3. Select the endpoint matching the caller format:
   - Chat Completions: `/v1/chat/completions`
   - Responses: `/v1/responses`
   - Claude Messages: `/v1/messages`
4. Add `Authorization: Bearer <relay-api-key>`.
5. Preserve secrets and avoid writing them to logs, URLs, or prompts.

## Output Expectations

Return provider responses in the requested client format when possible. Surface HTTP status, JSON error type, and request ID when calls fail.
