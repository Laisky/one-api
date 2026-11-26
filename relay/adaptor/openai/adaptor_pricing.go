package openai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
)

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

// UsesNativeOpenAIPricing identifies the native OpenAI tariff boundary.
// It takes no parameters and returns true for native or unset channel types;
// compatible and hosted channels retain their own operator pricing contracts.
func (a *Adaptor) UsesNativeOpenAIPricing() bool {
	return a.ChannelType == 0 || a.ChannelType == channeltype.OpenAI
}

// pricingTable returns the pricing map for the channel this adaptor instance serves.
//
// This adaptor backs every entry in CompatibleChannels, not just OpenAI, so all
// four pricing methods must key off a.ChannelType. Only channeltype.OpenAI
// selects native OpenAI's first-party table with its dated schedules; an unset
// ChannelType selects the shared catalog (see GetCompatibleChannelPricing). The
// global fallback catalog never reads this table: see GetGlobalFallbackModelPricing.
//
// Return values:
//   - map[string]adaptor.ModelConfig: the channel's audited pricing.
func (a *Adaptor) pricingTable() map[string]adaptor.ModelConfig {
	return GetCompatibleChannelPricing(a.ChannelType)
}

// GetGlobalFallbackModelPricing returns the catalog this adaptor contributes to
// the global fallback layer, implementing adaptor.GlobalPricingContributor. For
// native OpenAI that is the shared ModelRatios catalog, without first-party
// dated schedules (nativeModelRatios): a provider that prices an OpenAI model
// only through the global layer has no evidence that OpenAI's own price
// changes apply to it. A compatible channel type contributes its own table.
// It takes no parameters and returns shared read-only catalog data.
func (a *Adaptor) GetGlobalFallbackModelPricing() map[string]adaptor.ModelConfig {
	if a.UsesNativeOpenAIPricing() {
		return ModelRatios
	}
	return a.pricingTable()
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
