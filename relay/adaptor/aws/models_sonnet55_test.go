package aws

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	channelmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	awsclaude "github.com/Laisky/one-api/relay/adaptor/aws/claude"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// TestClaudeSonnet55BedrockCatalog checks the public catalog, child dispatch, and
// explicit launch reference rates. It takes a test handle and returns nothing;
// these assertions verify gateway defaults, not an AWS Marketplace invoice.
func TestClaudeSonnet55BedrockCatalog(t *testing.T) {
	t.Parallel()
	const id = "claude-sonnet-5-5"
	a := &Adaptor{}
	require.Contains(t, a.GetModelList(), id)
	require.IsType(t, &awsclaude.Adaptor{}, GetAdaptor(id))
	mapped, err := awsclaude.AwsModelID(id)
	require.NoError(t, err)
	require.Equal(t, "anthropic.claude-sonnet-5-5", mapped)
	cfg, ok := a.GetDefaultModelPricing()[id]
	require.True(t, ok)
	require.InDelta(t, 2*ratio.MilliTokensUsd, a.GetModelRatio(id), 1e-12)
	require.InDelta(t, 10*ratio.MilliTokensUsd, cfg.Ratio*a.GetCompletionRatio(id), 1e-12)
	require.InDelta(t, 0.2*ratio.MilliTokensUsd, cfg.CachedInputRatio, 1e-12)
	require.InDelta(t, 2.5*ratio.MilliTokensUsd, cfg.CacheWrite5mRatio, 1e-12)
	require.InDelta(t, 4*ratio.MilliTokensUsd, cfg.CacheWrite1hRatio, 1e-12)
	require.EqualValues(t, 1000000, cfg.ContextLength)
	require.EqualValues(t, 128000, cfg.MaxOutputTokens)
	require.Zero(t, cfg.MaxReasoningTokens)
	require.ElementsMatch(t, []string{"text", "image", "file"}, cfg.InputModalities)
	require.Equal(t, []string{"text"}, cfg.OutputModalities)
	require.ElementsMatch(t, []string{"tools", "reasoning"}, cfg.SupportedFeatures)
	require.ElementsMatch(t, []string{"stop", "max_tokens"}, cfg.SupportedSamplingParameters)
	require.Empty(t, cfg.TimeWindows)
	require.Contains(t, anthropic.ModelRatios[id].SupportedFeatures, "structured_outputs")
	for _, invented := range []string{id + "-latest", id + "-20260928", id + ":batch"} {
		require.NotContains(t, a.GetModelList(), invented)
		_, err := awsclaude.AwsModelID(invented)
		require.Error(t, err)
	}
}

// TestClaudeSonnet55BedrockInvoke exercises the real SDK for native and converted
// requests, streaming and JSON replies, and explicit profile overrides. It takes
// a test handle and returns nothing; its HTTP fixtures cannot reach AWS.
func TestClaudeSonnet55BedrockInvoke(t *testing.T) {
	t.Parallel()
	const id = "claude-sonnet-5-5"
	const global = "global.anthropic.claude-sonnet-5-5"
	const arn = "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/sonnet55-test"
	for _, stream := range []bool{false, true} {
		for _, native := range []bool{false, true} {
			for _, explicit := range []bool{false, true} {
				name := "json"
				if stream {
					name = "stream"
				}
				if native {
					name += "/native"
				} else {
					name += "/chat"
				}
				if explicit {
					name += "/explicit-arn"
				} else {
					name += "/global"
				}
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					body := []byte(strings.ReplaceAll(claudeInvokeReceipt, "claude-3-haiku-20240307", id))
					if stream {
						body = claudeEventFrames(t,
							strings.ReplaceAll(claudeInvokeStart, "claude-3-haiku-20240307", id),
							claudeInvokeTextStart, claudeInvokeText, claudeInvokeTextStop,
							claudeInvokeDelta, claudeInvokeDelta, claudeInvokeStop)
					}
					h := newClaudeInvokeHarness(t, stream, body, "us-east-1")
					h.meta.ActualModelName = id
					target := global
					if explicit {
						target = arn
						mapping := `{"` + id + `":"` + arn + `"}`
						h.context.Set(ctxkey.ChannelModel, &channelmodel.Channel{InferenceProfileArnMap: &mapping})
					}
					if native {
						h.prepareNative(t, `{"model":"claude-sonnet-5-5","max_tokens":256,"messages":[{"role":"user","content":"Hello"}],"output_config":{"effort":"high"},"future_field":{"id":9007199254740993}}`)
					} else {
						h.prepareChat(t)
					}
					usage, responseErr := h.adaptor.DoResponse(h.context, nil, h.meta)
					require.Nil(t, responseErr)
					require.NotNil(t, usage)
					require.Equal(t, 21, usage.PromptTokens)
					require.Equal(t, 50, usage.CompletionTokens)
					require.NotNil(t, usage.PromptTokensDetails)
					require.Equal(t, 1000, usage.PromptTokensDetails.CachedTokens)
					require.Equal(t, 100, usage.CacheWrite5mTokens)
					require.Equal(t, 200, usage.CacheWrite1hTokens)
					suffix := "/invoke"
					if stream {
						suffix = "/invoke-with-response-stream"
					}
					require.Equal(t, []string{"/model/" + target + suffix}, h.transport.paths, "one invocation, no availability probe")
					require.Len(t, h.transport.bodies, 1)
					var payload map[string]json.RawMessage
					require.NoError(t, json.Unmarshal([]byte(h.transport.bodies[0]), &payload))
					require.NotContains(t, payload, "model")
					require.NotContains(t, payload, "stream")
					require.JSONEq(t, `"bedrock-2023-05-31"`, string(payload["anthropic_version"]))
					if native {
						require.Contains(t, string(payload["future_field"]), "9007199254740993")
						require.JSONEq(t, `{"effort":"high"}`, string(payload["output_config"]))
					}
					require.Contains(t, h.recorder.Body.String(), "Hello")
				})
			}
		}
	}
}
