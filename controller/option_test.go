package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
)

// setupOptionTestEnvironment swaps in an in-memory SQLite DB for the option
// table and seeds an empty OptionMap so model.UpdateOption can mutate it
// without panicking. The returned cleanup must run via t.Cleanup to restore
// the original globals after the test.
func setupOptionTestEnvironment(t *testing.T) func() {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, errors.Wrap(err, "open sqlite memory db"))
	require.NoError(t, errors.Wrap(db.AutoMigrate(&model.Option{}), "auto-migrate options table"))

	originalDB := model.DB
	model.DB = db

	originalUsingSQLite := common.UsingSQLite.Load()
	common.UsingSQLite.Store(true)

	config.OptionMapRWMutex.Lock()
	originalOptionMap := config.OptionMap
	config.OptionMap = make(map[string]string)
	config.OptionMapRWMutex.Unlock()

	return func() {
		model.DB = originalDB
		common.UsingSQLite.Store(originalUsingSQLite)
		config.OptionMapRWMutex.Lock()
		config.OptionMap = originalOptionMap
		config.OptionMapRWMutex.Unlock()
	}
}

// callUpdateOption posts a JSON body to the UpdateOption handler and returns
// the parsed response body together with the HTTP status code.
func callUpdateOption(t *testing.T, key, value string) (int, map[string]any) {
	t.Helper()

	body, err := json.Marshal(model.Option{Key: key, Value: value})
	require.NoError(t, errors.Wrap(err, "marshal option payload"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/option", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	UpdateOption(c)

	var resp map[string]any
	if w.Body.Len() > 0 {
		require.NoError(t, errors.Wrap(json.Unmarshal(w.Body.Bytes(), &resp), "decode handler response"))
	}
	return w.Code, resp
}

// loadStoredOption returns the persisted option row keyed by its primary key
// or nil when no row exists.
func loadStoredOption(t *testing.T, key string) *model.Option {
	t.Helper()

	var stored model.Option
	err := model.DB.Where("`key` = ?", key).First(&stored).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	require.NoError(t, errors.Wrap(err, "load stored option"))
	return &stored
}

func TestUpdateOption_SensitiveEmptyValueIsIgnored(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupOptionTestEnvironment(t)
	t.Cleanup(cleanup)

	cases := []struct {
		name  string
		key   string
		value string
	}{
		{name: "secret suffix", key: "GitHubClientSecret", value: "real-github-secret"},
		{name: "token suffix", key: "SMTPToken", value: "real-smtp-token"},
		{name: "password suffix", key: "SMTPPassword", value: "real-smtp-password"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// First write the real secret and confirm it persisted.
			status, resp := callUpdateOption(t, tc.key, tc.value)
			require.Equal(t, http.StatusOK, status)
			require.True(t, resp["success"].(bool))

			stored := loadStoredOption(t, tc.key)
			require.NotNil(t, stored, "secret should be stored after first save")
			assert.Equal(t, tc.value, stored.Value)

			// Second call with empty value must be silently ignored.
			status, resp = callUpdateOption(t, tc.key, "")
			require.Equal(t, http.StatusOK, status)
			assert.True(t, resp["success"].(bool), "handler should report success for ignored empty save")
			assert.Equal(t, "empty value ignored for sensitive option", resp["message"])

			stored = loadStoredOption(t, tc.key)
			require.NotNil(t, stored, "row should still exist after empty save")
			assert.Equal(t, tc.value, stored.Value, "stored secret must not be overwritten")

			// Whitespace-only value must also be treated as empty and ignored.
			status, resp = callUpdateOption(t, tc.key, "   ")
			require.Equal(t, http.StatusOK, status)
			assert.True(t, resp["success"].(bool))
			assert.Equal(t, "empty value ignored for sensitive option", resp["message"])

			stored = loadStoredOption(t, tc.key)
			require.NotNil(t, stored)
			assert.Equal(t, tc.value, stored.Value, "whitespace-only save must not overwrite secret")
		})
	}
}

func TestUpdateOption_NonSensitiveEmptyValuePersists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupOptionTestEnvironment(t)
	t.Cleanup(cleanup)

	// Seed an initial value to make sure the empty save actually clears it.
	status, resp := callUpdateOption(t, "SystemName", "Original Name")
	require.Equal(t, http.StatusOK, status)
	require.True(t, resp["success"].(bool))

	stored := loadStoredOption(t, "SystemName")
	require.NotNil(t, stored)
	require.Equal(t, "Original Name", stored.Value)

	// Empty save on a non-sensitive key must persist (protection only fires
	// for Token/Secret/Password suffixes).
	status, resp = callUpdateOption(t, "SystemName", "")
	require.Equal(t, http.StatusOK, status)
	assert.True(t, resp["success"].(bool))
	assert.NotEqual(t, "empty value ignored for sensitive option", resp["message"],
		"non-sensitive keys must not trigger the protection branch")

	stored = loadStoredOption(t, "SystemName")
	require.NotNil(t, stored, "row should still exist after empty save")
	assert.Equal(t, "", stored.Value, "non-sensitive empty save must persist")
}

func TestUpdateOption_SensitiveValueStillUpdatable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupOptionTestEnvironment(t)
	t.Cleanup(cleanup)

	// Initial save.
	status, _ := callUpdateOption(t, "OidcClientSecret", "first-secret")
	require.Equal(t, http.StatusOK, status)

	// Rotate to a new non-empty secret — protection must NOT engage.
	status, resp := callUpdateOption(t, "OidcClientSecret", "second-secret")
	require.Equal(t, http.StatusOK, status)
	assert.True(t, resp["success"].(bool))
	assert.NotEqual(t, "empty value ignored for sensitive option", resp["message"],
		"non-empty sensitive update must persist normally")

	stored := loadStoredOption(t, "OidcClientSecret")
	require.NotNil(t, stored)
	assert.Equal(t, "second-secret", stored.Value, "rotation to a new secret must update the row")
}
