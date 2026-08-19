package controller

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
)

// TestResponseStatus ensures nil responses are handled without panics and return zero status.
func TestResponseStatus(t *testing.T) {
	t.Parallel()
	t.Run("nil response", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, 0, responseStatus(nil))
	})

	t.Run("non-nil response", func(t *testing.T) {
		t.Parallel()
		resp := &http.Response{StatusCode: http.StatusTeapot}
		require.Equal(t, http.StatusTeapot, responseStatus(resp))
	})
}

// TestModelConfigSupportsTextTest verifies that channel tests accept only text Chat Completions metadata.
func TestModelConfigSupportsTextTest(t *testing.T) {
	t.Parallel()

	require.True(t, modelConfigSupportsTextTest(adaptor.ModelConfig{
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"text"},
	}, true))
	require.False(t, modelConfigSupportsTextTest(adaptor.ModelConfig{
		InputModalities:  []string{"text"},
		OutputModalities: []string{"video"},
	}, true))
	require.False(t, modelConfigSupportsTextTest(adaptor.ModelConfig{
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Embedding:        &adaptor.EmbeddingPricingConfig{TextTokenRatio: 1},
	}, true))
	require.False(t, modelConfigSupportsTextTest(adaptor.ModelConfig{
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "A multilingual text embeddings model.",
	}, true))
	require.False(t, modelConfigSupportsTextTest(adaptor.ModelConfig{
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "A sentiment classifier for text classification.",
	}, true))
	require.True(t, modelConfigSupportsTextTest(adaptor.ModelConfig{}, false))
}

// TestModelNameLooksNonTextTestable distinguishes specialized tasks from similarly named chat models.
func TestModelNameLooksNonTextTestable(t *testing.T) {
	t.Parallel()

	require.True(t, modelNameLooksNonTextTestable("@cf/baai/bge-m3"))
	require.True(t, modelNameLooksNonTextTestable("gpt-3.5-turbo-instruct"))
	require.False(t, modelNameLooksNonTextTestable("amazon.nova-pro-v1:0"))
	require.False(t, modelNameLooksNonTextTestable("gpt-4o-mini"))
}

// TestChooseChannelTestModelFiltersNonChatModels verifies fallback selection skips models that cannot serve Chat Completions.
func TestChooseChannelTestModelFiltersNonChatModels(t *testing.T) {
	t.Parallel()

	channel := &model.Channel{
		Type:   channeltype.OpenAI,
		Models: "sora-2,gpt-4o-mini,text-embedding-3-small,gpt-3.5-turbo-instruct",
	}

	modelName, clearStored, err := chooseChannelTestModel(channel, "")
	require.NoError(t, err)
	require.False(t, clearStored)
	require.Equal(t, "gpt-4o-mini", modelName)
}

// TestChooseChannelTestModelSkipsCloudflareEmbeddingTasks reproduces the bge-m3 channel-test failure globally.
func TestChooseChannelTestModelSkipsCloudflareEmbeddingTasks(t *testing.T) {
	t.Parallel()

	channel := &model.Channel{
		Type: channeltype.Cloudflare,
		Models: strings.Join([]string{
			"@cf/baai/bge-m3",
			"@cf/baai/bge-reranker-base",
			"@cf/huggingface/distilbert-sst-2-int8",
			"@cf/qwen/qwen3-30b-a3b-fp8",
		}, ","),
	}

	modelName, clearStored, err := chooseChannelTestModel(channel, "")
	require.NoError(t, err)
	require.False(t, clearStored)
	require.Equal(t, "@cf/qwen/qwen3-30b-a3b-fp8", modelName)
}

// TestBuildChannelListResponseFiltersNonChatModels verifies channel rows expose only Chat Completions-compatible models.
func TestBuildChannelListResponseFiltersNonChatModels(t *testing.T) {
	t.Parallel()

	channels := []*model.Channel{
		{
			Type:   channeltype.OpenAI,
			Models: "sora-2,gpt-4o-mini,text-embedding-3-small,gpt-3.5-turbo-instruct",
		},
	}

	rows := buildChannelListResponse(channels)
	require.Len(t, rows, 1)
	require.Equal(t, []string{"gpt-4o-mini"}, rows[0].TestModels)
}

// TestBuildChannelListResponseFiltersCloudflareTasks verifies Cloudflare task models are absent from the test selector.
func TestBuildChannelListResponseFiltersCloudflareTasks(t *testing.T) {
	t.Parallel()

	channels := []*model.Channel{
		{
			Type: channeltype.Cloudflare,
			Models: strings.Join([]string{
				"@cf/baai/bge-m3",
				"@cf/baai/bge-reranker-base",
				"@cf/meta/m2m100-1.2b",
				"@cf/qwen/qwen3-30b-a3b-fp8",
			}, ","),
		},
	}

	rows := buildChannelListResponse(channels)
	require.Len(t, rows, 1)
	require.Equal(t, []string{"@cf/qwen/qwen3-30b-a3b-fp8"}, rows[0].TestModels)
}

// TestBuildChannelListResponseFiltersCompatibleChannels verifies compatible channels use global metadata.
func TestBuildChannelListResponseFiltersCompatibleChannels(t *testing.T) {
	t.Parallel()

	channels := []*model.Channel{
		{
			Type:   channeltype.OpenAICompatible,
			Models: "sora-2,gpt-4o-mini,text-embedding-3-small",
		},
	}

	rows := buildChannelListResponse(channels)
	require.Len(t, rows, 1)
	require.Equal(t, []string{"gpt-4o-mini"}, rows[0].TestModels)
}

// TestBuildChannelListResponseSerializesEmptyTestModels verifies empty filtered lists do not fall back to raw models.
func TestBuildChannelListResponseSerializesEmptyTestModels(t *testing.T) {
	t.Parallel()

	channels := []*model.Channel{
		{
			Type:   channeltype.OpenAI,
			Models: "dall-e-2,dall-e-3",
		},
	}

	rows := buildChannelListResponse(channels)
	require.Len(t, rows, 1)
	require.Empty(t, rows[0].TestModels)

	payload, err := json.Marshal(rows)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"test_models":[]`)
}

// TestBuildChannelListResponseRequiresChatEndpoint verifies endpoint-restricted channels expose no chat test choices.
func TestBuildChannelListResponseRequiresChatEndpoint(t *testing.T) {
	t.Parallel()

	channels := []*model.Channel{
		{
			Type:   channeltype.OpenAICompatible,
			Models: "gpt-4o-mini",
			Config: `{"supported_endpoints":["embeddings"]}`,
		},
	}

	rows := buildChannelListResponse(channels)
	require.Len(t, rows, 1)
	require.Empty(t, rows[0].TestModels)
}

// TestChooseChannelTestModelClearsStoredNonChatModel verifies stored testing models must support Chat Completions.
func TestChooseChannelTestModelClearsStoredNonChatModel(t *testing.T) {
	t.Parallel()

	stored := "sora-2"
	channel := &model.Channel{
		Type:         channeltype.OpenAI,
		Models:       "sora-2,gpt-4o-mini",
		TestingModel: &stored,
	}

	modelName, clearStored, err := chooseChannelTestModel(channel, "")
	require.NoError(t, err)
	require.True(t, clearStored)
	require.Equal(t, "gpt-4o-mini", modelName)
}

// TestChooseChannelTestModelClearsStoredCloudflareEmbedding verifies stale embedding selections are repaired automatically.
func TestChooseChannelTestModelClearsStoredCloudflareEmbedding(t *testing.T) {
	t.Parallel()

	stored := "@cf/baai/bge-m3"
	channel := &model.Channel{
		Type:         channeltype.Cloudflare,
		Models:       "@cf/baai/bge-m3,@cf/qwen/qwen3-30b-a3b-fp8",
		TestingModel: &stored,
	}

	modelName, clearStored, err := chooseChannelTestModel(channel, "")
	require.NoError(t, err)
	require.True(t, clearStored)
	require.Equal(t, "@cf/qwen/qwen3-30b-a3b-fp8", modelName)
}

// TestChooseChannelTestModelRejectsExplicitNonChatModel verifies explicit tests fail fast for non-chat models.
func TestChooseChannelTestModelRejectsExplicitNonChatModel(t *testing.T) {
	t.Parallel()

	channel := &model.Channel{
		Type:   channeltype.Cloudflare,
		Models: "@cf/baai/bge-m3,@cf/qwen/qwen3-30b-a3b-fp8",
	}

	modelName, clearStored, err := chooseChannelTestModel(channel, "@cf/baai/bge-m3")
	require.Error(t, err)
	require.False(t, clearStored)
	require.Empty(t, modelName)
	require.Contains(t, err.Error(), "does not support both text input and text output through Chat Completions")
}

// TestChooseChannelTestModelRejectsChannelWithoutChatEndpoint verifies all probes use the Chat Completions endpoint contract.
func TestChooseChannelTestModelRejectsChannelWithoutChatEndpoint(t *testing.T) {
	t.Parallel()

	channel := &model.Channel{
		Type:   channeltype.OpenAICompatible,
		Models: "gpt-4o-mini",
		Config: `{"supported_endpoints":["embeddings"]}`,
	}

	modelName, clearStored, err := chooseChannelTestModel(channel, "")
	require.Error(t, err)
	require.False(t, clearStored)
	require.Empty(t, modelName)
	require.Contains(t, err.Error(), "does not support the Chat Completions endpoint")
}

// TestChooseChannelTestModelAllowsUnknownCustomChatModel preserves custom-provider compatibility when metadata is unavailable.
func TestChooseChannelTestModelAllowsUnknownCustomChatModel(t *testing.T) {
	t.Parallel()

	channel := &model.Channel{
		Type:   channeltype.OpenAICompatible,
		Models: "vendor/new-chat-model",
	}

	modelName, clearStored, err := chooseChannelTestModel(channel, "")
	require.NoError(t, err)
	require.False(t, clearStored)
	require.Equal(t, "vendor/new-chat-model", modelName)
}

// TestChannelTestModelSupportsTextUsesMapping verifies aliases inherit task checks from mapped upstream models.
func TestChannelTestModelSupportsTextUsesMapping(t *testing.T) {
	t.Parallel()

	mapping := `{"embedding-alias":"@cf/baai/bge-m3","chat-alias":"@cf/qwen/qwen3-30b-a3b-fp8"}`
	channel := &model.Channel{
		Type:         channeltype.Cloudflare,
		Models:       "embedding-alias,chat-alias",
		ModelMapping: &mapping,
	}

	require.False(t, channelTestModelSupportsText(channel, "embedding-alias"))
	require.True(t, channelTestModelSupportsText(channel, "chat-alias"))
}
