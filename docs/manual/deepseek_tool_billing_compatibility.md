# DeepSeek native Search/Fetch: conservative billing and availability

OneAPI does **not** blanket-reject positively priced DeepSeek native Search/Fetch
requests solely because DeepSeek does not provide a documented paid-success
counter. Operators explicitly enable these capabilities and set prices using
the existing channel/provider tooling policy. OneAPI then bills an estimate
that may exceed upstream successful-operation charges.

## Admission and billing contract

1. The selected channel/model/provider must authorize the built-in tool and
   resolve a nonnegative effective per-call quota price. An unknown or forbidden
   capability is not made safe by this policy; existing whitelist rules still
   apply. A genuinely insufficient user/token balance still rejects dispatch.
2. For a positively priced hosted `web_search` or `web_fetch` on a real
   DeepSeek endpoint, OneAPI starts with a **conservative ten-attempt minimum per
   admitted tool declaration**. Claude Messages may declare a larger
   `max_uses`; in that case the larger count is used. WebSocket execution
   uses ten attempts for **each accepted `response.create` frame** that declares
   the tool.
3. Count observable server-tool events and usage counters using the existing
   protocol parsers and canonical capability names. Repeated streaming events
   with a stable ID are deduplicated. The effective billed count per tool is the
   greater of the conservative minimum and the observed attempts, not their sum.
   Search and Fetch have independent configured prices and counts.
4. HTTP Chat, Claude Messages, and native Responses reserve the tool floor
   alongside the existing token quote **before** provider dispatch. Claude
   separately adds independent non-Search/Fetch server-tool allowances; raw
   Search/Fetch and converted Search/Fetch share one conservative allowance, so
   a capability is never reserved twice merely because it appears in both forms.
   Successful responses settle measured Token/cache usage and ToolCost through existing
   user/token/request-cost/consume/tool ledgers. A known completed Token receipt
   must not retain an unrelated inflated Token reservation merely because tool
   attempt counts were estimated.
5. Requests with possibly accepted provider work but missing terminal usage, an
   ambiguous transport failure, or a provider 5xx after dispatch retain the
   funded conservative reservation and emit an estimated consume record. The
   retry/refund boundary must not refund paid tools and then retry them for free.
   Proven pre-dispatch failures and quota/validation denials retain existing
   zero-work refund and denial behavior.
6. Native Responses WebSocket opens with an existing token handshake reservation
   before client frames arrive. Each create first passes ownership, model,
   ambiguous-key and discovery validation **without an additional debit**.
   The admission callback then inspects the original frame and atomically reserves
   its extra Search/Fetch allowance against the same durable user and token
   balances used by HTTP **before provider forwarding**. A model-switch or
   ownership denial cannot bill an unforwarded paid frame; if no create was
   accepted, the existing handshake hold is refunded. Earlier accepted work
   remains billable after a subsequent denied frame. Only successfully
   forwarded create frames enter the tool meter; concurrent sessions cannot
   spend the same finite balance. The session reconciles all incremental
   holds with the handshake reservation and observed tool costs after both
   proxy legs drain. A transport write failure after a successful reservation
   remains ambiguous work and is conservatively retained to avoid underbilling.
   An abrupt process loss between durable debit and final log reconciliation
   still requires the existing billing-recovery audit.
7. Effective zero-price tools remain free to the OneAPI user but are not evidence
   that upstream execution is free. Client-executed local function calls,
   client Tool Search, and unrelated third-party model providers keep their
   established policies. Model name alone never determines the DeepSeek API
   contract; the current selected upstream host and endpoint override do.

### Worked example

If the effective OneAPI Search price is **17 quota per call**, a single
DeepSeek request that offers native Search is assigned at least
**10 × 17 = 170 quota for Search**, plus its ordinary measured Token fee.
One observed `web_search_call` with a failed or empty result does not reduce
this estimate. Twelve observed distinct attempts produce **12 × 17 = 204**
quota, not 170 + 204. If the provider returns a complete Token receipt worth
16 quota, the two total examples settle at **186** and **220** quota.

This pricing is intentionally conservative and can charge customers for tools
that were merely offered or that did not successfully execute. Even a declaration
that DeepSeek silently ignores (for example `web_search` on Flash or `web_fetch` in
Responses) may incur the explicitly configured OneAPI commercial floor: forwarding
an allowed, priced declaration is not proof of upstream execution, but neither
is missing execution evidence proof of zero upstream cost. Disclose this billing
policy and the effective per-call tariff before enabling it.

## What this policy does **not** guarantee

DeepSeek's direct Search/Fetch API is documented to bill per successful
non-empty HTTP 200 response. But its *model-internal* Search/Fetch process does
not provide a documented authoritative paid-operation count, and the Responses
API ignores `max_tool_calls`. Thus ten attempts is an explicitly chosen
**commercial floor**, **not a provider-guaranteed upper bound**. Invisible
internal calls above the floor could still lead to undercollection. The only
way to establish a strict no-loss guarantee is audited upstream bill
reconciliation or gateway-owned direct Search/Fetch execution.

Official source: [Search & Fetch billing](https://api-docs.deepseek.com/guides/search/),
[Responses API compatibility](https://api-docs.deepseek.com/guides/responses_api/).
As of October 2026, DeepSeek's English guide lists USD 0.75 per 1,000
successful Search/Fetch operations and a November 1, 2026 start for server-side
tool charging in Harness/Claude Code/Codex. These public upstream prices are
**not** automatically converted to OneAPI customer tariffs, and this PR does
not change the operator's configured currency, quota or effective date.

## Compatibility and deployment

- OneAPI's dedicated DeepSeek adaptor retains its existing empty default
  built-in Search/Fetch price table. The operator must explicitly configure
  allowed tool prices to enable them; unpriced/default-pruned tools remain as
  before. Native DeepSeek Responses documents server `web_search` for
  `deepseek-v4-pro`; Flash ignores it. Support for arbitrary `web_fetch`
  declarations does not imply provider support on every API surface.
- WebSocket original frame bytes and model-rewrite unknown JSON numerics are
  preserved. Duplicate/ambiguous billing keys, unsafe hosted discovery and
  model switching remain denied. Effective-zero and unrelated-provider controls
  retain existing behavior.
- There are no schema migrations, historical charge rewrites, new API keys,
  or a second billing/settlement subsystem. The existing tool invocation
  summary and request-cost logs are the audit records.
- The HTTP/Claude, MCP endpoint-switch and WebSocket tests use owned synthetic
  TLS/WS providers and persisted test ledgers; they cannot prove live upstream
  billing equivalence. Acceptance requires focused RED-to-GREEN, affected
  package race suites, Go 1.27.3 formatting/static checks and code review.
- Merge into main publishes/deploys production automatically and requires
  separate approval under [issue #592](https://github.com/Laisky/one-api/issues/592).
  Retain the immutable running image for rollback. A rollback does not undo
  historical user charges and restores the earlier tool admission/precision bugs.

### Served-candidate release acceptance (issue #592)

The [2026-10-27 release evidence and operator checklist](../../tasks/20261027_592_served_candidate_acceptance.md)
distinguishes source integration, published image digests and historical deployment
health from *current* production provenance, effective tariffs, synthetic billing
canaries, and recoverable rollback. The former #595 gate was explicitly cancelled;
the separate Responses WebSocket funding defect #596 was fixed by merged PR #598.
Neither event is proof of currently served behavior or consent for a new rollout.
Keep #592 open until its remaining, separately authorized acceptance evidence is
recorded. This section changes no release/CI or billing defaults.

Related tests: `relay/controller/deepseek_tool_receipt_test.go`,
`relay/controller/deepseek_receipt_alias_regression_test.go`,
`relay/controller/deepseek_fetch_receipt_test.go`,
`relay/controller/deepseek_mcp_route_regression_test.go`,
`relay/controller/deepseek_ws_receipt_test.go`,
`relay/controller/deepseek_ws_conservative_test.go`, and
`relay/tooling/deepseek_conservative_test.go`.
