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
name. Chat/Claude fallback returns HTTP 400 `tool_namespace_not_supported` before
state hydration or quota reservation because those conversions cannot preserve
qualified call identities. Flat local tools and existing top-level metered web
search retain their prior admission. Native JSON/SSE passthrough remains intact;
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
