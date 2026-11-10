package anthropic_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/adaptor/azure"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
)

// TestClaude52DispatchRejectsUnpricedSpeed proves public dispatch refuses paid fast requests before HTTP setup.
func TestClaude52DispatchRejectsUnpricedSpeed(t *testing.T) {
	for _, name := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
		m := &meta.Meta{ActualModelName: name, OriginModelName: name}
		response, err := (&anthropic.Adaptor{}).DoRequest(c, m, strings.NewReader(`{"speed":"fast"}`))
		require.Nil(t, response)
		require.ErrorContains(t, err, "only supports standard speed billing")

		m.ChannelType = channeltype.Azure
		m.ActualModelName = "opaque-deployment"
		c.Set(ctxkey.Meta, m)
		response, err = (&azure.Adaptor{}).DoRequest(c, m, strings.NewReader(`{"speed":"fast"}`))
		require.Nil(t, response)
		require.ErrorContains(t, err, "only supports standard speed billing")
	}
}
