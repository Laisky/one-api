package model

import (
	"encoding/json"
	"mime/multipart"
)

// RequestProvider customize how your requests are routed using the provider object
// in the request body for Chat Completions and Completions.
//
// https://openrouter.ai/docs/features/provider-routing
type RequestProvider struct {
	// Order is list of provider names to try in order (e.g. ["Anthropic", "OpenAI"]). Default: empty
	Order []string `json:"order,omitempty"`
	// AllowFallbacks is whether to allow backup providers when the primary is unavailable. Default: true
	AllowFallbacks bool `json:"allow_fallbacks,omitempty"`
	// RequireParameters is only use providers that support all parameters in your request. Default: false
	RequireParameters bool `json:"require_parameters,omitempty"`
	// DataCollection is control whether to use providers that may store data ("allow" or "deny"). Default: "allow"
	DataCollection string `json:"data_collection,omitempty" binding:"omitempty,oneof=allow deny"`
	// Ignore is list of provider names to skip for this request. Default: empty
	Ignore []string `json:"ignore,omitempty"`
	// Quantizations is list of quantization levels to filter by (e.g. ["int4", "int8"]). Default: empty
	Quantizations []string `json:"quantizations,omitempty"`
	// Sort is sort providers by price or throughput (e.g. "price" or "throughput"). Default: empty
	Sort string `json:"sort,omitempty" binding:"omitempty,oneof=price throughput latency"`
}

type ResponseFormat struct {
	Type       string      `json:"type,omitempty"`
	JsonSchema *JSONSchema `json:"json_schema,omitempty"`
}

type JSONSchema struct {
	Description string         `json:"description,omitempty"`
	Name        string         `json:"name"`
	Schema      map[string]any `json:"schema,omitempty"`
	Strict      *bool          `json:"strict,omitempty"`
}

type Audio struct {
	Voice  string `json:"voice,omitempty"`
	Format string `json:"format,omitempty"`
}

type StreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

type GeneralOpenAIRequest struct {
	// https://platform.openai.com/docs/api-reference/chat/create
	Messages []Message `json:"messages,omitempty"`
	Model    string    `json:"model,omitempty"`
	Arn      string    `json:"arn,omitempty"` // for aws arn
	Store    *bool     `json:"store,omitempty"`
	Metadata any       `json:"metadata,omitempty"`
	// FrequencyPenalty is a number between -2.0 and 2.0 that penalizes
	// new tokens based on their existing frequency in the text so far,
	// default is 0.
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty" binding:"omitempty,min=-2,max=2"`
	LogitBias        any      `json:"logit_bias,omitempty"`
	Logprobs         *bool    `json:"logprobs,omitempty"`
	TopLogprobs      *int     `json:"top_logprobs,omitempty"`
	// MaxTokens is the maximum number of tokens to generate in the chat completion.
	//
	// MaxTokens is not deprecated; most open-source models use max_tokens, not max_completion_tokens.
	MaxTokens           int  `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int `json:"max_completion_tokens,omitempty"`
	// N is how many chat completion choices to generate for each input message,
	// default to 1.
	N *int `json:"n,omitempty" binding:"omitempty,min=0"`
	// ReasoningEffort constrains effort on reasoning for reasoning models, reasoning models only.
	// Supported values: low, medium, high, minimal (GPT-5 series also supports "minimal")
	ReasoningEffort *string `json:"reasoning_effort,omitempty" binding:"omitempty,oneof=low medium high minimal"`
	// Verbosity hints the model to be more or less expansive in its replies (GPT-5 series).
	// Supported values: low, medium, high
	Verbosity *string `json:"verbosity,omitempty" binding:"omitempty,oneof=low medium high"`
	// Modalities currently the model only programmatically allows modalities = ["text", "audio"]
	Modalities []string `json:"modalities,omitempty"`
	Prediction any      `json:"prediction,omitempty"`
	Audio      *Audio   `json:"audio,omitempty"`
	// PresencePenalty is a number between -2.0 and 2.0 that penalizes
	// new tokens based on whether they appear in the text so far, default is 0.
	PresencePenalty  *float64        `json:"presence_penalty,omitempty" binding:"omitempty,min=-2,max=2"`
	ResponseFormat   *ResponseFormat `json:"response_format,omitempty"`
	Seed             float64         `json:"seed,omitempty"`
	ServiceTier      *string         `json:"service_tier,omitempty" binding:"omitempty,oneof=default auto"`
	Stop             any             `json:"stop,omitempty"`
	Stream           bool            `json:"stream,omitempty"`
	StreamOptions    *StreamOptions  `json:"stream_options,omitempty"`
	Temperature      *float64        `json:"temperature,omitempty"`
	TopP             *float64        `json:"top_p,omitempty"`
	TopK             *int            `json:"top_k,omitempty"`
	Tools            []Tool          `json:"tools,omitempty"`
	ToolChoice       any             `json:"tool_choice,omitempty"`
	ParallelTooCalls *bool           `json:"parallel_tool_calls,omitempty"`
	User             string          `json:"user,omitempty"`
	FunctionCall     any             `json:"function_call,omitempty"`
	Functions        []Function      `json:"functions,omitempty"`
	// https://platform.openai.com/docs/api-reference/embeddings/create
	Input          any    `json:"input,omitempty"`
	EncodingFormat string `json:"encoding_format,omitempty"`
	Dimensions     int    `json:"dimensions,omitempty"`
	// https://platform.openai.com/docs/api-reference/images/create
	Prompt           string            `json:"prompt,omitempty"`
	Quality          *string           `json:"quality,omitempty"`
	Size             string            `json:"size,omitempty"`
	Style            *string           `json:"style,omitempty"`
	WebSearchOptions *WebSearchOptions `json:"web_search_options,omitempty"`

	// Others
	Instruction string `json:"instruction,omitempty"`
	NumCtx      int    `json:"num_ctx,omitempty"`
	// Duration is the length of the audio/video in seconds
	Duration *int `json:"duration,omitempty"`
	// -------------------------------------
	// Openrouter
	// -------------------------------------
	Provider         *RequestProvider `json:"provider,omitempty"`
	IncludeReasoning *bool            `json:"include_reasoning,omitempty"`
	// -------------------------------------
	// Anthropic
	// -------------------------------------
	Thinking *Thinking `json:"thinking,omitempty"`
	// -------------------------------------
	// Response API
	// -------------------------------------
	Reasoning *OpenAIResponseReasoning `json:"reasoning,omitempty" binding:"omitempty,oneof=auto concise detailed"`
}

type OpenAIResponseReasoning struct {
	// Effort defines the reasoning effort level
	Effort *string `json:"effort,omitempty" binding:"omitempty,oneof=low medium high"`
	// Summary defines whether to include a summary of the reasoning
	Summary *string `json:"summary,omitempty" binding:"omitempty,oneof=auto concise detailed"`
}

// WebSearchOptions is the tool searches the web for relevant results to use in a response.
type WebSearchOptions struct {
	// SearchContextSize is the high level guidance for the amount of context window space to use for the search,
	// default is "medium".
	SearchContextSize *string           `json:"search_context_size,omitempty" binding:"omitempty,oneof=low medium high"`
	UserLocation      *UserLocation     `json:"user_location,omitempty"`
	Filters           *WebSearchFilters `json:"filters,omitempty"`
}

// WebSearchFilters constrains the domains consulted by the OpenAI web search tool.
type WebSearchFilters struct {
	AllowedDomains    []string `json:"allowed_domains,omitempty"`
	DisallowedDomains []string `json:"disallowed_domains,omitempty"`
}

// UserLocation is a struct that contains the location of the user.
type UserLocation struct {
	// Approximate is the approximate location parameters for the search.
	Approximate *UserLocationApproximate `json:"approximate,omitempty"`
	// Type is the type of location approximation.
	Type string `json:"type,omitempty" binding:"omitempty,oneof=approximate"`
	// Optional flattened fields accepted directly by the Responses API web_search tool.
	Country  *string `json:"country,omitempty"`
	City     *string `json:"city,omitempty"`
	Region   *string `json:"region,omitempty"`
	Timezone *string `json:"timezone,omitempty"`
}

// UserLocationApproximate is a struct that contains the approximate location of the user.
type UserLocationApproximate struct {
	// City is the city of the user, e.g. San Francisco.
	City *string `json:"city,omitempty"`
	// Country is the country of the user, e.g. US.
	Country *string `json:"country,omitempty"`
	// Region is the region of the user, e.g. California.
	Region *string `json:"region,omitempty"`
	// Timezone is the IANA timezone of the user, e.g. America/Los_Angeles.
	Timezone *string `json:"timezone,omitempty"`
}

// https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking#implementing-extended-thinking
type Thinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens" binding:"omitempty,min=1024"`
}

func (r GeneralOpenAIRequest) ParseInput() []string {
	if r.Input == nil {
		return nil
	}
	var input []string
	switch r.Input.(type) {
	case string:
		input = []string{r.Input.(string)}
	case []any:
		input = make([]string, 0, len(r.Input.([]any)))
		for _, item := range r.Input.([]any) {
			if str, ok := item.(string); ok {
				input = append(input, str)
			}
		}
	}
	return input
}

// OpenaiImageEditRequest is the request body for the OpenAI image edit API.
type OpenaiImageEditRequest struct {
	Image          *multipart.FileHeader `json:"image" form:"image" binding:"required"`
	Prompt         string                `json:"prompt" form:"prompt" binding:"required"`
	Mask           *multipart.FileHeader `json:"mask" form:"mask" binding:"required"`
	Model          string                `json:"model" form:"model" binding:"required"`
	N              int                   `json:"n" form:"n" binding:"min=0,max=10"`
	Size           string                `json:"size" form:"size"`
	ResponseFormat string                `json:"response_format" form:"response_format"`
	// -------------------------------------
	// Imagen-3
	// -------------------------------------
	EditMode *string `json:"edit_mode" form:"edit_mode"`
	MaskMode *string `json:"mask_mode" form:"mask_mode"`
}

// ClaudeRequest represents a Claude Messages API request
// This is a flexible structure that can handle both simple and complex content
type ClaudeRequest struct {
	Model         string          `json:"model" binding:"required"`
	MaxTokens     int             `json:"max_tokens" binding:"required"`
	Messages      []ClaudeMessage `json:"messages" binding:"required"`
	System        any             `json:"system,omitempty"`
	Temperature   *float64        `json:"temperature,omitempty"`
	TopP          *float64        `json:"top_p,omitempty"`
	TopK          *int            `json:"top_k,omitempty"`
	Stream        *bool           `json:"stream,omitempty"`
	StopSequences []string        `json:"stop_sequences,omitempty"`
	Tools         []ClaudeTool    `json:"tools,omitempty"`
	ToolChoice    any             `json:"tool_choice,omitempty"`
	Thinking      *Thinking       `json:"thinking,omitempty"`
	Metadata      any             `json:"metadata,omitempty"`
}

// ClaudeMessage represents a message that can have either string or structured content
type ClaudeMessage struct {
	Role    string `json:"role" binding:"required"`
	Content any    `json:"content" binding:"required"`
}

// ClaudeTool represents a tool definition in the Claude Messages API
type ClaudeTool struct {
	Type        string `json:"type,omitempty"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema,omitempty"`
}

// Claude Messages API Response Types
type ClaudeResponse struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	Role         string          `json:"role"`
	Model        string          `json:"model"`
	Content      []ClaudeContent `json:"content"`
	StopReason   string          `json:"stop_reason"`
	StopSequence *string         `json:"stop_sequence,omitempty"`
	Usage        ClaudeUsage     `json:"usage"`
}

type ClaudeContent struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ID       string          `json:"id,omitempty"`
	Name     string          `json:"name,omitempty"`
	Input    json.RawMessage `json:"input,omitempty"`
	Thinking string          `json:"thinking,omitempty"`
}

type ClaudeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}
