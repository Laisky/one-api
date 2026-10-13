package minimax

import "github.com/Laisky/one-api/relay/adaptor"

// Legacy metadata is retained for existing configurations, not as a claim that
// every historical endpoint remains available. Current M-series definitions are
// separate from these legacy defaults and use the documented pay-as-you-go API.
var (
	minimaxTextInputs        = []string{"text"}
	minimaxVisionInputs      = []string{"text", "image"}
	minimaxTextOutputs       = []string{"text"}
	minimaxChatFeatures      = []string{"tools", "json_mode"}
	minimaxReasoningFeatures = []string{"tools", "json_mode", "reasoning"}
	minimaxSamplingParams    = []string{"temperature", "top_p", "max_tokens", "stop", "frequency_penalty", "presence_penalty", "seed"}
)

// ModelRatios contains one complete definition per MiniMax API model ID.
// Current model and tariff sources were checked on 2026-10-12:
// https://platform.minimax.cn/docs/guides/models-intro
// https://platform.minimax.cn/docs/guides/pricing-paygo
// https://platform.minimax.cn/docs/api-reference/text-openai-api
// Native MiniMax transport is not implemented by this catalogue-only adaptor.
var ModelRatios = adaptor.JoinModelCatalogs(minimaxCurrentModels(), minimaxLegacyModels())

// ModelList retains the compatibility list derived from the complete catalogue.
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// MinimaxToolingDefaults remains empty because native server-tool transport is
// not implemented here. MiniMax publishes a separate CNY 0.03 web_search fee;
// omitting its default is not a claim that server-side search is free.
var MinimaxToolingDefaults = adaptor.ChannelToolConfig{}
