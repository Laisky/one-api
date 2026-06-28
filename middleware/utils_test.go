package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

func TestGetTokenKeyParts_ConfiguredPrefix(t *testing.T) {
	old := config.TokenKeyPrefix
	config.TokenKeyPrefix = "sk-"
	defer func() { config.TokenKeyPrefix = old }()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer sk-abc-123")
	c.Request = req

	parts := GetTokenKeyParts(c)
	require.GreaterOrEqual(t, len(parts), 2, "unexpected parts: %#v", parts)
	require.Equal(t, "abc", parts[0], "unexpected parts: %#v", parts)
	require.Equal(t, "123", parts[1], "unexpected parts: %#v", parts)
}

func TestGetTokenKeyParts_LegacyPrefix(t *testing.T) {
	old := config.TokenKeyPrefix
	config.TokenKeyPrefix = "custom-"
	defer func() { config.TokenKeyPrefix = old }()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer sk-abc-456")
	c.Request = req

	parts := GetTokenKeyParts(c)
	require.GreaterOrEqual(t, len(parts), 2, "unexpected parts for legacy: %#v", parts)
	require.Equal(t, "abc", parts[0], "unexpected parts for legacy: %#v", parts)
	require.Equal(t, "456", parts[1], "unexpected parts for legacy: %#v", parts)
}

func TestGetTokenKeyParts_WebSocketSubprotocol(t *testing.T) {
	old := config.TokenKeyPrefix
	config.TokenKeyPrefix = "sk-"
	defer func() { config.TokenKeyPrefix = old }()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/v1/realtime?model=gpt-4o-realtime-preview", nil)
	// Browser WebSocket auth via subprotocol (no Authorization header)
	req.Header.Set("Sec-WebSocket-Protocol", "realtime, openai-insecure-api-key.sk-abc-123, openai-beta.realtime-v1")
	c.Request = req

	parts := GetTokenKeyParts(c)
	require.GreaterOrEqual(t, len(parts), 2, "unexpected parts from subprotocol: %#v", parts)
	require.Equal(t, "abc", parts[0], "unexpected parts from subprotocol: %#v", parts)
	require.Equal(t, "123", parts[1], "unexpected parts from subprotocol: %#v", parts)
}

func TestGetTokenKeyParts_AuthorizationTakesPrecedenceOverSubprotocol(t *testing.T) {
	old := config.TokenKeyPrefix
	config.TokenKeyPrefix = "sk-"
	defer func() { config.TokenKeyPrefix = old }()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/v1/realtime?model=gpt-4o-realtime-preview", nil)
	req.Header.Set("Authorization", "Bearer sk-header-token")
	req.Header.Set("Sec-WebSocket-Protocol", "realtime, openai-insecure-api-key.sk-subproto-token, openai-beta.realtime-v1")
	c.Request = req

	parts := GetTokenKeyParts(c)
	// Authorization header should take precedence
	require.Equal(t, "header", parts[0], "Authorization should take precedence: %#v", parts)
}

func TestGetTokenKeyParts_SubprotocolWithoutPrefix(t *testing.T) {
	old := config.TokenKeyPrefix
	config.TokenKeyPrefix = ""
	defer func() { config.TokenKeyPrefix = old }()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/v1/realtime", nil)
	req.Header.Set("Sec-WebSocket-Protocol", "realtime, openai-insecure-api-key.mytoken123, openai-beta.realtime-v1")
	c.Request = req

	parts := GetTokenKeyParts(c)
	require.Equal(t, "mytoken123", parts[0], "unexpected parts: %#v", parts)
}

func TestGetTokenKeyParts_NoAuthAtAll(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/v1/realtime", nil)
	c.Request = req

	parts := GetTokenKeyParts(c)
	// Should return [""] when no auth is provided
	require.Equal(t, []string{""}, parts)
}

// realToken mirrors the shape produced by random.GenerateKey: 16 random
// alphanumerics followed by a 32-char hyphen-free hex UUID. The absence of
// hyphens is important — see TestParseTokenKey_RealTokenNeverSplits.
const realToken = "k24tal3x3eN6SjKhD78e85Fc4dD648F1B0781aF435455642"

// TestParseTokenKey_HeaderVariants verifies the credential is correctly
// extracted from every supported header form, that the `Bearer` scheme is
// matched case-insensitively (RFC 7235), and that the diagnostic metadata
// (Source/HadScheme) is reported accurately. These are the exact variants a
// third-party client such as GitHub Copilot's BYOK provider may send.
func TestParseTokenKey_HeaderVariants(t *testing.T) {
	old := config.TokenKeyPrefix
	config.TokenKeyPrefix = "sk-"
	defer func() { config.TokenKeyPrefix = old }()

	cases := []struct {
		name       string
		headers    map[string]string
		wantSource authTokenSource
		wantScheme bool
	}{
		{"standard Bearer", map[string]string{"Authorization": "Bearer sk-" + realToken}, authSourceAuthorization, true},
		{"lowercase bearer scheme", map[string]string{"Authorization": "bearer sk-" + realToken}, authSourceAuthorization, true},
		{"uppercase BEARER scheme", map[string]string{"Authorization": "BEARER sk-" + realToken}, authSourceAuthorization, true},
		{"mixed-case BeArEr scheme", map[string]string{"Authorization": "BeArEr sk-" + realToken}, authSourceAuthorization, true},
		{"extra spaces after scheme", map[string]string{"Authorization": "Bearer   sk-" + realToken}, authSourceAuthorization, true},
		{"surrounding whitespace", map[string]string{"Authorization": "  Bearer sk-" + realToken + "  "}, authSourceAuthorization, true},
		{"bare key without scheme", map[string]string{"Authorization": "sk-" + realToken}, authSourceAuthorization, false},
		{"anthropic X-Api-Key", map[string]string{"X-Api-Key": "sk-" + realToken}, authSourceXAPIKey, false},
		{"azure Api-Key (Copilot azure provider)", map[string]string{"Api-Key": "sk-" + realToken}, authSourceAPIKey, false},
		{"azure Api-Key without prefix", map[string]string{"Api-Key": realToken}, authSourceAPIKey, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			c.Request = req

			got := parseTokenKey(c)
			require.Equal(t, tc.wantSource, got.Source, "source for %q", tc.name)
			require.Equal(t, tc.wantScheme, got.HadScheme, "hadScheme for %q", tc.name)
			// All variants must resolve to exactly the token, with no spurious
			// channel-spec split that would 403 a non-admin user.
			require.Len(t, got.Parts, 1, "variant %q split into channel-spec: %#v", tc.name, got.Parts)
			require.Equal(t, realToken, got.Parts[0], "variant %q", tc.name)
		})
	}
}

// TestParseTokenKey_RealTokenNeverSplits is the core regression guard for the
// GitHub Copilot 403 report: a normally-generated, hyphen-free token must
// always parse to a single part regardless of the (case-insensitive) scheme,
// so non-admin callers never hit the "Ordinary users do not support specifying
// channels" 403 in TokenAuth.
func TestParseTokenKey_RealTokenNeverSplits(t *testing.T) {
	old := config.TokenKeyPrefix
	config.TokenKeyPrefix = "sk-"
	defer func() { config.TokenKeyPrefix = old }()

	for _, scheme := range []string{"Bearer ", "bearer ", "BEARER ", "BeArEr ", ""} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		req.Header.Set("Authorization", scheme+"sk-"+realToken)
		c.Request = req

		parts := GetTokenKeyParts(c)
		require.Len(t, parts, 1, "scheme %q produced channel-spec split: %#v", scheme, parts)
		require.Equal(t, realToken, parts[0], "scheme %q", scheme)
	}
}

// TestParseTokenKey_HeaderPrecedence verifies the precedence order
// Authorization > X-Api-Key > Api-Key > WebSocket subprotocol.
func TestParseTokenKey_HeaderPrecedence(t *testing.T) {
	old := config.TokenKeyPrefix
	config.TokenKeyPrefix = "sk-"
	defer func() { config.TokenKeyPrefix = old }()

	build := func(headers map[string]string) *gin.Context {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("GET", "/v1/realtime", nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		c.Request = req
		return c
	}

	// Authorization wins over everything else.
	c := build(map[string]string{
		"Authorization":          "Bearer sk-authkey",
		"X-Api-Key":              "sk-anthropickey",
		"Api-Key":                "sk-azurekey",
		"Sec-WebSocket-Protocol": "realtime, openai-insecure-api-key.sk-wskey, openai-beta.realtime-v1",
	})
	got := parseTokenKey(c)
	require.Equal(t, "authkey", got.Parts[0])
	require.Equal(t, authSourceAuthorization, got.Source)

	// Without Authorization, X-Api-Key wins.
	got = parseTokenKey(build(map[string]string{
		"X-Api-Key": "sk-anthropickey",
		"Api-Key":   "sk-azurekey",
	}))
	require.Equal(t, "anthropickey", got.Parts[0])
	require.Equal(t, authSourceXAPIKey, got.Source)

	// Without Authorization/X-Api-Key, Azure Api-Key wins over subprotocol.
	got = parseTokenKey(build(map[string]string{
		"Api-Key":                "sk-azurekey",
		"Sec-WebSocket-Protocol": "realtime, openai-insecure-api-key.sk-wskey, openai-beta.realtime-v1",
	}))
	require.Equal(t, "azurekey", got.Parts[0])
	require.Equal(t, authSourceAPIKey, got.Source)
}

// TestParseTokenKey_NoCredential ensures the no-auth case is reported as such
// (it resolves to 401 downstream, never a 403).
func TestParseTokenKey_NoCredential(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/v1/models", nil)
	c.Request = req

	got := parseTokenKey(c)
	require.Equal(t, authSourceNone, got.Source)
	require.False(t, got.HadScheme)
	require.Equal(t, []string{""}, got.Parts)
}

// TestStripAuthScheme covers the scheme-stripping helper directly.
func TestStripAuthScheme(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Bearer abc", "abc"},
		{"bearer abc", "abc"},
		{"BEARER abc", "abc"},
		{"Bearer   abc", "abc"},
		{"abc", "abc"},          // no scheme
		{"Bearer", "Bearer"},    // scheme word without trailing space/key
		{"Bearerabc", "Bearerabc"}, // not a scheme (no separating space)
		{"", ""},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, stripAuthScheme(tc.in), "input %q", tc.in)
	}
}

func TestShouldLogAsWarning_ClientErrorStatus(t *testing.T) {
	err := errors.New("No token provided")

	shouldWarn := shouldLogAsWarning(http.StatusUnauthorized, err)
	require.True(t, shouldWarn)
}

func TestShouldLogAsWarning_ServerErrorStatus(t *testing.T) {
	err := errors.New("database unavailable")

	shouldWarn := shouldLogAsWarning(http.StatusInternalServerError, err)
	require.False(t, shouldWarn)
}

func TestShouldLogAsWarning_IgnoredErrorPattern(t *testing.T) {
	err := errors.New("token not found for key: abc")

	shouldWarn := shouldLogAsWarning(http.StatusInternalServerError, err)
	require.True(t, shouldWarn)
}

func TestShouldLogAsWarning_NoAvailableChannels(t *testing.T) {
	err := errors.New("No available channels for Model glm-4.6v-flash under Group default")

	shouldWarn := shouldLogAsWarning(http.StatusServiceUnavailable, err)
	require.True(t, shouldWarn)
}
