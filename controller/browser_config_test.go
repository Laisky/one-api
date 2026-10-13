package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const browserPublicFixture = `{"dsn":"https://0123456789abcdef0123456789abcdef@o123.ingest.sentry.io/456","environment":"production"}`

// TestBrowserConfigProjection proves public delivery returns only typed approved fields and fails closed on corrupted stored options.
func TestBrowserConfigProjection(t *testing.T) {
	defer setupOptionTestEnvironment(t)()
	for _, raw := range []string{"", browserPublicFixture, strings.Replace(browserPublicFixture, `"production"`, `"secret"`, 1), `{"dsn":"secret","auth_token":"secret"}`} {
		config.OptionMapRWMutex.Lock()
		config.OptionMap["BrowserSentryConfig"] = raw
		config.OptionMap["SMTPToken"] = "unrelated-private-secret"
		config.OptionMap["ArbitraryValue"] = "unrelated-private-secret"
		config.OptionMapRWMutex.Unlock()
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/browser-config?token=secret", nil)
		c.Request.Header.Set("Authorization", "Bearer request-private-secret")
		GetBrowserConfig(c)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
		require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
		var body map[string]any
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
		require.Len(t, body, 1)
		require.NotContains(t, recorder.Body.String(), "secret")
		if raw == browserPublicFixture {
			require.Equal(t, map[string]any{"dsn": "https://0123456789abcdef0123456789abcdef@o123.ingest.sentry.io/456", "environment": "production"}, body["sentry"])
		} else {
			require.Nil(t, body["sentry"])
		}
	}
}

// TestBrowserConfigPersistence validates controller and direct-model writes before storage, preserving valid prior state and allowing an atomic blank clear.
func TestBrowserConfigPersistence(t *testing.T) {
	defer setupOptionTestEnvironment(t)()
	require.NoError(t, model.UpdateOption("BrowserSentryConfig", browserPublicFixture))
	malformed := strings.TrimSuffix(browserPublicFixture, "}") + `,"auth_token":"synthetic-secret"}`
	status, body := callUpdateOption(t, "BrowserSentryConfig", malformed)
	require.Equal(t, http.StatusBadRequest, status)
	require.NotEqual(t, true, body["success"])
	require.NotContains(t, body["message"], "synthetic-secret")
	require.Error(t, model.UpdateOption("BrowserSentryConfig", malformed))
	require.Equal(t, browserPublicFixture, loadStoredOption(t, "BrowserSentryConfig").Value)
	config.OptionMapRWMutex.RLock()
	previous := config.OptionMap["BrowserSentryConfig"]
	config.OptionMapRWMutex.RUnlock()
	require.Equal(t, browserPublicFixture, previous)
	status, body = callUpdateOption(t, "BrowserSentryConfig", "  ")
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, true, body["success"])
	require.Empty(t, loadStoredOption(t, "BrowserSentryConfig").Value)
}

// TestBrowserConfigStorageFailurePreservesState verifies failed database updates and clears leave both stored and active configuration unchanged.
func TestBrowserConfigStorageFailurePreservesState(t *testing.T) {
	defer setupOptionTestEnvironment(t)()
	require.NoError(t, model.UpdateOption("BrowserSentryConfig", browserPublicFixture))

	db := model.DB
	const callback = "test:browser-config-save-failure"
	writeErr := errors.New("injected browser option write failure")
	failedWrites := 0
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "options" {
			failedWrites++
			tx.AddError(writeErr)
		}
	}))
	defer func() { require.NoError(t, db.Callback().Update().Remove(callback)) }()

	changed := strings.Replace(browserPublicFixture, `"production"`, `"staging"`, 1)
	for _, next := range []string{changed, ""} {
		err := model.UpdateOption("BrowserSentryConfig", next)
		require.ErrorIs(t, err, writeErr)
		status, body := callUpdateOption(t, "BrowserSentryConfig", next)
		require.Equal(t, http.StatusOK, status)
		require.Equal(t, false, body["success"])
		require.NotContains(t, body["message"], "ingest.sentry.io")
		require.Equal(t, browserPublicFixture, loadStoredOption(t, "BrowserSentryConfig").Value)
		config.OptionMapRWMutex.RLock()
		current := config.OptionMap["BrowserSentryConfig"]
		config.OptionMapRWMutex.RUnlock()
		require.Equal(t, browserPublicFixture, current)

		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/browser-config", nil)
		GetBrowserConfig(c)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.JSONEq(t, `{"sentry":`+browserPublicFixture+`}`, recorder.Body.String())
	}
	require.Equal(t, 4, failedWrites, "both direct-model and controller updates must reach the injected storage failure")
}

// TestBrowserConfigCorruptedPersistedReloadFailsClosed proves the existing startup reload cannot activate or expose malformed persisted configuration.
func TestBrowserConfigCorruptedPersistedReloadFailsClosed(t *testing.T) {
	defer setupOptionTestEnvironment(t)()
	require.NoError(t, model.UpdateOption("BrowserSentryConfig", browserPublicFixture))
	for _, corrupted := range []string{
		strings.TrimSuffix(browserPublicFixture, "}") + `,"auth_token":"corrupted-private-fixture"}`,
		strings.Replace(browserPublicFixture, `"production"`, `"corrupted-private-fixture"`, 1),
		`{"dsn":`,
	} {
		// Bypass normal validation to represent an externally corrupted persisted row.
		require.NoError(t, model.DB.Model(&model.Option{}).Where("key = ?", "BrowserSentryConfig").Update("value", corrupted).Error)
		model.InitOptionMap()
		config.OptionMapRWMutex.RLock()
		reloaded := config.OptionMap["BrowserSentryConfig"]
		config.OptionMapRWMutex.RUnlock()
		require.Equal(t, corrupted, reloaded, "the assertion must exercise persisted-row reload rather than a manually seeded map")

		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/browser-config", nil)
		GetBrowserConfig(c)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
		require.JSONEq(t, `{"sentry":null}`, recorder.Body.String())
		require.NotContains(t, recorder.Body.String(), "corrupted-private-fixture")
		require.NotContains(t, recorder.Body.String(), "ingest.sentry.io")
	}
}

// TestBrowserConfigAdminReadProjectsCanonicalPublicValue proves option reads return only validated canonical JSON or a blank setting without exposing corrupted fields.
func TestBrowserConfigAdminReadProjectsCanonicalPublicValue(t *testing.T) {
	defer setupOptionTestEnvironment(t)()
	for _, testCase := range []struct {
		name     string
		raw      string
		expected string
	}{
		{name: "valid public value", raw: browserPublicFixture, expected: browserPublicFixture},
		{name: "canonical public value", raw: ` {"environment":"production","dsn":"https://0123456789abcdef0123456789abcdef@o123.ingest.sentry.io/456"} `, expected: browserPublicFixture},
		{name: "empty setting", raw: "", expected: ""},
		{name: "forbidden field", raw: strings.TrimSuffix(browserPublicFixture, "}") + `,"auth_token":"corrupted-private-fixture"}`, expected: ""},
		{name: "invalid environment", raw: strings.Replace(browserPublicFixture, `"production"`, `"corrupted-private-fixture"`, 1), expected: ""},
		{name: "malformed JSON", raw: `{"dsn":"corrupted-private-fixture"`, expected: ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			config.OptionMapRWMutex.Lock()
			config.OptionMap = map[string]string{
				"BrowserSentryConfig": testCase.raw,
				"SystemName":          "Synthetic workspace",
				"SMTPToken":           "unrelated-private-fixture",
			}
			config.OptionMapRWMutex.Unlock()
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/option/", nil)
			GetOptions(c)
			require.Equal(t, http.StatusOK, recorder.Code)
			var body struct {
				Success bool           `json:"success"`
				Data    []model.Option `json:"data"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
			require.True(t, body.Success)
			require.Len(t, body.Data, 2)
			values := make(map[string]string, len(body.Data))
			for _, option := range body.Data {
				values[option.Key] = option.Value
			}
			require.Contains(t, values, "BrowserSentryConfig")
			require.Equal(t, testCase.expected, values["BrowserSentryConfig"])
			require.Equal(t, "Synthetic workspace", values["SystemName"])
			require.NotContains(t, values, "SMTPToken")
			require.NotContains(t, recorder.Body.String(), "corrupted-private-fixture")
			require.NotContains(t, recorder.Body.String(), "auth_token")
			require.NotContains(t, recorder.Body.String(), "unrelated-private-fixture")
		})
	}
}
