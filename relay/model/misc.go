package model

import "github.com/Laisky/one-api/relay/realtime"

// Usage is the token usage information returned by OpenAI API.
type Usage struct {
	// BilledSearchUnits is an adaptor-validated non-token receipt. Nil means that
	// no valid search-unit count was received; public JSON cannot set this field.
	BilledSearchUnits *int64 `json:"-"`
	// BillingEstimateReason is server-only evidence that usage was conservatively
	// estimated rather than measured. Clients cannot supply it through JSON.
	BillingEstimateReason string `json:"-"`
	// BillingEstimateQuotaFloor retains the original quote for accepted but
	// unmeasured work, independently of whether admission took a physical hold.
	// This is server-only evidence, never a client-controlled pricing dimension.
	BillingEstimateQuotaFloor int64 `json:"-"`
	// Realtime is server-only accounting evidence; clients cannot inject it via JSON.
	Realtime *realtime.Ledger `json:"-"`
	// Omitting this field using 'omitempty' is crucial to avoid returning zero values
	// when conversion mechanisms are not employed, particularly in scenarios like image generation.
	//
	// TODO: With Go 1.24 ~ latest potentially supporting 'omitzero', do we need to use both 'omitempty' and 'omitzero' here?
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
	// CachedTokens captures a TOP-LEVEL cached-token count reported by some
	// providers (e.g. StepFun) directly under usage rather than nested inside
	// prompt_tokens_details. OpenAI and most providers omit this and instead
	// populate PromptTokensDetails.CachedTokens. Use NormalizeCachedTokens to
	// promote this value into the nested field so downstream billing only has
	// to read one location.
	CachedTokens int `json:"cached_tokens,omitempty"`
	// PromptCacheHitTokens and PromptCacheMissTokens capture DeepSeek's
	// provider-specific prompt-cache usage fields before normalization.
	PromptCacheHitTokens  int `json:"prompt_cache_hit_tokens,omitempty"`
	PromptCacheMissTokens int `json:"prompt_cache_miss_tokens,omitempty"`
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
	CacheWriteTokens   int `json:"cache_write_tokens,omitempty"`
	CacheWrite5mTokens int `json:"cache_write_5m_tokens,omitempty"`
	CacheWrite1hTokens int `json:"cache_write_1h_tokens,omitempty"`
	// CacheWrite12hTokens is an observed paid OpenAI retention class, not a
	// legacy 5m/30m or Anthropic 1h write. Pricing must explicitly handle it.
	CacheWrite12hTokens int `json:"cache_write_12h_tokens,omitempty"`
}

// NormalizeCachedTokens promotes top-level and DeepSeek provider-specific cache-hit
// counts into PromptTokensDetails.CachedTokens, the single location quota billing reads.
//
// It is a no-op when neither source has a cache-hit count, and it never overwrites
// a nested count that an upstream provider already populated (such providers are
// authoritative). After promotion, provider-specific fields are cleared so
// OpenAI-shaped responses expose only the standard prompt_tokens_details.cached_tokens field.
func (u *Usage) NormalizeCachedTokens() {
	if u == nil {
		return
	}

	cachedTokens := u.CachedTokens
	if u.PromptCacheHitTokens > 0 {
		if cachedTokens == 0 {
			cachedTokens = u.PromptCacheHitTokens
		}
		u.PromptCacheHitTokens = 0
	}
	u.PromptCacheMissTokens = 0
	if cachedTokens <= 0 {
		return
	}
	if u.PromptTokensDetails == nil {
		u.PromptTokensDetails = &UsagePromptTokensDetails{}
	}
	if u.PromptTokensDetails.CachedTokens == 0 {
		u.PromptTokensDetails.CachedTokens = cachedTokens
	}
	u.CachedTokens = 0
}

// NormalizeCacheWriteTokens translates overlapping OpenAI Responses aggregate
// and per-lifetime cache-write receipts to distinct settlement buckets. Legacy
// top-level-only providers retain their historical additive bucket semantics.
// The gateway does not yet price 12h writes: the distinct 12h bucket must
// trigger conservative unpriced-use settlement rather than a 30m charge.
func (u *Usage) NormalizeCacheWriteTokens() {
	if u == nil {
		return
	}

	if details := u.PromptTokensDetails; details != nil &&
		(details.CacheWriteTokens > 0 || details.CacheWriteTokensDetails != nil) {
		total := max(0, u.CacheWriteTokens, details.CacheWriteTokens)
		if ttl := details.CacheWriteTokensDetails; ttl != nil {
			// Per-TTL counters partition the aggregate. Never include 12h
			// writes in the ordinary 30m bucket, even if the receipt also
			// supplies an aggregate alias for the same tokens.
			writes12h := max(0, ttl.TTL12hTokens)
			u.CacheWrite12hTokens = max(u.CacheWrite12hTokens, writes12h)
			total = max(0, ttl.TTL30mTokens, total-writes12h)
		}
		u.CacheWrite5mTokens = max(u.CacheWrite5mTokens, total)
		u.CacheWriteTokens = 0
		return
	}
	if u.CacheWriteTokens <= 0 {
		return
	}
	u.CacheWrite5mTokens += u.CacheWriteTokens
	u.CacheWriteTokens = 0
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
	// ErrorTypeBaidu represents errors returned by Baidu Wenxin endpoints.
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
	// MessageContainsRawBody identifies complete-body parser fallbacks for explicit application log redaction; API JSON stays unchanged.
	MessageContainsRawBody bool `json:"-"`
	// RawError preserves the original upstream or internal error for diagnostics.
	// Omitted from JSON to avoid leaking provider internals.
	RawError error `json:"-"`
}

type ErrorWithStatusCode struct {
	Error
	StatusCode int `json:"status_code"`
}

// UsageCacheWriteTokensDetails records mutually exclusive OpenAI Responses cache
// write lifetimes. The one-api quota meter currently supports the default 30m
// bucket; explicit 12h writes require a separate tariff and admission contract.
type UsageCacheWriteTokensDetails struct {
	TTL30mTokens int `json:"ttl_30m_tokens,omitempty"`
	TTL12hTokens int `json:"ttl_12h_tokens,omitempty"`
}

// UsagePromptTokensDetails contains details about the prompt tokens used in a request.
type UsagePromptTokensDetails struct {
	CachedTokensDetails     *UsageCachedTokensDetails     `json:"cached_tokens_details,omitempty"`
	CachedTokens            int                           `json:"cached_tokens"`
	CacheWriteTokens        int                           `json:"cache_write_tokens,omitempty"`
	CacheWriteTokensDetails *UsageCacheWriteTokensDetails `json:"cache_write_tokens_details,omitempty"`
	AudioTokens             int                           `json:"audio_tokens"`
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
