package dto

// LogStatistic captures aggregated log metrics grouped by day and model name.
type LogStatistic struct {
	Day                string `json:"Day" gorm:"column:day"`
	ModelName          string `json:"ModelName" gorm:"column:model_name"`
	RequestCount       int    `json:"RequestCount" gorm:"column:request_count"`
	Quota              int    `json:"Quota" gorm:"column:quota"`
	PromptTokens       int    `json:"PromptTokens" gorm:"column:prompt_tokens"`
	CompletionTokens   int    `json:"CompletionTokens" gorm:"column:completion_tokens"`
	CachedPromptTokens int    `json:"CachedPromptTokens" gorm:"column:cached_prompt_tokens"`
	CacheHitCount      int    `json:"CacheHitCount" gorm:"column:cache_hit_count"`
	CacheHitQuota      int    `json:"CacheHitQuota" gorm:"column:cache_hit_quota"`
}

// LogStatisticByUser captures aggregated log metrics grouped by day and username.
type LogStatisticByUser struct {
	Day                string `json:"Day" gorm:"column:day"`
	Username           string `json:"Username" gorm:"column:username"`
	UserId             int    `json:"-" gorm:"column:user_id"`
	UserUUID           string `json:"user_uuid" gorm:"column:user_uuid"`
	RequestCount       int    `json:"RequestCount" gorm:"column:request_count"`
	Quota              int    `json:"Quota" gorm:"column:quota"`
	PromptTokens       int    `json:"PromptTokens" gorm:"column:prompt_tokens"`
	CompletionTokens   int    `json:"CompletionTokens" gorm:"column:completion_tokens"`
	CachedPromptTokens int    `json:"CachedPromptTokens" gorm:"column:cached_prompt_tokens"`
	CacheHitCount      int    `json:"CacheHitCount" gorm:"column:cache_hit_count"`
	CacheHitQuota      int    `json:"CacheHitQuota" gorm:"column:cache_hit_quota"`
}

// LogStatisticByToken captures aggregated log metrics grouped by day, token, and username.
type LogStatisticByToken struct {
	Day                string `json:"Day" gorm:"column:day"`
	Username           string `json:"Username" gorm:"column:username"`
	UserId             int    `json:"-" gorm:"column:user_id"`
	UserUUID           string `json:"user_uuid" gorm:"column:user_uuid"`
	TokenName          string `json:"TokenName" gorm:"column:token_name"`
	RequestCount       int    `json:"RequestCount" gorm:"column:request_count"`
	Quota              int    `json:"Quota" gorm:"column:quota"`
	PromptTokens       int    `json:"PromptTokens" gorm:"column:prompt_tokens"`
	CompletionTokens   int    `json:"CompletionTokens" gorm:"column:completion_tokens"`
	CachedPromptTokens int    `json:"CachedPromptTokens" gorm:"column:cached_prompt_tokens"`
	CacheHitCount      int    `json:"CacheHitCount" gorm:"column:cache_hit_count"`
	CacheHitQuota      int    `json:"CacheHitQuota" gorm:"column:cache_hit_quota"`
}

// ToolLogStatistic captures aggregated tool usage grouped by day and tool name.
type ToolLogStatistic struct {
	Day          string `json:"Day" gorm:"column:day"`
	ToolName     string `json:"ToolName" gorm:"column:tool_name"`
	RequestCount int    `json:"RequestCount" gorm:"column:request_count"`
	Quota        int64  `json:"Quota" gorm:"column:quota"`
}

// ToolLogStatisticByUser captures aggregated tool usage grouped by day and user.
type ToolLogStatisticByUser struct {
	Day          string `json:"Day" gorm:"column:day"`
	Username     string `json:"Username" gorm:"column:username"`
	UserId       int    `json:"-" gorm:"column:user_id"`
	UserUUID     string `json:"user_uuid" gorm:"column:user_uuid"`
	RequestCount int    `json:"RequestCount" gorm:"column:request_count"`
	Quota        int64  `json:"Quota" gorm:"column:quota"`
}

// ToolLogStatisticByToken captures aggregated tool usage grouped by day, token, and user.
type ToolLogStatisticByToken struct {
	Day          string `json:"Day" gorm:"column:day"`
	Username     string `json:"Username" gorm:"column:username"`
	UserId       int    `json:"-" gorm:"column:user_id"`
	UserUUID     string `json:"user_uuid" gorm:"column:user_uuid"`
	TokenName    string `json:"TokenName" gorm:"column:token_name"`
	RequestCount int    `json:"RequestCount" gorm:"column:request_count"`
	Quota        int64  `json:"Quota" gorm:"column:quota"`
}
