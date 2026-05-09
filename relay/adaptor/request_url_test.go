package adaptor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "trim-space-and-trailing-slash",
			input:  " https://api.example.com/v1/ ",
			expect: "https://api.example.com/v1",
		},
		{
			name:   "preserve-empty",
			input:  "   ",
			expect: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expect, NormalizeBaseURL(tt.input))
		})
	}
}

func TestNormalizeRequestPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "trim-and-prefix-slash",
			input:  " v1/chat/completions?foo=bar ",
			expect: "/v1/chat/completions?foo=bar",
		},
		{
			name:   "preserve-empty",
			input:  "   ",
			expect: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expect, NormalizeRequestPath(tt.input))
		})
	}
}

func TestHasVersionSuffix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		expect bool
	}{
		{
			name:   "plain-v1",
			input:  "https://api.example.com/v1",
			expect: true,
		},
		{
			name:   "alphanumeric-suffix",
			input:  "https://api.example.com/v1beta/",
			expect: true,
		},
		{
			name:   "not-terminal-segment",
			input:  "https://api.example.com/v1/chat/completions",
			expect: false,
		},
		{
			name:   "non-matching-preview-format",
			input:  "https://api.example.com/v1-preview",
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expect, HasVersionSuffix(tt.input))
		})
	}
}

func TestStripOpenAIV1Prefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		path          string
		exactV1Result string
		expect        string
	}{
		{
			name:          "exact-v1-preserve-root",
			path:          "/v1",
			exactV1Result: "/",
			expect:        "/",
		},
		{
			name:          "exact-v1-drop-root",
			path:          "/v1",
			exactV1Result: "",
			expect:        "",
		},
		{
			name:          "versioned-endpoint",
			path:          " /v1/chat/completions?foo=bar ",
			exactV1Result: "/",
			expect:        "/chat/completions?foo=bar",
		},
		{
			name:          "v11-not-trimmed",
			path:          "/v11/chat/completions",
			exactV1Result: "/",
			expect:        "/v11/chat/completions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expect, StripOpenAIV1Prefix(tt.path, tt.exactV1Result))
		})
	}
}

func TestJoinBaseURLAndPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		baseURL    string
		requestURL string
		expect     string
	}{
		{
			name:       "normalize-and-join",
			baseURL:    " https://api.example.com/v1/ ",
			requestURL: " v1/chat/completions ",
			expect:     "https://api.example.com/v1/v1/chat/completions",
		},
		{
			name:       "empty-path-returns-base",
			baseURL:    " https://api.example.com/v1/ ",
			requestURL: "   ",
			expect:     "https://api.example.com/v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expect, JoinBaseURLAndPath(tt.baseURL, tt.requestURL))
		})
	}
}
