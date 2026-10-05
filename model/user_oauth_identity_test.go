package model

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/errkind"
)

// TestBindUserOAuthIdentity verifies a bind writes only the identity column of
// an enabled account, leaves concurrently changed quota untouched, and refuses
// disabled, soft-deleted or missing accounts and unknown columns.
func TestBindUserOAuthIdentity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "bind.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}))
	originalDB := DB
	DB = db
	t.Cleanup(func() { DB = originalDB })
	require.NoError(t, db.Create(&[]User{
		{Id: 1, Username: "enabled", Status: UserStatusEnabled, Quota: 100, AffCode: "b1", AccessToken: "bind-access-1"},
		{Id: 2, Username: "disabled", Status: UserStatusDisabled, AffCode: "b2", AccessToken: "bind-access-2"},
		{Id: 3, Username: "soft-deleted", Status: UserStatusDeleted, AffCode: "b3", AccessToken: "bind-access-3"},
	}).Error)
	ctx := context.Background()

	// A quota change that lands after the caller's read must survive the bind.
	require.NoError(t, db.Model(&User{}).Where("id = ?", 1).Update("quota", 42).Error)
	require.NoError(t, BindUserOAuthIdentity(ctx, 1, OAuthIdentityGitHub, "octocat"))
	var user User
	require.NoError(t, db.First(&user, 1).Error)
	require.Equal(t, "octocat", user.GitHubId)
	require.EqualValues(t, 42, user.Quota)

	for _, id := range []int{2, 3, 404} {
		err := BindUserOAuthIdentity(ctx, id, OAuthIdentityOidc, "subject")
		require.Error(t, err)
		require.Equal(t, errkind.NotFound, errkind.Of(err), "user %d", id)
	}
	var bound int64
	require.NoError(t, db.Model(&User{}).Where("oidc_id = ?", "subject").Count(&bound).Error)
	require.Zero(t, bound)

	err = BindUserOAuthIdentity(ctx, 1, OAuthIdentityColumn("password"), "x")
	require.Equal(t, errkind.InvalidRequest, errkind.Of(err))
	err = BindUserOAuthIdentity(ctx, 1, OAuthIdentityLark, "")
	require.Equal(t, errkind.InvalidRequest, errkind.Of(err))
}
