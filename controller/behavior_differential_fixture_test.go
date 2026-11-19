package controller

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/model"
)

// Fixed fixture values. Timestamps are pinned (never helper.GetTimestamp()) so a
// captured baseline is reproducible; values are deliberately distinct from every
// quota/count in the fixture so exact-value normalization can never collide.
const (
	behaviorDiffCreatedAtMilli = int64(1730000000111)
	behaviorDiffUpdatedAtMilli = int64(1730000000222)
	behaviorDiffCreatedTime    = int64(1720000001)
	behaviorDiffAccessedTime   = int64(1720000002)
	behaviorDiffExpiredTime    = int64(1990000003)
	behaviorDiffRedeemedTime   = int64(1720000004)
	behaviorDiffTestTime       = int64(1720000005)
	behaviorDiffBalanceTime    = int64(1720000006)
	behaviorDiffLogCreatedAt   = int64(1720000007)
	behaviorDiffRequestID      = "behaviordiff-request-id"
)

// behaviorDiffFixture holds the seeded rows every case runs against. Each case
// gets its own freshly-seeded database, so mutating cases (update/add/consume)
// cannot leak into the list captures.
type behaviorDiffFixture struct {
	root       *model.User
	user       *model.User
	other      *model.User
	token      *model.Token
	otherToken *model.Token
	channel    *model.Channel
	redemption *model.Redemption
	log        *model.Log
	otherLog   *model.Log
	norm       *behaviorDiffNormalizer
	// requestStart/requestEnd bracket the wall clock across the handler call.
	// They exist only for the handful of response fields that are derived from
	// helper.GetTimestamp() and are *not* recoverable from any row afterwards
	// (see the ConsumeToken expires_at case), so the normalizer can enumerate the
	// small, exact candidate set instead of pattern-matching "numbers that look
	// like a timestamp".
	requestStart int64
	requestEnd   int64
}

// behaviorDiffSetup installs an isolated in-memory database plus deterministic
// global configuration, seeds fully-populated rows for every entity in the
// refactor's blast radius, and returns the fixture with a normalizer primed from
// the values the fixture actually generated.
// Parameters:
//   - t: active test handle; cleanup is registered on it.
//
// Return values:
//   - *behaviorDiffFixture: seeded rows and the fixture-derived normalizer.
func behaviorDiffSetup(t *testing.T) *behaviorDiffFixture {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// In-memory sqlite is per-connection; pin to one so the schema and rows stay
	// visible, and so any concurrent access inside a handler is serialized.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Channel{}, &model.ChannelGroup{},
		&model.Token{},
		&model.TokenTransaction{},
		&model.Redemption{},
		&model.Log{},
		&model.Ability{},
	))

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalUsingSQLite := common.UsingSQLite.Load()
	originalRedis := common.IsRedisEnabled()
	originalTokenKeyPrefix := config.TokenKeyPrefix
	originalDefaultItems := config.DefaultItemsPerPage
	originalMaxItems := config.MaxItemsPerPage
	originalRegister := config.RegisterEnabled
	originalPasswordRegister := config.PasswordRegisterEnabled
	originalEmailVerification := config.EmailVerificationEnabled
	originalPasswordLogin := config.PasswordLoginEnabled
	originalTurnstile := config.TurnstileCheckEnabled
	originalBillingDefault := config.ExternalBillingDefaultTimeoutSec
	originalBillingMax := config.ExternalBillingMaxTimeoutSec

	model.DB = db
	model.LOG_DB = db
	common.UsingSQLite.Store(true)
	common.SetRedisEnabled(false)
	config.TokenKeyPrefix = "sk-"
	config.DefaultItemsPerPage = 10
	config.MaxItemsPerPage = 100
	config.RegisterEnabled = true
	config.PasswordRegisterEnabled = true
	config.EmailVerificationEnabled = false
	config.PasswordLoginEnabled = true
	config.TurnstileCheckEnabled = false
	config.ExternalBillingDefaultTimeoutSec = 600
	config.ExternalBillingMaxTimeoutSec = 3600

	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.UsingSQLite.Store(originalUsingSQLite)
		common.SetRedisEnabled(originalRedis)
		config.TokenKeyPrefix = originalTokenKeyPrefix
		config.DefaultItemsPerPage = originalDefaultItems
		config.MaxItemsPerPage = originalMaxItems
		config.RegisterEnabled = originalRegister
		config.PasswordRegisterEnabled = originalPasswordRegister
		config.EmailVerificationEnabled = originalEmailVerification
		config.PasswordLoginEnabled = originalPasswordLogin
		config.TurnstileCheckEnabled = originalTurnstile
		config.ExternalBillingDefaultTimeoutSec = originalBillingDefault
		config.ExternalBillingMaxTimeoutSec = originalBillingMax
		_ = sqlDB.Close()
	})

	rootPassword, err := common.Password2Hash("rootpass123")
	require.NoError(t, err)
	userPassword, err := common.Password2Hash("userpass123")
	require.NoError(t, err)
	otherPassword, err := common.Password2Hash("otherpass123")
	require.NoError(t, err)

	root := &model.User{
		Username:         "behaviordiff-root",
		Password:         rootPassword,
		DisplayName:      "Behavior Diff Root",
		Role:             model.RoleRootUser,
		Status:           model.UserStatusEnabled,
		Email:            "root@behaviordiff.test",
		GitHubId:         "gh-root",
		WeChatId:         "wx-root",
		LarkId:           "lark-root",
		OidcId:           "oidc-root",
		AccessToken:      "behaviordiff-root-access-tok",
		Quota:            910001,
		UsedQuota:        810001,
		RequestCount:     71,
		Group:            "roots",
		AffCode:          "AFFROOT",
		MCPToolBlacklist: model.JSONStringSlice{"root_blocked_tool"},
		Metadata:         model.UserMetadata{},
		CreatedAt:        behaviorDiffCreatedAtMilli,
		UpdatedAt:        behaviorDiffUpdatedAtMilli,
	}
	require.NoError(t, model.DB.Create(root).Error)
	require.NotEmpty(t, root.UUID)

	user := &model.User{
		Username:         "behaviordiff-user",
		Password:         userPassword,
		DisplayName:      "Behavior Diff User",
		Role:             model.RoleCommonUser,
		Status:           model.UserStatusEnabled,
		Email:            "user@behaviordiff.test",
		GitHubId:         "gh-user",
		WeChatId:         "wx-user",
		LarkId:           "lark-user",
		OidcId:           "oidc-user",
		AccessToken:      "behaviordiff-user-access-tok",
		Quota:            920002,
		UsedQuota:        820002,
		RequestCount:     72,
		Group:            "default",
		AffCode:          "AFFUSER",
		InviterId:        root.Id,
		InviterUUID:      &root.UUID,
		MCPToolBlacklist: model.JSONStringSlice{"user_blocked_tool_a", "user_blocked_tool_b"},
		Metadata:         model.UserMetadata{},
		CreatedAt:        behaviorDiffCreatedAtMilli,
		UpdatedAt:        behaviorDiffUpdatedAtMilli,
	}
	require.NoError(t, model.DB.Create(user).Error)
	require.NotEmpty(t, user.UUID)

	// "other" carries a TOTP secret and a locked-password flag so the list
	// captures prove those secrets never reach the wire and that Metadata is
	// serialized with a non-zero value.
	other := &model.User{
		Username:         "behaviordiff-other",
		Password:         otherPassword,
		DisplayName:      "Behavior Diff Other",
		Role:             model.RoleAdminUser,
		Status:           model.UserStatusEnabled,
		Email:            "other@behaviordiff.test",
		GitHubId:         "gh-other",
		WeChatId:         "wx-other",
		LarkId:           "lark-other",
		OidcId:           "oidc-other",
		AccessToken:      "behaviordiff-other-access-to",
		TotpSecret:       "BEHAVIORDIFFOTHERTOTPSECRET",
		Quota:            930003,
		UsedQuota:        830003,
		RequestCount:     73,
		Group:            "vip",
		AffCode:          "AFFOTHER",
		InviterId:        root.Id,
		InviterUUID:      &root.UUID,
		MCPToolBlacklist: model.JSONStringSlice{"other_blocked_tool"},
		Metadata:         model.UserMetadata{PasswordLocked: true},
		CreatedAt:        behaviorDiffCreatedAtMilli,
		UpdatedAt:        behaviorDiffUpdatedAtMilli,
	}
	require.NoError(t, model.DB.Create(other).Error)
	require.NotEmpty(t, other.UUID)

	channelWeight := uint(9)
	channelPriority := int64(6)
	channelRateLimit := 55
	channelBaseURL := "https://upstream.behaviordiff.test"
	channelOther := "behaviordiff-other-field"
	channelHiddenModels := "gpt-3.5-turbo"
	channelModelConfigs := `{"gpt-4o":{"ratio":2.5,"completion_ratio":3}}`
	channelModelMapping := `{"gpt-4o-alias":"gpt-4o"}`
	channelSystemPrompt := "behavior diff system prompt"
	channelTestingModel := "gpt-4o"
	channelModelRatio := `{"gpt-4o":2.5}`
	channelCompletionRatio := `{"gpt-4o":3}`
	channelArnMap := `{"gpt-4o":"arn:aws:bedrock:us-east-1:1:inference-profile/x"}`
	channel := &model.Channel{
		Type:               1,
		Key:                "behaviordiff-channel-key",
		Status:             model.ChannelStatusEnabled,
		Name:               "behaviordiff-channel",
		Weight:             &channelWeight,
		CreatedTime:        behaviorDiffCreatedTime,
		TestTime:           behaviorDiffTestTime,
		ResponseTime:       321,
		BaseURL:            &channelBaseURL,
		Other:              &channelOther,
		Balance:            12.5,
		BalanceUpdatedTime: behaviorDiffBalanceTime,
		Models:             "gpt-4o,gpt-3.5-turbo",
		HiddenModels:       &channelHiddenModels,
		ModelConfigs:       &channelModelConfigs,
		Group:              "default,vip",
		UsedQuota:          940004,
		ModelMapping:       &channelModelMapping,
		Priority:           &channelPriority,
		// Config carries a tooling block so the tooling splice in
		// buildChannelResponsePayload is exercised by the capture.
		Config:                 `{"region":"us-east-1","tooling":{"whitelist":["search"]}}`,
		SystemPrompt:           &channelSystemPrompt,
		RateLimit:              &channelRateLimit,
		TestingModel:           &channelTestingModel,
		ModelRatio:             &channelModelRatio,
		CompletionRatio:        &channelCompletionRatio,
		CreatedAt:              behaviorDiffCreatedAtMilli,
		UpdatedAt:              behaviorDiffUpdatedAtMilli,
		InferenceProfileArnMap: &channelArnMap,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NotEmpty(t, channel.UUID)

	tokenModels := "gpt-4o,gpt-3.5-turbo"
	tokenSubnet := "192.168.0.0/24"
	token := &model.Token{
		UserId:         user.Id,
		UserUUID:       &user.UUID,
		Key:            strings.Repeat("a", 48),
		Status:         model.TokenStatusEnabled,
		Name:           "behaviordiff-token",
		CreatedTime:    behaviorDiffCreatedTime,
		AccessedTime:   behaviorDiffAccessedTime,
		ExpiredTime:    behaviorDiffExpiredTime,
		RemainQuota:    950005,
		UnlimitedQuota: false,
		UsedQuota:      850005,
		CreatedAt:      behaviorDiffCreatedAtMilli,
		UpdatedAt:      behaviorDiffUpdatedAtMilli,
		Models:         &tokenModels,
		Subnet:         &tokenSubnet,
	}
	require.NoError(t, model.DB.Create(token).Error)
	require.NotEmpty(t, token.UUID)

	otherTokenModels := "gpt-4o"
	otherTokenSubnet := "10.0.0.0/8"
	otherToken := &model.Token{
		UserId:         other.Id,
		UserUUID:       &other.UUID,
		Key:            strings.Repeat("b", 48),
		Status:         model.TokenStatusEnabled,
		Name:           "behaviordiff-other-token",
		CreatedTime:    behaviorDiffCreatedTime,
		AccessedTime:   behaviorDiffAccessedTime,
		ExpiredTime:    behaviorDiffExpiredTime,
		RemainQuota:    960006,
		UnlimitedQuota: true,
		UsedQuota:      860006,
		CreatedAt:      behaviorDiffCreatedAtMilli,
		UpdatedAt:      behaviorDiffUpdatedAtMilli,
		Models:         &otherTokenModels,
		Subnet:         &otherTokenSubnet,
	}
	require.NoError(t, model.DB.Create(otherToken).Error)
	require.NotEmpty(t, otherToken.UUID)

	redemption := &model.Redemption{
		UserId:       user.Id,
		UserUUID:     &user.UUID,
		Key:          "behaviordiff-redemption-key",
		Status:       model.RedemptionCodeStatusEnabled,
		Name:         "behaviordiff-redeem",
		Quota:        970007,
		CreatedTime:  behaviorDiffCreatedTime,
		RedeemedTime: behaviorDiffRedeemedTime,
		CreatedAt:    behaviorDiffCreatedAtMilli,
		UpdatedAt:    behaviorDiffUpdatedAtMilli,
	}
	require.NoError(t, model.DB.Create(redemption).Error)
	require.NotEmpty(t, redemption.UUID)

	logRow := &model.Log{
		UserId:             user.Id,
		UserUUID:           &user.UUID,
		CreatedAt:          behaviorDiffLogCreatedAt,
		Type:               model.LogTypeConsume,
		Content:            "behaviordiff consume log",
		Username:           user.Username,
		TokenName:          token.Name,
		TokenUUID:          &token.UUID,
		ModelName:          "gpt-4o",
		OriginModelName:    "gpt-4o-alias",
		Quota:              1301,
		PromptTokens:       1302,
		CompletionTokens:   1303,
		ChannelId:          channel.Id,
		ChannelUUID:        &channel.UUID,
		RequestId:          "behaviordiff-log-request",
		TraceId:            "behaviordiff-log-trace",
		UpdatedAt:          behaviorDiffUpdatedAtMilli,
		ElapsedTime:        1304,
		IsStream:           true,
		SystemPromptReset:  true,
		CachedPromptTokens: 1305,
		Metadata:           model.LogMetadata{"cache_write_tokens": json.Number("7")},
	}
	require.NoError(t, model.DB.Create(logRow).Error)
	require.NotEmpty(t, logRow.UUID)

	otherLog := &model.Log{
		UserId:           other.Id,
		UserUUID:         &other.UUID,
		CreatedAt:        behaviorDiffLogCreatedAt,
		Type:             model.LogTypeTopup,
		Content:          "behaviordiff topup log",
		Username:         other.Username,
		TokenName:        otherToken.Name,
		TokenUUID:        &otherToken.UUID,
		ModelName:        "gpt-3.5-turbo",
		OriginModelName:  "gpt-3.5-turbo",
		Quota:            1401,
		PromptTokens:     1402,
		CompletionTokens: 1403,
		ChannelId:        channel.Id,
		ChannelUUID:      &channel.UUID,
		RequestId:        "behaviordiff-other-log-request",
		TraceId:          "behaviordiff-other-log-trace",
		UpdatedAt:        behaviorDiffUpdatedAtMilli,
		ElapsedTime:      1404,
	}
	require.NoError(t, model.DB.Create(otherLog).Error)
	require.NotEmpty(t, otherLog.UUID)

	norm := &behaviorDiffNormalizer{}
	norm.addString(root.UUID, "<root-user-uuid>")
	norm.addString(user.UUID, "<user-uuid>")
	norm.addString(other.UUID, "<other-user-uuid>")
	norm.addString(channel.UUID, "<channel-uuid>")
	norm.addString(token.UUID, "<token-uuid>")
	norm.addString(otherToken.UUID, "<other-token-uuid>")
	norm.addString(redemption.UUID, "<redemption-uuid>")
	norm.addString(logRow.UUID, "<log-uuid>")
	norm.addString(otherLog.UUID, "<other-log-uuid>")

	return &behaviorDiffFixture{
		root:       root,
		user:       user,
		other:      other,
		token:      token,
		otherToken: otherToken,
		channel:    channel,
		redemption: redemption,
		log:        logRow,
		otherLog:   otherLog,
		norm:       norm,
	}
}
