package controller

import (
	"math"
	"net/http"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
	"github.com/Laisky/one-api/relay/quota"
)

// ocrBillingUnit names the single, disjoint pricing contract of an OCR model.
type ocrBillingUnit string

const (
	// ocrUnitToken bills the receipt's input/output tokens through the common
	// token calculator (catalog GLM-OCR on BigModel and Z.ai).
	ocrUnitToken ocrBillingUnit = "token"
	// ocrUnitPage bills the receipt's typed processed-page count (per_page tariff).
	ocrUnitPage ocrBillingUnit = "page"
	// ocrUnitCall bills one flat invocation (per_call tariff), ignoring volume.
	ocrUnitCall ocrBillingUnit = "call"
)

// maxOCRQuota bounds every OCR quote and charge to integers that float64 token
// arithmetic represents exactly; larger values indicate a corrupt tariff or receipt.
const maxOCRQuota = int64(1) << 52

// ocrReceiptUnrepresentable labels a syntactically valid receipt whose price
// cannot be represented; the conservative allowance is retained for review.
const ocrReceiptUnrepresentable = "ocr_receipt_unrepresentable"

// ocrBillingPlan snapshots, before dispatch, the one pricing contract and the
// conservative allowance of an OCR request. Settlement reuses the same tariff
// inputs, so the quote and the final charge are priced identically.
type ocrBillingPlan struct {
	unit       ocrBillingUnit
	modelName  string
	groupRatio float64
	allowance  ocrAllowance
	quote      int64

	// Token-contract inputs for the common token calculator.
	modelRatio             float64
	channelModelRatio      map[string]float64
	channelCompletionRatio map[string]float64
	channelModelConfigs    map[string]model.ModelConfigLocal
	pricingAdaptor         adaptor.Adaptor
	requestTime            time.Time

	// usdPerThousandUnits is the flat page or call tariff in USD per 1000 units.
	usdPerThousandUnits float64
}

// ocrSettlement is the single final charge derived from a receipt.
type ocrSettlement struct {
	quota            int64
	estimateReason   string
	promptTokens     int
	completionTokens int
	cachedTokens     int
	pages            int
	exceedsAllowance bool
	reconcileErr     error
}

// prepareOCRBillingPlan validates the request's document selection, resolves the
// model's single pricing contract (per_call, per_page, or token), and computes
// the conservative quote to reserve before dispatch.
// Parameters: c is the request context, m is the relay metadata, and request is
// the mapped OCR request. Returns: the plan, or a client error for an invalid
// page selection and a server error for an ambiguous or unrepresentable tariff.
func prepareOCRBillingPlan(c *gin.Context, m *metalib.Meta, request *relaymodel.OCRRequest) (ocrBillingPlan, *relaymodel.ErrorWithStatusCode) {
	allowance, err := quoteOCRAllowance(request)
	if err != nil {
		return ocrBillingPlan{}, openai.ErrorWrapper(err, "invalid_ocr_request", http.StatusBadRequest)
	}
	plan := ocrBillingPlan{
		unit:                ocrUnitToken,
		modelName:           request.Model,
		groupRatio:          c.GetFloat64(ctxkey.ChannelRatio),
		allowance:           allowance,
		channelModelConfigs: getChannelModelConfigs(c),
		pricingAdaptor:      resolvePricingAdaptor(m),
		requestTime:         m.StartTime,
	}
	if !validOCRRate(plan.groupRatio) {
		return ocrBillingPlan{}, openai.ErrorWrapper(errors.New("OCR group ratio must be finite and nonnegative"), "invalid_ocr_pricing", http.StatusInternalServerError)
	}

	cfg, found := pricing.ResolveModelConfig(plan.modelName, plan.channelModelConfigs, plan.pricingAdaptor, plan.requestTime)
	switch {
	case found && cfg.PerCall != nil && cfg.PerPage != nil:
		return ocrBillingPlan{}, openai.ErrorWrapper(errors.Errorf("model %s declares both per_call and per_page pricing", plan.modelName),
			"invalid_ocr_pricing", http.StatusInternalServerError)
	case found && cfg.PerCall != nil:
		plan.unit, plan.usdPerThousandUnits = ocrUnitCall, cfg.PerCall.UsdPerThousandCalls
		plan.quote, err = plan.unitQuota(1)
	case found && cfg.PerPage != nil:
		plan.unit, plan.usdPerThousandUnits = ocrUnitPage, cfg.PerPage.UsdPerThousandPages
		plan.quote, err = plan.unitQuota(allowance.pages)
	default:
		plan.channelModelRatio, plan.channelCompletionRatio = getChannelRatios(c)
		plan.modelRatio = pricing.ResolveModelRatioAt(plan.modelName, plan.channelModelConfigs, plan.channelModelRatio, plan.pricingAdaptor, plan.requestTime)
		plan.quote, err = plan.tokenQuota(&relaymodel.Usage{
			PromptTokens:     allowance.inputTokens,
			CompletionTokens: allowance.outputTokens,
			TotalTokens:      allowance.inputTokens + allowance.outputTokens,
		})
	}
	if err != nil {
		return ocrBillingPlan{}, openai.ErrorWrapper(errors.Wrap(err, "quote OCR allowance"), "invalid_ocr_pricing", http.StatusInternalServerError)
	}
	return plan, nil
}

// tokenQuota prices usage through the common token calculator with the plan's
// tariff. GLM-OCR has no cache-hit tariff on either brand, so reported cached
// tokens are billed as ordinary input. Because the calculator clamps any
// non-positive float result to one quota unit, the charge is re-derived from
// the calculator's effective ratios with exact decimal arithmetic and refused
// when it is not representable, instead of letting an overflow collapse to a
// one-unit charge. An explicit zero-token receipt is free.
// Parameters: usage holds nonnegative token counters. Returns: the quota or an error.
func (p ocrBillingPlan) tokenQuota(usage *relaymodel.Usage) (int64, error) {
	if usage == nil || usage.PromptTokens < 0 || usage.CompletionTokens < 0 ||
		usage.PromptTokens > relaymodel.MaxOCRReceiptTokens || usage.CompletionTokens > relaymodel.MaxOCRReceiptTokens {
		return 0, errors.New("OCR token counters are outside the billing range")
	}
	if !validOCRRate(p.modelRatio) {
		return 0, errors.New("OCR token ratio must be finite and nonnegative")
	}
	if usage.PromptTokens == 0 && usage.CompletionTokens == 0 {
		return 0, nil
	}
	result := quota.Compute(quota.ComputeInput{
		Usage: &relaymodel.Usage{
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.PromptTokens + usage.CompletionTokens,
		},
		ModelName:              p.modelName,
		ModelRatio:             p.modelRatio,
		ChannelModelRatio:      p.channelModelRatio,
		GroupRatio:             p.groupRatio,
		ChannelModelConfigs:    p.channelModelConfigs,
		ChannelCompletionRatio: p.channelCompletionRatio,
		PricingAdaptor:         p.pricingAdaptor,
		RequestTime:            p.requestTime,
	})
	input, err := decimalQuotaProduct(float64(usage.PromptTokens), result.UsedModelRatio, p.groupRatio)
	if err != nil {
		return 0, errors.Wrap(err, "check OCR input charge")
	}
	output, err := decimalQuotaProduct(float64(usage.CompletionTokens), result.UsedModelRatio, result.UsedCompletionRatio, p.groupRatio)
	if err != nil {
		return 0, errors.Wrap(err, "check OCR output charge")
	}
	if input > maxOCRQuota || output > maxOCRQuota-input || result.TotalQuota < 0 || result.TotalQuota > maxOCRQuota {
		return 0, errors.Errorf("OCR token charge is outside the representable quota range (input %d, output %d, computed %d)",
			input, output, result.TotalQuota)
	}
	return result.TotalQuota, nil
}

// unitQuota prices units of the plan's flat page or call tariff with exact
// decimal arithmetic, rounding up once.
// Parameters: units is the nonnegative number of pages or calls. Returns: the quota or an error.
func (p ocrBillingPlan) unitQuota(units int) (int64, error) {
	if units < 0 {
		return 0, errors.New("OCR billing units must be nonnegative")
	}
	amount, err := decimalQuotaRate(1000, float64(units), p.usdPerThousandUnits, float64(billingratio.QuotaPerUsd), p.groupRatio)
	if err != nil {
		return 0, errors.Wrap(err, "price OCR units")
	}
	if amount > maxOCRQuota {
		return 0, errors.Errorf("OCR unit charge %d is outside the representable quota range", amount)
	}
	return amount, nil
}

// settle derives the single final charge from receipt under the plan's contract.
// Per-call tariffs always bill one invocation. Page and token tariffs bill the
// receipt's validated dimension; missing, invalid, overflowing or unpriceable
// evidence keeps the conservative quote with an explicit estimate reason.
// Parameters: receipt is the provider evidence (nil is treated as missing).
// Returns: the settlement to record.
func (p ocrBillingPlan) settle(receipt *relaymodel.OCRReceipt) ocrSettlement {
	if receipt == nil {
		receipt = &relaymodel.OCRReceipt{UsageProblem: relaymodel.OCRReceiptMissing, PagesProblem: relaymodel.OCRReceiptMissing}
	}
	settlement := ocrSettlement{quota: p.quote}
	if receipt.UsageProblem == "" && receipt.Usage != nil {
		settlement.promptTokens = receipt.Usage.PromptTokens
		settlement.completionTokens = receipt.Usage.CompletionTokens
		if details := receipt.Usage.PromptTokensDetails; details != nil {
			settlement.cachedTokens = details.CachedTokens
		}
	}
	if receipt.PagesProblem == "" {
		settlement.pages = receipt.Pages
	}

	switch p.unit {
	case ocrUnitCall:
		return settlement
	case ocrUnitPage:
		if receipt.PagesProblem != "" {
			settlement.estimateReason = receipt.PagesProblem
			return settlement
		}
		pages := receipt.Pages
		if p.allowance.explicitRange {
			// The provider processes only the selected range; num_pages may count
			// the whole document, so the range bounds the billable pages.
			pages = min(pages, p.allowance.pages)
		}
		settlement.pages = pages
		amount, err := p.unitQuota(pages)
		if err != nil {
			settlement.estimateReason, settlement.reconcileErr = ocrReceiptUnrepresentable, err
			return settlement
		}
		settlement.quota, settlement.exceedsAllowance = amount, pages > p.allowance.pages
		return settlement
	default:
		if receipt.UsageProblem != "" || receipt.Usage == nil {
			settlement.estimateReason = receipt.UsageProblem
			if settlement.estimateReason == "" {
				settlement.estimateReason = relaymodel.OCRReceiptMissing
			}
			return settlement
		}
		amount, err := p.tokenQuota(receipt.Usage)
		if err != nil {
			settlement.estimateReason, settlement.reconcileErr = ocrReceiptUnrepresentable, err
			return settlement
		}
		settlement.quota, settlement.exceedsAllowance = amount, amount > p.quote
		return settlement
	}
}

// validOCRRate reports whether rate is a finite, nonnegative price multiplier.
func validOCRRate(rate float64) bool {
	return rate >= 0 && !math.IsNaN(rate) && !math.IsInf(rate, 0)
}
