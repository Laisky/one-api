package openai

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// TestModelBillingPoliciesMatchCatalog keeps every policy entry anchored to the
// explicit catalog it qualifies. Parameter t reports failures; nothing returns.
func TestModelBillingPoliciesMatchCatalog(t *testing.T) {
	t.Parallel()
	require.NotEmpty(t, modelBillingPolicies)
	for name, policy := range modelBillingPolicies {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, normalizedModelName(name), name, "policy keys are normalized model IDs")
			cfg, cataloged := ModelRatios[name]
			require.True(t, cataloged, "every policy entry must exist in ModelRatios")
			require.Equal(t, IsNativeOnlyModel(name), policy.NativeTariffOnly,
				"native tariff scoping must agree with the native-only catalog")
			for _, tier := range policy.SupportedServiceTiers {
				require.Equal(t, strings.ToLower(strings.TrimSpace(tier)), tier)
				require.NotEmpty(t, tier)
			}
			if policy.CacheWriteUnpricedFromInputTokens == 0 {
				return
			}
			require.Positive(t, cfg.CacheWrite5mRatio, "an unpublished tier write rate would otherwise inherit this base rate")
			var referenced []adaptor.ModelRatioTier
			for _, tier := range cfg.Tiers {
				if tier.InputTokenThreshold == policy.CacheWriteUnpricedFromInputTokens {
					referenced = append(referenced, tier)
				}
			}
			require.Len(t, referenced, 1, "the referenced long-context tier must exist in the catalog")
			require.Zero(t, referenced[0].OutputTokenThreshold, "the guard compares input tokens only")
			require.Zero(t, referenced[0].CacheWrite5mRatio, "a published tier write rate needs no unpriced guard")
			require.Zero(t, referenced[0].CacheWrite1hRatio)
		})
	}
}

// TestModelBillingPolicyPredicates checks the exported policy predicates across
// policy and ordinary models, model-ID spellings and channel types. Parameter
// t reports failures; nothing returns.
func TestModelBillingPolicyPredicates(t *testing.T) {
	t.Parallel()
	type expectation struct {
		declared, pinned, httpOnly, capture, nativeOnly bool
		unpricedWriteFrom                               int
	}
	terra := expectation{declared: true, pinned: true, httpOnly: true}
	gpt62 := expectation{declared: true, pinned: true, httpOnly: true, capture: true, nativeOnly: true}
	cyber := expectation{declared: true, pinned: true, httpOnly: true, capture: true, nativeOnly: true,
		unpricedWriteFrom: gpt6CyberLongContextTier.InputTokenThreshold}
	modelCases := []struct {
		model string
		want  expectation
	}{
		{"gpt-6.2-astra", gpt62},
		{" GPT-6.2-SOL ", gpt62},
		{"gpt-6.1-terra", terra},
		{" GPT-6.1-Terra ", terra},
		{"gpt-6-cyber", cyber},
		{"\tGPT-6-CYBER\n", cyber},
		{"gpt-6-sol", expectation{}},
		{"gpt-6.1-sol", expectation{}},
		{"gpt-5.6-cyber", expectation{}},
		{"gpt-6-cyber-preview", expectation{}},
		{"gpt-6.2-sol-2026-11-17", expectation{}},
		{"", expectation{}},
	}
	channelCases := []struct {
		name        string
		channelType int
		native      bool
	}{
		{"openai", channeltype.OpenAI, true},
		{"unset", 0, false},
		{"azure", channeltype.Azure, false},
		{"openai_compatible", channeltype.OpenAICompatible, false},
		{"aiproxy", channeltype.AIProxy, false},
		{"groq", channeltype.Groq, false},
		{"openrouter", channeltype.OpenRouter, false},
	}
	require.Equal(t, 272_001, cyber.unpricedWriteFrom)

	for _, modelCase := range modelCases {
		catalog, declared := CatalogModelBillingPolicy(modelCase.model)
		require.Equal(t, modelCase.want.declared, declared, modelCase.model)
		installed, installedDeclared := adaptor.LookupModelBillingPolicy(modelCase.model)
		require.Equal(t, declared, installedDeclared, "quota sees the same catalog: %q", modelCase.model)
		require.Equal(t, catalog, installed, modelCase.model)
		require.Equal(t, modelCase.want.nativeOnly, catalog.NativeTariffOnly, modelCase.model)
		require.Equal(t, modelCase.want.unpricedWriteFrom, catalog.CacheWriteUnpricedFromInputTokens, modelCase.model)
		if modelCase.want.unpricedWriteFrom > 0 {
			require.True(t, catalog.HasCacheWriteTariff(modelCase.want.unpricedWriteFrom-1), modelCase.model)
			require.False(t, catalog.HasCacheWriteTariff(modelCase.want.unpricedWriteFrom), modelCase.model)
		} else {
			require.True(t, catalog.HasCacheWriteTariff(1_050_000), modelCase.model)
		}

		for _, channelCase := range channelCases {
			name := channelCase.name + "/" + modelCase.model
			policy, nativeDeclared := NativeModelBillingPolicy(channelCase.channelType, modelCase.model)
			enforced := channelCase.native && modelCase.want.declared
			require.Equal(t, enforced, nativeDeclared, name)
			require.Equal(t, enforced && modelCase.want.pinned, policy.RestrictsServiceTier(), name)
			require.Equal(t, enforced && modelCase.want.httpOnly, !policy.PermitsWebSocket(), name)
			require.Equal(t, enforced && modelCase.want.capture, CapturesRawUsageReceipt(channelCase.channelType, modelCase.model), name)
			if !enforced {
				require.Empty(t, policy.PinnedServiceTier(), name)
				require.True(t, policy.SupportsServiceTier("fast"), name)
				continue
			}
			require.Equal(t, "default", policy.PinnedServiceTier(), name)
			for _, tier := range []string{"default", " DEFAULT ", "Default"} {
				require.True(t, policy.SupportsServiceTier(tier), "%s tier %q", name, tier)
			}
			for _, tier := range []string{"", " ", "auto", "fast", "flex", "priority", "scale"} {
				require.False(t, policy.SupportsServiceTier(tier), "%s tier %q", name, tier)
			}
		}
	}
}

// TestModelBillingPolicyLookupReturnsPrivateCopy keeps callers from mutating the
// shared catalog through a returned policy. Parameter t reports failures.
func TestModelBillingPolicyLookupReturnsPrivateCopy(t *testing.T) {
	t.Parallel()
	policy, declared := CatalogModelBillingPolicy("gpt-6.1-terra")
	require.True(t, declared)
	require.NotEmpty(t, policy.SupportedServiceTiers)
	policy.SupportedServiceTiers[0] = "fast"
	again, declared := CatalogModelBillingPolicy("gpt-6-cyber")
	require.True(t, declared)
	require.Equal(t, []string{"default"}, again.SupportedServiceTiers)
	require.Equal(t, []string{"default"}, standardServiceTierOnly)
}

// TestModelBillingPolicyServiceTierRequestScope checks request admission over
// policy models, spellings and channel types. Parameter t reports failures.
func TestModelBillingPolicyServiceTierRequestScope(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		channelType int
		model       string
		requested   *string
		wantTier    *string
		wantErr     bool
	}{
		{"native_astra62_omitted", channeltype.OpenAI, "gpt-6.2-astra", nil, stringRef("default"), false},
		{"native_sol62_fast", channeltype.OpenAI, "gpt-6.2-sol", stringRef("fast"), nil, true},
		{"azure_astra62_fast", channeltype.Azure, "gpt-6.2-astra", stringRef("fast"), stringRef("fast"), false},
		{"native_terra_omitted", channeltype.OpenAI, " GPT-6.1-TERRA ", nil, stringRef("default"), false},
		{"native_cyber_upper_default", channeltype.OpenAI, "gpt-6-cyber", stringRef(" DEFAULT "), stringRef("default"), false},
		{"native_cyber_fast", channeltype.OpenAI, "GPT-6-Cyber", stringRef("fast"), nil, true},
		{"native_terra_empty", channeltype.OpenAI, "gpt-6.1-terra", stringRef(""), nil, true},
		{"native_sol_fast", channeltype.OpenAI, "gpt-6-sol", stringRef("fast"), stringRef("fast"), false},
		{"native_sol_omitted", channeltype.OpenAI, "gpt-6-sol", nil, nil, false},
		{"azure_terra_fast", channeltype.Azure, "gpt-6.1-terra", stringRef("fast"), stringRef("fast"), false},
		{"compatible_cyber_fast", channeltype.OpenAICompatible, "gpt-6-cyber", stringRef("fast"), stringRef("fast"), false},
		{"compatible_terra_omitted", channeltype.OpenAICompatible, "gpt-6.1-terra", nil, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &Adaptor{ChannelType: tc.channelType}
			request := &model.GeneralOpenAIRequest{Model: "customer-alias", ServiceTier: tc.requested}
			info := &meta.Meta{ChannelType: tc.channelType, ActualModelName: tc.model}
			err := provider.applyRequestTransformations(info, request)
			if tc.wantErr {
				require.ErrorContains(t, err, "Standard billing does not support service_tier")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantTier, request.ServiceTier)
		})
	}
}
