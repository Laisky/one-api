package aws

import relaymodel "github.com/Laisky/one-api/relay/model"

// Request represents a Writer-specific request structure for AWS Bedrock.
// It contains the essential parameters needed for Writer model inference,
// including conversation messages, generation parameters, and control options.
type Request struct {
	// Messages contains the conversation history including system, user, and assistant messages
	Messages []relaymodel.Message `json:"messages"`

	// Temperature controls randomness in the response generation (0.0 to 1.0)
	// Lower values make the output more deterministic
	Temperature *float64 `json:"temperature,omitempty"`

	// TopP controls nucleus sampling, affecting the diversity of the response
	// It represents the cumulative probability threshold for token selection
	TopP *float64 `json:"top_p,omitempty"`

	// MaxTokens specifies the maximum number of tokens to generate in the response
	MaxTokens int `json:"max_tokens,omitempty"`

	// Stop contains sequences that will cause the generation to stop when encountered
	Stop []string `json:"stop,omitempty"`
}
