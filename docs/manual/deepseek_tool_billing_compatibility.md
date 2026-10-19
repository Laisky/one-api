# DeepSeek server-tool billing compatibility

OneAPI already supports built-in tool pricing, channel overrides, invocation accounting, and accepted-work settlement. This change reuses those mechanisms; it does not add a new tariff or billing subsystem.

## Admitted behavior

For a dedicated DeepSeek channel or an upstream host under `deepseek.com`, a request for `web_search` or Claude-style `web_fetch` with an effective positive tariff is rejected before quota reservation and provider dispatch. The response is HTTP 400 with `tool_billing_not_supported`. This includes positive USD-per-call and direct-quota overrides, and inherited OpenAI defaults on an OpenAI-compatible DeepSeek endpoint.

This changes previously unqualified operator overrides: a positive price no longer enables execution that OneAPI cannot settle from a documented DeepSeek paid-tool receipt. DeepSeek's own empty tooling defaults remain unchanged.

- Automatic pruning of disallowed default built-ins remains unchanged.
- Explicit zero tool tariffs remain subject to the existing allowlist and conversion rules. They mean zero tool charge to the OneAPI user; they do not establish an upstream free-service or receipt contract.
- Local functions named `search`, `fetch`, `web_search`, or `web_fetch`, custom tools, and explicitly client-executed `tool_search` retain their existing behavior.
- A model named DeepSeek on an unrelated provider keeps that provider's existing tooling policy. Provider identity, rather than a model or client name, selects this restriction.
- Native Responses `web_fetch` remains rejected by the existing unsupported-native-tool policy.

## Upstream context, not OneAPI price activation

The upstream Chinese schedule states RMB 0.005 per successful Search/Fetch request; the English schedule states USD 0.00075. These are distinct configured-currency schedules, not an instruction for OneAPI to calculate an exchange rate. The server-side grace boundary is 2026-10-31T16:00:00Z (Beijing November 1 at midnight); direct Search/Fetch endpoints already have their own charging contract. See the [Chinese pricing](https://api-docs.deepseek.com/zh-cn/quick_start/pricing/), [English pricing](https://api-docs.deepseek.com/quick_start/pricing/), and [Search guide](https://api-docs.deepseek.com/zh-cn/guides/search/).

The direct endpoints define successful HTTP 200 with nonempty search results or fetched page content. Search results remain valid when content is not requested; an empty Fetch title does not invalidate a nonempty page body; result count does not change the per-request price. Their JSON does not provide a tool-usage/cost counter. These direct-success rules do not establish a terminal server-side paid-operation receipt. See [Search](https://api-docs.deepseek.com/zh-cn/api/search/) and [Fetch](https://api-docs.deepseek.com/zh-cn/api/fetch/).

Native Responses web search is supported for Pro and ignored for Flash; the upstream also ignores max_tool_calls. Neither a progress/output item nor a token-usage receipt documents how many successful paid Search/Fetch operations occurred. See the [Responses guide](https://api-docs.deepseek.com/zh-cn/guides/responses_api/) and [response API contract](https://api-docs.deepseek.com/zh-cn/api/create-response/). Token accounting remains separate; the [Chat API](https://api-docs.deepseek.com/zh-cn/api/create-chat-completion/) documents its final streaming usage receipt.

The policy therefore behaves the same immediately before, at, and after the grace boundary: no calendar-driven tool activation and no guessed charge from declarations, history, client identity, or generic output shapes.

## Usage and protocol preservation

DeepSeek's documented Responses `web_search` tool does not establish a paid-success counter or one-output-item-to-paid-operation mapping. OpenAI-style progress/final items, usage extensions, implicit search-model inference, and Anthropic-style Search/Fetch blocks/counters therefore do not become DeepSeek paid-tool counts or tool-invocation ledger entries. The original native JSON/SSE fields and unknown extensions remain on the wire. Token/cache usage and existing accepted-work, error-settlement, and retry accounting remain intact.

This is a fail-closed compatibility boundary, not new direct Search/Fetch endpoint support. It does not infer success from a declaration, conversation history, progress event, empty result, cancellation, or provider-internal retry. Other provider capabilities are not newly qualified by this change.

No prices, currencies, agreements, effective dates, or production billing records are activated or modified. In particular, this change does not schedule automatic future pricing activation. A future positive DeepSeek tariff requires an explicitly implemented and tested provider receipt contract first.

Official contract reference: [DeepSeek Responses API guide](https://api-docs.deepseek.com/guides/responses_api/).

## Verification and rollback

Synthetic tests cover positive tariff admission before all physical side effects; explicit zero/default/local controls; actual native Claude Fetch transport and physical token/cache/user/token/request ledgers; provider identity and aliases; item and usage-only inference; raw extensions and exact numeric tokens; duplicate terminal events; and unrelated OpenAI/Anthropic controls. No paid provider request is required.

Revert the source change to restore the previous compatibility behavior. No data migration or price/configuration rollback is needed. Existing user prices should not be changed merely to bypass this capability error.

The billing check follows the selected dispatch metadata. A nonempty full endpoint
URL override replaces the channel/base target for this check; the parsed override
hostname uses the existing DeepSeek hostname contract. A prior attempt's observed
request URL is not used. This preserves an explicit non-DeepSeek override even on
a DeepSeek channel and closes the reverse OpenAI-channel-to-DeepSeek override gap.

An intentional effective zero quote stays allowed. A nonzero converted quote
cannot bypass the DeepSeek capability check when an extremely large finite USD
override overflows the existing integer conversion. This is a DeepSeek admission
guard, not a change to general tariff conversion or unrelated provider pricing.
