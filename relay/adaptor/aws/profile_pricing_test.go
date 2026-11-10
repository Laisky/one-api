package aws

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Laisky/one-api/common/ctxkey"
	channelmodel "github.com/Laisky/one-api/model"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
	"github.com/stretchr/testify/require"
)

// TestClaude52ProfilePricingAdmission blocks unknown or Geo tariff overrides before SDK invocation.
func TestClaude52ProfilePricingAdmission(t *testing.T) {
	for _, name := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		for _, target := range []string{"arn:aws:bedrock:us-east-1:123456789012:inference-profile/us.anthropic." + name, "arn:aws-us-gov:bedrock:us-gov-east-1:123456789012:application-inference-profile/operator"} {
			h := newClaudeInvokeHarness(t, false, []byte(claudeInvokeReceipt), "us-east-1")
			h.meta.ActualModelName = name
			mapping, err := json.Marshal(map[string]string{name: target})
			require.NoError(t, err)
			saved := string(mapping)
			ch := &channelmodel.Channel{InferenceProfileArnMap: &saved}
			h.context.Set(ctxkey.ChannelModel, ch)
			converted, err := h.adaptor.ConvertRequest(h.context, relaymode.ChatCompletions, &relaymodel.GeneralOpenAIRequest{Model: name, MaxTokens: 64, Messages: []relaymodel.Message{{Role: "user", Content: "hi"}}})
			require.NoError(t, err)
			body, err := json.Marshal(converted)
			require.NoError(t, err)
			_, err = h.adaptor.DoRequest(h.context, h.meta, bytes.NewReader(body))
			require.ErrorContains(t, err, "explicit")
			require.Empty(t, h.transport.paths)
			require.False(t, h.context.GetBool(ctxkey.UpstreamRequestPossiblyForwarded))
		}
	}
}

// TestClaude52ProfileTariffControls preserves automatic/legacy routing and validates complete override tariffs.
func TestClaude52ProfileTariffControls(t *testing.T) {
	h := newClaudeInvokeHarness(t, false, []byte(claudeInvokeReceipt), "us-east-1")
	h.meta.ActualModelName = "claude-fable-5-2"
	require.NoError(t, h.adaptor.ValidateRequestPricing(h.context, h.meta), "automatic global retains published default")
	mapping := `{"claude-fable-5-2":"arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/custom"}`
	cfg := channelmodel.ModelConfigLocal{Ratio: 4.4, CompletionRatio: 5, CachedInputRatio: .11, CacheWrite5mRatio: 5.5, CacheWrite1hRatio: 8.8}
	ch := &channelmodel.Channel{InferenceProfileArnMap: &mapping}
	h.context.Set(ctxkey.ChannelModel, ch)
	for _, which := range []string{"complete", "input", "output", "read", "write5", "write1"} {
		t.Run(which, func(t *testing.T) {
			c := cfg
			switch which {
			case "input":
				c.Ratio = 0
			case "output":
				c.CompletionRatio = 0
			case "read":
				c.CachedInputRatio = 0
			case "write5":
				c.CacheWrite5mRatio = 0
			case "write1":
				c.CacheWrite1hRatio = 0
			}
			raw, err := json.Marshal(map[string]channelmodel.ModelConfigLocal{h.meta.ActualModelName: c})
			require.NoError(t, err)
			saved := string(raw)
			ch.ModelConfigs = &saved
			err = h.adaptor.ValidateRequestPricing(h.context, h.meta)
			if which == "complete" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			require.Equal(t, saved, *ch.ModelConfigs, "validation must not rewrite administrator rates")
		})
	}
	h.meta.ActualModelName = "claude-3-haiku-20240307"
	ch.ModelConfigs = nil
	require.NoError(t, h.adaptor.ValidateRequestPricing(h.context, h.meta), "legacy tariffs are untouched")
}

// TestClaude52ProfilePricingMatchesDispatch binds alias lookup to the same override consumed by invocation.
func TestClaude52ProfilePricingMatchesDispatch(t *testing.T) {
	h := newClaudeInvokeHarness(t, false, []byte(claudeInvokeReceipt), "us-east-1")
	h.meta.ActualModelName = "claude-fable-5-2"
	h.prepareChat(t)
	h.context.Set(ctxkey.RequestModel, "public-alias")
	mapping := `{"public-alias":"arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/operator"}`
	ch := &channelmodel.Channel{InferenceProfileArnMap: &mapping}
	h.context.Set(ctxkey.ChannelModel, ch)
	_, err := h.adaptor.DoRequest(h.context, h.meta, bytes.NewReader([]byte(`{"messages":[]}`)))
	require.ErrorContains(t, err, "explicit")
	prices := `{"claude-fable-5-2":{"ratio":4.4,"completion_ratio":5,"cached_input_ratio":0.11,"cache_write_5m_ratio":5.5,"cache_write_1h_ratio":8.8}}`
	ch.ModelConfigs = &prices
	_, err = h.adaptor.DoRequest(h.context, h.meta, bytes.NewReader([]byte(`{"messages":[]}`)))
	require.NoError(t, err)
	_, failure := h.adaptor.DoResponse(h.context, nil, h.meta)
	require.Nil(t, failure)
	require.Len(t, h.transport.paths, 1)
	require.Contains(t, h.transport.paths[0], "application-inference-profile")
	require.Nil(t, GetAdaptor("us.anthropic.claude-fable-5-2"), "raw profile IDs cannot bypass canonical catalog admission")
}
