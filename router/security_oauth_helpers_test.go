package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gcrypto "github.com/Laisky/go-utils/v6/crypto"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
)

// oauthFixtureTotpSecret is a fixed base32 TOTP secret used by OAuth 2FA tests.
const oauthFixtureTotpSecret = "JBSWY3DPEHPK3PXP"

// oauthIdPStub serves every outbound identity-provider call in process, so the
// production GitHub, OIDC, Lark and WeChat exchanges run unchanged without
// network access. Each authorization code maps to one provider identity.
type oauthIdPStub struct {
	mu         sync.Mutex
	identities map[string]string
	calls      atomic.Int32
}

// RoundTrip answers req from the in-process provider stub. It records the call
// and returns the stub response; it never returns an error.
func (s *oauthIdPStub) RoundTrip(req *http.Request) (*http.Response, error) {
	s.calls.Add(1)
	recorder := httptest.NewRecorder()
	s.serve(recorder, req)
	response := recorder.Result()
	response.Request = req
	return response, nil
}

// identity returns the provider identity mapped to code, or an empty string.
func (s *oauthIdPStub) identity(code string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.identities[code]
}

// serve implements the minimal token and user-info endpoints of each provider.
// Token endpoints echo the code as the access token; user-info endpoints map the
// bearer token back to the configured identity.
func (s *oauthIdPStub) serve(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	bearer := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	switch req.URL.Host + req.URL.Path {
	case "wechat.idp.test/api/wechat/user":
		id := s.identity(req.URL.Query().Get("code"))
		writeStubJSON(w, map[string]any{"success": id != "", "message": "invalid code", "data": id})
	case "oidc.idp.test/token":
		if err := req.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		writeStubJSON(w, map[string]any{"access_token": req.PostForm.Get("code")})
	case "github.com/login/oauth/access_token", "open.feishu.cn/open-apis/authen/v2/oauth/token":
		var body map[string]string
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		writeStubJSON(w, map[string]any{"access_token": body["code"]})
	case "oidc.idp.test/userinfo":
		writeStubJSON(w, map[string]any{"sub": s.identity(bearer)})
	case "api.github.com/user":
		writeStubJSON(w, map[string]any{"login": s.identity(bearer)})
	case "passport.feishu.cn/suite/passport/oauth/userinfo":
		writeStubJSON(w, map[string]any{"open_id": s.identity(bearer)})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// writeStubJSON encodes payload as the stub response body.
func writeStubJSON(w http.ResponseWriter, payload map[string]any) {
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// installOAuthIdPStub enables every OAuth provider, routes all default-transport
// traffic to an in-process stub keyed by authorization code, and restores the
// previous configuration when the test ends. It returns the stub for call counts.
func installOAuthIdPStub(t *testing.T, identities map[string]string) *oauthIdPStub {
	t.Helper()
	stub := &oauthIdPStub{identities: identities}
	oldTransport := http.DefaultTransport
	oldWeChat, oldWeChatAddress := config.WeChatAuthEnabled, config.WeChatServerAddress
	oldOidc, oldToken, oldUserinfo := config.OidcEnabled, config.OidcTokenEndpoint, config.OidcUserinfoEndpoint
	oldGitHub := config.GitHubOAuthEnabled
	http.DefaultTransport = stub
	config.WeChatAuthEnabled, config.WeChatServerAddress = true, "https://wechat.idp.test"
	config.OidcEnabled, config.OidcTokenEndpoint, config.OidcUserinfoEndpoint = true, "https://oidc.idp.test/token", "https://oidc.idp.test/userinfo"
	config.GitHubOAuthEnabled = true
	t.Cleanup(func() {
		http.DefaultTransport = oldTransport
		config.WeChatAuthEnabled, config.WeChatServerAddress = oldWeChat, oldWeChatAddress
		config.OidcEnabled, config.OidcTokenEndpoint, config.OidcUserinfoEndpoint = oldOidc, oldToken, oldUserinfo
		config.GitHubOAuthEnabled = oldGitHub
	})
	return stub
}

// oauthBrowser models one browser profile: it carries the fixture session cookie
// between requests and applies every Set-Cookie the server returns.
type oauthBrowser struct {
	engine *gin.Engine
	cookie *http.Cookie
}

// do sends one request to the API with the given Fetch Metadata site and extra
// headers, updates the stored session cookie and returns the recorder.
func (b *oauthBrowser) do(method, path, body, site string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "https://gateway.test"+path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if site != "" {
		request.Header.Set("Sec-Fetch-Site", site)
	}
	if site == "same-origin" && method != http.MethodGet {
		request.Header.Set("Origin", "https://gateway.test")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	if b.cookie != nil {
		request.AddCookie(b.cookie)
	}
	w := httptest.NewRecorder()
	b.engine.ServeHTTP(w, request)
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name != "csrf-fixture" {
			continue
		}
		if cookie.MaxAge < 0 {
			b.cookie = nil
		} else {
			b.cookie = cookie
		}
	}
	return w
}

// oauthState fetches a state value with a same-origin fetch, as the dashboard
// does before redirecting to a provider, and returns it.
func (b *oauthBrowser) oauthState(t *testing.T) string {
	t.Helper()
	w := b.do(http.MethodGet, "/api/oauth/state", "", "same-origin", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var payload struct {
		Success bool   `json:"success"`
		Data    string `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.NotEmpty(t, payload.Data)
	return payload.Data
}

// selfStatus returns the HTTP status of an authenticated account read, which is
// 200 only when the browser holds an authenticated dashboard session.
func (b *oauthBrowser) selfStatus() int {
	return b.do(http.MethodGet, "/api/user/self", "", "same-origin", nil).Code
}

// registerOAuthSessionIssuer adds a test-only route that signs a dashboard
// session for the user id in the query string, mirroring a session issued
// while the account was still active. A non-numeric id is stored verbatim to
// model a malformed session.
func registerOAuthSessionIssuer(t *testing.T, engine *gin.Engine) {
	t.Helper()
	engine.POST("/issue-oauth-session", func(c *gin.Context) {
		session := sessions.Default(c)
		var id any = c.Query("id")
		if parsed, err := strconv.Atoi(c.Query("id")); err == nil {
			id = parsed
		}
		session.Set("id", id)
		session.Set("username", c.Query("username"))
		session.Set("role", 1)
		session.Set("status", 1)
		require.NoError(t, session.Save())
		c.Status(http.StatusNoContent)
	})
}

// issueOAuthSession returns a browser holding a signed session for id/username.
func issueOAuthSession(t *testing.T, engine *gin.Engine, id, username string) *oauthBrowser {
	t.Helper()
	browser := &oauthBrowser{engine: engine}
	w := browser.do(http.MethodPost, "/issue-oauth-session?id="+url.QueryEscape(id)+"&username="+url.QueryEscape(username), "", "", nil)
	require.Equal(t, http.StatusNoContent, w.Code)
	require.NotNil(t, browser.cookie)
	return browser
}

// oauthFixtureIDs hands out account ids that are unique within the process.
var oauthFixtureIDs atomic.Int64

func init() {
	oauthFixtureIDs.Store(time.Now().UTC().UnixNano()%1_000_000_000 + 1_000_000)
}

// oauthFixtureUser returns an enabled TOTP account with a process-unique id and
// UUID, so the global TOTP replay cache never sees a code from an earlier run.
func oauthFixtureUser(username string) model.User {
	id := oauthFixtureIDs.Add(1)
	return model.User{
		Id: int(id), UUID: fmt.Sprintf("018f0000-0000-7000-8000-%012d", id), Username: username,
		Password: "unused-test-password", TotpSecret: oauthFixtureTotpSecret, AffCode: fmt.Sprintf("oa%d", id),
		Role: model.RoleCommonUser, Status: model.UserStatusEnabled, Group: "default",
	}
}

// fixtureTotpCode returns the current TOTP code for the fixture secret. The
// server accepts only the current 30-second step, so the helper first waits out
// the final seconds of a step; otherwise a step boundary between generating and
// verifying the code makes the test flaky under load.
func fixtureTotpCode(t *testing.T) string {
	t.Helper()
	waitForStableTotpStep()
	totp, err := gcrypto.NewTOTP(gcrypto.OTPArgs{Base32Secret: oauthFixtureTotpSecret})
	require.NoError(t, err)
	return totp.Key()
}

// wrongTotpCode returns a six-digit code that differs from code.
func wrongTotpCode(code string) string {
	if code == "000000" {
		return "111111"
	}
	return "000000"
}

// waitForStableTotpStep sleeps until at least five seconds remain in the
// current 30-second TOTP step, so a freshly generated code is still current
// when the server verifies it.
func waitForStableTotpStep() {
	const step, margin = int64(30), int64(5)
	if remaining := step - time.Now().UTC().Unix()%step; remaining <= margin {
		time.Sleep(time.Duration(remaining)*time.Second + 100*time.Millisecond)
	}
}
