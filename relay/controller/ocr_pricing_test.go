package controller

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestQuoteOCRAllowance verifies the document selection admitted before
// dispatch: explicit ranges (capped at the provider limit), inline PNG/JPEG
// images as one page, everything else at the documented page limit, and
// rejection of negative or reversed ranges.
func TestQuoteOCRAllowance(t *testing.T) {
	t.Parallel()
	page := func(v int) *int { return &v }
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	const jpeg = "/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE="
	const pdf = "JVBERi0xLjcKJcfsj6IKMSAwIG9iago8PC9UeXBlL0NhdGFsb2c+PgplbmRvYmoK"
	for _, tc := range []struct {
		name       string
		request    relaymodel.OCRRequest
		pages      int
		source     string
		wantErr    bool
		wantExplic bool
	}{
		{name: "range", request: relaymodel.OCRRequest{File: "https://x.test/a.pdf", StartPageID: page(2), EndPageID: page(4)}, pages: 3, source: ocrAllowanceFromPageRange, wantExplic: true},
		{name: "zero_based_single", request: relaymodel.OCRRequest{File: "https://x.test/a.pdf", StartPageID: page(0), EndPageID: page(0)}, pages: 1, source: ocrAllowanceFromPageRange, wantExplic: true},
		{name: "range_capped", request: relaymodel.OCRRequest{File: "https://x.test/a.pdf", StartPageID: page(1), EndPageID: page(500)}, pages: ocrMaxDocumentPages, source: ocrAllowanceFromPageRange, wantExplic: true},
		{name: "start_only", request: relaymodel.OCRRequest{File: "https://x.test/a.pdf", StartPageID: page(3)}, pages: ocrMaxDocumentPages, source: ocrAllowanceFromMaxPages},
		{name: "end_only", request: relaymodel.OCRRequest{File: "https://x.test/a.pdf", EndPageID: page(3)}, pages: ocrMaxDocumentPages, source: ocrAllowanceFromMaxPages},
		{name: "png_data_uri", request: relaymodel.OCRRequest{File: "data:image/png;base64," + png}, pages: 1, source: ocrAllowanceFromInlineImage},
		{name: "jpeg_raw_base64", request: relaymodel.OCRRequest{File: jpeg}, pages: 1, source: ocrAllowanceFromInlineImage},
		{name: "image_url_is_not_trusted", request: relaymodel.OCRRequest{File: "https://x.test/photo.png"}, pages: ocrMaxDocumentPages, source: ocrAllowanceFromMaxPages},
		{name: "pdf_raw_base64", request: relaymodel.OCRRequest{File: pdf}, pages: ocrMaxDocumentPages, source: ocrAllowanceFromMaxPages},
		{name: "pdf_bytes_behind_image_mime", request: relaymodel.OCRRequest{File: "data:image/png;base64," + pdf}, pages: ocrMaxDocumentPages, source: ocrAllowanceFromMaxPages},
		{name: "image_bytes_behind_pdf_mime", request: relaymodel.OCRRequest{File: "data:application/pdf;base64," + png}, pages: ocrMaxDocumentPages, source: ocrAllowanceFromMaxPages},
		{name: "short_garbage", request: relaymodel.OCRRequest{File: "abc"}, pages: ocrMaxDocumentPages, source: ocrAllowanceFromMaxPages},
		{name: "negative_start", request: relaymodel.OCRRequest{File: "https://x.test/a.pdf", StartPageID: page(-1), EndPageID: page(2)}, wantErr: true},
		{name: "negative_end_only", request: relaymodel.OCRRequest{File: "https://x.test/a.pdf", EndPageID: page(-2)}, wantErr: true},
		{name: "reversed", request: relaymodel.OCRRequest{File: "https://x.test/a.pdf", StartPageID: page(5), EndPageID: page(4)}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			allowance, err := quoteOCRAllowance(&tc.request)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.pages, allowance.pages)
			require.Equal(t, tc.source, allowance.source)
			require.Equal(t, tc.wantExplic, allowance.explicitRange)
			require.Equal(t, tc.pages*ocrAllowanceInputTokensPerPage, allowance.inputTokens)
			require.Equal(t, tc.pages*ocrAllowanceOutputTokensPerPage, allowance.outputTokens)
		})
	}
	_, err := quoteOCRAllowance(nil)
	require.Error(t, err)
}

// newOCRPlanContext returns a request context whose channel carries configs and
// the given group ratio, plus matching Zhipu relay metadata.
func newOCRPlanContext(t *testing.T, configs map[string]model.ModelConfigLocal, group float64) (*gin.Context, *metalib.Meta) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/layout_parsing", nil)
	channel := &model.Channel{Id: 1, Type: channeltype.Zhipu}
	if configs != nil {
		require.NoError(t, channel.SetModelPriceConfigs(configs))
	}
	c.Set(ctxkey.ChannelModel, channel)
	c.Set(ctxkey.ChannelRatio, group)
	return c, &metalib.Meta{ChannelType: channeltype.Zhipu, StartTime: time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)}
}

// TestPrepareOCRBillingPlanRejectsUnpriceableTariffs verifies non-finite group
// ratios and tariffs whose allowance cannot be represented are refused before
// any reservation or dispatch, as server-side configuration errors.
func TestPrepareOCRBillingPlanRejectsUnpriceableTariffs(t *testing.T) {
	request := &relaymodel.OCRRequest{Model: ocrTestModel, File: "https://x.test/a.pdf"}
	for name, setup := range map[string]struct {
		configs map[string]model.ModelConfigLocal
		group   float64
	}{
		"nan_group":       {configs: ocrTestTokenTariff, group: math.NaN()},
		"negative_group":  {configs: ocrTestTokenTariff, group: -1},
		"huge_token_rate": {configs: map[string]model.ModelConfigLocal{ocrTestModel: {Ratio: 1e300, CompletionRatio: 1}}, group: 1},
		"huge_page_rate":  {configs: map[string]model.ModelConfigLocal{ocrTestModel: {PerPage: &model.PerPagePricingLocal{UsdPerThousandPages: 1e300}}}, group: 1},
	} {
		t.Run(name, func(t *testing.T) {
			c, m := newOCRPlanContext(t, setup.configs, setup.group)
			_, apiErr := prepareOCRBillingPlan(c, m, request)
			require.NotNil(t, apiErr)
			require.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
		})
	}
}

// TestOCRBillingPlanSettle verifies contract-specific settlement edge cases:
// explicit zero receipts are free, per-call ignores missing evidence, and an
// unpriceable receipt retains the quote with a reconciliation error.
func TestOCRBillingPlanSettle(t *testing.T) {
	request := &relaymodel.OCRRequest{Model: ocrTestModel, File: "https://x.test/a.pdf"}
	c, m := newOCRPlanContext(t, ocrTestTokenTariff, 1)
	plan, apiErr := prepareOCRBillingPlan(c, m, request)
	require.Nil(t, apiErr)
	require.Equal(t, ocrUnitToken, plan.unit)
	require.EqualValues(t, ocrMaxDocumentPages*ocrTestPageQuote, plan.quote)

	zero := plan.settle(&relaymodel.OCRReceipt{Usage: &relaymodel.Usage{}})
	require.Zero(t, zero.quota)
	require.Empty(t, zero.estimateReason)

	missing := plan.settle(nil)
	require.Equal(t, plan.quote, missing.quota)
	require.Equal(t, relaymodel.OCRReceiptMissing, missing.estimateReason)

	huge := plan
	huge.modelRatio = 1e300
	huge.channelModelRatio = map[string]float64{ocrTestModel: 1e300}
	unpriceable := huge.settle(&relaymodel.OCRReceipt{Usage: &relaymodel.Usage{PromptTokens: 10, CompletionTokens: 1}})
	require.Equal(t, plan.quote, unpriceable.quota)
	require.Equal(t, ocrReceiptUnrepresentable, unpriceable.estimateReason)
	require.Error(t, unpriceable.reconcileErr)

	call, apiErr := prepareOCRBillingPlan(newOCRPlanContextWith(t, map[string]model.ModelConfigLocal{
		ocrTestModel: {PerCall: &model.PerCallPricingLocal{UsdPerThousandCalls: 3}}}), m, request)
	require.Nil(t, apiErr)
	require.Equal(t, ocrUnitCall, call.unit)
	require.EqualValues(t, 1500, call.quote)
	settled := call.settle(relaymodel.UnreadableOCRReceipt())
	require.EqualValues(t, 1500, settled.quota, "a per-call tariff bills the invocation, not the receipt")
	require.Empty(t, settled.estimateReason)
}

// newOCRPlanContextWith returns a group-ratio-1 request context for configs.
func newOCRPlanContextWith(t *testing.T, configs map[string]model.ModelConfigLocal) *gin.Context {
	t.Helper()
	c, _ := newOCRPlanContext(t, configs, 1)
	return c
}
