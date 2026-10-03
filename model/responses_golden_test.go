package model

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// updateGolden regenerates the frozen response goldens from the current mapper
// output. Run `go test ./model -run TestResponseGoldens -update-golden` to
// refresh them; every refresh must be reviewed as a deliberate contract change.
var updateGolden = flag.Bool("update-golden", false, "regenerate model/testdata/*_response.golden.json")

// goldenDir is the on-disk home of the frozen external-contract fixtures.
const goldenDir = "testdata"

// assertGolden freezes the external JSON contract for a boundary response DTO.
//
// Parameters:
//   - t: active test.
//   - name: golden basename (without the .golden.json suffix).
//   - v: the value to serialize (a dto.*Response or, before an entity migrates,
//     the model entity itself through its legacy MarshalJSON).
//
// Return values: none. It writes the golden when -update-golden is set and
// otherwise asserts byte-for-byte (JSON-semantic) equality plus an exact
// top-level key-set match, so an added or dropped field fails loudly (T1/T2).
func assertGolden(t *testing.T, name string, v any) {
	t.Helper()

	got, err := json.MarshalIndent(v, "", "  ")
	require.NoError(t, err, "marshal %s response", name)

	path := filepath.Join(goldenDir, name+".golden.json")
	if *updateGolden {
		require.NoError(t, os.MkdirAll(goldenDir, 0o755))
		require.NoError(t, os.WriteFile(path, append(got, '\n'), 0o644))
		return
	}

	want, err := os.ReadFile(path)
	require.NoError(t, err, "read golden %s (run with -update-golden to create)", path)

	// Semantic equality is map-order-insensitive, so goldens never flake on
	// map key ordering (T2). It also distinguishes null-present from
	// key-absent, so an omitempty regression is caught.
	require.JSONEq(t, string(want), string(got), "golden mismatch for %s", name)

	// Exact top-level key-set walk: guards against a field being renamed to a
	// key that JSONEq would still accept only if values coincided.
	require.Equal(t, topLevelKeys(t, want), topLevelKeys(t, got),
		"top-level key-set drift for %s", name)
}

// topLevelKeys returns the sorted set of top-level object keys in a JSON blob.
func topLevelKeys(t *testing.T, blob []byte) []string {
	t.Helper()
	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(blob, &m))
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Deterministic order for comparison.
	sortStrings(keys)
	return keys
}

// sortStrings is a tiny dependency-free sort to avoid pulling sort into the
// test's import set for a single call site.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// --- golden pointer helpers (test-local to avoid colliding with model code) ---

func goldenStrPtr(s string) *string { return &s }
func goldenIntPtr(i int) *int       { return &i }
func goldenInt64Ptr(i int64) *int64 { return &i }
func goldenUintPtr(u uint) *uint    { return &u }

// --- fully-populated fixtures (every response field non-zero, so an omission is visible) ---

// goldenRedemptionFixture builds a Redemption with every response field set.
func goldenRedemptionFixture() Redemption {
	return Redemption{
		Id:           101,
		UUID:         "018f0000-0000-7000-8000-0000000000a1",
		UserId:       202,
		UserUUID:     goldenStrPtr("018f0000-0000-7000-8000-0000000000a2"),
		Key:          "REDEEMCODE0000000000000000000001",
		Status:       RedemptionCodeStatusEnabled,
		Name:         "golden-redemption",
		Quota:        123456,
		CreatedTime:  1710000000,
		RedeemedTime: 1710000600,
		Count:        7,
		CreatedAt:    1710000001,
		UpdatedAt:    1710000002,
	}
}

// goldenLogFixture builds a Log with every response field set (incl. the
// omitempty channel_name/metadata, populated so they appear in the golden).
func goldenLogFixture() Log {
	return Log{
		Id:                 303,
		UserId:             404,
		UUID:               "018f0000-0000-7000-8000-0000000000b1",
		UserUUID:           goldenStrPtr("018f0000-0000-7000-8000-0000000000b2"),
		CreatedAt:          1710001000,
		Type:               LogTypeConsume,
		Content:            "golden log content",
		Username:           "golden-user",
		TokenName:          "golden-token",
		TokenUUID:          goldenStrPtr("018f0000-0000-7000-8000-0000000000b3"),
		ModelName:          "gpt-golden",
		OriginModelName:    "gpt-golden-origin",
		Quota:              555,
		PromptTokens:       111,
		CompletionTokens:   222,
		ChannelId:          606,
		ChannelUUID:        goldenStrPtr("018f0000-0000-7000-8000-0000000000b4"),
		ChannelName:        "golden-channel",
		RequestId:          "req-golden-0001",
		TraceId:            "trace-golden-0001",
		UpdatedAt:          1710001002,
		ElapsedTime:        1234,
		IsStream:           true,
		SystemPromptReset:  true,
		CachedPromptTokens: 42,
		Metadata:           LogMetadata{"cache_write_tokens": float64(17)},
	}
}

// goldenTokenFixture builds a Token with every response field set. The Key
// carries a legacy "sk-" prefix so the golden exercises prefix normalization.
func goldenTokenFixture() Token {
	return Token{
		Id:             707,
		UUID:           "018f0000-0000-7000-8000-0000000000c1",
		UserId:         808,
		UserUUID:       goldenStrPtr("018f0000-0000-7000-8000-0000000000c2"),
		Key:            "sk-goldenrawtokenkey0000000000000000000000000001",
		Status:         TokenStatusEnabled,
		Name:           "golden-token",
		CreatedTime:    1710002000,
		AccessedTime:   1710002100,
		ExpiredTime:    1710002200,
		RemainQuota:    900,
		UnlimitedQuota: true,
		UsedQuota:      100,
		CreatedAt:      1710002001,
		UpdatedAt:      1710002002,
		Models:         goldenStrPtr("gpt-golden,gpt-golden-2"),
		Subnet:         goldenStrPtr("10.0.0.0/8"),
	}
}

// goldenUserFixture builds a User with every response field set. Secrets are
// populated to prove they never reach the response contract.
func goldenUserFixture() User {
	return User{
		Id:               909,
		UUID:             "018f0000-0000-7000-8000-0000000000d1",
		Username:         "golden-user",
		Password:         "$2a$10$goldenbcrypthashsecretvalue000",
		DisplayName:      "Golden User",
		Role:             RoleCommonUser,
		Status:           UserStatusEnabled,
		Email:            "golden@example.com",
		GitHubId:         "golden-github",
		WeChatId:         "golden-wechat",
		LarkId:           "golden-lark",
		OidcId:           "golden-oidc",
		VerificationCode: "verif-golden",
		AccessToken:      "golden-access-token-secret-00000",
		TotpSecret:       "GOLDENTOTPSECRET",
		Quota:            50000,
		UsedQuota:        1500,
		RequestCount:     33,
		Group:            "vip",
		AffCode:          "GOLDENAFF",
		InviterId:        111,
		InviterUUID:      goldenStrPtr("018f0000-0000-7000-8000-0000000000d2"),
		MCPToolBlacklist: JSONStringSlice{"tool-a", "tool-b"},
		Metadata:         UserMetadata{PasswordLocked: true},
		CreatedAt:        1710003001,
		UpdatedAt:        1710003002,
	}
}

// goldenChannelFixture builds a Channel with every response field set.
func goldenChannelFixture() Channel {
	return Channel{
		Id:                     121,
		UUID:                   "018f0000-0000-7000-8000-0000000000e1",
		Type:                   8,
		Key:                    "golden-channel-key-secret",
		Status:                 1,
		Name:                   "golden-channel",
		Weight:                 goldenUintPtr(5),
		CreatedTime:            1710004000,
		TestTime:               1710004100,
		ResponseTime:           321,
		BaseURL:                goldenStrPtr("https://golden.example.com"),
		Other:                  goldenStrPtr("golden-other"),
		Balance:                12.5,
		BalanceUpdatedTime:     1710004200,
		Models:                 "gpt-golden,gpt-golden-2",
		HiddenModels:           goldenStrPtr("gpt-hidden"),
		ModelConfigs:           goldenStrPtr(`{"gpt-golden":{"ratio":1}}`),
		Group:                  "default,vip",
		UsedQuota:              98765,
		ModelMapping:           goldenStrPtr(`{"alias":"gpt-golden"}`),
		Priority:               goldenInt64Ptr(9),
		Config:                 `{"region":"golden"}`,
		SystemPrompt:           goldenStrPtr("golden system prompt"),
		RateLimit:              goldenIntPtr(60),
		TestingModel:           goldenStrPtr("gpt-golden"),
		ModelRatio:             goldenStrPtr(`{"gpt-golden":2}`),
		CompletionRatio:        goldenStrPtr(`{"gpt-golden":3}`),
		CreatedAt:              1710004001,
		UpdatedAt:              1710004002,
		InferenceProfileArnMap: goldenStrPtr(`{"gpt-golden":"arn:aws:golden"}`),
	}
}

// TestResponseGoldens freezes the external contract for every migrated entity.
// Each subtest is switched from the legacy entity value to its dto.*Response
// mapper output as that entity migrates; the golden bytes stay frozen, so the
// mapper is proven byte-identical to the retired MarshalJSON (T1).
func TestResponseGoldens(t *testing.T) {
	// Pin the token key prefix so the Token golden is deterministic across
	// environments (the legacy marshaler and the mapper both read it).
	oldPrefix := config.TokenKeyPrefix
	config.TokenKeyPrefix = "sk-"
	t.Cleanup(func() { config.TokenKeyPrefix = oldPrefix })

	// NOTE: each subtest below is flipped from the raw model entity (legacy
	// MarshalJSON) to fixture.ToResponse() as its entity migrates (P1-P5). The
	// golden bytes are captured once from the legacy marshalers and then stay
	// frozen, so a flipped mapper is proven byte-identical to what it replaced.
	t.Run("redemption", func(t *testing.T) {
		fixture := goldenRedemptionFixture()
		assertGolden(t, "redemption_response", fixture.ToResponse())
	})
	t.Run("log", func(t *testing.T) {
		fixture := goldenLogFixture()
		assertGolden(t, "log_response", fixture.ToResponse())
	})
	t.Run("token", func(t *testing.T) {
		fixture := goldenTokenFixture()
		assertGolden(t, "token_response", fixture.ToResponse())
	})
	t.Run("user", func(t *testing.T) {
		fixture := goldenUserFixture()
		assertGolden(t, "user_response", fixture.ToResponse())
	})
	t.Run("channel", func(t *testing.T) {
		fixture := goldenChannelFixture()
		assertGolden(t, "channel_response", fixture.ToResponse())
	})
}
