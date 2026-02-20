package openai

import "github.com/songquanpeng/one-api/relay/adaptor"

type Adaptor struct {
	adaptor.DefaultPricingMethods
	ChannelType int
}
