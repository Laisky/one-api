package router

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/graceful"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

const (
	passthroughKeysUserID  = 929291
	passthroughKeysTokenID = 929292
	passthroughKeysQuota   = 50_000_000
	// passthroughKeysAllowedModel is the only model the token, channel and
	// ledger admit. passthroughKeysPremiumModel is what a smuggled field asks
	// the upstream to serve instead.
	passthroughKeysAllowedModel = "gpt-4o-mini"
	passthroughKeysPremiumModel = "gpt-4o"
)

// extraBodyGatewayCapture records what an OpenAI-compatible upstream observed.
// The fixture behaves like a LiteLLM-style gateway: it reads keys exactly
// (case-sensitive, like Python/Node JSON) and applies a wire `extra_body`
// object with OpenAI-SDK semantics, where its keys override the root.
type extraBodyGatewayCapture struct {
	mu     sync.Mutex
	bodies [][]byte
	served []string
	n      []json.RawMessage
}

// snapshot returns copies of the recorded raw bodies, effective models and
// effective n values in arrival order.
func (g *extraBodyGatewayCapture) snapshot() ([][]byte, []string, []json.RawMessage) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([][]byte(nil), g.bodies...), append([]string(nil), g.served...), append([]json.RawMessage(nil), g.n...)
}

// newExtraBodyGateway starts the upstream fixture and returns its server and capture.
func newExtraBodyGateway(t *testing.T) (*httptest.Server, *extraBodyGatewayCapture) {
	t.Helper()
	capture := &extraBodyGatewayCapture{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read failed", http.StatusBadRequest)
			return
		}
		var root map[string]json.RawMessage
		if err := json.Unmarshal(raw, &root); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if extra, ok := root["extra_body"]; ok {
			var extraFields map[string]json.RawMessage
			if err := json.Unmarshal(extra, &extraFields); err == nil {
				for key, value := range extraFields {
					root[key] = value
				}
			}
			delete(root, "extra_body")
		}
		var served string
		_ = json.Unmarshal(root["model"], &served)
		capture.mu.Lock()
		capture.bodies = append(capture.bodies, raw)
		capture.served = append(capture.served, served)
		capture.n = append(capture.n, root["n"])
		capture.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chatcmpl-keys","object":"chat.completion","created":1767225600,"model":"`+served+`","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`)
	}))
	t.Cleanup(server.Close)
	return server, capture
}

// passthroughKeysRouter builds the shipped relay router with real token auth,
// distributor and SQLite ledger. The token and the OpenAI-compatible channel
// admit only passthroughKeysAllowedModel. ENFORCE_INCLUDE_USAGE=false selects
// the raw OpenAI-compatible chat passthrough branch under test.
func passthroughKeysRouter(t *testing.T, base string) (*gin.Engine, string) {
	t.Helper()
	return ambiguousKeysRouter(t, base, channeltype.OpenAICompatible, passthroughKeysAllowedModel, false)
}

// ambiguousKeysRouter builds the shipped relay router with real token auth,
// distributor and SQLite ledger for one channel of channelType serving models
// (comma separated). The token admits the same models. enforceIncludeUsage
// sets ENFORCE_INCLUDE_USAGE (true is the shipped default).
func ambiguousKeysRouter(t *testing.T, base string, channelType int, models string, enforceIncludeUsage bool) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	require.NoError(t, err)
	conn, err := db.DB()
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	oldDB, oldLOG, oldClient := model.DB, model.LOG_DB, client.HTTPClient
	oldRedis, oldSQLite, oldMemory, oldRate, oldLogging := common.IsRedisEnabled(), common.UsingSQLite.Load(), config.MemoryCacheEnabled, config.RateLimitDisabled, config.IsLogConsumeEnabled()
	oldEnforceUsage := config.EnforceIncludeUsage
	model.DB, model.LOG_DB = db, db
	common.SetRedisEnabled(false)
	common.UsingSQLite.Store(true)
	config.MemoryCacheEnabled = false
	config.RateLimitDisabled = true
	config.SetLogConsumeEnabled(true)
	config.EnforceIncludeUsage = enforceIncludeUsage
	t.Cleanup(func() {
		drainCriticalTasksForRouter(t)
		model.DB, model.LOG_DB, client.HTTPClient = oldDB, oldLOG, oldClient
		common.SetRedisEnabled(oldRedis)
		common.UsingSQLite.Store(oldSQLite)
		config.MemoryCacheEnabled = oldMemory
		config.RateLimitDisabled = oldRate
		config.SetLogConsumeEnabled(oldLogging)
		config.EnforceIncludeUsage = oldEnforceUsage
		require.NoError(t, conn.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}, &model.UserRequestCost{}, &model.QuotaRefund{}, &model.Log{}, &model.Trace{}, &model.MCPServer{}, &model.MCPTool{}))
	allowed := models
	user := &model.User{Id: passthroughKeysUserID, UUID: uuid.NewString(), Username: "passthrough-keys", Status: model.UserStatusEnabled, Quota: passthroughKeysQuota, Group: "default"}
	key := strings.ReplaceAll(uuid.NewString(), "-", "")
	token := &model.Token{Id: passthroughKeysTokenID, UUID: uuid.NewString(), UserId: user.Id, Key: key, Status: model.TokenStatusEnabled, RemainQuota: passthroughKeysQuota, ExpiredTime: -1, Models: &allowed}
	channel := &model.Channel{Id: 929293, UUID: uuid.NewString(), Type: channelType, Status: model.ChannelStatusEnabled, Name: "passthrough-keys", Models: allowed, Group: "default", BaseURL: &base, Key: "fixture-only"}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(token).Error)
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities())
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		gmw.SetLogger(c, logger.Logger)
		c.Next()
	})
	engine.Use(middleware.RequestId())
	SetRelayRouter(engine)
	return engine, key
}

// drainCriticalTasksForRouter waits for detached billing so ledger reads are final.
func drainCriticalTasksForRouter(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, graceful.Drain(ctx))
}

// postChatCompletion sends one authenticated chat request through the engine.
func postChatCompletion(t *testing.T, engine *gin.Engine, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer sk-"+key)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	drainCriticalTasksForRouter(t)
	return w
}

// passthroughKeysLedger returns the user quota and logged consume models.
func passthroughKeysLedger(t *testing.T) (int64, []string) {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Where("id = ?", passthroughKeysUserID).Take(&user).Error)
	var models []string
	require.NoError(t, model.DB.Model(&model.Log{}).Where("user_id = ? AND type = ?", passthroughKeysUserID, model.LogTypeConsume).Pluck("model_name", &models).Error)
	return user.Quota, models
}

// jsonEscaped spells every rune of name as a six-character JSON unicode escape
// (backslash, 'u', four hex digits), which every JSON decoder reads back as name.
func jsonEscaped(name string) string {
	var b strings.Builder
	for _, r := range name {
		fmt.Fprintf(&b, "%cu%04x", '\\', r)
	}
	return b.String()
}

// rootKeysFoldEqual returns every decoded root key of payload that equals
// name under Unicode case folding (the matching rule of Go's encoding/json).
func rootKeysFoldEqual(t *testing.T, payload []byte, name string) []string {
	t.Helper()
	var root map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(payload, &root))
	var matches []string
	for key := range root {
		if strings.EqualFold(key, name) {
			matches = append(matches, key)
		}
	}
	return matches
}

// TestSecurityChatPassthroughExtraBodyAnySpelling drives POST /v1/chat/completions
// through the shipped router onto the raw OpenAI-compatible passthrough branch.
// The plain "extra_body" spelling is flattened under the allowlist (model, n and
// max_tokens are rejected). A JSON-escaped spelling decodes to the same key and
// must receive identical treatment instead of reaching the upstream verbatim,
// where an extra_body-honouring gateway would serve a model the token, channel
// and ledger never admitted. A case-folded spelling is read as extra_body by
// Go's decoder but not by case-sensitive providers, so it is rejected.
func TestSecurityChatPassthroughExtraBodyAnySpelling(t *testing.T) {
	const smuggled = `{"model":"` + passthroughKeysPremiumModel + `","n":8,"max_tokens":32000,"top_k":5}`
	for _, tc := range []struct {
		name     string
		key      string
		rejected bool
	}{
		{"plain", `"extra_body"`, false},
		{"unicode_escape_underscore", `"extra` + jsonEscaped("_") + `body"`, false},
		{"unicode_escape_every_letter", `"` + jsonEscaped("extra_body") + `"`, false},
		{"case_folded", `"Extra_Body"`, true},
		{"upper_case", `"EXTRA_BODY"`, true},
		{"case_folded_and_escaped", `"` + jsonEscaped("E") + `xtra_Body"`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream, capture := newExtraBodyGateway(t)
			engine, key := passthroughKeysRouter(t, upstream.URL)
			client.HTTPClient = upstream.Client()
			before, _ := passthroughKeysLedger(t)
			body := `{"model":"` + passthroughKeysAllowedModel + `","messages":[{"role":"user","content":"hello"}],` + tc.key + `:` + smuggled + `}`

			w := postChatCompletion(t, engine, key, body)

			bodies, served, n := capture.snapshot()
			after, models := passthroughKeysLedger(t)
			if tc.rejected {
				require.Equal(t, http.StatusBadRequest, w.Code,
					"a case-folded extra_body must be rejected; upstream served %v from %q", served, bodies)
				require.Empty(t, bodies)
				require.Equal(t, before, after)
				return
			}
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.Len(t, bodies, 1, "exactly one upstream dispatch")
			require.Equal(t, []string{passthroughKeysAllowedModel}, models)
			require.Equal(t, passthroughKeysAllowedModel, served[0],
				"the upstream must serve the admitted and billed model %v, got wire body: %s", models, bodies[0])
			require.Empty(t, n[0], "a rejected extra_body n must not reach the upstream: %s", bodies[0])
			require.Empty(t, rootKeysFoldEqual(t, bodies[0], "extra_body"),
				"no spelling of extra_body may reach the upstream un-normalized: %s", bodies[0])
			var forwarded map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(bodies[0], &forwarded))
			require.Equal(t, `5`, string(forwarded["top_k"]), "allowlisted extensions still merge into the root")
			require.NotContains(t, forwarded, "max_tokens")
		})
	}
}

// TestSecurityChatPassthroughAmbiguousRootKeysRejected proves that root keys
// which Go's case-insensitive, last-wins struct decoder reads differently from
// an exact-key provider (duplicates under escapes or case folding, and lone
// case-folded spellings of typed fields) are rejected before admission.
// Otherwise the gateway authorizes, routes and bills one value while a
// case-sensitive upstream serves another or falls back to its own default.
func TestSecurityChatPassthroughAmbiguousRootKeysRejected(t *testing.T) {
	const messages = `"messages":[{"role":"user","content":"hello"}]`
	for _, tc := range []struct {
		name string
		body string
	}{
		{"model_case_folded", `{"model":"` + passthroughKeysPremiumModel + `","Model":"` + passthroughKeysAllowedModel + `",` + messages + `}`},
		{"model_exact_duplicate", `{"model":"` + passthroughKeysPremiumModel + `","model":"` + passthroughKeysAllowedModel + `",` + messages + `}`},
		{"model_escaped_duplicate", `{"model":"` + passthroughKeysPremiumModel + `","` + jsonEscaped("m") + `odel":"` + passthroughKeysAllowedModel + `",` + messages + `}`},
		{"max_tokens_kelvin_sign", `{"model":"` + passthroughKeysAllowedModel + `","max_tokens":32000,"max_to` + jsonEscaped(string(rune(0x212A))) + `ens":1,` + messages + `}`},
		{"messages_case_folded", `{"model":"` + passthroughKeysAllowedModel + `",` + messages + `,"Messages":[{"role":"user","content":"hi"}]}`},
		{"extra_body_case_folded", `{"model":"` + passthroughKeysAllowedModel + `",` + messages + `,"extra_body":{"top_k":5},"Extra_Body":{"model":"` + passthroughKeysPremiumModel + `"}}`},
		{"lone_max_tokens_case_folded", `{"model":"` + passthroughKeysAllowedModel + `",` + messages + `,"Max_Tokens":1}`},
		{"lone_model_case_folded", `{"Model":"` + passthroughKeysAllowedModel + `",` + messages + `}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream, capture := newExtraBodyGateway(t)
			engine, key := passthroughKeysRouter(t, upstream.URL)
			client.HTTPClient = upstream.Client()
			before, _ := passthroughKeysLedger(t)

			w := postChatCompletion(t, engine, key, tc.body)

			bodies, served, _ := capture.snapshot()
			after, models := passthroughKeysLedger(t)
			require.Equal(t, http.StatusBadRequest, w.Code,
				"ambiguous root keys must be rejected; upstream served %v from %q while the token (limited to %s) was billed %d for %v",
				served, bodies, passthroughKeysAllowedModel, before-after, models)
			require.Empty(t, bodies, "a rejected request must not reach the upstream")
			require.Equal(t, before, after, "a rejected request must not move the user ledger")
			require.Empty(t, models)
		})
	}
}

// TestSecurityChatPassthroughPlainBodyStaysRaw guards the existing opt-in raw
// contract: an unambiguous body without extra_body is forwarded byte-for-byte.
func TestSecurityChatPassthroughPlainBodyStaysRaw(t *testing.T) {
	upstream, capture := newExtraBodyGateway(t)
	engine, key := passthroughKeysRouter(t, upstream.URL)
	client.HTTPClient = upstream.Client()
	// A message value spelled "extra_body" is content, not the root key, so it
	// must not trigger normalization.
	body := `{"model":"` + passthroughKeysAllowedModel + `","messages":[{"role":"user","content":"extra_body"}],"vendor_flag":{"id":9007199254740993}}`

	w := postChatCompletion(t, engine, key, body)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	bodies, _, _ := capture.snapshot()
	require.Len(t, bodies, 1)
	require.Equal(t, body, string(bodies[0]))
}
