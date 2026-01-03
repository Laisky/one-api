package groq

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
)

func TestGetRequestURL(t *testing.T) {
	t.Parallel()
	adaptor := &Adaptor{}

	testCases := []struct {
		name           string
		requestURLPath string
		expectedURL    string
		baseURL        string
		channelType    int
	}{
		{
			name:           "Claude Messages API with query conversion",
			requestURLPath: "/v1/messages?beta=true",
			expectedURL:    "https://api.groq.com/v1/chat/completions",
			baseURL:        "https://api.groq.com",
			channelType:    channeltype.Groq,
		},
		{
			name:           "Claude Messages API conversion",
			requestURLPath: "/v1/messages",
			expectedURL:    "https://api.groq.com/v1/chat/completions",
			baseURL:        "https://api.groq.com",
			channelType:    channeltype.Groq,
		},
		{
			name:           "OpenAI Chat Completions passthrough",
			requestURLPath: "/v1/chat/completions",
			expectedURL:    "https://api.groq.com/v1/chat/completions",
			baseURL:        "https://api.groq.com",
			channelType:    channeltype.Groq,
		},
		{
			name:           "Other endpoints passthrough",
			requestURLPath: "/v1/models",
			expectedURL:    "https://api.groq.com/v1/models",
			baseURL:        "https://api.groq.com",
			channelType:    channeltype.Groq,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			meta := &meta.Meta{
				RequestURLPath: tc.requestURLPath,
				BaseURL:        tc.baseURL,
				ChannelType:    tc.channelType,
			}

			url, err := adaptor.GetRequestURL(meta)
			require.NoError(t, err, "GetRequestURL failed")
			require.Equal(t, tc.expectedURL, url)
		})
	}
}
