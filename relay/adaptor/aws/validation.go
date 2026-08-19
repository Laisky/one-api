package aws

import (
	"fmt"
	"net/http"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/model"
)

// UnsupportedParameter represents a parameter that is not supported by a provider.
type UnsupportedParameter struct {
	Name        string
	Description string
}

// ValidateUnsupportedParameters checks for unsupported parameters and returns
// an error if any are found. It uses model names instead of provider names so
// that capability lookups can be model-specific.
func ValidateUnsupportedParameters(request *model.GeneralOpenAIRequest, modelName string) *model.ErrorWithStatusCode {
	capabilities := GetModelCapabilities(modelName)
	var unsupportedParams []UnsupportedParameter

	// Check for tools support
	if len(request.Tools) > 0 && !capabilities.SupportsTools {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "tools",
			Description: "Tool calling is not supported by this model",
		})
	}

	// Check for tool_choice support
	if request.ToolChoice != nil && !capabilities.SupportsTools {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "tool_choice",
			Description: "Tool choice is not supported by this model",
		})
	}

	// Check for parallel_tool_calls support
	if request.ParallelTooCalls != nil && !capabilities.SupportsParallelToolCalls {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "parallel_tool_calls",
			Description: "Parallel tool calls are not supported by this model",
		})
	}

	// Check for functions support (deprecated OpenAI feature)
	if len(request.Functions) > 0 && !capabilities.SupportsFunctions {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "functions",
			Description: "Functions (deprecated OpenAI feature) are not supported by this model. Use 'tools' instead",
		})
	}

	// Check for function_call support
	if request.FunctionCall != nil && !capabilities.SupportsFunctions {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "function_call",
			Description: "Function call (deprecated OpenAI feature) is not supported by this model. Use 'tool_choice' instead",
		})
	}

	// Check for logprobs support
	if request.Logprobs != nil && *request.Logprobs && !capabilities.SupportsLogprobs {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "logprobs",
			Description: "Log probabilities are not supported by this model",
		})
	}

	// Check for top_logprobs support
	if request.TopLogprobs != nil && !capabilities.SupportsTopLogprobs {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "top_logprobs",
			Description: "Top log probabilities are not supported by this model",
		})
	}

	// Check for logit_bias support
	if request.LogitBias != nil && !capabilities.SupportsLogitBias {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "logit_bias",
			Description: "Logit bias is not supported by this model",
		})
	}

	// Check for response_format support
	if request.ResponseFormat != nil && !capabilities.SupportsResponseFormat {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "response_format",
			Description: "Response format is not supported by this model",
		})
	}

	// Check for reasoning_effort support
	if request.ReasoningEffort != nil && !capabilities.SupportsReasoningEffort {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "reasoning_effort",
			Description: "Reasoning effort is not supported by this model",
		})
	}

	// Check for modalities support
	if len(request.Modalities) > 0 && !capabilities.SupportsModalities {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "modalities",
			Description: "Modalities are not supported by this model",
		})
	}

	// Check for audio support
	if request.Audio != nil && !capabilities.SupportsAudio {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "audio",
			Description: "Audio input/output is not supported by this model",
		})
	}

	// Check for web_search_options support
	if request.WebSearchOptions != nil && !capabilities.SupportsWebSearch {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "web_search_options",
			Description: "Web search is not supported by this model",
		})
	}

	// Check for thinking support (Anthropic-specific)
	if request.Thinking != nil && !capabilities.SupportsThinking {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "thinking",
			Description: "Extended thinking is not supported by this model",
		})
	}

	// Check for service_tier support
	if request.ServiceTier != nil && !capabilities.SupportsServiceTier {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "service_tier",
			Description: "Service tier is not supported by this model",
		})
	}

	// Check for prediction support
	if request.Prediction != nil && !capabilities.SupportsPrediction {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "prediction",
			Description: "Prediction is not supported by this model",
		})
	}

	// Do not treat max_completion_tokens as unsupported; we'll map it to max_tokens if needed

	// Check for stop support
	if request.Stop != nil && !capabilities.SupportsStop {
		unsupportedParams = append(unsupportedParams, UnsupportedParameter{
			Name:        "stop",
			Description: "Stop parameter is not supported by this model",
		})
	}

	// If we found unsupported parameters, return an error
	if len(unsupportedParams) > 0 {
		var errorMessage string
		if len(unsupportedParams) == 1 {
			errorMessage = fmt.Sprintf("Unsupported parameter '%s': %s",
				unsupportedParams[0].Name, unsupportedParams[0].Description)
		} else {
			errorMessage = fmt.Sprintf("Unsupported parameters for model '%s':", modelName)
			for _, param := range unsupportedParams {
				errorMessage += fmt.Sprintf("\n- %s: %s", param.Name, param.Description)
			}
		}

		return &model.ErrorWithStatusCode{
			StatusCode: http.StatusBadRequest,
			Error: model.Error{
				Message:  errorMessage,
				Type:     model.ErrorTypeInvalidRequest,
				Code:     "unsupported_parameter",
				RawError: errors.New(errorMessage),
			},
		}
	}

	return nil
}
