package controller

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/Laisky/one-api/relay/tooling"
)

// deepSeekWSLoopbackProxy returns a CONNECT proxy that sends every admitted
// fixture hostname exclusively to the supplied owned loopback provider.
func deepSeekWSLoopbackProxy(t *testing.T, target string) *httptest.Server {
	t.Helper()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || (r.Host != "api.deepseek.com:80" && r.Host != "api.openai.com:80") {
			http.Error(w, "fixture target refused", http.StatusBadGateway)
			return
		}
		upstream, err := net.DialTimeout("tcp", target, time.Second)
		if err != nil {
			t.Errorf("dial owned loopback fixture: %v", err)
			http.Error(w, "fixture unavailable", http.StatusBadGateway)
			return
		}
		defer func() { _ = upstream.Close() }()
		client, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack fixture proxy: %v", err)
			return
		}
		defer func() { _ = client.Close() }()
		if _, err := buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			t.Errorf("write fixture CONNECT response: %v", err)
			return
		}
		if err := buffer.Flush(); err != nil {
			t.Errorf("flush fixture CONNECT response: %v", err)
			return
		}
		copied := make(chan struct{})
		go func() {
			defer close(copied)
			if _, err := io.Copy(upstream, buffer); err != nil && !errors.Is(err, net.ErrClosed) {
				t.Logf("fixture client tunnel closed: %v", err)
			}
		}()
		if _, err := io.Copy(client, upstream); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Logf("fixture upstream tunnel closed: %v", err)
		}
		_ = client.Close()
		_ = upstream.Close()
		<-copied
	}))
	t.Cleanup(proxy.Close)
	return proxy
}

// deepSeekWSReadReceiptOrClose reads a terminal fixture receipt or a policy
// close from client, returning true only when an accepted receipt arrived.
func deepSeekWSReadReceiptOrClose(t *testing.T, client *websocket.Conn) bool {
	t.Helper()
	for {
		require.NoError(t, client.SetReadDeadline(time.Now().Add(5*time.Second)))
		_, frame, err := client.ReadMessage()
		if err != nil {
			var closed *websocket.CloseError
			require.ErrorAs(t, err, &closed, "the policy must close the socket without timing out")
			require.Equal(t, websocket.ClosePolicyViolation, closed.Code, "admission denial must be an explicit policy close")
			require.Equal(t, "tool_billing_not_supported", closed.Text)
			return false
		}
		var event struct {
			Type string `json:"type"`
		}
		require.NoError(t, json.Unmarshal(frame, &event))
		if event.Type == "response.completed" {
			return true
		}
	}
}

// TestDeepSeekToolReceiptWebSocketAdmission exercises real accepted paid tools,
// a conservative per-create floor, and malformed/insufficient-budget denials. A
// child test process isolates net/http's cached proxy environment from all other
// tests. Provider traffic is routed only to the owned loopback fixture.
func TestDeepSeekToolReceiptWebSocketAdmission(t *testing.T) {
	const childKey = "ONEAPI_TEST_DEEPSEEK_WS_RECEIPT_CHILD"
	if os.Getenv(childKey) != "1" {
		executable, err := os.Executable()
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, executable, "-test.run=^TestDeepSeekToolReceiptWebSocketAdmission$", "-test.timeout=40s", "-test.v")
		command.Env = append(os.Environ(), childKey+"=1")
		output, err := command.CombinedOutput()
		require.NoError(t, err, "isolated WebSocket behavioral acceptance:\n%s", output)
		t.Logf("isolated WebSocket behavioral acceptance:\n%s", output)
		return
	}

	upstream := newWSScriptedUpstream(t, [][]string{
		{wsEvent("response.completed", "ws_deepseek_first", "completed", 12, 3)},
		{wsEvent("response.completed", "ws_deepseek_second", "completed", 12, 3)},
	}, false)
	proxy := deepSeekWSLoopbackProxy(t, upstream.server.Listener.Addr().String())
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")
	t.Setenv("http_proxy", proxy.URL)
	t.Setenv("https_proxy", proxy.URL)
	t.Setenv("no_proxy", "")

	for _, tc := range []struct {
		name, host, actual, tool, override, input string
		price                                     *model.ToolPricingLocal
		frameType                                 int
		reject, previous                          bool
	}{
		{name: "paid_quota", price: &model.ToolPricingLocal{QuotaPerCall: 17}},
		{name: "paid_binary", price: &model.ToolPricingLocal{QuotaPerCall: 17}, frameType: websocket.BinaryMessage},
		{name: "overflow_usd", price: &model.ToolPricingLocal{UsdPerCall: 1e100}, reject: true},
		{name: "inherited_price"},
		{name: "paid_fetch", tool: `{"type":"web_fetch"}`, price: &model.ToolPricingLocal{QuotaPerCall: 17}},
		{name: "duplicate_tool_type", tool: `{"type":"web_search","type":"function","name":"local"}`, price: &model.ToolPricingLocal{QuotaPerCall: 17}, reject: true},
		{name: "folded_tool_type", tool: `{"type":"web_search","Type":"function","name":"local"}`, price: &model.ToolPricingLocal{QuotaPerCall: 17}, reject: true},
		{name: "duplicate_tools", price: &model.ToolPricingLocal{QuotaPerCall: 17}, reject: true},
		{name: "folded_tools", price: &model.ToolPricingLocal{QuotaPerCall: 17}, reject: true},
		{name: "hosted_discovery_output", tool: `{"type":"tool_search","execution":"client"}`, input: `[{"type":"tool_search_output","execution":"client","call_id":"search_1","tools":[{"type":"web_search"}]}]`, price: &model.ToolPricingLocal{QuotaPerCall: 17}, reject: true},
		{name: "hosted_additional_tools", tool: `{"type":"tool_search","execution":"client"}`, input: `[{"type":"additional_tools","role":"developer","tools":[{"type":"web_search"}]}]`, price: &model.ToolPricingLocal{QuotaPerCall: 17}, reject: true},
		{name: "duplicate_input", tool: `{"type":"tool_search","execution":"client"}`, input: `[{"type":"tool_search_output","execution":"client","call_id":"search_1","tools":[{"type":"web_search"}]}]`, price: &model.ToolPricingLocal{QuotaPerCall: 17}, reject: true},
		{name: "folded_input", tool: `{"type":"tool_search","execution":"client"}`, input: `[{"type":"tool_search_output","execution":"client","call_id":"search_1","tools":[{"type":"web_search"}]}]`, price: &model.ToolPricingLocal{QuotaPerCall: 17}, reject: true},
		{name: "accepted_work_before_denial", price: &model.ToolPricingLocal{QuotaPerCall: 17}, reject: true, previous: true},
		{name: "unused_openai_override", price: &model.ToolPricingLocal{QuotaPerCall: 17}, override: "http://api.openai.com/v1/responses"},
		{name: "unused_deepseek_override", host: "api.openai.com", actual: "gpt-4o-mini", price: &model.ToolPricingLocal{QuotaPerCall: 17}, override: "http://api.deepseek.com/v1/responses"},
		{name: "explicit_zero", price: &model.ToolPricingLocal{}},
		{name: "local_function", tool: `{"type":"function","name":"web_search","parameters":{"type":"object"}}`, price: &model.ToolPricingLocal{QuotaPerCall: 17}},
		{name: "client_search", tool: `{"type":"tool_search","execution":"client"}`, price: &model.ToolPricingLocal{QuotaPerCall: 17}},
		{name: "client_discovery_output", tool: `{"type":"tool_search","execution":"client"}`, input: `[{"type":"tool_search_call","execution":"client","call_id":"search_1","arguments":{"query":"lookup","exact":9007199254740993}},{"type":"tool_search_output","execution":"client","call_id":"search_1","tools":[{"type":"function","name":"web_search","parameters":{"type":"object"}}]}]`, price: &model.ToolPricingLocal{QuotaPerCall: 17}},
		{name: "client_additional_tools", tool: `{"type":"tool_search","execution":"client"}`, input: `[{"type":"additional_tools","role":"developer","tools":[{"type":"function","name":"web_search","parameters":{"type":"object"}}]},{"role":"user","content":"synthetic"}]`, price: &model.ToolPricingLocal{QuotaPerCall: 17}},
		{name: "unrelated_openai", host: "api.openai.com", actual: "gpt-4o-mini", price: &model.ToolPricingLocal{QuotaPerCall: 17}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.host == "" {
				tc.host = "api.deepseek.com"
			}
			if tc.actual == "" {
				tc.actual = "deepseek-v4-pro"
			}
			if tc.tool == "" {
				tc.tool = `{"type":"web_search"}`
			}
			if tc.input == "" {
				tc.input = `"synthetic"`
			}
			if tc.frameType == 0 {
				tc.frameType = websocket.TextMessage
			}
			wsSettlementSetup(t)
			base := "http://" + tc.host
			require.True(t, supportsNativeResponseAPI(&metalib.Meta{ChannelType: channeltype.OpenAI, BaseURL: base, ActualModelName: tc.actual}), "real controller route admits this upstream")
			channel := &model.Channel{Id: fallbackOpenAIChannelID, Type: channeltype.OpenAI}
			require.NoError(t, channel.SetModelPriceConfigs(map[string]model.ModelConfigLocal{tc.actual: {Ratio: 1, CompletionRatio: 1}}))
			if tc.price != nil {
				require.NoError(t, channel.SetToolingConfig(&model.ChannelToolingConfig{Pricing: map[string]model.ToolPricingLocal{"web_search": *tc.price, "web_fetch": *tc.price}}))
			}
			channelConfig := model.ChannelConfig{}
			if tc.override != "" {
				channelConfig.EndpointURLs = map[string]string{channeltype.RelayModeToEndpointName(relaymode.ResponseAPI): tc.override}
			}
			requestID := "ws-deepseek-" + tc.name
			done := make(chan *relaymodel.ErrorWithStatusCode, 1)
			toolAllowance := make(chan int64, 1)
			router := gin.New()
			router.GET("/v1/responses", func(c *gin.Context) {
				gmw.SetLogger(c, logger.Logger)
				for key, value := range map[string]any{
					ctxkey.Channel: channeltype.OpenAI, ctxkey.ChannelId: fallbackOpenAIChannelID,
					ctxkey.ChannelModel: channel, ctxkey.TokenId: fallbackTokenID, ctxkey.TokenName: "fallback-token",
					ctxkey.Id: fallbackUserID, ctxkey.Group: "default", ctxkey.ModelMapping: map[string]string{},
					ctxkey.ChannelRatio: 1.0, ctxkey.RequestModel: tc.actual, ctxkey.BaseURL: base,
					ctxkey.RequestId: requestID, ctxkey.Username: "response-fallback", ctxkey.Config: channelConfig,
					ctxkey.UserObj:             &model.User{Id: fallbackUserID, Quota: wsSettlementBalance},
					ctxkey.TokenQuotaUnlimited: false, ctxkey.TokenQuota: wsSettlementBalance,
				} {
					c.Set(key, value)
				}
				apiErr := RelayResponseAPIHelper(c)
				toolAllowance <- tooling.ConservativeDeepSeekToolQuota(c)
				done <- apiErr
			})
			gateway := httptest.NewServer(router)
			t.Cleanup(gateway.Close)
			before := len(upstream.received())
			client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(gateway.URL, "http")+"/v1/responses?model="+tc.actual, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = client.Close() })
			var accepted []string
			if tc.previous {
				first := `{"type":"response.create","model":"` + tc.actual + `","input":"accepted synthetic work"}`
				require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(first)))
				require.True(t, deepSeekWSReadReceiptOrClose(t, client))
				accepted = append(accepted, first)
			}
			frame := `{"type":"response.create","model":"` + tc.actual + `","input":` + tc.input + `,"tools":[` + tc.tool + `],"vendor_extension":{"exact":9007199254740993}}`
			switch tc.name {
			case "duplicate_tools":
				frame = strings.Replace(frame, `,"vendor_extension"`, `,"tools":[{"type":"function","name":"local"}],"vendor_extension"`, 1)
			case "folded_tools":
				frame = strings.Replace(frame, `,"vendor_extension"`, `,"Tools":[{"type":"function","name":"local"}],"vendor_extension"`, 1)
			case "duplicate_input":
				frame = strings.Replace(frame, `,"vendor_extension"`, `,"input":"synthetic","vendor_extension"`, 1)
			case "folded_input":
				frame = strings.Replace(frame, `,"vendor_extension"`, `,"Input":"synthetic","vendor_extension"`, 1)
			case "accepted_work_before_denial":
				frame = strings.Replace(frame, `,"vendor_extension"`, `,"tools":[{"type":"function","name":"local"}],"vendor_extension"`, 1)
			}
			require.NoError(t, client.WriteMessage(tc.frameType, []byte(frame)))
			if deepSeekWSReadReceiptOrClose(t, client) {
				require.NoError(t, client.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "fixture done"), time.Now().Add(time.Second)))
				require.NoError(t, client.Close())
			}
			apiErr := wsAwaitHandler(t, done)
			toolFloor := <-toolAllowance
			forwarded := upstream.received()[before:]
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			if tc.reject {
				if len(accepted) == 0 {
					require.Empty(t, forwarded, "positive-priced DeepSeek native tools must never dispatch a response.create")
				} else {
					require.Equal(t, accepted, forwarded, "denial must preserve only already accepted work")
				}
				var charge int64
				if tc.previous {
					charge = 15
					require.Nil(t, apiErr, "later denial retains completed work and its terminal receipt")
					require.EqualValues(t, charge, requestCostQuota(t, requestID))
				} else {
					require.NotNil(t, apiErr)
				}
				require.Equal(t, wsSettlementBalance-charge, reloadUserQuota(t), "deny new work without refunding accepted work")
				require.Equal(t, wsSettlementBalance-charge, token.RemainQuota)
				require.Equal(t, charge, token.UsedQuota)
			} else {
				require.Nil(t, apiErr)
				require.Equal(t, []string{frame}, forwarded, "accepted local/client/native payload remains byte-exact")
				expected := int64(15) + toolFloor
				if tc.host == "api.deepseek.com" && (tc.tool == `{"type":"web_search"}` || tc.tool == `{"type":"web_fetch"}`) && tc.price != nil && tc.price.QuotaPerCall > 0 {
					require.GreaterOrEqual(t, toolFloor, int64(170), "positive-price native tool must not be free")
				}
				require.Equal(t, wsSettlementBalance-expected, reloadUserQuota(t))
				require.Equal(t, wsSettlementBalance-expected, token.RemainQuota)
				require.EqualValues(t, expected, token.UsedQuota)
				require.EqualValues(t, expected, requestCostQuota(t, requestID))
			}
		})
	}
}
