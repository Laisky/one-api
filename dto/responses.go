package dto

// This file holds the boundary response DTOs for the management API. Each
// XResponse replicates, field-for-field, the whitelist that used to live in a
// model-level MarshalJSON method (see docs/proposals/20260714_boundary-response-dtos.md).
//
// These shapes enforce the S2 strict-out contract (external UUID identifiers
// only; no internal integer ids; no secrets) explicitly at the API boundary,
// instead of ambiently through a model type's MarshalJSON. The dto package is a
// leaf package and must never import model; the mapping functions that build
// these shapes live on the model types (model imports dto).
//
// Rule: these structs are copied, not redesigned. Any field addition or removal
// is a separate proposal — the golden files in model/testdata freeze their
// serialized form.

// RedemptionResponse is the external shape of a redemption code. It mirrors the
// legacy redemptionDTO (no id, no user_id).
type RedemptionResponse struct {
	UUID         string  `json:"uuid"`
	UserUUID     *string `json:"user_uuid"`
	Key          string  `json:"key"`
	Status       int     `json:"status"`
	Name         string  `json:"name"`
	Quota        int64   `json:"quota"`
	CreatedTime  int64   `json:"created_time"`
	RedeemedTime int64   `json:"redeemed_time"`
	Count        int     `json:"count"`
	CreatedAt    int64   `json:"created_at"`
	UpdatedAt    int64   `json:"updated_at"`
}

// LogResponse is the external shape of a usage/management log entry. It mirrors
// the legacy logJSON (no id, no integer FKs; channel_name/metadata omitempty).
type LogResponse struct {
	UUID               string         `json:"uuid"`
	UserUUID           *string        `json:"user_uuid"`
	CreatedAt          int64          `json:"created_at"`
	Type               int            `json:"type"`
	Content            string         `json:"content"`
	Username           string         `json:"username"`
	TokenName          string         `json:"token_name"`
	TokenUUID          *string        `json:"token_uuid"`
	ModelName          string         `json:"model_name"`
	OriginModelName    string         `json:"origin_model_name"`
	Quota              int            `json:"quota"`
	PromptTokens       int            `json:"prompt_tokens"`
	CompletionTokens   int            `json:"completion_tokens"`
	ChannelUUID        *string        `json:"channel_uuid"`
	ChannelName        string         `json:"channel_name,omitempty"`
	RequestId          string         `json:"request_id"`
	TraceId            string         `json:"trace_id"`
	UpdatedAt          int64          `json:"updated_at"`
	ElapsedTime        int64          `json:"elapsed_time"`
	IsStream           bool           `json:"is_stream"`
	SystemPromptReset  bool           `json:"system_prompt_reset"`
	CachedPromptTokens int            `json:"cached_prompt_tokens"`
	Metadata           map[string]any `json:"metadata,omitempty"`
}

// TokenResponse is the external shape of an API token. It mirrors the legacy
// tokenDTO (no id, no user_id; key carries the configured prefix).
type TokenResponse struct {
	UUID           string  `json:"uuid"`
	UserUUID       *string `json:"user_uuid"`
	Key            string  `json:"key"`
	Status         int     `json:"status"`
	Name           string  `json:"name"`
	CreatedTime    int64   `json:"created_time"`
	AccessedTime   int64   `json:"accessed_time"`
	ExpiredTime    int64   `json:"expired_time"`
	RemainQuota    int64   `json:"remain_quota"`
	UnlimitedQuota bool    `json:"unlimited_quota"`
	UsedQuota      int64   `json:"used_quota"`
	CreatedAt      int64   `json:"created_at"`
	UpdatedAt      int64   `json:"updated_at"`
	Models         *string `json:"models"`
	Subnet         *string `json:"subnet"`
}

// UserResponse is the external shape of a user. It mirrors the legacy userJSON
// (no id, no inviter_id, no secrets).
type UserResponse struct {
	UUID             string               `json:"uuid"`
	Username         string               `json:"username"`
	DisplayName      string               `json:"display_name"`
	Role             int                  `json:"role"`
	Status           int                  `json:"status"`
	Email            string               `json:"email"`
	GitHubId         string               `json:"github_id"`
	WeChatId         string               `json:"wechat_id"`
	LarkId           string               `json:"lark_id"`
	OidcId           string               `json:"oidc_id"`
	Quota            int64                `json:"quota"`
	UsedQuota        int64                `json:"used_quota"`
	RequestCount     int                  `json:"request_count"`
	Group            string               `json:"group"`
	AffCode          string               `json:"aff_code"`
	InviterUUID      *string              `json:"inviter_uuid"`
	MCPToolBlacklist []string             `json:"mcp_tool_blacklist"`
	Metadata         UserMetadataResponse `json:"metadata"`
	CreatedAt        int64                `json:"created_at"`
	UpdatedAt        int64                `json:"updated_at"`
}

// UserMetadataResponse mirrors model.UserMetadata for the boundary response.
// It must stay in sync with model.UserMetadata's JSON tags.
type UserMetadataResponse struct {
	PasswordLocked bool `json:"password_locked,omitempty"`
}

// ChannelResponse is the external shape of a channel. It mirrors the legacy
// channelJSON (no id).
type ChannelResponse struct {
	UUID                   string  `json:"uuid"`
	Type                   int     `json:"type"`
	Key                    string  `json:"key"`
	Status                 int     `json:"status"`
	Name                   string  `json:"name"`
	Weight                 *uint   `json:"weight"`
	CreatedTime            int64   `json:"created_time"`
	TestTime               int64   `json:"test_time"`
	ResponseTime           int     `json:"response_time"`
	BaseURL                *string `json:"base_url"`
	Other                  *string `json:"other"`
	Balance                float64 `json:"balance"`
	BalanceUpdatedTime     int64   `json:"balance_updated_time"`
	Models                 string  `json:"models"`
	HiddenModels           *string `json:"hidden_models"`
	ModelConfigs           *string `json:"model_configs"`
	Group                  string  `json:"group"`
	UsedQuota              int64   `json:"used_quota"`
	ModelMapping           *string `json:"model_mapping"`
	Priority               *int64  `json:"priority"`
	Config                 string  `json:"config"`
	SystemPrompt           *string `json:"system_prompt"`
	RateLimit              *int    `json:"ratelimit"`
	TestingModel           *string `json:"testing_model"`
	ModelRatio             *string `json:"model_ratio"`
	CompletionRatio        *string `json:"completion_ratio"`
	CreatedAt              int64   `json:"created_at"`
	UpdatedAt              int64   `json:"updated_at"`
	InferenceProfileArnMap *string `json:"inference_profile_arn_map"`
}
