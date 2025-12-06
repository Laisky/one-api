package channeltype

// ChannelBaseURLConfig defines the configuration for a channel's base URL.
// URL is the default base URL.
// Editable indicates whether the user can modify the base URL.
type ChannelBaseURLConfig struct {
	URL      string
	Editable bool
}

// ChannelBaseURLConfigs defines the default base URLs and editability for each channel type.
// Index corresponds to channel type constant (e.g., OpenAI=1, Azure=3).
var ChannelBaseURLConfigs = []ChannelBaseURLConfig{
	{URL: "", Editable: true},                                                          // 0 Unknown
	{URL: "https://api.openai.com", Editable: true},                                    // 1 OpenAI
	{URL: "https://oa.api2d.net", Editable: true},                                      // 2 API2D
	{URL: "", Editable: true},                                                          // 3 Azure - user must provide endpoint
	{URL: "https://api.closeai-proxy.xyz", Editable: true},                             // 4 CloseAI
	{URL: "https://api.openai-sb.com", Editable: true},                                 // 5 OpenAISB
	{URL: "https://api.openaimax.com", Editable: true},                                 // 6 OpenAIMax
	{URL: "https://api.ohmygpt.com", Editable: true},                                   // 7 OhMyGPT
	{URL: "", Editable: true},                                                          // 8 Custom
	{URL: "https://api.caipacity.com", Editable: true},                                 // 9 Ails
	{URL: "https://api.aiproxy.io", Editable: true},                                    // 10 AIProxy
	{URL: "https://generativelanguage.googleapis.com", Editable: false},                // 11 PaLM
	{URL: "https://api.api2gpt.com", Editable: true},                                   // 12 API2GPT
	{URL: "https://api.aigc2d.com", Editable: true},                                    // 13 AIGC2D
	{URL: "https://api.anthropic.com", Editable: true},                                 // 14 Anthropic
	{URL: "https://aip.baidubce.com", Editable: false},                                 // 15 Baidu
	{URL: "https://open.bigmodel.cn", Editable: false},                                 // 16 Zhipu
	{URL: "https://dashscope.aliyuncs.com", Editable: false},                           // 17 Ali
	{URL: "", Editable: false},                                                         // 18 Xunfei
	{URL: "https://ai.360.cn", Editable: false},                                        // 19 AI360
	{URL: "https://openrouter.ai/api", Editable: true},                                 // 20 OpenRouter
	{URL: "https://api.aiproxy.io", Editable: true},                                    // 21 AIProxyLibrary
	{URL: "https://fastgpt.run/api/openapi", Editable: true},                           // 22 FastGPT
	{URL: "https://hunyuan.tencentcloudapi.com", Editable: false},                      // 23 Tencent
	{URL: "https://generativelanguage.googleapis.com", Editable: false},                // 24 Gemini
	{URL: "https://api.moonshot.cn", Editable: false},                                  // 25 Moonshot
	{URL: "https://api.baichuan-ai.com", Editable: false},                              // 26 Baichuan
	{URL: "https://api.minimax.chat", Editable: false},                                 // 27 Minimax
	{URL: "https://api.mistral.ai", Editable: false},                                   // 28 Mistral
	{URL: "https://api.groq.com/openai", Editable: false},                              // 29 Groq
	{URL: "http://localhost:11434", Editable: true},                                    // 30 Ollama - often self-hosted
	{URL: "https://api.lingyiwanwu.com", Editable: false},                              // 31 LingYiWanWu
	{URL: "https://api.stepfun.com", Editable: false},                                  // 32 StepFun
	{URL: "", Editable: false},                                                         // 33 AwsClaude - region-based
	{URL: "https://api.coze.com", Editable: true},                                      // 34 Coze
	{URL: "https://api.cohere.ai", Editable: false},                                    // 35 Cohere
	{URL: "https://api.deepseek.com", Editable: false},                                 // 36 DeepSeek
	{URL: "https://api.cloudflare.com", Editable: false},                               // 37 Cloudflare
	{URL: "https://api-free.deepl.com", Editable: true},                                // 38 DeepL
	{URL: "https://api.together.xyz", Editable: false},                                 // 39 TogetherAI
	{URL: "https://ark.cn-beijing.volces.com", Editable: true},                         // 40 Doubao
	{URL: "https://api.novita.ai/v3/openai", Editable: false},                          // 41 Novita
	{URL: "", Editable: false},                                                         // 42 VertextAI - region-based
	{URL: "", Editable: true},                                                          // 43 Proxy
	{URL: "https://api.siliconflow.cn", Editable: false},                               // 44 SiliconFlow
	{URL: "https://api.x.ai", Editable: false},                                         // 45 XAI
	{URL: "https://api.replicate.com/v1/models/", Editable: false},                     // 46 Replicate
	{URL: "https://qianfan.baidubce.com", Editable: false},                             // 47 BaiduV2
	{URL: "https://spark-api-open.xf-yun.com", Editable: false},                        // 48 XunfeiV2
	{URL: "https://dashscope.aliyuncs.com", Editable: false},                           // 49 AliBailian
	{URL: "", Editable: true},                                                          // 50 OpenAICompatible - user must provide
	{URL: "https://generativelanguage.googleapis.com/v1beta/openai/", Editable: false}, // 51 GeminiOpenAICompatible
	{URL: "", Editable: true},                                                          // 52 ClaudeCompatible - user must provide
}

// ChannelBaseURLs provides backward compatibility by returning only the URL strings.
// Deprecated: Use ChannelBaseURLConfigs for full configuration.
var ChannelBaseURLs = func() []string {
	urls := make([]string, len(ChannelBaseURLConfigs))
	for i, cfg := range ChannelBaseURLConfigs {
		urls[i] = cfg.URL
	}
	return urls
}()

// GetChannelBaseURLConfig returns the base URL configuration for a channel type.
// Returns a zero value if the channel type is out of range.
func GetChannelBaseURLConfig(channelType int) ChannelBaseURLConfig {
	if channelType < 0 || channelType >= len(ChannelBaseURLConfigs) {
		return ChannelBaseURLConfig{}
	}
	return ChannelBaseURLConfigs[channelType]
}

func init() {
	if len(ChannelBaseURLConfigs) != Dummy {
		panic("channel base url configs length not match")
	}
}
