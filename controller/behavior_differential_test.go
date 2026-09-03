package controller

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
)

// This file implements the T17/T18 black-box differential harness described in
// docs/proposals/20260714_boundary-response-dtos.md. It drives every handler
// site in the proposal's Appendix A (28 sites: User 6, Token 9, Channel 4,
// Redemption 4, Log 5) plus the documented T18 error cases through the real
// handlers over httptest, and records (status, canonicalized JSON body) into
// controller/testdata/behavior/*.json.
//
// The baselines were captured on the PRE-refactor tree (git 6ebe28a5, which is
// the last commit before this refactor) and are replayed on the POST-refactor
// tree: a zero diff is the mechanical proof that relocating the external JSON
// contract from model-level MarshalJSON methods to boundary dto.*Response
// mappers changed no observable behavior. 78 of 81 cases are such a zero diff;
// the 3 exceptions are recorded in behaviorDiffKnownDeviations below.
//
// The file deliberately references only handler funcs, model entity
// types/constants, common/* helpers, gin, httptest, testify and the stdlib, and
// must never mention dto.*Response, ToResponse or XsToResponses. That constraint
// is what allowed this exact file to be compiled and run inside a pre-refactor
// worktree to generate the baselines in the first place; keeping it means the
// capture can be reproduced from scratch rather than taken on trust.
//
// Caveat when replaying in a pre-refactor worktree: the 3 cases listed in
// behaviorDiffKnownDeviations assert POST-refactor expectations and will fail
// there, correctly reporting that the output matches the pre-refactor baseline.
// The other 78 pass on both trees.
//
// Regenerate the baselines with:
//
//	go test ./controller/ -run TestBehaviorDifferential -count=1 -update-behavior
//
// Regeneration is a contract change and must be reviewed as such. In particular,
// regenerating on the CURRENT tree silently replaces pre-refactor truth with
// whatever the code does today, which would make this harness tautological — to
// re-record an accepted difference, use -update-behavior-deviations instead,
// which never touches the baselines.

// behaviorDiffUpdate regenerates controller/testdata/behavior/*.json instead of
// comparing against them. Pass -update-behavior to enable.
var behaviorDiffUpdate = flag.Bool("update-behavior", false,
	"regenerate the T17 behavior baselines in controller/testdata/behavior instead of comparing against them")

// behaviorDiffUpdateDeviations regenerates ONLY the known-deviation
// expectations in controller/testdata/behavior/deviations. It never touches the
// pre-refactor baselines, so the historical record cannot be lost by running it.
var behaviorDiffUpdateDeviations = flag.Bool("update-behavior-deviations", false,
	"regenerate the post-refactor expectations for cases listed in behaviorDiffKnownDeviations")

const behaviorDiffBaselineDir = "testdata/behavior"

// behaviorDiffDeviationDir holds the POST-refactor expectation for the small,
// explicitly reviewed set of cases whose behavior intentionally changed. The
// pre-refactor bytes stay in behaviorDiffBaselineDir as the historical record,
// so both sides of every accepted deviation remain readable in the tree.
const behaviorDiffDeviationDir = behaviorDiffBaselineDir + "/deviations"

// behaviorDiffKnownDeviations enumerates the cases where post-refactor behavior
// intentionally differs from the pre-refactor baseline, mapped to the reason on
// record. See §10.3 of docs/proposals/20260714_boundary-response-dtos.md.
//
// These are NOT skips. A listed case is still asserted byte-for-byte against its
// recorded post-refactor expectation, and it is additionally required to still
// differ from the pre-refactor baseline — so a stale entry cannot sit here
// quietly masking a case that has since converged back.
//
// Adding an entry here is a deliberate contract change and must be reviewed as
// such: it is the one escape hatch in this harness, and it exists so that an
// accepted deviation is recorded in code rather than hidden by re-baselining.
var behaviorDiffKnownDeviations = map[string]string{
	"user/register_wrong_type_unbound_field":    behaviorDiffUnboundFieldReason,
	"user/create_wrong_type_unbound_field":      behaviorDiffUnboundFieldReason,
	"user/update_self_wrong_type_unbound_field": behaviorDiffUnboundFieldReason,
}

// behaviorDiffUnboundFieldReason documents the single accepted I2 deviation.
const behaviorDiffUnboundFieldReason = "" +
	"Register/CreateUser/UpdateSelf used to decode into model.User, so a type " +
	"mismatch on ANY of its JSON-tagged fields — including fields the handler " +
	"never read — rejected the whole body with 400 'invalid parameter'. The " +
	"narrow request DTOs omit those fields, so encoding/json now discards them " +
	"as unknown and the request succeeds. The old strictness was an accident of " +
	"decoding into the entity, not a designed check. Restoring it exactly would " +
	"require each DTO to mirror model.User's entire pre-refactor tag set, " +
	"including the four secret fields whose tags were deliberately removed for " +
	"G3 — re-creating the coupling this refactor removes. The relaxation only " +
	"loosens validation on fields these handlers ignore: no secret, no id, no " +
	"stored row changes, and no bundled frontend can trigger it (all three " +
	"themes send such keys only with correctly-typed values echoed from the " +
	"server's own encoding). Accepted and documented rather than re-baselined."

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

// behaviorDiffReplacement maps one concrete fixture value to a stable placeholder.
type behaviorDiffReplacement struct {
	value       string
	placeholder string
}

// behaviorDiffNormalizer rewrites run-varying fixture values (generated UUIDs,
// generated keys, wall-clock timestamps) to stable placeholders. Every entry is
// seeded from a value the fixture actually produced — this is normalization *by
// the fixture*, never a blind regex over "things that look like a uuid".
type behaviorDiffNormalizer struct {
	strs []behaviorDiffReplacement
	nums []behaviorDiffReplacement
}

// addString registers a string value whose every occurrence (including inside a
// larger message) is rewritten to placeholder.
// Parameters:
//   - value: the concrete value produced by the fixture.
//   - placeholder: the stable token written to the baseline.
//
// Return values: none.
func (n *behaviorDiffNormalizer) addString(value, placeholder string) {
	if value == "" {
		return
	}
	n.strs = append(n.strs, behaviorDiffReplacement{value: value, placeholder: placeholder})
}

// addNumber registers a numeric value that is rewritten to placeholder on an
// exact match of its JSON text.
// Parameters:
//   - value: the concrete numeric value produced by the fixture.
//   - placeholder: the stable token written to the baseline.
//
// Return values: none.
func (n *behaviorDiffNormalizer) addNumber(value int64, placeholder string) {
	if value == 0 {
		return
	}
	n.nums = append(n.nums, behaviorDiffReplacement{value: fmt.Sprintf("%d", value), placeholder: placeholder})
}

// addChangedNumber registers a numeric value only when the handler actually
// changed it away from the fixture's pinned value. Registering unconditionally
// would let a placeholder mask a field the handler left alone, which would
// silently weaken the capture.
// Parameters:
//   - original: the value the fixture pinned before the request.
//   - current: the value observed after the request.
//   - placeholder: the stable token written to the baseline.
//
// Return values: none.
func (n *behaviorDiffNormalizer) addChangedNumber(original, current int64, placeholder string) {
	if current == original {
		return
	}
	n.addNumber(current, placeholder)
}

// addTimestampWindow registers every second-resolution timestamp the handler
// could have observed, offset by a known constant. Used only where a response
// field is computed as helper.GetTimestamp()+offset and never persisted (the
// ConsumeToken pre-hold expiry is zeroed in the database on confirmation), so
// the exact value cannot be read back from a row. The candidate set is bounded
// by the wall clock measured around this very request.
// Parameters:
//   - f: fixture carrying the measured request window.
//   - offset: constant added to the handler's timestamp.
//   - placeholder: the stable token written to the baseline.
//
// Return values: none.
func (n *behaviorDiffNormalizer) addTimestampWindow(f *behaviorDiffFixture, offset int64, placeholder string) {
	for ts := f.requestStart; ts <= f.requestEnd; ts++ {
		n.addNumber(ts+offset, placeholder)
	}
}

// apply rewrites a decoded JSON tree in place, replacing known fixture values.
// Parameters:
//   - v: decoded JSON value (map, slice, string, json.Number, bool or nil).
//
// Return values:
//   - any: the normalized value.
func (n *behaviorDiffNormalizer) apply(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		for key, val := range typed {
			typed[key] = n.apply(val)
		}
		return typed
	case []any:
		for i, val := range typed {
			typed[i] = n.apply(val)
		}
		return typed
	case string:
		out := canonicalizeGoStructFieldPath(typed)
		for _, r := range n.sortedStrings() {
			out = strings.ReplaceAll(out, r.value, r.placeholder)
		}
		return out
	case json.Number:
		text := typed.String()
		for _, r := range n.nums {
			if text == r.value {
				return r.placeholder
			}
		}
		return typed
	default:
		return v
	}
}

// goStructFieldPathPattern matches the struct path encoding/json embeds in an
// unmarshal error: `Go struct field <path> of type <type>`.
var goStructFieldPathPattern = regexp.MustCompile(`Go struct field ([A-Za-z0-9_.]+) of type `)

// canonicalizeGoStructFieldPath reduces the struct path in an encoding/json
// unmarshal error to its final segment, i.e. the JSON field name.
//
// The full path is a Go-internal detail whose spelling depends on the toolchain:
// for a field promoted from an embedded struct, Go 1.26 renders
// `channelPayload.Channel.type` while Go 1.27 renders `channelPayload.type`.
// Pinning either spelling makes this suite fail on the other toolchain without
// any behavior having changed. The parts a client can actually act on — the
// failing field name and the expected Go type — are preserved, so a real change
// to which field rejects the payload still breaks the baseline.
//
// Parameters:
//   - value: a candidate response string.
//
// Return values:
//   - string: value with any struct path reduced to its final segment.
func canonicalizeGoStructFieldPath(value string) string {
	return goStructFieldPathPattern.ReplaceAllStringFunc(value, func(match string) string {
		groups := goStructFieldPathPattern.FindStringSubmatch(match)
		if len(groups) != 2 {
			return match
		}
		path := groups[1]
		if idx := strings.LastIndex(path, "."); idx >= 0 {
			path = path[idx+1:]
		}
		return "Go struct field " + path + " of type "
	})
}

// sortedStrings returns the string replacements ordered longest-value-first so a
// shorter value can never consume a prefix of a longer one.
// Parameters: none.
//
// Return values:
//   - []behaviorDiffReplacement: replacements in descending value length.
func (n *behaviorDiffNormalizer) sortedStrings() []behaviorDiffReplacement {
	out := make([]behaviorDiffReplacement, len(n.strs))
	copy(out, n.strs)
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].value) > len(out[j].value) })
	return out
}

// behaviorDiffRequest describes one HTTP call against a real handler.
type behaviorDiffRequest struct {
	method   string
	pattern  string
	target   func(f *behaviorDiffFixture) string
	body     func(f *behaviorDiffFixture) string
	prepare  func(c *gin.Context, f *behaviorDiffFixture)
	handler  gin.HandlerFunc
	sessions bool
}

// behaviorDiffCase is a single captured (status, canonical body) observation.
type behaviorDiffCase struct {
	entity  string
	name    string
	site    string
	req     behaviorDiffRequest
	dynamic func(t *testing.T, f *behaviorDiffFixture, n *behaviorDiffNormalizer)
}

// behaviorDiffRecord is the on-disk baseline shape.
type behaviorDiffRecord struct {
	Case   string          `json:"case"`
	Site   string          `json:"site"`
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
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
		&model.Channel{},
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

// behaviorDiffServe runs one request through a real handler on a bare gin engine.
// Parameters:
//   - t: active test handle.
//   - f: seeded fixture the request is issued against.
//   - r: request description (route pattern, target, body, context preparation).
//
// Return values:
//   - *httptest.ResponseRecorder: the recorded handler response.
func behaviorDiffServe(t *testing.T, f *behaviorDiffFixture, r behaviorDiffRequest) *httptest.ResponseRecorder {
	t.Helper()

	engine := gin.New()
	if r.sessions {
		engine.Use(sessions.Sessions("behaviordiff-session", cookie.NewStore([]byte("behaviordiff-secret"))))
	}
	engine.Use(func(c *gin.Context) {
		gmw.SetLogger(c, logger.Logger)
		c.Set(helper.RequestIdKey, behaviorDiffRequestID)
		if r.prepare != nil {
			r.prepare(c, f)
		}
	})
	engine.Handle(r.method, r.pattern, r.handler)

	var reader *strings.Reader
	if r.body != nil {
		reader = strings.NewReader(r.body(f))
	} else {
		reader = strings.NewReader("")
	}
	request := httptest.NewRequest(r.method, r.target(f), reader)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	f.requestStart = helper.GetTimestamp()
	engine.ServeHTTP(recorder, request)
	f.requestEnd = helper.GetTimestamp()
	return recorder
}

// behaviorDiffEncode serializes v deterministically: map keys are sorted by
// encoding/json and HTML escaping is disabled so the "<placeholder>" tokens stay
// readable in the committed baselines.
// Parameters:
//   - t: active test handle.
//   - v: value to encode.
//   - indent: indentation applied to nested values.
//
// Return values:
//   - []byte: encoded JSON without the encoder's trailing newline.
func behaviorDiffEncode(t *testing.T, v any, indent string) []byte {
	t.Helper()
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", indent)
	require.NoError(t, encoder.Encode(v))
	return bytes.TrimRight(buf.Bytes(), "\n")
}

// behaviorDiffCanonicalize decodes a response body, rewrites known fixture values
// to placeholders and re-encodes deterministically (map keys sorted by
// encoding/json) so byte comparison across trees is stable.
// Parameters:
//   - t: active test handle.
//   - raw: raw response bytes.
//   - n: normalizer primed from the fixture.
//
// Return values:
//   - json.RawMessage: canonical body, or nil for an empty response.
func behaviorDiffCanonicalize(t *testing.T, raw []byte, n *behaviorDiffNormalizer) json.RawMessage {
	t.Helper()
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var decoded any
	require.NoError(t, decoder.Decode(&decoded), "response body must be valid JSON: %s", raw)

	return json.RawMessage(behaviorDiffEncode(t, n.apply(decoded), ""))
}

// TestBehaviorDifferential drives every Appendix A handler site plus its T18
// error cases and byte-compares (status, canonical body) against the baselines
// captured on the pre-refactor tree.
// Parameters:
//   - t: active test handle.
//
// Return values: none.
func TestBehaviorDifferential(t *testing.T) {
	gin.SetMode(gin.TestMode)

	if *behaviorDiffUpdate {
		require.NoError(t, os.MkdirAll(behaviorDiffBaselineDir, 0o755))
	}

	for _, tc := range behaviorDiffCases() {
		name := tc.entity + "/" + tc.name
		t.Run(name, func(t *testing.T) {
			fixture := behaviorDiffSetup(t)
			recorder := behaviorDiffServe(t, fixture, tc.req)
			if tc.dynamic != nil {
				tc.dynamic(t, fixture, fixture.norm)
			}

			record := behaviorDiffRecord{
				Case:   name,
				Site:   tc.site,
				Status: recorder.Code,
				Body:   behaviorDiffCanonicalize(t, recorder.Body.Bytes(), fixture.norm),
			}
			got := append(behaviorDiffEncode(t, record, "  "), '\n')

			path := filepath.Join(behaviorDiffBaselineDir, tc.entity+"_"+tc.name+".json")
			if *behaviorDiffUpdate {
				require.NoError(t, os.WriteFile(path, got, 0o644))
				return
			}

			baseline, err := os.ReadFile(path)
			require.NoError(t, err, "missing behavior baseline %s; regenerate with -update-behavior", path)

			reason, deviates := behaviorDiffKnownDeviations[name]
			if !deviates {
				require.Equal(t, string(baseline), string(got),
					"observable behavior changed for %s (%s); this harness exists to catch exactly this", name, tc.site)
				return
			}

			// Accepted deviation: assert against the recorded post-refactor bytes
			// instead of the pre-refactor baseline, but keep asserting.
			devPath := filepath.Join(behaviorDiffDeviationDir, tc.entity+"_"+tc.name+".json")
			if *behaviorDiffUpdateDeviations {
				require.NoError(t, os.MkdirAll(behaviorDiffDeviationDir, 0o755))
				require.NoError(t, os.WriteFile(devPath, got, 0o644))
				return
			}
			want, err := os.ReadFile(devPath)
			require.NoError(t, err,
				"missing deviation expectation %s; regenerate with -update-behavior-deviations", devPath)
			require.Equal(t, string(want), string(got),
				"behavior drifted for known-deviation case %s (%s); reason on record: %s", name, tc.site, reason)
			require.NotEqual(t, string(baseline), string(got),
				"case %s is listed in behaviorDiffKnownDeviations but now matches the pre-refactor "+
					"baseline; delete the entry and its deviation file rather than leave it masking the case", name)
		})
	}
}

// behaviorDiffAsUser sets the self-scoped auth context keys for the fixture user.
// Parameters:
//   - c: gin request context.
//   - f: seeded fixture.
//
// Return values: none.
func behaviorDiffAsUser(c *gin.Context, f *behaviorDiffFixture) {
	c.Set(ctxkey.Id, f.user.Id)
	c.Set(ctxkey.UserUUID, f.user.UUID)
	c.Set(ctxkey.Role, model.RoleCommonUser)
}

// behaviorDiffAsRoot sets the root-admin auth context keys.
// Parameters:
//   - c: gin request context.
//   - f: seeded fixture.
//
// Return values: none.
func behaviorDiffAsRoot(c *gin.Context, f *behaviorDiffFixture) {
	c.Set(ctxkey.Id, f.root.Id)
	c.Set(ctxkey.UserUUID, f.root.UUID)
	c.Set(ctxkey.Role, model.RoleRootUser)
}

// behaviorDiffAsAdmin sets the (non-root) admin auth context keys.
// Parameters:
//   - c: gin request context.
//   - f: seeded fixture.
//
// Return values: none.
func behaviorDiffAsAdmin(c *gin.Context, f *behaviorDiffFixture) {
	c.Set(ctxkey.Id, f.other.Id)
	c.Set(ctxkey.UserUUID, f.other.UUID)
	c.Set(ctxkey.Role, model.RoleAdminUser)
}

// behaviorDiffStatic returns a target/body builder for a value that does not
// depend on the fixture.
// Parameters:
//   - value: literal URL or body.
//
// Return values:
//   - func(*behaviorDiffFixture) string: builder returning value verbatim.
func behaviorDiffStatic(value string) func(f *behaviorDiffFixture) string {
	return func(*behaviorDiffFixture) string { return value }
}

// behaviorDiffMissingUUID is a well-formed UUID that matches no fixture row; it
// drives the "non-existent uuid" error cases.
const behaviorDiffMissingUUID = "018f0000-0000-7000-8000-0000deadbeef"

// behaviorDiffNormalizeToken registers the run-varying columns of a token row
// (generated uuid/key plus wall-clock timestamps written by the handler).
// Parameters:
//   - t: active test handle.
//   - name: token name to look up.
//   - n: normalizer to extend.
//
// Return values: none.
func behaviorDiffNormalizeToken(t *testing.T, name string, n *behaviorDiffNormalizer) {
	t.Helper()
	var row model.Token
	require.NoError(t, model.DB.Where("name = ?", name).First(&row).Error)
	n.addString(row.UUID, "<new-token-uuid>")
	n.addString(row.Key, "<new-token-key>")
	n.addNumber(row.CreatedTime, "<ts>")
	n.addNumber(row.AccessedTime, "<ts>")
	n.addNumber(row.CreatedAt, "<ts-milli>")
	n.addNumber(row.UpdatedAt, "<ts-milli>")
}

// behaviorDiffNormalizeUserIfCreated registers the generated identifiers of a
// user row only if the handler actually created one. It exists for the
// wrong-type probes: if a tree rejects the body no row appears (nothing to
// normalize), and if a tree accepts it the row's generated uuid is normalized —
// so the pre/post difference shows up in the response body, never as a
// normalization artifact.
// Parameters:
//   - t: active test handle.
//   - username: username the probe attempted to create.
//   - n: normalizer to extend.
//
// Return values: none.
func behaviorDiffNormalizeUserIfCreated(t *testing.T, username string, n *behaviorDiffNormalizer) {
	t.Helper()
	var rows []model.User
	require.NoError(t, model.DB.Where("username = ?", username).Find(&rows).Error)
	for _, row := range rows {
		n.addString(row.UUID, "<new-user-uuid>")
		n.addNumber(row.CreatedAt, "<ts-milli>")
		n.addNumber(row.UpdatedAt, "<ts-milli>")
	}
}

// behaviorDiffCases enumerates every Appendix A site (User 6, Token 9,
// Channel 4, Redemption 4, Log 5) plus the T18 error cases.
// Parameters: none.
//
// Return values:
//   - []behaviorDiffCase: the full capture set.
func behaviorDiffCases() []behaviorDiffCase {
	return append(append(append(append(
		behaviorDiffUserCases(),
		behaviorDiffTokenCases()...),
		behaviorDiffChannelCases()...),
		behaviorDiffRedemptionCases()...),
		behaviorDiffLogCases()...)
}

// behaviorDiffUserCases covers Appendix A sites U1-U6 and the T18 error cases
// for the flipped user handlers and the three inbound user request DTOs
// (register / create / self-update). The request DTOs are referenced only
// through HTTP bodies, never by Go type, so this file compiles pre-refactor too.
// Parameters: none.
//
// Return values:
//   - []behaviorDiffCase: user capture set.
func behaviorDiffUserCases() []behaviorDiffCase {
	return []behaviorDiffCase{
		{
			entity: "user", name: "login_success", site: "U1 SetupLogin",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/login",
				target:   behaviorDiffStatic("/api/user/login"),
				body:     behaviorDiffStatic(`{"username":"behaviordiff-user","password":"userpass123"}`),
				handler:  Login,
				sessions: true,
			},
		},
		{
			entity: "user", name: "login_bad_password", site: "U1 SetupLogin (error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/login",
				target:   behaviorDiffStatic("/api/user/login"),
				body:     behaviorDiffStatic(`{"username":"behaviordiff-user","password":"wrongpass123"}`),
				handler:  Login,
				sessions: true,
			},
		},
		{
			entity: "user", name: "login_malformed_json", site: "U1 SetupLogin (error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/login",
				target:   behaviorDiffStatic("/api/user/login"),
				body:     behaviorDiffStatic(`{"username":`),
				handler:  Login,
				sessions: true,
			},
		},
		{
			entity: "user", name: "get_all", site: "U2 GetAllUsers",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/user/",
				target:  behaviorDiffStatic("/api/user/?p=0&size=10"),
				prepare: behaviorDiffAsRoot,
				handler: GetAllUsers,
			},
		},
		{
			entity: "user", name: "search", site: "U3 SearchUsers",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/user/search",
				target:  behaviorDiffStatic("/api/user/search?keyword=behaviordiff"),
				prepare: behaviorDiffAsRoot,
				handler: SearchUsers,
			},
		},
		{
			entity: "user", name: "get_by_uuid", site: "U4 GetUser",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/user/:id",
				target:  func(f *behaviorDiffFixture) string { return "/api/user/" + f.user.UUID },
				prepare: behaviorDiffAsRoot,
				handler: GetUser,
			},
		},
		{
			entity: "user", name: "get_by_uuid_not_found", site: "U4 GetUser (error)",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/user/:id",
				target:  behaviorDiffStatic("/api/user/" + behaviorDiffMissingUUID),
				prepare: behaviorDiffAsRoot,
				handler: GetUser,
			},
		},
		{
			entity: "user", name: "get_by_uuid_insufficient_role", site: "U4 GetUser (error)",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/user/:id",
				// A non-root admin may not read a peer admin's record.
				target:  func(f *behaviorDiffFixture) string { return "/api/user/" + f.other.UUID },
				prepare: behaviorDiffAsAdmin,
				handler: GetUser,
			},
		},
		{
			entity: "user", name: "get_by_uuid_missing_role", site: "U4 GetUser (error)",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/user/:id",
				target: func(f *behaviorDiffFixture) string { return "/api/user/" + f.user.UUID },
				// No role key at all: the handler must still deny.
				prepare: func(c *gin.Context, f *behaviorDiffFixture) { c.Set(ctxkey.Id, f.user.Id) },
				handler: GetUser,
			},
		},
		{
			entity: "user", name: "get_self", site: "U5 GetSelf",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/user/self",
				target:  behaviorDiffStatic("/api/user/self"),
				prepare: behaviorDiffAsUser,
				handler: GetSelf,
			},
		},
		{
			entity: "user", name: "manage_disable", site: "U6 ManageUser",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/manage",
				target:  behaviorDiffStatic("/api/user/manage"),
				body:    behaviorDiffStatic(`{"username":"behaviordiff-user","action":"disable"}`),
				prepare: behaviorDiffAsRoot,
				handler: ManageUser,
			},
		},
		{
			entity: "user", name: "manage_malformed_json", site: "U6 ManageUser (error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/manage",
				target:  behaviorDiffStatic("/api/user/manage"),
				body:    behaviorDiffStatic(`{"username":`),
				prepare: behaviorDiffAsRoot,
				handler: ManageUser,
			},
		},
		{
			entity: "user", name: "manage_wrong_field_type", site: "U6 ManageUser (error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/manage",
				target:  behaviorDiffStatic("/api/user/manage"),
				body:    behaviorDiffStatic(`{"username":123,"action":"disable"}`),
				prepare: behaviorDiffAsRoot,
				handler: ManageUser,
			},
		},
		{
			entity: "user", name: "manage_unknown_user", site: "U6 ManageUser (error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/manage",
				target:  behaviorDiffStatic("/api/user/manage"),
				body:    behaviorDiffStatic(`{"username":"behaviordiff-nobody","action":"disable"}`),
				prepare: behaviorDiffAsRoot,
				handler: ManageUser,
			},
		},
		{
			entity: "user", name: "manage_insufficient_role", site: "U6 ManageUser (error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/manage",
				target:  behaviorDiffStatic("/api/user/manage"),
				body:    behaviorDiffStatic(`{"username":"behaviordiff-root","action":"disable"}`),
				prepare: behaviorDiffAsAdmin,
				handler: ManageUser,
			},
		},
		// --- T18: inbound request-DTO error paths (Register) ---
		{
			entity: "user", name: "register_malformed_json", site: "Register (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/register",
				target:  behaviorDiffStatic("/api/user/register"),
				body:    behaviorDiffStatic(`{"username":`),
				handler: Register,
			},
		},
		{
			entity: "user", name: "register_wrong_field_type", site: "Register (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/register",
				target:  behaviorDiffStatic("/api/user/register"),
				body:    behaviorDiffStatic(`{"username":123,"password":"newpass123"}`),
				handler: Register,
			},
		},
		{
			entity: "user", name: "register_wrong_type_unbound_field", site: "Register (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/register",
				target: behaviorDiffStatic("/api/user/register"),
				// "quota" is a real, JSON-tagged field of the struct this handler
				// binds into, but the handler never reads it. A type mismatch on it
				// is still a decode error for the whole body, so the request is
				// rejected — that rejection is part of the frozen inbound contract
				// (proposal I3) and must survive any change to what the handler
				// binds into.
				body:    behaviorDiffStatic(`{"username":"behaviordiff-new","password":"newpass123","quota":"not-a-number"}`),
				handler: Register,
			},
			dynamic: func(t *testing.T, f *behaviorDiffFixture, n *behaviorDiffNormalizer) {
				behaviorDiffNormalizeUserIfCreated(t, "behaviordiff-new", n)
			},
		},
		{
			entity: "user", name: "register_short_password", site: "Register (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/register",
				target:  behaviorDiffStatic("/api/user/register"),
				body:    behaviorDiffStatic(`{"username":"behaviordiff-new","password":"short"}`),
				handler: Register,
			},
		},
		{
			entity: "user", name: "register_duplicate_username", site: "Register (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/register",
				target:  behaviorDiffStatic("/api/user/register"),
				body:    behaviorDiffStatic(`{"username":"behaviordiff-user","password":"newpass123"}`),
				handler: Register,
			},
		},
		// --- T18: inbound request-DTO error paths (CreateUser) ---
		{
			entity: "user", name: "create_malformed_json", site: "CreateUser (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/",
				target:  behaviorDiffStatic("/api/user/"),
				body:    behaviorDiffStatic(`{"username":`),
				prepare: behaviorDiffAsRoot,
				handler: CreateUser,
			},
		},
		{
			entity: "user", name: "create_wrong_field_type", site: "CreateUser (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/",
				target:  behaviorDiffStatic("/api/user/"),
				body:    behaviorDiffStatic(`{"username":"behaviordiff-new","password":"newpass123","quota":"not-a-number"}`),
				prepare: behaviorDiffAsRoot,
				handler: CreateUser,
			},
		},
		{
			entity: "user", name: "create_wrong_type_unbound_field", site: "CreateUser (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/",
				target: behaviorDiffStatic("/api/user/"),
				// "status" is a real JSON-tagged field of the struct this handler
				// binds into but is never read by it; a type mismatch on it must
				// still reject the whole body (proposal I3).
				body:    behaviorDiffStatic(`{"username":"behaviordiff-new","password":"newpass123","status":"enabled"}`),
				prepare: behaviorDiffAsRoot,
				handler: CreateUser,
			},
			dynamic: func(t *testing.T, f *behaviorDiffFixture, n *behaviorDiffNormalizer) {
				behaviorDiffNormalizeUserIfCreated(t, "behaviordiff-new", n)
			},
		},
		{
			entity: "user", name: "create_role_too_high", site: "CreateUser (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/",
				target:  behaviorDiffStatic("/api/user/"),
				body:    behaviorDiffStatic(`{"username":"behaviordiff-new","password":"newpass123","role":100}`),
				prepare: behaviorDiffAsAdmin,
				handler: CreateUser,
			},
		},
		{
			entity: "user", name: "create_invalid_input", site: "CreateUser (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/user/",
				target:  behaviorDiffStatic("/api/user/"),
				body:    behaviorDiffStatic(`{"username":"behaviordiff-new","password":"waaaaaaaaaaaaaaaaaaaaay-too-long-password"}`),
				prepare: behaviorDiffAsRoot,
				handler: CreateUser,
			},
		},
		// --- T18: inbound request-DTO error paths (UpdateSelf) ---
		{
			entity: "user", name: "update_self_malformed_json", site: "UpdateSelf (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/user/self",
				target:  behaviorDiffStatic("/api/user/self"),
				body:    behaviorDiffStatic(`{"username":`),
				prepare: behaviorDiffAsUser,
				handler: UpdateSelf,
			},
		},
		{
			entity: "user", name: "update_self_wrong_field_type", site: "UpdateSelf (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/user/self",
				target:  behaviorDiffStatic("/api/user/self"),
				body:    behaviorDiffStatic(`{"display_name":123}`),
				prepare: behaviorDiffAsUser,
				handler: UpdateSelf,
			},
		},
		{
			entity: "user", name: "update_self_wrong_type_unbound_field", site: "UpdateSelf (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/user/self",
				target: behaviorDiffStatic("/api/user/self"),
				// "quota" is a real JSON-tagged field of the struct this handler
				// binds into but is never read by it; a type mismatch on it must
				// still reject the whole body (proposal I3).
				body:    behaviorDiffStatic(`{"display_name":"Renamed","quota":"lots"}`),
				prepare: behaviorDiffAsUser,
				handler: UpdateSelf,
			},
		},
		{
			entity: "user", name: "update_self_validation_error", site: "UpdateSelf (request DTO, error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/user/self",
				target: behaviorDiffStatic("/api/user/self"),
				// display_name exceeds the validate:"max=20" tag; the emitted
				// validator message embeds the validated struct's name, which the
				// request-DTO rebinding must not change.
				body:    behaviorDiffStatic(`{"display_name":"this display name is definitely far too long"}`),
				prepare: behaviorDiffAsUser,
				handler: UpdateSelf,
			},
		},
	}
}

// behaviorDiffTokenCases covers Appendix A sites T1-T9 and the T18 error cases
// for the flipped token handlers.
// Parameters: none.
//
// Return values:
//   - []behaviorDiffCase: token capture set.
func behaviorDiffTokenCases() []behaviorDiffCase {
	return []behaviorDiffCase{
		{
			entity: "token", name: "get_all", site: "T1 GetAllTokens",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/token/",
				target:  behaviorDiffStatic("/api/token/?p=0&size=10"),
				prepare: behaviorDiffAsUser,
				handler: GetAllTokens,
			},
		},
		{
			entity: "token", name: "search", site: "T2 SearchTokens",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/token/search",
				target:  behaviorDiffStatic("/api/token/search?keyword=behaviordiff&p=0&size=10"),
				prepare: behaviorDiffAsUser,
				handler: SearchTokens,
			},
		},
		{
			entity: "token", name: "get_by_uuid", site: "T3 GetToken",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/token/:id",
				target:  func(f *behaviorDiffFixture) string { return "/api/token/" + f.token.UUID },
				prepare: behaviorDiffAsUser,
				handler: GetToken,
			},
		},
		{
			entity: "token", name: "get_by_uuid_not_found", site: "T3 GetToken (error)",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/token/:id",
				target:  behaviorDiffStatic("/api/token/" + behaviorDiffMissingUUID),
				prepare: behaviorDiffAsUser,
				handler: GetToken,
			},
		},
		{
			entity: "token", name: "get_by_uuid_foreign_owner", site: "T3 GetToken (error)",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/token/:id",
				// The fixture user asks for another user's token.
				target:  func(f *behaviorDiffFixture) string { return "/api/token/" + f.otherToken.UUID },
				prepare: behaviorDiffAsUser,
				handler: GetToken,
			},
		},
		{
			entity: "token", name: "add", site: "T4 AddToken",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/token/",
				target:  behaviorDiffStatic("/api/token/"),
				body:    behaviorDiffStatic(`{"name":"behaviordiff-added-token","remain_quota":4242,"expired_time":-1,"unlimited_quota":false,"models":"gpt-4o","subnet":"172.16.0.0/12"}`),
				prepare: behaviorDiffAsUser,
				handler: AddToken,
			},
			dynamic: func(t *testing.T, f *behaviorDiffFixture, n *behaviorDiffNormalizer) {
				behaviorDiffNormalizeToken(t, "behaviordiff-added-token", n)
			},
		},
		{
			entity: "token", name: "add_malformed_json", site: "T4 AddToken (error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/token/",
				target:  behaviorDiffStatic("/api/token/"),
				body:    behaviorDiffStatic(`{"name":`),
				prepare: behaviorDiffAsUser,
				handler: AddToken,
			},
		},
		{
			entity: "token", name: "add_wrong_field_type", site: "T4 AddToken (error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/token/",
				target:  behaviorDiffStatic("/api/token/"),
				body:    behaviorDiffStatic(`{"name":"behaviordiff-bad","remain_quota":"lots"}`),
				prepare: behaviorDiffAsUser,
				handler: AddToken,
			},
		},
		{
			entity: "token", name: "add_empty_name", site: "T4 AddToken (error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/token/",
				target:  behaviorDiffStatic("/api/token/"),
				body:    behaviorDiffStatic(`{"name":"   ","remain_quota":1}`),
				prepare: behaviorDiffAsUser,
				handler: AddToken,
			},
		},
		{
			entity: "token", name: "consume_single", site: "T5 ConsumeToken",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/token/consume",
				target: behaviorDiffStatic("/api/token/consume"),
				body:   behaviorDiffStatic(`{"phase":"single","add_used_quota":150,"add_reason":"behaviordiff-consume"}`),
				prepare: func(c *gin.Context, f *behaviorDiffFixture) {
					c.Set(ctxkey.Id, f.user.Id)
					c.Set(ctxkey.UserUUID, f.user.UUID)
					c.Set(ctxkey.TokenId, f.token.Id)
					c.Set(ctxkey.TokenName, f.token.Name)
				},
				handler: ConsumeToken,
			},
			dynamic: func(t *testing.T, f *behaviorDiffFixture, n *behaviorDiffNormalizer) {
				var txn model.TokenTransaction
				require.NoError(t, model.DB.Where("reason = ?", "behaviordiff-consume").First(&txn).Error)
				n.addString(txn.UUID, "<consume-txn-uuid>")
				n.addString(txn.TransactionID, "<consume-txn-id>")
				n.addString(txn.TraceId, "<trace-id>")
				if txn.LogUUID != nil {
					n.addString(*txn.LogUUID, "<consume-log-uuid>")
				}
				if txn.ConfirmedAt != nil {
					n.addNumber(*txn.ConfirmedAt, "<ts>")
				}
				// The response echoes the in-memory pre-hold expiry, which
				// processPostConsume zeroes in the row, so it cannot be read back:
				// it is pre-phase GetTimestamp() + the default hold window.
				n.addTimestampWindow(f, int64(config.ExternalBillingDefaultTimeoutSec), "<ts>")

				var tok model.Token
				require.NoError(t, model.DB.First(&tok, f.token.Id).Error)
				n.addChangedNumber(behaviorDiffUpdatedAtMilli, tok.UpdatedAt, "<ts-milli>")
				n.addChangedNumber(behaviorDiffAccessedTime, tok.AccessedTime, "<ts>")
			},
		},
		{
			entity: "token", name: "consume_missing_reason", site: "T5 ConsumeToken (error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/token/consume",
				target: behaviorDiffStatic("/api/token/consume"),
				body:   behaviorDiffStatic(`{"phase":"single","add_used_quota":150,"add_reason":"   "}`),
				prepare: func(c *gin.Context, f *behaviorDiffFixture) {
					c.Set(ctxkey.Id, f.user.Id)
					c.Set(ctxkey.TokenId, f.token.Id)
				},
				handler: ConsumeToken,
			},
		},
		{
			entity: "token", name: "consume_malformed_json", site: "T5 ConsumeToken (error)",
			req: behaviorDiffRequest{
				method: http.MethodPost, pattern: "/api/token/consume",
				target: behaviorDiffStatic("/api/token/consume"),
				body:   behaviorDiffStatic(`{"phase":`),
				prepare: func(c *gin.Context, f *behaviorDiffFixture) {
					c.Set(ctxkey.Id, f.user.Id)
					c.Set(ctxkey.TokenId, f.token.Id)
				},
				handler: ConsumeToken,
			},
		},
		{
			entity: "token", name: "update", site: "T6 UpdateToken",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/token/",
				target: behaviorDiffStatic("/api/token/"),
				body: func(f *behaviorDiffFixture) string {
					return fmt.Sprintf(`{"uuid":%q,"name":"behaviordiff-token-renamed","status":1,"expired_time":-1,"remain_quota":123456,"unlimited_quota":false,"models":"gpt-4o","subnet":"192.168.0.0/24"}`, f.token.UUID)
				},
				prepare: behaviorDiffAsUser,
				handler: UpdateToken,
			},
			dynamic: func(t *testing.T, f *behaviorDiffFixture, n *behaviorDiffNormalizer) {
				var tok model.Token
				require.NoError(t, model.DB.First(&tok, f.token.Id).Error)
				n.addChangedNumber(behaviorDiffUpdatedAtMilli, tok.UpdatedAt, "<ts-milli>")
			},
		},
		{
			entity: "token", name: "update_missing_uuid", site: "T6 UpdateToken (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/token/",
				target:  behaviorDiffStatic("/api/token/"),
				body:    behaviorDiffStatic(`{"id":1,"name":"legacy-int-writer","status":1}`),
				prepare: behaviorDiffAsUser,
				handler: UpdateToken,
			},
		},
		{
			entity: "token", name: "update_not_found", site: "T6 UpdateToken (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/token/",
				target:  behaviorDiffStatic("/api/token/"),
				body:    behaviorDiffStatic(`{"uuid":"` + behaviorDiffMissingUUID + `","name":"nope","status":1}`),
				prepare: behaviorDiffAsUser,
				handler: UpdateToken,
			},
		},
		{
			entity: "token", name: "update_foreign_owner", site: "T6 UpdateToken (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/token/",
				target: behaviorDiffStatic("/api/token/"),
				body: func(f *behaviorDiffFixture) string {
					return fmt.Sprintf(`{"uuid":%q,"name":"stolen","status":1}`, f.otherToken.UUID)
				},
				prepare: behaviorDiffAsUser,
				handler: UpdateToken,
			},
		},
		{
			entity: "token", name: "update_malformed_json", site: "T6 UpdateToken (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/token/",
				target:  behaviorDiffStatic("/api/token/"),
				body:    behaviorDiffStatic(`{"uuid":`),
				prepare: behaviorDiffAsUser,
				handler: UpdateToken,
			},
		},
		{
			entity: "token", name: "admin_get_all", site: "T7 AdminGetAllTokens",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/admin/tokens/",
				target:  behaviorDiffStatic("/api/admin/tokens/?p=0&size=10"),
				prepare: behaviorDiffAsRoot,
				handler: AdminGetAllTokens,
			},
		},
		{
			entity: "token", name: "admin_search", site: "T8 AdminSearchTokens",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/admin/tokens/search",
				target:  behaviorDiffStatic("/api/admin/tokens/search?keyword=behaviordiff&p=0&size=10"),
				prepare: behaviorDiffAsRoot,
				handler: AdminSearchTokens,
			},
		},
		{
			entity: "token", name: "admin_get_by_uuid", site: "T9 AdminGetToken",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/admin/tokens/:id",
				target:  func(f *behaviorDiffFixture) string { return "/api/admin/tokens/" + f.otherToken.UUID },
				prepare: behaviorDiffAsRoot,
				handler: AdminGetToken,
			},
		},
		{
			entity: "token", name: "admin_get_by_uuid_not_found", site: "T9 AdminGetToken (error)",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/admin/tokens/:id",
				target:  behaviorDiffStatic("/api/admin/tokens/" + behaviorDiffMissingUUID),
				prepare: behaviorDiffAsRoot,
				handler: AdminGetToken,
			},
		},
	}
}

// behaviorDiffChannelCases covers Appendix A sites C1-C4 and the T18 error cases
// for the flipped channel handlers (including the channelListItem wrapper and
// the buildChannelResponsePayload tooling splice).
// Parameters: none.
//
// Return values:
//   - []behaviorDiffCase: channel capture set.
func behaviorDiffChannelCases() []behaviorDiffCase {
	return []behaviorDiffCase{
		{
			entity: "channel", name: "get_all", site: "C1 GetAllChannels",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/channel/",
				target:  behaviorDiffStatic("/api/channel/?p=0&size=10"),
				prepare: behaviorDiffAsRoot,
				handler: GetAllChannels,
			},
		},
		{
			entity: "channel", name: "search", site: "C2 SearchChannels",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/channel/search",
				target:  behaviorDiffStatic("/api/channel/search?keyword=behaviordiff"),
				prepare: behaviorDiffAsRoot,
				handler: SearchChannels,
			},
		},
		{
			entity: "channel", name: "get_by_uuid", site: "C3 GetChannel",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/channel/:id",
				target:  func(f *behaviorDiffFixture) string { return "/api/channel/" + f.channel.UUID },
				prepare: behaviorDiffAsRoot,
				handler: GetChannel,
			},
		},
		{
			entity: "channel", name: "get_by_uuid_not_found", site: "C3 GetChannel (error)",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/channel/:id",
				target:  behaviorDiffStatic("/api/channel/" + behaviorDiffMissingUUID),
				prepare: behaviorDiffAsRoot,
				handler: GetChannel,
			},
		},
		{
			entity: "channel", name: "update", site: "C4 UpdateChannel",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/channel/",
				target: behaviorDiffStatic("/api/channel/"),
				body: func(f *behaviorDiffFixture) string {
					return fmt.Sprintf(`{"uuid":%q,"name":"behaviordiff-channel-renamed","type":1,"key":"behaviordiff-channel-key","models":"gpt-4o,gpt-3.5-turbo","group":"default,vip","status":1,"priority":6,"weight":9}`, f.channel.UUID)
				},
				prepare: behaviorDiffAsRoot,
				handler: UpdateChannel,
			},
			dynamic: func(t *testing.T, f *behaviorDiffFixture, n *behaviorDiffNormalizer) {
				var ch model.Channel
				require.NoError(t, model.DB.First(&ch, f.channel.Id).Error)
				n.addChangedNumber(behaviorDiffUpdatedAtMilli, ch.UpdatedAt, "<ts-milli>")
			},
		},
		{
			entity: "channel", name: "update_missing_uuid", site: "C4 UpdateChannel (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/channel/",
				target:  behaviorDiffStatic("/api/channel/"),
				body:    behaviorDiffStatic(`{"id":1,"name":"legacy-int-writer","type":1}`),
				prepare: behaviorDiffAsRoot,
				handler: UpdateChannel,
			},
		},
		{
			entity: "channel", name: "update_not_found", site: "C4 UpdateChannel (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/channel/",
				target:  behaviorDiffStatic("/api/channel/"),
				body:    behaviorDiffStatic(`{"uuid":"` + behaviorDiffMissingUUID + `","name":"nope","type":1}`),
				prepare: behaviorDiffAsRoot,
				handler: UpdateChannel,
			},
		},
		{
			entity: "channel", name: "update_empty_name", site: "C4 UpdateChannel (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/channel/",
				target: behaviorDiffStatic("/api/channel/"),
				body: func(f *behaviorDiffFixture) string {
					return fmt.Sprintf(`{"uuid":%q,"name":"   ","type":1}`, f.channel.UUID)
				},
				prepare: behaviorDiffAsRoot,
				handler: UpdateChannel,
			},
		},
		{
			entity: "channel", name: "update_malformed_json", site: "C4 UpdateChannel (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/channel/",
				target:  behaviorDiffStatic("/api/channel/"),
				body:    behaviorDiffStatic(`{"uuid":`),
				prepare: behaviorDiffAsRoot,
				handler: UpdateChannel,
			},
		},
		{
			entity: "channel", name: "update_wrong_field_type", site: "C4 UpdateChannel (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/channel/",
				target: behaviorDiffStatic("/api/channel/"),
				body: func(f *behaviorDiffFixture) string {
					return fmt.Sprintf(`{"uuid":%q,"name":"behaviordiff","type":"openai"}`, f.channel.UUID)
				},
				prepare: behaviorDiffAsRoot,
				handler: UpdateChannel,
			},
		},
	}
}

// behaviorDiffRedemptionCases covers Appendix A sites R1-R4 and the T18 error
// cases for the flipped redemption handlers.
// Parameters: none.
//
// Return values:
//   - []behaviorDiffCase: redemption capture set.
func behaviorDiffRedemptionCases() []behaviorDiffCase {
	return []behaviorDiffCase{
		{
			entity: "redemption", name: "get_all", site: "R1 GetAllRedemptions",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/redemption/",
				target:  behaviorDiffStatic("/api/redemption/?p=0&size=10"),
				prepare: behaviorDiffAsRoot,
				handler: GetAllRedemptions,
			},
		},
		{
			entity: "redemption", name: "search", site: "R2 SearchRedemptions",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/redemption/search",
				target:  behaviorDiffStatic("/api/redemption/search?keyword=behaviordiff&p=0&size=10"),
				prepare: behaviorDiffAsRoot,
				handler: SearchRedemptions,
			},
		},
		{
			entity: "redemption", name: "get_by_uuid", site: "R3 GetRedemption",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/redemption/:id",
				target:  func(f *behaviorDiffFixture) string { return "/api/redemption/" + f.redemption.UUID },
				prepare: behaviorDiffAsRoot,
				handler: GetRedemption,
			},
		},
		{
			entity: "redemption", name: "get_by_uuid_not_found", site: "R3 GetRedemption (error)",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/redemption/:id",
				target:  behaviorDiffStatic("/api/redemption/" + behaviorDiffMissingUUID),
				prepare: behaviorDiffAsRoot,
				handler: GetRedemption,
			},
		},
		{
			entity: "redemption", name: "update", site: "R4 UpdateRedemption",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/redemption/",
				target: behaviorDiffStatic("/api/redemption/"),
				body: func(f *behaviorDiffFixture) string {
					return fmt.Sprintf(`{"uuid":%q,"name":"behaviordiff-renamed","quota":5150}`, f.redemption.UUID)
				},
				prepare: behaviorDiffAsRoot,
				handler: UpdateRedemption,
			},
			dynamic: func(t *testing.T, f *behaviorDiffFixture, n *behaviorDiffNormalizer) {
				var row model.Redemption
				require.NoError(t, model.DB.First(&row, f.redemption.Id).Error)
				n.addChangedNumber(behaviorDiffUpdatedAtMilli, row.UpdatedAt, "<ts-milli>")
			},
		},
		{
			entity: "redemption", name: "update_status_only", site: "R4 UpdateRedemption",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/redemption/",
				target: behaviorDiffStatic("/api/redemption/?status_only=true"),
				body: func(f *behaviorDiffFixture) string {
					return fmt.Sprintf(`{"uuid":%q,"status":2}`, f.redemption.UUID)
				},
				prepare: behaviorDiffAsRoot,
				handler: UpdateRedemption,
			},
			dynamic: func(t *testing.T, f *behaviorDiffFixture, n *behaviorDiffNormalizer) {
				var row model.Redemption
				require.NoError(t, model.DB.First(&row, f.redemption.Id).Error)
				n.addChangedNumber(behaviorDiffUpdatedAtMilli, row.UpdatedAt, "<ts-milli>")
			},
		},
		{
			entity: "redemption", name: "update_missing_uuid", site: "R4 UpdateRedemption (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/redemption/",
				target:  behaviorDiffStatic("/api/redemption/?status_only=true"),
				body:    behaviorDiffStatic(`{"id":1,"status":1}`),
				prepare: behaviorDiffAsRoot,
				handler: UpdateRedemption,
			},
		},
		{
			entity: "redemption", name: "update_not_found", site: "R4 UpdateRedemption (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/redemption/",
				target:  behaviorDiffStatic("/api/redemption/?status_only=true"),
				body:    behaviorDiffStatic(`{"uuid":"` + behaviorDiffMissingUUID + `","status":1}`),
				prepare: behaviorDiffAsRoot,
				handler: UpdateRedemption,
			},
		},
		{
			entity: "redemption", name: "update_empty_name", site: "R4 UpdateRedemption (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/redemption/",
				target: behaviorDiffStatic("/api/redemption/"),
				body: func(f *behaviorDiffFixture) string {
					return fmt.Sprintf(`{"uuid":%q,"name":"  ","quota":1}`, f.redemption.UUID)
				},
				prepare: behaviorDiffAsRoot,
				handler: UpdateRedemption,
			},
		},
		{
			entity: "redemption", name: "update_malformed_json", site: "R4 UpdateRedemption (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/redemption/",
				target:  behaviorDiffStatic("/api/redemption/"),
				body:    behaviorDiffStatic(`{"uuid":`),
				prepare: behaviorDiffAsRoot,
				handler: UpdateRedemption,
			},
		},
		{
			entity: "redemption", name: "update_wrong_field_type", site: "R4 UpdateRedemption (error)",
			req: behaviorDiffRequest{
				method: http.MethodPut, pattern: "/api/redemption/",
				target: behaviorDiffStatic("/api/redemption/"),
				body: func(f *behaviorDiffFixture) string {
					return fmt.Sprintf(`{"uuid":%q,"name":"behaviordiff","quota":"lots"}`, f.redemption.UUID)
				},
				prepare: behaviorDiffAsRoot,
				handler: UpdateRedemption,
			},
		},
	}
}

// behaviorDiffLogCases covers Appendix A sites L1-L5 and the T18 error cases for
// the flipped log handlers.
// Parameters: none.
//
// Return values:
//   - []behaviorDiffCase: log capture set.
func behaviorDiffLogCases() []behaviorDiffCase {
	return []behaviorDiffCase{
		{
			entity: "log", name: "get_all", site: "L1 GetAllLogs",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/log/",
				target:  behaviorDiffStatic("/api/log/?p=0&size=10"),
				prepare: behaviorDiffAsRoot,
				handler: GetAllLogs,
			},
		},
		{
			entity: "log", name: "get_all_filtered_by_channel", site: "L1 GetAllLogs",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/log/",
				target: func(f *behaviorDiffFixture) string {
					return "/api/log/?p=0&size=10&channel=" + f.channel.UUID
				},
				prepare: behaviorDiffAsRoot,
				handler: GetAllLogs,
			},
		},
		{
			entity: "log", name: "get_all_bad_channel_ref", site: "L1 GetAllLogs (error)",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/log/",
				target:  behaviorDiffStatic("/api/log/?p=0&size=10&channel=" + behaviorDiffMissingUUID),
				prepare: behaviorDiffAsRoot,
				handler: GetAllLogs,
			},
		},
		{
			entity: "log", name: "get_all_sort_range_too_large", site: "L1 GetAllLogs (error)",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/log/",
				target:  behaviorDiffStatic("/api/log/?p=0&size=10&sort=quota&start_timestamp=1700000000&end_timestamp=1799999999"),
				prepare: behaviorDiffAsRoot,
				handler: GetAllLogs,
			},
		},
		{
			entity: "log", name: "get_user", site: "L2 GetUserLogs",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/log/self",
				target:  behaviorDiffStatic("/api/log/self?p=0&size=10"),
				prepare: behaviorDiffAsUser,
				handler: GetUserLogs,
			},
		},
		{
			entity: "log", name: "get_user_sort_range_too_large", site: "L2 GetUserLogs (error)",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/log/self",
				target:  behaviorDiffStatic("/api/log/self?p=0&size=10&sort=quota&start_timestamp=1700000000&end_timestamp=1799999999"),
				prepare: behaviorDiffAsUser,
				handler: GetUserLogs,
			},
		},
		{
			entity: "log", name: "get_token", site: "L3 GetTokenLogs",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/token/logs",
				target: behaviorDiffStatic("/api/token/logs?p=0&size=10"),
				prepare: func(c *gin.Context, f *behaviorDiffFixture) {
					c.Set(ctxkey.Id, f.user.Id)
					c.Set(ctxkey.TokenId, f.token.Id)
					c.Set(ctxkey.TokenName, f.token.Name)
				},
				handler: GetTokenLogs,
			},
		},
		{
			entity: "log", name: "get_token_foreign_owner", site: "L3 GetTokenLogs (error)",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/token/logs",
				// Token name of another user's token combined with this user's id
				// must yield no rows (ownership is enforced by the user_id filter).
				target: behaviorDiffStatic("/api/token/logs?p=0&size=10"),
				prepare: func(c *gin.Context, f *behaviorDiffFixture) {
					c.Set(ctxkey.Id, f.user.Id)
					c.Set(ctxkey.TokenId, f.otherToken.Id)
					c.Set(ctxkey.TokenName, f.otherToken.Name)
				},
				handler: GetTokenLogs,
			},
		},
		{
			entity: "log", name: "search_all", site: "L4 SearchAllLogs",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/log/search",
				target:  behaviorDiffStatic("/api/log/search?keyword=behaviordiff&p=0&size=10"),
				prepare: behaviorDiffAsRoot,
				handler: SearchAllLogs,
			},
		},
		{
			entity: "log", name: "search_user", site: "L5 SearchUserLogs",
			req: behaviorDiffRequest{
				method: http.MethodGet, pattern: "/api/log/self/search",
				target:  behaviorDiffStatic("/api/log/self/search?keyword=behaviordiff&p=0&size=10"),
				prepare: behaviorDiffAsUser,
				handler: SearchUserLogs,
			},
		},
	}
}
