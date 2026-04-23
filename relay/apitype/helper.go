package apitype

func String(apiType int) string {
	switch apiType {
	case OpenAI:
		return "openai"
	case Anthropic:
		return "anthropic"
	case PaLM:
		return "palm"
	case Baidu:
		return "baidu"
	case Zhipu:
		return "zhipu"
	case Ali:
		return "ali"
	case Xunfei:
		return "xunfei"
	case AIProxyLibrary:
		return "aiproxy_library"
	case Tencent:
		return "tencent"
	case Gemini:
		return "gemini"
	case Ollama:
		return "ollama"
	case AwsClaude:
		return "aws_claude"
	case Coze:
		return "coze"
	case Cohere:
		return "cohere"
	case Cloudflare:
		return "cloudflare"
	case DeepL:
		return "deepl"
	case VertexAI:
		return "vertex_ai"
	case Proxy:
		return "proxy"
	case Replicate:
		return "replicate"
	case DeepSeek:
		return "deepseek"
	case Groq:
		return "groq"
	case Mistral:
		return "mistral"
	case Moonshot:
		return "moonshot"
	case XAI:
		return "xai"
	case OpenRouter:
		return "openrouter"
	case Copilot:
		return "copilot"
	case Fireworks:
		return "fireworks"
	default:
		return ""
	}
}
