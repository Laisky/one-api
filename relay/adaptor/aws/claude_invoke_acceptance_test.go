package aws

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/quota"
)

const claudeNativeAcceptanceBody = `{"model":"claude-3-haiku-20240307","max_tokens":256,"messages":[{"role":"user","content":"Hello"}]}`

// completeClaudeFrames returns a complete real-SDK EventStream fixture.
func completeClaudeFrames(t *testing.T) []byte {
	t.Helper()
	return claudeEventFrames(t, claudeInvokeStart, claudeInvokeTextStart, claudeInvokeText,
		claudeInvokeTextStop, claudeInvokeDelta, claudeInvokeStop)
}

// claudeDeliveryWriter injects a synchronous client write failure or cancellation.
type claudeDeliveryWriter struct {
	gin.ResponseWriter
	fail   bool
	cancel context.CancelFunc
}

// Write fails before delivery or cancels immediately after a successful write.
func (w *claudeDeliveryWriter) Write(p []byte) (int, error) {
	if w.fail {
		return 0, errors.New("synthetic client write failure")
	}
	n, err := w.ResponseWriter.Write(p)
	if w.cancel != nil {
		w.cancel()
	}
	return n, err
}

// WriteString exercises the same failure path for string-based renderers.
func (w *claudeDeliveryWriter) WriteString(p string) (int, error) { return w.Write([]byte(p)) }

// TestClaudeInvokeDeliveryFailuresPreserveReceipts verifies failed delivery is
// not successful generation, but cannot discard already-reported paid usage.
func TestClaudeInvokeDeliveryFailuresPreserveReceipts(t *testing.T) {
	t.Parallel()
	for _, stream := range []bool{false, true} {
		for _, native := range []bool{false, true} {
			name := map[bool]string{false: "json", true: "stream"}[stream] + "/" + map[bool]string{false: "chat", true: "native"}[native]
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				body := []byte(claudeInvokeReceipt)
				if stream {
					body = completeClaudeFrames(t)
				}
				h := newClaudeInvokeHarness(t, stream, body, "test-region-1")
				if native {
					h.prepareNative(t, claudeNativeAcceptanceBody)
				} else {
					h.prepareChat(t)
				}
				h.context.Writer = &claudeDeliveryWriter{ResponseWriter: h.context.Writer, fail: true}
				usage, responseErr := h.adaptor.DoResponse(h.context, nil, h.meta)
				require.NotNil(t, responseErr)
				require.NotNil(t, usage)
				require.Equal(t, 21, usage.PromptTokens)
				require.Equal(t, 1000, usage.PromptTokensDetails.CachedTokens)
				require.Equal(t, 100, usage.CacheWrite5mTokens)
				require.Equal(t, 200, usage.CacheWrite1hTokens)
				if !stream {
					require.Equal(t, 50, usage.CompletionTokens)
				}
				require.Len(t, h.transport.bodies, 1)
				require.NotContains(t, h.recorder.Body.String(), "[DONE]")
			})
		}
	}
}

// TestClaudeInvokeCancellationRetainsPartialReceipt deterministically cancels
// after message_start without sleeps or concurrent access to a gin context.
func TestClaudeInvokeCancellationRetainsPartialReceipt(t *testing.T) {
	t.Parallel()
	h := newClaudeInvokeHarness(t, true, completeClaudeFrames(t), "test-region-1")
	h.prepareChat(t)
	ctx, cancel := context.WithCancel(h.context.Request.Context())
	defer cancel()
	h.context.Request = h.context.Request.WithContext(ctx)
	h.context.Writer = &claudeDeliveryWriter{ResponseWriter: h.context.Writer, cancel: cancel}
	usage, responseErr := h.adaptor.DoResponse(h.context, nil, h.meta)
	require.NotNil(t, responseErr)
	require.ErrorIs(t, responseErr.RawError, context.Canceled)
	require.NotNil(t, usage)
	require.Equal(t, 21, usage.PromptTokens)
	require.Equal(t, 1, usage.CompletionTokens)
	require.NotContains(t, h.recorder.Body.String(), "[DONE]")
}

// TestClaudeInvokeLateSDKFailureWithholdsCompletion checks failure after the
// JSON message_stop still prevents successful client completion markers.
func TestClaudeInvokeLateSDKFailureWithholdsCompletion(t *testing.T) {
	t.Parallel()
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{false: "chat", true: "native"}[native], func(t *testing.T) {
			t.Parallel()
			frames := append(completeClaudeFrames(t), claudeExceptionFrame(t)...)
			h := newClaudeInvokeHarness(t, true, frames, "test-region-1")
			if native {
				h.prepareNative(t, claudeNativeAcceptanceBody)
			} else {
				h.prepareChat(t)
			}
			usage, responseErr := h.adaptor.DoResponse(h.context, nil, h.meta)
			require.NotNil(t, responseErr)
			require.Equal(t, 50, usage.CompletionTokens)
			require.NotContains(t, h.recorder.Body.String(), "[DONE]")
			require.NotContains(t, h.recorder.Body.String(), "event: message_stop")
		})
	}
}

// TestClaudeInvokeNativeUnknownContentPreserved checks nested unmodeled fields
// and opaque signatures survive without a lossy typed Content round trip.
func TestClaudeInvokeNativeUnknownContentPreserved(t *testing.T) {
	t.Parallel()
	const block = `{"type":"content_block_start","index":0,"content_block":{"type":"future_tool_result","content":[{"id":9007199254740993}],"signature":"opaque+/=="}}`
	h := newClaudeInvokeHarness(t, true, claudeEventFrames(t, claudeInvokeStart, block,
		claudeInvokeTextStop, `{"type":"future_event","value":{"x":1}}`, claudeInvokeDelta, claudeInvokeStop), "test-region-1")
	h.prepareNative(t, claudeNativeAcceptanceBody)
	_, responseErr := h.adaptor.DoResponse(h.context, nil, h.meta)
	require.Nil(t, responseErr)
	require.Contains(t, h.recorder.Body.String(), block)
	require.Contains(t, h.recorder.Body.String(), "event: future_event")
}

// TestClaudeInvokeUsesPreparedBodyAndRejectsStaleBytes checks authoritative
// sanitized-reader selection, re-preparation, beta headers, and failure cleanup.
func TestClaudeInvokeUsesPreparedBodyAndRejectsStaleBytes(t *testing.T) {
	t.Parallel()
	h := newClaudeInvokeHarness(t, false, []byte(claudeInvokeReceipt), "test-region-1")
	h.prepareNative(t, claudeNativeAcceptanceBody)
	h.context.Request.Body = io.NopCloser(strings.NewReader(`{"messages":"unsanitized inbound sentinel"}`))
	h.context.Request.Header.Set("anthropic-beta", "first, second, first")
	prepared := strings.TrimSuffix(claudeNativeAcceptanceBody, "}") + `,"output_config":{"effort":"medium"},"anthropic_beta":["first"],"future_id":9007199254740993}`
	_, err := h.adaptor.DoRequest(h.context, h.meta, strings.NewReader(prepared))
	require.NoError(t, err)
	_, responseErr := h.adaptor.DoResponse(h.context, nil, h.meta)
	require.Nil(t, responseErr)
	require.Len(t, h.transport.bodies, 1)
	require.Contains(t, h.transport.bodies[0], `"future_id":9007199254740993`)
	require.Contains(t, h.transport.bodies[0], `"anthropic_beta":["first","second"]`)
	require.NotContains(t, h.transport.bodies[0], "unsanitized inbound sentinel")
	for _, bad := range []string{"{", "null", "[]"} {
		_, err := h.adaptor.DoRequest(h.context, h.meta, strings.NewReader(bad))
		require.Error(t, err)
		_, responseErr := h.adaptor.DoResponse(h.context, nil, h.meta)
		require.NotNil(t, responseErr)
		require.Len(t, h.transport.bodies, 1, "a failed preparation must not reuse the prior body")
	}
}

// claudeRejectTransport returns an AWS service rejection after capturing the request.
type claudeRejectTransport struct {
	capture *claudeInvokeTransport
	status  int
}

// RoundTrip returns a real SDK-decodable error without making a network request.
func (r *claudeRejectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	response, err := r.capture.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if err := response.Body.Close(); err != nil {
		return nil, errors.Wrap(err, "close unused test response")
	}
	response.StatusCode = r.status
	response.Header.Set("X-Amzn-Errortype", "AccessDeniedException")
	response.Body = io.NopCloser(strings.NewReader(`{"message":"synthetic service rejection"}`))
	return response, nil
}

// TestClaudeInvokeRejectionNeverProbesOrFallsBack checks status propagation and
// deterministic profile selection even when the account cannot use the model.
func TestClaudeInvokeRejectionNeverProbesOrFallsBack(t *testing.T) {
	t.Parallel()
	for _, status := range []int{400, 403, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			h := newClaudeInvokeHarness(t, false, nil, "us-east-1")
			h.meta.ActualModelName = "claude-sonnet-5"
			options := h.adaptor.AwsClient.Options()
			options.HTTPClient = &http.Client{Transport: &claudeRejectTransport{capture: h.transport, status: status}}
			h.adaptor.AwsClient = bedrockruntime.New(options)
			h.prepareChat(t)
			usage, responseErr := h.adaptor.DoResponse(h.context, nil, h.meta)
			require.NotNil(t, responseErr)
			require.Equal(t, status, responseErr.StatusCode)
			require.Nil(t, usage)
			require.Equal(t, []string{"/model/global.anthropic.claude-sonnet-5/invoke"}, h.transport.paths)
		})
	}
}

// claudeResponseBridge records the interface used by /v1/responses conversion.
type claudeResponseBridge struct {
	chunks int
	done   int
	usage  *relaymodel.Usage
}

// HandleChunk records a converted chunk and reports it as handled.
func (b *claudeResponseBridge) HandleChunk(_ *gin.Context, _ *openai_compatible.ChatCompletionsStreamResponse) (bool, bool) {
	b.chunks++
	return true, false
}

// HandleUpstreamDone reports an upstream completion without finalizing twice.
func (b *claudeResponseBridge) HandleUpstreamDone(_ *gin.Context) (bool, bool) { return true, false }

// HandleDone records exactly one successful Responses completion.
func (b *claudeResponseBridge) HandleDone(_ *gin.Context) (bool, bool) { b.done++; return true, true }

// FinalizeUsage records the final receipt presented to the Responses bridge.
func (b *claudeResponseBridge) FinalizeUsage(usage *relaymodel.Usage) { b.usage = usage }

// TestClaudeInvokeResponsesBridgeCompletion distinguishes successful completion
// from failure while ensuring failed requests retain their billing receipt.
func TestClaudeInvokeResponsesBridgeCompletion(t *testing.T) {
	t.Parallel()
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[failure], func(t *testing.T) {
			t.Parallel()
			frames := completeClaudeFrames(t)
			if failure {
				frames = append(claudeEventFrames(t, claudeInvokeStart), claudeExceptionFrame(t)...)
			}
			h := newClaudeInvokeHarness(t, true, frames, "test-region-1")
			h.prepareChat(t)
			bridge := &claudeResponseBridge{}
			h.context.Set(ctxkey.ResponseStreamRewriteHandler, bridge)
			usage, responseErr := h.adaptor.DoResponse(h.context, nil, h.meta)
			require.Positive(t, bridge.chunks)
			require.NotNil(t, usage)
			require.Empty(t, h.recorder.Body.String(), "bridge must not leak raw chat chunks")
			if failure {
				require.NotNil(t, responseErr)
				require.Zero(t, bridge.done)
				require.Nil(t, bridge.usage)
			} else {
				require.Nil(t, responseErr)
				require.Equal(t, 1, bridge.done)
				require.Equal(t, usage, bridge.usage)
			}
		})
	}
}

// TestClaudeInvokeActualQuotaCalculation validates SDK receipts against the real
// quota engine and an independent five-bucket arithmetic oracle.
func TestClaudeInvokeActualQuotaCalculation(t *testing.T) {
	t.Parallel()
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "stream"}[stream], func(t *testing.T) {
			t.Parallel()
			body := []byte(claudeInvokeReceipt)
			if stream {
				// The final aggregate must retain the one-hour write price.
				body = claudeEventFrames(t, claudeInvokeStart, claudeInvokeTextStart, claudeInvokeText,
					claudeInvokeTextStop, claudeInvokeDelta,
					`{"type":"message_delta","delta":{},"usage":{"output_tokens":50,"cache_creation_input_tokens":300}}`, claudeInvokeStop)
			}
			h := newClaudeInvokeHarness(t, stream, body, "test-region-1")
			h.prepareChat(t)
			usage, responseErr := h.adaptor.DoResponse(h.context, nil, h.meta)
			require.Nil(t, responseErr)
			cfg := h.adaptor.GetDefaultModelPricing()[h.meta.ActualModelName]
			for _, group := range []float64{1, 1.5, 2} {
				result := quota.Compute(quota.ComputeInput{Usage: usage, ModelName: h.meta.ActualModelName,
					ModelRatio: cfg.Ratio, GroupRatio: group, PricingAdaptor: h.adaptor})
				want := int64(math.Ceil(group * (21*cfg.Ratio + 50*cfg.Ratio*cfg.CompletionRatio +
					1000*cfg.CachedInputRatio + 100*cfg.CacheWrite5mRatio + 200*cfg.CacheWrite1hRatio)))
				require.Equal(t, want, result.TotalQuota)
				require.Equal(t, 1000, result.CachedPromptTokens)
			}
		})
	}
}

// TestClaudeInvokeMultipleToolsKeepIndices verifies argument fragments and empty
// tool objects cannot be attached to another tool when content indices differ.
func TestClaudeInvokeMultipleToolsKeepIndices(t *testing.T) {
	t.Parallel()
	frames := claudeEventFrames(t, claudeInvokeStart,
		`{"type":"content_block_start","index":3,"content_block":{"type":"tool_use","id":"tool_a","name":"first","input":{}}}`,
		`{"type":"content_block_delta","index":3,"delta":{"type":"input_json_delta","partial_json":"{\"id\":"}}`,
		`{"type":"content_block_delta","index":3,"delta":{"type":"input_json_delta","partial_json":"9007199254740993}"}}`,
		`{"type":"content_block_stop","index":3}`,
		`{"type":"content_block_start","index":7,"content_block":{"type":"tool_use","id":"tool_b","name":"second","input":{}}}`,
		`{"type":"content_block_stop","index":7}`,
		strings.Replace(claudeInvokeDelta, "end_turn", "tool_use", 1), claudeInvokeStop)
	h := newClaudeInvokeHarness(t, true, frames, "test-region-1")
	h.prepareChat(t)
	_, responseErr := h.adaptor.DoResponse(h.context, nil, h.meta)
	require.Nil(t, responseErr)
	arguments := map[int]string{}
	for _, line := range strings.Split(h.recorder.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: {") {
			continue
		}
		var chunk openai_compatible.ChatCompletionsStreamResponse
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk))
		for _, choice := range chunk.Choices {
			for _, tool := range choice.Delta.ToolCalls {
				require.NotNil(t, tool.Index)
				require.NotNil(t, tool.Function)
				if tool.Function.Arguments != nil {
					raw, ok := tool.Function.Arguments.(string)
					require.True(t, ok)
					arguments[*tool.Index] += raw
				}
			}
		}
	}
	require.Equal(t, map[int]string{0: `{"id":9007199254740993}`, 1: `{}`}, arguments)
	require.Equal(t, 1, bytes.Count(h.recorder.Body.Bytes(), []byte("[DONE]")))
}
