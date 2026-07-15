package model

import "github.com/Laisky/one-api/dto"

// ToResponse builds the external boundary DTO for a user. It replaces the
// retired User.MarshalJSON whitelist: no internal integer id or inviter_id, and
// no secret (password/access_token/totp_secret/verification_code) crosses the
// API.
//
// Parameters: none (pointer receiver).
//
// Return values:
//   - dto.UserResponse: the UUID-only, secret-free external shape. A nil
//     receiver yields the zero shape.
func (user *User) ToResponse() dto.UserResponse {
	if user == nil {
		return dto.UserResponse{}
	}
	// The legacy userJSON typed this field as JSONStringSlice, whose custom
	// MarshalJSON emits "[]" (never "null") for a nil/empty slice. The dto uses
	// a plain []string, so preserve that contract by never handing it a nil.
	blacklist := []string(user.MCPToolBlacklist)
	if blacklist == nil {
		blacklist = []string{}
	}
	return dto.UserResponse{
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
		MCPToolBlacklist: blacklist,
		Metadata:         dto.UserMetadataResponse{PasswordLocked: user.Metadata.PasswordLocked},
		CreatedAt:        user.CreatedAt,
		UpdatedAt:        user.UpdatedAt,
	}
}

// UsersToResponses maps a slice of users to their external DTOs, pre-allocating
// the result.
//
// Parameters:
//   - users: rows to convert; nil elements map to the zero shape.
//
// Return values:
//   - []dto.UserResponse: one entry per input row.
func UsersToResponses(users []*User) []dto.UserResponse {
	out := make([]dto.UserResponse, 0, len(users))
	for _, u := range users {
		out = append(out, u.ToResponse())
	}
	return out
}
