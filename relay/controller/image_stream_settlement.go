package controller

import (
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	relayadaptor "github.com/Laisky/one-api/relay/adaptor"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

const (
	// imageStreamReceiptIncompleteReason marks the usage of a streamed image
	// response whose receipts may omit paid work: the stream was interrupted
	// or failed, an event was unreadable, or fewer final-image receipts
	// arrived than images were requested. Such usage never settles below the
	// hold (strict GPT Image: the larger of the fallback and the receipt's
	// ceiling).
	imageStreamReceiptIncompleteReason = "image_stream_receipt_incomplete"
	// imageStreamReceiptsSummedReason flags a strict GPT Image charge summed
	// from several streamed final-image receipts. OpenAI does not document
	// whether each receipt covers one image or the whole request, so the sum,
	// the largest consistent reading, is charged and flagged as an upper bound.
	imageStreamReceiptsSummedReason = "gpt_image_stream_receipts_summed_upper_bound"
)

// annotateImageStreamUsage folds the billing evidence of a relayed image event
// stream into its usage before settlement. Partial images are billed as
// OpenAI's documented 100 additional image output tokens each, counting the
// larger of the partial images streamed and those requested for every image,
// because the receipt does not say whether its output tokens include them.
// The usage is then marked incomplete unless every requested image delivered
// a final-image receipt in a clean stream, and marked summed when several
// receipts were added. A non-stream response has no stream receipt and is
// returned unchanged. Parameters: c carries the receipt the image handler
// stored, usage is the handler's summed usage (possibly nil), expected the
// rendered image count, and partialImages the requested partial images per
// image. Returns: the usage to settle, a copy when annotated.
func annotateImageStreamUsage(c *gin.Context, usage *relaymodel.Usage, expected, partialImages int) *relaymodel.Usage {
	raw, ok := c.Get(relayadaptor.ImageStreamReceiptKey)
	if !ok {
		return usage
	}
	receipt, ok := raw.(relayadaptor.ImageStreamReceipt)
	if !ok {
		return usage
	}
	expected = max(expected, 1)
	partialTokens := strictGPTImagePartialImageTokens * max(receipt.Partials, max(partialImages, 0)*expected)
	reason := ""
	switch {
	case receipt.Failed || receipt.Interrupted || receipt.Receipts < expected:
		reason = imageStreamReceiptIncompleteReason
	case receipt.Receipts > 1:
		reason = imageStreamReceiptsSummedReason
	}
	gmw.GetLogger(c).Debug("annotated streamed image receipt for settlement",
		zap.Int("expected_images", expected),
		zap.Int("usage_receipts", receipt.Receipts),
		zap.Int("partial_image_tokens", partialTokens),
		zap.String("estimate_reason", reason))
	if usage == nil {
		// Without any final-image receipt the hold settles; it already
		// covers the requested partial images.
		return nil
	}
	annotated := *usage
	annotated.CompletionTokens += partialTokens
	annotated.TotalTokens += partialTokens
	annotated.BillingEstimateReason = reason
	return &annotated
}
