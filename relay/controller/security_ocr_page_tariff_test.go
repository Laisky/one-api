package controller

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
)

// TestSecurityOCRPageTariffSettlement proves a per_page tariff bills the
// receipt's typed processed-page count, never its token counters, holds the
// page allowance before dispatch, bounds billable pages by an explicit range,
// labels a missing page receipt as an estimate, and honours a free tariff.
func TestSecurityOCRPageTariffSettlement(t *testing.T) {
	// 10 USD per 1000 pages * 500000 quota/USD = 5000 quota per page.
	pageTariff := map[string]model.ModelConfigLocal{ocrTestModel: {PerPage: &model.PerPagePricingLocal{UsdPerThousandPages: 10}}}
	const perPage = int64(5000)
	tokens := `"usage":{"prompt_tokens":100000,"completion_tokens":40000,"total_tokens":140000}`
	rangeBody := func(start, end string) string {
		return `{"model":"glm-ocr","file":"https://documents.example.test/scan.pdf","start_page_id":` + start + `,"end_page_id":` + end + `}`
	}
	for _, tc := range []struct {
		name, requestID, payload, upstream, reason string
		configs                                    map[string]model.ModelConfigLocal
		held, want                                 int64
	}{
		{name: "measured_pages_ignore_tokens", requestID: "ocr472-page-measured", configs: pageTariff, payload: rangeBody("1", "3"),
			upstream: `{"md_results":"x",` + tokens + `,"data_info":{"num_pages":2}}`, held: 3 * perPage, want: 2 * perPage},
		{name: "explicit_range_bounds_document_total", requestID: "ocr472-page-range", configs: pageTariff, payload: rangeBody("4", "5"),
			upstream: `{"md_results":"x",` + tokens + `,"data_info":{"num_pages":40}}`, held: 2 * perPage, want: 2 * perPage},
		{name: "unknown_document_holds_page_limit", requestID: "ocr472-page-unknown", configs: pageTariff,
			payload:  `{"model":"glm-ocr","file":"https://documents.example.test/unknown"}`,
			upstream: `{"md_results":"x",` + tokens + `,"data_info":{"num_pages":7}}`, held: ocrTestMaxPages * perPage, want: 7 * perPage},
		{name: "missing_page_receipt_is_labelled_estimate", requestID: "ocr472-page-missing", configs: pageTariff, payload: rangeBody("1", "3"),
			upstream: `{"md_results":"x",` + tokens + `}`, held: 3 * perPage, want: 3 * perPage, reason: "ocr_receipt_missing"},
		{name: "explicit_free_page_tariff", requestID: "ocr472-page-free", payload: rangeBody("1", "3"),
			configs:  map[string]model.ModelConfigLocal{ocrTestModel: {PerPage: &model.PerPagePricingLocal{UsdPerThousandPages: 0}}},
			upstream: `{"md_results":"x",` + tokens + `,"data_info":{"num_pages":3}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := runOCRSettlementCase(t, ocrSettlementCase{requestID: tc.requestID, groupRatio: 1, configs: tc.configs,
				payload: tc.payload, upstreamBody: tc.upstream, userQuota: ocrTestLargeQuota, tokenQuota: ocrTestLargeQuota})
			require.Nil(t, res.apiErr)
			require.EqualValues(t, tc.held, res.heldAtDispatch, "page allowance held before dispatch")
			requireOCRSettledOnce(t, res, tc.want)
			require.Equal(t, "page", res.logs[0].Metadata["ocr_billing_unit"])
			if tc.reason != "" {
				requireOCREstimate(t, res, tc.reason)
				return
			}
			requireOCRMeasured(t, res)
		})
	}
}
