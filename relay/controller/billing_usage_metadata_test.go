package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	relaymodel "github.com/Laisky/one-api/relay/model"
	quotautil "github.com/Laisky/one-api/relay/quota"
)

// TestBillingLogMetadataCopiesInput checks that the consume-log metadata
// helpers label a copy, never the caller's map, accept a nil map, and return
// the input unchanged when they have nothing to add.
func TestBillingLogMetadataCopiesInput(t *testing.T) {
	for _, input := range []model.LogMetadata{nil, {"cache_write_5m_tokens": 7}} {
		original := map[string]any(nil)
		if input != nil {
			original = map[string]any{"cache_write_5m_tokens": 7}
		}

		estimated := billingEstimateMetadata(input, "response_usage_capture_limit")
		require.Equal(t, true, estimated["billing_estimated"])
		require.Equal(t, true, estimated[model.LogMetadataKeyEstimatedCharge])
		require.Equal(t, "response_usage_capture_limit", estimated["billing_estimate_reason"])
		require.Equal(t, len(input)+3, len(estimated), "the copy keeps every input entry")

		usage := &relaymodel.Usage{UpstreamCostUSD: 1.25}
		floored := reportedCostFloorMetadata(input, usage, quotautil.ComputeResult{ReportedCostFloor: true})
		require.Equal(t, "provider_reported_cost", floored["billing_source"])
		require.InDelta(t, 1.25, floored["upstream_cost_usd"], 1e-12)
		require.Equal(t, len(input)+2, len(floored))

		require.Equal(t, model.LogMetadata(original), input, "the caller's map must not be mutated")
		require.Equal(t, input, billingEstimateMetadata(input, ""))
		require.Equal(t, input, reportedCostFloorMetadata(input, usage, quotautil.ComputeResult{}))
	}
}
