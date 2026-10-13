package alibailian

import "github.com/Laisky/one-api/relay/adaptor"

// octoberOCRModels returns fresh Beijing-compatible OCR model definitions.
// It takes no arguments and returns independently owned metadata for the two IDs.
// Sources verified 2026-10-12:
//   - https://help.aliyun.com/zh/model-studio/qwen3-5-ocr
//   - https://help.aliyun.com/zh/model-studio/qwenvl-ocr
//   - https://help.aliyun.com/zh/model-studio/qwen-vl-ocr
func octoberOCRModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"qwen3.5-ocr": {
			Ratio: nativeRate(0.5), CompletionRatio: 4,
			ContextLength: 65536, MaxOutputTokens: 16384,
			InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
			SupportedSamplingParameters: []string{"max_tokens"},
			Description:                 "Qwen 3.5 OCR on the Beijing compatible Chat Completions API: CNY 0.50 input and 2.00 output per million tokens. Image input with optional text instructions; no tools, structured outputs, reasoning or caching. PDF Responses and native OCR task controls are not part of this route.",
		},
		"qwen-vl-ocr-2025-11-20": {
			Ratio: nativeRate(0.3), CompletionRatio: 0.5 / 0.3,
			ContextLength: 38192, MaxOutputTokens: 8192,
			InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
			SupportedSamplingParameters: []string{"max_tokens"},
			Description:                 "Qwen VL OCR snapshot 2025-11-20, equivalent to qwen-vl-ocr. Beijing standard CNY 0.30 input and 0.50 output per million tokens. Text/image input, text output; no tools, structured outputs or caching. The 8192 output ceiling may require account enablement; no PDF Responses or native task controls are added.",
		},
	}
}
