package controller

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
)

// TestPasskeyInitializationFailureReportedOnce verifies request-level logging
// and response behavior without changing once-only initialization in other tests.
func TestPasskeyInitializationFailureReportedOnce(t *testing.T) {
	if os.Getenv("ONEAPI_TEST_PASSKEY_LOG") == "" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestPasskeyInitializationFailureReportedOnce$")
		cmd.Env = append(os.Environ(), "ONEAPI_TEST_PASSKEY_LOG=1")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", out)
		return
	}
	config.WebAuthnRPID = "://invalid"
	config.WebAuthnRPOrigins = "https://example.com"
	core, logs := observer.New(zapcore.DebugLevel)
	lg, err := glog.NewWithName("passkey-test", glog.LevelDebug, zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
	require.NoError(t, err)
	logger.Logger = lg
	for range 2 {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/passkey", nil)
		gmw.SetLogger(c, lg)
		PasskeyRegisterBegin(c)
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Contains(t, recorder.Body.String(), "WebAuthn not available")
	}
	require.Len(t, logs.FilterLevelExact(zapcore.ErrorLevel).All(), 2)
}
