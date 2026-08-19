package aws

import (
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// Request is the request for AWS Llama using Converse API
type Request struct {
	// Messages contains the conversation history using the relay model format.
	// This field is required and must contain at least one message.
	// Llama models process these messages through AWS Bedrock's Converse API
	// to generate contextually aware responses with high performance.
	Messages []relaymodel.Message `json:"messages"`

	// MaxTokens specifies the maximum number of tokens to generate in the response.
	// Optional field that helps control response length and API costs.
	// Llama models use this to limit generation while maintaining coherent responses
	// across different model sizes and conversation contexts.
	MaxTokens int `json:"max_tokens,omitempty"`

	// Temperature controls the randomness of the model's responses.
	// Range: 0.0 to 1.0, where 0.0 is deterministic and 1.0 is most random.
	// Optional field, uses model default if not specified.
	// Llama models maintain coherence and quality across different temperature values,
	// making them suitable for both creative and analytical tasks.
	Temperature *float64 `json:"temperature,omitempty"`

	// TopP controls nucleus sampling, limiting the cumulative probability of token choices.
	// Range: 0.0 to 1.0, where lower values make responses more focused.
	// Optional field, uses model default if not specified.
	// Optimized for Llama's text generation capabilities and conversation quality.
	TopP *float64 `json:"top_p,omitempty"`

	// Stop contains custom strings that will stop generation when encountered.
	// Optional field that allows fine-grained control over response termination.
	// Useful for controlling when Llama models stop generating in specific contexts,
	// supporting both conversational and task-specific applications.
	Stop []string `json:"stop,omitempty"`
}
