/**
 * Model Capabilities Detection System
 *
 * This module provides runtime detection of AI model capabilities based on model names.
 * Originally implemented by h0llyw00dzz for development with self-hosted GPU providers
 * such as Hyperbolic and DeepInfra.
 *
 * ## Purpose
 * Different AI models and providers support different features and parameters. This system
 * automatically detects which capabilities a model supports based on its name pattern,
 * enabling the UI to show/hide relevant parameters and preventing API errors from
 * unsupported parameters.
 *
 * ## Architecture
 * 1. Model Type Detection: Identifies the model family (Claude, OpenAI, Llama, etc.)
 * 2. Capability Mapping: Returns which parameters each model type supports
 * 3. Special Cases: Handles provider-specific implementations (Hyperbolic, DeepInfra, Vercel)
 *
 * ## Supported Capabilities
 * - `supportsTools`: Function/tool calling (e.g., Claude, OpenAI, Cohere)
 * - `supportsThinking`: Extended reasoning mode (Claude Opus/Sonnet 4+, DeepSeek-R1, Qwen thinking models)
 * - `supportsStop`: Custom stop sequences for generation control
 * - `supportsReasoningEffort`: Reasoning effort parameter (DeepSeek v3.1+)
 * - `supportsLogprobs`: Log probabilities for token generation (OpenAI, DeepInfra, Vercel)
 * - `supportsTopK`: Top-K sampling (Cohere, Llama, Mistral, Google)
 * - `supportsTopP`: Top-P (nucleus) sampling (most models)
 * - `supportsFrequencyPenalty`: Penalize frequent tokens (OpenAI, Claude, most models)
 * - `supportsPresencePenalty`: Penalize repeated topics (OpenAI, Claude, most models)
 * - `supportsMaxCompletionTokens`: Max output tokens parameter (OpenAI, DeepInfra, Vercel, Hyperbolic)
 * - `supportsVision`: Image/multimodal input (GPT-4 Vision, Claude 3+, Gemini, Nova, etc.)
 *
 * ## Provider Detection
 * The system detects models from these providers:
 * - **Hyperbolic**: Models with prefixes like `openai/gpt-oss`, `qwen/qwen3-next`, `deepseek-ai/deepseek-r1`
 * - **DeepInfra**: Models with prefixes like `deepseek-ai/`, `qwen/`, `moonshotai/`, `nvidia/`
 * - **Vercel AI Gateway**: Models with prefix `alibaba/`
 * - **Direct APIs**: Claude, OpenAI, Cohere, Google, AWS (Nova), Writer (Palmyra)
 *
 * ## Maintenance Guide
 *
 * ### Adding a New Model Type
 * 1. Add detection logic in `getModelType()`:
 *    ```typescript
 *    if (lowerName.includes('newmodel')) return 'newmodel'
 *    ```
 * 2. Add capability mapping in `getModelCapabilities()`:
 *    ```typescript
 *    case 'newmodel':
 *      return {
 *        supportsTools: false,
 *        supportsThinking: false,
 *        // ... set all capabilities
 *      }
 *    ```
 *
 * ### Adding a New Provider
 * 1. Add provider detection in `getModelType()` before generic model checks:
 *    ```typescript
 *    if (lowerName.includes('provider-prefix/')) return 'provider'
 *    ```
 * 2. Add provider-specific capabilities with appropriate support levels
 *
 * ### Adding a New Capability
 * 1. Add to `ModelCapabilities` interface:
 *    ```typescript
 *    supportsNewFeature: boolean
 *    ```
 * 2. Update `getDefaultCapabilities()` with default value (usually `false`)
 * 3. Set capability for each model type in `getModelCapabilities()`
 * 4. Create helper function if capability varies within a model family
 *
 * ### Adding Model-Specific Features
 * If only certain models within a family support a feature:
 * 1. Create a helper function like `claudeSupportsThinking()` or `hyperbolicSupportsThinking()`
 * 2. Check for specific model name patterns
 * 3. Call the helper in the capability mapping
 *
 * ## Examples
 *
 * ```typescript
 * // Detect capabilities for Claude Opus 4
 * const caps = getModelCapabilities('claude-opus-4-20250514')
 * // Returns: supportsTools=true, supportsThinking=true, supportsTopP=true
 *
 * // Detect capabilities for Hyperbolic DeepSeek-R1
 * const caps = getModelCapabilities('deepseek-ai/deepseek-r1')
 * // Returns: supportsTools=true, supportsThinking=true, supportsLogprobs=true
 *
 * // Detect capabilities for GPT-4 Vision
 * const caps = getModelCapabilities('gpt-4o')
 * // Returns: supportsTools=true, supportsVision=true, supportsLogprobs=true
 * ```
 *
 * ## Important Notes
 * - Model detection is case-insensitive
 * - Provider-specific checks must come BEFORE generic model checks in `getModelType()`
 * - Vision support is detected separately via `modelSupportsVision()` due to complex patterns
 * - AWS OpenAI OSS models are checked before generic GPT detection to avoid misclassification
 * - Unknown models return default (minimal) capabilities for safety
 *
 * @see ModelCapabilities for capability definitions
 * @see getModelCapabilities for usage
 */

export interface ModelCapabilities {
  supportsTools: boolean;
  supportsThinking: boolean;
  supportsStop: boolean;
  supportsReasoningEffort: boolean;
  supportsLogprobs: boolean;
  supportsTopK: boolean;
  supportsTopP: boolean;
  supportsFrequencyPenalty: boolean;
  supportsPresencePenalty: boolean;
  supportsMaxCompletionTokens: boolean;
  supportsVision: boolean;
}

// Check if model is AWS OpenAI OSS model
const isOpenAIOSSModel = (modelName: string): boolean => {
  const lowerName = modelName.toLowerCase();
  return (
    lowerName.includes("gpt-oss-20b") || lowerName.includes("gpt-oss-120b")
  );
};

// Model type detection helper
const getModelType = (modelName: string): string => {
  const lowerName = modelName.toLowerCase();

  if (lowerName.includes("claude")) return "claude";
  if (lowerName.includes("cohere") || lowerName.includes("command"))
    return "cohere";
  // Check for Vercel AI Gateway hosted models - these should be handled by vercel type
  // since they have specific capabilities but are hosted through Vercel
  if (lowerName.includes("alibaba/")) return "vercel";
  // Check for DeepInfra hosted models - these should be handled by deepinfra type
  // since they have similar capabilities but are hosted through DeepInfra
  if (
    lowerName.includes("deepseek-ai/") ||
    lowerName.includes("qwen/") ||
    lowerName.includes("moonshotai/") ||
    lowerName.includes("baai/") ||
    lowerName.includes("nvidia/")
  )
    return "deepinfra";
  // Check for Hyperbolic hosted models - these should be handled by hyperbolic type
  // since they have similar capabilities but are hosted through Hyperbolic
  if (
    lowerName.includes("openai/gpt-oss") ||
    lowerName.includes("qwen/qwen3-next") ||
    lowerName.includes("qwen/qwen3-235b") ||
    lowerName.includes("qwen/qwq") ||
    lowerName.includes("deepseek-ai/deepseek-r1") ||
    lowerName.includes("deepseek-ai/deepseek-v3") ||
    lowerName.includes("moonshotai/kimi-k2")
  )
    return "hyperbolic";
  if (lowerName.includes("deepseek")) return "deepseek";
  if (lowerName.includes("llama")) return "llama";
  if (lowerName.includes("mistral") || lowerName.includes("mixtral"))
    return "mistral";
  if (lowerName.includes("nova")) return "nova";
  if (lowerName.includes("palmyra")) return "writer";
  // Check for AWS OpenAI OSS models first, before general GPT check
  if (isOpenAIOSSModel(modelName)) return "openai-oss";
  if (
    isOpenAIMediumOnlyReasoningModel(modelName) ||
    lowerName.startsWith("gpt-5")
  )
    return "openai";
  if (lowerName.includes("gpt")) return "openai";
  if (lowerName.includes("gemini")) return "google";

  return "unknown";
};

// Check if Claude model supports extended thinking
const claudeSupportsThinking = (modelName: string): boolean => {
  const lowerName = modelName.toLowerCase();

  // Supported models according to the issue and documentation:
  // - Claude Opus 4.1 (claude-opus-4-1-20250805)
  // - Claude Opus 4 (claude-opus-4-20250514)
  // - Claude Sonnet 4 (claude-sonnet-4-20250514)
  // - Claude Sonnet 3.7 (claude-3-7-sonnet-20250219)

  return (
    lowerName.includes("claude-opus-4-1-20250805") ||
    lowerName.includes("claude-opus-4-20250514") ||
    lowerName.includes("claude-sonnet-4-20250514") ||
    lowerName.includes("claude-3-7-sonnet-20250219") ||
    // More flexible patterns for model variations
    (lowerName.includes("claude") &&
      lowerName.includes("opus") &&
      lowerName.includes("4")) ||
    (lowerName.includes("claude") &&
      lowerName.includes("sonnet") &&
      lowerName.includes("4")) ||
    (lowerName.includes("claude") &&
      lowerName.includes("sonnet") &&
      lowerName.includes("3.7"))
  );
};

// Check if Hyperbolic model supports thinking
const hyperbolicSupportsThinking = (modelName: string): boolean => {
  const lowerName = modelName.toLowerCase();

  // Hyperbolic models that support thinking based on model names:
  // - Qwen3-Next-80B-A3B-Thinking (explicitly has "thinking" in name)
  // - DeepSeek-R1 models (reasoning models)

  return (
    lowerName.includes("thinking") ||
    lowerName.includes("deepseek-r1") ||
    lowerName.includes("qwen3-next-80b-a3b-thinking")
  );
};

// Check if DeepSeek model supports reasoning effort
const deepseekSupportsReasoningEffort = (modelName: string): boolean => {
  const lowerName = modelName.toLowerCase();

  // DeepSeek models that support reasoning_effort parameter:
  // - deepseek-v3.1: Supports reasoning effort

  return lowerName.includes("deepseek-v3.1");
};

export const isOpenAIMediumOnlyReasoningModel = (
  modelName: string
): boolean => {
  const lowerName = modelName.toLowerCase().trim();
  if (!lowerName) return false;

  if (lowerName.includes("gpt-5.1-chat")) {
    return true;
  }

  if (lowerName.startsWith("o")) {
    if (lowerName.length === 1) {
      return true;
    }

    const nextChar = lowerName.charAt(1);
    if ((nextChar >= "0" && nextChar <= "9") || nextChar === "-") {
      return true;
    }
  }

  return false;
};

// Check if OpenAI model supports reasoning effort
const openaiSupportsReasoningEffort = (modelName: string): boolean => {
  const lowerName = modelName.toLowerCase().trim();
  if (!lowerName) return false;

  if (
    isOpenAIMediumOnlyReasoningModel(modelName) ||
    lowerName.includes("deep-research")
  ) {
    return true;
  }

  if (lowerName.startsWith("gpt-5") && !lowerName.startsWith("gpt-5-chat")) {
    return true;
  }

  return false;
};

// Check if model supports vision/image input
const modelSupportsVision = (modelName: string): boolean => {
  const lowerName = modelName.toLowerCase();

  // Claude models - Most Claude 3+ models support vision
  if (lowerName.includes("claude")) {
    return (
      lowerName.includes("claude-3") ||
      lowerName.includes("claude-4") ||
      lowerName.includes("sonnet") ||
      lowerName.includes("opus") ||
      lowerName.includes("haiku")
    );
  }

  // OpenAI GPT models with vision
  if (lowerName.includes("gpt") || lowerName.includes("chatgpt")) {
    return (
      (lowerName.includes("gpt-4") &&
        (lowerName.includes("vision") ||
          lowerName.includes("gpt-4o") ||
          lowerName.includes("gpt-4-turbo") ||
          (!lowerName.includes("gpt-4-0613") &&
            !lowerName.includes("gpt-4-0314")))) || // Exclude older GPT-4 versions without vision
      lowerName.includes("chatgpt-4o") || // chatgpt-4o-latest and similar
      lowerName.includes("gpt-5")
    ); // gpt-5-chat-latest and similar
  }

  // Google Gemini models - Most support vision
  if (lowerName.includes("gemini")) {
    return (
      lowerName.includes("pro") ||
      lowerName.includes("ultra") ||
      lowerName.includes("flash") ||
      lowerName.includes("exp")
    );
  }

  // AWS Nova models - Support vision
  if (lowerName.includes("nova")) {
    return (
      lowerName.includes("lite") ||
      lowerName.includes("pro") ||
      lowerName.includes("micro") ||
      lowerName.includes("premier")
    );
  }

  // Llama4 vision models - New models with vision support
  if (lowerName.includes("llama4")) {
    return (
      lowerName.includes("llama4-maverick-17b-1m") ||
      lowerName.includes("llama4-scout-17b-3.5m")
    );
  }

  // Other vision models
  return (
    lowerName.includes("vision") ||
    lowerName.includes("pixtral") || // Mistral vision model
    lowerName.includes("llava") || // LLaVA vision models
    lowerName.includes("cogvlm")
  ); // CogVLM vision models
};

export const getModelCapabilities = (modelName: string): ModelCapabilities => {
  if (!modelName) return getDefaultCapabilities();

  const modelType = getModelType(modelName);

  switch (modelType) {
    case "claude":
      return {
        supportsTools: true,
        supportsThinking: claudeSupportsThinking(modelName),
        supportsStop: false,
        supportsReasoningEffort: false,
        supportsLogprobs: false,
        supportsTopK: false,
        supportsTopP: true,
        supportsFrequencyPenalty: true,
        supportsPresencePenalty: true,
        supportsMaxCompletionTokens: false,
        supportsVision: modelSupportsVision(modelName),
      };

    case "cohere":
      return {
        supportsTools: true,
        supportsThinking: false,
        supportsStop: true,
        supportsReasoningEffort: false,
        supportsLogprobs: false,
        supportsTopK: true,
        supportsTopP: true,
        supportsFrequencyPenalty: true,
        supportsPresencePenalty: true,
        supportsMaxCompletionTokens: false,
        supportsVision: modelSupportsVision(modelName),
      };

    case "deepseek":
      return {
        supportsTools: false,
        supportsThinking: false,
        supportsStop: true,
        supportsReasoningEffort: deepseekSupportsReasoningEffort(modelName),
        supportsLogprobs: false,
        supportsTopK: false,
        supportsTopP: true,
        supportsFrequencyPenalty: true,
        supportsPresencePenalty: true,
        supportsMaxCompletionTokens: false,
        supportsVision: modelSupportsVision(modelName),
      };

    case "llama":
      return {
        supportsTools: false,
        supportsThinking: false,
        supportsStop: true,
        supportsReasoningEffort: false,
        supportsLogprobs: false,
        supportsTopK: true,
        supportsTopP: true,
        supportsFrequencyPenalty: true,
        supportsPresencePenalty: true,
        supportsMaxCompletionTokens: false,
        supportsVision: modelSupportsVision(modelName),
      };

    case "mistral":
      return {
        supportsTools: false,
        supportsThinking: false,
        supportsStop: true,
        supportsReasoningEffort: false,
        supportsLogprobs: false,
        supportsTopK: true,
        supportsTopP: true,
        supportsFrequencyPenalty: true,
        supportsPresencePenalty: true,
        supportsMaxCompletionTokens: false,
        supportsVision: modelSupportsVision(modelName),
      };

    case "nova":
      return {
        supportsTools: false,
        supportsThinking: false,
        supportsStop: true,
        supportsReasoningEffort: false,
        supportsLogprobs: false,
        supportsTopK: false,
        supportsTopP: true,
        supportsFrequencyPenalty: false,
        supportsPresencePenalty: false,
        supportsMaxCompletionTokens: false,
        supportsVision: modelSupportsVision(modelName),
      };

    case "openai-oss":
      return {
        supportsTools: false,
        supportsThinking: false,
        supportsStop: false,
        supportsReasoningEffort: false,
        supportsLogprobs: false,
        supportsTopK: false,
        supportsTopP: true,
        supportsFrequencyPenalty: false,
        supportsPresencePenalty: false,
        supportsMaxCompletionTokens: false,
        supportsVision: modelSupportsVision(modelName),
      };

    case "openai":
      return {
        supportsTools: true,
        supportsThinking: false,
        supportsStop: true,
        supportsReasoningEffort: openaiSupportsReasoningEffort(modelName),
        supportsLogprobs: true,
        supportsTopK: false,
        supportsTopP: true,
        supportsFrequencyPenalty: true,
        supportsPresencePenalty: true,
        supportsMaxCompletionTokens: true,
        supportsVision: modelSupportsVision(modelName),
      };

    case "deepinfra":
      return {
        supportsTools: true,
        supportsThinking: false,
        supportsStop: true,
        supportsReasoningEffort: deepseekSupportsReasoningEffort(modelName),
        supportsLogprobs: true,
        supportsTopK: false,
        supportsTopP: true,
        supportsFrequencyPenalty: true,
        supportsPresencePenalty: true,
        supportsMaxCompletionTokens: true,
        supportsVision: modelSupportsVision(modelName),
      };

    case "vercel":
      return {
        supportsTools: true,
        supportsThinking: false,
        supportsStop: true,
        supportsReasoningEffort: false,
        supportsLogprobs: true,
        supportsTopK: false,
        supportsTopP: true,
        supportsFrequencyPenalty: true,
        supportsPresencePenalty: true,
        supportsMaxCompletionTokens: true,
        supportsVision: modelSupportsVision(modelName),
      };

    case "hyperbolic":
      return {
        supportsTools: true,
        supportsThinking: hyperbolicSupportsThinking(modelName),
        supportsStop: true,
        supportsReasoningEffort: deepseekSupportsReasoningEffort(modelName),
        supportsLogprobs: true,
        supportsTopK: false,
        supportsTopP: true,
        supportsFrequencyPenalty: true,
        supportsPresencePenalty: true,
        supportsMaxCompletionTokens: true,
        supportsVision: modelSupportsVision(modelName),
      };

    case "google":
      return {
        supportsTools: true,
        supportsThinking: false,
        supportsStop: true,
        supportsReasoningEffort: false,
        supportsLogprobs: false,
        supportsTopK: true,
        supportsTopP: true,
        supportsFrequencyPenalty: false,
        supportsPresencePenalty: false,
        supportsMaxCompletionTokens: true,
        supportsVision: modelSupportsVision(modelName),
      };

    case "writer":
      return {
        supportsTools: false,
        supportsThinking: false,
        supportsStop: true,
        supportsReasoningEffort: false,
        supportsLogprobs: false,
        supportsTopK: false,
        supportsTopP: true,
        supportsFrequencyPenalty: false,
        supportsPresencePenalty: false,
        supportsMaxCompletionTokens: false,
        supportsVision: modelSupportsVision(modelName),
      };

    default:
      return getDefaultCapabilities();
  }
};

const getDefaultCapabilities = (): ModelCapabilities => ({
  supportsTools: false,
  supportsThinking: false,
  supportsStop: false,
  supportsReasoningEffort: false,
  supportsLogprobs: false,
  supportsTopK: false,
  supportsTopP: true,
  supportsFrequencyPenalty: false,
  supportsPresencePenalty: false,
  supportsMaxCompletionTokens: false,
  supportsVision: false,
});
