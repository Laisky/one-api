package openai

import "github.com/songquanpeng/one-api/relay/adaptor"

// DefaultToolingConfig returns OpenAI's upstream tooling defaults so channel
// policy resolution can merge in provider pricing and allowlists.
func (a *Adaptor) DefaultToolingConfig() adaptor.ChannelToolConfig {
	return OpenAIToolingDefaults
}

func (a *Adaptor) GetModelList() []string {
	return adaptor.GetModelListFromPricing(ModelRatios)
}

func (a *Adaptor) GetChannelName() string {
	channelName, _ := GetCompatibleChannelMeta(a.ChannelType)
	return channelName
}

// GetDefaultModelPricing returns the default OpenAI model pricing map.
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return ModelRatios
}

func (a *Adaptor) GetModelRatio(modelName string) float64 {
	if price, exists := ModelRatios[modelName]; exists {
		return price.Ratio
	}
	return a.DefaultPricingMethods.GetModelRatio(modelName)
}

func (a *Adaptor) GetCompletionRatio(modelName string) float64 {
	if price, exists := ModelRatios[modelName]; exists {
		return price.CompletionRatio
	}
	return a.DefaultPricingMethods.GetCompletionRatio(modelName)
}
