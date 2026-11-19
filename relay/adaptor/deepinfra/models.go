package deepinfra

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// ModelRatios assembles the deepinfra catalog from stable model-family files.
// Each model ID has one definition; JoinModelCatalogs rejects duplicate IDs.
// Source: https://api.deepinfra.com/models/list
var ModelRatios = modelCatalog()

// modelCatalog returns a fresh complete catalog with independently owned family metadata.
// It takes no arguments and rejects duplicate model IDs during assembly, then
// marks deprecated models and floors their forwarded tariffs (applyDeprecations).
func modelCatalog() map[string]adaptor.ModelConfig {
	catalog := adaptor.JoinModelCatalogs(
		anthropicModels(),
		baaiModels(),
		blackForestLabsModels(),
		briaModels(),
		bytedanceModels(),
		clarityaiModels(),
		deepseekAiModels(),
		googleModels(),
		intfloatModels(),
		metaLlamaModels(),
		mistralaiModels(),
		moonshotaiModels(),
		nvidiaModels(),
		openaiModels(),
		otherModels(),
		qwenModels(),
		sentenceTransformersModels(),
		wanAiModels(),
		xiaomimimoModels(),
		zaiOrgModels(),
	)
	applyDeprecations(catalog)
	return catalog
}

// nativeRate converts a USD per-million input-unit price into billing ratios.
// It accepts the published amount and returns the existing float64 calculation,
// preserving established rounding for token and character fallback tariffs.
func nativeRate(amount float64) float64 { return amount * ratio.MilliTokensUsd }
