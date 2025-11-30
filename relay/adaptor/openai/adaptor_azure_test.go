package openai

import (
	"testing"

	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/stretchr/testify/require"
)

func TestGetRequestURL_AzureRequiresModel(t *testing.T) {
	a := &Adaptor{}
	m := &meta.Meta{ChannelType: channeltype.Azure, BaseURL: "https://example.openai.azure.com", RequestURLPath: "/v1/chat/completions"}
	_, err := a.GetRequestURL(m)
	require.Error(t, err, "expected error when ActualModelName is empty for Azure")

	m.ActualModelName = "gpt-4o-mini"
	_, err = a.GetRequestURL(m)
	require.NoError(t, err, "unexpected error building Azure URL with model")
}
