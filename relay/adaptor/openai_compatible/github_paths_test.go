package openai_compatible

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/relaymode"
)

func TestIsGitHubModelsBaseURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		baseURL  string
		expected bool
	}{
		{name: "empty", baseURL: "", expected: false},
		{name: "plain host", baseURL: "models.github.ai", expected: true},
		{name: "https scheme", baseURL: "https://models.github.ai", expected: true},
		{name: "uppercase", baseURL: "HTTPS://MODELS.GITHUB.AI", expected: true},
		{name: "with path", baseURL: "https://models.github.ai/custom", expected: true},
		{name: "other host", baseURL: "https://api.openai.com", expected: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.expected, IsGitHubModelsBaseURL(tc.baseURL))
		})
	}
}

func TestNormalizeGitHubRequestPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		path     string
		mode     int
		expected string
	}{
		{name: "empty defaults", path: "", mode: relaymode.ChatCompletions, expected: "/inference/chat/completions"},
		{name: "root defaults", path: "/", mode: relaymode.ChatCompletions, expected: "/inference/chat/completions"},
		{name: "chat completions", path: "/v1/chat/completions", mode: relaymode.ChatCompletions, expected: "/inference/chat/completions"},
		{name: "responses", path: "/v1/responses", mode: relaymode.ChatCompletions, expected: "/inference/chat/completions"},
		{name: "messages", path: "/v1/messages", mode: relaymode.ChatCompletions, expected: "/inference/chat/completions"},
		{name: "embeddings default", path: "/v1/embeddings", mode: relaymode.Embeddings, expected: "/inference/embeddings"},
		{name: "org missing inference", path: "/orgs/octo", mode: relaymode.ChatCompletions, expected: "/orgs/octo/inference/chat/completions"},
		{name: "org chat completions", path: "/orgs/octo/chat/completions", mode: relaymode.ChatCompletions, expected: "/orgs/octo/inference/chat/completions"},
		{name: "org embeddings", path: "/orgs/octo/chat/completions", mode: relaymode.Embeddings, expected: "/orgs/octo/inference/embeddings"},
		{name: "already inference", path: "/inference/chat/completions", mode: relaymode.ChatCompletions, expected: "/inference/chat/completions"},
		{name: "already inference org", path: "/orgs/octo/inference/chat/completions/", mode: relaymode.ChatCompletions, expected: "/orgs/octo/inference/chat/completions"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.expected, NormalizeGitHubRequestPath(tc.path, tc.mode))
		})
	}
}
