package mcp

import (
	"context"
	"fmt"
	"maps"

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
