# Local tool namespaces in native Responses

Native Responses accepts a namespace containing client-executed `function` or
`custom` definitions. The group and child names remain separate; the gateway
never invents dotted names or executes these tools. Function definitions remain
part of model input and ordinary input/output token settlement is unchanged.
There is no extra namespace hosted-tool fee.

The previous native billing guard rejected every namespace before reservation
or provider dispatch. It was introduced to prevent unmetered hosted tools, but
also rejected supported local grouping. This repair retains the deny-by-default
hosted-tool boundary and admits only reviewed local group contents.

Groups require nonempty ASCII tool identifiers (letters, digits, underscore or
hyphen, at most 64 bytes) and one to 1024 children. The child-count ceiling is a
gateway resource bound. Nested namespaces, hosted children (including web search),
unknown types, malformed groups and ambiguous typed JSON keys are rejected before
upstream dispatch. Decode preflight prevents recursive namespace expansion.
Function/custom metadata, grammar, strict/deferred flags and original child order
survive native raw and typed-fallback preparation. Policy pruning/reordering
preserves unknown metadata within each matching namespace without resurrecting
removed children. Equal leaf names in different groups remain distinct.

Namespaces retain provider ownership even when a gateway MCP alias has the same
name. Chat/Claude fallback (channels without native Responses support) now
emulates namespaces instead of rejecting them: see
[Chat/Claude fallback emulation](#chatclaude-fallback-emulation) below. Flat
local tools and existing top-level metered web search retain their prior
admission. Native JSON/SSE passthrough remains intact;
typed output calls now retain the separate namespace and custom input fields.
Echoed response tool-definition DTOs retain their existing representation; this
change does not claim generalized namespace conversion on other API formats.

Regression fixtures use loopback providers and isolated SQLite ledgers, including
stream/nonstream forwarding, exact input/output token charges, zero reservation
and dispatch on rejection, scoped metadata merge, DTO size/depth and malformed
JSON controls. No production request contents, credentials or billing records
are used or migrated. Reverting this source change restores the prior fail-closed
namespace rejection; catalog metadata is independent of server admission.

Protocol references:
- https://developers.openai.com/api/docs/guides/tools
- https://developers.openai.com/api/docs/guides/function-calling

Production rollout and a paid upstream acceptance probe require separate approval.

## Chat/Claude fallback emulation

Chat Completions and Claude Messages have no namespaces, so the fallback
converter flattens every namespace child into one upstream tool and keeps a
reversible mapping that is rebuilt from the same request when the reply is
rendered (no shared state):

- `functions` (and an empty namespace) is the client default namespace; its
  children keep their own names.
- Other children become `<namespace>__<name>`, or `<namespace><name>` when the
  name starts with `_` or the namespace ends with `_` (Codex app connectors and
  older MCP namespaces), matching Codex's historical MCP names.
- Names longer than 64 bytes or with characters upstreams reject are replaced by
  a sanitized prefix plus an FNV-1a suffix of the full identity; a collision with
  an existing flat name is disambiguated the same way.
- The namespace description prefixes each child description as `[description]`.
- Custom (freeform/grammar) tools become a function with one required string
  argument `input`; the grammar is appended to the description.

Upstream calls to a flattened name are rendered back as `function_call` items
with separate `namespace` and `name` (namespaced calls also carry an empty
`encrypted_function_args` list, which Codex reads as plaintext arguments) or as
`custom_tool_call` items with the raw `input`, in JSON and SSE. Namespaced
`function_call` / `custom_tool_call` history is lowered with the same names.
Catalogs are bounded to 1,024 children per namespace and 4,096 flattened
definitions per request; nested namespaces and hosted children return
400 `response_fallback_not_supported` before state hydration or quota
reservation. Coverage: `responseapi_fallback_tool_*_test.go`,
`response_fallback_codex_{http,stream}_test.go`, and
`TestResponseNamespaceHTTPFallbackDispatchesFlattenedTools`.

