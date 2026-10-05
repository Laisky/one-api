package controller

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/client"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/xai"
	"github.com/Laisky/one-api/relay/channeltype"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

const lyriaClipModel = "google/lyria-3-clip-preview"

// lyriaProtocolRequest returns the endpoint and body for one Lyria request in the
// requested client API format. Parameters: protocol is chat, responses or
// messages, and stream selects SSE delivery. Returns: the path and JSON body.
func lyriaProtocolRequest(protocol string, stream bool) (string, string) {
	streamField := ""
	if stream {
		streamField = `,"stream":true`
	}
	switch protocol {
	case "responses":
		return "/v1/responses", `{"model":"alias","input":"Create music","max_output_tokens":128` + streamField + `}`
	case "messages":
		return "/v1/messages", `{"model":"alias","messages":[{"role":"user","content":"Create music"}],"max_tokens":128` + streamField + `}`
	default:
		return "/v1/chat/completions", `{"model":"alias","messages":[{"role":"user","content":"Create music"}],"max_tokens":128` + streamField + `}`
	}
}

// relayLyriaProtocol dispatches one request through the production helper for
// the client API format. Parameters: c is the prepared request and protocol
// selects the helper. Returns: the helper's API error, or nil on success.
func relayLyriaProtocol(c *gin.Context, protocol string) *relaymodel.ErrorWithStatusCode {
	switch protocol {
	case "responses":
		return RelayResponseAPIHelper(c)
	case "messages":
		return RelayClaudeMessagesHelper(c)
	default:
		return RelayTextHelper(c)
	}
}

// lyriaChatReply returns a non-streaming OpenRouter-style chat completion that
// carries generated audio. Parameters: t owns the WAV fixture. Returns: JSON text.
func lyriaChatReply(t *testing.T) string {
	t.Helper()
	audioData := base64.StdEncoding.EncodeToString(silentWAV(t, 1))
	return `{"id":"generation-fixture","object":"chat.completion","model":"` + lyriaClipModel + `","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"Generated music","audio":{"id":"audio-fixture","data":"` + audioData + `","transcript":"Music"}}}],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`
}

// consumeLogQuota sums finalized consume-log charges for one request. Parameters:
// t owns assertions and requestID selects the ledger rows. Returns: the total.
func consumeLogQuota(t *testing.T, requestID string) int64 {
	t.Helper()
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&logs).Error)
	var total int64
	for _, entry := range logs {
		total += int64(entry.Quota)
	}
	return total
}

// TestSecurityLyriaUpstreamErrorRemainsRetryable verifies that an explicit provider
// error response does not finalize the one-generation hold or veto the shared
// cross-channel retry. Parameters: t owns the fixture. Returns: none; the retry
// attempt must settle exactly one generation and refund the abandoned hold.
func TestSecurityLyriaUpstreamErrorRemainsRetryable(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "messages"} {
		for _, status := range []int{http.StatusTooManyRequests, http.StatusBadGateway} {
			t.Run(protocol+"/"+http.StatusText(status), func(t *testing.T) {
				const balance = int64(100000)
				xaiVideoSetup(t, balance, false)
				var calls atomic.Int32
				reply := lyriaChatReply(t)
				upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if calls.Add(1) == 1 {
						w.WriteHeader(status)
						_, _ = io.WriteString(w, `{"error":{"message":"provider unavailable","code":`+strconv.Itoa(status)+`}}`)
						return
					}
					_, _ = io.WriteString(w, reply)
				}))
				t.Cleanup(upstream.Close)
				previous := client.HTTPClient
				client.HTTPClient = upstream.Client()
				t.Cleanup(func() { client.HTTPClient = previous })

				path, body := lyriaProtocolRequest(protocol, false)
				c, _, id := protocolContext(t, channeltype.OpenRouter, lyriaClipModel, path, body, upstream.URL+"/v1", balance, 1, false, nil)
				first := relayLyriaProtocol(c, protocol)
				drainCriticalTasks(t)
				require.NotNil(t, first, "the provider error must surface")
				require.EqualValues(t, 1, calls.Load())
				require.True(t, BillingAllowsRetry(c), "an explicit provider error must not veto the cross-channel retry")
				require.Zero(t, consumeLogQuota(t, id), "an explicit provider error must not finalize a generation charge")

				ResetPerAttemptBillingForRetry(gmw.Ctx(c), c)
				drainCriticalTasks(t)
				require.Equal(t, balance, reloadUserQuota(t), "the abandoned attempt's hold must be refunded before retry")

				requestBody, err := common.GetRequestBody(c)
				require.NoError(t, err)
				c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
				second := relayLyriaProtocol(c, protocol)
				drainCriticalTasks(t)
				require.Nil(t, second)
				require.EqualValues(t, 2, calls.Load())
				require.Equal(t, balance-20000, reloadUserQuota(t), "the successful retry settles exactly one generation")
				var token model.Token
				require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
				require.Equal(t, balance-20000, token.RemainQuota)
				require.EqualValues(t, 20000, consumeLogQuota(t, id))
			})
		}
	}
}

// TestSecurityLyriaStreamingGenerationTariff verifies the SSE delivery that
// OpenRouter requires for audio output reserves and settles one generation.
// Parameters: t owns the fixture. Returns: none; missing stream usage and a
// disconnected client must still settle the full flat generation exactly once.
func TestSecurityLyriaStreamingGenerationTariff(t *testing.T) {
	for _, protocol := range []string{"chat", "responses", "messages"} {
		for _, tc := range []struct {
			name      string
			usage     bool
			writeFail bool
		}{
			{name: "usage", usage: true},
			{name: "missing_usage"},
			{name: "client_disconnect", usage: true, writeFail: true},
		} {
			t.Run(protocol+"/"+tc.name, func(t *testing.T) {
				const balance = int64(100000)
				xaiVideoSetup(t, balance, false)
				audioData := base64.StdEncoding.EncodeToString(silentWAV(t, 1))
				var calls atomic.Int32
				reserved := make(chan int64, 1)
				upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					var wire map[string]any
					if err := json.NewDecoder(r.Body).Decode(&wire); err == nil {
						var user model.User
						if err := model.DB.First(&user, fallbackUserID).Error; err == nil {
							reserved <- user.Quota
						}
					}
					w.Header().Set("Content-Type", "text/event-stream")
					chunks := []string{
						`{"id":"gen-stream","object":"chat.completion.chunk","model":"` + lyriaClipModel + `","choices":[{"index":0,"delta":{"role":"assistant","content":"Generated music"}}]}`,
						`{"id":"gen-stream","object":"chat.completion.chunk","model":"` + lyriaClipModel + `","choices":[{"index":0,"delta":{"audio":{"id":"audio-fixture","data":"` + audioData + `","transcript":"Music"}}}]}`,
						`{"id":"gen-stream","object":"chat.completion.chunk","model":"` + lyriaClipModel + `","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
					}
					if tc.usage {
						chunks = append(chunks, `{"id":"gen-stream","object":"chat.completion.chunk","model":"`+lyriaClipModel+`","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`)
					}
					for _, chunk := range chunks {
						_, _ = io.WriteString(w, "data: "+chunk+"\n\n")
					}
					_, _ = io.WriteString(w, "data: [DONE]\n\n")
				}))
				t.Cleanup(upstream.Close)
				previous := client.HTTPClient
				client.HTTPClient = upstream.Client()
				t.Cleanup(func() { client.HTTPClient = previous })

				path, body := lyriaProtocolRequest(protocol, true)
				c, _, id := protocolContext(t, channeltype.OpenRouter, lyriaClipModel, path, body, upstream.URL+"/v1", balance, 1, false, nil)
				if tc.writeFail {
					c.Writer = xaiDisconnectedWriter{ResponseWriter: c.Writer}
				}
				apiErr := relayLyriaProtocol(c, protocol)
				drainCriticalTasks(t)
				if !tc.writeFail {
					require.Nil(t, apiErr)
				}
				require.EqualValues(t, 1, calls.Load())
				require.Equal(t, balance-20000, <-reserved, "the generation must be reserved before provider work")
				require.Equal(t, balance-20000, reloadUserQuota(t))
				var token model.Token
				require.NoError(t, model.DB.First(&token, fallbackTokenID).Error)
				require.Equal(t, balance-20000, token.RemainQuota)
				require.EqualValues(t, 20000, consumeLogQuota(t, id))
			})
		}
	}
}

// TestSecurityLyriaNonChatEndpointFailsClosed verifies a per-generation catalog
// contract is not silently re-priced by an endpoint whose settlement uses another
// unit. Parameters: t owns the fixture. Returns: none; the provider is never called.
func TestSecurityLyriaNonChatEndpointFailsClosed(t *testing.T) {
	const balance = int64(100000)
	xaiVideoSetup(t, balance, false)
	var calls atomic.Int32
	sound := silentWAV(t, 1)
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(sound)
	}))
	t.Cleanup(upstream.Close)
	previous := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = previous })

	body := `{"model":"alias","input":"Create music","voice":"alloy","response_format":"wav"}`
	c, _, _ := protocolContext(t, channeltype.OpenRouter, lyriaClipModel, "/v1/audio/speech", body, upstream.URL+"/api", balance, 1, false, nil)
	apiErr := RelayAudioHelper(c, relaymode.AudioSpeech)
	drainCriticalTasks(t)
	require.NotNil(t, apiErr)
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	require.Zero(t, calls.Load(), "an unsupported generation endpoint must fail before provider work")
	require.Equal(t, balance, reloadUserQuota(t))
}

// TestSecurityLyriaTokenRatioOverrideFailsClosed verifies a channel token-ratio
// override cannot silently re-price a per-generation contract. Parameters: t owns
// the fixture. Returns: none; the operator must configure per_call pricing instead.
func TestSecurityLyriaTokenRatioOverrideFailsClosed(t *testing.T) {
	const balance = int64(100000)
	xaiVideoSetup(t, balance, false)
	var calls atomic.Int32
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, lyriaChatReply(t))
	}))
	t.Cleanup(upstream.Close)
	previous := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = previous })

	path, body := lyriaProtocolRequest("chat", false)
	local := &model.ModelConfigLocal{Ratio: 2, CompletionRatio: 1}
	c, _, _ := protocolContext(t, channeltype.OpenRouter, lyriaClipModel, path, body, upstream.URL+"/v1", balance, 1, false, local)
	apiErr := RelayTextHelper(c)
	drainCriticalTasks(t)
	require.NotNil(t, apiErr)
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	require.Zero(t, calls.Load())
	require.Equal(t, balance, reloadUserQuota(t))
}

// TestSecurityGenerationTariffNativeResponsesFailsClosed verifies the native
// Responses reservation, which prices tokens itself, cannot dispatch a catalog
// per-generation contract without reserving it. Parameters: t owns the fixture.
// Returns: none; a provider catalog fixture is restored after the test.
func TestSecurityGenerationTariffNativeResponsesFailsClosed(t *testing.T) {
	const name = "generation-tariff-fixture"
	xai.ModelRatios[name] = adaptor.ModelConfig{CompletionRatio: 1, PerCall: &adaptor.PerCallPricingConfig{UsdPerThousandCalls: 40},
		PricingProvenance: &adaptor.PricingProvenance{State: adaptor.TariffStatePaid, Unit: adaptor.TariffUnitGeneration, Source: "https://example.test/tariff", VerifiedAt: "2026-10-05"}}
	t.Cleanup(func() { delete(xai.ModelRatios, name) })
	const balance = int64(100000)
	xaiVideoSetup(t, balance, false)
	var calls atomic.Int32
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(upstream.Close)
	previous := client.HTTPClient
	client.HTTPClient = upstream.Client()
	t.Cleanup(func() { client.HTTPClient = previous })

	path, body := lyriaProtocolRequest("responses", false)
	c, _, _ := protocolContext(t, channeltype.XAI, name, path, body, upstream.URL, balance, 1, false, nil)
	apiErr := RelayResponseAPIHelper(c)
	drainCriticalTasks(t)
	require.NotNil(t, apiErr)
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	require.Zero(t, calls.Load(), "an unreserved generation must not reach the provider")
	require.Equal(t, balance, reloadUserQuota(t))
}
