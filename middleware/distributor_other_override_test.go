package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// resolvedOtherOverrideFields returns the three relay config fields that the
// legacy `other` value can populate, keyed by their config JSON names.
func resolvedOtherOverrideFields(cfg model.ChannelConfig) map[string]string {
	return map[string]string{"api_version": cfg.APIVersion, "library_id": cfg.LibraryID, "plugin": cfg.Plugin}
}

// TestSetupContextForSelectedChannelOtherOverridePrecedence pins the backward
// compatible precedence between the legacy `other` column and its typed config
// override: a non-empty override always wins, an empty or absent override
// falls back to `other`, and only the field owned by the channel type is
// populated. Types without an override rule never copy `other` into config.
func TestSetupContextForSelectedChannelOtherOverridePrecedence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	owners := []struct {
		name        string
		channelType int
		key         string
	}{
		{name: "azure", channelType: channeltype.Azure, key: "api_version"},
		{name: "ali", channelType: channeltype.Ali, key: "plugin"},
		{name: "xunfei", channelType: channeltype.Xunfei, key: "api_version"},
		{name: "aiproxy_library", channelType: channeltype.AIProxyLibrary, key: "library_id"},
		{name: "gemini", channelType: channeltype.Gemini, key: "api_version"},
		// OpenAI has no override rule: `other` must never reach the relay config.
		{name: "openai_unmapped", channelType: channeltype.OpenAI},
	}
	legacy := "legacy-value"
	cases := []struct {
		name string
		// other is the legacy column; nil models a NULL column.
		other *string
		// override is the config override; nil omits the key.
		override *string
		// want is the effective value when the type owns an override rule.
		want string
	}{
		{name: "override_wins_over_other", other: &legacy, override: new("override-value"), want: "override-value"},
		{name: "empty_override_falls_back_to_other", other: &legacy, override: new(""), want: legacy},
		{name: "absent_override_falls_back_to_other", other: &legacy, want: legacy},
		{name: "override_only", override: new("override-value"), want: "override-value"},
		{name: "override_with_empty_other", other: new(""), override: new("override-value"), want: "override-value"},
		{name: "neither_set", want: ""},
	}
	for _, owner := range owners {
		for _, tc := range cases {
			t.Run(owner.name+"/"+tc.name, func(t *testing.T) {
				cfg := map[string]any{}
				if tc.override != nil && owner.key != "" {
					cfg[owner.key] = *tc.override
				}
				raw, err := json.Marshal(cfg)
				require.NoError(t, err)
				channel := &model.Channel{Id: 31, Name: "override", Type: owner.channelType, Group: "default", Other: tc.other, Config: string(raw)}

				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				gmw.SetLogger(c, logger.Logger)
				SetupContextForSelectedChannel(c, channel, "gpt-4o")

				value, ok := c.Get(ctxkey.Config)
				require.True(t, ok, "the distributor must publish the relay config")
				got, ok := value.(model.ChannelConfig)
				require.True(t, ok, "the relay config must be a model.ChannelConfig")
				want := map[string]string{"api_version": "", "library_id": "", "plugin": ""}
				if owner.key != "" {
					want[owner.key] = tc.want
				}
				require.Equal(t, want, resolvedOtherOverrideFields(got))
			})
		}
	}
}
