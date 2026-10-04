package model

import (
	"context"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/common/identity"
)

// GetUserForTokenAuthentication reads the current account from the shared
// database for authorization. Redis user objects are deliberately not consulted:
// their TTL cannot establish immediate deletion/disable revocation across
// replicas. The returned account excludes credential fields; lookup and context
// failures are returned to the caller and cannot authorize a request.
func GetUserForTokenAuthentication(ctx context.Context, id int) (*User, error) {
	if id <= 0 {
		return nil, errors.New("invalid token owner id")
	}
	var user User
	if err := DB.WithContext(ctx).Omit("password", "access_token", "totp_secret", "verification_code").First(&user, "id = ?", id).Error; err != nil {
		return nil, identity.Tag(errors.Wrap(err, "read current token owner"), identity.NewUserRef(id, "", ""))
	}
	return &user, nil
}
