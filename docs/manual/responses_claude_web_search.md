# Responses `web_search` on Claude fallback channels

OpenAI Responses requests that reach a Claude channel run through the Responses → Chat →
Claude Messages fallback. Before this change, the hosted `web_search` declaration was
admitted, priced and then silently dropped. Codex (0.166) always declares it. Now the fallback
forwards it as Claude's server-side web search tool on platforms that can run it and meter it.

## Request mapping

The first root tool whose canonical capability is `web_search` (`web_search`,
`web_search_preview`, or a dated `web_search_YYYYMMDD`) is the one mapped. Admission
priced exactly these tools.

| Responses field | Claude tool field |
| --- | --- |
| tool type | `web_search_20260209` or `web_search_20250305` (see the platform table below); `name` is always `web_search` |
| `filters.allowed_domains` | `allowed_domains` |
| `filters.disallowed_domains` | `blocked_domains`. Setting both lists returns `400 response_fallback_not_supported` before any reservation; Claude rejects that combination. |
| `user_location` (`type: approximate`, with flattened or nested `city`/`region`/`country`/`timezone`) | `user_location` (`type: approximate`); omitted when empty |
| `search_context_size`, `search_content_types`, `indexed_web_access` | not forwarded; Claude has no equivalent |
| `external_web_access` | not forwarded; see below |
| (none) | `max_uses` is omitted, which leaves the search count uncapped, as Anthropic's default |

The gateway bounds its own input handling: at most 100 domains, and at most 256 bytes per
domain or location field.

### Cached mode (`external_web_access: false`)

Codex's "cached" mode still expects search results; it only asks for an indexed source rather
than live page access. Claude has no cached mode. Claude web search returns results from
Anthropic's search provider and never fetches arbitrary pages; that is `web_fetch`, which this
mapping never enables. That behavior is closer to the cached contract than dropping the tool,
so **both live and cached declarations enable Claude web search**, at the same price. Operators
who want no search on a channel can leave `web_search` out of its tooling whitelist. The request
is then rejected up front with a 400 instead of being silently degraded.

## Platforms and versions

| Channel | Mapping | Reason |
| --- | --- | --- |
| `Anthropic` (first-party) | `web_search_20260209` for Claude 4.6+ (Opus/Sonnet 4.6, Opus 4.7/4.8, Opus/Sonnet 5, Opus/Sonnet/Haiku 5.5, Fable 5, Mythos 5, Mythos Preview); otherwise `web_search_20250305` | Anthropic documents dynamic filtering only for 4.6+ models. Earlier models need the basic version, or the request fails with a 400. |
| `Azure` (Claude on Microsoft Foundry) | `web_search_20250305` | Foundry deployments hosted on Azure support only the basic version. Azure tool admission requires an explicit channel price. |
| `VertextAI` (Claude model) | `web_search_20250305` | Google-managed Claude offers only basic web search. |
| `AwsClaude` (Bedrock) | dropped | Bedrock offers no Claude web search, and its receipt parser has no `server_tool_use` counters, so a search could not be metered. |
| `ClaudeCompatible` | dropped | Third-party Claude-compatible hosts have no documented server-tool contract. |
| Chat-only upstreams | dropped | Unchanged. No Chat adaptor maps hosted web search, and no tool billing is invented for them. |
| MCP-orchestrated fallback | dropped | MCP rounds keep their existing tool handling. |

The doc source for versions and platforms is
<https://platform.claude.com/docs/en/agents-and-tools/tool-use/web-search-tool> (verified 2026-11-05).
With dynamic filtering, Claude may run provider-provisioned code execution. Its blocks are never
exposed as caller tool calls. Anthropic does not charge for that code execution, and the default
Anthropic tooling policy leaves `code_execution` unpriced.

## Response rendering

- Each Claude `server_tool_use` block named `web_search` becomes a Responses
  `web_search_call` item with ID `ws_<suffix>`, where the suffix is the Claude
  `srvtoolu_` ID with the prefix removed. The item carries
  `action: {"type":"search","query":...}` and a status:
  - `completed` when a result list arrives.
  - `failed` when the result is an error object, or when no result arrives before the
    message ends.
- **JSON:** the output is ordered message, reasoning, `web_search_call` items, then caller tool calls.
- **SSE:** each search emits `response.output_item.added` (`in_progress`),
  `response.web_search_call.in_progress`, `response.web_search_call.searching`,
  `response.web_search_call.completed` (successful searches only), and `response.output_item.done`.
  All of these events share one `output_index`. The message keeps index 0, as before. Indexes are
  allocated in arrival order, and `response.completed.output` is sorted by index.
- No server-side block (`server_tool_use`, `mcp_tool_use`, `*_tool_result`) ever leaks as a
  caller `function_call`.
- Assistant text is plain text. Claude citations are not converted to `url_citation` annotations,
  because the fallback had no existing annotation pattern. Text written before a search round and
  text written after it is joined with a blank line.
- `pause_turn` is not resumed automatically. It surfaces as a completed response.

## History replay

Codex replays `web_search_call` items verbatim. On fallback they are accepted and **dropped**:

- fidelity admission accepts them;
- the converter removes them without splitting the surrounding assistant turn;
- `state.FallbackLowering` classifies them as `drop` for hydrated chains.

The searches' findings already live in the assistant text that follows. Claude's encrypted search
results cannot be reconstructed from Responses items. Native Responses passthrough is unchanged,
and other hosted items (`code_interpreter_call`, `file_search_call`, `mcp_call`, …) still fail
closed with `state_not_portable`.

## Billing

The fallback uses the same server-tool billing path as native Claude Messages:

1. Admission is unchanged: `tooling.ValidateResponseBuiltinToolsWithExclusions` requires a
   whitelisted, priced `web_search`. The Anthropic default is 10 USD per 1,000 searches, which is
   5,000 quota per search; channel tooling pricing overrides it.
2. **Reservation:** when the search is forwarded, the Chat reservation adds 10 searches at the
   admitted per-call price (`claudeServerToolDefaultReservedUses`, the same floor native Claude
   Messages uses when `max_uses` is absent). A complete receipt refunds the unused part. A missing
   or incomplete receipt keeps the funded estimate.
3. **Settlement:** the Claude response handlers merge `usage.server_tool_use.web_search_requests`
   by maximum, so cumulative and duplicate stream receipts never add up. When no receipt counter is
   present, they fall back to the observed search blocks. `tooling.ApplyBuiltinToolCharges` then
   charges `count × price` once.

**Example:** a request whose upstream reports `web_search_requests: 2` at 17 quota per search,
with 11 input and 7 output tokens at ratio 1, settles at **18 + 2 × 17 = 52 quota**.

Tests:

- `relay/controller/response_fallback_claude_websearch_test.go` covers JSON, SSE, replay, the
  unsupported-platform and reservation-delta cases, and filter rejection.
- `relay/adaptor/anthropic/converted_web_search_test.go` and `web_search_render_test.go` cover
  the conversion and rendering.
- `relay/adaptor/openai/responseapi_web_search_history_test.go` covers history lowering.
- `relay/state/fallback_lowering_test.go` covers the state lowering table.

Removing or doubling the fallback `ApplyBuiltinToolCharges` call makes the JSON and SSE ledger
assertions fail: they settle at 18 and 86 instead of 52.
