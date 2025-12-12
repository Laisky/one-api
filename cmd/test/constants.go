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
// Rationale:
// Keep this list small and strictly evidence-based. If a provider supports structured
// outputs via any compatible API surface, one-api should convert the request rather than
// skipping (e.g. Claude Messages structured -> Response API structured).
var structuredVariantSkips = map[string]map[string]string{
	"claude_structured_stream_false": {},
	"claude_structured_stream_true":  {},
}
