package model

import (
	"encoding/json"

	"github.com/Laisky/errors/v2"
)

type userJSON struct {
	UUID             string          `json:"uuid"`
	Username         string          `json:"username"`
	DisplayName      string          `json:"display_name"`
	Role             int             `json:"role"`
	Status           int             `json:"status"`
	Email            string          `json:"email"`
	GitHubId         string          `json:"github_id"`
	WeChatId         string          `json:"wechat_id"`
	LarkId           string          `json:"lark_id"`
	OidcId           string          `json:"oidc_id"`
	Quota            int64           `json:"quota"`
	UsedQuota        int64           `json:"used_quota"`
	RequestCount     int             `json:"request_count"`
	Group            string          `json:"group"`
	AffCode          string          `json:"aff_code"`
	InviterUUID      *string         `json:"inviter_uuid"`
	MCPToolBlacklist JSONStringSlice `json:"mcp_tool_blacklist"`
	Metadata         UserMetadata    `json:"metadata"`
	CreatedAt        int64           `json:"created_at"`
	UpdatedAt        int64           `json:"updated_at"`
}

// MarshalJSON serializes User API responses with UUID identifiers and omits internal integer IDs and secrets.
// Parameters: none.
// Return values:
//   - []byte: JSON payload for the user.
//   - error: wrapped marshal error when serialization fails.
func (user User) MarshalJSON() ([]byte, error) {
	payload := userJSON{
		UUID:             user.UUID,
		Username:         user.Username,
		DisplayName:      user.DisplayName,
		Role:             user.Role,
		Status:           user.Status,
		Email:            user.Email,
		GitHubId:         user.GitHubId,
		WeChatId:         user.WeChatId,
		LarkId:           user.LarkId,
		OidcId:           user.OidcId,
		Quota:            user.Quota,
		UsedQuota:        user.UsedQuota,
		RequestCount:     user.RequestCount,
		Group:            user.Group,
		AffCode:          user.AffCode,
		InviterUUID:      user.InviterUUID,
		MCPToolBlacklist: user.MCPToolBlacklist,
		Metadata:         user.Metadata,
		CreatedAt:        user.CreatedAt,
		UpdatedAt:        user.UpdatedAt,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.Wrap(err, "marshal user")
	}
	return raw, nil
}
