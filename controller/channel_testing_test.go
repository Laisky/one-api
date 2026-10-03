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

// TestModelConfigUsesChatFormat verifies metadata classification by API format.
func TestModelConfigUsesChatFormat(t *testing.T) {
	t.Parallel()

	require.True(t, modelConfigUsesChatFormat(adaptor.ModelConfig{
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"text"},
	}))
	require.False(t, modelConfigUsesChatFormat(adaptor.ModelConfig{
		InputModalities:  []string{"text"},
		OutputModalities: []string{"video"},
	}))
	require.False(t, modelConfigUsesChatFormat(adaptor.ModelConfig{
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Embedding:        &adaptor.EmbeddingPricingConfig{TextTokenRatio: 1},
	}))
	require.False(t, modelConfigUsesChatFormat(adaptor.ModelConfig{
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "A multilingual text embeddings model.",
	}))
	require.False(t, modelConfigUsesChatFormat(adaptor.ModelConfig{
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		Description:      "A sentiment classifier for text classification.",
	}))
}

// TestModelNameLooksNonChatFormat verifies the name markers denote non-chat API
// formats, and do not reject specialized models that are still served over chat.
func TestModelNameLooksNonChatFormat(t *testing.T) {
	t.Parallel()

	// Non-chat API formats.
	require.True(t, modelNameLooksNonChatFormat("@cf/baai/bge-m3"))
	require.True(t, modelNameLooksNonChatFormat("gpt-3.5-turbo-instruct"))
	require.True(t, modelNameLooksNonChatFormat("text-embedding-3-small"))
	require.True(t, modelNameLooksNonChatFormat("omni-moderation-2024-09-26"))

	// Ordinary chat models.
	require.False(t, modelNameLooksNonChatFormat("amazon.nova-pro-v1:0"))
	require.False(t, modelNameLooksNonChatFormat("gpt-4o-mini"))

	// Specialized, but served over Chat Completions: a vision-language model, an
	// OCR-tuned VLM, a video-understanding chat model and a safety classifier all
	// take text in and return text, so the probe can use them.
	require.False(t, modelNameLooksNonChatFormat("@cf/moondream/moondream3.1-9B-A2B"))
	require.False(t, modelNameLooksNonChatFormat("qwen-vl-ocr"))
	require.False(t, modelNameLooksNonChatFormat("hunyuan-turbos-vision-video"))
	require.False(t, modelNameLooksNonChatFormat("meta-llama/Llama-Guard-4-12B"))
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
	require.Contains(t, err.Error(), "is not served through a chat API format")
}

// TestChooseChannelTestModelRejectsChannelWithoutChatEndpoint verifies a channel
// exposing no chat-capable surface is reported as not-applicable rather than failed.
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
	require.Contains(t, err.Error(), "no chat-capable endpoint")
	require.True(t, isChannelTestNotApplicable(err),
		"an unprobeable channel must be skipped, never routed to the auto-disable path")
}

// TestChooseChannelTestModelExcludesUnknownFormatByDefault verifies a model whose
// API format cannot be determined is left out of automatic selection, and that the
// exclusion is a skip rather than a failure.
//
// Guessing "chat" for an unrecognised name is what sends a Chat Completions request
// to a self-hosted embeddings deployment, so the default has to be exclusion.
func TestChooseChannelTestModelExcludesUnknownFormatByDefault(t *testing.T) {
	t.Parallel()

	channel := &model.Channel{
		Type:   channeltype.OpenAICompatible,
		Models: "vendor/new-chat-model",
	}

	modelName, clearStored, err := chooseChannelTestModel(channel, "")
	require.Error(t, err)
	require.False(t, clearStored)
	require.Empty(t, modelName)
	require.True(t, isChannelTestNotApplicable(err),
		"an unknown-format model must be skipped, never routed to the auto-disable path")
	require.Contains(t, err.Error(), "unknown format")
}

// TestChooseChannelTestModelHonoursExplicitUnknownModel verifies the administrator
// override: naming a model deliberately opts it back into the probe even when the
// gateway holds no metadata describing its API format.
func TestChooseChannelTestModelHonoursExplicitUnknownModel(t *testing.T) {
	t.Parallel()

	stored := "vendor/new-chat-model"
	channel := &model.Channel{
		Type:         channeltype.OpenAICompatible,
		Models:       "vendor/new-chat-model",
		TestingModel: &stored,
	}

	// Explicit via the stored channel testing model.
	modelName, clearStored, err := chooseChannelTestModel(channel, "")
	require.NoError(t, err)
	require.False(t, clearStored)
	require.Equal(t, "vendor/new-chat-model", modelName)

	// Explicit via ?model= on the manual test.
	modelName, clearStored, err = chooseChannelTestModel(
		&model.Channel{Type: channeltype.OpenAICompatible, Models: "vendor/new-chat-model"},
		"vendor/new-chat-model")
	require.NoError(t, err)
	require.False(t, clearStored)
	require.Equal(t, "vendor/new-chat-model", modelName)
}

// TestChannelTestModelCandidatesSplitExplicitFromAutomatic verifies the admin
// selector still offers an unknown-format model (so the override is reachable in the
// UI) while automatic selection leaves it out.
func TestChannelTestModelCandidatesSplitExplicitFromAutomatic(t *testing.T) {
	t.Parallel()

	channel := &model.Channel{
		Type:   channeltype.OpenAICompatible,
		Models: "vendor/new-chat-model,gpt-4o-mini,text-embedding-3-small",
	}

	require.Equal(t, []string{"gpt-4o-mini", "vendor/new-chat-model"}, channelTextTestModels(channel),
		"the selector offers known chat models plus unknown-format ones")
	require.Equal(t, []string{"gpt-4o-mini"}, channelAutoTestModels(channel),
		"automatic selection is limited to models known to use a chat API format")
}

// TestSpecializedChatModelsRemainTestable pins the blocklist fix: a model that is
// specialized (vision-language, OCR-tuned, video-understanding, safety classifier)
// but still served over Chat Completions must stay in the probe's scope.
func TestSpecializedChatModelsRemainTestable(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"@cf/moondream/moondream3.1-9B-A2B",
		"qwen-vl-ocr",
		"hunyuan-turbos-vision-video",
		"meta-llama/Llama-Guard-4-12B",
	} {
		require.False(t, modelNameLooksNonChatFormat(name), name)
	}

	for _, name := range []string{
		"text-embedding-3-small",
		"omni-moderation-2024-09-26",
		"gpt-3.5-turbo-instruct",
		"@cf/ai4bharat/indictrans2-en-indic-1B",
	} {
		require.True(t, modelNameLooksNonChatFormat(name), name)
	}
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

	require.False(t, channelTestModelUsesChatFormat(channel, "embedding-alias", false))
	require.True(t, channelTestModelUsesChatFormat(channel, "chat-alias", false))
}
