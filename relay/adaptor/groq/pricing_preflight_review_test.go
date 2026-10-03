package groq

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

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	store "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestGroqPricingPreflightDispatch proves missing contract prices and retired
// models cannot perform upstream work. A configured tariff is a positive control,
// and model mapping cannot borrow the customer's alias price for another model.
func TestGroqPricingPreflightDispatch(t *testing.T) {
	// These serial transport tests intentionally have no tracing database.
	// Disable only the optional trace sink, not pricing, HTTP, or forwarding.
	// Restore the package global before parallel tests resume.
	previousSinks := config.TraceSinks
	config.TraceSinks = []string{config.TraceSinkNone}
	t.Cleanup(func() { config.TraceSinks = previousSinks })

	const receipt = `{"usage":{"prompt_tokens":73,"completion_tokens":19}}`
	for _, name := range []string{"minimaxai/minimax-m2.7", "groq/compound", "groq/compound-mini"} {
		for _, tc := range []struct {
			name, config string
			configured   bool
		}{
			{"missing", "", false},
			{"zero", fmt.Sprintf(`{%q:{"ratio":0,"completion_ratio":2}}`, name), false},
			{"missing-output", fmt.Sprintf(`{%q:{"ratio":1}}`, name), false},
			{"alias-only", `{"customer-alias":{"ratio":1,"completion_ratio":2}}`, false},
			{"contract", fmt.Sprintf(`{%q:{"ratio":1,"completion_ratio":2}}`, name), true},
		} {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					_, _ = io.Copy(io.Discard, r.Body)
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, receipt)
				}))
				defer server.Close()
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				c.Request.Header.Set("Content-Type", "application/json")
				c.Set(ctxkey.ContentType, "application/json")
				if tc.config != "" {
					modelConfig := tc.config
					c.Set(ctxkey.ChannelModel, &store.Channel{ModelConfigs: &modelConfig})
				}
				m := &meta.Meta{Mode: relaymode.ChatCompletions, OriginModelName: "customer-alias", ActualModelName: name, BaseURL: server.URL, RequestURLPath: "/v1/chat/completions", StartTime: time.Unix(1791000000, 0)}
				m.Config.EndpointURLs = map[string]string{"chat_completions": server.URL}
				a := &Adaptor{}
				a.Init(m)
				resp, err := a.DoRequest(c, m, strings.NewReader(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hello"}]}`, name)))
				if resp != nil && resp.Body != nil {
					defer resp.Body.Close()
				}
				if tc.configured && name == "minimaxai/minimax-m2.7" {
					require.NoError(t, err)
					require.NotNil(t, resp)
					require.Equal(t, http.StatusOK, resp.StatusCode)
					body, readErr := io.ReadAll(resp.Body)
					require.NoError(t, readErr)
					require.JSONEq(t, receipt, string(body))
					require.EqualValues(t, 1, calls.Load())
					require.True(t, c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded))
					return
				}
				require.ErrorContains(t, err, "validate request pricing")
				if name == "minimaxai/minimax-m2.7" {
					require.ErrorContains(t, err, "requires explicit positive input and output pricing")
				} else {
					require.ErrorContains(t, err, "is retired")
				}
				require.Nil(t, resp)
				require.Zero(t, calls.Load(), "rejection must precede all upstream work")
				require.False(t, c.GetBool(ctxkey.UpstreamRequestPossiblyForwarded))
			})
		}
	}
}
