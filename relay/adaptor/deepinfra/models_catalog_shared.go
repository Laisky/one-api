package deepinfra

import (
	"fmt"

	"github.com/Laisky/one-api/relay/adaptor"
)

type catalogTextEntry struct {
	name           string
	inputUSD       float64
	outputUSD      float64
	cachedInputUSD float64
	contextLength  int32
	modalities     []string
	reasoning      bool
}

func addDeepInfraCatalogModel(modelName string, config adaptor.ModelConfig) {
	if _, exists := ModelRatios[modelName]; exists {
		panic(fmt.Sprintf("duplicate DeepInfra model catalog entry %q", modelName))
	}
	ModelRatios[modelName] = config
}

func catalogTextModel(inputUSD, outputUSD, cachedInputUSD float64, contextLength int32, modalities []string, reasoning bool, description string) adaptor.ModelConfig {
	config := textModel(inputUSD, outputUSD, cachedInputUSD, contextLength, false, reasoning, description)
	config.InputModalities = append([]string(nil), modalities...)
	return config
}

func catalogEmbeddingModel(inputUSD float64, contextLength int32, modalities []string, description string) adaptor.ModelConfig {
	config := embeddingModel(inputUSD, contextLength, description)
	config.InputModalities = append([]string(nil), modalities...)
	return config
}

func flatImageModel(pricePerImageUSD float64, acceptsImageInput bool, description string) adaptor.ModelConfig {
	config := imageModel(pricePerImageUSD, description)
	// These catalog entries are flat per-image prices, independent of image size.
	config.Image.SizeMultipliers = nil
	config.InputModalities = []string{"text"}
	if acceptsImageInput {
		config.InputModalities = []string{"text", "image"}
	}
	return config
}
