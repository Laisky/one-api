package controller

import (
	"context"
	"math"
	"testing"

	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/stretchr/testify/require"
)

// TestCohereSearchUnitsQuota checks exact decimal rounding and bounded arithmetic.
func TestCohereSearchUnitsQuota(t *testing.T) {
	for _, tc := range []struct {
		name       string
		units      int64
		rate, group float64
		want       int64
		valid      bool
	}{
		{name: "measured", units: 3, rate: 1000, group: 2, want: 6000, valid: true},
		{name: "single_ceiling", units: 3, rate: 0.1, group: 2, want: 1, valid: true},
		{name: "decimal_integral", units: 10, rate: 0.1, group: 1, want: 1, valid: true},
		{name: "largest_unit_count", units: math.MaxInt64, rate: 1, group: 1, want: math.MaxInt64, valid: true},
		{name: "overflow", units: math.MaxInt64, rate: 2, group: 1},
		{name: "float_overflow", units: 1, rate: math.MaxFloat64, group: 2},
		{name: "tiny_positive", units: 1, rate: math.SmallestNonzeroFloat64, group: 1, want: 1, valid: true},
		{name: "free_operator", units: 100, group: 1, valid: true},
		{name: "free_group", units: 100, rate: 1000, valid: true},
		{name: "zero_units", rate: 1000, group: 1},
		{name: "negative_units", units: -1, rate: 1000, group: 1},
		{name: "negative_rate", units: 1, rate: -1, group: 1},
		{name: "nan_rate", units: 1, rate: math.NaN(), group: 1},
		{name: "infinite_rate", units: 1, rate: math.Inf(1), group: 1},
		{name: "negative_group", units: 1, rate: 1, group: -1},
		{name: "nan_group", units: 1, rate: 1, group: math.NaN()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, valid := cohereSearchUnitsQuota(tc.units, tc.rate, tc.group)
			require.Equal(t, tc.valid, valid)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestCohereRerankMeterIsolation keeps search receipts from changing other pricing contracts.
func TestCohereRerankMeterIsolation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		channel int
		perCall bool
		units   int64
		want    int64
	}{
		{name: "cohere_search", channel: channeltype.Cohere, perCall: true, units: 3, want: 6},
		{name: "other_per_call", channel: channeltype.OpenAI, perCall: true, units: 3, want: 1000},
		{name: "cohere_token", channel: channeltype.Cohere, units: 3, want: 84},
		{name: "cohere_token_invalid_search", channel: channeltype.Cohere, units: 0, want: 84},
		{name: "other_token", channel: channeltype.OpenAI, units: 3, want: 84},
	} {
		t.Run(tc.name, func(t *testing.T) {
			usage := &relaymodel.Usage{PromptTokens: 42, BilledSearchUnits: &tc.units}
			meta := &metalib.Meta{ChannelType: tc.channel}
			got := postConsumeRerankQuota(context.Background(), usage, meta,
				&relaymodel.RerankRequest{Model: "synthetic"}, 0, 1000, 2, 1, tc.perCall)
			require.Equal(t, tc.want, got)
			require.Empty(t, usage.BillingEstimateReason)
		})
	}
}
