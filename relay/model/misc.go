package model

// Usage is the token usage information returned by OpenAI API.
type Usage struct {
	// Omitting this field using 'omitempty' is crucial to avoid returning zero values
	// when conversion mechanisms are not employed, particularly in scenarios like image generation.
	//
	// TODO: With Go 1.24 ~ latest potentially supporting 'omitzero', do we need to use both 'omitempty' and 'omitzero' here?
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
	// PromptTokensDetails may be empty for some models
	PromptTokensDetails *UsagePromptTokensDetails `json:"prompt_tokens_details,omitempty"`
	// CompletionTokensDetails may be empty for some models
	CompletionTokensDetails *UsageCompletionTokensDetails `json:"completion_tokens_details,omitempty"`
	ServiceTier             string                        `json:"service_tier,omitempty"`
	SystemFingerprint       string                        `json:"system_fingerprint,omitempty"`

	// -------------------------------------
	// Custom fields
	// -------------------------------------
	// ToolsCost is the cost of using tools, in quota.
	ToolsCost int64 `json:"tools_cost,omitempty"`

	// Cache write token details (Anthropic Claude prompt caching)
	// These fields capture how many input tokens were charged as cache creation writes.
	// They are optional and only set when providers return such details.
	CacheWrite5mTokens int `json:"cache_write_5m_tokens,omitempty"`
	CacheWrite1hTokens int `json:"cache_write_1h_tokens,omitempty"`
}

// ErrorType enumerates the standardized error categories we expose to clients.
// Keeping them centralized avoids magic strings across the codebase.
type ErrorType string

const (
	// ErrorTypeUnknown is the default zero value when no error type is provided by upstream.
	ErrorTypeUnknown ErrorType = ""
	// ErrorTypeUpstream indicates the upstream provider returned an unexpected payload.
	ErrorTypeUpstream ErrorType = "upstream_error"
	// ErrorTypeInternal represents failures originating inside one-api infrastructure.
	ErrorTypeInternal ErrorType = "internal_error"
	// ErrorTypeServer indicates a generic 5xx-style server failure surfaced to clients.
	ErrorTypeServer ErrorType = "server_error"
	// ErrorTypeOneAPI is used for validation errors raised by one-api before contacting upstream.
	ErrorTypeOneAPI ErrorType = "one_api_error"
	// ErrorTypeInvalidRequest signals that the caller's request payload is malformed.
	ErrorTypeInvalidRequest ErrorType = "invalid_request_error"
	// ErrorTypeAuthentication covers invalid credentials or authentication failures.
	ErrorTypeAuthentication ErrorType = "authentication_error"
	// ErrorTypePermission is returned when the token lacks permission for the requested action.
	ErrorTypePermission ErrorType = "permission_error"
	// ErrorTypeInsufficientQuota indicates the account has run out of quota or credit.
	ErrorTypeInsufficientQuota ErrorType = "insufficient_quota"
	// ErrorTypeForbidden captures policy violations or organization restrictions from providers.
	ErrorTypeForbidden ErrorType = "forbidden"
	// ErrorTypeRateLimit denotes rate limiting responses (typically HTTP 429).
	ErrorTypeRateLimit ErrorType = "rate_limit_error"
	// ErrorTypeNotFound maps to missing resources such as jobs or files.
	ErrorTypeNotFound ErrorType = "not_found_error"
	// ErrorTypeTest is reserved for synthetic failures inside tests.
	ErrorTypeTest ErrorType = "test_error"
	// ErrorTypeAli represents errors emitted by Aliyun DashScope endpoints.
	ErrorTypeAli ErrorType = "ali_error"
	// ErrorTypeBaidu represents errors emitted by Baidu Wenxin endpoints.
	ErrorTypeBaidu ErrorType = "baidu_error"
	// ErrorTypeZhipu represents errors returned by Zhipu/ChatGLM providers.
	ErrorTypeZhipu ErrorType = "zhipu_error"
	// ErrorTypeOllama represents errors returned by local Ollama runtimes.
	ErrorTypeOllama ErrorType = "ollama_error"
	// ErrorTypeGemini represents errors returned by Google Gemini APIs.
	ErrorTypeGemini ErrorType = "gemini_error"
)

type Error struct {
	Message string    `json:"message"`
	Type    ErrorType `json:"type"`
	Param   string    `json:"param"`
	Code    any       `json:"code"`
	// RawError preserves the original upstream or internal error for diagnostics.
	// Omitted from JSON to avoid leaking provider internals.
	RawError error `json:"-"`
}

type ErrorWithStatusCode struct {
	Error
	StatusCode int `json:"status_code"`
}

// UsagePromptTokensDetails contains details about the prompt tokens used in a request.
type UsagePromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
	AudioTokens  int `json:"audio_tokens"`
	// TextTokens could be zero for pure text chats
	TextTokens     int     `json:"text_tokens"`
	ImageTokens    int     `json:"image_tokens"`
	VideoTokens    int     `json:"video_tokens,omitempty"`
	DocumentTokens int     `json:"document_tokens,omitempty"`
	ImageCount     int     `json:"image_count,omitempty"`
	AudioSeconds   float64 `json:"audio_seconds,omitempty"`
	VideoFrames    int     `json:"video_frames,omitempty"`
	DocumentPages  int     `json:"document_pages,omitempty"`
}

// UsageCompletionTokensDetails contains details about the completion tokens used in a request.
type UsageCompletionTokensDetails struct {
	ReasoningTokens          int `json:"reasoning_tokens"`
	AudioTokens              int `json:"audio_tokens"`
	AcceptedPredictionTokens int `json:"accepted_prediction_tokens"`
	RejectedPredictionTokens int `json:"rejected_prediction_tokens"`
	// TextTokens could be zero for pure text chats
	TextTokens int `json:"text_tokens"`
	// CachedTokens indicates the count of completion tokens served from cache
	CachedTokens int `json:"cached_tokens"`
}
