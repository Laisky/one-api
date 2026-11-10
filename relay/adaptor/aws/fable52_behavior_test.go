package aws

import (
	"bytes"
	"encoding/json"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"net/http"
	"testing"

	"github.com/Laisky/one-api/common/ctxkey"
	channelmodel "github.com/Laisky/one-api/model"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/stretchr/testify/require"
)

// TestClaude52PublicReasoningEfforts verifies advertised efforts survive public validation and reach the SDK payload.
func TestClaude52PublicReasoningEfforts(t *testing.T) {
	for name, cfg := range awsFable52ModelPricing {
		for _, effort := range cfg.SupportedReasoningEfforts {
			t.Run(name+"/"+effort, func(t *testing.T) {
				h := newClaudeInvokeHarness(t, false, []byte(claudeInvokeReceipt), "us-east-1")
				h.meta.ActualModelName = name
				request := &relaymodel.GeneralOpenAIRequest{Model: name, MaxTokens: 256, ReasoningEffort: &effort, Messages: []relaymodel.Message{{Role: "user", Content: "Hello"}}}
				converted, err := h.adaptor.ConvertRequest(h.context, relaymode.ChatCompletions, request)
				require.NoError(t, err)
				body, err := json.Marshal(converted)
				require.NoError(t, err)
				_, err = h.adaptor.DoRequest(h.context, h.meta, bytes.NewReader(body))
				require.NoError(t, err)
				_, failure := h.adaptor.DoResponse(h.context, nil, h.meta)
				require.Nil(t, failure)
				require.Len(t, h.transport.bodies, 1)
				var payload struct {
					OutputConfig struct {
						Effort string `json:"effort"`
					} `json:"output_config"`
				}
				require.NoError(t, json.Unmarshal([]byte(h.transport.bodies[0]), &payload))
				require.Equal(t, effort, payload.OutputConfig.Effort)
				require.Equal(t, effort, *request.ReasoningEffort)
			})
		}
	}
}

// TestClaude52ProfileAdmission verifies unqualified regions fail before the SDK while explicit administrator profiles remain usable.
func TestClaude52ProfileAdmission(t *testing.T) {
	for _, name := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		for _, stream := range []bool{false, true} {
			for _, region := range []string{"us-gov-east-1", "us-gov-west-1", "cn-north-1"} {
				t.Run(name+"/"+region+map[bool]string{false: "/sync", true: "/stream"}[stream], func(t *testing.T) {
					body := []byte(claudeInvokeReceipt)
					if stream {
						body = completeClaudeFrames(t)
					}
					h := newClaudeInvokeHarness(t, stream, body, region)
					h.meta.ActualModelName = name
					h.prepareChat(t)
					_, failure := h.adaptor.DoResponse(h.context, nil, h.meta)
					require.NotNil(t, failure)
					require.Contains(t, failure.Error.Message, "explicit inference profile ARN")
					require.Empty(t, h.transport.bodies)
					require.False(t, h.context.GetBool(ctxkey.UpstreamRequestPossiblyForwarded))
				})
			}
		}
		t.Run(name+"/explicit-profile", func(t *testing.T) {
			h := newClaudeInvokeHarness(t, false, []byte(claudeInvokeReceipt), "us-gov-east-1")
			h.meta.ActualModelName = name
			mapping := "{\"" + name + "\":\"arn:aws-us-gov:bedrock:us-gov-east-1:123456789012:application-inference-profile/test\"}"
			prices := "{\"" + name + "\":{\"ratio\":4.4,\"completion_ratio\":5,\"cached_input_ratio\":0.11,\"cache_write_5m_ratio\":5.5,\"cache_write_1h_ratio\":8.8}}"
			h.context.Set(ctxkey.ChannelModel, &channelmodel.Channel{InferenceProfileArnMap: &mapping, ModelConfigs: &prices})
			h.prepareChat(t)
			_, failure := h.adaptor.DoResponse(h.context, nil, h.meta)
			require.Nil(t, failure)
			require.Len(t, h.transport.paths, 1)
			require.Contains(t, h.transport.paths[0], "application-inference-profile")
		})
	}
}

// TestClaude52LegacyValidation preserves existing unsupported-model rejection and legacy GovCloud direct invocation.
func TestClaude52LegacyValidation(t *testing.T) {
	h := newClaudeInvokeHarness(t, false, []byte(claudeInvokeReceipt), "us-gov-east-1")
	effort := "high"
	_, err := h.adaptor.ConvertRequest(h.context, relaymode.ChatCompletions, &relaymodel.GeneralOpenAIRequest{Model: "claude-3-haiku-20240307", ReasoningEffort: &effort})
	require.ErrorContains(t, err, "reasoning_effort")
	h.prepareChat(t)
	_, failure := h.adaptor.DoResponse(h.context, nil, h.meta)
	require.Nil(t, failure)
	require.Len(t, h.transport.bodies, 1)
}

// TestClaude52NativeOpaqueThinking preserves opaque history and unknown future fields through the real SDK transport.
func TestClaude52NativeOpaqueThinking(t *testing.T) {
	for _, name := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		t.Run(name, func(t *testing.T) {
			h := newClaudeInvokeHarness(t, false, []byte(claudeInvokeReceipt), "us-east-1")
			h.meta.ActualModelName = name
			body := "{\"model\":\"" + name + "\",\"max_tokens\":256,\"messages\":[{\"role\":\"assistant\",\"content\":[{\"type\":\"thinking\",\"thinking\":\"opaque\",\"signature\":\"keep-exact==\"},{\"type\":\"redacted_thinking\",\"data\":\"opaque-data==\"}]},{\"role\":\"user\",\"content\":\"Continue\"}],\"future_field\":{\"id\":9007199254740993}}"
			h.prepareNative(t, body)
			_, failure := h.adaptor.DoResponse(h.context, nil, h.meta)
			require.Nil(t, failure)
			require.Len(t, h.transport.bodies, 1)
			var original, actual map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(body), &original))
			require.NoError(t, json.Unmarshal([]byte(h.transport.bodies[0]), &actual))
			require.JSONEq(t, string(original["messages"]), string(actual["messages"]))
			require.JSONEq(t, string(original["future_field"]), string(actual["future_field"]))
		})
	}
}

// TestClaude52EntitlementFailureNeverSubstitutes verifies a denied account does not trigger an alternate model or profile.
func TestClaude52EntitlementFailureNeverSubstitutes(t *testing.T) {
	for _, name := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		for _, stream := range []bool{false, true} {
			t.Run(name+map[bool]string{false: "/sync", true: "/stream"}[stream], func(t *testing.T) {
				h := newClaudeInvokeHarness(t, stream, nil, "us-east-1")
				h.meta.ActualModelName = name
				options := h.adaptor.AwsClient.Options()
				options.HTTPClient = &http.Client{Transport: &claudeRejectTransport{capture: h.transport, status: http.StatusForbidden}}
				h.adaptor.AwsClient = bedrockruntime.New(options)
				h.prepareChat(t)
				usage, failure := h.adaptor.DoResponse(h.context, nil, h.meta)
				require.NotNil(t, failure)
				require.Equal(t, http.StatusForbidden, failure.StatusCode)
				require.Nil(t, usage)
				require.Len(t, h.transport.paths, 1)
				require.Contains(t, h.transport.paths[0], "/model/global.anthropic."+name+"/invoke")
			})
		}
	}
}
