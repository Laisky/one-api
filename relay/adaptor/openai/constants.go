package openai

import (
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on OpenAI pricing: https://platform.openai.com/docs/pricing
var ModelRatios = map[string]adaptor.ModelConfig{
	// GPT-3.5 Models (Standard)
	// -------------------------------------

	"gpt-3.5-turbo":          {Ratio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 1.5 / 0.5},
	"gpt-3.5-turbo-0301":     {Ratio: 1.5 * ratio.MilliTokensUsd, CompletionRatio: 2.0 / 1.5},
	"gpt-3.5-turbo-0613":     {Ratio: 1.5 * ratio.MilliTokensUsd, CompletionRatio: 2.0 / 1.5},
	"gpt-3.5-turbo-1106":     {Ratio: 1.0 * ratio.MilliTokensUsd, CompletionRatio: 2.0 / 1.0},
	"gpt-3.5-turbo-0125":     {Ratio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 1.5 / 0.5},
	"gpt-3.5-turbo-16k":      {Ratio: 3.0 * ratio.MilliTokensUsd, CompletionRatio: 4.0 / 3.0},
	"gpt-3.5-turbo-16k-0613": {Ratio: 3.0 * ratio.MilliTokensUsd, CompletionRatio: 4.0 / 3.0},
	"gpt-3.5-turbo-instruct": {Ratio: 1.5 * ratio.MilliTokensUsd, CompletionRatio: 2.0 / 1.5},

	// GPT-4 Models (Standard)
	// -------------------------------------

	"gpt-4":                  {Ratio: 30.0 * ratio.MilliTokensUsd, CompletionRatio: 60.0 / 30.0},
	"gpt-4-0314":             {Ratio: 30.0 * ratio.MilliTokensUsd, CompletionRatio: 60.0 / 30.0},
	"gpt-4-0613":             {Ratio: 30.0 * ratio.MilliTokensUsd, CompletionRatio: 60.0 / 30.0},
	"gpt-4-32k":              {Ratio: 60.0 * ratio.MilliTokensUsd, CompletionRatio: 120.0 / 60.0},
	"gpt-4-32k-0314":         {Ratio: 60.0 * ratio.MilliTokensUsd, CompletionRatio: 120.0 / 60.0},
	"gpt-4-32k-0613":         {Ratio: 60.0 * ratio.MilliTokensUsd, CompletionRatio: 120.0 / 60.0},
	"gpt-4-1106-preview":     {Ratio: 10.0 * ratio.MilliTokensUsd, CompletionRatio: 30.0 / 10.0},
	"gpt-4-0125-preview":     {Ratio: 10.0 * ratio.MilliTokensUsd, CompletionRatio: 30.0 / 10.0},
	"gpt-4-turbo-preview":    {Ratio: 10.0 * ratio.MilliTokensUsd, CompletionRatio: 30.0 / 10.0},
	"gpt-4-vision-preview":   {Ratio: 10.0 * ratio.MilliTokensUsd, CompletionRatio: 30.0 / 10.0},
	"gpt-4-turbo":            {Ratio: 10.0 * ratio.MilliTokensUsd, CompletionRatio: 30.0 / 10.0},
	"gpt-4-turbo-2024-04-09": {Ratio: 10.0 * ratio.MilliTokensUsd, CompletionRatio: 30.0 / 10.0},

	// GPT-4o Models
	// -------------------------------------

	"gpt-4o":                 {Ratio: 2.5 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 1.25 * ratio.MilliTokensUsd},
	"gpt-4o-2024-05-13":      {Ratio: 5.0 * ratio.MilliTokensUsd, CompletionRatio: 3.0},
	"gpt-4o-2024-08-06":      {Ratio: 2.5 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 1.25 * ratio.MilliTokensUsd},
	"gpt-4o-2024-11-20":      {Ratio: 2.5 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 1.25 * ratio.MilliTokensUsd},
	"gpt-4o-mini":            {Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 0.075 * ratio.MilliTokensUsd},
	"gpt-4o-mini-2024-07-18": {Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 0.075 * ratio.MilliTokensUsd},
	"gpt-4o-mini-audio-preview": {
		Ratio:           0.15 * ratio.MilliTokensUsd,
		CompletionRatio: 4.0,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           10.0 / 0.15,
			CompletionRatio:       2,
			PromptTokensPerSecond: 10,
		},
	},
	"gpt-4o-mini-audio-preview-2024-12-17": {
		Ratio:           0.15 * ratio.MilliTokensUsd,
		CompletionRatio: 4.0,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           10.0 / 0.15,
			CompletionRatio:       2,
			PromptTokensPerSecond: 10,
		},
	},
	"gpt-4o-audio-preview": {
		Ratio:           2.5 * ratio.MilliTokensUsd,
		CompletionRatio: 4.0,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           16,
			CompletionRatio:       2,
			PromptTokensPerSecond: 10,
		},
	},
	"gpt-4o-audio-preview-2024-12-17": {
		Ratio:           2.5 * ratio.MilliTokensUsd,
		CompletionRatio: 4.0,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           16,
			CompletionRatio:       2,
			PromptTokensPerSecond: 10,
		},
	},
	"gpt-4o-audio-preview-2024-10-01": {
		Ratio:           2.5 * ratio.MilliTokensUsd,
		CompletionRatio: 4.0,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           40,
			CompletionRatio:       2,
			PromptTokensPerSecond: 10,
		},
	},
	"gpt-4o-audio-preview-2025-06-03": {
		Ratio:           2.5 * ratio.MilliTokensUsd,
		CompletionRatio: 4.0,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           16,
			CompletionRatio:       2,
			PromptTokensPerSecond: 10,
		},
	},

	// Realtime Models
	// -------------------------------------

	"gpt-4o-realtime-preview":                 {Ratio: 5.0 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 2.5 * ratio.MilliTokensUsd},
	"gpt-4o-realtime-preview-2025-06-03":      {Ratio: 5.0 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 2.5 * ratio.MilliTokensUsd},
	"gpt-4o-mini-realtime-preview":            {Ratio: 0.6 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd},
	"gpt-4o-mini-realtime-preview-2024-12-17": {Ratio: 0.6 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 0.3 * ratio.MilliTokensUsd},

	// GPT-4.5 Models
	// -------------------------------------

	"gpt-4.5-preview":            {Ratio: 75.0 * ratio.MilliTokensUsd, CompletionRatio: 2.0},
	"gpt-4.5-preview-2025-02-27": {Ratio: 75.0 * ratio.MilliTokensUsd, CompletionRatio: 2.0},

	// GPT-4.1 Models
	// -------------------------------------

	"gpt-4.1":                 {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 0.5 * ratio.MilliTokensUsd},
	"gpt-4.1-2025-04-14":      {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 0.5 * ratio.MilliTokensUsd},
	"gpt-4.1-mini":            {Ratio: 0.4 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 0.1 * ratio.MilliTokensUsd},
	"gpt-4.1-mini-2025-04-14": {Ratio: 0.4 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 0.1 * ratio.MilliTokensUsd},
	"gpt-4.1-nano":            {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 0.025 * ratio.MilliTokensUsd},
	"gpt-4.1-nano-2025-04-14": {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 0.025 * ratio.MilliTokensUsd},

	// GPT-5 Models
	// -------------------------------------

	"gpt-5.1":               {Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 10 / 1.25, CachedInputRatio: 0.125 * ratio.MilliTokensUsd},
	"gpt-5.1-2025-11-13":    {Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 10 / 1.25, CachedInputRatio: 0.125 * ratio.MilliTokensUsd},
	"gpt-5.1-chat-latest":   {Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 10 / 1.25, CachedInputRatio: 0.125 * ratio.MilliTokensUsd},
	"gpt-5.1-codex":         {Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 10 / 1.25, CachedInputRatio: 0.125 * ratio.MilliTokensUsd},
	"gpt-5.1-codex-mini":    {Ratio: 0.25 * ratio.MilliTokensUsd, CompletionRatio: 2 / 0.25, CachedInputRatio: 0.025 * ratio.MilliTokensUsd},
	"gpt-5.1-codex-max":     {Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 10 / 1.25, CachedInputRatio: 0.125 * ratio.MilliTokensUsd},
	"gpt-5-chat-latest":     {Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 10 / 1.25, CachedInputRatio: 0.125 * ratio.MilliTokensUsd},
	"gpt-5-codex":           {Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 10 / 1.25, CachedInputRatio: 0.125 * ratio.MilliTokensUsd},
	"gpt-5-pro":             {Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 120 / 15},
	"gpt-5-pro-2025-10-06":  {Ratio: 15 * ratio.MilliTokensUsd, CompletionRatio: 120 / 15},
	"gpt-5":                 {Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 10 / 1.25, CachedInputRatio: 0.125 * ratio.MilliTokensUsd},
	"gpt-5-2025-08-07":      {Ratio: 1.25 * ratio.MilliTokensUsd, CompletionRatio: 10 / 1.25, CachedInputRatio: 0.125 * ratio.MilliTokensUsd},
	"gpt-5-mini":            {Ratio: 0.25 * ratio.MilliTokensUsd, CompletionRatio: 2 / 0.25, CachedInputRatio: 0.025 * ratio.MilliTokensUsd},
	"gpt-5-mini-2025-08-07": {Ratio: 0.25 * ratio.MilliTokensUsd, CompletionRatio: 2 / 0.25, CachedInputRatio: 0.025 * ratio.MilliTokensUsd},
	"gpt-5-nano":            {Ratio: 0.05 * ratio.MilliTokensUsd, CompletionRatio: 0.4 / 0.05, CachedInputRatio: 0.005 * ratio.MilliTokensUsd},
	"gpt-5-nano-2025-08-07": {Ratio: 0.05 * ratio.MilliTokensUsd, CompletionRatio: 0.4 / 0.05, CachedInputRatio: 0.005 * ratio.MilliTokensUsd},

	// o1 Models
	// -------------------------------------

	"o1":                    {Ratio: 15.0 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 7.5 * ratio.MilliTokensUsd},
	"o1-2024-12-17":         {Ratio: 15.0 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 7.5 * ratio.MilliTokensUsd},
	"o1-pro":                {Ratio: 150.0 * ratio.MilliTokensUsd, CompletionRatio: 4.0},
	"o1-pro-2025-03-19":     {Ratio: 150.0 * ratio.MilliTokensUsd, CompletionRatio: 4.0},
	"o1-preview":            {Ratio: 15.0 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 7.5 * ratio.MilliTokensUsd},
	"o1-preview-2024-09-12": {Ratio: 15.0 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 7.5 * ratio.MilliTokensUsd},
	"o1-mini":               {Ratio: 1.1 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 0.55 * ratio.MilliTokensUsd},
	"o1-mini-2024-09-12":    {Ratio: 1.1 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 0.55 * ratio.MilliTokensUsd},

	// o3 Models
	// -------------------------------------

	"o3":                          {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 0.5 * ratio.MilliTokensUsd},
	"o3-2025-04-16":               {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 0.5 * ratio.MilliTokensUsd},
	"o3-mini":                     {Ratio: 1.1 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 0.55 * ratio.MilliTokensUsd},
	"o3-mini-2025-01-31":          {Ratio: 1.1 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 0.55 * ratio.MilliTokensUsd},
	"o3-pro":                      {Ratio: 20.0 * ratio.MilliTokensUsd, CompletionRatio: 4.0},
	"o3-pro-2025-06-10":           {Ratio: 20.0 * ratio.MilliTokensUsd, CompletionRatio: 4.0},
	"o3-deep-research":            {Ratio: 10.0 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 2.5 * ratio.MilliTokensUsd},
	"o3-deep-research-2025-06-26": {Ratio: 10.0 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 2.5 * ratio.MilliTokensUsd},

	// o4 Models
	// -------------------------------------

	"o4-mini":                          {Ratio: 1.1 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 0.275 * ratio.MilliTokensUsd},
	"o4-mini-2025-04-16":               {Ratio: 1.1 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 0.275 * ratio.MilliTokensUsd},
	"o4-mini-deep-research":            {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 0.5 * ratio.MilliTokensUsd},
	"o4-mini-deep-research-2025-06-26": {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 0.5 * ratio.MilliTokensUsd},

	// Codex Models
	// -------------------------------------

	"codex-mini-latest": {Ratio: 1.5 * ratio.MilliTokensUsd, CompletionRatio: 4.0, CachedInputRatio: 0.375 * ratio.MilliTokensUsd},

	// Search Models
	// -------------------------------------

	"gpt-4o-mini-search-preview":            {Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 4.0},
	"gpt-4o-mini-search-preview-2025-03-11": {Ratio: 0.15 * ratio.MilliTokensUsd, CompletionRatio: 4.0},
	"gpt-4o-search-preview":                 {Ratio: 2.5 * ratio.MilliTokensUsd, CompletionRatio: 4.0},
	"gpt-4o-search-preview-2025-03-11":      {Ratio: 2.5 * ratio.MilliTokensUsd, CompletionRatio: 4.0},

	// Computer Use Models
	// -------------------------------------

	"computer-use-preview":            {Ratio: 3.0 * ratio.MilliTokensUsd, CompletionRatio: 4.0},
	"computer-use-preview-2025-03-11": {Ratio: 3.0 * ratio.MilliTokensUsd, CompletionRatio: 4.0},

	// ChatGPT Models (Standard)
	// -------------------------------------

	"chatgpt-4o-latest": {Ratio: 5.0 * ratio.MilliTokensUsd, CompletionRatio: 15.0 / 5.0},

	// Embedding Models (Standard)
	// -------------------------------------

	"text-embedding-ada-002": {Ratio: 0.1 * ratio.MilliTokensUsd, CompletionRatio: 1.0},
	"text-embedding-3-small": {Ratio: 0.02 * ratio.MilliTokensUsd, CompletionRatio: 1.0},
	"text-embedding-3-large": {Ratio: 0.13 * ratio.MilliTokensUsd, CompletionRatio: 1.0},
	"text-curie-001":         {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 1.0},
	"text-babbage-001":       {Ratio: 0.5 * ratio.MilliTokensUsd, CompletionRatio: 1.0},
	"text-ada-001":           {Ratio: 0.4 * ratio.MilliTokensUsd, CompletionRatio: 1.0},
	"text-davinci-002":       {Ratio: 20.0 * ratio.MilliTokensUsd, CompletionRatio: 1.0},
	"text-davinci-003":       {Ratio: 20.0 * ratio.MilliTokensUsd, CompletionRatio: 1.0},
	"text-davinci-edit-001":  {Ratio: 20.0 * ratio.MilliTokensUsd, CompletionRatio: 1.0},
	"davinci-002":            {Ratio: 2.0 * ratio.MilliTokensUsd, CompletionRatio: 1.0},
	"babbage-002":            {Ratio: 0.4 * ratio.MilliTokensUsd, CompletionRatio: 1.0},

	// Moderation Models
	// -------------------------------------

	"text-moderation-latest":     {Ratio: 0.0 * ratio.MilliTokensUsd, CompletionRatio: 1.0}, // Free
	"text-moderation-stable":     {Ratio: 0.0 * ratio.MilliTokensUsd, CompletionRatio: 1.0}, // Free
	"text-moderation-007":        {Ratio: 0.0 * ratio.MilliTokensUsd, CompletionRatio: 1.0}, // Free
	"omni-moderation-latest":     {Ratio: 0.0 * ratio.MilliTokensUsd, CompletionRatio: 1.0}, // Free
	"omni-moderation-2024-09-26": {Ratio: 0.0 * ratio.MilliTokensUsd, CompletionRatio: 1.0}, // Free

	// Audio Models
	// -------------------------------------

	"whisper-1": {
		Ratio:           6.0 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0, // $0.006 per minute
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           16,
			CompletionRatio:       0,
			PromptTokensPerSecond: 10,
			UsdPerSecond:          0.0001,
		},
	},

	// TTS Models
	// -------------------------------------

	"tts-1":         {Ratio: 15.0 * ratio.MilliTokensUsd, CompletionRatio: 1.0}, // $15.00 per 1M characters
	"tts-1-1106":    {Ratio: 15.0 * ratio.MilliTokensUsd, CompletionRatio: 1.0}, // $15.00 per 1M characters
	"tts-1-hd":      {Ratio: 30.0 * ratio.MilliTokensUsd, CompletionRatio: 1.0}, // $30.00 per 1M characters
	"tts-1-hd-1106": {Ratio: 30.0 * ratio.MilliTokensUsd, CompletionRatio: 1.0}, // $30.00 per 1M characters
	"gpt-4o-transcribe": {
		Ratio:           2.5 * ratio.MilliTokensUsd,
		CompletionRatio: 4.0, // $2.50 input, $10.00 output per 1M tokens
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           6.0 / 2.5,
			CompletionRatio:       2,
			PromptTokensPerSecond: 10,
		},
	},
	"gpt-4o-mini-transcribe": {
		Ratio:           1.25 * ratio.MilliTokensUsd,
		CompletionRatio: 4.0, // $1.25 input, $5.00 output per 1M tokens
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           3.0 / 1.25,
			CompletionRatio:       2,
			PromptTokensPerSecond: 10,
		},
	},
	"gpt-4o-mini-tts": {Ratio: 0.6 * ratio.MilliTokensUsd, CompletionRatio: 20.0}, // $0.60 input, $12.00 output per 1M tokens

	// Image Generation Models (Standard)
	//
	// ⚠️ should also update relay/billing/ratio/image.go when changing these values.
	// -------------------------------------

	// Policy: If a model is billed per image only, set Ratio=0 and configure Image.PricePerImageUsd.
	// GPT Image models bill both prompt tokens and per-image output; keep Ratio in sync with prompt pricing while retaining Image.PricePerImageUsd for renders.
	"dall-e-2": {
		Ratio:           0,
		CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.016,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 1000,
			MinImages:        1,
			MaxImages:        10,
			QualitySizeMultipliers: map[string]map[string]float64{
				"default": {
					"256x256":   1,
					"512x512":   1.125,
					"1024x1024": 1.25,
				},
				"standard": {
					"256x256":   1,
					"512x512":   1.125,
					"1024x1024": 1.25,
				},
			},
		},
	},
	"dall-e-3": {
		Ratio:           0,
		CompletionRatio: 1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.04,
			DefaultSize:      "1024x1024",
			DefaultQuality:   "standard",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        1,
			QualitySizeMultipliers: map[string]map[string]float64{
				"default": {
					"1024x1024": 1,
					"1024x1792": 2,
					"1792x1024": 2,
				},
				"standard": {
					"1024x1024": 1,
					"1024x1792": 2,
					"1792x1024": 2,
				},
				"hd": {
					"1024x1024": 2,
					"1024x1792": 3,
					"1792x1024": 3,
				},
			},
		},
	},
	"gpt-image-1": {
		Ratio:            5.0 * ratio.MilliTokensUsd,
		CachedInputRatio: 1.25 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.011,
			DefaultSize:      "1024x1536",
			DefaultQuality:   "high",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        1,
			QualitySizeMultipliers: map[string]map[string]float64{
				"default": {
					"1024x1024": 1,
					"1024x1536": 16.0 / 11,
					"1536x1024": 16.0 / 11,
				},
				"low": {
					"1024x1024": 1,
					"1024x1536": 16.0 / 11,
					"1536x1024": 16.0 / 11,
				},
				"medium": {
					"1024x1024": 42.0 / 11,
					"1024x1536": 63.0 / 11,
					"1536x1024": 63.0 / 11,
				},
				"high": {
					"1024x1024": 167.0 / 11,
					"1024x1536": 250.0 / 11,
					"1536x1024": 250.0 / 11,
				},
				"auto": {
					"1024x1024": 167.0 / 11,
					"1024x1536": 250.0 / 11,
					"1536x1024": 250.0 / 11,
				},
			},
		},
	},
	"gpt-image-1-mini": {
		Ratio:            2.0 * ratio.MilliTokensUsd,
		CachedInputRatio: 0.2 * ratio.MilliTokensUsd,
		CompletionRatio:  1.0,
		Image: &adaptor.ImagePricingConfig{
			PricePerImageUsd: 0.005,
			DefaultSize:      "1024x1536",
			DefaultQuality:   "high",
			PromptTokenLimit: 4000,
			MinImages:        1,
			MaxImages:        1,
			QualitySizeMultipliers: map[string]map[string]float64{
				"default": {
					"1024x1024": 1,
					"1024x1536": 0.006 / 0.005,
					"1536x1024": 0.006 / 0.005,
				},
				"low": {
					"1024x1024": 1,
					"1024x1536": 0.006 / 0.005,
					"1536x1024": 0.006 / 0.005,
				},
				"medium": {
					"1024x1024": 0.011 / 0.005,
					"1024x1536": 0.015 / 0.005,
					"1536x1024": 0.015 / 0.005,
				},
				"high": {
					"1024x1024": 0.036 / 0.005,
					"1024x1536": 0.052 / 0.005,
					"1536x1024": 0.052 / 0.005,
				},
				"auto": {
					"1024x1024": 0.036 / 0.005,
					"1024x1536": 0.052 / 0.005,
					"1536x1024": 0.052 / 0.005,
				},
			},
		},
	},

	// Video Generation Models
	// -------------------------------------
	// Pricing sourced from OpenAI Sora preview documentation (USD per rendered second at 720p).
	"sora-2": {
		Video: &adaptor.VideoPricingConfig{
			PerSecondUsd:   0.10,
			BaseResolution: "1280x720",
		},
	},
	"sora-2-pro": {
		Video: &adaptor.VideoPricingConfig{
			PerSecondUsd:   0.50,
			BaseResolution: "1920x1080",
		},
	},
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// OpenAIToolingDefaults enumerates OpenAI's built-in tool whitelist and pricing (retrieved 2025-11-11).
// Source: https://r.jina.ai/https://platform.openai.com/docs/pricing#built-in-tools
var OpenAIToolingDefaults = adaptor.ChannelToolConfig{
	Pricing: map[string]adaptor.ToolPricingConfig{
		"code_interpreter":                 {UsdPerCall: 0.03},   // $0.03 per container session
		"file_search":                      {UsdPerCall: 0.0025}, // $2.50 per 1K tool calls
		"web_search":                       {UsdPerCall: 0.01},   // $10 per 1K tool calls
		"web_search_preview_reasoning":     {UsdPerCall: 0.01},   // Preview tier for reasoning models, $10 per 1K tool calls
		"web_search_preview_non_reasoning": {UsdPerCall: 0.025},  // Preview tier for non-reasoning models, $25 per 1K tool calls
	},
}
