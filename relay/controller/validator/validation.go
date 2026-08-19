package validator

import (
	"math"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

func ValidateTextRequest(textRequest *model.GeneralOpenAIRequest, relayMode int) error {
	// Prefer max_completion_tokens; validate both for compatibility
	if textRequest.MaxCompletionTokens != nil {
		if *textRequest.MaxCompletionTokens < 0 || *textRequest.MaxCompletionTokens > math.MaxInt32/2 {
			return errors.New("max_completion_tokens is invalid")
		}
	}
	if textRequest.MaxTokens < 0 || textRequest.MaxTokens > math.MaxInt32/2 {
		return errors.New("max_tokens is invalid")
	}
	if textRequest.Model == "" {
		return errors.New("model is required")
	}
	switch relayMode {
	case relaymode.Completions:
		if textRequest.Prompt == "" {
			return errors.New("field prompt is required")
		}
	case relaymode.ChatCompletions:
		if len(textRequest.Messages) == 0 {
			return errors.New("field messages is required")
		}
	case relaymode.Embeddings:
	case relaymode.Moderations:
		if textRequest.Input == "" {
			return errors.New("field input is required")
		}
	case relaymode.Edits:
		if textRequest.Instruction == "" {
			return errors.New("field instruction is required")
		}
	}

	return nil
}

func ValidateRerankRequest(rerankRequest *model.RerankRequest) error {
	if rerankRequest == nil {
		return errors.New("request is nil")
	}

	if strings.TrimSpace(rerankRequest.Model) == "" {
		return errors.New("model is required")
	}
	if strings.TrimSpace(rerankRequest.Query) == "" {
		return errors.New("field query is required")
	}
	if len(rerankRequest.Documents) == 0 {
		return errors.New("field documents is required")
	}
	if rerankRequest.TopN != nil && *rerankRequest.TopN <= 0 {
		return errors.New("top_n must be greater than 0")
	}
	if rerankRequest.MaxTokensPerDoc != nil && *rerankRequest.MaxTokensPerDoc < 0 {
		return errors.New("max_tokens_per_doc must be >= 0")
	}
	if rerankRequest.Priority != nil {
		if *rerankRequest.Priority < 0 || *rerankRequest.Priority > 999 {
			return errors.New("priority must be between 0 and 999")
		}
	}

	return nil
}
