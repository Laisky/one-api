package dto

// AdminTokenResponse is the read-only cross-user token inventory. Its explicit
// field set contains no credential, even when the viewed token belongs to root.
// Owner creation/reveal endpoints use the separate TokenResponse contract.
type AdminTokenResponse struct {
	UUID           string  `json:"uuid"`
	UserUUID       *string `json:"user_uuid"`
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
