package test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	dbmodel "github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/quota"
	"github.com/Laisky/one-api/relay/relaymode"
)

// nativeOnlyOpenAIModelIDs lists the OpenAI IDs whose tariff the 2026-11-16
// audit verified only for OpenAI's first-party API. The production predicate
// is checked against this independent list so a shrinking policy is visible.
var nativeOnlyOpenAIModelIDs = []string{"gpt-6-cyber", "gpt-image-2.6-sunburst", "gpt-image-2.6-flare"}

// inheritingOpenAIHosts are channels that inherit OpenAI's catalog but publish
// no independent contract for the native-only IDs above.
var inheritingOpenAIHosts = []int{channeltype.Azure, channeltype.AIProxyLibrary}

// TestNativeOnlyModelsNeverBorrowGlobalTariff drives the real three-layer
// resolver with the production global pricing manager. A host that has no
// provider-qualified tariff for a model must not inherit native OpenAI's
// price through the global layer, every resolution API must agree, and an
// operator-configured channel tariff must remain authoritative.
func TestNativeOnlyModelsNeverBorrowGlobalTariff(t *testing.T) {
	pricing.InitializeGlobalPricingManager(relay.GetAdaptor)
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

	native := pricingAdaptorForChannel(channeltype.OpenAI)
	require.NotNil(t, native)
	nativeCfg, found := pricing.ResolveModelConfigRatioOnly("gpt-6-cyber", nil, native, now)
	require.True(t, found, "control: native OpenAI must keep its published tariff")
	require.InDelta(t, 20*billingratio.MilliTokensUsd, nativeCfg.Ratio, 1e-12)
	_, globallyPriced := pricing.GetGlobalModelConfig("gpt-6-cyber")
	require.True(t, globallyPriced, "control: the global layer must contain the native tariff this test guards against")

	operator := map[string]dbmodel.ModelConfigLocal{}
	for _, name := range nativeOnlyOpenAIModelIDs {
		operator[name] = dbmodel.ModelConfigLocal{Ratio: 3 * billingratio.MilliTokensUsd, CompletionRatio: 2}
	}

	for _, channelType := range inheritingOpenAIHosts {
		provider := pricingAdaptorForChannel(channelType)
		require.NotNil(t, provider, "channel type %d", channelType)
		for _, name := range nativeOnlyOpenAIModelIDs {
			require.True(t, openai.IsNativeOnlyModel(name), name)
			require.True(t, adaptor.RequiresOperatorTariff(provider, name), "%s on %s", name, provider.GetChannelName())
			require.NotContains(t, provider.GetModelList(), name)
			require.NotContains(t, provider.GetDefaultModelPricing(), name)

			_, found := pricing.ResolveModelConfigRatioOnly(name, nil, provider, now)
			require.False(t, found, "%s on %s must not resolve another provider's tariff", name, provider.GetChannelName())
			_, found = pricing.ResolveModelConfig(name, nil, provider, now)
			require.False(t, found, "%s on %s full resolver", name, provider.GetChannelName())
			_, found = pricing.ResolveImagePricing(name, nil, provider, now)
			require.False(t, found, "%s on %s image resolver", name, provider.GetChannelName())
			require.Nil(t, pricing.GetVideoPricingWithThreeLayers(name, nil, provider))

			// Every scalar resolution API must give the same answer.
			require.Equal(t, provider.GetModelRatio(name), pricing.ResolveModelRatioAt(name, nil, nil, provider, now),
				"%s on %s: ResolveModelRatioAt disagrees with the adaptor", name, provider.GetChannelName())
			require.Equal(t, provider.GetCompletionRatio(name), pricing.ResolveCompletionRatioAt(name, nil, nil, provider, now),
				"%s on %s: ResolveCompletionRatioAt disagrees with the adaptor", name, provider.GetChannelName())
			require.Equal(t, provider.GetModelRatio(name), pricing.GetModelRatioWithThreeLayers(name, nil, provider))
			require.Equal(t, provider.GetCompletionRatio(name), pricing.GetCompletionRatioWithThreeLayers(name, nil, provider))

			// The operator's channel tariff stays the first, authoritative layer.
			cfg, found := pricing.ResolveModelConfigRatioOnly(name, operator, provider, now)
			require.True(t, found)
			require.InDelta(t, 3*billingratio.MilliTokensUsd, cfg.Ratio, 1e-12)
			require.InDelta(t, 2, cfg.CompletionRatio, 1e-12)
			require.InDelta(t, 3*billingratio.MilliTokensUsd, pricing.ResolveModelRatioAt(name, operator, nil, provider, now), 1e-12)
			require.InDelta(t, 2, pricing.ResolveCompletionRatioAt(name, operator, nil, provider, now), 1e-12)
		}

		// Other inherited OpenAI models keep their historical global/provider pricing.
		cfg, found := pricing.ResolveModelConfigRatioOnly("gpt-4o-mini", nil, provider, now)
		require.True(t, found)
		require.Equal(t, openai.ModelRatios["gpt-4o-mini"].Ratio, cfg.Ratio)
		require.False(t, adaptor.RequiresOperatorTariff(provider, "gpt-4o-mini"))
	}
}

// TestNativeOnlyModelsRequireOperatorTariffBeforeDispatch proves that an
// inheriting host never contacts its upstream for a native-only model unless
// the channel carries a complete operator tariff, and that a priced channel
// is still forwarded. The fake upstream counts real HTTP calls.
func TestNativeOnlyModelsRequireOperatorTariffBeforeDispatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Adapter-level dispatch has no tracing database; keep real HTTP enabled.
	previousSinks := config.TraceSinks
	config.TraceSinks = []string{config.TraceSinkNone}
	t.Cleanup(func() { config.TraceSinks = previousSinks })
	for _, channelType := range inheritingOpenAIHosts {
		for _, name := range nativeOnlyOpenAIModelIDs {
			for _, priced := range []bool{false, true} {
				t.Run(channeltype.IdToName(channelType)+"/"+name+"/priced="+map[bool]string{false: "no", true: "yes"}[priced], func(t *testing.T) {
					var calls atomic.Int32
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						calls.Add(1)
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write([]byte(`{}`))
					}))
					t.Cleanup(upstream.Close)

					channel := &dbmodel.Channel{Id: 1, Type: channelType, Name: "operator-tariff-fixture"}
					if priced {
						require.NoError(t, channel.SetModelPriceConfigs(map[string]dbmodel.ModelConfigLocal{
							name: {Ratio: 3 * billingratio.MilliTokensUsd, CompletionRatio: 2},
						}))
					}
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
					c.Set(ctxkey.ChannelModel, channel)
					m := &meta.Meta{ChannelType: channelType, APIType: channeltype.ToAPIType(channelType),
						Mode: relaymode.ChatCompletions, OriginModelName: "alias", ActualModelName: name,
						BaseURL: upstream.URL, APIKey: "fixture", StartTime: time.Now().UTC(),
						Config: dbmodel.ChannelConfig{APIVersion: "2025-04-01-preview"}}
					provider := relay.GetAdaptor(m.APIType)
					require.NotNil(t, provider)
					provider.Init(m)

					resp, err := provider.DoRequest(c, m, strings.NewReader(`{"model":"`+name+`"}`))
					if resp != nil && resp.Body != nil {
						_ = resp.Body.Close()
					}
					if !priced {
						require.Error(t, err)
						require.Contains(t, err.Error(), "explicit positive input and output pricing")
						require.Zero(t, calls.Load(), "an unpriced native-only model must never reach the upstream")
						return
					}
					require.NoError(t, err)
					require.EqualValues(t, 1, calls.Load(), "an operator-priced deployment must still be forwarded")
				})
			}
		}
	}
}

// requireOperatorTariffEnforced proves that provider rejects modelName before
// dispatch when the channel carries no tariff for it, and that no provider or
// global layer resolves a substitute price. Parameters: t reports failures,
// channelType and provider identify the host, modelName the mapped model, and
// now the request time.
func requireOperatorTariffEnforced(t *testing.T, channelType int, provider adaptor.Adaptor, modelName string, now time.Time) {
	t.Helper()
	validator, ok := provider.(adaptor.RequestPricingValidator)
	require.True(t, ok, "%s declares an operator tariff for %s but cannot reject unpriced dispatch", provider.GetChannelName(), modelName)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(ctxkey.ChannelModel, &dbmodel.Channel{Id: 1, Type: channelType})
	err := validator.ValidateRequestPricing(c, &meta.Meta{ChannelType: channelType, ActualModelName: modelName,
		OriginModelName: modelName, StartTime: now})
	require.Error(t, err, "%s must not dispatch %s without an operator tariff", provider.GetChannelName(), modelName)
	_, found := pricing.ResolveModelConfigRatioOnly(modelName, nil, provider, now)
	require.False(t, found, "%s on %s must not resolve a borrowed tariff", modelName, provider.GetChannelName())
}

// TestOperatorTariffPolicyImpliesDispatchValidation guards the policy contract
// for every registered channel: a provider that declares operator-only models
// must also implement the pre-dispatch validator that enforces it.
func TestOperatorTariffPolicyImpliesDispatchValidation(t *testing.T) {
	declared := 0
	for channelType := 1; channelType < channeltype.Dummy; channelType++ {
		provider := pricingAdaptorForChannel(channelType)
		if provider == nil {
			continue
		}
		if _, ok := provider.(adaptor.OperatorTariffPolicy); !ok {
			continue
		}
		declared++
		_, ok := provider.(adaptor.RequestPricingValidator)
		require.True(t, ok, "channel type %d (%s) declares OperatorTariffPolicy without RequestPricingValidator",
			channelType, provider.GetChannelName())
	}
	require.Positive(t, declared, "control: at least one provider must declare the policy")
}

// TestOperatorTariffAdmissionKeepsPublishedOutputBound proves that hiding a
// borrowed tariff does not also hide the model's published output bound: a
// tiered operator tariff without max_tokens must still be held against the
// upstream limit instead of failing admission or being underfunded.
func TestOperatorTariffAdmissionKeepsPublishedOutputBound(t *testing.T) {
	pricing.InitializeGlobalPricingManager(relay.GetAdaptor)
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	provider := pricingAdaptorForChannel(channeltype.Azure)
	require.True(t, adaptor.RequiresOperatorTariff(provider, "gpt-6-cyber"))
	operator := map[string]dbmodel.ModelConfigLocal{"gpt-6-cyber": {
		Ratio: 2 * billingratio.MilliTokensUsd, CompletionRatio: 4,
		Tiers: []dbmodel.ModelRatioTierLocal{{Ratio: 4 * billingratio.MilliTokensUsd, CompletionRatio: 4, InputTokenThreshold: 1000}},
	}}
	quote, tiered, err := quota.EstimateTierAdmission(quota.ComputeInput{
		Usage: &relaymodel.Usage{PromptTokens: 10}, ModelName: "gpt-6-cyber", GroupRatio: 1,
		ChannelModelConfigs: operator, PricingAdaptor: provider, RequestTime: now,
	}, 0, 0, quota.AdmissionOptions{})
	require.NoError(t, err, "the published 128K output bound must remain available")
	require.True(t, tiered)
	// 10 prompt tokens at $2/M plus 128,000 output tokens at $8/M, in quota units.
	require.EqualValues(t, int64(10+128_000*4), quote)
}
