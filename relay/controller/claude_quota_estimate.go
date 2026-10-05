package controller

import (
	"math"

	"github.com/Laisky/errors/v2"
)

// checkedClaudeQuotaEstimate retains the existing floating tariff arithmetic
// and truncation while rejecting invalid or unrepresentable token/quota sums.
// Callers retain their separate ordinary-admission or MCP minimum semantics.
func checkedClaudeQuotaEstimate(promptTokens, maxTokens int, ratio, completionRatio float64) (int64, error) {
	if promptTokens < 0 {
		return 0, errors.New("invalid Claude prompt token estimate")
	}
	for _, value := range []float64{ratio, completionRatio} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return 0, errors.New("invalid Claude quota ratio")
		}
	}
	total := float64(promptTokens) * ratio
	if maxTokens > 0 {
		total += float64(maxTokens) * ratio * completionRatio
	}
	// float64(MaxInt64) rounds to 2^63, which cannot be converted to int64.
	if math.IsNaN(total) || math.IsInf(total, 0) || total < 0 || total >= float64(math.MaxInt64) {
		return 0, errors.New("Claude quota estimate exceeds integer range")
	}
	return int64(total), nil
}
