package controller

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/stretchr/testify/require"
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

// TestModelConfigSupportsTextTest verifies that channel tests only accept text-in/text-out model metadata.
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
		OutputModalities: []string{},
		Embedding:        &adaptor.EmbeddingPricingConfig{TextTokenRatio: 1},
	}, true))
	require.True(t, modelConfigSupportsTextTest(adaptor.ModelConfig{}, false))
}

// TestChooseChannelTestModelFiltersNonTextModels verifies fallback selection skips models that cannot output text.
func TestChooseChannelTestModelFiltersNonTextModels(t *testing.T) {
	t.Parallel()

	channel := &model.Channel{
		Type:   channeltype.OpenAI,
		Models: "sora-2,gpt-4o-mini,text-embedding-3-small",
	}

	modelName, clearStored, err := chooseChannelTestModel(channel, "")
	require.NoError(t, err)
	require.False(t, clearStored)
	require.Equal(t, "gpt-4o-mini", modelName)
}

// TestBuildChannelListResponseFiltersNonTextModels verifies channel rows expose only text-compatible test models.
func TestBuildChannelListResponseFiltersNonTextModels(t *testing.T) {
	t.Parallel()

	channels := []*model.Channel{
		{
			Type:   channeltype.OpenAI,
			Models: "sora-2,gpt-4o-mini,text-embedding-3-small",
		},
	}

	rows := buildChannelListResponse(channels)
	require.Len(t, rows, 1)
	require.Equal(t, []string{"gpt-4o-mini"}, rows[0].TestModels)
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

// TestChooseChannelTestModelClearsStoredNonTextModel verifies stored testing models must support text output.
func TestChooseChannelTestModelClearsStoredNonTextModel(t *testing.T) {
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

// TestChooseChannelTestModelRejectsExplicitNonTextModel verifies explicit tests fail fast for non-text models.
func TestChooseChannelTestModelRejectsExplicitNonTextModel(t *testing.T) {
	t.Parallel()

	channel := &model.Channel{
		Type:   channeltype.OpenAI,
		Models: "sora-2,gpt-4o-mini",
	}

	modelName, clearStored, err := chooseChannelTestModel(channel, "sora-2")
	require.Error(t, err)
	require.False(t, clearStored)
	require.Empty(t, modelName)
	require.Contains(t, err.Error(), "does not support both text input and text output")
}

// TestChannelTestModelSupportsTextUsesMapping verifies aliases inherit capability checks from mapped upstream models.
func TestChannelTestModelSupportsTextUsesMapping(t *testing.T) {
	t.Parallel()

	mapping := `{"video-alias":"sora-2","chat-alias":"gpt-4o-mini"}`
	channel := &model.Channel{
		Type:         channeltype.OpenAI,
		Models:       "video-alias,chat-alias",
		ModelMapping: &mapping,
	}

	require.False(t, channelTestModelSupportsText(channel, "video-alias"))
	require.True(t, channelTestModelSupportsText(channel, "chat-alias"))
}
