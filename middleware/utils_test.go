package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/config"
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
