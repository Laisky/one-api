package router

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// TestSecurityOAuthStateCrossSiteOverwrite proves a hostile cross-site or
// sibling-site navigation to /api/oauth/state cannot replace the state of an
// in-progress login: it is rejected without a Set-Cookie and the original state
// still completes the OIDC login.
func TestSecurityOAuthStateCrossSiteOverwrite(t *testing.T) {
	for _, site := range []string{"cross-site", "same-site"} {
		t.Run(site, func(t *testing.T) {
			db, engine, _ := csrfFixture(t)
			user := oauthFixtureUser("state-" + site)
			user.TotpSecret = ""
			user.OidcId = "state-oidc-" + site
			require.NoError(t, db.Create(&user).Error)
			installOAuthIdPStub(t, map[string]string{"state-code": user.OidcId})
			browser := &oauthBrowser{engine: engine}
			state := browser.oauthState(t)
			pending := browser.cookie

			hostile := browser.do(http.MethodGet, "/api/oauth/state", "", site, map[string]string{"Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document"})
			require.Equal(t, http.StatusForbidden, hostile.Code, hostile.Body.String())
			require.Empty(t, hostile.Result().Cookies(), "hostile navigation rewrote the session")
			require.Equal(t, pending, browser.cookie)

			callback := browser.do(http.MethodGet, "/api/oauth/oidc?code=state-code&state="+state, "", "same-origin", nil)
			require.Equal(t, http.StatusOK, callback.Code, callback.Body.String())
			require.Contains(t, callback.Body.String(), `"success":true`)
			require.Equal(t, http.StatusOK, browser.selfStatus())
		})
	}
}

// TestSecurityOAuthStateLegitimateFetches is the positive control: same-origin
// fetches, user-initiated navigations, clients without Fetch Metadata and a
// configured external frontend origin can all obtain an OAuth state.
func TestSecurityOAuthStateLegitimateFetches(t *testing.T) {
	for _, tc := range []struct {
		name    string
		site    string
		headers map[string]string
	}{
		{name: "same_origin_fetch", site: "same-origin"},
		{name: "typed_navigation", site: "none", headers: map[string]string{"Sec-Fetch-Mode": "navigate"}},
		{name: "no_fetch_metadata"},
		{name: "trusted_frontend_origin", site: "same-site", headers: map[string]string{"Origin": "https://dashboard.gateway.test"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, engine, _ := csrfFixture(t)
			config.FrontendBaseURL = "https://dashboard.gateway.test"
			browser := &oauthBrowser{engine: engine}
			w := browser.do(http.MethodGet, "/api/oauth/state", "", tc.site, tc.headers)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), `"success":true`)
			require.NotNil(t, browser.cookie)
		})
	}
}
