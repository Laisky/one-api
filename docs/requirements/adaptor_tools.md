# Adaptor Tools

## User Story

A lot of LLM providers now supply their own built‑in tools, typically billed per invocation (see <https://platform.openai.com/docs/guides/tools>). One‑API currently charges only for token usage when forwarding requests, which can cause users to bypass tool‑related fees.

As a system administrator, I want to specify which LLM tools are permitted and define their usage pricing at the channel/adaptor level, so that my platform accurately enforces usage policies and bills my end-users in alignment with upstream provider requirements.”

### Acceptance Criteria

Admins must be able to:

- View, add, and remove tools from an allowlist for each channel.
- Define default and per-tool pricing, with pre-filled provider defaults where available.
- Requests invoking a disallowed tool must be rejected with a clear error message.
- The billing system must charge per tool usage as defined, in addition to token costs.
- Usage reports and quota logs must include both token consumption and tool invocation costs.
- The UI must offer context-sensitive validation and guidance.
- For each channel type, default provider tool lists and prices should be maintained and updatable.

### Example User Story in Standard Template:

As an administrator, I want to restrict the available tools for each LLM channel and set explicit per-tool prices, so that costs are predictable and match my provider’s billing, and so that clients cannot accidentally or maliciously use unsupported or high-cost tools.

## Feature Request List

### Channel Configuration Enhancements

- Add `tool_whitelist` and `tool_pricing` fields to the channel configuration schema and database.
- Introduce admin UI controls for editing the tool whitelist and per-tool pricing, with validation.

### Provider Defaults and Synchronization

- Implement per-provider default tool lists and prices (auto-populated based on upstream docs, updatable via admin panel).

### Request Validation

- Enhance relay/adaptor code to parse the incoming request’s tool invocations, compare against the channel’s tool whitelist, and reject requests with disallowed tools prior to forwarding.

### Billing System Extension

- Modify the billing engine such that the cost for tool invocation is calculated (and charged) per request in line with explicit per-tool pricing.
- In the case of multiple tools in a single request, sum costs accordingly.

### Usage Reporting and Logging

- Update method of quota usage and billing data export to include per-tool breakdowns in addition to token counts.

### API Documentation and Error Handling

- Document tool availability and pricing per channel/tenant endpoint.
- Ensure error messages for tool whitelist violations are clear and actionable.

### Security and Integrity Controls

- Validate tool/pricing configuration inputs at API and UI layer.
- Admin actions to adjust tool policies are logged and may (optionally) require elevated privileges.

### Backward Compatibility and Migration

- Default existing channels to allow all tools (or only tools already used historically) and migrate as per admin guidance.
- Provide batch update tooling for admins.

### Performance and Caching

- If performance bottlenecks emerge, cache tool pricing/whitelist configurations per channel in memory.

### Provider Change Adaptation

- Provide alerts/notifications for admins when upstream providers (e.g., OpenAI) modify tool offerings or pricing.

## Conclusion

The addition of a tool-level whitelist and explicit per-tool pricing mechanism to One-API’s channel configuration is both technically feasible and strategically essential for aligning the gateway with upstream provider billing practices and for providing stronger, auditable controls in multi-tenant LLM service aggregation.

The adaptor-based channel system in One-API is flexible and amenable to new fields and logic. By extending both configuration schemas and admin UI to allow per-tool access and cost policies, One-API can accurately enforce tool restrictions, prevent misconfiguration, and offer robust, transparent billing. Request validation and relay logic should intercept and reject non-whitelisted tool invocations, while billing must aggregate both token and tool costs in line with admin policies (and upstream pricing).

These improvements will ensure One-API remains a trustworthy, adaptable gateway as LLM APIs and their monetization strategies become more complex and as tools become a primary axis of both innovation and cost exposure.
