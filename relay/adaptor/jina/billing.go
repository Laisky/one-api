package jina

import (
	"encoding/json"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

const (
	budgetKey = "jina_billing_budget"
	// MaxBillingTokens bounds arithmetic and memory-independent preauthorization.
	MaxBillingTokens = 64 * 1024 * 1024
	maxBillingItems  = 512
)

// BillingBudget is an immutable, conservative token allowance, not measured usage.
// Search budgets cover every submitted item, never just top_n returned results.
type BillingBudget struct{ Input, Output int }

// StoreBillingBudget attaches a validated allowance to c for response accounting.
// It does not deduct balances and returns no value.
func StoreBillingBudget(c *gin.Context, budget BillingBudget) { c.Set(budgetKey, budget) }

// GetBillingBudget returns the per-attempt allowance and whether it was prepared.
func GetBillingBudget(c *gin.Context) (BillingBudget, bool) {
	if c == nil {
		return BillingBudget{}, false
	}
	value, ok := c.Get(budgetKey)
	budget, valid := value.(BillingBudget)
	return budget, ok && valid
}

// ClearBillingBudget removes a failed attempt's allowance before a safe retry.
func ClearBillingBudget(c *gin.Context) { c.Set(budgetKey, nil) }

// EstimatedUsage returns the prepared conservative allowance with an audit reason.
// A missing allowance returns nil so the controller's retained-reservation safety
// net remains responsible; this function never invents authoritative zero usage.
func EstimatedUsage(c *gin.Context, reason string) *model.Usage {
	budget, ok := GetBillingBudget(c)
	if !ok {
		return nil
	}
	return &model.Usage{PromptTokens: budget.Input, CompletionTokens: budget.Output,
		TotalTokens: budget.Input + budget.Output, BillingEstimateReason: reason}
}

// QuoteRequest validates supported, bounded inputs and returns a conservative
// allowance for a mapped embedding or OCR request. It never makes an upstream call.
func QuoteRequest(request *model.GeneralOpenAIRequest, mode int) (BillingBudget, error) {
	if request == nil {
		return BillingBudget{}, errors.New("jina request is nil")
	}
	cfg, ok := ModelRatios[request.Model]
	if !ok {
		return BillingBudget{}, errors.New("jina model has no verified billing configuration")
	}
	if mode != relaymode.Embeddings {
		if request.Model != "jina-ocr-v1" || (mode != relaymode.ChatCompletions && mode != relaymode.ClaudeMessages) {
			return BillingBudget{}, errors.New("jina model does not support the requested endpoint")
		}
		if len(request.Tools) > 0 || len(request.Functions) > 0 || (request.N != nil && *request.N != 1) || request.ExtraBody != nil {
			return BillingBudget{}, errors.New("jina OCR requires one completion and no tools or extra_body for bounded billing")
		}
		limit := int(cfg.MaxOutputTokens)
		if request.MaxCompletionTokens != nil {
			limit = *request.MaxCompletionTokens
		} else if request.MaxTokens != 0 {
			limit = request.MaxTokens
		}
		if limit <= 0 || limit > int(cfg.MaxOutputTokens) {
			return BillingBudget{}, errors.New("jina OCR output limit must be between 1 and 8192 tokens")
		}
		// Inspect content rather than URL length; only text/image content is bounded
		// by this OCR model's context contract. Do not fetch remote media here.
		raw, err := json.Marshal(request.Messages)
		if err != nil {
			return BillingBudget{}, errors.Wrap(err, "inspect jina OCR input")
		}
		if len(raw) > 8*1024*1024 {
			return BillingBudget{}, errors.New("jina OCR input exceeds billing admission limit")
		}
		for _, msg := range request.Messages {
			if err := validateOCRContent(msg.Content); err != nil {
				return BillingBudget{}, errors.Wrap(err, "validate jina OCR content")
			}
		}
		return BillingBudget{Input: int(cfg.ContextLength), Output: limit}, nil
	}
	if !strings.Contains(request.Model, "embeddings") && !strings.HasPrefix(request.Model, "jina-clip-") && !strings.HasPrefix(request.Model, "jina-colbert-") {
		return BillingBudget{}, errors.New("jina model is not an embedding model")
	}
	image := false
	for _, modality := range cfg.InputModalities {
		image = image || modality == "image"
	}
	items := []any{request.Input}
	if values, ok := request.Input.([]any); ok {
		items = values
	}
	if values, ok := request.Input.([]string); ok {
		items = make([]any, len(values))
		for i, v := range values {
			items[i] = v
		}
	}
	if len(items) == 0 || len(items) > maxBillingItems {
		return BillingBudget{}, errors.New("jina input batch must contain 1 to 512 items")
	}
	total := 0
	for _, item := range items {
		allowance, err := quoteItem(item, int(cfg.ContextLength), image)
		if err != nil {
			return BillingBudget{}, errors.Wrap(err, "quote jina embedding item")
		}
		if allowance > MaxBillingTokens-total {
			return BillingBudget{}, errors.New("jina request exceeds billing token admission limit")
		}
		total += allowance
	}
	return BillingBudget{Input: total}, nil
}

// QuoteRerank returns a conservative allowance for all query/document work.
// Query work is reserved for every document; top_n never lowers the allowance.
func QuoteRerank(request *model.RerankRequest) (BillingBudget, error) {
	if request == nil {
		return BillingBudget{}, errors.New("jina rerank request is nil")
	}
	cfg, ok := ModelRatios[request.Model]
	if !ok || (!strings.HasPrefix(request.Model, "jina-reranker-") && !strings.HasPrefix(request.Model, "jina-colbert-")) {
		return BillingBudget{}, errors.New("jina model has no verified rerank billing configuration")
	}
	n := len(request.Documents)
	if n == 0 || n > maxBillingItems {
		return BillingBudget{}, errors.New("jina rerank requires 1 to 512 documents")
	}
	query := any(request.Query)
	if len(request.NativeQuery) > 0 {
		if err := json.Unmarshal(request.NativeQuery, &query); err != nil {
			return BillingBudget{}, errors.Wrap(err, "decode jina rerank query")
		}
	}
	q, err := quoteItem(query, int(cfg.ContextLength), request.Model == "jina-reranker-m0")
	if err != nil {
		return BillingBudget{}, errors.Wrap(err, "quote jina rerank query")
	}
	total := 0
	for i, doc := range request.Documents {
		item := any(doc)
		if len(request.NativeDocuments) > 0 {
			if len(request.NativeDocuments) != n {
				return BillingBudget{}, errors.New("jina native document count mismatch")
			}
			if err := json.Unmarshal(request.NativeDocuments[i], &item); err != nil {
				return BillingBudget{}, errors.Wrap(err, "decode jina rerank document")
			}
		}
		d, err := quoteItem(item, int(cfg.ContextLength), request.Model == "jina-reranker-m0")
		if err != nil {
			return BillingBudget{}, errors.Wrap(err, "quote jina rerank document")
		}
		if q+d > MaxBillingTokens-total {
			return BillingBudget{}, errors.New("jina rerank exceeds billing token admission limit")
		}
		total += q + d
	}
	return BillingBudget{Input: total}, nil
}

// quoteItem returns a conservative text/image allowance or rejects opaque work.
// PDFs, audio, video and grouped content have no verified billing bound here.
// A remote URL's byte length is never treated as its downloaded content length.
func quoteItem(value any, contextLength int, images bool) (int, error) {
	switch item := value.(type) {
	case string:
		if len(item) == 0 || len(item) > 1024*1024 {
			return 0, errors.New("jina text item must contain 1 byte to 1 MiB")
		}
		// Reserve context plus a byte-based allowance for chunked long text and
		// provider task prefixes. This is explicitly an estimate, not a tokenizer.
		return contextLength + 4*len(item), nil
	case map[string]any:
		if len(item) != 1 {
			return 0, errors.New("jina grouped/opaque input has no verified billing bound")
		}
		if text, ok := item["text"].(string); ok {
			return quoteItem(text, contextLength, images)
		}
		if image, ok := item["image"].(string); ok && images && image != "" && len(image) <= 8*1024*1024 {
			return contextLength, nil
		}
	}
	return 0, errors.New("jina input modality has no verified billing bound; only supported text/image inputs are admitted")
}

// validateOCRContent rejects unbounded OCR modalities while accepting typed or
// JSON-decoded text/image content. It returns a wrapped validation error.
func validateOCRContent(content any) error {
	if _, ok := content.(string); ok {
		return nil
	}
	raw, err := json.Marshal(content)
	if err != nil {
		return errors.Wrap(err, "encode OCR content")
	}
	var parts []struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return errors.Wrap(err, "decode OCR content parts")
	}
	for _, part := range parts {
		if part.Type != "text" && part.Type != "image_url" {
			return errors.New("jina OCR supports only bounded text/image content")
		}
	}
	return nil
}

// BudgetQuota validates effective prices and rounds a reservation upward once.
// Invalid, non-finite or unrepresentable charges are rejected before dispatch.
func BudgetQuota(budget BillingBudget, inputRatio, completionRatio, groupRatio float64) (int64, error) {
	for _, rate := range []float64{inputRatio, completionRatio, groupRatio} {
		if math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 {
			return 0, errors.New("invalid jina billing rate")
		}
	}
	if budget.Input < 0 || budget.Output < 0 || budget.Input > MaxBillingTokens || budget.Output > MaxBillingTokens || budget.Input+budget.Output > MaxBillingTokens {
		return 0, errors.New("invalid jina billing budget")
	}
	// Parse the decimal rate representation into rationals: float multiplication
	// can underflow a positive fee to zero or cross a quota rounding boundary.
	rates := make([]*big.Rat, 3)
	for i, rate := range []float64{inputRatio, completionRatio, groupRatio} {
		value, ok := new(big.Rat).SetString(strconv.FormatFloat(rate, 'g', -1, 64))
		if !ok {
			return 0, errors.New("invalid decimal jina billing rate")
		}
		rates[i] = value
	}
	cost := new(big.Rat).Mul(new(big.Rat).SetInt64(int64(budget.Output)), rates[1])
	cost.Add(cost, new(big.Rat).SetInt64(int64(budget.Input)))
	cost.Mul(cost, rates[0])
	cost.Mul(cost, rates[2])
	if cost.Cmp(new(big.Rat).SetInt64(1<<52)) > 0 {
		return 0, errors.New("jina billing cost exceeds safe arithmetic range")
	}
	units, remainder := new(big.Int), new(big.Int)
	units.QuoRem(cost.Num(), cost.Denom(), remainder)
	if remainder.Sign() > 0 {
		units.Add(units, big.NewInt(1))
	}
	return units.Int64(), nil
}
