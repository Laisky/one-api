package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestCloudflareChannelTestModelsAreChatOnly verifies channel tests only expose
// and select text-in/text-out generation models. Text-input embedding models
// such as bge-m3 must never be sent through Chat Completions.
// Parameters: t is the test handle used to run assertions.
// Returns: no values.
func TestCloudflareChannelTestModelsAreChatOnly(t *testing.T) {
	t.Parallel()

	storedModel := "@cf/baai/bge-m3"
	channel := &model.Channel{
		Type: channeltype.Cloudflare,
		Models: "@cf/baai/bge-small-en-v1.5," +
			"@cf/baai/bge-base-en-v1.5," +
			"@cf/baai/bge-large-en-v1.5," +
			"@cf/baai/bge-m3," +
			"@cf/pfnet/plamo-embedding-1b," +
			"@cf/qwen/qwen3-embedding-0.6b," +
			"@cf/meta/llama-3.2-1b-instruct",
		TestingModel: &storedModel,
	}

	require.Equal(t,
		[]string{"@cf/meta/llama-3.2-1b-instruct"},
		channelTextTestModels(channel),
	)

	selectedModel, clearStored, err := chooseChannelTestModel(channel, "")
	require.NoError(t, err)
	require.True(t, clearStored)
	require.Equal(t, "@cf/meta/llama-3.2-1b-instruct", selectedModel)

	selectedModel, clearStored, err = chooseChannelTestModel(channel, "@cf/baai/bge-m3")
	require.Error(t, err)
	require.False(t, clearStored)
	require.Empty(t, selectedModel)
	require.Contains(t, err.Error(), "does not support both text input and text output")
}
