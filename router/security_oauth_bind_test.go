package router

import (
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/model"
)

// oauthBindColumn maps each public OAuth bind provider to its identity column.
var oauthBindColumn = map[string]string{
	"github": "github_id",
	"oidc":   "oidc_id",
	"lark":   "lark_id",
}

// oauthBindCallback runs the public GET bind callback for provider in browser
// with a freshly issued state and returns status and body, converting a
// handler panic into status 599 so a malformed session is observable.
func oauthBindCallback(t *testing.T, browser *oauthBrowser, provider, code string) (result testHTTPResult) {
	t.Helper()
	state := browser.oauthState(t)
	defer func() {
		if recovered := recover(); recovered != nil {
			result = testHTTPResult{status: 599, body: fmt.Sprint(recovered)}
		}
	}()
	w := browser.do(http.MethodGet, "/api/oauth/"+provider+"?code="+code+"&state="+state, "", "same-origin", nil)
	return testHTTPResult{status: w.Code, body: w.Body.String()}
}

// boundIdentity returns the provider identity column of user id, or an empty
// string when the row no longer exists.
func boundIdentity(t *testing.T, db *gorm.DB, provider string, id int) string {
	t.Helper()
	var value string
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", id).Select(oauthBindColumn[provider]).Scan(&value).Error)
	return value
}

// TestSecurityOAuthBindRequiresActiveAccount proves the public GitHub, OIDC and
// Lark bind callbacks apply the dashboard account checks: a disabled,
// soft-deleted or removed account, or a malformed session, cannot bind a provider identity (and the
// provider is never contacted), while an active account still binds.
func TestSecurityOAuthBindRequiresActiveAccount(t *testing.T) {
	for provider := range oauthBindColumn {
		t.Run(provider, func(t *testing.T) {
			for _, tc := range []struct {
				name       string
				mutate     func(t *testing.T, db *gorm.DB, id int)
				sessionID  func(id int) string
				wantStatus int
				wantBound  bool
			}{
				{name: "active", wantStatus: http.StatusOK, wantBound: true},
				{name: "disabled", mutate: func(t *testing.T, db *gorm.DB, id int) {
					require.NoError(t, db.Model(&model.User{}).Where("id = ?", id).Update("status", model.UserStatusDisabled).Error)
				}, wantStatus: http.StatusForbidden},
				{name: "soft_deleted", mutate: func(t *testing.T, db *gorm.DB, id int) {
					require.NoError(t, db.Model(&model.User{}).Where("id = ?", id).Update("status", model.UserStatusDeleted).Error)
				}, wantStatus: http.StatusForbidden},
				{name: "removed", mutate: func(t *testing.T, db *gorm.DB, id int) {
					require.NoError(t, db.Delete(&model.User{}, id).Error)
				}, wantStatus: http.StatusUnauthorized},
				{name: "malformed_session", sessionID: func(id int) string { return "not-a-number" }, wantStatus: http.StatusUnauthorized},
			} {
				t.Run(tc.name, func(t *testing.T) {
					db, engine, _ := csrfFixture(t)
					registerOAuthSessionIssuer(t, engine)
					user := oauthFixtureUser("bind-" + provider + "-" + tc.name)
					user.TotpSecret = ""
					require.NoError(t, db.Create(&user).Error)
					identity := "bind-identity-" + provider + "-" + tc.name
					stub := installOAuthIdPStub(t, map[string]string{"bind-code": identity})
					sessionID := strconv.Itoa(user.Id)
					if tc.sessionID != nil {
						sessionID = tc.sessionID(user.Id)
					}
					browser := issueOAuthSession(t, engine, sessionID, user.Username)
					if tc.mutate != nil {
						tc.mutate(t, db, user.Id)
					}

					result := oauthBindCallback(t, browser, provider, "bind-code")
					require.Equal(t, tc.wantStatus, result.status, result.body)
					if tc.wantBound {
						require.Contains(t, result.body, `"message":"bind"`)
						require.Equal(t, identity, boundIdentity(t, db, provider, user.Id))
						return
					}
					require.NotContains(t, result.body, `"success":true`)
					require.Empty(t, boundIdentity(t, db, provider, user.Id), "rejected account was bound")
					require.Zero(t, stub.calls.Load(), "rejected bind contacted the identity provider")
					// Re-enabling the account must not revive the rejected session.
					require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("status", model.UserStatusEnabled).Error)
					require.Equal(t, http.StatusUnauthorized, browser.selfStatus(), "rejected session was not cleared")
				})
			}
		})
	}
}
