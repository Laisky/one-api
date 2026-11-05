package controller

import (
	"context"
	stderrors "errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/Laisky/one-api/relay/tooling"
)

// nativeSettledErrorWriter commits headers and then fails every body write,
// modelling a downstream client whose connection broke after the response began.
type nativeSettledErrorWriter struct {
	gin.ResponseWriter
	cause  error
	writes atomic.Int32
}

// Write commits the status line like a real connection, then reports the delivery failure.
func (w *nativeSettledErrorWriter) Write([]byte) (int, error) {
	w.writes.Add(1)
	w.WriteHeaderNow()
	return 0, w.cause
}

// WriteString follows the same failed delivery boundary as Write.
func (w *nativeSettledErrorWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

// nativeSettledErrorCancelWriter forwards writes and cancels the client request
// after the provider receipt was delivered, like a client closing its stream.
type nativeSettledErrorCancelWriter struct {
	gin.ResponseWriter
	cancel context.CancelFunc
}

// Write delivers the frame and cancels only once the terminal receipt has been forwarded.
func (w *nativeSettledErrorCancelWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if strings.Contains(string(p), "response.completed") {
		w.cancel()
	}
	return n, err
}

// WriteString keeps the cancellation boundary for string renderers.
func (w *nativeSettledErrorCancelWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

const nativeSettledCompletedEvent = "event: response.completed\n" +
	`data: {"type":"response.completed","response":{"id":"resp-settled","object":"response","status":"completed","model":"gpt-4o","output":[{"type":"message","id":"msg-settled","role":"assistant","content":[{"type":"output_text","text":"answer"}]}],"usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18}}}` +
	"\n\n"

const nativeSettledCompletedJSON = `{"id":"resp-settled","object":"response","status":"completed","model":"gpt-4o","output":[{"type":"message","id":"msg-settled","role":"assistant","content":[{"type":"output_text","text":"answer"}]}],"usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18}}`

// clearNativeSettledLedger removes rows left by an earlier -count round, because
// the shared SQLite fixture derives request IDs deterministically from the test name.
func clearNativeSettledLedger(t *testing.T, requestID string) {
	t.Helper()
	require.NoError(t, model.LOG_DB.Where("request_id = ?", requestID).Delete(&model.Log{}).Error)
	require.NoError(t, model.DB.Where("request_id = ?", requestID).Delete(&model.UserRequestCost{}).Error)
}

// TestNativeResponseSettledErrorIsReturned drives the real native Responses
// handler over real HTTP and checks that a failure observed after the provider
// receipt is settled exactly once AND still reported to the relay controller,
// instead of being converted into a success.
func TestNativeResponseSettledErrorIsReturned(t *testing.T) {
	for _, tc := range []struct {
		name string
		// stream selects SSE versus a JSON body.
		stream bool
		// exit selects how the attempt ends after the provider receipt:
		// "delivery" fails every client write, "cancel" cancels the client after
		// the receipt was delivered, "reset" aborts the provider connection.
		exit string
		// code is the one-api error code reported for the interruption.
		code string
	}{
		{name: "stream/delivery", stream: true, exit: "delivery", code: "write_stream_failed"},
		{name: "stream/cancel", stream: true, exit: "cancel", code: "read_stream_failed"},
		{name: "stream/reset", stream: true, exit: "reset", code: "read_stream_failed"},
		{name: "json/delivery", stream: false, exit: "delivery", code: "write_response_body_failed"},
		{name: "stream/success", stream: true, exit: "success"},
		{name: "json/success", stream: false, exit: "success"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance = int64(100_000)
			xaiVideoSetup(t, balance, false)
			var calls atomic.Int32
			upstreamStopped := make(chan struct{})
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				defer close(upstreamStopped)
				_, _ = io.Copy(io.Discard, r.Body)
				if !tc.stream {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, nativeSettledCompletedJSON)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if tc.exit == "reset" || tc.exit == "cancel" {
					// An unfinished body makes the interruption observable as a
					// transport failure rather than an ordinary chunked EOF.
					w.Header().Set("Content-Length", "100000")
				}
				_, _ = io.WriteString(w, nativeSettledCompletedEvent)
				w.(http.Flusher).Flush()
				switch tc.exit {
				case "reset":
					panic(http.ErrAbortHandler)
				case "cancel":
					select {
					case <-r.Context().Done():
					case <-time.After(5 * time.Second):
					}
					panic(http.ErrAbortHandler)
				}
			}))
			t.Cleanup(server.Close)
			previousClient := client.HTTPClient
			client.HTTPClient = server.Client()
			client.HTTPClient.Timeout = 6 * time.Second
			t.Cleanup(func() { client.HTTPClient = previousClient })

			body := `{"model":"alias","input":"hello","max_output_tokens":16,"stream":false}`
			if tc.stream {
				body = `{"model":"alias","input":"hello","max_output_tokens":16,"stream":true}`
			}
			c, downstream, requestID := protocolContext(t, channeltype.OpenAICompatible, "gpt-4o", "/v1/responses", body, server.URL, balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 1})
			clearNativeSettledLedger(t, requestID)
			c.Set(ctxkey.Config, model.ChannelConfig{APIFormat: channeltype.OpenAICompatibleAPIFormatResponse})
			require.True(t, supportsNativeResponseAPI(metalib.GetByContext(c)), "fixture must enter the actual native Responses handler")

			ctx, cancel := context.WithCancel(c.Request.Context())
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
			deliveryCause := stderrors.New("synthetic downstream delivery failure")
			failing := &nativeSettledErrorWriter{ResponseWriter: c.Writer, cause: deliveryCause}
			switch tc.exit {
			case "delivery":
				c.Writer = failing
			case "cancel":
				c.Writer = &nativeSettledErrorCancelWriter{ResponseWriter: c.Writer, cancel: cancel}
			}

			apiErr := RelayResponseAPIHelper(c)
			drainCriticalTasks(t)
			select {
			case <-upstreamStopped:
			case <-time.After(6 * time.Second):
				t.Fatal("fake provider did not finish")
			}
			require.EqualValues(t, 1, calls.Load(), "exactly one provider dispatch")

			// The measured receipt (11 input + 7 output at ratio 1) is settled
			// exactly once, whether or not the downstream delivery succeeded.
			const charge = int64(18)
			require.EqualValues(t, charge, requestCostQuota(t, requestID))
			require.Equal(t, balance-charge, reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, balance-charge, token.RemainQuota)
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1, "settlement consumes exactly once")
			require.EqualValues(t, charge, logs[0].Quota)

			if tc.exit == "success" {
				require.Nil(t, apiErr, "a delivered response must remain a success")
				require.True(t, c.Writer.Written(), "a delivered response is committed")
				require.Contains(t, downstream.Body.String(), "resp-settled")
				return
			}

			// The failure observed after settlement must reach the relay
			// controller so it is logged, measured and excluded from success
			// metrics, while the settlement marker vetoes a paid replay.
			require.NotNil(t, apiErr, "a settled attempt that failed must not be reported as success")
			require.Equal(t, tc.code, apiErr.Code, "the code names where the interruption originated")
			require.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
			require.Equal(t, relaymodel.ErrorTypeOneAPI, apiErr.Type)
			require.False(t, BillingAllowsRetry(c), "a settled attempt must never be replayed")
			require.True(t, c.Writer.Written(), "the committed response must not be overwritten by an error body")
			switch tc.exit {
			case "delivery":
				require.ErrorIs(t, apiErr.RawError, deliveryCause)
				require.Positive(t, failing.writes.Load())
			case "cancel":
				// The relay controller reports client cancellation at WARN without
				// penalising the channel; that relies on the preserved cause.
				require.ErrorIs(t, apiErr.RawError, context.Canceled)
				require.Contains(t, downstream.Body.String(), "resp-settled", "the receipt was delivered before cancellation")
			case "reset":
				require.NotErrorIs(t, apiErr.RawError, context.Canceled)
				require.Contains(t, downstream.Body.String(), "resp-settled", "the receipt was delivered before the provider reset")
			}
		})
	}
}

// TestNativeResponseMissingDeepSeekReceiptReportsError reproduces the silent
// empty-200 variant: a DeepSeek native Responses reply that cannot be decoded
// writes nothing downstream, yet the admitted paid-tool floor is retained. The
// helper must report the failure so the relay controller can still answer the
// client, and must veto a replay of the funded attempt.
func TestNativeResponseMissingDeepSeekReceiptReportsError(t *testing.T) {
	for _, tc := range []struct {
		name, body string
	}{
		{name: "truncated_json", body: `{"id":"truncated","object":"response","status":"comp`},
		{name: "error_envelope", body: `{"error":{"message":"synthetic provider failure","type":"server_error","code":"synthetic"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const balance int64 = 1_000_000
			xaiVideoSetup(t, balance, false)
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(server.Close)
			target, err := url.Parse(server.URL)
			require.NoError(t, err)
			previous := client.HTTPClient
			client.HTTPClient = &http.Client{Transport: deepSeekReceiptTransport{next: server.Client().Transport, target: target}}
			t.Cleanup(func() { client.HTTPClient.CloseIdleConnections(); client.HTTPClient = previous })

			c, downstream, requestID := protocolContext(t, channeltype.OpenAI, "deepseek-v4-pro", "/v1/responses",
				`{"model":"alias","input":"synthetic","tools":[{"type":"web_search"}],"tool_choice":"auto"}`,
				"https://api.openai.com", balance, 1, false, &model.ModelConfigLocal{Ratio: 1, CompletionRatio: 2})
			clearNativeSettledLedger(t, requestID)
			c.Set(ctxkey.Config, model.ChannelConfig{EndpointURLs: map[string]string{channeltype.RelayModeToEndpointName(relaymode.ResponseAPI): "https://api.deepseek.com/v1/responses"}})
			raw, _ := c.Get(ctxkey.ChannelModel)
			require.NoError(t, raw.(*model.Channel).SetToolingConfig(&model.ChannelToolingConfig{Pricing: map[string]model.ToolPricingLocal{"web_search": {QuotaPerCall: 17}}}))

			apiErr := RelayResponseAPIHelper(c)
			drainCriticalTasks(t)
			require.EqualValues(t, 1, calls.Load(), "exactly one provider dispatch")
			floor := tooling.ConservativeDeepSeekToolQuota(c)
			require.Positive(t, floor, "fixture must admit a funded DeepSeek tool")

			require.NotNil(t, apiErr, "an undecodable provider reply must not become an empty success")
			require.False(t, c.Writer.Written(), "nothing was delivered, so the relay controller can still write the error")
			require.Empty(t, downstream.Body.String())
			require.False(t, BillingAllowsRetry(c), "the retained paid-tool floor must not be replayed on another channel")

			// The admitted paid-tool floor stays charged exactly once.
			charge := requestCostQuota(t, requestID)
			require.GreaterOrEqual(t, charge, floor)
			require.Equal(t, balance-charge, reloadUserQuota(t))
			var token model.Token
			require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
			require.Equal(t, balance-charge, token.RemainQuota)
			var consume []model.Log
			require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&consume).Error)
			require.Len(t, consume, 1, "settlement consumes exactly once")
			require.Equal(t, "deepseek_tool_response_missing_usage", consume[0].Metadata["billing_estimate_reason"])
		})
	}
}
