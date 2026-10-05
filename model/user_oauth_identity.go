package model

import (
	"context"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/common/errkind"
)

// OAuthIdentityColumn names a users column that stores an external login
// identity. Only the constants below are accepted, so a column name never
// comes from request input.
type OAuthIdentityColumn string

const (
	// OAuthIdentityGitHub is the GitHub login column.
	OAuthIdentityGitHub OAuthIdentityColumn = "github_id"
	// OAuthIdentityOidc is the OIDC subject column.
	OAuthIdentityOidc OAuthIdentityColumn = "oidc_id"
	// OAuthIdentityLark is the Lark open id column.
	OAuthIdentityLark OAuthIdentityColumn = "lark_id"
	// OAuthIdentityWeChat is the WeChat id column.
	OAuthIdentityWeChat OAuthIdentityColumn = "wechat_id"
)

// BindUserOAuthIdentity stores value in the external identity column of the
// enabled account userID. It writes only that column, so a bind
// never rewrites quota or other fields from a stale read. It returns an
// InvalidRequest-kind error for an unknown column, a non-positive id or an
// empty value, a NotFound-kind error when no enabled account matched, and a
// wrapped database error otherwise.
func BindUserOAuthIdentity(ctx context.Context, userID int, column OAuthIdentityColumn, value string) error {
	switch column {
	case OAuthIdentityGitHub, OAuthIdentityOidc, OAuthIdentityLark, OAuthIdentityWeChat:
	default:
		return errkind.InvalidRequestErr(errors.Errorf("unsupported OAuth identity column %q", column))
	}
	if userID <= 0 || value == "" {
		return errkind.InvalidRequestErr(errors.Errorf("invalid %s binding for user %d", column, userID))
	}
	result := DB.WithContext(ctx).Model(&User{}).Where("id = ? AND status = ?", userID, UserStatusEnabled).Update(string(column), value)
	if result.Error != nil {
		return errors.Wrapf(result.Error, "bind %s for user %d", column, userID)
	}
	if result.RowsAffected == 0 {
		return errkind.NotFoundErr(errors.Errorf("user %d not found", userID))
	}
	return nil
}
