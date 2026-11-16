package alibailian

import (
	"github.com/Laisky/one-api/relay/adaptor"
)

// stepfunModels returns independently owned stepfun defaults for alibailian.
// It takes no arguments and returns the complete family catalog.
// Source: https://help.aliyun.com/zh/model-studio/model-pricing
func stepfunModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"stepfun/step-3.7-flash": {
			Ratio:           nativeRate(1.35),
			CompletionRatio: 5.999999999999999,
			Description:     "stepfun/step-3.7-flash on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
		"stepfun/step-5-preview": {
			Ratio:           nativeRate(7),
			CompletionRatio: 2.857142857142857,
			Description:     "stepfun/step-5-preview on alibailian; official catalog snapshot 2026-09-24. Provider access and deployment configuration remain administrator-controlled.",
		},
	}
}
