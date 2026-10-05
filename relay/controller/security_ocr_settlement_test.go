package controller

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
)

// ocrTestTokenTariff prices input at 2 quota per token and output at 6 quota
// per token (completion ratio 3), so input and output weights are distinct.
var ocrTestTokenTariff = map[string]model.ModelConfigLocal{ocrTestModel: {Ratio: 2, CompletionRatio: 3}}

// ocrTestPageQuote is the conservative single-page allowance for
// ocrTestTokenTariff at group ratio 1: 16384*2 + 4096*6.
const ocrTestPageQuote = int64(ocrTestPageInputTokens*2 + ocrTestPageOutputTokens*6)

// TestSecurityOCRTokenReceiptSettlement proves token-priced native OCR settles
// the provider's measured input/output counters through the token calculator,
// applying distinct input/output prices and the group ratio, for small,
// multi-page and beyond-allowance receipts and for both GLM-OCR catalogs.
func TestSecurityOCRTokenReceiptSettlement(t *testing.T) {
	for _, tc := range []struct {
		name        string
		requestID   string
		channelType int
		configs     map[string]model.ModelConfigLocal
		group       float64
		payload     string
		usage       string
		want        int64
	}{
		// (100*2 + 40*6) * 1.5 = 660; the flat per-request quote was ceil(2*1.5) = 3.
		{name: "small_receipt_distinct_ratios_and_group", requestID: "ocr472-token-small", configs: ocrTestTokenTariff, group: 1.5,
			usage: `{"prompt_tokens":100,"completion_tokens":40,"total_tokens":140}`, want: 660},
		// 50000*2 + 12000*6 = 172000 within a five-page allowance.
		{name: "large_multi_page_receipt", requestID: "ocr472-token-large", configs: ocrTestTokenTariff, group: 1,
			payload: `{"model":"glm-ocr","file":"https://documents.example.test/scan.pdf","start_page_id":1,"end_page_id":5}`,
			usage:   `{"prompt_tokens":50000,"completion_tokens":12000,"total_tokens":62000}`, want: 172000},
		// 150000*2 + 30000*6 = 480000: measured work beyond the admission allowance is still billed.
		{name: "receipt_beyond_allowance_bills_measured", requestID: "ocr472-token-beyond", configs: ocrTestTokenTariff, group: 1,
			usage: `{"prompt_tokens":150000,"completion_tokens":30000,"total_tokens":180000}`, want: 480000},
		// BigModel catalog: 0.2 CNY per 1M tokens => 1200 * 0.2*0.5/7 = 17.14 -> 18.
		{name: "bigmodel_catalog_tariff", requestID: "ocr472-token-bigmodel", channelType: channeltype.Zhipu, group: 1,
			usage: `{"prompt_tokens":800,"completion_tokens":400,"total_tokens":1200}`, want: 18},
		// Z.ai catalog: 0.03 USD per 1M tokens => 1200 * 0.015 = 18.
		{name: "zai_catalog_tariff", requestID: "ocr472-token-zai", channelType: channeltype.Zai, group: 1,
			usage: `{"prompt_tokens":800,"completion_tokens":400,"total_tokens":1200}`, want: 18},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := runOCRSettlementCase(t, ocrSettlementCase{requestID: tc.requestID, channelType: tc.channelType,
				configs: tc.configs, groupRatio: tc.group, payload: tc.payload, upstreamBody: ocrTestReceipt(tc.usage),
				userQuota: ocrTestLargeQuota, tokenQuota: ocrTestLargeQuota})
			require.Nil(t, res.apiErr)
			requireOCRSettledOnce(t, res, tc.want)
			requireOCRMeasured(t, res)
		})
	}
}

// TestSecurityOCRUnitTariffControls proves an explicit per-call tariff bills one
// invocation regardless of nonzero token counters, and that explicit free
// tariffs (zero per-call price or a free group) settle to zero without a hold.
func TestSecurityOCRUnitTariffControls(t *testing.T) {
	usage := ocrTestReceipt(`{"prompt_tokens":100,"completion_tokens":40,"total_tokens":140}`)
	t.Run("per_call_ignores_token_volume", func(t *testing.T) {
		// 2 USD per 1000 calls * 500000 quota/USD * group 1.5 = 1500 per call.
		res := runOCRSettlementCase(t, ocrSettlementCase{requestID: "ocr472-per-call", groupRatio: 1.5,
			configs:      map[string]model.ModelConfigLocal{ocrTestModel: {PerCall: &model.PerCallPricingLocal{UsdPerThousandCalls: 2}}},
			upstreamBody: usage, userQuota: ocrTestLargeQuota, tokenQuota: ocrTestLargeQuota})
		require.Nil(t, res.apiErr)
		require.EqualValues(t, 1500, res.heldAtDispatch, "per-call quote must be held before dispatch")
		requireOCRSettledOnce(t, res, 1500)
		requireOCRMeasured(t, res)
	})
	t.Run("explicit_free_per_call", func(t *testing.T) {
		res := runOCRSettlementCase(t, ocrSettlementCase{requestID: "ocr472-free-call", groupRatio: 1,
			configs:      map[string]model.ModelConfigLocal{ocrTestModel: {PerCall: &model.PerCallPricingLocal{UsdPerThousandCalls: 0}}},
			upstreamBody: usage, userQuota: ocrTestLargeQuota, tokenQuota: ocrTestLargeQuota})
		require.Nil(t, res.apiErr)
		require.Zero(t, res.heldAtDispatch, "a free tariff must not hold quota")
		requireOCRSettledOnce(t, res, 0)
	})
	t.Run("explicit_free_group", func(t *testing.T) {
		res := runOCRSettlementCase(t, ocrSettlementCase{requestID: "ocr472-free-group", groupRatio: 0,
			configs: ocrTestTokenTariff, upstreamBody: usage, userQuota: ocrTestLargeQuota, tokenQuota: ocrTestLargeQuota})
		require.Nil(t, res.apiErr)
		require.Zero(t, res.heldAtDispatch)
		requireOCRSettledOnce(t, res, 0)
	})
}

// TestSecurityOCRAdmissionBeforeDispatch proves the conservative allowance is
// reserved against both the user and the token before the provider is called,
// and that malformed page selections are rejected before any paid work.
func TestSecurityOCRAdmissionBeforeDispatch(t *testing.T) {
	usage := ocrTestReceipt(`{"prompt_tokens":100,"completion_tokens":40,"total_tokens":140}`)
	t.Run("insufficient_user_quota", func(t *testing.T) {
		res := runOCRSettlementCase(t, ocrSettlementCase{requestID: "ocr472-reject-user", groupRatio: 1,
			configs: ocrTestTokenTariff, upstreamBody: usage, userQuota: ocrTestPageQuote - 1, tokenQuota: ocrTestLargeQuota})
		requireOCRRejectedBeforeDispatch(t, res, http.StatusForbidden)
	})
	t.Run("insufficient_token_quota", func(t *testing.T) {
		res := runOCRSettlementCase(t, ocrSettlementCase{requestID: "ocr472-reject-token", groupRatio: 1,
			configs: ocrTestTokenTariff, upstreamBody: usage, userQuota: ocrTestLargeQuota, tokenQuota: ocrTestPageQuote - 1})
		requireOCRRejectedBeforeDispatch(t, res, http.StatusForbidden)
	})
	for _, tc := range []struct{ name, requestID, payload string }{
		{name: "reversed_page_range", requestID: "ocr472-reject-range",
			payload: `{"model":"glm-ocr","file":"https://documents.example.test/scan.pdf","start_page_id":5,"end_page_id":2}`},
		{name: "negative_page", requestID: "ocr472-reject-negative",
			payload: `{"model":"glm-ocr","file":"https://documents.example.test/scan.pdf","start_page_id":-3,"end_page_id":2}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := runOCRSettlementCase(t, ocrSettlementCase{requestID: tc.requestID, groupRatio: 1, configs: ocrTestTokenTariff,
				payload: tc.payload, upstreamBody: usage, userQuota: ocrTestLargeQuota, tokenQuota: ocrTestLargeQuota})
			requireOCRRejectedBeforeDispatch(t, res, http.StatusBadRequest)
		})
	}
}

// TestSecurityOCRConservativeAllowance proves the hold taken before dispatch
// covers the validated document: an explicit page range, a single inline image,
// or the documented 100-page maximum when the page count cannot be known.
func TestSecurityOCRConservativeAllowance(t *testing.T) {
	const pngDataURI = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	for _, tc := range []struct {
		name, requestID, payload string
		pages                    int64
	}{
		{name: "explicit_page_range", requestID: "ocr472-hold-range", pages: 3,
			payload: `{"model":"glm-ocr","file":"https://documents.example.test/scan.pdf","start_page_id":2,"end_page_id":4}`},
		{name: "inline_png_is_one_page", requestID: "ocr472-hold-png", pages: 1,
			payload: `{"model":"glm-ocr","file":"` + pngDataURI + `"}`},
		{name: "remote_document_without_range", requestID: "ocr472-hold-remote", pages: ocrTestMaxPages,
			payload: `{"model":"glm-ocr","file":"https://documents.example.test/unknown"}`},
		{name: "inline_pdf_without_range", requestID: "ocr472-hold-pdf", pages: ocrTestMaxPages,
			payload: `{"model":"glm-ocr","file":"JVBERi0xLjcKJcfsj6IKMSAwIG9iago8PC9UeXBlL0NhdGFsb2c+PgplbmRvYmoK"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := runOCRSettlementCase(t, ocrSettlementCase{requestID: tc.requestID, groupRatio: 1, configs: ocrTestTokenTariff,
				payload: tc.payload, upstreamBody: ocrTestReceipt(`{"prompt_tokens":100,"completion_tokens":40,"total_tokens":140}`),
				userQuota: ocrTestLargeQuota, tokenQuota: ocrTestLargeQuota})
			require.Nil(t, res.apiErr)
			require.EqualValues(t, tc.pages*ocrTestPageQuote, res.heldAtDispatch, "conservative allowance held at dispatch")
			requireOCRSettledOnce(t, res, 100*2+40*6)
			require.Contains(t, res.forwardedBody, `"model":"glm-ocr"`)
		})
	}
}

// TestSecurityOCRUnverifiableReceipts proves missing, invalid, overflowing and
// inconsistent receipts settle exactly once at the conservative allowance with
// an explicit estimate label, never at a flat or fabricated zero-token charge.
func TestSecurityOCRUnverifiableReceipts(t *testing.T) {
	for _, tc := range []struct {
		name, requestID, body, reason string
	}{
		{name: "missing_usage", requestID: "ocr472-rcpt-missing", body: ocrTestReceipt(""), reason: "ocr_receipt_missing"},
		{name: "missing_counter", requestID: "ocr472-rcpt-counter", body: ocrTestReceipt(`{"total_tokens":140}`), reason: "ocr_receipt_missing"},
		{name: "negative_counter", requestID: "ocr472-rcpt-negative",
			body: ocrTestReceipt(`{"prompt_tokens":-100,"completion_tokens":40,"total_tokens":-60}`), reason: "ocr_receipt_invalid"},
		{name: "fractional_counter", requestID: "ocr472-rcpt-fraction",
			body: ocrTestReceipt(`{"prompt_tokens":100.5,"completion_tokens":40,"total_tokens":140.5}`), reason: "ocr_receipt_invalid"},
		{name: "string_counter", requestID: "ocr472-rcpt-string",
			body: ocrTestReceipt(`{"prompt_tokens":"100","completion_tokens":40}`), reason: "ocr_receipt_invalid"},
		{name: "beyond_int64_counter", requestID: "ocr472-rcpt-int64",
			body: ocrTestReceipt(`{"prompt_tokens":99999999999999999999,"completion_tokens":40}`), reason: "ocr_receipt_overflow"},
		{name: "implausible_counter", requestID: "ocr472-rcpt-huge",
			body: ocrTestReceipt(`{"prompt_tokens":9000000000000,"completion_tokens":40}`), reason: "ocr_receipt_overflow"},
		{name: "inconsistent_total", requestID: "ocr472-rcpt-total",
			body: ocrTestReceipt(`{"prompt_tokens":100,"completion_tokens":40,"total_tokens":99999}`), reason: "ocr_receipt_inconsistent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := runOCRSettlementCase(t, ocrSettlementCase{requestID: tc.requestID, groupRatio: 1, configs: ocrTestTokenTariff,
				upstreamBody: tc.body, userQuota: ocrTestLargeQuota, tokenQuota: ocrTestLargeQuota})
			require.Equal(t, int32(1), res.dispatches)
			requireOCRSettledOnce(t, res, ocrTestPageQuote)
			requireOCREstimate(t, res, tc.reason)
		})
	}
	t.Run("non_json_success_body", func(t *testing.T) {
		res := runOCRSettlementCase(t, ocrSettlementCase{requestID: "ocr472-rcpt-garbage", groupRatio: 1, configs: ocrTestTokenTariff,
			upstreamBody: "upstream proxy page", userQuota: ocrTestLargeQuota, tokenQuota: ocrTestLargeQuota})
		require.NotNil(t, res.apiErr, "an undecodable provider response is not forwarded as success")
		requireOCRSettledOnce(t, res, ocrTestPageQuote)
		requireOCREstimate(t, res, "ocr_receipt_unreadable")
	})
}

// TestSecurityOCRDeliveryFailureSettlesAcceptedWork proves work accepted by the
// provider is settled exactly once from its receipt even when the response can
// no longer be delivered to the client.
func TestSecurityOCRDeliveryFailureSettlesAcceptedWork(t *testing.T) {
	t.Run("measured_receipt", func(t *testing.T) {
		res := runOCRSettlementCase(t, ocrSettlementCase{requestID: "ocr472-deliver-measured", groupRatio: 1.5,
			configs: ocrTestTokenTariff, upstreamBody: ocrTestReceipt(`{"prompt_tokens":100,"completion_tokens":40,"total_tokens":140}`),
			userQuota: ocrTestLargeQuota, tokenQuota: ocrTestLargeQuota, failDelivery: true})
		require.NotNil(t, res.apiErr, "the delivery failure must be reported")
		requireOCRSettledOnce(t, res, 660)
		requireOCRMeasured(t, res)
	})
	t.Run("missing_receipt", func(t *testing.T) {
		res := runOCRSettlementCase(t, ocrSettlementCase{requestID: "ocr472-deliver-missing", groupRatio: 1,
			configs: ocrTestTokenTariff, upstreamBody: ocrTestReceipt(""),
			userQuota: ocrTestLargeQuota, tokenQuota: ocrTestLargeQuota, failDelivery: true})
		require.NotNil(t, res.apiErr)
		requireOCRSettledOnce(t, res, ocrTestPageQuote)
		requireOCREstimate(t, res, "ocr_receipt_missing")
	})
}
