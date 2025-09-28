package mcp

import (
	"context"
	"fmt"
	"maps"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// addRelayAPITools registers all One-API relay endpoint tools with the MCP server.
// This method configures the server to provide documentation generation tools for
// various API endpoints including OpenAI Compatible-compatible APIs and Claude messages.
//
// The registered tools include:
//   - chat_completions: OpenAI Compatible Chat Completions API documentation
//   - completions: OpenAI Compatible Completions API documentation
//   - embeddings: OpenAI Compatible Embeddings API documentation
//   - images_generations: OpenAI Compatible Image Generation API documentation
//   - audio_transcriptions: OpenAI Compatible Audio Transcriptions API documentation
//   - audio_translations: OpenAI Compatible Audio Translations API documentation
//   - audio_speech: OpenAI Compatible Audio Speech API documentation
//   - moderations: OpenAI Compatible Moderations API documentation
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
		Description: "Generate comprehensive documentation for OpenAI Compatible Chat Completions API with your specific parameters. Creates detailed API documentation including endpoint URL, authentication, request/response examples, and parameter descriptions. Perfect for understanding how to integrate chat completions with your model, messages, temperature, and token settings.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ChatCompletionsArgs) (*mcp.CallToolResult, any, error) {
		baseURL := getBaseURL()

		// Convert args struct to map for template data
		params := map[string]any{
			"model":       args.Model,
			"messages":    args.Messages,
			"temperature": args.Temperature,
			"max_tokens":  args.MaxTokens,
			"stream":      args.Stream,
		}

		doc := GenerateDocumentationWithParams(ChatCompletions, baseURL, params)

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
		Description: "Generate detailed documentation for OpenAI Compatible Completions API with your specific prompt and parameters. Creates comprehensive API documentation including endpoint details, authentication headers, request examples with your exact prompt text, and parameter explanations for model, temperature, and max_tokens settings.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CompletionsArgs) (*mcp.CallToolResult, any, error) {
		baseURL := getBaseURL()

		// Convert args struct to map for template data
		params := map[string]any{
			"model":       args.Model,
			"prompt":      args.Prompt,
			"max_tokens":  args.MaxTokens,
			"temperature": args.Temperature,
		}

		doc := GenerateDocumentationWithParams(Completions, baseURL, params)

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
		Description: "Generate comprehensive documentation for OpenAI Compatible Embeddings API with your specific input text and model. Creates detailed API documentation showing how to convert your text into vector embeddings, including endpoint URL, authentication, request examples with your exact input text, and response format explanations.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args EmbeddingsArgs) (*mcp.CallToolResult, any, error) {
		baseURL := getBaseURL()

		// Convert args struct to map for template data
		params := map[string]any{
			"model": args.Model,
			"input": args.Input,
		}

		doc := GenerateDocumentationWithParams(Embeddings, baseURL, params)

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
		Description: "Generate detailed documentation for OpenAI Compatible Image Generation API with your specific prompt and settings. Creates comprehensive API documentation for generating images from text, including endpoint details, authentication, request examples with your exact prompt, and parameter explanations for model, size, and count options.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ImagesArgs) (*mcp.CallToolResult, any, error) {
		baseURL := getBaseURL()

		// Convert args struct to map for template data
		params := map[string]any{
			"model":  args.Model,
			"prompt": args.Prompt,
			"n":      args.N,
			"size":   args.Size,
		}

		doc := GenerateDocumentationWithParams(Images, baseURL, params)

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
		Description: "Generate comprehensive documentation for OpenAI Compatible Audio Transcriptions API with your specific file and settings. Creates detailed API documentation for converting audio to text, including endpoint URL, authentication, multipart form examples with your audio file, and parameter explanations for model, language, and file handling.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args AudioTranscriptionsArgs) (*mcp.CallToolResult, any, error) {
		baseURL := getBaseURL()

		// Convert args struct to map for template data
		params := map[string]any{
			"model":    args.Model,
			"file":     args.File,
			"language": args.Language,
		}

		doc := GenerateDocumentationWithParams(AudioTranscriptions, baseURL, params)

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
		Description: "Generate detailed documentation for OpenAI Compatible Audio Translations API with your specific audio file. Creates comprehensive API documentation for translating audio to English text, including endpoint details, authentication, multipart form examples with your audio file, and parameter explanations for model and file processing.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args AudioTranslationsArgs) (*mcp.CallToolResult, any, error) {
		baseURL := getBaseURL()

		// Convert args struct to map for template data
		params := map[string]any{
			"model": args.Model,
			"file":  args.File,
		}

		doc := GenerateDocumentationWithParams(AudioTranslations, baseURL, params)

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
		Description: "Generate comprehensive documentation for OpenAI Compatible Text-to-Speech API with your specific text and voice settings. Creates detailed API documentation for converting text to audio, including endpoint URL, authentication, request examples with your exact input text, and parameter explanations for model, voice options, and audio format settings.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args AudioSpeechArgs) (*mcp.CallToolResult, any, error) {
		baseURL := getBaseURL()

		// Convert args struct to map for template data
		params := map[string]any{
			"model": args.Model,
			"input": args.Input,
			"voice": args.Voice,
		}

		doc := GenerateDocumentationWithParams(AudioSpeech, baseURL, params)

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
		Description: "Generate detailed documentation for OpenAI Compatible Moderations API with your specific text input. Creates comprehensive API documentation for content moderation and policy compliance checking, including endpoint details, authentication, request examples with your exact text, and response format explanations for safety categories and scores.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ModerationsArgs) (*mcp.CallToolResult, any, error) {
		baseURL := getBaseURL()

		// Convert args struct to map for template data
		params := map[string]any{
			"input": args.Input,
			"model": args.Model,
		}

		doc := GenerateDocumentationWithParams(Moderations, baseURL, params)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: doc},
			},
		}, nil, nil
	})

	// Models List tool
	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "models_list",
		Description: "Generate comprehensive documentation for the Models List API endpoint. Creates detailed API documentation for retrieving available models through One-API relay, including endpoint URL, authentication headers, request examples, and response format showing model IDs, ownership, and capabilities information.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		baseURL := getBaseURL()

		// Convert args struct to map for template data (empty struct)
		params := map[string]any{}

		doc := GenerateDocumentationWithParams(ModelsList, baseURL, params)

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
		Description: "Generate comprehensive documentation for Claude Messages API with your specific conversation and settings. Creates detailed API documentation for Anthropic's Claude messaging format, including endpoint details, authentication, request examples with your exact messages array, and parameter explanations for model, max_tokens, and Claude-specific features.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ClaudeMessagesArgs) (*mcp.CallToolResult, any, error) {
		baseURL := getBaseURL()

		// Convert args struct to map for template data
		params := map[string]any{
			"model":      args.Model,
			"messages":   args.Messages,
			"max_tokens": args.MaxTokens,
		}

		doc := GenerateDocumentationWithParams(ClaudeMessages, baseURL, params)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: doc},
			},
		}, nil, nil
	})
}

// addInstructionTools registers instruction generation tools with the MCP server.
// This method configures the server to provide instruction generation capabilities
// for various instruction types including general usage, tool usage, API endpoints,
// error handling, and best practices.
//
// The instruction tool provides:
//   - Dynamic instruction generation based on instruction type
//   - Custom instruction support via server options
//   - Template-based instruction rendering
//   - Fallback instruction generation for unsupported types
//
// This method is called automatically during server initialization when
// instructions are enabled in ServerOptions.
func (s *Server) addInstructionTools() {
	// Instruction generation tool
	type InstructionArgs struct {
		Type          string         `json:"type,omitempty" jsonschema_description:"Type of instructions to generate (general, tool_usage, api_endpoints, error_handling, best_practices)"`
		CustomData    map[string]any `json:"custom_data,omitempty" jsonschema_description:"Custom data to pass to instruction template"`
		UseCustomText bool           `json:"use_custom_text,omitempty" jsonschema_description:"Use custom instruction text instead of template"`
		CustomText    string         `json:"custom_text,omitempty" jsonschema_description:"Custom instruction text (when use_custom_text is true)"`
	}

	mcp.AddTool(s.server, &mcp.Tool{
		Name:        "instructions",
		Description: "Generate comprehensive instructions for using this MCP server and its tools.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args InstructionArgs) (*mcp.CallToolResult, any, error) {
		// Use custom text if specified
		if args.UseCustomText && args.CustomText != "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: args.CustomText},
				},
			}, nil, nil
		}

		// Use custom instructions from server options if available
		if s.options.Instructions != nil && s.options.Instructions.CustomInstructions != "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: s.options.Instructions.CustomInstructions},
				},
			}, nil, nil
		}

		// Determine instruction type
		instructionType := GeneralInstructions // default
		if args.Type != "" {
			instructionType = InstructionType(args.Type)
		} else if s.options.Instructions != nil && s.options.Instructions.Type != "" {
			instructionType = s.options.Instructions.Type
		}

		// Prepare template data
		templateData := InstructionTemplateData{
			BaseURL:        s.getEffectiveBaseURL(),
			ServerName:     s.options.Name,
			ServerVersion:  s.options.Version,
			AvailableTools: s.getAvailableToolNames(),
			CustomData:     make(map[string]any),
		}

		// Add custom data from arguments
		if args.CustomData != nil {
			maps.Copy(templateData.CustomData, args.CustomData)
		}

		// Add custom data from server options
		if s.options.CustomTemplateData != nil {
			maps.Copy(templateData.CustomData, s.options.CustomTemplateData)
		}

		// Add custom data from instruction config
		if s.options.Instructions != nil && s.options.Instructions.TemplateData != nil {
			maps.Copy(templateData.CustomData, s.options.Instructions.TemplateData)
		}

		// Generate instructions using the global renderer
		if globalRenderer == nil {
			// Fallback if renderer is not available
			fallbackText := generateFallbackInstructions(string(instructionType), templateData)
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fallbackText},
				},
			}, nil, nil
		}

		instructionRenderer := globalRenderer.GetInstructionRenderer()
		if instructionRenderer == nil {
			// Fallback if instruction renderer is not available
			fallbackText := generateFallbackInstructions(string(instructionType), templateData)
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fallbackText},
				},
			}, nil, nil
		}

		instructions, err := instructionRenderer.GenerateInstructions(instructionType, templateData)
		if err != nil {
			// Fallback on error
			fallbackText := generateFallbackInstructions(string(instructionType), templateData)
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fallbackText},
				},
			}, nil, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: instructions},
			},
		}, nil, nil
	})
}

// generateFallbackInstructions provides basic fallback instructions when
// the instruction renderer is not available or fails.
func generateFallbackInstructions(instructionType string, data InstructionTemplateData) string {
	return fmt.Sprintf(`# MCP Server Instructions

## Server Information
- **Name**: %s
- **Version**: %s
- **Base URL**: %s

## Instruction Type: %s

The instruction template system is currently unavailable. Please refer to the server documentation or contact your administrator for detailed instructions.

## Available Tools
%s

## Basic Usage
1. Connect to this MCP server using a compatible client
2. Use the available tools to generate API documentation
3. Follow the generated documentation to integrate with One-API services

---
*Fallback instructions for %s v%s*`,
		data.ServerName,
		data.ServerVersion,
		data.BaseURL,
		instructionType,
		joinTools(data.AvailableTools),
		data.ServerName,
		data.ServerVersion)
}

// joinTools formats the available tools list for display
func joinTools(tools []string) string {
	if len(tools) == 0 {
		return "No tools available"
	}

	result := ""
	for _, tool := range tools {
		result += fmt.Sprintf("- %s\n", tool)
	}
	return result
}
