package client

import (
	"encoding/json"
	"testing"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
)

// TestInitProxyLoggingRedactsConfiguredCredentials exercises both real proxy
// initialization branches with fake credentials and captures fatal parse failures.
func TestInitProxyLoggingRedactsConfiguredCredentials(t *testing.T) {
	previousLogger := logger.Logger
	previousUserProxy, previousRelayProxy := config.UserContentRequestProxy, config.RelayProxy
	previousUserClient, previousClient, previousImpatient := UserContentRequestHTTPClient, HTTPClient, ImpatientHTTPClient
	t.Cleanup(func() {
		logger.Logger = previousLogger
		config.UserContentRequestProxy, config.RelayProxy = previousUserProxy, previousRelayProxy
		UserContentRequestHTTPClient, HTTPClient, ImpatientHTTPClient = previousUserClient, previousClient, previousImpatient
	})
	for _, target := range []string{"user_content", "relay"} {
		for _, invalid := range []bool{false, true} {
			t.Run(target+"/"+map[bool]string{false: "valid", true: "invalid"}[invalid], func(t *testing.T) {
				core, entries := observer.New(zapcore.DebugLevel)
				lg, err := glog.NewConsoleWithName("proxy-test", glog.LevelDebug,
					zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }),
					zap.WithFatalHook(zapcore.WriteThenPanic))
				require.NoError(t, err)
				logger.Logger = lg
				const password = "fake-proxy-password"
				proxy := "http://operator:" + password + "@proxy.invalid:8080"
				if invalid {
					proxy = "http://operator:" + password + "@proxy.invalid:invalid"
				}
				config.UserContentRequestProxy, config.RelayProxy = "", ""
				if target == "user_content" {
					config.UserContentRequestProxy = proxy
				} else {
					config.RelayProxy = proxy
				}
				if invalid {
					require.Panics(t, Init)
				} else {
					require.NotPanics(t, Init)
				}
				require.NotEmpty(t, entries.All())
				for _, entry := range entries.All() {
					fields, marshalErr := json.Marshal(entry.ContextMap())
					require.NoError(t, marshalErr)
					require.NotContains(t, entry.Message, password)
					require.NotContains(t, string(fields), password)
				}
				var matched bool
				for _, entry := range entries.All() {
					if invalid && entry.Level != zapcore.FatalLevel {
						continue
					}
					if !invalid && entry.Message != "using proxy to fetch user content" && entry.Message != "using api relay proxy" {
						continue
					}
					matched = true
					require.Equal(t, logger.Redact(proxy), entry.ContextMap()["proxy"])
					if invalid {
						require.NotEmpty(t, entry.ContextMap()["error"])
						require.NotEmpty(t, entry.ContextMap()["error_type"])
					}
				}
				require.True(t, matched)
			})
		}
	}
}
