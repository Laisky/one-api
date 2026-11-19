package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// strictImageSettlement is the observable outcome of one strict GPT Image
// relay: the balance delta, the persisted request cost, and the consume log.
type strictImageSettlement struct {
	charged  int64
	cost     int64
	logQuota int
	content  string
	metadata model.LogMetadata
}

// relayStrictImageReceipt drives one /v1/images/generations request for name
// through RelayImageHelper against a fake OpenAI upstream that answers with
// usageJSON, and returns what the SQLite ledger recorded. Parameters: t runs
// the case, name is the strict-billing model, usageJSON is the upstream usage
// object, and requestID labels the ledger rows. Returns: the settlement.
func relayStrictImageReceipt(t *testing.T, name, usageJSON, requestID string) strictImageSettlement {
	t.Helper()
	const balance = int64(1_000_000)
	cleanup := setupCacheBillingLogTest(t)
	t.Cleanup(cleanup)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelGroup{}, &model.UserRequestCost{}))
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", 1).Update("quota", balance).Error)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Update("remain_quota", balance).Error)
	oldLog := config.IsLogConsumeEnabled()
	config.SetLogConsumeEnabled(true)
	t.Cleanup(func() { config.SetLogConsumeEnabled(oldLog) })
	oldBatch := config.BatchUpdateEnabled
	config.BatchUpdateEnabled = false
	t.Cleanup(func() { config.BatchUpdateEnabled = oldBatch })

	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"created":1,"data":[{"b64_json":"AQ=="}],"usage":%s}`, usageJSON)
	}))
	t.Cleanup(upstream.Close)

	// The operator reservation/fallback is deliberately small ($0.01) so a
	// fallback settlement is distinguishable from every token-priced outcome.
	pricingJSON := fmt.Sprintf(`{%q:{"image":{"price_per_image_usd":0.01}}}`, name)
	channel := &model.Channel{Id: 1, Type: channeltype.OpenAI, Name: "strict-image", Status: model.ChannelStatusEnabled, ModelConfigs: &pricingJSON}
	require.NoError(t, model.DB.Create(channel).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations",
		strings.NewReader(fmt.Sprintf(`{"model":%q,"prompt":"A lighthouse","n":1}`, name)))
	c.Request.Header.Set("Content-Type", "application/json")
	gmw.SetLogger(c, logger.Logger)
	c.Set(ctxkey.Id, 1)
	c.Set(ctxkey.TokenId, 1)
	c.Set(ctxkey.TokenName, "relay-cache-log-token")
	c.Set(ctxkey.ChannelId, 1)
	c.Set(ctxkey.ChannelRatio, 1.0)
	c.Set(ctxkey.ChannelModel, channel)
	c.Set(ctxkey.ContentType, "application/json")
	c.Set(ctxkey.RequestId, requestID)
	mode := relaymode.GetByPath("/v1/images/generations")
	metalib.Set2Context(c, &metalib.Meta{
		Mode: mode, ChannelType: channeltype.OpenAI, APIType: channeltype.ToAPIType(channeltype.OpenAI),
		ChannelId: 1, UserId: 1, TokenId: 1, TokenName: "relay-cache-log-token",
		BaseURL: upstream.URL, APIKey: "test-only", OriginModelName: name,
		ActualModelName: name, RequestURLPath: "/v1/images/generations", StartTime: time.Now(),
	})

	require.Nil(t, RelayImageHelper(c, mode))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.EqualValues(t, 1, calls.Load(), "control: the paid upstream must be reached exactly once")

	var user model.User
	require.NoError(t, model.DB.First(&user, 1).Error)
	var cost model.UserRequestCost
	require.NoError(t, model.DB.Where("request_id = ?", requestID).First(&cost).Error)
	var entry model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).First(&entry).Error)
	return strictImageSettlement{
		charged: balance - user.Quota, cost: cost.Quota, logQuota: entry.Quota,
		content: entry.Content, metadata: entry.Metadata,
	}
}

// TestStrictGPTImageModalityAmbiguityNeverUnderbills drives GPT Image 2.5/2.6
// receipts whose input modality or cache split is not fully stated. The token
// totals are authoritative, so the gateway must charge the most expensive
// assignment consistent with them (never the cheaper text rate, never a small
// operator fallback below the provable cost) and flag the charge as estimated.
// Fully classified receipts stay exact and unflagged. Prices for 2.6 and
// 2.5-flare: text $5, cached text $1.25, image $8, cached image $2, output $30
// per million tokens; $1 = 500,000 quota.
func TestStrictGPTImageModalityAmbiguityNeverUnderbills(t *testing.T) {
	for _, tc := range []struct {
		name, model, usage string
		want               int64
		estimated          bool
	}{
		// Control: every input token is classified, so the charge is exact.
		// 200*5 + 800*8 + 1000*30 = 37,400 micro-USD.
		{"classified_control", "gpt-image-2.6-sunburst",
			`{"input_tokens":1000,"output_tokens":1000,"input_tokens_details":{"text_tokens":200,"image_tokens":800}}`, 18_700, false},
		// No modality split at all: the 1,000 input tokens may be image tokens.
		// 1000*8 + 1000*30 = 38,000 (the text rate would give 35,000).
		{"unclassified_input", "gpt-image-2.6-sunburst",
			`{"input_tokens":1000,"output_tokens":1000}`, 19_000, true},
		// 800 of 1,000 input tokens are unclassified: 200*5 + 800*8 + 30,000.
		{"partially_classified_input", "gpt-image-2.6-flare",
			`{"input_tokens":1000,"output_tokens":1000,"input_tokens_details":{"text_tokens":200}}`, 18_700, true},
		// Unclassified input with an aggregate cache count: 500 cached tokens of
		// unknown modality at most $2, 500 uncached at most $8: 4,000 + 1,000 + 30,000.
		{"unclassified_cached_input", "gpt-image-2.6-sunburst",
			`{"input_tokens":1000,"output_tokens":1000,"input_tokens_details":{"cached_tokens":500}}`, 17_500, true},
		// Stated split with an unknown cache split: the cache discount is taken
		// on the cheaper-discount (text) bucket first: 1000*1.25 + 1000*8 + 30,000.
		{"ambiguous_cache_split", "gpt-image-2.5-flare",
			`{"input_tokens":2000,"output_tokens":1000,"input_tokens_details":{"text_tokens":1000,"image_tokens":1000,"cached_tokens":1000}}`, 19_625, true},
		// Fully stated cache split plus an unclassified uncached remainder:
		// 100*5 + 100*1.25 + 300*8 + 500*8 + 30,000 = 37,025 -> ceil(18,512.5).
		{"cache_split_with_unclassified_remainder", "gpt-image-2.6-sunburst",
			`{"input_tokens":1000,"output_tokens":1000,"input_tokens_details":{"text_tokens":200,"image_tokens":300,"cached_tokens":100,"cached_tokens_details":{"text_tokens":100,"image_tokens":0}}}`, 18_513, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := relayStrictImageReceipt(t, tc.model, tc.usage, "strict-image-"+tc.name)
			require.Equal(t, tc.want, got.charged, "balance delta")
			require.Equal(t, tc.want, got.cost, "request cost")
			require.EqualValues(t, tc.want, got.logQuota, "consume log quota")
			require.NotContains(t, got.content, "billing_source=configured_fallback",
				"a usable token receipt must never settle at the operator fallback")
			if tc.estimated {
				require.Contains(t, got.content, "billing_source=provider_tokens_upper_bound")
				require.Equal(t, true, got.metadata["billing_estimated"], "an upper-bound charge must be flagged")
				require.Equal(t, strictImageModalityUpperBoundReason, got.metadata["billing_estimate_reason"])
				return
			}
			require.Contains(t, got.content, "billing_source=provider_tokens")
			require.NotContains(t, got.content, "upper_bound")
			require.Nil(t, got.metadata["billing_estimated"], "an exact receipt must not be flagged as estimated")
		})
	}
}

// TestStrictGPTImageUnusableReceiptFallbackIsFlagged proves an incomplete or
// inconsistent receipt settles the larger of the operator fallback and the
// receipt's own ceiling (every reported token at the highest applicable rate),
// so it never charges less than the tokens it reports, and that either charge
// is flagged as an estimate. The fallback here is $0.01 = 5,000 quota.
func TestStrictGPTImageUnusableReceiptFallbackIsFlagged(t *testing.T) {
	for _, tc := range []struct {
		name, usage, reason, source string
		want                        int64
	}{
		{"missing_usage", `null`, strictImageConfiguredFallbackReason, "configured_fallback", 5_000},
		// Input only: 1000*$8 = 4,000 quota, below the fallback.
		{"no_output_tokens", `{"input_tokens":1000,"output_tokens":0}`, strictImageConfiguredFallbackReason, "configured_fallback", 5_000},
		// The split (160) exceeds the input total (100): price 160*$8 + 1000*$30.
		{"inconsistent_split", `{"input_tokens":100,"output_tokens":1000,"input_tokens_details":{"text_tokens":80,"image_tokens":80}}`,
			strictImageIncompleteReceiptReason, "provider_tokens_upper_bound", 15_640},
		// No input count at all: output alone (1000*$30 = 15,000) exceeds the fallback.
		{"input_missing", `{"output_tokens":1000}`, strictImageIncompleteReceiptReason, "provider_tokens_upper_bound", 15_000},
		// input_tokens omitted but total_tokens recovers 500 unclassified input
		// tokens: 500*$8 + 1000*$30 = 34,000 micro-USD.
		{"input_recovered_from_total", `{"output_tokens":1000,"total_tokens":1500}`,
			strictImageModalityUpperBoundReason, "provider_tokens_upper_bound", 17_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := relayStrictImageReceipt(t, "gpt-image-2.6-sunburst", tc.usage, "strict-image-fallback-"+tc.name)
			require.EqualValues(t, tc.want, got.charged)
			require.EqualValues(t, tc.want, got.cost)
			require.EqualValues(t, tc.want, got.logQuota)
			require.Contains(t, got.content, "billing_source="+tc.source+" ")
			require.Equal(t, true, got.metadata["billing_estimated"])
			require.Equal(t, tc.reason, got.metadata["billing_estimate_reason"])
		})
	}
}

// TestStrictGPTImageCacheSplitFlagIsAccurate proves an omitted cache split is
// flagged as an upper bound only when the split could change the charge: not
// when every input token is cached, and not when both modalities share one
// cache discount (an operator prompt_ratio of 1), where the charge is exact.
func TestStrictGPTImageCacheSplitFlagIsAccurate(t *testing.T) {
	t.Parallel()
	catalog := gptImageTokenBucketPrices["gpt-image-2.6-sunburst"]
	split := func(text, image, cached int) *relaymodel.Usage {
		return &relaymodel.Usage{PromptTokens: text + image, CompletionTokens: 100,
			PromptTokensDetails: &relaymodel.UsagePromptTokensDetails{TextTokens: text, ImageTokens: image, CachedTokens: cached}}
	}
	require.True(t, computeStrictGPTImageTokenQuote(split(500, 500, 300), catalog, 1).upperBound, "control: distinct discounts are ambiguous")
	allCached := computeStrictGPTImageTokenQuote(split(500, 500, 1000), catalog, 1)
	require.False(t, allCached.upperBound, "every input token cached leaves no split to bound")
	require.InDelta(t, (500*1.25+500*2.0+100*30.0)*0.5, allCached.quota, 1e-9)

	equal := catalog
	equal.inputImageUSD, equal.cachedInputImageUSD = equal.inputTextUSD, equal.cachedInputTextUSD
	sameDiscount := computeStrictGPTImageTokenQuote(split(500, 500, 300), equal, 1)
	require.False(t, sameDiscount.upperBound, "equal discounts make every split cost the same")
	require.InDelta(t, (700*5.0+300*1.25+100*30.0)*0.5, sameDiscount.quota, 1e-9)
}

// TestStrictGPTImageFinalizeFlagsFallback pins the summary contract directly:
// a strict model without usage, or with an unusable receipt, settles the
// reservation flagged as the configured fallback, while legacy models and a
// zero group multiplier never carry an estimate flag.
func TestStrictGPTImageFinalizeFlagsFallback(t *testing.T) {
	t.Parallel()
	missing := finalizeImageQuota(5_000, true, "gpt-image-2.6-flare", "gpt-image-2.6-flare", nil, 1)
	require.Equal(t, int64(5_000), missing.TotalQuota)
	require.Equal(t, strictImageConfiguredFallbackReason, missing.EstimateReason)

	unusable := finalizeImageQuota(5_000, true, "gpt-image-2.6-flare", "gpt-image-2.6-flare",
		&relaymodel.Usage{PromptTokens: 10, CompletionTokens: 0}, 1)
	require.Equal(t, int64(5_000), unusable.TotalQuota)
	require.Equal(t, strictImageConfiguredFallbackReason, unusable.EstimateReason)

	free := finalizeImageQuota(0, true, "gpt-image-2.6-flare", "gpt-image-2.6-flare", nil, 0)
	require.Empty(t, free.EstimateReason, "a free group settles nothing to flag")
	legacy := finalizeImageQuota(20_000, true, "dall-e-3", "dall-e-3", nil, 1)
	require.Empty(t, legacy.EstimateReason, "legacy per-image billing is exact operator pricing")
}
