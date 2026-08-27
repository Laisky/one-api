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

	// ChannelUUID is the external UUID of the selected channel.
	// Set in: middleware/distributor after the channel row is selected.
	// Read by relay metadata and log writers that need client-safe identifiers.
	ChannelUUID = "channel_uuid"

	// SpecificChannelId indicates the caller explicitly requested a particular channel.
	// Set in: middleware/auth.TokenAuth via token suffix or :channelid route param (admin-only).
	// Read in: middleware/distributor to bypass normal selection and use that specific channel.
	SpecificChannelId = "specific_channel_id"

	// ResponseStateAffinityChannelId records the channel that a Responses request was
	// pinned to by gateway provider affinity (proposal ST-005). Unlike
	// SpecificChannelId it is a SOFT preference: it never disables retry/failover, so
	// an ineligible or unhealthy binding transparently falls back to normal selection.
	// Set in: middleware/distributor when a referenced gateway response/conversation
	// resolves to a still-eligible bound channel. Read for observability/tests.
	ResponseStateAffinityChannelId = "response_state_affinity_channel_id"

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

	// RelayMode records the relay processing mode (chat, embeddings, etc.) selected for the request.
	// Set by adaptors when branching on relay behavior.
	// Read by downstream handlers to adjust response handling.
	RelayMode = "relay_mode"

	// ImageRequest caches the converted image generation payload for downstream handlers.
	// Set in: AWS image adaptor when preparing provider-specific requests.
	// Read in: response handlers that need to inspect the converted structure.
	ImageRequest = "image_request"

	// OutputImageCount stores the number of output images observed in upstream responses.
	// Set by: adaptors that parse image outputs (e.g., Gemini inlineData).
	// Read in: controllers to apply per-image billing for image-capable models.
	OutputImageCount = "output_image_count"
	// OutputAudioSeconds stores the total output audio duration in seconds for billing.
	// Set by: adaptors that parse output audio payloads (e.g., base64 audio responses).
	// Read in: controllers to apply per-second audio billing when output audio is produced.
	OutputAudioSeconds = "output_audio_seconds"
	// OutputAudioTokens stores the output audio token count when per-token billing is required.
	// Set by: adaptors that parse output audio usage details.
	// Read in: controllers to apply fallback audio billing when duration is unavailable.
	OutputAudioTokens = "output_audio_tokens"
	// OutputVideoSeconds stores the total output video duration in seconds for billing.
	// Set by: adaptors that parse video generation outputs.
	// Read in: controllers to apply per-second video billing.
	OutputVideoSeconds = "output_video_seconds"
	// OutputVideoResolution stores the output video resolution string (e.g., "1920x1080") for pricing multipliers.
	// Set by: adaptors that parse video generation outputs.
	// Read in: controllers to apply resolution-based video pricing multipliers.
	OutputVideoResolution = "output_video_resolution"

	// WebSearchCallCount stores the number of OpenAI web search tool invocations observed in the upstream
	// response. Set by adaptors after parsing provider responses and consumed during billing adjustments.
	WebSearchCallCount = "web_search_call_count"

	// ToolInvocationCounts stores raw built-in tool invocation counts aggregated during response handling.
	// Adaptors may populate this alongside specialized counters like WebSearchCallCount.
	ToolInvocationCounts = "tool_invocation_counts"
	// ToolInvocationSummary captures per-request built-in tool usage, including counts and billed quota.
	// Populated by tooling.ApplyBuiltinToolCharges and consumed by billing metadata generation.
	ToolInvocationSummary = "tool_invocation_summary"

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

	// TokenUUID is the external UUID of the API token used for this request.
	// Set in: middleware/auth.TokenAuth.
	// Read by relay metadata and log writers that need client-safe identifiers.
	TokenUUID = "token_uuid"

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

	// UserObj stores the authenticated *model.User in context.
	// Set in: middleware/auth (both session and token auth paths).
	// Read in: downstream handlers to avoid redundant DB/cache lookups for user fields
	// (username, group, quota, status, etc.).
	UserObj = "user_obj"

	// UserUUID is the external UUID of the authenticated user.
	// Set in: middleware/auth for session and token authentication.
	// Read by relay metadata and log writers that need client-safe identifiers.
	UserUUID = "user_uuid"

	// BaseURL is the provider base URL resolved from the selected channel.
	// Set in: middleware/distributor from channel.GetBaseURL().
	// Read in: meta/audio and by adaptors that need to construct request URLs.
	BaseURL = "base_url"

	// AvailableModels is the CSV of models allowed by the API token (token.Models).
	// Set in: middleware/auth.TokenAuth when token has model restrictions.
	// Read in: controller/model.filterAbilitiesByTokenAllowList (scopes GET /v1/models
	// and GET /v1/models/:model to what the key may invoke),
	// controller/model.GetAvailableModelsByToken, and controller/user.GetTokenInfo.
	// Absent means the token is unrestricted -- never treat absence as "deny all".
	AvailableModels = "available_models"

	// KeyRequestBody caches the raw request body bytes for reuse (avoid double read).
	// Set in: common/gin.go GetRequestBody and UnmarshalBodyReusable.
	// Read in: controllers (e.g., response/claude_messages) for debugging/logging.
	KeyRequestBody = gin.BodyBytesKey

	// ClientRequestPayloadLogged marks whether the inbound client payload has already been logged.
	// Set in: common.LogClientRequestPayload.
	// Read in: common.LogClientRequestPayload to avoid duplicate request-body logs in middleware/controllers.
	ClientRequestPayloadLogged = "client_request_payload_logged"

	// AsyncTaskRequestMetadata stores a sanitized snapshot of asynchronous task request parameters (e.g., /v1/videos POST payload).
	// Set in: RelayVideoHelper after parsing the incoming request.
	// Read in: async task persistence to capture request context for later diagnostics.
	AsyncTaskRequestMetadata = "async_task_request_metadata"

	// SystemPrompt is a forced/extra system prompt configured on the channel.
	// Set in: middleware/distributor if channel.SystemPrompt is non-empty.
	// Read in: text controller to inject as system prompt when present.
	SystemPrompt = "system_prompt"

	// Meta holds the aggregated per-request meta (relay/meta.GetByContext).
	// Set in: relay/meta after composing fields from context and request.
	// Read widely anywhere Meta is needed (billing, adaptors, response handling).
	Meta = "meta"

	// EmbeddingPromptTokensDetails stores preflight embedding modality token details.
	// Set in: text/response controllers after provider-side token counting.
	// Read in: embedding adaptors and quota billing to preserve multimodal costs.
	EmbeddingPromptTokensDetails = "embedding_prompt_tokens_details"

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

	// ClaudeToolSearchEnabled marks that the current Claude request includes Anthropic
	// Tool Search built-ins and should attach corresponding Anthropic beta headers.
	// Set in: anthropic adaptor during Claude request conversion.
	// Read in: anthropic adaptor SetupRequestHeader.
	ClaudeToolSearchEnabled = "claude_tool_search_enabled"

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

	// ResponseAPIUpstreamID carries the raw upstream Responses id (resp_...) observed
	// when a Chat Completions or Claude Messages request was served by a
	// Responses-format upstream. It is set by the openai Chat/Claude<-Responses render
	// handlers so the controller can record a continuation checkpoint against that
	// upstream handle (ST-022, stateless-client optimization). It is an internal
	// gateway handle and must never be surfaced to the client.
	ResponseAPIUpstreamID = "response_api_upstream_id"

	// ResponseAPIAssistantMessage carries the rendered assistant turn (as the client
	// will echo it back on its next stateless request) so the checkpoint key covers
	// the full downstream-visible transcript, not only the request messages (ST-022).
	ResponseAPIAssistantMessage = "response_api_assistant_message"

	// SkipAdaptorResponseBodyLog suppresses verbose adaptor-level upstream body logging when the
	// controller already captured and will emit a debug preview. This avoids duplicate payload logs
	// while keeping high-level status metadata available.
	SkipAdaptorResponseBodyLog = "skip_adaptor_response_body_log"

	// DebugResponseWriter stores the body-capturing response writer used for debug logging of outbound payloads.
	// Set in: relay/controller debug logging helpers when enhanced diagnostics are enabled.
	// Read in: controller/relay and relay/controller helpers when writing response debug logs.
	DebugResponseWriter = "debug_response_writer"

	// ResponseRewriteHandler stores a function that rewrites upstream OpenAI-compatible
	// chat responses into another format (e.g., Response API) before returning to the client.
	ResponseRewriteHandler = "response_rewrite_handler"

	// ResponseRewriteApplied marks whether a rewrite handler already emitted the outbound payload,
	// preventing duplicate bodies when fallback logic inspects captured responses.
	ResponseRewriteApplied = "response_rewrite_applied"

	// ResponseAPIRequestOriginal keeps the original Response API request payload so that
	// downstream converters can hydrate metadata when rewriting responses.
	ResponseAPIRequestOriginal = "response_api_request_original"

	// ResponseStreamRewriteHandler stores a streaming rewrite adapter that can transform
	// upstream chat completion SSE chunks into another streaming format (e.g., Response API)
	// before flushing them to the client.
	ResponseStreamRewriteHandler = "response_stream_rewrite_handler"

	// ToolNameSanitizeMap stores a per-request bidirectional rename table produced when
	// tool/function names are sanitized for strict upstream validators (DeepSeek, OpenAI,
	// Anthropic — all of which enforce `^[a-zA-Z0-9_-]+$`). The value is a map[string]string
	// keyed by the sanitized name and yielding the original client-provided name. Response
	// handlers consult this map to restore original names on
	// `choices[].message.tool_calls[].function.name` (non-stream) and
	// `choices[].delta.tool_calls[].function.name` (stream) before forwarding to the client,
	// so the round-trip stays transparent to MCP-namespaced names like `server.tool`.
	// Set in: relay/adaptor/common/toolnamesafe.SanitizeRequestToolNames.
	// Read in: relay/adaptor/openai_compatible.Handler / UnifiedStreamProcessing and
	// relay/adaptor/openai.Handler / StreamHandler / relay/adaptor/anthropic handlers when
	// restoring names.
	ToolNameSanitizeMap = "tool_name_sanitize_map"

	// ResponseFormat is used by image APIs to carry desired output format when posted via JSON.
	// Set in: image controller from request payload.
	// Read in: image controller to format the response properly.
	ResponseFormat = "response_format"

	// StreamingQuotaTracker stores the active quota tracker for incremental billing in streaming flows.
	// Set in: relay/controller/text when initializing a streaming request.
	// Read in: streaming adaptors to record completion progress and enforce quota limits mid-stream.
	StreamingQuotaTracker = "streaming_quota_tracker"

	// UpstreamRequestPossiblyForwarded indicates the request may already have been
	// forwarded to upstream (or accepted by upstream transport). Controllers use this
	// to apply conservative no-underbilling policy on ambiguous failures by avoiding
	// automatic pre-consume refunds once forwarding has started.
	UpstreamRequestPossiblyForwarded = "upstream_request_possibly_forwarded"

	// APIFormat is the detected API format of the request (OpenAI, Claude, Response API).
	// Set in: middleware/api_format_detect.
	// Read in: metrics for labeling.
	APIFormat = "api_format"

	// BillingReconciled is set to true when post-billing or refund has been completed
	// for pre-consumed quota. Used by the billing audit safety net to detect leaked quota.
	// Set in: billing.PostConsumeQuotaDetailed, returnPreConsumedQuotaConservative.
	// Read in: billing audit defer in relay handlers.
	BillingReconciled = "billing_reconciled"

	// PreConsumedQuotaAmount stores the pre-consumed quota amount for billing audit.
	// Set in: preConsumeQuota, preConsumeResponseAPIQuota, preConsumeClaudeMessagesQuota.
	// Read in: billing audit defer in relay handlers.
	PreConsumedQuotaAmount = "pre_consumed_quota_amount"

	// ProvisionalLogId stores the database ID of the provisional consume log entry
	// created at pre-consume time. Post-billing uses this to reconcile the log
	// with actual usage data.
	ProvisionalLogId = "provisional_log_id"

	// Identity holds the common/identity.Set bound for this request (user + token
	// + channel references, each carrying id + uuid + name).
	// Set in: common/identity.Bind, driven by middleware/auth and middleware/distributor.
	// Read in: error funnels and any handler that needs identity without a lookup.
	Identity = "identity"

	// BaseLogger holds the pristine request logger captured before any identity
	// field was bound, so common/identity.Bind can REBUILD the logger instead of
	// appending a duplicate channel_id when the channel changes on a relay retry.
	// Set in: common/identity.BindBase, driven by middleware.RequestId.
	// Read in: common/identity.Bind only.
	BaseLogger = "base_logger"
)
