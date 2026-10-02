package azure

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

// The azure adaptor must satisfy the shared adaptor interface and expose
// per-model tooling defaults so Claude vs OpenAI tool pricing dispatch correctly.
var (
	_ adaptor.Adaptor                         = (*Adaptor)(nil)
	_ adaptor.ToolingDefaultsForModelProvider = (*Adaptor)(nil)
)

// TestGetRequestURL_DispatchesByModelFamily verifies Claude models resolve to the
// native Anthropic surface (/anthropic/v1/messages) while OpenAI models fall
// through to the embedded Azure OpenAI deployment URL.
func TestGetRequestURL_DispatchesByModelFamily(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}
	base := "https://myres.services.ai.azure.com"

	claudeURL, err := a.GetRequestURL(&meta.Meta{
		ChannelType:     channeltype.Azure,
		BaseURL:         base,
		OriginModelName: "claude-sonnet-5",
		ActualModelName: "claude-sonnet-5",
	})
	require.NoError(t, err)
	require.Equal(t, base+"/anthropic/v1/messages", claudeURL)

	// Trailing slash on the resource endpoint is normalized.
	claudeURL2, err := a.GetRequestURL(&meta.Meta{
		ChannelType:     channeltype.Azure,
		BaseURL:         base + "/",
		OriginModelName: "claude-opus-4-8",
		ActualModelName: "claude-opus-4-8",
	})
	require.NoError(t, err)
	require.Equal(t, base+"/anthropic/v1/messages", claudeURL2)

	gptURL, err := a.GetRequestURL(&meta.Meta{
		ChannelType:     channeltype.Azure,
		BaseURL:         base,
		OriginModelName: "gpt-4o-mini",
		ActualModelName: "gpt-4o-mini",
		Mode:            relaymode.ChatCompletions,
		RequestURLPath:  "/v1/chat/completions",
	})
	require.NoError(t, err)
	require.Contains(t, gptURL, "/openai/deployments/gpt-4o-mini/")
	require.NotContains(t, gptURL, "/anthropic/")
}

// TestPricingDispatchesByModelFamily verifies Claude models bill at Anthropic
// rates and OpenAI models at OpenAI rates.
func TestPricingDispatchesByModelFamily(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}
	oai := &openai.Adaptor{}
	ant := &anthropic.Adaptor{}

	require.Greater(t, a.GetModelRatio("claude-sonnet-5"), 0.0)
	require.Equal(t, ant.GetModelRatio("claude-sonnet-5"), a.GetModelRatio("claude-sonnet-5"))
	require.Equal(t, ant.GetCompletionRatio("claude-sonnet-5"), a.GetCompletionRatio("claude-sonnet-5"))

	require.Equal(t, oai.GetModelRatio("gpt-4o-mini"), a.GetModelRatio("gpt-4o-mini"))
	require.Equal(t, oai.GetCompletionRatio("gpt-4o-mini"), a.GetCompletionRatio("gpt-4o-mini"))

	pricing := a.GetDefaultModelPricing()
	require.Contains(t, pricing, "claude-sonnet-5")
	require.Contains(t, pricing, "gpt-4o-mini")
}

// TestGetModelListCoversBothFamilies verifies the curated Azure catalog includes
// the Azure OpenAI models and the Foundry Claude models, but excludes legacy
// Claude models Azure AI Foundry does not host.
func TestGetModelListCoversBothFamilies(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}
	models := a.GetModelList()

	require.Contains(t, models, "claude-sonnet-5")
	require.Contains(t, models, "claude-opus-4-8")
	require.Contains(t, models, "gpt-4o-mini")

	// Legacy Claude models not offered on Azure AI Foundry must not be advertised.
	require.NotContains(t, models, "claude-2.0")
	require.NotContains(t, models, "claude-3-opus-20240229")
}

// TestGetChannelName pins the channel identity used in logs/metrics.
func TestGetChannelName(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}
	a.Init(&meta.Meta{ChannelType: channeltype.Azure})
	require.Equal(t, "azure", a.GetChannelName())
}

// TestDefaultToolingConfigForModel verifies Claude models receive Anthropic tool
// defaults while OpenAI models receive OpenAI tool defaults.
func TestDefaultToolingConfigForModel(t *testing.T) {
	t.Parallel()

	a := &Adaptor{}
	require.Equal(t, (&anthropic.Adaptor{}).DefaultToolingConfig(), a.DefaultToolingConfigForModel("claude-sonnet-5"))
	require.Equal(t, (&openai.Adaptor{}).DefaultToolingConfig(), a.DefaultToolingConfigForModel("gpt-4o-mini"))
}
