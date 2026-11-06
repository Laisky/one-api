package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestSetupContextForSelectedChannelUsesNormalizedGroups checks that the billed
// channel ratio is the minimum over the channel's groups parsed with the same
// rule as abilities: entries are trimmed, so " vip" resolves to "vip".
func TestSetupContextForSelectedChannelUsesNormalizedGroups(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := ratio.GroupRatio2JSONString()
	require.NoError(t, ratio.UpdateGroupRatioByJSONString(`{"default":1,"vip":0.5,"svip":0.25}`))
	t.Cleanup(func() {
		require.NoError(t, ratio.UpdateGroupRatioByJSONString(previous))
	})

	cases := []struct {
		group string
		want  float64
	}{
		{group: "default", want: 1},
		{group: "default,vip", want: 0.5},
		{group: "default, vip", want: 0.5},
		{group: " svip ,vip", want: 0.25},
		{group: "default,,vip,", want: 0.5},
	}
	for _, tc := range cases {
		t.Run(tc.group, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			gmw.SetLogger(c, logger.Logger)

			SetupContextForSelectedChannel(c, &model.Channel{Id: 7, Name: "ch", Group: tc.group}, "gpt-4o")

			require.InDelta(t, tc.want, c.GetFloat64(ctxkey.ChannelRatio), 1e-9)
		})
	}
}
