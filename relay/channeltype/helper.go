package channeltype

import "github.com/songquanpeng/one-api/relay/apitype"

func ToAPIType(channelType int) int {
	apiType := apitype.OpenAI
	switch channelType {
	case Anthropic:
		apiType = apitype.Anthropic
	case ClaudeCompatible:
		apiType = apitype.Anthropic
	case Baidu:
		apiType = apitype.Baidu
	case PaLM:
		apiType = apitype.PaLM
	case Zhipu:
		apiType = apitype.Zhipu
	case Ali:
		apiType = apitype.Ali
	case Xunfei:
		apiType = apitype.Xunfei
	case AIProxyLibrary:
		apiType = apitype.AIProxyLibrary
	case Tencent:
		apiType = apitype.Tencent
	case Gemini:
		apiType = apitype.Gemini
	case Ollama:
		apiType = apitype.Ollama
	case AwsClaude:
		apiType = apitype.AwsClaude
	case Coze:
		apiType = apitype.Coze
	case Cohere:
		apiType = apitype.Cohere
	case Cloudflare:
		apiType = apitype.Cloudflare
	case DeepL:
		apiType = apitype.DeepL
	case VertextAI:
		apiType = apitype.VertexAI
	case Replicate:
		apiType = apitype.Replicate
	case Proxy:
		apiType = apitype.Proxy
	case DeepSeek:
		apiType = apitype.DeepSeek
	case Groq:
		apiType = apitype.Groq
	case Mistral:
		apiType = apitype.Mistral
	case Moonshot:
		apiType = apitype.Moonshot
	case XAI:
		apiType = apitype.XAI
	}

	return apiType
}

// IsOpenAICompatible reports whether the provided channel type should use the OpenAI-compatible
// adaptor stack. Legacy custom channels (type 8) are treated as OpenAI-compatible to preserve
// backward compatibility after the migration to the unified type.
func IsOpenAICompatible(channelType int) bool {
	return channelType == OpenAICompatible || channelType == Custom
}

// IdToName converts channel type ID to readable name
func IdToName(channelType int) string {
	switch channelType {
	case Unknown:
		return "unknown"
	case OpenAI:
		return "openai"
	case API2D:
		return "api2d"
	case Azure:
		return "azure"
	case CloseAI:
		return "closeai"
	case OpenAISB:
		return "openaisb"
	case OpenAIMax:
		return "openaimax"
	case OhMyGPT:
		return "ohmygpt"
	case Custom:
		return "custom"
	case Ails:
		return "ails"
	case AIProxy:
		return "aiproxy"
	case PaLM:
		return "palm"
	case API2GPT:
		return "api2gpt"
	case AIGC2D:
		return "aigc2d"
	case Anthropic:
		return "anthropic"
	case ClaudeCompatible:
		return "claudecompatible"
	case Baidu:
		return "baidu"
	case Zhipu:
		return "zhipu"
	case Ali:
		return "ali"
	case Xunfei:
		return "xunfei"
	case AI360:
		return "ai360"
	case OpenRouter:
		return "openrouter"
	case AIProxyLibrary:
		return "aiproxylibrary"
	case FastGPT:
		return "fastgpt"
	case Tencent:
		return "tencent"
	case Gemini:
		return "gemini"
	case Moonshot:
		return "moonshot"
	case Baichuan:
		return "baichuan"
	case Minimax:
		return "minimax"
	case Mistral:
		return "mistral"
	case Groq:
		return "groq"
	case Ollama:
		return "ollama"
	case LingYiWanWu:
		return "lingyiwanwu"
	case StepFun:
		return "stepfun"
	case AwsClaude:
		return "awsclaude"
	case Coze:
		return "coze"
	case Cohere:
		return "cohere"
	case DeepSeek:
		return "deepseek"
	case Cloudflare:
		return "cloudflare"
	case DeepL:
		return "deepl"
	case TogetherAI:
		return "togetherai"
	case Doubao:
		return "doubao"
	case Novita:
		return "novita"
	case VertextAI:
		return "vertextai"
	case Proxy:
		return "proxy"
	case SiliconFlow:
		return "siliconflow"
	case XAI:
		return "xai"
	case Replicate:
		return "replicate"
	case BaiduV2:
		return "baiduv2"
	case XunfeiV2:
		return "xunfeiv2"
	case AliBailian:
		return "alibailian"
	case OpenAICompatible:
		return "openaicompatible"
	case GeminiOpenAICompatible:
		return "geminiopenaicompatible"
	case Dummy:
		return "dummy"
	default:
		return "unknown"
	}
}
