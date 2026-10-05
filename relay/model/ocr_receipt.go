package model

// OCR receipt problem labels explain why a provider receipt cannot be billed as
// measured evidence. They are recorded verbatim as billing estimate reasons.
const (
	// OCRReceiptMissing means the provider omitted the counter entirely.
	OCRReceiptMissing = "ocr_receipt_missing"
	// OCRReceiptInvalid means a counter was present but not a nonnegative JSON integer.
	OCRReceiptInvalid = "ocr_receipt_invalid"
	// OCRReceiptOverflow means a counter exceeded the representable billing bound.
	OCRReceiptOverflow = "ocr_receipt_overflow"
	// OCRReceiptInconsistent means the counters contradicted each other.
	OCRReceiptInconsistent = "ocr_receipt_inconsistent"
	// OCRReceiptUnreadable means the response body could not be read or decoded.
	OCRReceiptUnreadable = "ocr_receipt_unreadable"
)

const (
	// MaxOCRReceiptTokens bounds each token counter accepted from an OCR receipt.
	// It is far above any single layout-parsing job (providers cap documents at
	// 100 pages) while keeping every downstream product exactly representable.
	MaxOCRReceiptTokens = 64 << 20
	// MaxOCRReceiptPages bounds the page counter accepted from an OCR receipt.
	MaxOCRReceiptPages = 100_000
)

// OCRReceipt is the provider's billing evidence for one native layout-parsing
// call. Token and page dimensions are validated independently because a model is
// billed by exactly one of them; an unusable dimension carries a problem label
// instead of a fabricated zero.
type OCRReceipt struct {
	// Usage holds validated token counters, or nil when UsageProblem is set.
	Usage *Usage
	// UsageProblem is empty when Usage is authoritative measured evidence.
	UsageProblem string
	// Pages is the validated processed-page count; it is zero when PagesProblem is set.
	Pages int
	// PagesProblem is empty when Pages is authoritative measured evidence.
	PagesProblem string
}

// UnreadableOCRReceipt returns a receipt for a response whose body could not be
// read or decoded, marking both billing dimensions as unverifiable.
func UnreadableOCRReceipt() *OCRReceipt {
	return &OCRReceipt{UsageProblem: OCRReceiptUnreadable, PagesProblem: OCRReceiptUnreadable}
}

// UsageOrEstimate converts r into token usage for token-based consumers such as
// the chat-completions OCR route. Valid counters are copied; otherwise it returns
// an empty usage labelled with the problem so the caller retains its reservation
// instead of treating the missing evidence as a free request.
func (r *OCRReceipt) UsageOrEstimate() *Usage {
	if r == nil {
		return &Usage{BillingEstimateReason: OCRReceiptMissing}
	}
	if r.Usage != nil && r.UsageProblem == "" {
		usage := *r.Usage
		return &usage
	}
	reason := r.UsageProblem
	if reason == "" {
		reason = OCRReceiptMissing
	}
	return &Usage{BillingEstimateReason: reason}
}
