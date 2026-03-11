package ali

import (
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios.
// The model list is derived from the keys of this map.
// All prices are in MilliTokensRmb (quota per milli-token, RMB pricing):
//
//	1 RMB per 1,000 tokens = 1 * 1000 * ratio.MilliTokensRmb
//	Example: 0.002 RMB/1k tokens = 0.002 * 1000 * ratio.MilliTokensRmb
//
// https://help.aliyun.com/zh/model-studio/models
var ModelRatios = map[string]adaptor.ModelConfig{
	// Qwen Turbo Models (2025-11)
	// Non-thinking mode: 0.0006 RMB/1k tokens, Thinking mode: 0.0003 RMB/1k tokens
	"qwen-turbo":        {Ratio: 0.0006 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-turbo-latest": {Ratio: 0.0006 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Qwen Plus Models (2025-11)
	// Non-thinking mode: 0.002 RMB/1k tokens, Thinking mode: 0.008 RMB/1k tokens
	"qwen-plus":        {Ratio: 0.002 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-plus-latest": {Ratio: 0.002 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Qwen Max Models (2025-11)
	// Tiered pricing: 0<Token≤32K: 0.006 RMB/1k tokens, 32K<Token≤128K: 0.01 RMB/1k tokens, 128K<Token≤252K: 0.015 RMB/1k tokens
	// Using the lowest tier here
	"qwen-max":             {Ratio: 0.006 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-max-latest":      {Ratio: 0.006 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-max-longcontext": {Ratio: 0.006 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Qwen Vision Models (2025-11)
	// Example: VL Plus, 0.001 RMB/1k tokens
	"qwen-vl-max":         {Ratio: 0.001 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-vl-max-latest":  {Ratio: 0.001 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-vl-plus":        {Ratio: 0.001 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-vl-plus-latest": {Ratio: 0.001 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	// OCR 0.005 RMB/1k tokens
	"qwen-vl-ocr":        {Ratio: 0.005 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-vl-ocr-latest": {Ratio: 0.005 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Qwen Audio Models (2025-11)
	// Currently free for trial
	"qwen-audio-turbo": {Ratio: 0, CompletionRatio: 1},

	// Qwen Math Models (2025-11)
	"qwen-math-plus":         {Ratio: 0.004 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-math-plus-latest":  {Ratio: 0.004 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-math-turbo":        {Ratio: 0.002 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-math-turbo-latest": {Ratio: 0.002 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Qwen Coder Models (2025-11)
	// Example: Plus, 0.004 RMB/1k tokens
	"qwen-coder-plus":         {Ratio: 0.004 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-coder-plus-latest":  {Ratio: 0.004 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-coder-turbo":        {Ratio: 0.002 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-coder-turbo-latest": {Ratio: 0.002 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Qwen MT Models (2025-11)
	"qwen-mt-plus":  {Ratio: 0.0018 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-mt-turbo": {Ratio: 0.0007 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// QwQ Models (2025-11)
	"qwq-32b-preview": {Ratio: 0.002 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Qwen 2.5 Models (2025-11)
	"qwen2.5-72b-instruct":  {Ratio: 0.004 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen2.5-32b-instruct":  {Ratio: 0.03 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen2.5-14b-instruct":  {Ratio: 0.001 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen2.5-7b-instruct":   {Ratio: 0.0005 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen2.5-3b-instruct":   {Ratio: 0.006 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen2.5-1.5b-instruct": {Ratio: 0.0003 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen2.5-0.5b-instruct": {Ratio: 0.0003 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Qwen 2 Models (2025-11)
	"qwen2-72b-instruct":      {Ratio: 0.004 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen2-57b-a14b-instruct": {Ratio: 0.0035 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen2-7b-instruct":       {Ratio: 0.001 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen2-1.5b-instruct":     {Ratio: 0.0003 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen2-0.5b-instruct":     {Ratio: 0.0003 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Qwen 1.5 Models (2025-11)
	"qwen1.5-110b-chat": {Ratio: 0.008 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen1.5-72b-chat":  {Ratio: 0.004 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen1.5-32b-chat":  {Ratio: 0.002 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen1.5-14b-chat":  {Ratio: 0.001 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen1.5-7b-chat":   {Ratio: 0.0005 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen1.5-1.8b-chat": {Ratio: 0.0003 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen1.5-0.5b-chat": {Ratio: 0.0003 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Qwen 1 Models (2025-11)
	"qwen-72b-chat":              {Ratio: 0.004 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-14b-chat":              {Ratio: 0.001 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-7b-chat":               {Ratio: 0.0005 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-1.8b-chat":             {Ratio: 0.0003 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-1.8b-longcontext-chat": {Ratio: 0.0003 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// QVQ Models (2025-11)
	"qvq-72b-preview": {Ratio: 0.012 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Qwen 2.5 VL Models (2025-11)
	"qwen2.5-vl-72b-instruct":  {Ratio: 0.004 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen2.5-vl-7b-instruct":   {Ratio: 0.0005 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen2.5-vl-2b-instruct":   {Ratio: 0.0003 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen2.5-vl-1b-instruct":   {Ratio: 0.0003 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen2.5-vl-0.5b-instruct": {Ratio: 0.0003 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Qwen 2 VL Models (2025-11)
	"qwen2-vl-7b-instruct": {Ratio: 0.0005 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen2-vl-2b-instruct": {Ratio: 0.0003 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-vl-v1":           {Ratio: 0.001 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen-vl-chat-v1":      {Ratio: 0.001 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Qwen Audio Models (2025-11)
	"qwen2-audio-instruct": {Ratio: 0, CompletionRatio: 1},
	"qwen-audio-chat":      {Ratio: 0, CompletionRatio: 1},

	// Qwen Math Models (additional, 2025-11)
	"qwen2.5-math-72b-instruct":  {Ratio: 0.004 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen2.5-math-7b-instruct":   {Ratio: 0.001 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen2.5-math-1.5b-instruct": {Ratio: 0, CompletionRatio: 1},
	"qwen2-math-72b-instruct":    {Ratio: 0.004 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen2-math-7b-instruct":     {Ratio: 0.0005 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen2-math-1.5b-instruct":   {Ratio: 0.0003 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Qwen Coder Models (additional, 2025-11)
	"qwen2.5-coder-32b-instruct":  {Ratio: 0.002 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen2.5-coder-14b-instruct":  {Ratio: 0.001 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen2.5-coder-7b-instruct":   {Ratio: 0.0005 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen2.5-coder-3b-instruct":   {Ratio: 0.0003 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen2.5-coder-1.5b-instruct": {Ratio: 0.0003 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"qwen2.5-coder-0.5b-instruct": {Ratio: 0.0003 * 1000 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// DeepSeek Models (hosted on Ali)
	"deepseek-r1":                   {Ratio: 1 * ratio.MilliTokensRmb, CompletionRatio: 8},
	"deepseek-v3":                   {Ratio: 1 * ratio.MilliTokensRmb, CompletionRatio: 2},
	"deepseek-r1-distill-qwen-1.5b": {Ratio: 0.07 * ratio.MilliTokensRmb, CompletionRatio: 0.28},
	"deepseek-r1-distill-qwen-7b":   {Ratio: 0.14 * ratio.MilliTokensRmb, CompletionRatio: 0.28},
	"deepseek-r1-distill-qwen-14b":  {Ratio: 0.28 * ratio.MilliTokensRmb, CompletionRatio: 0.28},
	"deepseek-r1-distill-qwen-32b":  {Ratio: 0.42 * ratio.MilliTokensRmb, CompletionRatio: 0.28},
	"deepseek-r1-distill-llama-8b":  {Ratio: 0.14 * ratio.MilliTokensRmb, CompletionRatio: 0.28},
	"deepseek-r1-distill-llama-70b": {Ratio: 1 * ratio.MilliTokensRmb, CompletionRatio: 2},

	// Embedding Models
	"text-embedding-v1":       {Ratio: 0.5 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"text-embedding-v3":       {Ratio: 0.5 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"text-embedding-v2":       {Ratio: 0.5 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"text-embedding-async-v2": {Ratio: 0.5 * ratio.MilliTokensRmb, CompletionRatio: 1},
	"text-embedding-async-v1": {Ratio: 0.5 * ratio.MilliTokensRmb, CompletionRatio: 1},

	// Qwen-Image Models (synchronous multimodal-generation endpoint)
	// Pricing: https://help.aliyun.com/zh/model-studio/qwen-image-api
	// qwen-image-2.0-pro: 0.06 RMB/image (1024x1024)
	"qwen-image-2.0-pro": {
		Ratio:           60 * ratio.MilliTokensRmb,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 800,
			MinImages:        1,
			MaxImages:        6,
			SizeMultipliers: map[string]float64{
				"512x512":   0.25,
				"512x1024":  0.5,
				"1024x512":  0.5,
				"1024x1024": 1,
				"1024x2048": 2,
				"2048x1024": 2,
				"2048x2048": 4,
			},
		},
	},
	// qwen-image-2.0: 0.02 RMB/image (1024x1024)
	"qwen-image-2.0": {
		Ratio:           20 * ratio.MilliTokensRmb,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 800,
			MinImages:        1,
			MaxImages:        6,
			SizeMultipliers: map[string]float64{
				"512x512":   0.25,
				"512x1024":  0.5,
				"1024x512":  0.5,
				"1024x1024": 1,
				"1024x2048": 2,
				"2048x1024": 2,
				"2048x2048": 4,
			},
		},
	},
	// qwen-image-max: 0.16 RMB/image
	"qwen-image-max": {
		Ratio:           160 * ratio.MilliTokensRmb,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 800,
			MinImages:        1,
			MaxImages:        1,
			SizeMultipliers: map[string]float64{
				"1664x928":  1,
				"1472x1104": 1,
				"1328x1328": 1,
				"1104x1472": 1,
				"928x1664":  1,
			},
		},
	},
	// qwen-image-plus (async): 0.04 RMB/image
	"qwen-image-plus": {
		Ratio:           40 * ratio.MilliTokensRmb,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 500,
			MinImages:        1,
			MaxImages:        1,
			SizeMultipliers: map[string]float64{
				"1664x928":  1,
				"1472x1104": 1,
				"1328x1328": 1,
				"1104x1472": 1,
				"928x1664":  1,
			},
		},
	},
	// qwen-image (async): 0.04 RMB/image
	"qwen-image": {
		Ratio:           40 * ratio.MilliTokensRmb,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 500,
			MinImages:        1,
			MaxImages:        1,
			SizeMultipliers: map[string]float64{
				"1664x928":  1,
				"1472x1104": 1,
				"1328x1328": 1,
				"1104x1472": 1,
				"928x1664":  1,
			},
		},
	},

	// Legacy Image Generation Models
	"ali-stable-diffusion-xl": {
		Ratio:           8 * ratio.MilliTokensRmb,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        4,
			SizeMultipliers: map[string]float64{
				"512x1024":  1,
				"1024x768":  1,
				"1024x1024": 1,
				"576x1024":  1,
				"1024x576":  1,
			},
		},
	},
	"ali-stable-diffusion-v1.5": {
		Ratio:           8 * ratio.MilliTokensRmb,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        4,
			SizeMultipliers: map[string]float64{
				"512x1024":  1,
				"1024x768":  1,
				"1024x1024": 1,
				"576x1024":  1,
				"1024x576":  1,
			},
		},
	},
	"wanx-v1": {
		Ratio:           8 * ratio.MilliTokensRmb,
		CompletionRatio: 1,
		Image: &adaptor.ImagePricingConfig{
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        4,
			SizeMultipliers: map[string]float64{
				"1024x1024": 1,
				"720x1280":  1,
				"1280x720":  1,
			},
		},
	},
}

// AliToolingDefaults notes that Alibaba Model Studio does not expose public built-in tool pricing (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://help.aliyun.com/en/model-studio/developer-reference/tools-reference (requires authentication)
var AliToolingDefaults = adaptor.ChannelToolConfig{}
