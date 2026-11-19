package controller

import (
	"cmp"
	"math"
	"slices"

	"github.com/Laisky/errors/v2"

	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

const (
	// strictImageModalityUpperBoundReason flags a strict GPT Image charge whose
	// receipt totals are usable but whose input modality or cache split is not
	// fully stated; the charge is the most expensive consistent assignment.
	strictImageModalityUpperBoundReason = "gpt_image_receipt_modality_upper_bound"
	// strictImageConfiguredFallbackReason flags a strict GPT Image charge settled
	// at the operator's configured per-image fallback because the receipt carried
	// no usable token totals.
	strictImageConfiguredFallbackReason = "gpt_image_receipt_unusable_configured_fallback"
	// strictImageIncompleteReceiptReason flags a strict GPT Image charge for a
	// receipt that is missing its input count, its output count, or whose
	// buckets contradict its totals, when pricing every reported token at the
	// highest applicable rate exceeds the operator fallback.
	strictImageIncompleteReceiptReason = "gpt_image_receipt_incomplete_upper_bound"
)

// usesStrictGPTImageTokenBilling reports whether name is a verified GPT Image
// 2.5/2.6 token-priced ID settled under the strict policy: provider token
// buckets replace the reservation, an operator per-image fallback is required
// at admission, and unused reservation is refunded. Parameters: name is the
// mapped provider model. Returns: whether the strict policy applies.
func usesStrictGPTImageTokenBilling(name string) bool {
	switch name {
	case "gpt-image-2.5-sunburst", "gpt-image-2.5-sunburst-2026-09-08",
		"gpt-image-2.5-flare", "gpt-image-2.5-flare-2026-09-08",
		"gpt-image-2.6-sunburst", "gpt-image-2.6-flare":
		return true
	default:
		return false
	}
}

// isGPTImage26Model identifies the exact published GPT Image 2.6 IDs, the only
// models whose request contract accepts complexity. Parameters: name is the
// mapped provider model. Returns: true only for gpt-image-2.6-sunburst and
// gpt-image-2.6-flare; unpublished aliases and snapshots are excluded.
func isGPTImage26Model(name string) bool {
	switch name {
	case "gpt-image-2.6-sunburst", "gpt-image-2.6-flare":
		return true
	default:
		return false
	}
}

// imagePostConsumeDelta computes the remaining debit or reservation refund.
// Parameters: name selects the billing policy; used and reserved are quota units.
// Returns: a signed delta for GPT Image 2.5/2.6, preserving legacy settlement elsewhere.
func imagePostConsumeDelta(name string, used, reserved int64) int64 {
	delta := used - reserved
	if delta < 0 && !usesStrictGPTImageTokenBilling(name) {
		return 0
	}
	return delta
}

// strictGPTImageQuote is the token-priced outcome of one strict GPT Image
// receipt. quota is in quota units and is zero when the receipt is unusable;
// upperBound reports that the receipt did not fully state its input modality
// or cache split, so quota is the most expensive assignment consistent with
// the stated token totals rather than an exact provider price.
type strictGPTImageQuote struct {
	quota      float64
	upperBound bool
}

// strictGPTImageInputBucket is one input-token bucket: its token count, how
// many of those tokens were cache reads, and its uncached/cached USD prices.
type strictGPTImageInputBucket struct {
	tokens, cachedTokens   int
	uncachedUSD, cachedUSD float64
}

// computeStrictGPTImageTokenQuote prices normalized provider usage under the
// strict GPT Image token policy. The receipt's token totals are authoritative.
// When the provider omits part of the input modality split, the unclassified
// tokens are priced as the more expensive modality; when it omits the cache
// split across several possible buckets, cache reads are credited where the
// cache discount is smallest. Either case yields a provable upper bound of the
// provider's charge, flagged as such, instead of under-billing at the text rate
// or settling a fallback below the receipt's own output cost. Parameters: usage
// carries token counts, prices are USD per million tokens, and group is the
// quota multiplier. Returns: the quote, with zero quota when the buckets are
// negative, inconsistent with the totals, or unrepresentable.
func computeStrictGPTImageTokenQuote(usage *relaymodel.Usage, prices gptImageTokenBucketPricing, group float64) strictGPTImageQuote {
	if usage == nil || usage.PromptTokens < 0 || usage.CompletionTokens < 0 || group <= 0 || math.IsNaN(group) || math.IsInf(group, 0) {
		return strictGPTImageQuote{}
	}
	var textIn, imageIn, cached int
	var cacheDetails *relaymodel.UsageCachedTokensDetails
	if d := usage.PromptTokensDetails; d != nil {
		textIn, imageIn, cached = d.TextTokens, d.ImageTokens, d.CachedTokens
		cacheDetails = d.CachedTokensDetails
	}
	if textIn < 0 || imageIn < 0 || cached < 0 || textIn > math.MaxInt-imageIn {
		return strictGPTImageQuote{}
	}
	unclassified := 0
	if classified, input := textIn+imageIn, strictGPTImageInputTokens(usage); input > 0 {
		if classified > input {
			return strictGPTImageQuote{}
		}
		unclassified = input - classified
	}
	buckets := [3]strictGPTImageInputBucket{
		{tokens: textIn, uncachedUSD: prices.inputTextUSD, cachedUSD: prices.cachedInputTextUSD},
		{tokens: imageIn, uncachedUSD: prices.inputImageUSD, cachedUSD: prices.cachedInputImageUSD},
		// An unclassified token is either text or image: each of its rates is
		// bounded by the larger of the two modality rates.
		{tokens: unclassified,
			uncachedUSD: max(prices.inputTextUSD, prices.inputImageUSD),
			cachedUSD:   max(prices.cachedInputTextUSD, prices.cachedInputImageUSD)},
	}
	if cached > textIn+imageIn+unclassified {
		return strictGPTImageQuote{}
	}
	upperBound := unclassified > 0
	switch {
	case cacheDetails != nil:
		cachedText, cachedImage := cacheDetails.TextTokens, cacheDetails.ImageTokens
		if cachedText < 0 || cachedText > textIn || cachedImage < 0 || cachedImage > imageIn {
			return strictGPTImageQuote{}
		}
		// A supplied aggregate must agree with the modality-specific buckets.
		// Some compatible endpoints omit the aggregate but supply the split.
		if cached > 0 && cachedText+cachedImage != cached {
			return strictGPTImageQuote{}
		}
		buckets[0].cachedTokens, buckets[1].cachedTokens = cachedText, cachedImage
	case cached > 0:
		upperBound = assignStrictGPTImageCacheReads(&buckets, cached) || upperBound
	}
	total := float64(usage.CompletionTokens) * prices.outputImageUSD
	for _, bucket := range buckets {
		total += float64(bucket.tokens-bucket.cachedTokens)*bucket.uncachedUSD + float64(bucket.cachedTokens)*bucket.cachedUSD
	}
	quota := total * billingratio.MilliTokensUsd * group
	if math.IsNaN(quota) || math.IsInf(quota, 0) || quota < 0 || quota >= float64(uint64(1)<<63) {
		return strictGPTImageQuote{}
	}
	return strictGPTImageQuote{quota: quota, upperBound: upperBound}
}

// assignStrictGPTImageCacheReads distributes an aggregate cache-read count
// that the receipt did not split by modality. Crediting the buckets with the
// smallest cache discount first maximises the charge, which keeps the linear
// price an upper bound of every split consistent with the receipt. Parameters:
// buckets are mutated in place and cached does not exceed their total tokens.
// Returns: whether the split could change the charge, i.e. whether the result
// is an upper bound rather than exact: false when every input token is cached
// or when every non-empty bucket has the same cache discount.
func assignStrictGPTImageCacheReads(buckets *[3]strictGPTImageInputBucket, cached int) bool {
	discount := func(i int) float64 { return buckets[i].uncachedUSD - buckets[i].cachedUSD }
	order := []int{0, 1, 2}
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(discount(a), discount(b)) })
	total, ambiguous := 0, false
	first := -1
	for i := range buckets {
		if buckets[i].tokens <= 0 {
			continue
		}
		total += buckets[i].tokens
		if first < 0 {
			first = i
		} else if discount(i) != discount(first) {
			ambiguous = true
		}
	}
	remaining := cached
	for _, i := range order {
		take := min(remaining, buckets[i].tokens)
		buckets[i].cachedTokens = take
		remaining -= take
	}
	return ambiguous && cached < total
}

// strictGPTImageInputTokens returns the input-token count a strict GPT Image
// receipt establishes: the larger of input_tokens, total_tokens minus
// output_tokens, and the stated modality split, so a receipt that omits
// input_tokens but reports a larger total is not billed as output only.
// Parameters: usage is the normalized receipt and may be nil. Returns: the
// non-negative input count.
func strictGPTImageInputTokens(usage *relaymodel.Usage) int {
	if usage == nil {
		return 0
	}
	input := max(usage.PromptTokens, 0)
	if usage.TotalTokens > 0 && usage.CompletionTokens >= 0 && usage.TotalTokens > usage.CompletionTokens {
		input = max(input, usage.TotalTokens-usage.CompletionTokens)
	}
	return input
}

// strictGPTImageReceiptCeiling prices every token a strict GPT Image receipt
// reports at the highest applicable uncached rate, ignoring any cache
// discount: input at the larger of the text/image input rates (counting the
// larger of the input total and the stated modality split) and output at the
// output rate. It is the provable maximum the receipt's own counters allow and
// the floor applied when the receipt is too incomplete or inconsistent for
// bucket pricing. Parameters: usage is the receipt, prices the bucket prices,
// and group the quota multiplier. Returns: quota units, or zero when unusable.
func strictGPTImageReceiptCeiling(usage *relaymodel.Usage, prices gptImageTokenBucketPricing, group float64) float64 {
	if usage == nil || group <= 0 || math.IsNaN(group) || math.IsInf(group, 0) {
		return 0
	}
	input := strictGPTImageInputTokens(usage)
	if d := usage.PromptTokensDetails; d != nil {
		input = max(input, max(d.TextTokens, 0)+max(d.ImageTokens, 0), max(d.CachedTokens, 0))
	}
	inputRate := max(prices.inputTextUSD, prices.inputImageUSD, prices.cachedInputTextUSD, prices.cachedInputImageUSD, 0)
	ceiling := (float64(input)*inputRate + float64(max(usage.CompletionTokens, 0))*max(prices.outputImageUSD, 0)) *
		billingratio.MilliTokensUsd * group
	if math.IsNaN(ceiling) || math.IsInf(ceiling, 0) || ceiling < 0 || ceiling >= float64(uint64(1)<<62) {
		return 0
	}
	return ceiling
}

// validateStrictGPTImageBilling checks admission before quota lookup or upstream I/O.
// Parameters: name is the mapped model; fallbackUSD, tier, group, and count define
// the operator's reservation/fallback tariff. Returns: a configuration error when
// the operator fallback is missing or no finite, representable reservation
// exists for a paid group; nil for a free group (ratio 0) and for other models.
func validateStrictGPTImageBilling(name string, fallbackUSD, tier, group float64, count int) error {
	if !usesStrictGPTImageTokenBilling(name) {
		return nil
	}
	if fallbackUSD <= 0 || tier <= 0 || count <= 0 {
		return errors.New("GPT Image 2.5/2.6 requires a positive image.price_per_image_usd reservation/fallback tariff in channel model_configs; this is operator policy, not an official per-image price")
	}
	if group < 0 {
		return errors.New("GPT Image 2.5/2.6 billing requires a non-negative group ratio")
	}
	if group == 0 {
		// A free group (ratio 0) pays nothing at any receipt: there is nothing
		// to reserve, and settlement prices every token bucket at zero.
		return nil
	}
	perImage := math.Ceil(fallbackUSD * billingratio.QuotaPerUsd * tier * group)
	if math.IsNaN(perImage) || math.IsInf(perImage, 0) || perImage <= 0 || perImage >= float64(uint64(1)<<63)/float64(count) {
		return errors.New("GPT Image 2.5/2.6 reservation/fallback tariff must produce finite, representable quota")
	}
	return nil
}
