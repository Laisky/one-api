package vertexai

import "slices"

// vertexSuggestedModels is the Vertex-owned, documentation-checked suggestion
// catalog as of 2026-11-12. It is deliberately independent of legacy dispatch and
// pricing maps. Inclusion is not an entitlement, region, quota, or live-service
// guarantee. See docs/vertexai-model-audit-20261112.md for lifecycle and limits.
var vertexSuggestedModels = []string{
	// Native generateContent: active, deprecated-but-served, and public preview.
	"gemini-4-hydrogen",
	"gemini-4-argon",
	"gemini-3.8-flash",
	"gemini-3.7-flash",
	"gemini-3.6-flash",
	"gemini-3.5-flash",
	"gemini-3.5-flash-lite",
	"gemini-3.1-flash-lite",
	"gemini-3.1-pro-preview",
	"gemini-3-flash-preview",
	"gemini-robotics-er-2-preview",
	// Restricted models require the upstream entitlement; pricing gates are separate.
	"gemini-4-carbon",
	"gemini-4-carbon-cyber-permissive",
	"gemini-4-argon-cyber-permissive",
	"gemini-3.8-flash-cyber",
	// Native image generation, including documented legacy models still served.
	"gemini-nano-banana-2.1",
	"gemini-3.1-flash-lite-image",
	"gemini-3-pro-image",
	"gemini-3.1-flash-image",
	"gemini-2.5-flash-image",
	// Native Live WebSocket routes require explicit Vertex realtime pricing.
	"gemini-3.8-live",
	"gemini-live-2.5-flash-native-audio",
	// Claude rawPredict routes: older retired Claude generations stay routable only.
	"claude-fable-5",
	"claude-fable-5-1",
	"claude-fable-5-2",
	"claude-opus-4-5@20251101",
	"claude-opus-4-6",
	"claude-opus-4-7",
	"claude-opus-4-8",
	"claude-opus-5",
	"claude-opus-5-5",
	"claude-sonnet-4-5@20250929",
	"claude-sonnet-4-6",
	"claude-sonnet-5",
	"claude-sonnet-5-5",
	"claude-haiku-4-5@20251001",
	"claude-haiku-5-5",
	// Veo 3.1 GA and documented Lite preview ID. Audio billing limits still apply.
	"veo-3.1-generate-001",
	"veo-3.1-fast-generate-001",
	"veo-3.1-lite-generate-001",
	// Verified OpenAI-compatible MaaS families.
	"openai/gpt-oss-120b-maas",
	"zai-org/glm-5.2-maas",
	"google/gemma-4-26b-a4b-it-maas",
	"meta/llama-4-maverick-17b-128e-instruct-maas",
	"meta/llama-4-scout-17b-16e-instruct-maas",
	"deepseek-ai/deepseek-v4.1-flash-maas",
}

// vertexModelSuggestions returns a sorted independent copy of the catalog.
// Parameters: none. Returns: unique suggested model IDs, without changing routes.
func vertexModelSuggestions() []string {
	models := slices.Clone(vertexSuggestedModels)
	slices.Sort(models)
	return slices.Compact(models)
}
