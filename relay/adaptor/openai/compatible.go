package openai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/ai360"
	"github.com/Laisky/one-api/relay/adaptor/alibailian"
	"github.com/Laisky/one-api/relay/adaptor/baichuan"
	"github.com/Laisky/one-api/relay/adaptor/baiduv2"
	"github.com/Laisky/one-api/relay/adaptor/deepseek"
	"github.com/Laisky/one-api/relay/adaptor/doubao"
	"github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/Laisky/one-api/relay/adaptor/groq"
	"github.com/Laisky/one-api/relay/adaptor/lingyiwanwu"
	"github.com/Laisky/one-api/relay/adaptor/minimax"
	"github.com/Laisky/one-api/relay/adaptor/mistral"
	"github.com/Laisky/one-api/relay/adaptor/moonshot"
	"github.com/Laisky/one-api/relay/adaptor/novita"
	"github.com/Laisky/one-api/relay/adaptor/openrouter"
	"github.com/Laisky/one-api/relay/adaptor/siliconflow"
	"github.com/Laisky/one-api/relay/adaptor/stepfun"
	"github.com/Laisky/one-api/relay/adaptor/togetherai"
	"github.com/Laisky/one-api/relay/adaptor/xai"
	"github.com/Laisky/one-api/relay/adaptor/xunfeiv2"
	"github.com/Laisky/one-api/relay/channeltype"
)

var CompatibleChannels = []int{
	channeltype.Azure,
	channeltype.AI360,
	channeltype.Moonshot,
	channeltype.Baichuan,
	channeltype.Minimax,
	channeltype.Doubao,
	channeltype.Mistral,
	channeltype.Groq,
	channeltype.LingYiWanWu,
	channeltype.StepFun,
	channeltype.DeepSeek,
	channeltype.TogetherAI,
	channeltype.Novita,
	channeltype.SiliconFlow,
	channeltype.XAI,
	channeltype.BaiduV2,
	channeltype.XunfeiV2,
}

// GetCompatibleChannelPricing returns the default pricing table for an
// OpenAI-compatible channel type.
//
// The OpenAI adaptor serves every channel in CompatibleChannels, so its pricing
// methods must answer for the channel actually in use rather than for OpenAI.
// Before this existed, a Doubao/MiniMax/BaiduV2/... channel resolved no price at
// all and fell through to DefaultPricingMethods' 2.5 USD/1M — e.g. Doubao-pro-32k
// billed ~8750x its published rate. Keep the cases here in lockstep with
// GetCompatibleChannelMeta; TestCompatibleChannelPricingCoversAdvertisedModels
// fails if the two drift apart.
//
// Parameters:
//   - channelType: the channel type from relay/channeltype.
//
// Return values:
//   - map[string]adaptor.ModelConfig: pricing for that channel, or OpenAI's table
//     for channel types that are plain OpenAI.
func GetCompatibleChannelPricing(channelType int) map[string]adaptor.ModelConfig {
	switch channelType {
	case channeltype.Azure:
		return ModelRatios
	case channeltype.AI360:
		return ai360.ModelRatios
	case channeltype.Moonshot:
		return moonshot.ModelRatios
	case channeltype.Baichuan:
		return baichuan.ModelRatios
	case channeltype.Minimax:
		return minimax.ModelRatios
	case channeltype.Mistral:
		return mistral.ModelRatios
	case channeltype.Groq:
		return groq.ModelRatios
	case channeltype.LingYiWanWu:
		return lingyiwanwu.ModelRatios
	case channeltype.StepFun:
		return stepfun.ModelRatios
	case channeltype.DeepSeek:
		deepseekAdaptor := &deepseek.Adaptor{}
		return deepseekAdaptor.GetDefaultModelPricing()
	case channeltype.TogetherAI:
		return togetherai.ModelRatios
	case channeltype.Doubao:
		return doubao.ModelRatios
	case channeltype.Novita:
		return novita.ModelRatios
	case channeltype.SiliconFlow:
		return siliconflow.ModelRatios
	case channeltype.XAI:
		return xai.ModelRatios
	case channeltype.BaiduV2:
		return baiduv2.ModelRatios
	case channeltype.XunfeiV2:
		return xunfeiv2.ModelRatios
	case channeltype.OpenRouter:
		openrouterAdaptor := &openrouter.Adaptor{}
		return openrouterAdaptor.GetDefaultModelPricing()
	case channeltype.AliBailian:
		return alibailian.ModelRatios
	case channeltype.GeminiOpenAICompatible:
		return geminiOpenaiCompatible.ModelRatios
	default:
		return ModelRatios
	}
}

func GetCompatibleChannelMeta(channelType int) (string, []string) {
	switch channelType {
	case channeltype.Azure:
		return "azure", ModelList
	case channeltype.AI360:
		return "360", ai360.ModelList
	case channeltype.Moonshot:
		return "moonshot", moonshot.ModelList
	case channeltype.Baichuan:
		return "baichuan", baichuan.ModelList
	case channeltype.Minimax:
		return "minimax", minimax.ModelList
	case channeltype.Mistral:
		return "mistralai", mistral.ModelList
	case channeltype.Groq:
		return "groq", groq.ModelList
	case channeltype.LingYiWanWu:
		return "lingyiwanwu", lingyiwanwu.ModelList
	case channeltype.StepFun:
		return "stepfun", stepfun.ModelList
	case channeltype.DeepSeek:
		deepseekAdaptor := &deepseek.Adaptor{}
		return "deepseek", deepseekAdaptor.GetModelList()
	case channeltype.TogetherAI:
		return "together.ai", togetherai.ModelList
	case channeltype.Doubao:
		return "doubao", doubao.ModelList
	case channeltype.Novita:
		return "novita", novita.ModelList
	case channeltype.SiliconFlow:
		return "siliconflow", siliconflow.ModelList
	case channeltype.XAI:
		return "xai", xai.ModelList
	case channeltype.BaiduV2:
		return "baiduv2", baiduv2.ModelList
	case channeltype.XunfeiV2:
		return "xunfeiv2", xunfeiv2.ModelList
	case channeltype.OpenRouter:
		adaptor := &openrouter.Adaptor{}
		return "openrouter", adaptor.GetModelList()
	case channeltype.AliBailian:
		return "alibailian", alibailian.ModelList
	case channeltype.GeminiOpenAICompatible:
		return "geminiv2", geminiOpenaiCompatible.ModelList
	default:
		return "openai", ModelList
	}
}
