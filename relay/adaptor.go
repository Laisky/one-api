package relay

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/aiproxy"
	"github.com/Laisky/one-api/relay/adaptor/ali"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/adaptor/aws"
	"github.com/Laisky/one-api/relay/adaptor/baidu"
	"github.com/Laisky/one-api/relay/adaptor/cloudflare"
	"github.com/Laisky/one-api/relay/adaptor/cohere"
	"github.com/Laisky/one-api/relay/adaptor/copilot"
	"github.com/Laisky/one-api/relay/adaptor/coze"
	"github.com/Laisky/one-api/relay/adaptor/deepl"
	"github.com/Laisky/one-api/relay/adaptor/deepseek"
	"github.com/Laisky/one-api/relay/adaptor/fireworks"
	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/adaptor/groq"
	"github.com/Laisky/one-api/relay/adaptor/mistral"
	"github.com/Laisky/one-api/relay/adaptor/moonshot"
	"github.com/Laisky/one-api/relay/adaptor/ollama"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openrouter"
	"github.com/Laisky/one-api/relay/adaptor/palm"
	"github.com/Laisky/one-api/relay/adaptor/proxy"
	"github.com/Laisky/one-api/relay/adaptor/replicate"
	"github.com/Laisky/one-api/relay/adaptor/tencent"
	"github.com/Laisky/one-api/relay/adaptor/vertexai"
	"github.com/Laisky/one-api/relay/adaptor/xai"
	"github.com/Laisky/one-api/relay/adaptor/xunfei"
	"github.com/Laisky/one-api/relay/adaptor/zhipu"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/pricing"
)

func GetAdaptor(apiType int) adaptor.Adaptor {
	switch apiType {
	case apitype.AIProxyLibrary:
		return &aiproxy.Adaptor{}
	case apitype.Ali:
		return &ali.Adaptor{}
	case apitype.Anthropic:
		return &anthropic.Adaptor{}
	case apitype.AwsClaude:
		return &aws.Adaptor{}
	case apitype.Baidu:
		return &baidu.Adaptor{}
	case apitype.Gemini:
		return &gemini.Adaptor{}
	case apitype.OpenAI:
		return &openai.Adaptor{}
	case apitype.PaLM:
		return &palm.Adaptor{}
	case apitype.Tencent:
		return &tencent.Adaptor{}
	case apitype.Xunfei:
		return &xunfei.Adaptor{}
	case apitype.Zhipu:
		return &zhipu.Adaptor{}
	case apitype.Ollama:
		return &ollama.Adaptor{}
	case apitype.Coze:
		return &coze.Adaptor{}
	case apitype.Cohere:
		return &cohere.Adaptor{}
	case apitype.Cloudflare:
		return &cloudflare.Adaptor{}
	case apitype.DeepL:
		return &deepl.Adaptor{}
	case apitype.VertexAI:
		return &vertexai.Adaptor{}
	case apitype.Proxy:
		return &proxy.Adaptor{}
	case apitype.Replicate:
		return &replicate.Adaptor{}
	case apitype.DeepSeek:
		return &deepseek.Adaptor{}
	case apitype.Groq:
		return &groq.Adaptor{}
	case apitype.Mistral:
		return &mistral.Adaptor{}
	case apitype.Moonshot:
		return &moonshot.Adaptor{}
	case apitype.XAI:
		return &xai.Adaptor{}
	case apitype.OpenRouter:
		return &openrouter.Adaptor{}
	case apitype.Copilot:
		return &copilot.Adaptor{}
	case apitype.Fireworks:
		return &fireworks.Adaptor{}
	}

	return nil
}

// InitializeGlobalPricing initializes the global pricing manager with the GetAdaptor function
func InitializeGlobalPricing() {
	pricing.InitializeGlobalPricingManager(GetAdaptor)
}
