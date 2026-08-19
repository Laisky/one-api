package openai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/relaymode"
)

func TestAzureGetRequestURLRequiresModel(t *testing.T) {
	m := &meta.Meta{
		Mode:            relaymode.ChatCompletions,
		ChannelType:     channeltype.Azure,
		BaseURL:         "https://example.azure.com",
		ActualModelName: "",
		RequestURLPath:  "/v1/chat/completions",
		Config:          model.ChannelConfig{APIVersion: "2024-06-01"},
	}

	a := &Adaptor{}
	a.Init(m)

	_, err := a.GetRequestURL(m)
	require.Error(t, err)

	m.ActualModelName = "gpt-4o-mini"
	url, err := a.GetRequestURL(m)
	require.NoError(t, err)
	require.Contains(t, url, "/openai/deployments/gpt-4o-mini/chat/completions?api-version=")
}

func TestAzureGPT5UsesResponseAPI(t *testing.T) {
	config := model.ChannelConfig{APIVersion: "2025-04-01-preview"}
	m := &meta.Meta{
		Mode:            relaymode.ChatCompletions,
		ChannelType:     channeltype.Azure,
		BaseURL:         "https://example.azure.com",
		ActualModelName: "gpt-5-mini",
		RequestURLPath:  "/v1/chat/completions",
		Config:          config,
	}

	a := &Adaptor{}
	a.Init(m)

	url, err := a.GetRequestURL(m)
	require.NoError(t, err)
	require.Contains(t, url, "/openai/v1/responses?api-version=v1")
}
