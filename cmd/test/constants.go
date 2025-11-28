package main

const (
	defaultAPIBase    = "https://oneapi.laisky.com"
	defaultTestModels = "gpt-4o-mini,gpt-5-mini,claude-haiku-4-5,gemini-2.5-flash,openai/gpt-oss-20b,deepseek-chat,grok-4-fast-non-reasoning,azure-gpt-5-nano"
	// defaultTestModels = "azure-gpt-5-nano"

	defaultMaxTokens   = 2048
	defaultTemperature = 0.7
	defaultTopP        = 0.9
	defaultTopK        = 40

	maxResponseBodySize = 1 << 20 // 1 MiB
	maxLoggedBodyBytes  = 2048
)

// visionUnsupportedModels enumerates models that are known to reject vision payloads.
var visionUnsupportedModels = map[string]struct{}{
	"deepseek-chat":      {},
	"openai/gpt-oss-20b": {},
}

// azureEOFProneVariants was historically used to skip non-streaming Response API variants
// where Azure would prematurely close the connection. This has since been fixed upstream
// and these variants now work reliably. Keeping the map empty but preserved for future use.
var azureEOFProneVariants = map[string]struct{}{}

// structuredVariantSkips enumerates provider/variant combinations where the upstream API
// provably lacks JSON-schema structured output support. Each entry provides a human-readable
// reason that will be surfaced in the regression report when the combination is skipped.
//
// Rationale for current skips:
//   - azure-gpt-5-nano (Azure-hosted GPT-5 nano) never emits structured JSON for Claude
//     Messages, returning empty message content even when forced; both streaming states are
//     skipped to avoid false failures while the provider lacks the capability.
//   - gpt-5-mini fails to stream Claude structured output (the stream only carries usage
//     deltas with no JSON blocks). Non-streaming is kept because it succeeds.
var structuredVariantSkips = map[string]map[string]string{
	"claude_structured_stream_false": {
		"azure-gpt-5-nano": "Azure GPT-5 nano does not return structured JSON for Claude messages (empty content)",
		"gpt-5-mini":       "GPT-5 mini returns empty content for Claude structured requests",
	},
	"claude_structured_stream_true": {
		"azure-gpt-5-nano": "Azure GPT-5 nano does not return structured JSON for Claude messages (empty content)",
		"gpt-5-mini":       "GPT-5 mini streams only usage deltas, never emitting structured JSON blocks",
	},
}

// toolHistoryVariantSkips enumerates model/variant pairs where the upstream refuses to
// invoke tools despite forced tool_choice. Each skip provides a reason surfaced in the
// regression report to distinguish provider-side limitations from adapter bugs.
//
// Rationale:
//   - openai/gpt-oss-20b: returns HTTP 400 "tool choice required but model did not call
//     tool" even when tool_choice is forced, indicating the model ignores the directive.
//   - gemini-2.5-flash (Chat Tools, Claude Tools stream): returns natural language instead
//     of tool invocations despite forced tool_choice.
//   - azure-gpt-5-nano (Chat Tools, Chat Tools History): GPT-5 reasoning models inconsistently
//     return reasoning text or empty tool_calls instead of proper tool invocations.
//   - gpt-5-mini (Chat Tools History stream): GPT-5 reasoning models inconsistently
//     return reasoning text instead of tool invocations when streaming is enabled.
//   - deepseek-chat (Chat Tools non-streaming): consistently times out on tool invocation
//     requests, likely due to model processing overhead.
var toolHistoryVariantSkips = map[string]map[string]string{
	"chat_tools_stream_false": {
		"gemini-2.5-flash": "Model returns text instead of tool invocations despite forced tool_choice",
		"deepseek-chat":    "Model times out on non-streaming tool invocation requests",
		"azure-gpt-5-nano": "GPT-5 reasoning model inconsistently returns reasoning instead of tool calls",
	},
	"chat_tools_history_stream_false": {
		"openai/gpt-oss-20b": "Model refuses forced tool_choice (upstream returns 400 tool_use_failed)",
		"azure-gpt-5-nano":   "Model returns reasoning text instead of tool calls",
	},
	"chat_tools_history_stream_true": {
		"azure-gpt-5-nano": "Model returns reasoning text instead of tool calls when streaming",
		"gpt-5-mini":       "GPT-5 reasoning model inconsistently returns reasoning instead of tool calls when streaming",
	},
	"response_tools_history_stream_false": {
		"openai/gpt-oss-20b": "Model refuses forced tool_choice (upstream returns 400 tool_use_failed)",
	},
	"claude_tools_stream_true": {
		"gemini-2.5-flash": "Model returns text instead of tool invocations despite forced tool_choice",
	},
	"claude_tools_history_stream_false": {
		"openai/gpt-oss-20b": "Model refuses forced tool_choice (upstream returns 400 tool_use_failed)",
	},
	"claude_tools_history_stream_true": {
		"gpt-5-mini": "GPT-5 reasoning model inconsistently returns reasoning instead of tool calls",
	},
}
