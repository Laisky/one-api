package xai

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	store "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestLegacyGrokPricingPreflightDispatch verifies historical slugs without a
// verified current tariff cannot fall through to generic pricing. An explicit
// channel tariff permits legacy deployments without inventing public defaults.
func TestLegacyGrokPricingPreflightDispatch(t *testing.T) {
	for _, name := range []string{"grok-3-fast", "grok-3-mini-fast", "grok-2-1212", "grok-beta", "grok-2", "grok-2-latest", "grok-vision-beta"} {
		for _, tc := range []struct {
			name, config string
			allowed bool
		}{
			{"missing", "", false},
			{"zero", fmt.Sprintf(`{%q:{"ratio":0,"completion_ratio":2}}`, name), false},
			{"missing-output", fmt.Sprintf(`{%q:{"ratio":1}}`, name), false},
			{"alias-only", `{"customer-alias":{"ratio":1,"completion_ratio":2}}`, false},
			{"configured", fmt.Sprintf(`{%q:{"ratio":1,"completion_ratio":2}}`, name), true},
		} {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					_, _ = io.Copy(io.Discard, r.Body)
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"usage":{"prompt_tokens":73,"completion_tokens":19}}`)
				}))
				defer server.Close()
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				c.Request.Header.Set("Content-Type", "application/json")
				c.Set(ctxkey.ContentType, "application/json")
				if tc.config != "" {
					config := tc.config
					c.Set(ctxkey.ChannelModel, &store.Channel{ModelConfigs: &config})
				}
				m := &meta.Meta{Mode: relaymode.ChatCompletions, OriginModelName: "customer-alias", ActualModelName: name, BaseURL: server.URL, RequestURLPath: "/v1/chat/completions", StartTime: time.Unix(1791000000, 0)}
				m.Config.EndpointURLs = map[string]string{"chat_completions": server.URL}
				a := &Adaptor{}
				a.Init(m)
				resp, err := a.DoRequest(c, m, strings.NewReader(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hello"}]}`, name)))
				if resp != nil && resp.Body != nil {
					defer resp.Body.Close()
				}
				if tc.allowed {
					require.NoError(t, err)
					require.EqualValues(t, 1, calls.Load())
					return
				}
				require.Error(t, err)
				require.Nil(t, resp)
				require.Zero(t, calls.Load(), "rejection must precede all upstream work")
				require.False(t, c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded))
			})
		}
	}
}

// TestVerifiedGrokRedirectPricingRemainsIntact verifies the current known
// redirect preserves the complete tariff rather than resurrecting stale rates.
func TestVerifiedGrokRedirectPricingRemainsIntact(t *testing.T) {
	legacy, exists := ModelRatios["grok-3"]
	require.True(t, exists)
	current, exists := ModelRatios["grok-4.3"]
	require.True(t, exists)
	require.Equal(t, current.Ratio, legacy.Ratio)
	require.Equal(t, current.CompletionRatio, legacy.CompletionRatio)
	require.Equal(t, current.CachedInputRatio, legacy.CachedInputRatio)
	require.Equal(t, current.Tiers, legacy.Tiers)
}
