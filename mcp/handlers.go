package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// addRelayAPITools registers all One-API relay endpoint tools with the MCP server.
// This method configures the server to provide documentation generation tools for
// various API endpoints including OpenAI-compatible APIs and Claude messages.
//
// The registered tools include:
//   - chat_completions: OpenAI Chat Completions API documentation
//   - completions: OpenAI Completions API documentation
//   - embeddings: OpenAI Embeddings API documentation
//   - images_generations: OpenAI Image Generation API documentation
//   - audio_transcriptions: OpenAI Audio Transcriptions API documentation
//   - audio_translations: OpenAI Audio Translations API documentation
//   - audio_speech: OpenAI Audio Speech API documentation
//   - moderations: OpenAI Moderations API documentation
//   - models_list: Models List API documentation
//   - claude_messages: Claude Messages API documentation
//
// Each tool is configured with:
//   - Appropriate input argument schemas with validation
//   - JSON schema descriptions for all parameters
//   - Required/optional parameter specifications
//   - Tool-specific documentation generation using the unified system
//
// The tools use the new GenerateDocumentation function with appropriate
// DocumentationType constants to ensure consistent and maintainable
// documentation generation across all endpoints.
//
// This method is called automatically during server initialization and
// should not be called directly by external code.
func (s *Server) addRelayAPITools() {
	// Chat Completions tool
	type ChatCompletionsArgs struct {
		Model       string           `json:"model" jsonschema_description:"ID of the model to use" jsonschema_required:"true"`
		Messages    []map[string]any `json:"messages" jsonschema_description:"Array of message objects" jsonschema_required:"true"`
		Temperature *float64         `json:"temperature,omitempty" jsonschema_description:"Sampling temperature between 0 and 2"`
		MaxTokens   *int             `json:"max_tokens,omitempty" jsonschema_description:"Maximum number of tokens to generate"`
		Stream      *bool            `json:"stream,omitempty" jsonschema_description:"Whether to stream responses"`
	}

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "chat_completions",
		Description: "Create a chat completion using OpenAI-compatible API. Supports streaming and non-streaming responses.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ChatCompletionsArgs) (*mcp.CallToolResult, any, error) {
		baseURL := getBaseURL()
		doc := generateChatCompletionsDocumentationFromTemplate(baseURL)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: doc},
			},
		}, nil, nil
	})

	// Completions tool
	type CompletionsArgs struct {
		Model       string   `json:"model" jsonschema_description:"ID of the model to use" jsonschema_required:"true"`
		Prompt      string   `json:"prompt" jsonschema_description:"The prompt to generate completions for" jsonschema_required:"true"`
		MaxTokens   *int     `json:"max_tokens,omitempty" jsonschema_description:"Maximum number of tokens to generate"`
		Temperature *float64 `json:"temperature,omitempty" jsonschema_description:"Sampling temperature"`
	}

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "completions",
		Description: "Create a text completion using OpenAI-compatible API.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CompletionsArgs) (*mcp.CallToolResult, any, error) {
		baseURL := getBaseURL()
		doc := generateCompletionsDocumentationFromTemplate(baseURL)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: doc},
			},
		}, nil, nil
	})

	// Embeddings tool
	type EmbeddingsArgs struct {
		Model string `json:"model" jsonschema_description:"ID of the model to use" jsonschema_required:"true"`
		Input string `json:"input" jsonschema_description:"Input text to embed" jsonschema_required:"true"`
	}

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "embeddings",
		Description: "Create embeddings for input text using OpenAI-compatible API.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args EmbeddingsArgs) (*mcp.CallToolResult, any, error) {
		baseURL := getBaseURL()
		doc := generateEmbeddingsDocumentationFromTemplate(baseURL)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: doc},
			},
		}, nil, nil
	})

	// Images Generation tool
	type ImagesArgs struct {
		Model  string `json:"model" jsonschema_description:"ID of the model to use" jsonschema_required:"true"`
		Prompt string `json:"prompt" jsonschema_description:"Text description of desired image" jsonschema_required:"true"`
		N      *int   `json:"n,omitempty" jsonschema_description:"Number of images to generate"`
		Size   string `json:"size,omitempty" jsonschema_description:"Size of generated images"`
	}

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "images_generations",
		Description: "Create images from text prompts using OpenAI-compatible API.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ImagesArgs) (*mcp.CallToolResult, any, error) {
		baseURL := getBaseURL()
		doc := generateImagesDocumentationFromTemplate(baseURL)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: doc},
			},
		}, nil, nil
	})

	// Audio Transcriptions tool
	type AudioTranscriptionsArgs struct {
		Model    string  `json:"model" jsonschema_description:"ID of the model to use" jsonschema_required:"true"`
		File     string  `json:"file" jsonschema_description:"Audio file to transcribe" jsonschema_required:"true"`
		Language *string `json:"language,omitempty" jsonschema_description:"Language of input audio"`
	}

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "audio_transcriptions",
		Description: "Transcribe audio into text using OpenAI-compatible API.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args AudioTranscriptionsArgs) (*mcp.CallToolResult, any, error) {
		baseURL := getBaseURL()
		doc := generateAudioTranscriptionsDocumentationFromTemplate(baseURL)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: doc},
			},
		}, nil, nil
	})

	// Audio Translations tool
	type AudioTranslationsArgs struct {
		Model string `json:"model" jsonschema_description:"ID of the model to use" jsonschema_required:"true"`
		File  string `json:"file" jsonschema_description:"Audio file to translate" jsonschema_required:"true"`
	}

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "audio_translations",
		Description: "Translate audio into English text using OpenAI-compatible API.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args AudioTranslationsArgs) (*mcp.CallToolResult, any, error) {
		baseURL := getBaseURL()
		doc := generateAudioTranslationsDocumentationFromTemplate(baseURL)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: doc},
			},
		}, nil, nil
	})

	// Audio Speech tool
	type AudioSpeechArgs struct {
		Model string `json:"model" jsonschema_description:"ID of the model to use" jsonschema_required:"true"`
		Input string `json:"input" jsonschema_description:"Text to generate audio for" jsonschema_required:"true"`
		Voice string `json:"voice" jsonschema_description:"Voice to use" jsonschema_required:"true"`
	}

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "audio_speech",
		Description: "Generate speech from text using OpenAI-compatible API.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args AudioSpeechArgs) (*mcp.CallToolResult, any, error) {
		baseURL := getBaseURL()
		doc := generateAudioSpeechDocumentationFromTemplate(baseURL)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: doc},
			},
		}, nil, nil
	})

	// Moderations tool
	type ModerationsArgs struct {
		Input string  `json:"input" jsonschema_description:"Text to classify" jsonschema_required:"true"`
		Model *string `json:"model,omitempty" jsonschema_description:"Moderation model to use"`
	}

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "moderations",
		Description: "Check if text violates OpenAI's usage policies using moderation API.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ModerationsArgs) (*mcp.CallToolResult, any, error) {
		baseURL := getBaseURL()
		doc := generateModerationsDocumentationFromTemplate(baseURL)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: doc},
			},
		}, nil, nil
	})

	// Models List tool
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "models_list",
		Description: "List available models through the One-API relay.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		baseURL := getBaseURL()
		doc := generateModelsListDocumentationFromTemplate(baseURL)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: doc},
			},
		}, nil, nil
	})

	// Claude Messages tool
	type ClaudeMessagesArgs struct {
		Model     string           `json:"model" jsonschema_description:"ID of Claude model to use" jsonschema_required:"true"`
		Messages  []map[string]any `json:"messages" jsonschema_description:"Array of message objects" jsonschema_required:"true"`
		MaxTokens int              `json:"max_tokens" jsonschema_description:"Maximum tokens to generate" jsonschema_required:"true"`
	}

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "claude_messages",
		Description: "Create messages using Claude API format.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ClaudeMessagesArgs) (*mcp.CallToolResult, any, error) {
		baseURL := getBaseURL()
		doc := generateClaudeMessagesDocumentationFromTemplate(baseURL)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: doc},
			},
		}, nil, nil
	})
}
