package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/Laisky/zap/zapcore"
	"github.com/Laisky/zap/zaptest/observer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestSetupContextForSelectedChannelReportsMalformedConfig checks that a
// stored channel config the relay cannot parse is reported once as a warning
// naming the channel, never silently ignored, while the request keeps the
// existing degraded behavior: the typed overrides are empty, so the legacy
// `other` value applies. The warning must not echo config contents, which can
// hold credentials. Parameters: t runs the case. Returns: none.
func TestSetupContextForSelectedChannelReportsMalformedConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, config string
		warns        bool
	}{
		{name: "malformed_config_warns", config: `{"api_version":"2024-10-21","sk":"secret-value"`, warns: true},
		{name: "valid_config_is_silent", config: `{"region":"us-east-1"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core, logs := observer.New(zapcore.DebugLevel)
			lg, err := glog.NewWithName("distributor-config-test", glog.LevelDebug,
				zap.WrapCore(func(zapcore.Core) zapcore.Core { return core }))
			require.NoError(t, err)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			gmw.SetLogger(c, lg)
			other := "2024-02-01"
			channel := &model.Channel{Id: 7, Type: channeltype.Azure, Name: "azure-config", Group: "default", Other: &other, Config: tc.config}

			SetupContextForSelectedChannel(c, channel, "gpt-4o")

			cfg, ok := c.Get(ctxkey.Config)
			require.True(t, ok)
			require.Equal(t, other, cfg.(model.ChannelConfig).APIVersion, "the legacy value still applies")
			warnings := logs.FilterMessage("channel config is malformed; typed config overrides are ignored").All()
			if !tc.warns {
				require.Empty(t, warnings)
				return
			}
			require.Len(t, warnings, 1)
			require.Equal(t, zapcore.WarnLevel, warnings[0].Level)
			fields := warnings[0].ContextMap()
			require.EqualValues(t, 7, fields["channel_id"])
			require.Contains(t, fields, "error")
			for _, entry := range logs.All() {
				require.NotContains(t, entry.Message, "secret-value")
				for key, value := range entry.ContextMap() {
					require.NotContains(t, fmt.Sprint(value), "secret-value", "config contents must never be logged (field %s)", key)
				}
			}
		})
	}
}
