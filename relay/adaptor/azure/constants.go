package azure

import "github.com/Laisky/one-api/relay/adaptor/openai"

// This file records the model catalog available through the Azure AI Foundry
// channel. The channel exposes two upstream surfaces on one resource, and this
// adaptor supports both:
//
//   - Azure OpenAI models (GPT / o-series / embeddings / image / audio) via the
//     Azure OpenAI surface (/openai/deployments/{deployment}/...). These mirror
//     OpenAI's catalog, so they are sourced from openai.ModelList and priced by
//     the embedded openai.Adaptor.
//   - Anthropic Claude models via the native Anthropic Messages surface
//     (/anthropic/v1/messages). These are enumerated in FoundryClaudeModels and
//     priced by the anthropic adaptor.
//
// Sources (retrieved 2026-07):
//   - https://learn.microsoft.com/en-us/azure/ai-foundry/openai/concepts/models
//   - https://learn.microsoft.com/en-us/azure/foundry/foundry-models/concepts/claude-models
//   - https://learn.microsoft.com/en-us/azure/foundry/foundry-models/concepts/models-sold-directly-by-azure
//
// Azure requires the request `model` field to be the user-chosen deployment name.
// one-api sends the (post-mapping) model name, so name the deployment after the
// model id below to get automatic routing and pricing; otherwise use the channel's
// model mapping plus a per-channel price override.

// FoundryClaudeModels enumerates the Anthropic Claude models sold directly by
// Azure in AI Foundry, served via the native Anthropic Messages API. Every id
// here has a matching entry in the anthropic adaptor's ModelRatios so pricing and
// the Claude-family routing predicate (meta.IsClaudeModelName) resolve correctly.
//
// The gated research preview "claude-mythos-preview" is intentionally omitted: it
// requires Microsoft Entra ID auth, is access-restricted, and has no published
// per-token price. Deploy it via a custom model entry + per-channel price if needed.
var FoundryClaudeModels = []string{
	// Generally available
	"claude-opus-4-8",
	"claude-opus-4-7",
	"claude-opus-4-6",
	"claude-opus-4-5",
	"claude-opus-4-1",
	"claude-sonnet-5",
	"claude-sonnet-4-6",
	"claude-sonnet-4-5",
	"claude-haiku-4-5",
	// Preview
	"claude-fable-5",
	// Gated research preview (priced)
	"claude-mythos-5",
}

// FoundryPartnerModels documents the other model families "sold directly by Azure"
// in AI Foundry that are reachable through an OpenAI-compatible Chat Completions
// surface, and therefore route through this adaptor's embedded Azure OpenAI path.
//
// They are deliberately NOT included in ModelList: one-api has no first-party
// pricing for them, so advertising them would imply a (wrong) default price.
// Operators can still deploy and use them on an Azure channel by adding the model
// and setting a per-channel price. Families served through provider-native /
// proprietary surfaces (Cohere rerank/embed, Mistral OCR, Black Forest FLUX,
// Microsoft MAI image) are not handled by this adaptor and are excluded entirely.
var FoundryPartnerModels = []string{
	// xAI Grok
	"grok-4.3", "grok-4", "grok-code-fast-1",
	// DeepSeek
	"DeepSeek-V4-Pro", "DeepSeek-V4-Flash", "DeepSeek-V3.2", "DeepSeek-R1",
	// Meta Llama
	"Llama-4-Maverick-17B-128E-Instruct-FP8", "Llama-3.3-70B-Instruct",
	// Mistral AI
	"Mistral-Large-3",
	// Moonshot AI
	"Kimi-K2.6",
	// Microsoft
	"model-router",
}

// ModelList is the set of models this adaptor advertises for the Azure channel:
// the full Azure OpenAI catalog plus the Azure AI Foundry Claude models. Both
// groups price correctly through the adaptor's per-family pricing dispatch.
var ModelList = func() []string {
	models := make([]string, 0, len(openai.ModelList)+len(FoundryClaudeModels))
	models = append(models, openai.ModelList...)
	models = append(models, FoundryClaudeModels...)
	return models
}()
