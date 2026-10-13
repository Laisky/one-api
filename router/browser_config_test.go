package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Laisky/one-api/common/config"
	"github.com/stretchr/testify/require"
)

// TestRegisteredBrowserConfigPublicProjectionAndOptionRootAuth verifies the shipped router exposes only approved browser fields anonymously while configuration reads and writes require root authentication.
func TestRegisteredBrowserConfigPublicProjectionAndOptionRootAuth(t *testing.T) {
	const publicFixture = `{"dsn":"https://0123456789abcdef0123456789abcdef@o123.ingest.sentry.io/456","environment":"production"}`
	_, engine, rootCookie := csrfFixture(t)
	config.OptionMapRWMutex.Lock()
	originalMap := config.OptionMap
	config.OptionMap = map[string]string{
		"BrowserSentryConfig":  publicFixture,
		"SMTPToken":            "unrelated-private-fixture",
		"PrivateFixtureOption": "unrelated-private-fixture",
	}
	config.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		config.OptionMapRWMutex.Lock()
		config.OptionMap = originalMap
		config.OptionMapRWMutex.Unlock()
	})

	publicRequest := httptest.NewRequest(http.MethodGet, "/api/browser-config?private=unrelated-request-fixture", nil)
	publicRecorder := httptest.NewRecorder()
	engine.ServeHTTP(publicRecorder, publicRequest)
	require.Equal(t, http.StatusOK, publicRecorder.Code)
	require.Equal(t, "no-store", publicRecorder.Header().Get("Cache-Control"))
	require.Equal(t, "nosniff", publicRecorder.Header().Get("X-Content-Type-Options"))
	require.JSONEq(t, `{"sentry":`+publicFixture+`}`, publicRecorder.Body.String())
	require.NotContains(t, publicRecorder.Body.String(), "unrelated")
	require.NotContains(t, publicRecorder.Body.String(), "BrowserSentryConfig")

	for _, method := range []string{http.MethodGet, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			payload := []byte(`{"key":"BrowserSentryConfig","value":""}`)
			request := httptest.NewRequest(method, "/api/option/", bytes.NewReader(payload))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusUnauthorized, recorder.Code)
			var body map[string]any
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
			require.Equal(t, false, body["success"])
			require.NotContains(t, recorder.Body.String(), "ingest.sentry.io")
			require.NotContains(t, recorder.Body.String(), "unrelated-private-fixture")
			config.OptionMapRWMutex.RLock()
			current := config.OptionMap["BrowserSentryConfig"]
			config.OptionMapRWMutex.RUnlock()
			require.Equal(t, publicFixture, current)
		})
	}

	// A valid root session is the positive control that rules out a broken or missing option route.
	rootRequest := httptest.NewRequest(http.MethodGet, "/api/option/", nil)
	rootRequest.AddCookie(rootCookie)
	rootRecorder := httptest.NewRecorder()
	engine.ServeHTTP(rootRecorder, rootRequest)
	require.Equal(t, http.StatusOK, rootRecorder.Code)
	var rootBody struct {
		Success bool `json:"success"`
		Data    []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rootRecorder.Body.Bytes(), &rootBody))
	require.True(t, rootBody.Success)
	var visibleConfig string
	for _, option := range rootBody.Data {
		if option.Key == "BrowserSentryConfig" {
			visibleConfig = option.Value
		}
		require.NotEqual(t, "SMTPToken", option.Key)
	}
	require.Equal(t, publicFixture, visibleConfig)

	// The registered route also fails closed when persisted runtime state contains forbidden fields.
	config.OptionMapRWMutex.Lock()
	config.OptionMap["BrowserSentryConfig"] = `{"dsn":"corrupted-private-fixture","auth_token":"corrupted-private-fixture"}`
	config.OptionMapRWMutex.Unlock()
	corruptRecorder := httptest.NewRecorder()
	engine.ServeHTTP(corruptRecorder, httptest.NewRequest(http.MethodGet, "/api/browser-config", nil))
	require.Equal(t, http.StatusOK, corruptRecorder.Code)
	require.JSONEq(t, `{"sentry":null}`, corruptRecorder.Body.String())
	require.NotContains(t, corruptRecorder.Body.String(), "corrupted-private-fixture")
}
