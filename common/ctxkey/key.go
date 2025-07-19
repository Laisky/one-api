package ctxkey

import "github.com/gin-gonic/gin"

const (
	Config                   = "config"
	Id                       = "id"
	RequestId                = "X-Oneapi-Request-Id"
	Username                 = "username"
	Role                     = "role"
	Status                   = "status"
	ChannelModel             = "channel_model"
	ChannelRatio             = "channel_ratio"
	Channel                  = "channel"
	ChannelId                = "channel_id"
	SpecificChannelId        = "specific_channel_id"
	RequestModel             = "request_model"
	ConvertedRequest         = "converted_request"
	OriginalModel            = "original_model"
	Group                    = "group"
	ModelMapping             = "model_mapping"
	ChannelName              = "channel_name"
	ContentType              = "content_type"
	TokenId                  = "token_id"
	TokenName                = "token_name"
	TokenQuota               = "token_quota"
	TokenQuotaUnlimited      = "token_quota_unlimited"
	UserQuota                = "user_quota"
	BaseURL                  = "base_url"
	AvailableModels          = "available_models"
	KeyRequestBody           = gin.BodyBytesKey
	SystemPrompt             = "system_prompt"
	Meta                     = "meta"
	RateLimit                = "rate_limit"
	ClaudeMessagesConversion = "claude_messages_conversion"
	OriginalClaudeRequest    = "original_claude_request"

	// Claude-specific context keys
	ClaudeModel             = "claude_model"
	ClaudeMessagesNative    = "claude_messages_native"
	ClaudeDirectPassthrough = "claude_direct_passthrough"
	ConversationId          = "conversation_id"
	TempSignatureKey        = "temp_signature_key"

	// Additional context keys
	ConvertedResponse = "converted_response"
	ResponseFormat    = "response_format"
)
