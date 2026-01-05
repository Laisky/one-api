export interface ChannelType {
  key: number;
  text: string;
  value: number;
  color?: string;
  tip?: string;
  description?: string;
}

export interface Model {
  id: string;
  name: string;
}

export const CHANNEL_TYPES: ChannelType[] = [
  {
    key: 1,
    text: "OpenAI",
    value: 1,
    color: "green",
    description:
      "Direct OpenAI API; supports chat, images, audio, and Response API surfaces.",
  },
  {
    key: 50,
    text: "OpenAI Compatible",
    value: 50,
    color: "olive",
    description:
      "Custom api_base; OpenAI-style API with ChatCompletion or Response API payloads.",
  },
  {
    key: 52,
    text: "Claude Compatible",
    value: 52,
    color: "black",
    description:
      "Custom Claude Messages api_base; converts Chat/Response requests to Claude format upstream.",
  },
  {
    key: 53,
    text: "GitHub Copilot",
    value: 53,
    color: "black",
    description:
      "GitHub Copilot gateway; exchanges a GitHub access token for a short-lived Copilot API token and forwards OpenAI-style requests.",
  },
  {
    key: 14,
    text: "Anthropic",
    value: 14,
    color: "black",
    description: "Native Anthropic Claude Messages API.",
  },
  {
    key: 33,
    text: "AWS",
    value: 33,
    color: "black",
    description:
      "AWS Bedrock deployments; configure AK/SK, region, or inference profiles.",
  },
  {
    key: 3,
    text: "Azure",
    value: 3,
    color: "olive",
    description:
      "Azure OpenAI deployments; requires resource endpoint and API version.",
  },
  {
    key: 11,
    text: "PaLM2",
    value: 11,
    color: "orange",
    description: "Legacy Google PaLM2 text/chat endpoint.",
  },
  {
    key: 24,
    text: "Gemini",
    value: 24,
    color: "orange",
    description: "Native Gemini Generative Language API.",
  },
  {
    key: 51,
    text: "Gemini (OpenAI)",
    value: 51,
    color: "orange",
    description: "Gemini served through Google's OpenAI-compatible surface.",
  },
  {
    key: 28,
    text: "Mistral AI",
    value: 28,
    color: "orange",
    description: "Native Mistral platform API for chat and embeddings.",
  },
  {
    key: 41,
    text: "Novita",
    value: 41,
    color: "purple",
    description: "Novita AI OpenAI-compatible hosting.",
  },
  {
    key: 40,
    text: "ByteDance Volcano Engine",
    value: 40,
    color: "blue",
    description: "Volcano Engine (Doubao) OpenAI-style endpoints.",
  },
  {
    key: 15,
    text: "Baidu Wenxin Qianfan",
    value: 15,
    color: "blue",
    tip: "Get AK (API Key) and SK (Secret Key) from Baidu console",
    description: "Baidu Qianfan v1 (AK/SK) chat models.",
  },
  {
    key: 47,
    text: "Baidu Wenxin Qianfan V2",
    value: 47,
    color: "blue",
    tip: "For V2 inference service, get API Key from Baidu IAM",
    description: "Baidu Qianfan v2 IAM-based inference service.",
  },
  {
    key: 17,
    text: "Alibaba Tongyi Qianwen",
    value: 17,
    color: "orange",
    description: "DashScope Tongyi Qianwen API.",
  },
  {
    key: 49,
    text: "Alibaba Cloud Bailian",
    value: 49,
    color: "orange",
    description: "Alibaba Cloud Bailian enterprise Qwen service.",
  },
  {
    key: 18,
    text: "iFlytek Spark Cognition",
    value: 18,
    color: "blue",
    tip: "WebSocket version API",
    description: "Spark cognition WebSocket chat API.",
  },
  {
    key: 48,
    text: "iFlytek Spark Cognition V2",
    value: 48,
    color: "blue",
    tip: "HTTP version API",
    description: "Spark cognition HTTP chat API.",
  },
  {
    key: 16,
    text: "Zhipu ChatGLM",
    value: 16,
    color: "violet",
    description: "Zhipu GLM native API.",
  },
  {
    key: 19,
    text: "360 ZhiNao",
    value: 19,
    color: "blue",
    description: "360 ZhiNao conversational API.",
  },
  {
    key: 25,
    text: "Moonshot AI",
    value: 25,
    color: "black",
    description: "Moonshot/Kimi native chat models.",
  },
  {
    key: 23,
    text: "Tencent Hunyuan",
    value: 23,
    color: "teal",
    description: "Tencent Hunyuan generative models.",
  },
  {
    key: 26,
    text: "Baichuan Model",
    value: 26,
    color: "orange",
    description: "Baichuan chat/completion API.",
  },
  {
    key: 27,
    text: "MiniMax",
    value: 27,
    color: "red",
    description: "MiniMax text/chat and embeddings.",
  },
  {
    key: 29,
    text: "Groq",
    value: 29,
    color: "orange",
    description: "Groq-hosted LLaMA-style models via OpenAI schema.",
  },
  {
    key: 30,
    text: "Ollama",
    value: 30,
    color: "black",
    description: "Local Ollama host; routes to self-hosted models.",
  },
  {
    key: 31,
    text: "01.AI",
    value: 31,
    color: "green",
    description: "01.AI Yi model API (Lingyi Wanwu).",
  },
  {
    key: 32,
    text: "StepFun",
    value: 32,
    color: "blue",
    description: "StepFun open platform chat API.",
  },
  {
    key: 34,
    text: "Coze",
    value: 34,
    color: "blue",
    description: "Coze bot runtime; supports personal token or OAuth JWT.",
  },
  {
    key: 35,
    text: "Cohere",
    value: 35,
    color: "blue",
    description: "Cohere Generate and Rerank APIs.",
  },
  {
    key: 36,
    text: "DeepSeek",
    value: 36,
    color: "black",
    description: "DeepSeek native API with thinking outputs.",
  },
  {
    key: 37,
    text: "Cloudflare",
    value: 37,
    color: "orange",
    description: "Cloudflare Workers AI/AI Gateway OpenAI-compatible routes.",
  },
  {
    key: 38,
    text: "DeepL",
    value: 38,
    color: "black",
    description: "DeepL text translation API.",
  },
  {
    key: 39,
    text: "together.ai",
    value: 39,
    color: "blue",
    description: "Together AI OpenAI-compatible model hub.",
  },
  {
    key: 42,
    text: "VertexAI",
    value: 42,
    color: "blue",
    description: "Google Vertex AI models via region and project settings.",
  },
  {
    key: 43,
    text: "Proxy",
    value: 43,
    color: "blue",
    description: "Transparent passthrough proxy for arbitrary endpoints.",
  },
  {
    key: 44,
    text: "SiliconFlow",
    value: 44,
    color: "blue",
    description: "SiliconFlow OpenAI-style API.",
  },
  {
    key: 45,
    text: "xAI",
    value: 45,
    color: "blue",
    description: "xAI Grok models over OpenAI-style API.",
  },
  {
    key: 46,
    text: "Replicate",
    value: 46,
    color: "blue",
    description: "Replicate model inference by model slug.",
  },
  {
    key: 22,
    text: "Knowledge Base: FastGPT",
    value: 22,
    color: "blue",
    description: "FastGPT knowledge-base retrieval channel.",
  },
  {
    key: 21,
    text: "Knowledge Base: AI Proxy",
    value: 21,
    color: "purple",
    description: "AI Proxy knowledge-base retrieval channel.",
  },
  {
    key: 20,
    text: "OpenRouter",
    value: 20,
    color: "black",
    description: "OpenRouter aggregated model marketplace.",
  },
  {
    key: 2,
    text: "Proxy: API2D",
    value: 2,
    color: "blue",
    description: "API2D OpenAI proxy service.",
  },
  {
    key: 5,
    text: "Proxy: OpenAI-SB",
    value: 5,
    color: "brown",
    description: "OpenAI-SB proxy service.",
  },
  {
    key: 7,
    text: "Proxy: OhMyGPT",
    value: 7,
    color: "purple",
    description: "OhMyGPT proxy service.",
  },
  {
    key: 10,
    text: "Proxy: AI Proxy",
    value: 10,
    color: "purple",
    description: "aiproxy.io proxy service.",
  },
  {
    key: 4,
    text: "Proxy: CloseAI",
    value: 4,
    color: "teal",
    description: "CloseAI proxy service.",
  },
  {
    key: 6,
    text: "Proxy: OpenAI Max",
    value: 6,
    color: "violet",
    description: "OpenAI Max proxy service.",
  },
  {
    key: 9,
    text: "Proxy: AI.LS",
    value: 9,
    color: "yellow",
    description: "AI.LS proxy service.",
  },
  {
    key: 12,
    text: "Proxy: API2GPT",
    value: 12,
    color: "blue",
    description: "API2GPT proxy service.",
  },
  {
    key: 13,
    text: "Proxy: AIGC2D",
    value: 13,
    color: "purple",
    description: "AIGC2D proxy service.",
  },
];

export const CHANNEL_TYPES_WITH_DEDICATED_BASE_URL = new Set<number>([
  3, 50, 52,
]);
export const CHANNEL_TYPES_WITH_CUSTOM_KEY_FIELD = new Set<number>([34]);

export const OPENAI_COMPATIBLE_API_FORMAT_OPTIONS = [
  { value: "chat_completion", label: "ChatCompletion (default)" },
  { value: "response", label: "Response" },
];

export const COZE_AUTH_OPTIONS = [
  {
    key: "personal_access_token",
    text: "Personal Access Token",
    value: "personal_access_token",
  },
  { key: "oauth_jwt", text: "OAuth JWT", value: "oauth_jwt" },
];

export const MODEL_MAPPING_EXAMPLE = {
  "gpt-3.5-turbo-0301": "gpt-3.5-turbo",
  "gpt-4-0314": "gpt-4",
  "gpt-4-32k-0314": "gpt-4-32k",
};

export const MODEL_CONFIGS_EXAMPLE = {
  "gpt-3.5-turbo-0301": {
    ratio: 0.0015,
    completion_ratio: 2.0,
    max_tokens: 65536,
  },
  "gpt-4": {
    ratio: 0.03,
    completion_ratio: 2.0,
    max_tokens: 128000,
  },
} satisfies Record<string, Record<string, unknown>>;

export const TOOLING_CONFIG_EXAMPLE = {
  whitelist: ["web_search"],
  pricing: {
    web_search: {
      usd_per_call: 0.025,
    },
  },
} satisfies Record<string, unknown>;

export const OAUTH_JWT_CONFIG_EXAMPLE = {
  client_type: "jwt",
  client_id: "123456789",
  coze_www_base: "https://www.coze.cn",
  coze_api_base: "https://api.coze.cn",
  private_key: "-----BEGIN PRIVATE KEY-----\n***\n-----END PRIVATE KEY-----",
  public_key_id: "***********************************************************",
};

export const INFERENCE_PROFILE_ARN_MAP_EXAMPLE = {
  "anthropic.claude-3-5-sonnet-20240620-v1:0":
    "arn:aws:bedrock:us-east-1:123456789012:inference-profile/us.anthropic.claude-3-5-sonnet-20240620-v1:0",
};
