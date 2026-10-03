package openai

import "github.com/Laisky/one-api/relay/adaptor"

// DefaultToolingConfig returns OpenAI's upstream tooling defaults so channel
// policy resolution can merge in provider pricing and allowlists.
func (a *Adaptor) DefaultToolingConfig() adaptor.ChannelToolConfig {
	return OpenAIToolingDefaults
}

func (a *Adaptor) GetModelList() []string {
	return adaptor.GetModelListFromPricing(a.pricingTable())
}

func (a *Adaptor) GetChannelName() string {
	channelName, _ := GetCompatibleChannelMeta(a.ChannelType)
	return channelName
}

// SetChannelType binds this adaptor instance to a channel type so the pricing
// methods answer for that channel. It implements adaptor.ChannelTypeAware.
//
// Parameters:
//   - channelType: the channel type from relay/channeltype.
func (a *Adaptor) SetChannelType(channelType int) {
	a.ChannelType = channelType
}

// pricingTable returns the pricing map for the channel this adaptor instance serves.
//
// This adaptor backs every entry in CompatibleChannels, not just OpenAI, so all
// four pricing methods must key off a.ChannelType. Callers that want OpenAI's own
// table leave ChannelType at its zero value, which falls to the default branch.
//
// Return values:
//   - map[string]adaptor.ModelConfig: the channel's audited pricing.
func (a *Adaptor) pricingTable() map[string]adaptor.ModelConfig {
	return GetCompatibleChannelPricing(a.ChannelType)
}

// GetDefaultModelPricing returns the pricing map for this adaptor's channel type.
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return a.pricingTable()
}

func (a *Adaptor) GetModelRatio(modelName string) float64 {
	if price, exists := a.pricingTable()[modelName]; exists {
		return price.Ratio
	}
	return a.DefaultPricingMethods.GetModelRatio(modelName)
}

func (a *Adaptor) GetCompletionRatio(modelName string) float64 {
	if price, exists := a.pricingTable()[modelName]; exists {
		return price.CompletionRatio
	}
	return a.DefaultPricingMethods.GetCompletionRatio(modelName)
}
