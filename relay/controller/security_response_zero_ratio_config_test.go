package controller

import (
	"net/http"
	"testing"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/state"
	"github.com/stretchr/testify/require"
)

// TestSecurityResponseZeroRatioConfiguration distinguishes an omitted modern
// ratio and an unused legacy field from an authoritative free group. Parameters:
// t owns isolated pricing and parent state. Returns: none; paid fallback pricing
// must not accidentally bypass the inherited-context guard as a free request.
func TestSecurityResponseZeroRatioConfiguration(t *testing.T) {
	const name = "native-zero-ratio-continuation-fixture"
	for _, scenario := range []string{"omitted_ratio", "legacy_zero", "free_group"} {
		t.Run(scenario, func(t *testing.T) {
			local := model.ModelConfigLocal{CompletionRatio: 2, Tiers: []model.ModelRatioTierLocal{{
				InputTokenThreshold: 100, CompletionRatio: 3,
			}}}
			group := 1.0
			if scenario == "free_group" {
				group = 0
			}
			c, _, _ := protocolContext(t, channeltype.OpenAICompatible, name, "/v1/responses", "{}", "", 10000, group, false, &local)
			if scenario == "legacy_zero" {
				channel := c.MustGet(ctxkey.ChannelModel).(*model.Channel)
				legacy := `{"native-zero-ratio-continuation-fixture":0}`
				channel.ModelRatio = &legacy
			}
			ratios, configs := getChannelModelPricingFromContext(c)
			_, explicit := ratios[name]
			require.False(t, explicit, "a zero modern ratio is omitted, not a scalar override")
			require.Len(t, configs[name].Tiers, 1, "the configured tier contract remains active")
			meta := metalib.GetByContext(c)
			meta.OriginModelName, meta.ActualModelName = name, name
			memory := enableStateForTest(t)
			observed := &continuationLookupStore{ResponseStateStore: memory}
			state.SetForTest(observed)
			output := 20
			request := &openai.ResponseAPIRequest{Model: name, MaxOutputTokens: &output}
			for _, prompt := range []int{0, 10, 99, 100, 101, 1000000} {
				quote, applies, err := quoteResponseTierAdmission(c, meta, request, prompt, nil)
				require.NoError(t, err)
				require.True(t, applies)
				if group == 0 {
					require.Zero(t, quote)
				} else {
					require.Positive(t, quote, "the actual resolved tariff must be established before testing continuation policy")
				}
			}
			previous := "provider_parent"
			request.PreviousResponseId = &previous
			c.Set(ctxNativeGatewayParent, "owned_parent")
			quote, applies, err := quoteResponseTierAdmission(c, meta, request, 10, nil)
			require.True(t, applies)
			require.Zero(t, quote)
			if group == 0 {
				require.NoError(t, err)
				require.Zero(t, observed.reads, "authoritative free groups need no new parent usage lookup")
			} else {
				require.ErrorIs(t, err, errResponseContinuationUnpriceable)
				require.Equal(t, 1, observed.reads)
				held, apiErr := preConsumeResponseAPIQuota(c, request, 10, 1, 2, false, meta)
				require.Zero(t, held)
				require.NotNil(t, apiErr)
				require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
				require.Zero(t, c.GetInt64(ctxkey.PreConsumedQuotaAmount))
			}
		})
	}
}
