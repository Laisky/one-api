package dto

// LogStatistic captures aggregated log metrics grouped by day and model name.
type LogStatistic struct {
	Day                string `gorm:"column:day"`
	ModelName          string `gorm:"column:model_name"`
	RequestCount       int    `gorm:"column:request_count"`
	Quota              int    `gorm:"column:quota"`
	PromptTokens       int    `gorm:"column:prompt_tokens"`
	CompletionTokens   int    `gorm:"column:completion_tokens"`
	CachedPromptTokens int    `gorm:"column:cached_prompt_tokens"`
	CacheHitCount      int    `gorm:"column:cache_hit_count"`
	CacheHitQuota      int    `gorm:"column:cache_hit_quota"`
}

// LogStatisticByUser captures aggregated log metrics grouped by day and username.
type LogStatisticByUser struct {
	Day                string `gorm:"column:day"`
	Username           string `gorm:"column:username"`
	UserId             int    `gorm:"column:user_id"`
	RequestCount       int    `gorm:"column:request_count"`
	Quota              int    `gorm:"column:quota"`
	PromptTokens       int    `gorm:"column:prompt_tokens"`
	CompletionTokens   int    `gorm:"column:completion_tokens"`
	CachedPromptTokens int    `gorm:"column:cached_prompt_tokens"`
	CacheHitCount      int    `gorm:"column:cache_hit_count"`
	CacheHitQuota      int    `gorm:"column:cache_hit_quota"`
}

// LogStatisticByToken captures aggregated log metrics grouped by day, token, and username.
type LogStatisticByToken struct {
	Day                string `gorm:"column:day"`
	Username           string `gorm:"column:username"`
	UserId             int    `gorm:"column:user_id"`
	TokenName          string `gorm:"column:token_name"`
	RequestCount       int    `gorm:"column:request_count"`
	Quota              int    `gorm:"column:quota"`
	PromptTokens       int    `gorm:"column:prompt_tokens"`
	CompletionTokens   int    `gorm:"column:completion_tokens"`
	CachedPromptTokens int    `gorm:"column:cached_prompt_tokens"`
	CacheHitCount      int    `gorm:"column:cache_hit_count"`
	CacheHitQuota      int    `gorm:"column:cache_hit_quota"`
}
