package zai

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/zhipu"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/pricing"
)

// TestPricingIsNotInheritedFromZhipu guards the embedding trap that makes Z.AI
// bill at BigModel's CNY rates: zhipu.GetModelRatio and zhipu.GetCompletionRatio
// resolve pricing through a.GetDefaultModelPricing() on the INNER receiver, so
// overriding GetDefaultModelPricing alone is not enough. Asserted through both
// the concrete type and the adaptor.Adaptor interface, because interface dispatch
// does not rescue embedding either.
func TestPricingIsNotInheritedFromZhipu(t *testing.T) {
	t.Parallel()

	const model = "glm-4.7"

	zhipuAdaptor := &zhipu.Adaptor{}
	wantRatio := 0.60 * ratio.MilliTokensUsd
	wantCompletion := 2.20 / 0.60

	for name, a := range map[string]adaptor.Adaptor{
		"concrete":  &Adaptor{},
		"interface": adaptor.Adaptor(&Adaptor{}),
	} {
		t.Run(name, func(t *testing.T) {
			require.InDelta(t, wantRatio, a.GetModelRatio(model), 1e-12)
			require.InDelta(t, wantCompletion, a.GetCompletionRatio(model), 1e-12)

			// The same id must resolve differently on the two brands.
			require.NotEqual(t, zhipuAdaptor.GetModelRatio(model), a.GetModelRatio(model),
				"glm-4.7 must not bill at BigModel's CNY ratio on a Z.AI channel")

			require.Equal(t, len(ModelRatios), len(a.GetModelList()))
			require.NotEqual(t, len(zhipu.ModelRatios), len(a.GetModelList()),
				"Z.AI advertises a smaller catalog than BigModel")

			pricing := a.GetDefaultModelPricing()
			require.InDelta(t, wantRatio, pricing[model].Ratio, 1e-12)
		})
	}
}

// TestNoCNYPricingArtifacts pins that Z.AI's flat price list never carries a
// BigModel tier table, inherited CNY time window, or cache-write rate: Z.AI
// publishes exactly one value per column with no tiering by input or output
// length.
//
// zaiOwnTimeWindows lists the entries allowed to declare a window of their own.
// Those windows must be declared in USD here (via launchPromoUsd), never
// inherited from BigModel, and TestGLM53FlashLaunchPromoWindow pins their values.
func TestNoCNYPricingArtifacts(t *testing.T) {
	t.Parallel()

	zaiOwnTimeWindows := map[string]bool{"glm-5.3-flash": true}

	for name, cfg := range ModelRatios {
		require.Empty(t, cfg.Tiers, "%s must not carry BigModel's input/output tiers", name)
		if !zaiOwnTimeWindows[name] {
			require.Empty(t, cfg.TimeWindows, "%s must not carry time-window pricing", name)
		}
		require.Zero(t, cfg.CacheWrite5mRatio, name)
		require.Zero(t, cfg.CacheWrite1hRatio, name)
		require.Equal(t, strings.ToLower(name), name, "model ids must be lowercase")
	}
}

// TestDeriveBasesResolve exercises every derive() call so a rename on the BigModel
// side fails here rather than silently producing a zero-metadata entry in
// production. Package init already panics on a missing base; this pins the
// resulting metadata actually carried over.
func TestDeriveBasesResolve(t *testing.T) {
	t.Parallel()

	require.NotEmpty(t, ModelRatios)

	// Metadata is inherited from the BigModel entry for the same model.
	glm47 := ModelRatios["glm-4.7"]
	require.Equal(t, zhipu.ModelRatios["glm-4.7"].ContextLength, glm47.ContextLength)
	require.Equal(t, zhipu.ModelRatios["glm-4.7"].MaxOutputTokens, glm47.MaxOutputTokens)
	require.Equal(t, zhipu.ModelRatios["glm-4.7"].InputModalities, glm47.InputModalities)
	require.NotEqual(t, zhipu.ModelRatios["glm-4.7"].Description, glm47.Description,
		"the description must be re-authored for Z.AI")

	// Cloning must not alias BigModel's slices.
	require.NotEmpty(t, zhipu.ModelRatios["glm-4.7"].Tiers,
		"BigModel still tiers glm-4.7; clearing Tiers on the Z.AI clone must not mutate it")
}

// TestUnsupportedSurfacesAbsent pins the catalog boundary. Advertising a model
// Z.AI does not serve would route requests to a 404 and bill at the fallback rate.
func TestUnsupportedSurfacesAbsent(t *testing.T) {
	t.Parallel()

	for _, absent := range []string{
		"embedding-2", "embedding-3", "rerank",
		"glm-tts", "glm-tts-clone",
		"charglm-4", "codegeex-4", "emohaa",
		"glm-4-plus", "glm-4-flash", "glm-4v-plus", "glm-z1-air",
		"autoglm-phone", "autoglm-phone-multilingual",
		"cogview-4", // Z.AI's model enum requires the dated id instead
	} {
		require.NotContains(t, ModelRatios, absent)
	}

	for name := range ModelRatios {
		require.False(t, strings.HasPrefix(name, "glm-realtime"),
			"Z.AI has no realtime surface, found %s", name)
	}
}

// TestKeyPricePins anchors a representative entry from each billing shape against
// Z.AI's published list prices.
func TestKeyPricePins(t *testing.T) {
	t.Parallel()

	// per-token
	require.InDelta(t, 0.60*ratio.MilliTokensUsd, ModelRatios["glm-4.7"].Ratio, 1e-12)
	require.InDelta(t, 0.11*ratio.MilliTokensUsd, ModelRatios["glm-4.7"].CachedInputRatio, 1e-12)
	require.InDelta(t, 1.40*ratio.MilliTokensUsd, ModelRatios["glm-5.3"].Ratio, 1e-12)

	// free
	require.Zero(t, ModelRatios["glm-4.7-flash"].Ratio)
	require.Zero(t, ModelRatios["glm-4.7-flash"].CachedInputRatio)
	require.Zero(t, ModelRatios["glm-4.6v-flash"].Ratio)

	// GLM-5.3-Flash's base price is list; the 50% launch promotion is a window.
	require.InDelta(t, 0.15*ratio.MilliTokensUsd, ModelRatios["glm-5.3-flash"].Ratio, 1e-12)
	require.InDelta(t, 0.50/0.15, ModelRatios["glm-5.3-flash"].CompletionRatio, 1e-12)

	// per-image
	glmImage := ModelRatios["glm-image"]
	require.Zero(t, glmImage.Ratio)
	require.NotNil(t, glmImage.Image)
	require.InDelta(t, 0.015, glmImage.Image.PricePerImageUsd, 1e-12)
	require.InDelta(t, 0.01, ModelRatios["cogview-4-250304"].Image.PricePerImageUsd, 1e-12)

	// per-call video
	cogvideo := ModelRatios["cogvideox-3"]
	require.NotNil(t, cogvideo.PerCall)
	require.InDelta(t, 200.0, cogvideo.PerCall.UsdPerThousandCalls, 1e-9)
	require.InDelta(t, 0.20*ratio.QuotaPerUsd, cogvideo.Ratio, 1e-9)
	require.InDelta(t, 400.0, ModelRatios["viduq1-text"].PerCall.UsdPerThousandCalls, 1e-9)

	// audio: billed per token, with the duration->token estimate preserved
	asr := ModelRatios["glm-asr-2512"]
	require.InDelta(t, 0.03*ratio.MilliTokensUsd, asr.Ratio, 1e-12)
	require.NotNil(t, asr.Audio)
	require.InDelta(t, 10.0, asr.Audio.PromptTokensPerSecond, 1e-9)

	// Z.AI-only id with no BigModel counterpart
	only := ModelRatios["glm-4-32b-0414-128k"]
	require.InDelta(t, 0.10*ratio.MilliTokensUsd, only.Ratio, 1e-12)
	require.InDelta(t, 1.0, only.CompletionRatio, 1e-12)
	require.NotContains(t, zhipu.ModelRatios, "glm-4-32b-0414-128k")
}

// TestToolingDefaultsAreZaiSpecific pins that BigModel's tiered search engines do
// not leak through: Z.AI publishes a single flat $0.01-per-use web search.
func TestToolingDefaultsAreZaiSpecific(t *testing.T) {
	t.Parallel()

	cfg := (&Adaptor{}).DefaultToolingConfig()
	require.Contains(t, cfg.Pricing, "web_search")
	require.InDelta(t, 0.01, cfg.Pricing["web_search"].UsdPerCall, 1e-12)

	for _, bigmodelOnly := range []string{"search_std", "search_pro", "search_pro_sogou", "search_pro_quark"} {
		require.NotContains(t, cfg.Pricing, bigmodelOnly)
	}
}

// TestGLM53FlashLaunchPromoWindow verifies Z.AI's 50% launch promotion is carried
// as a USD time window over the list price -- derive() strips BigModel's CNY
// window, so this asserts the USD overlay was re-attached -- and that it expires
// by itself at 24:00 on 2026-09-09 (UTC+8) without any further code change.
func TestGLM53FlashLaunchPromoWindow(t *testing.T) {
	t.Parallel()

	cfg, ok := ModelRatios["glm-5.3-flash"]
	require.True(t, ok)
	require.Len(t, cfg.TimeWindows, 1)
	require.Equal(t, "glm-5.3-flash-launch-promo", cfg.TimeWindows[0].Name)
	require.Equal(t, "Asia/Shanghai", cfg.TimeWindows[0].TimeZone)
	require.Equal(t, "2026-09-10", cfg.TimeWindows[0].DateTo)

	shanghai, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)

	promo := pricing.ApplyTimeWindow(cfg, time.Date(2026, 9, 9, 23, 59, 0, 0, shanghai))
	require.InDelta(t, 0.075*ratio.MilliTokensUsd, promo.Ratio, 1e-12)
	require.InDelta(t, 0.015*ratio.MilliTokensUsd, promo.CachedInputRatio, 1e-12)
	require.InDelta(t, 0.25*ratio.MilliTokensUsd, promo.Ratio*promo.CompletionRatio, 1e-12)

	expired := pricing.ApplyTimeWindow(cfg, time.Date(2026, 9, 10, 0, 0, 0, 0, shanghai))
	require.InDelta(t, 0.15*ratio.MilliTokensUsd, expired.Ratio, 1e-12)
	require.InDelta(t, 0.03*ratio.MilliTokensUsd, expired.CachedInputRatio, 1e-12)
	require.InDelta(t, 0.50*ratio.MilliTokensUsd, expired.Ratio*expired.CompletionRatio, 1e-12)
}
