package gemini

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

func TestAdaptorGetRequestURLGeminiVersions(t *testing.T) {
	t.Parallel()
	adaptor := &Adaptor{}
	baseURL := "https://generativelanguage.googleapis.com"
	originalVersion := config.GeminiVersion
	config.GeminiVersion = "v1"
	defer func() {
		config.GeminiVersion = originalVersion
	}()

	testCases := []struct {
		name            string
		model           string
		expectedVersion string
	}{
		{name: "GeminiThree", model: "gemini-3-pro-preview", expectedVersion: "v1beta"},
		{name: "GeminiTwoFive", model: "gemini-2.5-flash", expectedVersion: "v1beta"},
		{name: "GeminiTwoZero", model: "gemini-2.0-flash", expectedVersion: "v1beta"},
		{name: "GeminiLegacy", model: "gemini-1.0-pro", expectedVersion: "v1"},
		{name: "GemmaThree", model: "gemma-3-8b-it", expectedVersion: "v1beta"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			metaInfo := &meta.Meta{
				ActualModelName: tc.model,
				Mode:            relaymode.ChatCompletions,
				BaseURL:         baseURL,
			}

			url, err := adaptor.GetRequestURL(metaInfo)
			require.NoError(t, err)
			expected := fmt.Sprintf("%s/%s/models/%s:%s", baseURL, tc.expectedVersion, tc.model, "generateContent")
			require.Equal(t, expected, url)
		})
	}
}
