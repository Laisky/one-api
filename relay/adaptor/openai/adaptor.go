package openai

import "github.com/Laisky/one-api/relay/adaptor"

type Adaptor struct {
	adaptor.DefaultPricingMethods
	ChannelType int
}
