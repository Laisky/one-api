package controller

import (
	"context"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/channeltype"
)

// setupChannelSweepTestEnvironment installs a clean in-memory database carrying the
// tables the channel sweep touches, and restores every global it swaps.
// Parameters: t is the test handle.
// Returns: no values; cleanup is registered with t.
func setupChannelSweepTestEnvironment(t *testing.T) {
	t.Helper()

	// A file under t.TempDir() rather than a shared-cache ":memory:" DSN: the
	// shared cache is process-wide, so two tests using it would silently observe
	// each other's channels and rows.
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/sweep.db"), &gorm.Config{Logger: glogger.Discard})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.Log{}))

	// main.go does this at boot. Without it the cross-provider catalogue is empty,
	// so every model would classify as unknown format and the suite would be
	// measuring a world the server never runs in.
	relay.InitializeGlobalPricing()

	originalDB, originalLogDB := model.DB, model.LOG_DB
	originalUsingSQLite := common.UsingSQLite.Load()
	originalAutoDisable := config.AutomaticDisableChannelEnabled
	originalRootEmail := config.RootUserEmail
	originalInterval := config.RequestInterval
	originalThreshold := config.ChannelDisableThreshold

	model.DB, model.LOG_DB = db, db
	common.UsingSQLite.Store(true)
	// The sweep only reaches the disable path when auto-disable is on, which is
	// precisely the configuration issue #400 was reported under.
	config.AutomaticDisableChannelEnabled = true
	// Non-empty so the sweep does not query the User table for the root address.
	config.RootUserEmail = "root@example.com"
	config.RequestInterval = 0
	config.ChannelDisableThreshold = 0

	t.Cleanup(func() {
		model.DB, model.LOG_DB = originalDB, originalLogDB
		common.UsingSQLite.Store(originalUsingSQLite)
		config.AutomaticDisableChannelEnabled = originalAutoDisable
		config.RootUserEmail = originalRootEmail
		config.RequestInterval = originalInterval
		config.ChannelDisableThreshold = originalThreshold
		sqlDB.Close()
	})
}

// runChannelSweep runs the bulk channel test and waits for its background goroutine.
// Parameters: t is the test handle.
// Returns: no values; fails the test if the sweep does not finish in time.
func runChannelSweep(t *testing.T) {
	t.Helper()

	ctx := gmw.SetLogger(context.Background(), logger.Logger)
	require.NoError(t, testChannels(ctx, false, "all"))
	require.Eventually(t, func() bool {
		testAllChannelsLock.Lock()
		defer testAllChannelsLock.Unlock()
		return !testAllChannelsRunning
	}, 30*time.Second, 10*time.Millisecond, "channel sweep did not finish")
}

// channelStatus reads a channel's persisted status.
// Parameters: t is the test handle and id identifies the channel.
// Returns: the stored status column.
func channelStatus(t *testing.T, id int) int {
	t.Helper()

	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, "id = ?", id).Error)
	return stored.Status
}

// TestSweepKeepsEmbeddingOnlyChannelEnabled reproduces issue #400: a channel that
// serves only embedding models has no chat surface to probe, and the periodic
// sweep used to treat that unconstructible probe as a failure and auto-disable it.
// Parameters: t is the test handle.
// Returns: no values.
func TestSweepKeepsEmbeddingOnlyChannelEnabled(t *testing.T) {
	setupChannelSweepTestEnvironment(t)

	// Base URL points at a closed port: if the sweep ever builds a probe for this
	// channel the request fails, so the assertion below cannot pass by accident.
	baseURL := "http://127.0.0.1:1"
	byModels := &model.Channel{
		Name:    "embedding-only-by-model-list",
		Type:    channeltype.OpenAICompatible,
		Status:  model.ChannelStatusEnabled,
		Key:     "sk-test",
		BaseURL: &baseURL,
		Models:  "text-embedding-3-small,bge-m3",
	}
	byEndpoints := &model.Channel{
		Name:    "embedding-only-by-endpoint-config",
		Type:    channeltype.OpenAICompatible,
		Status:  model.ChannelStatusEnabled,
		Key:     "sk-test",
		BaseURL: &baseURL,
		Models:  "some-private-embedding-deployment",
		Config:  `{"supported_endpoints":["embeddings"]}`,
	}
	require.NoError(t, model.DB.Create(byModels).Error)
	require.NoError(t, model.DB.Create(byEndpoints).Error)

	runChannelSweep(t)

	require.Equal(t, model.ChannelStatusEnabled, channelStatus(t, byModels.Id),
		"a channel whose every model is an embedding model must be skipped, not disabled")
	require.Equal(t, model.ChannelStatusEnabled, channelStatus(t, byEndpoints.Id),
		"a channel declaring only the embeddings endpoint must be skipped, not disabled")
}

// TestSweepKeepsEndpointlessChannelEnabled covers DeepL, whose channel type
// declares no standard endpoints at all and was therefore disabled on every sweep.
// Parameters: t is the test handle.
// Returns: no values.
func TestSweepKeepsEndpointlessChannelEnabled(t *testing.T) {
	setupChannelSweepTestEnvironment(t)

	baseURL := "http://127.0.0.1:1"
	channel := &model.Channel{
		Name:    "deepl-translation-only",
		Type:    channeltype.DeepL,
		Status:  model.ChannelStatusEnabled,
		Key:     "sk-test",
		BaseURL: &baseURL,
		Models:  "deepl-en",
	}
	require.NoError(t, model.DB.Create(channel).Error)

	runChannelSweep(t)

	require.Equal(t, model.ChannelStatusEnabled, channelStatus(t, channel.Id),
		"a channel type exposing no standard endpoint must be skipped, not disabled")
}

// TestSweepStillDisablesUnreachableChatChannel is the control for the skip tests:
// it proves the harness genuinely exercises the auto-disable path, so the
// assertions above are not passing vacuously.
// Parameters: t is the test handle.
// Returns: no values.
func TestSweepStillDisablesUnreachableChatChannel(t *testing.T) {
	setupChannelSweepTestEnvironment(t)

	baseURL := "http://127.0.0.1:1"
	channel := &model.Channel{
		Name:    "unreachable-chat-channel",
		Type:    channeltype.OpenAICompatible,
		Status:  model.ChannelStatusEnabled,
		Key:     "sk-test",
		BaseURL: &baseURL,
		Models:  "gpt-4o-mini",
	}
	require.NoError(t, model.DB.Create(channel).Error)

	runChannelSweep(t)

	require.Equal(t, model.ChannelStatusAutoDisabled, channelStatus(t, channel.Id),
		"a chat-capable channel that cannot be reached must still be disabled")
}

// TestChannelTestNotApplicableIsDistinctFromFailure verifies only unprobeable
// channels are skipped, and that genuine operator mistakes remain hard errors.
// Parameters: t is the test handle.
// Returns: no values.
func TestChannelTestNotApplicableIsDistinctFromFailure(t *testing.T) {
	t.Parallel()

	channel := &model.Channel{
		Type:   channeltype.OpenAICompatible,
		Models: "gpt-4o-mini",
	}

	_, _, err := chooseChannelTestModel(channel, "model-not-on-this-channel")
	require.Error(t, err)
	require.False(t, isChannelTestNotApplicable(err),
		"an explicitly requested unsupported model is operator error, not a skip")

	_, _, err = chooseChannelTestModel(nil, "")
	require.Error(t, err)
	require.False(t, isChannelTestNotApplicable(err),
		"a nil channel is a programming error, not a skip")

	require.False(t, isChannelTestNotApplicable(nil))
}

// TestChannelChatSurfaceEligibility verifies chat-probe eligibility follows the
// endpoints a channel actually exposes, with administrator config winning over
// the channel type's defaults.
// Parameters: t is the test handle.
// Returns: no values.
func TestChannelChatSurfaceEligibility(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		channel *model.Channel
		want    bool
	}{
		{"channel type default exposes chat completions",
			&model.Channel{Type: channeltype.OpenAICompatible}, true},
		{"claude messages alone is still a chat surface",
			&model.Channel{Type: channeltype.OpenAICompatible,
				Config: `{"supported_endpoints":["claude_messages"]}`}, true},
		{"response api alone is still a chat surface",
			&model.Channel{Type: channeltype.OpenAICompatible,
				Config: `{"supported_endpoints":["response_api"]}`}, true},
		{"embeddings and rerank are not chat surfaces",
			&model.Channel{Type: channeltype.OpenAICompatible,
				Config: `{"supported_endpoints":["embeddings","rerank"]}`}, false},
		{"a channel type declaring no endpoints is ineligible",
			&model.Channel{Type: channeltype.DeepL}, false},
		{"nil channel is ineligible",
			nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, channelHasChatSurface(tc.channel))
		})
	}
}

// TestChannelTextTestModelsCoversClaudeMessagesOnlyChannel verifies the admin
// test-model selector is populated for a channel whose only chat surface is the
// Claude Messages API.
// Parameters: t is the test handle.
// Returns: no values.
func TestChannelTextTestModelsCoversClaudeMessagesOnlyChannel(t *testing.T) {
	t.Parallel()

	models := channelTextTestModels(&model.Channel{
		Type:   channeltype.OpenAICompatible,
		Models: "claude-sonnet-4-20250514,text-embedding-3-small",
		Config: `{"supported_endpoints":["claude_messages"]}`,
	})
	require.Equal(t, []string{"claude-sonnet-4-20250514"}, models)
}

// TestModelFormatKnownAcrossChannelCatalogues pins the cross-provider fallback.
//
// Which API format serves a model is a property of the model, not of the channel
// carrying it, and a channel type's own catalogue can legitimately lack it:
// GeminiOpenAICompatible resolves to the OpenAI adaptor's table, which lists no
// Gemini models. Without the global fallback every Gemini model on such a channel
// classifies as unknown format and the channel is skipped entirely.
// Parameters: t is the test handle.
// Returns: no values.
func TestModelFormatKnownAcrossChannelCatalogues(t *testing.T) {
	relay.InitializeGlobalPricing()

	cases := []struct {
		name        string
		channelType int
		model       string
	}{
		{"gemini model on a gemini openai-compatible channel", channeltype.GeminiOpenAICompatible, "gemini-2.0-flash"},
		{"gemini model on its native channel", channeltype.Gemini, "gemini-2.0-flash"},
		{"openai model on an openai-compatible channel", channeltype.OpenAICompatible, "gpt-4o-mini"},
		{"claude model on a claude-compatible channel", channeltype.ClaudeCompatible, "claude-sonnet-4-20250514"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			channel := &model.Channel{Type: tc.channelType, Models: tc.model}
			require.Equal(t, []string{tc.model}, channelAutoTestModels(channel),
				"a catalogued chat model must be auto-selectable regardless of channel type")
		})
	}

	// The fallback must not launder a non-chat model into scope.
	require.Empty(t, channelAutoTestModels(&model.Channel{
		Type: channeltype.GeminiOpenAICompatible, Models: "text-embedding-3-small",
	}))
}

// TestChannelTestingSkipSentinel covers the SKIP opt-out at the selection layer,
// including the manual-test escape hatch.
// Parameters: t is the test handle.
// Returns: no values.
func TestChannelTestingSkipSentinel(t *testing.T) {
	t.Parallel()

	skip := model.ChannelTestingModelSkip
	newChannel := func() *model.Channel {
		value := skip
		return &model.Channel{
			Type:         channeltype.OpenAICompatible,
			Models:       "gpt-4o-mini,gpt-4o",
			TestingModel: &value,
		}
	}

	t.Run("automatic selection reports skipped", func(t *testing.T) {
		t.Parallel()
		modelName, clearStored, err := chooseChannelTestModel(newChannel(), "")
		require.Error(t, err)
		require.Empty(t, modelName)
		require.True(t, isChannelTestNotApplicable(err),
			"SKIP is an opt-out, never a failure that could disable the channel")
		require.False(t, clearStored,
			"the SKIP sentinel must not be cleared as an invalid model name")
		require.Contains(t, err.Error(), "SKIP")
	})

	t.Run("an explicitly named model still forces a one-off test", func(t *testing.T) {
		t.Parallel()
		modelName, clearStored, err := chooseChannelTestModel(newChannel(), "gpt-4o")
		require.NoError(t, err)
		require.False(t, clearStored)
		require.Equal(t, "gpt-4o", modelName)
	})

	t.Run("the sentinel is recognised case-insensitively", func(t *testing.T) {
		t.Parallel()
		upper := "__SKIP__"
		channel := &model.Channel{
			Type:         channeltype.OpenAICompatible,
			Models:       "gpt-4o-mini",
			TestingModel: &upper,
		}
		_, _, err := chooseChannelTestModel(channel, "")
		require.True(t, isChannelTestNotApplicable(err))
	})

	t.Run("the sentinel is not offered as a testable model", func(t *testing.T) {
		t.Parallel()
		require.NotContains(t, channelTextTestModels(newChannel()), model.ChannelTestingModelSkip)
	})
}
