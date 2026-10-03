package zai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/zhipu"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// withTraceDB points model.DB at a throwaway in-memory database so
// DoRequestHelper's best-effort trace bookkeeping does not panic on a nil DB.
// Mirrors relay/adaptor/endpoint_override_dispatch_test.go.
func withTraceDB(t *testing.T) {
	t.Helper()
	prev := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Trace{}))
	model.DB = db
	t.Cleanup(func() { model.DB = prev })
}

// TestSetupRequestHeaderUsesPlainBearer pins the single most dangerous inherited
// behavior: zhipu.SetupRequestHeader HS256 JWT-signs a dotted {id}.{secret} key
// and returns "" for anything else. A Z.AI key normally has no dot, so inheriting
// it would send an empty Authorization header and surface as an unexplained
// upstream 401 with nothing logged locally.
func TestSetupRequestHeaderUsesPlainBearer(t *testing.T) {
	t.Parallel()

	const dotlessKey = "zaikey123456"

	// Sanity-check the trap actually exists, so this test keeps its meaning if the
	// zhipu side ever changes.
	require.Empty(t, zhipu.GetToken(dotlessKey),
		"zhipu.GetToken must still return empty for a dotless key, otherwise this regression test is vacuous")

	a := &Adaptor{}
	req := httptest.NewRequest(http.MethodPost, "https://api.z.ai/api/paas/v4/chat/completions", nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	require.NoError(t, a.SetupRequestHeader(c, req, &meta.Meta{APIKey: dotlessKey}))

	got := req.Header.Get("Authorization")
	require.Equal(t, "Bearer "+dotlessKey, got)
	require.NotEmpty(t, got)
	require.False(t, strings.Contains(got, "eyJ"), "must not be a JWT: %q", got)
	require.Equal(t, "en-US,en", req.Header.Get("Accept-Language"))
}

// TestSetupRequestHeaderNeverJWTSignsDottedKey proves the JWT path is bypassed
// even for a key shaped like a BigModel credential.
func TestSetupRequestHeaderNeverJWTSignsDottedKey(t *testing.T) {
	t.Parallel()

	const dottedKey = "abc.def"

	a := &Adaptor{}
	req := httptest.NewRequest(http.MethodPost, "https://api.z.ai/api/paas/v4/chat/completions", nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	require.NoError(t, a.SetupRequestHeader(c, req, &meta.Meta{APIKey: dottedKey}))
	require.Equal(t, "Bearer "+dottedKey, req.Header.Get("Authorization"))
}

// TestDoRequestDispatchesOnOuterReceiver guards the embedding trap that no static
// check catches: zhipu.DoRequest calls adaptor.DoRequestHelper(a, ...) bound to the
// INNER receiver, so dropping the DoRequest override here would silently route Z.AI
// traffic through zhipu's JWT auth while every other override looks correct.
func TestDoRequestDispatchesOnOuterReceiver(t *testing.T) {
	withTraceDB(t)

	const key = "zaikey-outer-receiver"

	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")

	a := &Adaptor{}
	m := &meta.Meta{
		Mode:            relaymode.ChatCompletions,
		BaseURL:         upstream.URL,
		APIKey:          key,
		ActualModelName: "glm-4.7",
	}

	resp, err := a.DoRequest(c, m, strings.NewReader(`{"model":"glm-4.7"}`))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	require.Equal(t, "Bearer "+key, gotAuth,
		"DoRequest must re-dispatch on the outer receiver so SetupRequestHeader runs")
}

// TestGetRequestURL confirms the inherited zhipu URL builder produces Z.AI's
// documented paths when combined with the api.z.ai base URL.
func TestGetRequestURL(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		mode int
		want string
	}{
		{"chat", relaymode.ChatCompletions, "https://api.z.ai/api/paas/v4/chat/completions"},
		{"images", relaymode.ImagesGenerations, "https://api.z.ai/api/paas/v4/images/generations"},
		{"videos", relaymode.Videos, "https://api.z.ai/api/paas/v4/videos/generations"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &Adaptor{}
			got, err := a.GetRequestURL(&meta.Meta{
				Mode:            tc.mode,
				BaseURL:         "https://api.z.ai",
				ActualModelName: "glm-4.7",
			})
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestGetChannelNameIsConstantWithoutInit pins that controller/model.go, which
// builds a fresh non-Init'd adaptor when aggregating /v1/models, gets "zai" and
// not the embedded adaptor's name.
func TestGetChannelNameIsConstantWithoutInit(t *testing.T) {
	t.Parallel()

	require.Equal(t, "zai", (&Adaptor{}).GetChannelName())

	var iface adaptor.Adaptor = &Adaptor{}
	require.Equal(t, "zai", iface.GetChannelName())
}
