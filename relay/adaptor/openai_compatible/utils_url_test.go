package openai_compatible

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/channeltype"
)

func TestGetFullRequestURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		base        string
		path        string
		expect      string
		channelType int
	}{
		{
			name:        "compatible-base-with-v1",
			base:        "https://proxy.example.com/v1",
			path:        "/v1/chat/completions",
			expect:      "https://proxy.example.com/v1/chat/completions",
			channelType: channeltype.OpenAICompatible,
		},
		{
			name:        "compatible-base-without-v1",
			base:        "https://proxy.example.com",
			path:        "/v1/chat/completions",
			expect:      "https://proxy.example.com/v1/chat/completions",
			channelType: channeltype.OpenAICompatible,
		},
		{
			name:        "compatible-non-v1",
			base:        "https://proxy.example.com/v1",
			path:        "/dashboard/billing/usage",
			expect:      "https://proxy.example.com/v1/dashboard/billing/usage",
			channelType: channeltype.OpenAICompatible,
		},
		{
			name:        "other-type",
			base:        "https://api.example.com",
			path:        "/v1/chat/completions",
			expect:      "https://api.example.com/v1/chat/completions",
			channelType: channeltype.OpenAI,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.expect, GetFullRequestURL(tc.base, tc.path, tc.channelType))
		})
	}
}
