package openai

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// TestOpenAIOctoberProRequestNormalization verifies real request transformations
// against the documented Pro effort contracts. Parameter t owns assertions;
// the test returns no values and performs no network calls.
func TestOpenAIOctoberProRequestNormalization(t *testing.T) {
	for name, defaultEffort := range map[string]string{
		"gpt-5.4-pro":            "medium",
		"gpt-5.4-pro-2026-03-05": "medium",
		"gpt-5.5-pro":            "high",
		"gpt-5.5-pro-2026-04-23": "high",
	} {
		for _, surface := range []struct {
			mode int
			path string
		}{
			{relaymode.ChatCompletions, "/v1/chat/completions"},
			{relaymode.ClaudeMessages, "/v1/messages"},
			{relaymode.ResponseAPI, "/v1/responses"},
		} {
			for _, effort := range []string{"", "medium", "high", "xhigh", "low", "minimal", "none", "max"} {
				t.Run(name+surface.path+"/"+effort, func(t *testing.T) {
					cfg, ok := ModelRatios[name]
					require.True(t, ok)
					expected := effort
					if effort != "medium" && effort != "high" && effort != "xhigh" {
						expected = defaultEffort
					}
					var requested *string
					if effort != "" {
						requested = &effort
					}
					temperature, topP := 0.2, 0.9
					messages := []model.Message{{Role: "user", Content: "Explain the patch."}}
					tools := []model.Tool{{Type: "web_search"}}
					req := &model.GeneralOpenAIRequest{
						Model: "tenant-alias", ReasoningEffort: requested,
						Temperature: &temperature, TopP: &topP, MaxTokens: 256,
						Messages: messages, Tools: tools,
					}
					info := &meta.Meta{ChannelType: channeltype.OpenAI, ActualModelName: name,
						Mode: surface.mode, BaseURL: "https://api.openai.com", RequestURLPath: surface.path}
					a := &Adaptor{ChannelType: channeltype.OpenAI}
					url, err := a.GetRequestURL(info)
					require.NoError(t, err)
					require.Equal(t, "https://api.openai.com/v1/responses", url)
					require.NoError(t, a.applyRequestTransformations(info, req))
					require.NotNil(t, req.ReasoningEffort)
					require.Equal(t, expected, *req.ReasoningEffort)
					require.Equal(t, []string{"medium", "high", "xhigh"}, cfg.SupportedReasoningEfforts)
					require.Equal(t, defaultEffort, cfg.DefaultReasoningEffort)
					require.Nil(t, req.Temperature)
					require.Nil(t, req.TopP)
					require.Equal(t, messages, req.Messages)
					require.Equal(t, tools, req.Tools)
					require.NotNil(t, req.MaxCompletionTokens)
					require.Equal(t, 256, *req.MaxCompletionTokens)
				})
			}
		}
	}
}

// TestOpenAIOctoberPublishedSnapshots checks discoverability, metadata and actual
// audio routing. Parameter t owns assertions; the test returns no values.
func TestOpenAIOctoberPublishedSnapshots(t *testing.T) {
	a := &Adaptor{ChannelType: channeltype.OpenAI}
	for snapshot, base := range map[string]string{
		"gpt-5.4-pro-2026-03-05":            "gpt-5.4-pro",
		"gpt-5.5-pro-2026-04-23":            "gpt-5.5-pro",
		"gpt-4o-mini-transcribe-2025-03-20": "gpt-4o-mini-transcribe",
		"gpt-4o-mini-transcribe-2025-12-15": "gpt-4o-mini-transcribe",
	} {
		t.Run(snapshot, func(t *testing.T) {
			require.Contains(t, a.GetModelList(), snapshot)
			require.Contains(t, ModelList, snapshot)
			cfg, ok := a.GetDefaultModelPricing()[snapshot]
			require.True(t, ok)
			baseCfg := ModelRatios[base]
			cfg.Description = baseCfg.Description
			require.Equal(t, baseCfg, cfg)
			if strings.Contains(snapshot, "transcribe") {
				require.EqualValues(t, 16000, cfg.ContextLength)
				require.EqualValues(t, 2000, cfg.MaxOutputTokens)
				require.Contains(t, cfg.Description, "json or text only")
				info := &meta.Meta{ChannelType: channeltype.OpenAI, ActualModelName: snapshot,
					Mode: relaymode.AudioTranscription, BaseURL: "https://api.openai.com", RequestURLPath: "/v1/audio/transcriptions"}
				url, err := a.GetRequestURL(info)
				require.NoError(t, err)
				require.Equal(t, "https://api.openai.com/v1/audio/transcriptions", url)
			}
		})
	}
	require.NotContains(t, ModelRatios["gpt-5.4-pro"].SupportedFeatures, "structured_outputs")
	require.Contains(t, ModelRatios["gpt-5.5-pro"].SupportedFeatures, "structured_outputs")
}

// TestOpenAIOctoberChatAlias verifies the rolling alias is not described as a
// fixed GPT-5.5 model. Parameter t owns assertions; the test returns no values.
func TestOpenAIOctoberChatAlias(t *testing.T) {
	cfg := ModelRatios["chat-latest"]
	require.NotContains(t, cfg.Description, "GPT-5.5")
	require.Contains(t, cfg.Description, "2026-10-07")
	require.Contains(t, cfg.SupportedFeatures, "web_search")
	require.EqualValues(t, 400000, cfg.ContextLength)
	require.EqualValues(t, 128000, cfg.MaxOutputTokens)
}

// TestOpenAIOctoberLifecycleGuidance checks representative exact retirement dates
// while retaining format warnings. Parameter t owns assertions; no values return.
func TestOpenAIOctoberLifecycleGuidance(t *testing.T) {
	for name, date := range map[string]string{
		"gpt-5.3-codex": "2027-04-01", "gpt-5.4-nano": "2027-04-01",
		"tts-1": "2027-01-06", "whisper-1": "2027-02-26",
		"gpt-4o-mini-transcribe-2025-03-20": "2027-01-20",
		"gpt-image-1.5":                     "2026-12-01", "gpt-4o-2024-08-06": "2027-01-14",
		"gpt-3.5-turbo": "2026-10-23", "sora-2": "2026-09-24",
		"gpt-3.5-turbo-instruct": "2026-09-28", "gpt-5.3-chat-latest": "2026-08-10",
	} {
		t.Run(name, func(t *testing.T) {
			cfg, ok := ModelRatios[name]
			require.True(t, ok)
			require.Contains(t, cfg.Description, "announced shutdown date "+date)
			require.Contains(t, ModelList, name)
		})
	}
	require.Contains(t, ModelRatios["whisper-1"].Description, "srt/vtt/verbose_json")
	require.Contains(t, ModelRatios["gpt-4o-mini-transcribe"].Description, "json or text only")
	require.NotContains(t, ModelRatios["gpt-5.3-chat-latest"].Description, "use gpt-5.5)")
	// No invented lifecycle propagation to similarly named models or aliases.
	for _, name := range []string{"gpt-6.1-sol", "gpt-4o", "gpt-4.1", "gpt-4-1106-preview", "gpt-4o-mini-transcribe-2025-12-15"} {
		require.NotContains(t, ModelRatios[name].Description, "[OpenAI lifecycle:", name)
	}
}
