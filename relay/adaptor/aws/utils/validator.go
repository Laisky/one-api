package utils

import (
	"fmt"
	"net/http"

	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// UnsupportedParameter represents a parameter that is not supported by a provider
type UnsupportedParameter struct {
	Name        string
	Description string
}

// ProviderCapabilities defines what features are supported by different AWS providers
type ProviderCapabilities struct {
	SupportsTools               bool
	SupportsFunctions           bool
	SupportsLogprobs            bool
	SupportsResponseFormat      bool
	SupportsReasoningEffort     bool
	SupportsModalities          bool
	SupportsAudio               bool
	SupportsWebSearch           bool
	SupportsThinking            bool
	SupportsLogitBias           bool
	SupportsServiceTier         bool
	SupportsParallelToolCalls   bool
	SupportsFrequencyPenalty    bool
	SupportsPresencePenalty     bool
	SupportsTopLogprobs         bool
	SupportsPrediction          bool
	SupportsMaxCompletionTokens bool
}

// GetProviderCapabilities returns the capabilities for different AWS providers
func GetProviderCapabilities(providerName string) ProviderCapabilities {
	switch providerName {
	case "claude", "anthropic":
		return ProviderCapabilities{
			SupportsTools:               true,  // Claude supports tools via Anthropic format
			SupportsFunctions:           false, // Claude doesn't support OpenAI functions
			SupportsLogprobs:            false,
			SupportsResponseFormat:      true, // Claude supports some response formats
			SupportsReasoningEffort:     false,
			SupportsModalities:          false,
			SupportsAudio:               false,
			SupportsWebSearch:           false,
			SupportsThinking:            true, // Claude supports thinking
			SupportsLogitBias:           false,
			SupportsServiceTier:         false,
			SupportsParallelToolCalls:   false,
			SupportsFrequencyPenalty:    false,
			SupportsPresencePenalty:     false,
			SupportsTopLogprobs:         false,
			SupportsPrediction:          false,
			SupportsMaxCompletionTokens: false,
		}
	case "deepseek":
		return ProviderCapabilities{
			SupportsTools:               false,
			SupportsFunctions:           false,
			SupportsLogprobs:            false,
			SupportsResponseFormat:      false,
			SupportsReasoningEffort:     true, // DeepSeek R1 supports reasoning
			SupportsModalities:          false,
			SupportsAudio:               false,
			SupportsWebSearch:           false,
			SupportsThinking:            false,
			SupportsLogitBias:           false,
			SupportsServiceTier:         false,
			SupportsParallelToolCalls:   false,
			SupportsFrequencyPenalty:    false,
			SupportsPresencePenalty:     false,
			SupportsTopLogprobs:         false,
			SupportsPrediction:          false,
			SupportsMaxCompletionTokens: false,
		}
	case "llama3", "llama":
		return ProviderCapabilities{
			SupportsTools:               true, // Llama3 supports tools via Converse API
			SupportsFunctions:           false,
			SupportsLogprobs:            false,
			SupportsResponseFormat:      false,
			SupportsReasoningEffort:     false,
			SupportsModalities:          false,
			SupportsAudio:               false,
			SupportsWebSearch:           false,
			SupportsThinking:            false,
			SupportsLogitBias:           false,
			SupportsServiceTier:         false,
			SupportsParallelToolCalls:   false,
			SupportsFrequencyPenalty:    false,
			SupportsPresencePenalty:     false,
			SupportsTopLogprobs:         false,
			SupportsPrediction:          false,
			SupportsMaxCompletionTokens: false,
		}
	case "mistral":
		return ProviderCapabilities{
			SupportsTools:               true, // Mistral supports tools
			SupportsFunctions:           false,
			SupportsLogprobs:            false,
			SupportsResponseFormat:      false,
			SupportsReasoningEffort:     false,
			SupportsModalities:          false,
			SupportsAudio:               false,
			SupportsWebSearch:           false,
			SupportsThinking:            false,
			SupportsLogitBias:           false,
			SupportsServiceTier:         false,
			SupportsParallelToolCalls:   false,
			SupportsFrequencyPenalty:    false,
			SupportsPresencePenalty:     false,
			SupportsTopLogprobs:         false,
			SupportsPrediction:          false,
			SupportsMaxCompletionTokens: false,
		}
	case "nova":
		return ProviderCapabilities{
			SupportsTools:               true, // Nova supports tools via Converse API
			SupportsFunctions:           false,
			SupportsLogprobs:            false,
			SupportsResponseFormat:      false,
			SupportsReasoningEffort:     false,
			SupportsModalities:          true, // Nova supports multimodal
			SupportsAudio:               false,
			SupportsWebSearch:           false,
			SupportsThinking:            false,
			SupportsLogitBias:           false,
			SupportsServiceTier:         false,
			SupportsParallelToolCalls:   false,
			SupportsFrequencyPenalty:    false,
			SupportsPresencePenalty:     false,
			SupportsTopLogprobs:         false,
			SupportsPrediction:          false,
			SupportsMaxCompletionTokens: false,
		}
	case "titan":
		return ProviderCapabilities{
			SupportsTools:               false,
			SupportsFunctions:           false,
			SupportsLogprobs:            false,
			SupportsResponseFormat:      false,
			SupportsReasoningEffort:     false,
			SupportsModalities:          false,
			SupportsAudio:               false,
			SupportsWebSearch:           false,
			SupportsThinking:            false,
			SupportsLogitBias:           false,
			SupportsServiceTier:         false,
			SupportsParallelToolCalls:   false,
			SupportsFrequencyPenalty:    false,
			SupportsPresencePenalty:     false,
			SupportsTopLogprobs:         false,
			SupportsPrediction:          false,
			SupportsMaxCompletionTokens: false,
		}
	default:
		// Default to minimal capabilities for unknown providers
		return ProviderCapabilities{}
	}
}

// ValidateUnsupportedParameters checks for unsupported parameters and returns an error if any are found
func ValidateUnsupportedParameters(request *relaymodel.GeneralOpenAIRequest, providerName string) *relaymodel.ErrorWithStatusCode {
	capabilities := GetProviderCapabilities(providerName)
	var unsupportedParams []UnsupportedParameter

	// Check for tools support
	if len(request.Tools) > 0 && !capabilities.SupportsTools {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "tools",
			Description: "Tool calling is not supported by this provider",
		})
	}

	// Check for tool_choice support
	if request.ToolChoice != nil && !capabilities.SupportsTools {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "tool_choice",
			Description: "Tool choice is not supported by this provider",
		})
	}

	// Check for parallel_tool_calls support
	if request.ParallelTooCalls != nil && !capabilities.SupportsParallelToolCalls {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "parallel_tool_calls",
			Description: "Parallel tool calls are not supported by this provider",
		})
	}

	// Check for functions support (deprecated OpenAI feature)
	if len(request.Functions) > 0 && !capabilities.SupportsFunctions {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "functions",
			Description: "Functions (deprecated OpenAI feature) are not supported by this provider. Use 'tools' instead",
		})
	}

	// Check for function_call support
	if request.FunctionCall != nil && !capabilities.SupportsFunctions {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "function_call",
			Description: "Function call (deprecated OpenAI feature) is not supported by this provider. Use 'tool_choice' instead",
		})
	}

	// Check for logprobs support
	if request.Logprobs != nil && *request.Logprobs && !capabilities.SupportsLogprobs {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "logprobs",
			Description: "Log probabilities are not supported by this provider",
		})
	}

	// Check for top_logprobs support
	if request.TopLogprobs != nil && !capabilities.SupportsTopLogprobs {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "top_logprobs",
			Description: "Top log probabilities are not supported by this provider",
		})
	}

	// Check for logit_bias support
	if request.LogitBias != nil && !capabilities.SupportsLogitBias {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "logit_bias",
			Description: "Logit bias is not supported by this provider",
		})
	}

	// Check for response_format support
	if request.ResponseFormat != nil && !capabilities.SupportsResponseFormat {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "response_format",
			Description: "Response format is not supported by this provider",
		})
	}

	// Check for reasoning_effort support
	if request.ReasoningEffort != nil && !capabilities.SupportsReasoningEffort {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "reasoning_effort",
			Description: "Reasoning effort is not supported by this provider",
		})
	}

	// Check for modalities support
	if len(request.Modalities) > 0 && !capabilities.SupportsModalities {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "modalities",
			Description: "Modalities are not supported by this provider",
		})
	}

	// Check for audio support
	if request.Audio != nil && !capabilities.SupportsAudio {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "audio",
			Description: "Audio input/output is not supported by this provider",
		})
	}

	// Check for web_search_options support
	if request.WebSearchOptions != nil && !capabilities.SupportsWebSearch {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "web_search_options",
			Description: "Web search is not supported by this provider",
		})
	}

	// Check for thinking support (Anthropic-specific)
	if request.Thinking != nil && !capabilities.SupportsThinking {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "thinking",
			Description: "Extended thinking is not supported by this provider",
		})
	}

	// Check for service_tier support
	if request.ServiceTier != nil && !capabilities.SupportsServiceTier {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "service_tier",
			Description: "Service tier is not supported by this provider",
		})
	}

	// Check for prediction support
	if request.Prediction != nil && !capabilities.SupportsPrediction {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "prediction",
			Description: "Prediction is not supported by this provider",
		})
	}

	// Check for max_completion_tokens support
	if request.MaxCompletionTokens != nil && !capabilities.SupportsMaxCompletionTokens {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "max_completion_tokens",
			Description: "max_completion_tokens is not supported by this provider. Use 'max_tokens' instead",
		})
	}

	// Check for frequency_penalty support
	if request.FrequencyPenalty != nil && !capabilities.SupportsFrequencyPenalty {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "frequency_penalty",
			Description: "Frequency penalty is not supported by this provider",
		})
	}

	// Check for presence_penalty support
	if request.PresencePenalty != nil && !capabilities.SupportsPresencePenalty {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "presence_penalty",
			Description: "Presence penalty is not supported by this provider",
		})
	}

	// If we found unsupported parameters, return an error
	if len(unsupportedParams) > 0 {
		var errorMessage string
		if len(unsupportedParams) == 1 {
			errorMessage = fmt.Sprintf("Unsupported parameter '%s': %s",
				unsupportedParams[0].Name, unsupportedParams[0].Description)
		} else {
			errorMessage = fmt.Sprintf("Unsupported parameters for provider '%s':", providerName)
			for _, param := range unsupportedParams {
				errorMessage += fmt.Sprintf("\n- %s: %s", param.Name, param.Description)
			}
		}

		return &relaymodel.ErrorWithStatusCode{
			StatusCode: http.StatusBadRequest,
			Error: relaymodel.Error{
				Message: errorMessage,
				Type:    "invalid_request_error",
				Code:    "unsupported_parameter",
			},
		}
	}

	return nil
}
