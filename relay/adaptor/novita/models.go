package novita

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios assembles the novita catalog from stable model-family files.
// Each model ID has one definition; JoinModelCatalogs rejects duplicate IDs.
// Source: https://api.novita.ai/v3/openai/models
var ModelRatios = modelCatalog()

// modelCatalog returns a fresh complete catalog with independently owned family metadata.
// It takes no arguments and rejects duplicate model IDs during assembly.
func modelCatalog() map[string]adaptor.ModelConfig {
	return adaptor.JoinModelCatalogs(
		baiduModels(),
		deepseekModels(),
		googleModels(),
		inclusionaiModels(),
		metaLlamaModels(),
		minimaxModels(),
		moonshotaiModels(),
		otherModels(),
		qwenModels(),
		sao10kModels(),
		tencentModels(),
		xiaomimimoModels(),
		zaiOrgModels(),
	)
}

// ModelList retains the existing channel discovery interface.
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// NovitaToolingDefaults retains this provider's separately metered tool defaults.
var NovitaToolingDefaults = adaptor.ChannelToolConfig{}

// nativeRate converts a USD per-million input-unit price into billing ratios.
// It accepts the published amount and returns the existing float64 calculation,
// preserving established rounding for token and character fallback tariffs.
func nativeRate(amount float64) float64 { return amount * ratio.MilliTokensUsd }
