package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
)

const (
	// overrideChannelID and overrideAdminID identify the channel under edit and
	// the root administrator who saves it.
	overrideChannelID = 939951
	overrideAdminID   = 939952
	// overrideAdminToken is the administrator's dashboard access token.
	overrideAdminToken = "e2e-override-admin-access-token"
	// overrideKeptHeader is an unrelated config key (a channel custom header)
	// every stored config carries. It must survive every save and still reach
	// the provider, proving the editor round-trips the rest of config.
	overrideKeptHeader = "X-E2e-Kept-Config"
	// overrideCasesPath is the shared editor/backend contract fixture.
	overrideCasesPath = "../test/testdata/channel_other_override_cases.json"
	// overrideAliPath is the DashScope native text-generation path.
	overrideAliPath = "/api/v1/services/aigc/text-generation/generation"
)

// overrideState is the stored or submitted pair of the legacy `other` value
// and the typed config override (nil when the key is absent from config).
type overrideState struct {
	Other    string  `json:"other"`
	Override *string `json:"override"`
}

// overrideCase is one case of the shared contract fixture.
type overrideCase struct {
	Name        string        `json:"name"`
	Type        int           `json:"type"`
	OverrideKey string        `json:"override_key"`
	Stored      overrideState `json:"stored"`
	Edit        *string       `json:"edit"`
	Displayed   string        `json:"displayed"`
	Payload     overrideState `json:"payload"`
	Effective   string        `json:"effective"`
}

// overrideProvider describes how one channel type is served and observed.
type overrideProvider struct {
	// key is the typed config key that overrides `other` for this type.
	key string
	// baseURL and modelName configure the channel row and the client request.
	baseURL, modelName string
	// httpRelay reports whether the type is relayed over HTTP. Xunfei speaks
	// websockets, so only its resolved relay config is asserted.
	httpRelay bool
	// path is the upstream path an HTTP relay must reach.
	path string
	// observe asserts the value the provider actually received.
	observe func(t *testing.T, sent overrideUpstreamRequest, call e2eCall, effective string)
}

// overrideProviders maps every fixture channel type to its provider surface.
var overrideProviders = map[int]overrideProvider{
	channeltype.Azure: {
		key: "api_version", baseURL: e2eAzureBase, modelName: e2eMiniModel, httpRelay: true,
		path: "/openai/deployments/" + e2eMiniModel + "/chat/completions",
		observe: func(t *testing.T, sent overrideUpstreamRequest, _ e2eCall, effective string) {
			t.Helper()
			require.Equal(t, []string{effective}, sent.URL.Query()["api-version"], "Azure must receive the effective api-version; url=%s", sent.URL)
		},
	},
	channeltype.Ali: {
		key: "plugin", baseURL: "https://dashscope.aliyuncs.com", modelName: "qwen-plus", httpRelay: true,
		path: overrideAliPath,
		observe: func(t *testing.T, sent overrideUpstreamRequest, _ e2eCall, effective string) {
			t.Helper()
			if effective == "" {
				require.Empty(t, sent.Header.Values("X-DashScope-Plugin"), "an empty plugin must not be sent to DashScope")
				return
			}
			require.Equal(t, []string{effective}, sent.Header.Values("X-DashScope-Plugin"), "DashScope must receive the effective plugin")
		},
	},
	channeltype.AIProxyLibrary: {
		key: "library_id", baseURL: e2eAIProxyBase, modelName: e2eMiniModel, httpRelay: true,
		path: "/api/library/ask",
		observe: func(t *testing.T, _ overrideUpstreamRequest, call e2eCall, effective string) {
			t.Helper()
			require.Equal(t, effective, call.Body["libraryId"], "the AIProxy library must receive the effective libraryId; body=%s", call.Raw)
		},
	},
	channeltype.Xunfei: {key: "api_version", baseURL: "https://spark-api.xf-yun.com", modelName: "Spark-Lite"},
}

// overrideUpstreamRequest is the URL and headers of one dispatched request.
type overrideUpstreamRequest struct {
	URL    *url.URL
	Header http.Header
}

// overrideUpstreamRecorder wraps the fake upstream's transport to capture the
// query and headers e2eUpstream does not record.
type overrideUpstreamRecorder struct {
	inner    http.RoundTripper
	mu       sync.Mutex
	requests []overrideUpstreamRequest
}

// RoundTrip records req and forwards it unchanged. Parameters: req is the
// outbound provider request. Returns: the inner transport's response or its
// wrapped error.
func (r *overrideUpstreamRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	sentURL := *req.URL
	r.mu.Lock()
	r.requests = append(r.requests, overrideUpstreamRequest{URL: &sentURL, Header: req.Header.Clone()})
	r.mu.Unlock()
	resp, err := r.inner.RoundTrip(req)
	if err != nil {
		return nil, errors.Wrap(err, "forward recorded upstream request")
	}
	return resp, nil
}

// sent returns a copy of every recorded request in dispatch order.
func (r *overrideUpstreamRecorder) sent() []overrideUpstreamRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]overrideUpstreamRequest(nil), r.requests...)
}

// overrideEnv is one gateway with the channel under edit, the admin API and a
// recording upstream transport.
type overrideEnv struct {
	*e2eEnv
	admin    *gin.Engine
	probe    *gin.Engine
	recorder *overrideUpstreamRecorder
	// probed is the relay config the distributor resolved on the last probe.
	probed model.ChannelConfig
}

// loadOverrideCases reads the shared contract fixture. Parameters: t owns
// assertions. Returns: every case in file order.
func loadOverrideCases(t *testing.T) []overrideCase {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(overrideCasesPath))
	require.NoError(t, err)
	var fixture struct {
		Cases []overrideCase `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(raw, &fixture))
	require.NotEmpty(t, fixture.Cases, "the contract fixture must list cases")
	return fixture.Cases
}

// resolveOverride applies the distributor precedence: a non-empty override
// wins, otherwise the legacy value is used. Parameters: state is the pair.
// Returns: the effective value.
func resolveOverride(state overrideState) string {
	if state.Override != nil && *state.Override != "" {
		return *state.Override
	}
	return state.Other
}

// overrideConfigJSON renders a channel config holding the kept custom header
// and, when override is non-nil, the override key. Parameters: t owns
// assertions, key is the override key and override its value. Returns: JSON.
func overrideConfigJSON(t *testing.T, key string, override *string) string {
	t.Helper()
	cfg := map[string]any{"custom_headers": map[string]string{overrideKeptHeader: "kept"}}
	if override != nil {
		cfg[key] = *override
	}
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	return string(raw)
}

// newOverrideEnv stores the channel of tc in state stored and mounts the admin
// API, a distributor probe and the relay on one ledger. Parameters: t owns
// cleanup, tc names the channel type and stored is its persisted state.
// Returns: the env.
func newOverrideEnv(t *testing.T, tc overrideCase, stored overrideState) *overrideEnv {
	t.Helper()
	provider := overrideProviders[tc.Type]
	env := &overrideEnv{e2eEnv: newE2EEnv(t, func(call e2eCall) e2eReply {
		if call.Path == overrideAliPath {
			return e2eReply{Body: `{"output":{"choices":[{"index":0,"message":{"role":"assistant","content":"ack"},"finish_reason":"stop"}]},` +
				`"usage":{"input_tokens":12,"output_tokens":3,"total_tokens":15},"request_id":"e2e-ali"}`}
		}
		return e2eSuccess(call, provider.modelName, "", e2eUsage{Input: 12, Output: 3})
	})}
	// newE2EEnv's cleanup restores the client it found, so no extra cleanup is needed.
	base := *client.HTTPClient
	env.recorder = &overrideUpstreamRecorder{inner: base.Transport}
	base.Transport = env.recorder
	client.HTTPClient = &base

	other, baseURL, priority := stored.Other, provider.baseURL, int64(0)
	row := &model.Channel{Id: overrideChannelID, UUID: uuid.NewString(), Type: tc.Type, Status: model.ChannelStatusEnabled,
		Name: "override-" + tc.Name, Models: provider.modelName, Group: "default", BaseURL: &baseURL, Key: overrideChannelKey(tc.Type),
		Priority: &priority, Other: &other, Config: overrideConfigJSON(t, provider.key, stored.Override)}
	require.NoError(t, model.DB.Create(row).Error)
	require.NoError(t, row.AddAbilities())
	require.NoError(t, model.DB.Create(&model.User{Id: overrideAdminID, UUID: uuid.NewString(), Username: "e2e-override-admin",
		Password: "unused-test-password", AccessToken: overrideAdminToken, AffCode: "e2e-override-aff", Role: model.RoleRootUser, Status: model.UserStatusEnabled, Group: "default"}).Error)

	env.admin = gin.New()
	env.admin.Use(func(c *gin.Context) {
		gmw.SetLogger(c, logger.Logger)
		c.Next()
	})
	env.admin.Use(sessions.Sessions("e2e-override", cookie.NewStore([]byte("e2e-override-cookie-signing-secret!"))))
	SetApiRouter(env.admin)

	// The probe runs the shipped token auth and distributor, then records the
	// relay config the adaptor would read, without dialing the provider.
	env.probe = gin.New()
	env.probe.Use(func(c *gin.Context) {
		gmw.SetLogger(c, logger.Logger)
		c.Next()
	})
	env.probe.Use(middleware.RequestId())
	env.probe.POST("/v1/chat/completions", middleware.TokenAuth(), middleware.Distribute(), func(c *gin.Context) {
		env.probed = meta.GetByContext(c).Config
		c.Status(http.StatusNoContent)
	})
	return env
}

// overrideChannelKey returns a well-formed fixture credential for a type.
func overrideChannelKey(channelType int) string {
	if channelType == channeltype.Xunfei {
		return "e2e-app|e2e-secret|e2e-key"
	}
	return fmt.Sprintf("e2e-override-%d", channelType)
}

// save submits payload through PUT /api/channel/ exactly as the editors do:
// the stored config with the override key set or removed, the new `other`,
// a renamed channel and no key (an empty key keeps the stored secret).
// Parameters: t owns assertions, tc names the channel and payload is the
// submitted pair. Returns: the persisted channel row.
func (e *overrideEnv) save(t *testing.T, tc overrideCase, payload overrideState) model.Channel {
	t.Helper()
	provider := overrideProviders[tc.Type]
	var stored model.Channel
	require.NoError(t, model.DB.Where("id = ?", overrideChannelID).Take(&stored).Error)
	body, err := json.Marshal(map[string]any{
		"uuid": stored.UUID, "name": "override-saved", "type": tc.Type, "base_url": stored.GetBaseURL(), "models": stored.Models,
		"group": "default", "other": payload.Other, "config": overrideConfigJSON(t, provider.key, payload.Override),
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/api/channel/", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+overrideAdminToken)
	w := httptest.NewRecorder()
	e.admin.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"success":true`, w.Body.String())

	var saved model.Channel
	require.NoError(t, model.DB.Where("id = ?", overrideChannelID).Take(&saved).Error)
	return saved
}

// requirePersisted asserts the saved row holds payload unchanged and kept the
// unrelated config key and the stored key. Parameters: t owns assertions, tc
// names the channel, payload is the submitted pair and saved the read-back row.
func requirePersisted(t *testing.T, tc overrideCase, payload overrideState, saved model.Channel) {
	t.Helper()
	provider := overrideProviders[tc.Type]
	require.Equal(t, "override-saved", saved.Name, "the save must be applied")
	require.Equal(t, overrideChannelKey(tc.Type), saved.Key, "an omitted key keeps the stored secret")
	require.NotNil(t, saved.Other, "other must be persisted")
	require.Equal(t, payload.Other, *saved.Other, "persisted other")
	var cfg map[string]any
	require.NoError(t, json.Unmarshal([]byte(saved.Config), &cfg), "persisted config=%s", saved.Config)
	value, present := cfg[provider.key]
	if payload.Override == nil {
		require.False(t, present, "the override key must be absent; config=%s", saved.Config)
	} else {
		require.True(t, present, "the override key must be present; config=%s", saved.Config)
		require.Equal(t, *payload.Override, value, "persisted config.%s", provider.key)
	}
	require.Equal(t, map[string]any{overrideKeptHeader: "kept"}, cfg["custom_headers"], "unrelated config keys must survive the save")
}

// requireEffective drives the persisted channel through the distributor and,
// for HTTP providers, a real relay, asserting the provider uses effective.
// Parameters: t owns assertions, tc names the channel and effective is the
// value the relay must use.
func (e *overrideEnv) requireEffective(t *testing.T, tc overrideCase, effective string) {
	t.Helper()
	provider := overrideProviders[tc.Type]
	body := `{"model":"` + provider.modelName + `","messages":[{"role":"user","content":"hello"}],"max_tokens":` + fmt.Sprint(e2eMaxTokens) + `}`

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer sk-"+e.key)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.probe.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code, "the distributor must select the saved channel; body=%s", w.Body.String())
	resolved := map[string]string{"api_version": e.probed.APIVersion, "library_id": e.probed.LibraryID, "plugin": e.probed.Plugin}
	require.Equal(t, effective, resolved[provider.key], "resolved relay config.%s", provider.key)
	require.Equal(t, "kept", e.probed.CustomHeaders[overrideKeptHeader], "unrelated config keys reach the relay")
	if !provider.httpRelay {
		return
	}

	callsBefore, sentBefore := len(e.upstream.callsFor(overrideChannelKey(tc.Type))), len(e.recorder.sent())
	res := e.post(t, "/v1/chat/completions", body)
	require.Equal(t, http.StatusOK, res.Code, res.Body)
	calls := e.upstream.callsFor(overrideChannelKey(tc.Type))
	require.Len(t, calls, callsBefore+1, "the saved channel serves the request exactly once")
	call := calls[len(calls)-1]
	require.Equal(t, provider.path, call.Path)
	sent := e.recorder.sent()
	require.Len(t, sent, sentBefore+1, "exactly one provider request is dispatched")
	last := sent[len(sent)-1]
	require.Equal(t, []string{"kept"}, last.Header.Values(overrideKeptHeader), "the kept custom header reaches the provider")
	provider.observe(t, last, call, effective)
}

// TestE2EChannelOtherOverrideContract proves the backend half of the shared
// editor contract for every fixture case: the stored channel is saved through
// the shipped admin API with the payload the editors send, the row holds that
// payload unchanged, and the relay (distributor plus the real provider
// request) uses the effective value. A final case pins the reviewer's bug at
// the backend boundary: an editor that edits only `other` while a conflicting
// override is stored leaves the provider on the stale override.
func TestE2EChannelOtherOverrideContract(t *testing.T) {
	cases := loadOverrideCases(t)
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			provider, ok := overrideProviders[tc.Type]
			require.True(t, ok, "fixture type %d has no provider surface", tc.Type)
			require.Equal(t, provider.key, tc.OverrideKey, "the fixture override key must match the distributor rule")
			require.Equal(t, resolveOverride(tc.Stored), tc.Displayed, "displayed must be the stored effective value")
			require.Equal(t, resolveOverride(tc.Payload), tc.Effective, "effective must follow distributor precedence")
			if tc.Edit == nil {
				require.Equal(t, tc.Stored, tc.Payload, "an unrelated save must leave other and the override untouched")
			} else {
				require.Equal(t, *tc.Edit, tc.Payload.Other, "an explicit edit writes other")
				require.Equal(t, *tc.Edit, tc.Effective, "an explicit edit must take effect")
			}

			env := newOverrideEnv(t, tc, tc.Stored)
			// Baseline: before the save the relay uses what the editor displays.
			env.requireEffective(t, tc, tc.Displayed)

			saved := env.save(t, tc, tc.Payload)
			requirePersisted(t, tc, tc.Payload, saved)
			env.requireEffective(t, tc, tc.Effective)
		})
	}

	t.Run("pre_fix_editor_payload_keeps_stale_override", func(t *testing.T) {
		var conflict overrideCase
		for _, tc := range cases {
			if tc.Name == "azure_conflict_edit" {
				conflict = tc
			}
		}
		require.NotNil(t, conflict.Edit, "the fixture must keep azure_conflict_edit")
		require.NotNil(t, conflict.Stored.Override)
		require.NotEqual(t, *conflict.Edit, *conflict.Stored.Override, "the case must conflict to show the edit is ignored")
		// The pre-fix editors wrote only `other` and round-tripped config, so
		// the stored override keeps winning and the typed value never applies.
		stale := overrideState{Other: *conflict.Edit, Override: conflict.Stored.Override}

		env := newOverrideEnv(t, conflict, conflict.Stored)
		saved := env.save(t, conflict, stale)
		requirePersisted(t, conflict, stale, saved)
		env.requireEffective(t, conflict, *conflict.Stored.Override)
	})
}
