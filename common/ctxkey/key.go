package ctxkey

import "github.com/gin-gonic/gin"

const (
	// Config holds the resolved channel configuration struct (loaded via channel.LoadConfig).
	// Set in: middleware/distributor.SetupContextForSelectedChannel.
	// Read in: relay/meta to embed into Meta and by adaptors that need provider-specific config.
	Config = "config"

	// Id is the authenticated user id for the current request.
	// Set in: middleware/auth (session or token auth).
	// Read widely by controllers for billing, logs, and ownership checks.
	Id = "id"

	// RequestId is a per-request unique identifier (also used for logging/metrics).
	// Set in: middleware/distributor.SetupContextForSelectedChannel (if not already present).
	// Read in: controllers (text/image/audio/claude/proxy/response) for billing trace & logs.
	// Note: the literal value is "X-Oneapi-Request-Id" for consistency with header naming.
	RequestId = "X-Oneapi-Request-Id"

	// Username is the authenticated username (only for session/admin panels & metrics labeling).
	// Set in: middleware/auth (session branch).
	// Read in: e.g. controller/prometheus for user metrics labels.
	Username = "username"

	// Role is the authenticated user role (common/admin/root).
	// Set in: middleware/auth (session branch).
	// Read in: user and admin controllers for permission checks.
	Role = "role"

	// Status is reserved for user status if ever stored on context.
	// Currently not set via middleware (status is checked internally in auth middleware).
	// Kept for compatibility; avoid relying on it.
	Status = "status"

	// ChannelModel holds the selected Channel instance (*model.Channel) used to serve this request.
	// Set in: middleware/distributor after channel selection (by model/group/priority or explicit id).
	// Read in: controllers (e.g., text/image/audio) to fetch channel-specific pricing or settings.
	ChannelModel = "channel_model"

	// ChannelRatio is the minimal ratio across all groups attached to the selected channel.
	// Set in: middleware/distributor (computed from channel groups via billing ratio).
	// Read in: controllers to scale pricing/billing (multiplied with model ratio).
	ChannelRatio = "channel_ratio"

	// Channel is the numeric channel type (see relay/channeltype).
	// Set in: middleware/distributor.
	// Read in: meta building and controllers for labeling and routing logic.
	Channel = "channel"

	// ChannelId is the numeric id of the selected channel (database id).
	// Set in: middleware/distributor (or from explicit selection).
	// Read widely for billing, logging, and meta.
	ChannelId = "channel_id"

	// SpecificChannelId indicates the caller explicitly requested a particular channel.
	// Set in: middleware/auth.TokenAuth via token suffix or :channelid route param (admin-only).
	// Read in: middleware/distributor to bypass normal selection and use that specific channel.
	SpecificChannelId = "specific_channel_id"

	// RequestModel is the model name as requested by the client (e.g., "gpt-4o").
	// Set in: middleware/auth.TokenAuth (parsed from body/query depending on endpoint) or early in adaptor handlers
	//         when TokenAuth did not parse the body yet.
	// Invariant: never mutate this value; it must always reflect the user's original input.
	// Mapping/rewriting to provider-specific names is handled via ModelMapping/Meta (ActualModelName), not by
	// mutating RequestModel. Use RequestModel for logging, billing trace, retries, and response.model.
	RequestModel = "request_model"

	// ConvertedRequest holds the provider-specific request body after conversion.
	// Set in: controller/text during conversion, and in several adaptors (AWS/Gemini/OpenAI variants).
	// Read in: adaptor DoRequest/DoResponse or signing steps that need the converted structure.
	ConvertedRequest = "converted_request"

	// WebSearchCallCount stores the number of OpenAI web search tool invocations observed in the upstream
	// response. Set by adaptors after parsing provider responses and consumed during billing adjustments.
	WebSearchCallCount = "web_search_call_count"

	// Group is the user group resolved for the current user (affects routing & ratios).
	// Set in: middleware/distributor (via model.CacheGetUserGroup).
	// Read in: meta/metrics and for channel selection.
	Group = "group"

	// ModelMapping is the mapping table for this channel (logical -> provider model names).
	// Set in: middleware/distributor from channel.GetModelMapping().
	// Read in: meta and controllers when rewriting model names.
	ModelMapping = "model_mapping"

	// ChannelName is the human-readable name of the selected channel.
	// Set in: middleware/distributor.
	// Read in: controller/relay for logging.
	ChannelName = "channel_name"

	// ContentType is the incoming request content type header value.
	// Set in: middleware/distributor from the request header.
	// Read in: image controller to decide JSON vs multipart/form handling.
	ContentType = "content_type"

	// TokenId is the id of the API token used for this request (if TokenAuth).
	// Set in: middleware/auth.TokenAuth.
	// Read in: billing and logs.
	TokenId = "token_id"

	// TokenName is the name/label of the API token used for this request.
	// Set in: middleware/auth.TokenAuth.
	// Read in: image controller logs and metrics.
	TokenName = "token_name"

	// TokenQuota is the remaining quota on the API token at the time of auth.
	// Set in: middleware/auth.TokenAuth.
	// Read in: controllers for pre-consumption logic.
	TokenQuota = "token_quota"

	// TokenQuotaUnlimited indicates the API token has unlimited quota semantics.
	// Set in: middleware/auth.TokenAuth.
	// Read in: controllers to bypass quota checks when true.
	TokenQuotaUnlimited = "token_quota_unlimited"

	// UserQuota optionally carries the user’s quota for metrics/UI labeling.
	// Not set by default middleware; controllers typically fetch from cache directly.
	// Used in: controller/text metrics recording (if present). Treat as optional.
	UserQuota = "user_quota"

	// BaseURL is the provider base URL resolved from the selected channel.
	// Set in: middleware/distributor from channel.GetBaseURL().
	// Read in: meta/audio and by adaptors that need to construct request URLs.
	BaseURL = "base_url"

	// AvailableModels is the CSV of models allowed by the API token (token.Models).
	// Set in: middleware/auth.TokenAuth when token has model restrictions.
	// Read in: controller/model.GetUserAvailableModels to build filtered model lists.
	AvailableModels = "available_models"

	// KeyRequestBody caches the raw request body bytes for reuse (avoid double read).
	// Set in: common/gin.go GetRequestBody and UnmarshalBodyReusable.
	// Read in: controllers (e.g., response/claude_messages) for debugging/logging.
	KeyRequestBody = gin.BodyBytesKey

	// SystemPrompt is a forced/extra system prompt configured on the channel.
	// Set in: middleware/distributor if channel.SystemPrompt is non-empty.
	// Read in: text controller to inject as system prompt when present.
	SystemPrompt = "system_prompt"

	// Meta holds the aggregated per-request meta (relay/meta.GetByContext).
	// Set in: relay/meta after composing fields from context and request.
	// Read widely anywhere Meta is needed (billing, adaptors, response handling).
	Meta = "meta"

	// RateLimit is the per-channel request-per-minute limit (integer).
	// Set in: middleware/distributor based on channel.RateLimit (or 0 if disabled).
	// Read in: middleware/rate-limit to enforce QPS/RPM limits.
	RateLimit = "rate_limit"

	// ClaudeMessagesConversion flags that this request/response should be converted
	// between Claude Messages API and another provider format.
	// Set in: many non-Anthropic adaptors when supporting Claude Messages via conversion.
	// Read in: openai_compatible.HandleClaudeMessagesResponse and controller/claude_messages.
	ClaudeMessagesConversion = "claude_messages_conversion"

	// OriginalClaudeRequest stores the original Claude Messages request struct for conversion.
	// Set alongside ClaudeMessagesConversion in adaptors.
	// Read during response conversion and logging.
	OriginalClaudeRequest = "original_claude_request"

	// Claude-specific context keys
	// ClaudeModel is the Claude model name for native Anthropic flows.
	// Set in: anthropic adaptor when handling native requests.
	ClaudeModel = "claude_model"

	// ClaudeMessagesNative marks that the request is using native Claude Messages passthrough
	// (no conversion to other formats).
	// Set in: anthropic/aws adaptors and controller/claude_messages when applicable.
	// Read in: tests and controller branches.
	ClaudeMessagesNative = "claude_messages_native"

	// ClaudeDirectPassthrough indicates the request should be proxied to Claude directly
	// without conversion, often used for streaming behavior and native support.
	// Set in: anthropic/aws adaptors.
	// Read in: controller/claude_messages to choose passthrough paths.
	ClaudeDirectPassthrough = "claude_direct_passthrough"

	// ConversationId is a deterministic id derived from messages for Claude "thinking"
	// signature caching and response verification.
	// Set in: anthropic adaptor when building/thinking with signatures.
	// Read in: anthropic adaptor to build cache/signature keys.
	ConversationId = "conversation_id"

	// TempSignatureKey stores a temporary cache key for Claude "thinking" signatures.
	// Set in: anthropic adaptor when buffering and stitching thinking segments.
	// Read nearby in the same flow to finalize signature verification.
	TempSignatureKey = "temp_signature_key"

	// Additional context keys
	// ConvertedResponse holds a ClaudeMessages response converted from provider-specific responses
	// (non-streaming paths). Set by conversion helpers (e.g., openai_compatible, gemini adaptor).
	// Read in: controller/claude_messages to return converted responses.
	ConvertedResponse = "converted_response"

	// ResponseRewriteHandler stores a function that rewrites upstream OpenAI-compatible
	// chat responses into another format (e.g., Response API) before returning to the client.
	ResponseRewriteHandler = "response_rewrite_handler"

	// ResponseAPIRequestOriginal keeps the original Response API request payload so that
	// downstream converters can hydrate metadata when rewriting responses.
	ResponseAPIRequestOriginal = "response_api_request_original"

	// ResponseStreamRewriteHandler stores a streaming rewrite adapter that can transform
	// upstream chat completion SSE chunks into another streaming format (e.g., Response API)
	// before flushing them to the client.
	ResponseStreamRewriteHandler = "response_stream_rewrite_handler"

	// ResponseFormat is used by image APIs to carry desired output format when posted via JSON.
	// Set in: image controller from request payload.
	// Read in: image controller to format the response properly.
	ResponseFormat = "response_format"

	// StreamingQuotaTracker stores the active quota tracker for incremental billing in streaming flows.
	// Set in: relay/controller/text when initializing a streaming request.
	// Read in: streaming adaptors to record completion progress and enforce quota limits mid-stream.
	StreamingQuotaTracker = "streaming_quota_tracker"
)
