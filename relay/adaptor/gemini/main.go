package gemini

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/image"
	"github.com/songquanpeng/one-api/common/random"
	"github.com/songquanpeng/one-api/common/render"
	"github.com/songquanpeng/one-api/common/tracing"
	"github.com/songquanpeng/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/constant"
	"github.com/songquanpeng/one-api/relay/model"
)

// https://ai.google.dev/docs/gemini_api_overview?hl=zh-cn

const (
	VisionMaxImageNum = 16
)

var mimeTypeMap = map[string]string{
	"json_object": "application/json",
	"text":        "text/plain",
}

// cleanJsonSchemaForGemini removes unsupported fields and converts types for Gemini API compatibility
func cleanJsonSchemaForGemini(schema any) any {
	switch v := schema.(type) {
	case map[string]any:
		cleaned := make(map[string]any)

		// List of supported fields in Gemini (from official documentation)
		supportedFields := map[string]bool{
			"anyOf": true, "enum": true, "format": true, "items": true,
			"maximum": true, "maxItems": true, "minimum": true, "minItems": true,
			"nullable": true, "properties": true, "propertyOrdering": true,
			"required": true, "type": true,
		}

		// Type mapping from lowercase to uppercase (Gemini requirement)
		typeMapping := map[string]string{
			"object":  "OBJECT",
			"array":   "ARRAY",
			"string":  "STRING",
			"number":  "NUMBER",
			"integer": "INTEGER",
			"boolean": "BOOLEAN",
			"null":    "NULL",
		}

		// Format mapping from OpenAI to Gemini supported formats
		// Based on error message: only 'enum' and 'date-time' are supported for STRING type
		formatMapping := map[string]string{
			"date":      "date-time", // Convert unsupported "date" to "date-time"
			"time":      "date-time", // Convert unsupported "time" to "date-time"
			"date-time": "date-time", // Keep supported "date-time"
			"duration":  "date-time", // Convert to supported format
			"enum":      "enum",      // Keep supported "enum"
		}

		for key, value := range v {
			// Skip unsupported fields like additionalProperties, description, strict
			if !supportedFields[key] {
				continue
			}

			switch key {
			case "type":
				// Convert type to uppercase if it's a string
				if typeStr, ok := value.(string); ok {
					if mappedType, exists := typeMapping[strings.ToLower(typeStr)]; exists {
						cleaned[key] = mappedType
					} else {
						cleaned[key] = strings.ToUpper(typeStr)
					}
				} else {
					cleaned[key] = value
				}
			case "format":
				// Map format values to Gemini-supported formats
				if formatStr, ok := value.(string); ok {
					if mappedFormat, exists := formatMapping[formatStr]; exists {
						cleaned[key] = mappedFormat
					}
					// Skip unsupported formats that have no mapping
				}
			case "properties":
				// Handle properties object - recursively clean each property
				if props, ok := value.(map[string]any); ok {
					cleanedProps := make(map[string]any)
					for propKey, propValue := range props {
						cleanedProps[propKey] = cleanJsonSchemaForGemini(propValue)
					}
					cleaned[key] = cleanedProps
				} else {
					cleaned[key] = value
				}
			case "items":
				// Handle array items schema - recursively clean
				cleaned[key] = cleanJsonSchemaForGemini(value)
			default:
				// For other supported fields, recursively clean if they're objects/arrays
				cleaned[key] = cleanJsonSchemaForGemini(value)
			}
		}
		return cleaned
	case []any:
		// Clean arrays recursively
		cleaned := make([]any, len(v))
		for i, item := range v {
			cleaned[i] = cleanJsonSchemaForGemini(item)
		}
		return cleaned
	default:
		// Return primitive values as-is
		return v
	}
}

// cleanFunctionParameters recursively removes additionalProperties and other unsupported fields from function parameters
func cleanFunctionParameters(params any) any {
	cleaned := cleanFunctionParametersInternal(params, true)

	// Gemini function declarations require parameters.type to be OBJECT (uppercase).
	// Force the top-level schema type to OBJECT to avoid Vertex 400 errors.
	if cleanedMap, ok := cleaned.(map[string]any); ok {
		cleanedMap["type"] = "OBJECT"
		return cleanedMap
	}

	return cleaned
}

// cleanFunctionParametersInternal recursively removes additionalProperties and other unsupported fields from function parameters
// isTopLevel indicates if we're at the top level where description and strict should be removed
func cleanFunctionParametersInternal(params any, isTopLevel bool) any {
	switch v := params.(type) {
	case map[string]any:
		cleaned := make(map[string]any)

		// Format mapping from OpenAI to Gemini supported formats
		// Based on error message: only 'enum' and 'date-time' are supported for STRING type
		formatMapping := map[string]string{
			"date":      "date-time", // Convert unsupported "date" to "date-time"
			"time":      "date-time", // Convert unsupported "time" to "date-time"
			"date-time": "date-time", // Keep supported "date-time"
			"duration":  "date-time", // Convert to supported format
			"enum":      "enum",      // Keep supported "enum"
		}

		for key, value := range v {
			// Skip additionalProperties at all levels
			if key == "additionalProperties" {
				continue
			}
			// Remove $schema markers; Gemini rejects JSON Schema meta keys
			if key == "$schema" {
				continue
			}
			// Skip description and strict only at top level
			if isTopLevel && (key == "description" || key == "strict") {
				continue
			}

			// Handle format field - map to supported formats
			if key == "format" {
				if formatStr, ok := value.(string); ok {
					if mappedFormat, exists := formatMapping[formatStr]; exists {
						cleaned[key] = mappedFormat
					}
					// Skip unsupported formats that have no mapping
				}
				continue
			}

			// Recursively clean nested objects (not top level anymore)
			cleaned[key] = cleanFunctionParametersInternal(value, false)
		}
		return cleaned
	case []any:
		// Clean arrays recursively
		cleaned := make([]any, len(v))
		for i, item := range v {
			cleaned[i] = cleanFunctionParametersInternal(item, false)
		}
		return cleaned
	default:
		// Return primitive values as-is
		return v
	}
}

// ConvertRequest converts an OpenAI-compatible chat completion request to Gemini's native ChatRequest format.
// It transforms messages, handles tool definitions, sets safety settings based on configuration,
// and configures generation parameters like temperature, top_p, max_tokens, etc.
// Safety settings are configured to the lowest possible thresholds by default.
func ConvertRequest(textRequest model.GeneralOpenAIRequest) *ChatRequest {
	geminiRequest := ChatRequest{
		Contents: make([]ChatContent, 0, len(textRequest.Messages)),
		SafetySettings: []ChatSafetySettings{
			{
				Category:  "HARM_CATEGORY_HARASSMENT",
				Threshold: config.GeminiSafetySetting,
			},
			{
				Category:  "HARM_CATEGORY_HATE_SPEECH",
				Threshold: config.GeminiSafetySetting,
			},
			{
				Category:  "HARM_CATEGORY_SEXUALLY_EXPLICIT",
				Threshold: config.GeminiSafetySetting,
			},
			{
				Category:  "HARM_CATEGORY_DANGEROUS_CONTENT",
				Threshold: config.GeminiSafetySetting,
			},
			{
				Category:  "HARM_CATEGORY_CIVIC_INTEGRITY",
				Threshold: config.GeminiSafetySetting,
			},
		},
		GenerationConfig: ChatGenerationConfig{
			Temperature:        textRequest.Temperature,
			TopP:               textRequest.TopP,
			MaxOutputTokens:    textRequest.MaxTokens,
			ResponseModalities: geminiOpenaiCompatible.GetModelModalities(textRequest.Model),
		},
	}

	if geminiRequest.GenerationConfig.MaxOutputTokens == 0 {
		geminiRequest.GenerationConfig.MaxOutputTokens = config.DefaultMaxToken
	}
	if textRequest.ResponseFormat != nil {
		if mimeType, ok := mimeTypeMap[textRequest.ResponseFormat.Type]; ok {
			geminiRequest.GenerationConfig.ResponseMimeType = mimeType
		}
		if textRequest.ResponseFormat.JsonSchema != nil {
			// Clean the schema to remove unsupported properties for Gemini
			cleanedSchema := cleanJsonSchemaForGemini(textRequest.ResponseFormat.JsonSchema.Schema)
			geminiRequest.GenerationConfig.ResponseSchema = cleanedSchema
			geminiRequest.GenerationConfig.ResponseMimeType = mimeTypeMap["json_object"]
		}
	}

	// remove temperature & top_p for Gemini image models; only enforce response modalities when allowed
	if strings.Contains(strings.ToLower(textRequest.Model), "-image") {
		geminiRequest.GenerationConfig.Temperature = nil
		geminiRequest.GenerationConfig.TopP = nil
		if geminiRequest.GenerationConfig.ResponseModalities != nil {
			geminiRequest.GenerationConfig.ResponseModalities = []string{"TEXT", "IMAGE"}
		}
	}

	// FIX(https://github.com/Laisky/one-api/issues/60):
	// Gemini's function call supports fewer parameters than OpenAI's,
	// so a conversion is needed here to keep only the parameters supported by Gemini.
	if textRequest.Tools != nil {
		convertedGeminiFunctions := make([]model.Function, 0, len(textRequest.Tools))
		for _, tool := range textRequest.Tools {
			// Use the helper function to recursively clean function parameters
			cleanedParams := cleanFunctionParameters(tool.Function.Parameters)
			// Type assert to map[string]any
			cleanedParamsMap, ok := cleanedParams.(map[string]any)
			if !ok {
				// If type assertion fails, fallback to original parameters without additionalProperties
				cleanedParamsMap = make(map[string]any)
				if originalParams, ok := tool.Function.Parameters.(map[string]any); ok {
					for k, v := range originalParams {
						if k != "additionalProperties" && k != "description" && k != "strict" {
							cleanedParamsMap[k] = v
						}
					}
				}
			}
			convertedGeminiFunctions = append(convertedGeminiFunctions, model.Function{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  cleanedParamsMap,
				Required:    tool.Function.Required,
			})
		}
		geminiRequest.Tools = []ChatTools{
			{
				FunctionDeclarations: convertedGeminiFunctions,
			},
		}
	} else if textRequest.Functions != nil {
		for _, function := range textRequest.Functions {
			// Use the helper function to recursively clean function parameters
			cleanedParams := cleanFunctionParameters(function.Parameters)
			// Type assert to map[string]any
			cleanedParamsMap, ok := cleanedParams.(map[string]any)
			if !ok {
				// If type assertion fails, fallback to original parameters without additionalProperties
				cleanedParamsMap = make(map[string]any)
				if originalParams, ok := function.Parameters.(map[string]any); ok {
					for k, v := range originalParams {
						if k != "additionalProperties" && k != "description" && k != "strict" {
							cleanedParamsMap[k] = v
						}
					}
				}
			}
			geminiRequest.Tools = append(geminiRequest.Tools, ChatTools{
				FunctionDeclarations: []model.Function{
					{
						Name:        function.Name,
						Description: function.Description,
						Parameters:  cleanedParamsMap,
						Required:    function.Required,
					},
				},
			})
		}

	}

	if cfg := convertToolChoiceToConfig(textRequest.ToolChoice); cfg != nil {
		geminiRequest.ToolConfig = cfg
	}

	if geminiRequest.GenerationConfig.TopP != nil &&
		(*geminiRequest.GenerationConfig.TopP < 0 || *geminiRequest.GenerationConfig.TopP > 1) {
		geminiRequest.GenerationConfig.TopP = nil
	}

	shouldAddDummyModelMessage := false
	for _, message := range textRequest.Messages {
		// Start with initial content based on message string content
		initialText := message.StringContent()
		var parts []Part

		// Add text content if it's not empty
		if initialText != "" {
			parts = append(parts, Part{
				Text: initialText,
			})
		}

		// Handle OpenAI tool calls - convert them to Gemini function calls
		if len(message.ToolCalls) > 0 {
			for _, toolCall := range message.ToolCalls {
				// Parse the arguments from JSON string to interface{}
				var args any
				if err := json.Unmarshal([]byte(toolCall.Function.Arguments.(string)), &args); err != nil {
					// If parsing fails, use the raw string
					args = toolCall.Function.Arguments
				}

				parts = append(parts, Part{
					FunctionCall: &FunctionCall{
						FunctionName: toolCall.Function.Name,
						Arguments:    args,
					},
				})
			}
		}

		// Parse structured content and add additional parts
		openaiContent := message.ParseContent()
		imageNum := 0
		for _, part := range openaiContent {
			if part.Type == model.ContentTypeText && part.Text != nil && *part.Text != "" {
				// Only add if we haven't already added this text from StringContent()
				if *part.Text != initialText {
					parts = append(parts, Part{
						Text: *part.Text,
					})
				}
			} else if part.Type == model.ContentTypeImageURL {
				imageNum += 1
				if imageNum > VisionMaxImageNum {
					continue
				}
				mimeType, data, _ := image.GetImageFromUrl(part.ImageURL.Url)
				parts = append(parts, Part{
					InlineData: &InlineData{
						MimeType: mimeType,
						Data:     data,
					},
				})
			}
		}

		// If we have no parts at all (empty content with tool calls), add a minimal text part
		// to satisfy Gemini's requirement that parts cannot be empty
		if len(parts) == 0 {
			parts = append(parts, Part{
				Text: " ", // Minimal non-empty text to satisfy Gemini's requirements
			})
		}

		content := ChatContent{
			Role:  message.Role,
			Parts: parts,
		}

		// there's no assistant role in gemini and API shall vomit if Role is not user or model
		if content.Role == "assistant" {
			content.Role = "model"
		}
		// Converting system prompt to prompt from user for the same reason
		if content.Role == "system" {
			if IsModelSupportSystemInstruction(textRequest.Model) {
				geminiRequest.SystemInstruction = &content
				geminiRequest.SystemInstruction.Role = ""
				continue
			}
			// Models without native system instruction support require an initial
			// assistant turn between two user prompts to keep Gemini's turn
			// alternation happy.
			shouldAddDummyModelMessage = true
			content.Role = "user"
		}
		// Handle tool responses - convert to user role with function response format
		if content.Role == "tool" {
			// Tool responses in OpenAI are converted to user messages in Gemini
			// with the function response content
			content.Role = "user"
			// Keep the original content as text, as Gemini expects function responses
			// to be handled in a different format than OpenAI
		}

		geminiRequest.Contents = append(geminiRequest.Contents, content)

		// If a system message is the last message, we need to add a dummy model message to make gemini happy
		if shouldAddDummyModelMessage {
			geminiRequest.Contents = append(geminiRequest.Contents, ChatContent{
				Role: "model",
				Parts: []Part{
					{
						Text: "Okay",
					},
				},
			})
			shouldAddDummyModelMessage = false
		}
	}

	return &geminiRequest
}

func convertToolChoiceToConfig(toolChoice any) *ToolConfig {
	switch choice := toolChoice.(type) {
	case string:
		switch strings.ToLower(strings.TrimSpace(choice)) {
		case "none":
			return &ToolConfig{FunctionCallingConfig: FunctionCallingConfig{Mode: "NONE"}}
		default:
			return nil
		}
	case map[string]any:
		choiceType := strings.ToLower(strings.TrimSpace(fmt.Sprint(choice["type"])))
		var allowed []string
		switch choiceType {
		case "function":
			if fn, ok := choice["function"].(map[string]any); ok {
				if name, ok := fn["name"].(string); ok && strings.TrimSpace(name) != "" {
					allowed = append(allowed, strings.TrimSpace(name))
				}
			}
		case "tool":
			if name, ok := choice["name"].(string); ok && strings.TrimSpace(name) != "" {
				allowed = append(allowed, strings.TrimSpace(name))
			}
		}
		if len(allowed) > 0 {
			return &ToolConfig{FunctionCallingConfig: FunctionCallingConfig{
				Mode:                 "ANY",
				AllowedFunctionNames: allowed,
			}}
		}
	}
	return nil
}

// ConvertEmbeddingRequest converts an OpenAI-compatible embedding request to Gemini's BatchEmbeddingRequest format.
// It transforms input text(s) into the format expected by Gemini's embedding API.
func ConvertEmbeddingRequest(request model.GeneralOpenAIRequest) *BatchEmbeddingRequest {
	inputs := request.ParseInput()
	requests := make([]EmbeddingRequest, len(inputs))
	model := fmt.Sprintf("models/%s", request.Model)

	for i, input := range inputs {
		requests[i] = EmbeddingRequest{
			Model: model,
			Content: ChatContent{
				Parts: []Part{
					{
						Text: input,
					},
				},
			},
		}
	}

	return &BatchEmbeddingRequest{
		Requests: requests,
	}
}

type ChatResponse struct {
	Candidates     []ChatCandidate    `json:"candidates"`
	PromptFeedback ChatPromptFeedback `json:"promptFeedback"`
	UsageMetadata  *UsageMetadata     `json:"usageMetadata,omitempty"`
	ModelVersion   string             `json:"modelVersion,omitempty"`
	ResponseId     string             `json:"responseId,omitempty"`
}

func (g *ChatResponse) GetResponseText() string {
	if g == nil {
		return ""
	}
	if len(g.Candidates) > 0 && len(g.Candidates[0].Content.Parts) > 0 {
		return g.Candidates[0].Content.Parts[0].Text
	}
	return ""
}

type ChatCandidate struct {
	Content       ChatContent        `json:"content"`
	FinishReason  string             `json:"finishReason"`
	Index         int64              `json:"index"`
	SafetyRatings []ChatSafetyRating `json:"safetyRatings"`
}

type ChatSafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
}

type ChatPromptFeedback struct {
	SafetyRatings []ChatSafetyRating `json:"safetyRatings"`
}

// getToolCalls extracts function call tool information from a Gemini chat candidate.
// It processes all parts that contain function calls and returns them as OpenAI-compatible tool calls.
// Returns an empty slice if no function calls are present or if the candidate has no parts.
func getToolCalls(c *gin.Context, candidate *ChatCandidate) []model.Tool {
	lg := gmw.GetLogger(c)
	var toolCalls []model.Tool

	// Guard against empty Parts slice to prevent index out of range panic
	if len(candidate.Content.Parts) == 0 {
		lg.Debug("getToolCalls: candidate has no parts, returning empty tool calls")
		return toolCalls
	}

	// Process all parts that contain function calls, not just the first one
	for _, part := range candidate.Content.Parts {
		if part.FunctionCall == nil {
			continue
		}
		argsBytes, err := json.Marshal(part.FunctionCall.Arguments)
		if err != nil {
			lg.Error("getToolCalls: failed to marshal function call arguments",
				zap.String("function_name", part.FunctionCall.FunctionName),
				zap.Error(err))
			continue
		}
		toolCall := model.Tool{
			Id:   fmt.Sprintf("call_%s", random.GetUUID()),
			Type: "function",
			Function: &model.Function{
				Arguments: string(argsBytes),
				Name:      part.FunctionCall.FunctionName,
			},
		}
		toolCalls = append(toolCalls, toolCall)
	}
	return toolCalls
}

// getStreamingToolCalls creates tool calls for streaming responses with Index field set.
// It processes all parts that contain function calls and returns them with proper indexing
// for stream delta accumulation.
func getStreamingToolCalls(c *gin.Context, candidate *ChatCandidate) []model.Tool {
	lg := gmw.GetLogger(c)
	var toolCalls []model.Tool

	// Process all parts in case there are multiple function calls
	for partIndex, part := range candidate.Content.Parts {
		if part.FunctionCall == nil {
			continue
		}
		argsBytes, err := json.Marshal(part.FunctionCall.Arguments)
		if err != nil {
			lg.Error("getStreamingToolCalls: failed to marshal function call arguments",
				zap.String("function_name", part.FunctionCall.FunctionName),
				zap.Error(err))
			continue
		}
		// Set index for streaming tool calls - use the part index to ensure proper ordering
		// This handles the case where Gemini might support multiple parallel tool calls in the future
		index := partIndex
		toolCall := model.Tool{
			Id:   fmt.Sprintf("call_%s", random.GetUUID()),
			Type: "function",
			Function: &model.Function{
				Arguments: string(argsBytes),
				Name:      part.FunctionCall.FunctionName,
			},
			Index: &index, // Set index for streaming delta accumulation
		}
		toolCalls = append(toolCalls, toolCall)
	}
	return toolCalls
}

func responseGeminiChat2OpenAI(c *gin.Context, response *ChatResponse) *openai.TextResponse {
	fullTextResponse := openai.TextResponse{
		Id:      tracing.GenerateChatCompletionID(c),
		Object:  "chat.completion",
		Created: helper.GetTimestamp(),
		Choices: make([]openai.TextResponseChoice, 0, len(response.Candidates)),
	}
	for i, candidate := range response.Candidates {
		choice := openai.TextResponseChoice{
			Index: i,
			Message: model.Message{
				Role: "assistant",
			},
			FinishReason: constant.StopFinishReason,
		}

		toolCalls := getToolCalls(c, &candidate)
		if len(toolCalls) > 0 {
			choice.Message.ToolCalls = toolCalls
		}

		if len(candidate.Content.Parts) > 0 {
			var textParts []string
			var structured []model.MessageContent

			for _, part := range candidate.Content.Parts {
				if part.FunctionCall != nil {
					continue
				}

				if part.Text != "" {
					textParts = append(textParts, part.Text)
					structured = append(structured, model.MessageContent{
						Type: model.ContentTypeText,
						Text: &part.Text,
					})
				}

				if part.InlineData != nil && part.InlineData.MimeType != "" && part.InlineData.Data != "" {
					imageURL := &model.ImageURL{
						Url: fmt.Sprintf("data:%s;base64,%s", part.InlineData.MimeType, part.InlineData.Data),
					}
					structured = append(structured, model.MessageContent{
						Type:     model.ContentTypeImageURL,
						ImageURL: imageURL,
					})
				}
			}

			joined := strings.Join(textParts, "\n")
			if len(structured) > 1 || (len(structured) == 1 && structured[0].Type != model.ContentTypeText) {
				choice.Message.Content = structured
			} else if joined != "" {
				choice.Message.Content = joined
			} else if len(toolCalls) == 0 {
				choice.Message.Content = ""
			}
		} else {
			choice.Message.Content = ""
			choice.FinishReason = candidate.FinishReason
		}

		fullTextResponse.Choices = append(fullTextResponse.Choices, choice)
	}
	return &fullTextResponse
}

func streamResponseGeminiChat2OpenAI(c *gin.Context, geminiResponse *ChatResponse) *openai.ChatCompletionsStreamResponse {
	var choice openai.ChatCompletionsStreamResponseChoice
	choice.Delta.Role = "assistant"

	// Check if we have any candidates
	if len(geminiResponse.Candidates) == 0 {
		return nil
	}

	// Get the first candidate
	candidate := geminiResponse.Candidates[0]

	// Check if there are parts in the content
	if len(candidate.Content.Parts) == 0 {
		return nil
	}

	// Handle different content types in the parts
	for _, part := range candidate.Content.Parts {
		// Handle text content
		if part.Text != "" {
			// Store as string for simple text responses
			textContent := part.Text
			choice.Delta.Content = textContent
		}

		// Handle image content
		if part.InlineData != nil && part.InlineData.MimeType != "" && part.InlineData.Data != "" {
			// Create a structured response for image content
			imageUrl := fmt.Sprintf("data:%s;base64,%s", part.InlineData.MimeType, part.InlineData.Data)

			// If we already have text content, create a mixed content response
			if strContent, ok := choice.Delta.Content.(string); ok && strContent != "" {
				// Convert the existing text content and add the image
				messageContents := []model.MessageContent{
					{
						Type: model.ContentTypeText,
						Text: &strContent,
					},
					{
						Type: model.ContentTypeImageURL,
						ImageURL: &model.ImageURL{
							Url: imageUrl,
						},
					},
				}
				choice.Delta.Content = messageContents
			} else {
				// Only have image content
				choice.Delta.Content = []model.MessageContent{
					{
						Type: model.ContentTypeImageURL,
						ImageURL: &model.ImageURL{
							Url: imageUrl,
						},
					},
				}
			}
		}

		// Handle function calls (if present)
		if part.FunctionCall != nil {
			choice.Delta.ToolCalls = getStreamingToolCalls(c, &candidate)
		}
	}

	// Create response
	var response openai.ChatCompletionsStreamResponse
	response.Id = tracing.GenerateChatCompletionID(c)
	response.Created = helper.GetTimestamp()
	response.Object = "chat.completion.chunk"
	response.Model = "gemini"
	response.Choices = []openai.ChatCompletionsStreamResponseChoice{choice}

	return &response
}

func embeddingResponseGemini2OpenAI(response *EmbeddingResponse) *openai.EmbeddingResponse {
	openAIEmbeddingResponse := openai.EmbeddingResponse{
		Object: "list",
		Data:   make([]openai.EmbeddingResponseItem, 0, len(response.Embeddings)),
		Model:  "gemini-embedding",
		Usage:  model.Usage{TotalTokens: 0},
	}
	for _, item := range response.Embeddings {
		openAIEmbeddingResponse.Data = append(openAIEmbeddingResponse.Data, openai.EmbeddingResponseItem{
			Object:    `embedding`,
			Index:     0,
			Embedding: item.Values,
		})
	}
	return &openAIEmbeddingResponse
}

// StreamHandler processes streaming responses from the Gemini API and converts them to OpenAI-compatible
// Server-Sent Events (SSE) format. It reads the response body line by line, unmarshals each chunk,
// converts it to OpenAI format, and streams it to the client.
// Returns an error if the response processing fails, and the accumulated response text on success.
func StreamHandler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, string) {
	lg := gmw.GetLogger(c)
	responseText := ""
	scanner := bufio.NewScanner(resp.Body)
	helper.ConfigureScannerBuffer(scanner)
	scanner.Split(bufio.ScanLines)

	common.SetEventStreamHeaders(c)

	for scanner.Scan() {
		data := scanner.Text()
		data = strings.TrimSpace(data)

		if !strings.HasPrefix(data, "data: ") {
			continue
		}
		data = strings.TrimPrefix(data, "data: ")
		data = strings.TrimSuffix(data, "\"")

		var geminiResponse ChatResponse
		err := json.Unmarshal([]byte(data), &geminiResponse)
		if err != nil {
			lg.Error("error unmarshalling stream response",
				zap.Error(errors.Wrap(err, "unmarshal stream")))
			continue
		}

		response := streamResponseGeminiChat2OpenAI(c, &geminiResponse)
		if response == nil {
			continue
		}

		responseText += response.Choices[0].Delta.StringContent()

		err = render.ObjectData(c, response)
		if err != nil {
			lg.Error("error rendering stream",
				zap.Error(errors.Wrap(err, "render stream")))
		}
	}
	if err := scanner.Err(); err != nil {
		lg.Error("error reading stream",
			zap.Error(errors.Wrap(err, "scanner stream")),
			zap.Int("scanner_max_token_size", helper.DefaultScannerMaxTokenSize))
	}

	render.Done(c)

	err := resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "close_response_body_failed"), "close_response_body_failed", http.StatusInternalServerError), ""
	}

	return nil, responseText
}

// Handler processes non-streaming responses from the Gemini API and converts them to OpenAI-compatible format.
// It reads the complete response body, unmarshals it, converts to OpenAI TextResponse format,
// calculates token usage (preferring Gemini's usage metadata when available), and writes the response.
// Returns an error if processing fails, and the usage statistics on success.
func Handler(c *gin.Context, resp *http.Response, promptTokens int, modelName string) (*model.ErrorWithStatusCode, *model.Usage) {
	lg := gmw.GetLogger(c)
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "read_response_body_failed"), "read_response_body_failed", http.StatusInternalServerError), nil
	}

	err = resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "close_response_body_failed"), "close_response_body_failed", http.StatusInternalServerError), nil
	}

	var geminiResponse ChatResponse
	err = json.Unmarshal(responseBody, &geminiResponse)
	if err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "unmarshal_response_body_failed"), "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}

	// Debug logging for Gemini response structure
	lg.Debug("gemini response received",
		zap.String("model", modelName),
		zap.Int("candidates_count", len(geminiResponse.Candidates)),
		zap.Bool("has_usage_metadata", geminiResponse.UsageMetadata != nil),
	)

	if len(geminiResponse.Candidates) == 0 {
		return &model.ErrorWithStatusCode{
			Error: model.Error{
				Message:  "No candidates returned",
				Type:     model.ErrorTypeServer,
				Param:    "",
				Code:     500,
				RawError: errors.New("No candidates returned"),
			},
			StatusCode: resp.StatusCode,
		}, nil
	}

	// Debug logging for candidate structure
	for i, candidate := range geminiResponse.Candidates {
		lg.Debug("gemini candidate details",
			zap.String("model", modelName),
			zap.Int("candidate_index", i),
			zap.Int("parts_count", len(candidate.Content.Parts)),
			zap.String("finish_reason", candidate.FinishReason),
		)
	}

	fullTextResponse := responseGeminiChat2OpenAI(c, &geminiResponse)
	fullTextResponse.Model = modelName

	// Prioritize usageMetadata from Gemini response
	var usage model.Usage
	if geminiResponse.UsageMetadata != nil &&
		geminiResponse.UsageMetadata.TotalTokenCount > 0 {
		// Use Gemini's provided token counts
		usage = model.Usage{
			PromptTokens: geminiResponse.UsageMetadata.PromptTokenCount,
			CompletionTokens: geminiResponse.UsageMetadata.CandidatesTokenCount +
				geminiResponse.UsageMetadata.ThoughtsTokenCount,
			TotalTokens: geminiResponse.UsageMetadata.TotalTokenCount,
		}
	} else {
		// Fall back to manual calculation if usageMetadata is unavailable or zero
		completionTokens := openai.CountTokenText(geminiResponse.GetResponseText(), modelName)
		usage = model.Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		}
	}

	fullTextResponse.Usage = usage
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "marshal_response_body_failed"), "marshal_response_body_failed", http.StatusInternalServerError), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	return nil, &usage
}

// EmbeddingHandler processes embedding responses from the Gemini API and converts them to OpenAI-compatible format.
// It reads the response body, unmarshals it into Gemini's embedding format, converts it to OpenAI's format,
// and writes the response back to the client.
// Returns an error if processing fails, and the usage statistics on success.
func EmbeddingHandler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, *model.Usage) {
	var geminiEmbeddingResponse EmbeddingResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "read_response_body_failed"), "read_response_body_failed", http.StatusInternalServerError), nil
	}
	err = resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "close_response_body_failed"), "close_response_body_failed", http.StatusInternalServerError), nil
	}
	err = json.Unmarshal(responseBody, &geminiEmbeddingResponse)
	if err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "unmarshal_response_body_failed"), "unmarshal_response_body_failed", http.StatusInternalServerError), nil
	}
	if geminiEmbeddingResponse.Error != nil {
		return &model.ErrorWithStatusCode{
			Error: model.Error{
				Message:  geminiEmbeddingResponse.Error.Message,
				Type:     model.ErrorTypeGemini,
				Param:    "",
				Code:     geminiEmbeddingResponse.Error.Code,
				RawError: errors.New(geminiEmbeddingResponse.Error.Message),
			},
			StatusCode: resp.StatusCode,
		}, nil
	}
	fullTextResponse := embeddingResponseGemini2OpenAI(&geminiEmbeddingResponse)
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "marshal_response_body_failed"), "marshal_response_body_failed", http.StatusInternalServerError), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	return nil, &fullTextResponse.Usage
}
