package openai

import (
	"maps"
	"strings"

	"github.com/Laisky/one-api/relay/adaptor"
)

// nativeOnlyModelIDs lists models whose bundled tariff is verified only for
// OpenAI's first-party API, reviewed 2026-11-22. Azure publishes GPT-6.2
// availability, but its retrieved pricing table contains no numeric quote.
// Hosts inheriting OpenAI's catalog (Azure AI Foundry, the AIProxy library API)
// therefore neither advertise nor price these IDs automatically: a
// deployment is billable only through an operator-configured channel tariff.
// Sources:
//   - https://learn.microsoft.com/en-us/azure/foundry/foundry-models/concepts/models-sold-directly-by-azure
//   - https://azure.microsoft.com/en-us/pricing/details/azure-openai/
//   - https://docs.aiproxy.io/dev/library
var nativeOnlyModelIDs = map[string]struct{}{
	"gpt-6.2-astra":          {},
	"gpt-6.2-sol":            {},
	"gpt-6-cyber":            {},
	"gpt-image-2.6-sunburst": {},
	"gpt-image-2.6-flare":    {},
}

// inheritedModelRatios is OpenAI's catalog as inherited by hosts that resell
// or redeploy OpenAI models without independent evidence for native-only IDs.
var inheritedModelRatios = buildInheritedModelRatios()

// inheritedModelList is the discovery list derived from inheritedModelRatios.
var inheritedModelList = adaptor.GetModelListFromPricing(inheritedModelRatios)

// IsNativeOnlyModel reports whether name has a bundled tariff only for OpenAI's
// first-party API. Parameters: name is a requested or mapped model ID; matching
// ignores case and surrounding whitespace so a respelled deployment cannot
// escape the policy. Returns: true when an inheriting host needs an operator tariff.
func IsNativeOnlyModel(name string) bool {
	_, ok := nativeOnlyModelIDs[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

// InheritedModelRatios returns a private copy of OpenAI's catalog without
// native-only models. It takes no parameters and returns a map callers may
// mutate; OpenAI's own ModelRatios is never modified.
func InheritedModelRatios() map[string]adaptor.ModelConfig {
	return maps.Clone(inheritedModelRatios)
}

// InheritedModelList returns the discovery IDs matching InheritedModelRatios.
// It takes no parameters and returns a fresh slice callers may mutate.
func InheritedModelList() []string {
	return append([]string(nil), inheritedModelList...)
}

// buildInheritedModelRatios filters native-only IDs out of ModelRatios. It
// takes no parameters and returns a new map sharing only immutable entries.
func buildInheritedModelRatios() map[string]adaptor.ModelConfig {
	inherited := make(map[string]adaptor.ModelConfig, len(ModelRatios))
	for name, cfg := range ModelRatios {
		if !IsNativeOnlyModel(name) {
			inherited[name] = cfg
		}
	}
	return inherited
}
