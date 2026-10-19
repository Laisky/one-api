package deepseekcompat

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
)

// TestDeepSeekToolReceiptContextIdentity checks cached active metadata precedence
// and the credential-free fallback used by standalone response handlers.
func TestDeepSeekToolReceiptContextIdentity(t *testing.T) {
	require.False(t, UsesDeepSeekToolReceiptContractFromContext(nil))
	for _, tc := range []struct {
		name, base string
		channel    int
		want       bool
	}{
		{"dedicated", "https://fixture.invalid", channeltype.DeepSeek, true},
		{"official_host", "https://api.deepseek.com/anthropic", channeltype.Anthropic, true},
		{"lookalike", "https://api.deepseek.com.example", channeltype.OpenAI, false},
		{"third_party_same_family", "https://provider.example", channeltype.OpenAI, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(ctxkey.Channel, tc.channel)
			c.Set(ctxkey.BaseURL, tc.base)
			require.Equal(t, tc.want, UsesDeepSeekToolReceiptContractFromContext(c))
			require.Nil(t, c.Request, "identity inspection does not build metadata or inspect request credentials")
		})
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(ctxkey.Channel, channeltype.OpenAI)
	c.Set(ctxkey.BaseURL, "https://api.openai.com")
	meta.Set2Context(c, &meta.Meta{ChannelType: channeltype.DeepSeek, BaseURL: "https://fixture.invalid"})
	require.True(t, UsesDeepSeekToolReceiptContractFromContext(c), "active cached metadata wins over older raw facts")
	c.Set(ctxkey.Channel, channeltype.DeepSeek)
	c.Set(ctxkey.BaseURL, "https://api.deepseek.com")
	meta.Set2Context(c, &meta.Meta{ChannelType: channeltype.OpenAI, BaseURL: "https://api.openai.com"})
	require.False(t, UsesDeepSeekToolReceiptContractFromContext(c), "a refreshed next-provider cache replaces the prior identity")
	require.Nil(t, c.Request)
}
