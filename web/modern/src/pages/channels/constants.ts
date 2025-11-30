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
  { key: 1, text: "OpenAI", value: 1, color: "green" },
  {
    key: 50,
    text: "OpenAI Compatible",
    value: 50,
    color: "olive",
    description: "OpenAI compatible channel, supports custom Base URL",
  },
  { key: 14, text: "Anthropic", value: 14, color: "black" },
  { key: 33, text: "AWS", value: 33, color: "black" },
  { key: 3, text: "Azure", value: 3, color: "olive" },
  { key: 11, text: "PaLM2", value: 11, color: "orange" },
  { key: 24, text: "Gemini", value: 24, color: "orange" },
  {
    key: 51,
    text: "Gemini (OpenAI)",
    value: 51,
    color: "orange",
    description: "Gemini OpenAI compatible format",
  },
  { key: 28, text: "Mistral AI", value: 28, color: "orange" },
  { key: 41, text: "Novita", value: 41, color: "purple" },
  {
    key: 40,
    text: "ByteDance Volcano Engine",
    value: 40,
    color: "blue",
    description: "Formerly ByteDance Doubao",
  },
  {
    key: 15,
    text: "Baidu Wenxin Qianfan",
    value: 15,
    color: "blue",
    tip: "Get AK (API Key) and SK (Secret Key) from Baidu console",
  },
  {
    key: 47,
    text: "Baidu Wenxin Qianfan V2",
    value: 47,
    color: "blue",
    tip: "For V2 inference service, get API Key from Baidu IAM",
  },
  { key: 17, text: "Alibaba Tongyi Qianwen", value: 17, color: "orange" },
  { key: 49, text: "Alibaba Cloud Bailian", value: 49, color: "orange" },
  {
    key: 18,
    text: "iFlytek Spark Cognition",
    value: 18,
    color: "blue",
    tip: "WebSocket version API",
  },
  {
    key: 48,
    text: "iFlytek Spark Cognition V2",
    value: 48,
    color: "blue",
    tip: "HTTP version API",
  },
  { key: 16, text: "Zhipu ChatGLM", value: 16, color: "violet" },
  { key: 19, text: "360 ZhiNao", value: 19, color: "blue" },
  { key: 25, text: "Moonshot AI", value: 25, color: "black" },
  { key: 23, text: "Tencent Hunyuan", value: 23, color: "teal" },
  { key: 26, text: "Baichuan Model", value: 26, color: "orange" },
  { key: 27, text: "MiniMax", value: 27, color: "red" },
  { key: 29, text: "Groq", value: 29, color: "orange" },
  { key: 30, text: "Ollama", value: 30, color: "black" },
  { key: 31, text: "01.AI", value: 31, color: "green" },
  { key: 32, text: "StepFun", value: 32, color: "blue" },
  { key: 34, text: "Coze", value: 34, color: "blue" },
  { key: 35, text: "Cohere", value: 35, color: "blue" },
  { key: 36, text: "DeepSeek", value: 36, color: "black" },
  { key: 37, text: "Cloudflare", value: 37, color: "orange" },
  { key: 38, text: "DeepL", value: 38, color: "black" },
  { key: 39, text: "together.ai", value: 39, color: "blue" },
  { key: 42, text: "VertexAI", value: 42, color: "blue" },
  { key: 43, text: "Proxy", value: 43, color: "blue" },
  { key: 44, text: "SiliconFlow", value: 44, color: "blue" },
  { key: 45, text: "xAI", value: 45, color: "blue" },
  { key: 46, text: "Replicate", value: 46, color: "blue" },
  { key: 22, text: "Knowledge Base: FastGPT", value: 22, color: "blue" },
  { key: 21, text: "Knowledge Base: AI Proxy", value: 21, color: "purple" },
  { key: 20, text: "OpenRouter", value: 20, color: "black" },
  { key: 2, text: "Proxy: API2D", value: 2, color: "blue" },
  { key: 5, text: "Proxy: OpenAI-SB", value: 5, color: "brown" },
  { key: 7, text: "Proxy: OhMyGPT", value: 7, color: "purple" },
  { key: 10, text: "Proxy: AI Proxy", value: 10, color: "purple" },
  { key: 4, text: "Proxy: CloseAI", value: 4, color: "teal" },
  { key: 6, text: "Proxy: OpenAI Max", value: 6, color: "violet" },
  { key: 9, text: "Proxy: AI.LS", value: 9, color: "yellow" },
  { key: 12, text: "Proxy: API2GPT", value: 12, color: "blue" },
  { key: 13, text: "Proxy: AIGC2D", value: 13, color: "purple" },
];

export const CHANNEL_TYPES_WITH_DEDICATED_BASE_URL = new Set<number>([3, 50]);
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
