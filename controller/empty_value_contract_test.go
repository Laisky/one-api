package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
)

// TestEmptyValueContract is an integration-style suite that exercises the
// HTTP -> controller -> model -> DB -> read-back path for every empty-value
// contract the codebase now claims to support. The unit-level tests in
// channel_nullable_update_test.go, option_test.go and user_self_update_test.go
// each cover one slice; this file locks them together end-to-end so a
// regression in any layer (route binding, controller handler, model store,
// DB column) is caught by a single failing run.
//
// Each subtest provisions its own fresh in-memory SQLite database and
// gin.Engine so that subtests can be selected individually with -run and so
// that there is no cross-talk between cases.
func TestEmptyValueContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("ChannelClearModelMappingViaNull", func(t *testing.T) {
		teardown := setupEmptyValueChannelEnv(t)
		defer teardown()

		router := newChannelRouter()

		// Step 1: create channel with non-empty model_mapping.
		createPayload := map[string]any{
			"name":          "channel-clear-null",
			"type":          1,
			"key":           "sk-test-1",
			"models":        "gpt-3.5-turbo",
			"group":         "default",
			"model_mapping": `{"a":"b"}`,
		}
		require.True(t, postChannel(t, router, createPayload), "create must succeed")

		ch := loadChannelByName(t, "channel-clear-null")
		require.NotNil(t, ch.ModelMapping)
		require.Equal(t, `{"a":"b"}`, *ch.ModelMapping)

		// Step 2: PUT with explicit null model_mapping.
		updatePayload := map[string]any{
			"id":            ch.Id,
			"name":          ch.Name,
			"type":          ch.Type,
			"models":        ch.Models,
			"group":         ch.Group,
			"model_mapping": nil,
		}
		require.True(t, putChannel(t, router, updatePayload), "update must succeed")

		// Step 3: GET /api/channel/:id and assert empty/nil.
		got := getChannel(t, router, ch.Id)
		assertChannelMappingCleared(t, got, "model_mapping")
	})

	t.Run("ChannelOmittedModelMappingPreserved", func(t *testing.T) {
		teardown := setupEmptyValueChannelEnv(t)
		defer teardown()

		router := newChannelRouter()

		createPayload := map[string]any{
			"name":          "channel-preserve",
			"type":          1,
			"key":           "sk-test-2",
			"models":        "gpt-3.5-turbo",
			"group":         "default",
			"model_mapping": `{"a":"b"}`,
		}
		require.True(t, postChannel(t, router, createPayload))

		ch := loadChannelByName(t, "channel-preserve")
		require.NotNil(t, ch.ModelMapping)
		require.Equal(t, `{"a":"b"}`, *ch.ModelMapping)

		// PUT WITHOUT model_mapping key at all.
		updatePayload := map[string]any{
			"id":     ch.Id,
			"name":   ch.Name,
			"type":   ch.Type,
			"models": ch.Models,
			"group":  ch.Group,
		}
		require.True(t, putChannel(t, router, updatePayload))

		got := getChannel(t, router, ch.Id)
		require.NotNil(t, got.ModelMapping, "ModelMapping must be preserved when omitted")
		require.Equal(t, `{"a":"b"}`, *got.ModelMapping)
	})

	t.Run("ChannelClearModelMappingViaEmptyString", func(t *testing.T) {
		teardown := setupEmptyValueChannelEnv(t)
		defer teardown()

		router := newChannelRouter()

		createPayload := map[string]any{
			"name":          "channel-clear-empty",
			"type":          1,
			"key":           "sk-test-3",
			"models":        "gpt-3.5-turbo",
			"group":         "default",
			"model_mapping": `{"a":"b"}`,
		}
		require.True(t, postChannel(t, router, createPayload))

		ch := loadChannelByName(t, "channel-clear-empty")
		require.NotNil(t, ch.ModelMapping)
		require.Equal(t, `{"a":"b"}`, *ch.ModelMapping)

		// PUT with explicit empty-string model_mapping.
		updatePayload := map[string]any{
			"id":            ch.Id,
			"name":          ch.Name,
			"type":          ch.Type,
			"models":        ch.Models,
			"group":         ch.Group,
			"model_mapping": "",
		}
		require.True(t, putChannel(t, router, updatePayload))

		got := getChannel(t, router, ch.Id)
		// Empty-string variant: the contract guarantees the value is
		// either nil or empty after read-back.
		assertChannelMappingCleared(t, got, "model_mapping")
	})

	t.Run("MCPServerClearDescription", func(t *testing.T) {
		teardown := setupEmptyValueMCPServerEnv(t)
		defer teardown()

		router := newMCPServerRouter()

		// Create with description="foo".
		createPayload := map[string]any{
			"name":        "mcp-empty-desc",
			"description": "foo",
			"base_url":    "https://example.com/mcp",
			"protocol":    model.MCPProtocolStreamableHTTP,
			"auth_type":   model.MCPAuthTypeNone,
		}
		body, err := json.Marshal(createPayload)
		require.NoError(t, errors.Wrap(err, "marshal mcp create payload"))

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/mcp_servers/", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var createResp struct {
			Success bool             `json:"success"`
			Data    *model.MCPServer `json:"data"`
			Message string           `json:"message"`
		}
		require.NoError(t, errors.Wrap(json.Unmarshal(w.Body.Bytes(), &createResp), "decode mcp create response"))
		require.True(t, createResp.Success, "mcp create must succeed: %s", createResp.Message)
		require.NotNil(t, createResp.Data)
		require.Equal(t, "foo", createResp.Data.Description)
		serverID := createResp.Data.Id
		require.NotZero(t, serverID)

		// PUT with description="" (and base_url is required by validate).
		updatePayload := map[string]any{
			"description": "",
			"base_url":    "https://example.com/mcp",
			"protocol":    model.MCPProtocolStreamableHTTP,
			"auth_type":   model.MCPAuthTypeNone,
		}
		body, err = json.Marshal(updatePayload)
		require.NoError(t, errors.Wrap(err, "marshal mcp update payload"))

		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPut, "/api/mcp_servers/"+strconv.Itoa(serverID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var updateResp map[string]any
		require.NoError(t, errors.Wrap(json.Unmarshal(w.Body.Bytes(), &updateResp), "decode mcp update response"))
		require.True(t, updateResp["success"].(bool), "mcp update must succeed: %v", updateResp["message"])

		// Read back and assert description cleared.
		stored, err := model.GetMCPServerByID(serverID)
		require.NoError(t, errors.Wrap(err, "load mcp server"))
		require.NotNil(t, stored)
		assert.Equal(t, "", stored.Description, "description must be cleared")
	})

	t.Run("OptionSensitiveSecretSkipOnEmpty", func(t *testing.T) {
		teardown := setupEmptyValueOptionEnv(t)
		defer teardown()

		router := newOptionRouter()

		// First save: real secret.
		require.True(t, putOption(t, router, "GitHubClientSecret", "the-secret"))
		stored := loadOptionRow(t, "GitHubClientSecret")
		require.NotNil(t, stored)
		require.Equal(t, "the-secret", stored.Value)

		// Empty save: must be ignored — secret must remain.
		require.True(t, putOption(t, router, "GitHubClientSecret", ""))
		stored = loadOptionRow(t, "GitHubClientSecret")
		require.NotNil(t, stored)
		assert.Equal(t, "the-secret", stored.Value, "empty save must not overwrite secret")

		// Rotation save: new value persists.
		require.True(t, putOption(t, router, "GitHubClientSecret", "new-secret"))
		stored = loadOptionRow(t, "GitHubClientSecret")
		require.NotNil(t, stored)
		assert.Equal(t, "new-secret", stored.Value)
	})

	t.Run("OptionNonSensitiveEmptyPersists", func(t *testing.T) {
		teardown := setupEmptyValueOptionEnv(t)
		defer teardown()

		router := newOptionRouter()

		require.True(t, putOption(t, router, "SystemName", "My Site"))
		stored := loadOptionRow(t, "SystemName")
		require.NotNil(t, stored)
		require.Equal(t, "My Site", stored.Value)

		require.True(t, putOption(t, router, "SystemName", ""))
		stored = loadOptionRow(t, "SystemName")
		require.NotNil(t, stored, "row should still exist after empty save")
		assert.Equal(t, "", stored.Value)

		// Cross-check via OptionMap (simulates GET /api/option/).
		config.OptionMapRWMutex.RLock()
		mapValue, ok := config.OptionMap["SystemName"]
		config.OptionMapRWMutex.RUnlock()
		require.True(t, ok, "SystemName must be present in OptionMap")
		assert.Equal(t, "", mapValue, "non-sensitive empty must propagate to OptionMap")
	})

	t.Run("OptionEmailDomainWhitelistEmptyProducesNilSlice", func(t *testing.T) {
		teardown := setupEmptyValueOptionEnv(t)
		defer teardown()

		router := newOptionRouter()

		// Seed two domains.
		require.True(t, putOption(t, router, "EmailDomainWhitelist", "a.com,b.com"))
		require.Len(t, config.EmailDomainWhitelist, 2)
		assert.Contains(t, config.EmailDomainWhitelist, "a.com")
		assert.Contains(t, config.EmailDomainWhitelist, "b.com")

		// Clear with empty value.
		require.True(t, putOption(t, router, "EmailDomainWhitelist", ""))
		assert.Equal(t, 0, len(config.EmailDomainWhitelist),
			"empty whitelist must be a nil/zero-length slice, not [\"\"]")
	})

	t.Run("UserSelfUpdateClearsDisplayName", func(t *testing.T) {
		teardown := setupEmptyValueUserEnv(t)
		defer teardown()

		user := seedSelfUpdateUser(t, "Original")

		router := gin.New()
		router.PUT("/api/user/self", func(c *gin.Context) {
			c.Set(ctxkey.Id, user.Id)
			UpdateSelf(c)
		})

		payload := map[string]string{"display_name": ""}
		body, err := json.Marshal(payload)
		require.NoError(t, errors.Wrap(err, "marshal self update payload"))

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/user/self", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		require.NoError(t, errors.Wrap(json.Unmarshal(w.Body.Bytes(), &resp), "decode self update response"))
		require.True(t, resp["success"].(bool), "self update must succeed: %v", resp["message"])

		var updated model.User
		require.NoError(t, errors.Wrap(model.DB.First(&updated, user.Id).Error, "reload user after clear"))
		assert.Equal(t, "", updated.DisplayName, "display_name must be cleared")
		// Username is a login key and must NOT be cleared by the silent-restore branch.
		assert.Equal(t, user.Username, updated.Username)
	})

	t.Run("UserSelfUpdateOmittedDisplayNamePreserved", func(t *testing.T) {
		teardown := setupEmptyValueUserEnv(t)
		defer teardown()

		user := seedSelfUpdateUser(t, "Original")

		router := gin.New()
		router.PUT("/api/user/self", func(c *gin.Context) {
			c.Set(ctxkey.Id, user.Id)
			UpdateSelf(c)
		})

		// Only password change — display_name omitted entirely.
		payload := map[string]string{"password": "newpw_for_preserve"}
		body, err := json.Marshal(payload)
		require.NoError(t, errors.Wrap(err, "marshal self update payload"))

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/user/self", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		require.NoError(t, errors.Wrap(json.Unmarshal(w.Body.Bytes(), &resp), "decode self update response"))
		require.True(t, resp["success"].(bool), "self update must succeed: %v", resp["message"])

		var updated model.User
		require.NoError(t, errors.Wrap(model.DB.First(&updated, user.Id).Error, "reload user after omit"))
		assert.Equal(t, "Original", updated.DisplayName,
			"omitted display_name must preserve the existing value")
	})
}

// ---------------------------------------------------------------------------
// channel helpers
// ---------------------------------------------------------------------------

func setupEmptyValueChannelEnv(t *testing.T) func() {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, errors.Wrap(err, "open sqlite for channel env"))
	require.NoError(t, errors.Wrap(
		db.AutoMigrate(&model.Channel{}, &model.Ability{}),
		"auto-migrate channel tables"))

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalUsingSQLite := common.UsingSQLite.Load()

	model.DB = db
	model.LOG_DB = db
	common.UsingSQLite.Store(true)

	return func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.UsingSQLite.Store(originalUsingSQLite)
	}
}

func newChannelRouter() *gin.Engine {
	r := gin.New()
	r.POST("/api/channel/", AddChannel)
	r.PUT("/api/channel/", UpdateChannel)
	r.GET("/api/channel/:id", GetChannel)
	return r
}

func postChannel(t *testing.T, r http.Handler, payload map[string]any) bool {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, errors.Wrap(err, "marshal channel POST payload"))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/channel/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, errors.Wrap(json.Unmarshal(w.Body.Bytes(), &resp), "decode channel POST response"))
	if !resp["success"].(bool) {
		t.Logf("channel POST failed: %v", resp["message"])
	}
	return resp["success"].(bool)
}

func putChannel(t *testing.T, r http.Handler, payload map[string]any) bool {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, errors.Wrap(err, "marshal channel PUT payload"))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/channel/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, errors.Wrap(json.Unmarshal(w.Body.Bytes(), &resp), "decode channel PUT response"))
	if !resp["success"].(bool) {
		t.Logf("channel PUT failed: %v", resp["message"])
	}
	return resp["success"].(bool)
}

func getChannel(t *testing.T, r http.Handler, id int) *model.Channel {
	t.Helper()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/channel/%d", id), nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Success bool           `json:"success"`
		Message string         `json:"message"`
		Data    *model.Channel `json:"data"`
	}
	require.NoError(t, errors.Wrap(json.Unmarshal(w.Body.Bytes(), &resp), "decode channel GET response"))
	require.True(t, resp.Success, "channel GET failed: %s", resp.Message)
	require.NotNil(t, resp.Data)
	return resp.Data
}

func loadChannelByName(t *testing.T, name string) *model.Channel {
	t.Helper()
	var ch model.Channel
	require.NoError(t, errors.Wrap(
		model.DB.Where("name = ?", name).First(&ch).Error,
		"load channel by name"))
	return &ch
}

// assertChannelMappingCleared encapsulates the SQLite null-vs-empty-string
// permissiveness used by the channel-clear contract (mirrors the model-level
// channel_nullable_update_test.go assertion).
func assertChannelMappingCleared(t *testing.T, ch *model.Channel, label string) {
	t.Helper()
	if ch.ModelMapping == nil {
		return
	}
	assert.Equal(t, "", *ch.ModelMapping, "%s should be nil or empty after clear", label)
}

// ---------------------------------------------------------------------------
// MCP server helpers
// ---------------------------------------------------------------------------

func setupEmptyValueMCPServerEnv(t *testing.T) func() {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, errors.Wrap(err, "open sqlite for mcp env"))
	require.NoError(t, errors.Wrap(
		db.AutoMigrate(&model.MCPServer{}, &model.MCPTool{}),
		"auto-migrate mcp tables"))

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalUsingSQLite := common.UsingSQLite.Load()

	model.DB = db
	model.LOG_DB = db
	common.UsingSQLite.Store(true)

	return func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.UsingSQLite.Store(originalUsingSQLite)
	}
}

func newMCPServerRouter() *gin.Engine {
	r := gin.New()
	r.POST("/api/mcp_servers/", CreateMCPServer)
	r.PUT("/api/mcp_servers/:id", UpdateMCPServer)
	return r
}

// ---------------------------------------------------------------------------
// option helpers
// ---------------------------------------------------------------------------

func setupEmptyValueOptionEnv(t *testing.T) func() {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, errors.Wrap(err, "open sqlite for option env"))
	require.NoError(t, errors.Wrap(
		db.AutoMigrate(&model.Option{}),
		"auto-migrate option table"))

	originalDB := model.DB
	originalUsingSQLite := common.UsingSQLite.Load()

	model.DB = db
	common.UsingSQLite.Store(true)

	config.OptionMapRWMutex.Lock()
	originalOptionMap := config.OptionMap
	config.OptionMap = make(map[string]string)
	config.OptionMapRWMutex.Unlock()

	originalEmailDomainWhitelist := append([]string(nil), config.EmailDomainWhitelist...)
	originalSystemName := config.SystemName
	originalGitHubClientSecret := config.GitHubClientSecret

	return func() {
		model.DB = originalDB
		common.UsingSQLite.Store(originalUsingSQLite)

		config.OptionMapRWMutex.Lock()
		config.OptionMap = originalOptionMap
		config.OptionMapRWMutex.Unlock()

		config.EmailDomainWhitelist = originalEmailDomainWhitelist
		config.SystemName = originalSystemName
		config.GitHubClientSecret = originalGitHubClientSecret
	}
}

func newOptionRouter() *gin.Engine {
	r := gin.New()
	r.PUT("/api/option/", UpdateOption)
	return r
}

func putOption(t *testing.T, r http.Handler, key, value string) bool {
	t.Helper()
	body, err := json.Marshal(model.Option{Key: key, Value: value})
	require.NoError(t, errors.Wrap(err, "marshal option PUT payload"))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/option/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, errors.Wrap(json.Unmarshal(w.Body.Bytes(), &resp), "decode option PUT response"))
	return resp["success"].(bool)
}

func loadOptionRow(t *testing.T, key string) *model.Option {
	t.Helper()
	var opt model.Option
	err := model.DB.Where("`key` = ?", key).First(&opt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	require.NoError(t, errors.Wrap(err, "load option row"))
	return &opt
}

// ---------------------------------------------------------------------------
// user helpers
// ---------------------------------------------------------------------------

func setupEmptyValueUserEnv(t *testing.T) func() {
	t.Helper()

	originalRedisEnabled := common.IsRedisEnabled()
	common.SetRedisEnabled(false)

	tempDir := t.TempDir()
	originalSQLitePath := common.SQLitePath
	common.SQLitePath = filepath.Join(tempDir, "empty-value-user.db")

	model.InitDB()
	model.InitLogDB()

	return func() {
		if model.DB != nil {
			require.NoError(t, errors.Wrap(model.CloseDB(), "close empty-value user DB"))
			model.DB = nil
			model.LOG_DB = nil
		}
		common.SetRedisEnabled(originalRedisEnabled)
		common.SQLitePath = originalSQLitePath
	}
}

func seedSelfUpdateUser(t *testing.T, displayName string) *model.User {
	t.Helper()
	hashedPw, err := common.Password2Hash("oldpassword")
	require.NoError(t, errors.Wrap(err, "hash seed password"))

	user := &model.User{
		Username:    "empty-value-self",
		Password:    hashedPw,
		Email:       "empty-value-self@example.com",
		DisplayName: displayName,
		Group:       "default",
		Status:      model.UserStatusEnabled,
	}
	require.NoError(t, errors.Wrap(model.DB.Create(user).Error, "seed self-update user"))
	return user
}
