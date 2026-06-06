package gemini

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/image"
	"github.com/Laisky/one-api/common/render"
	commonsse "github.com/Laisky/one-api/common/sse"
	"github.com/Laisky/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/model"
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
// Parameters: request is the OpenAI-compatible embeddings request.
// Returns: the Gemini batch embedding request or an error when the input cannot be normalized into Gemini contents.
func ConvertEmbeddingRequest(request model.GeneralOpenAIRequest) (*BatchEmbeddingRequest, error) {
	contents, _, err := BuildEmbeddingContents(request.Input)
	if err != nil {
		return nil, errors.Wrap(err, "build gemini embedding contents")
	}

	requests := make([]EmbeddingRequest, len(contents))
	model := fmt.Sprintf("models/%s", request.Model)

	for i, content := range contents {
		requests[i] = EmbeddingRequest{
			Model:   model,
			Content: content,
		}
	}

	return &BatchEmbeddingRequest{
		Requests: requests,
	}, nil
}

// StreamHandler processes streaming responses from the Gemini API and converts them to OpenAI-compatible
// Server-Sent Events (SSE) format. It reads the response body line by line, unmarshals each chunk,
// converts it to OpenAI format, and streams it to the client.
//
// Returns an error if the response processing fails, the accumulated response text, and the
// authoritative usage captured from the upstream usageMetadata. The returned usage is nil when no
// usageMetadata was present in the stream, signalling the caller to fall back to a local estimate.
func StreamHandler(c *gin.Context, resp *http.Response) (*model.ErrorWithStatusCode, string, *model.Usage) {
	lg := gmw.GetLogger(c)
	responseText := ""
	outputImageCount := 0
	inlineImageCount := 0
	fileImageCount := 0
	lineReader := commonsse.NewLineReader(resp.Body, commonsse.DefaultLineBufferSize)

	common.SetEventStreamHeaders(c)

	// Wrap the reader with heartbeats to prevent reverse-proxy timeouts (e.g. Cloudflare 524).
	hbr := render.NewHeartbeatLineReader(c, lineReader, render.DefaultHeartbeatInterval)
	defer hbr.Close()
	var streamErr error

	// usageMetadata captures the authoritative upstream token accounting. Gemini reports
	// cumulative running totals (typically in the final chunk), so we overwrite with the
	// latest non-empty snapshot rather than summing, to avoid double-counting.
	var usageMetadata *UsageMetadata

	for {
		line, err := hbr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			streamErr = err
			break
		}

		var data string
		if line.Oversized {
			payload, err := io.ReadAll(line.Large)
			if err != nil {
				streamErr = err
				break
			}
			data = "data: " + string(payload)
		} else {
			data = line.Text()
		}

		data = strings.TrimSpace(data)

		if !strings.HasPrefix(data, "data: ") {
			continue
		}
		data = strings.TrimPrefix(data, "data: ")
		data = strings.TrimSuffix(data, "\"")

		if data == "[DONE]" {
			break
		}

		var geminiResponse ChatResponse
		err = json.Unmarshal([]byte(data), &geminiResponse)
		if err != nil {
			lg.Error("error unmarshalling stream response",
				zap.Error(errors.Wrap(err, "unmarshal stream")))
			continue
		}

		// Capture the latest non-empty usageMetadata (cumulative totals, not deltas).
		if geminiResponse.UsageMetadata != nil && geminiResponse.UsageMetadata.TotalTokenCount > 0 {
			usageMetadata = geminiResponse.UsageMetadata
		}

		chunkCounts := countGeminiOutputImages(&geminiResponse)
		outputImageCount += chunkCounts.Total
		inlineImageCount += chunkCounts.Inline
		fileImageCount += chunkCounts.File

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
	if streamErr != nil {
		render.LogHeartbeatLineReaderError(c, lg, errors.Wrap(streamErr, "line reader stream"), hbr)
	}

	if outputImageCount > 0 {
		recordGeminiOutputImageCount(c, outputImageCount)
		modelName := c.GetString(ctxkey.RequestModel)
		lg.Debug("gemini stream output images counted",
			zap.String("model", modelName),
			zap.Int("image_count", outputImageCount),
			zap.Int("inline_image_count", inlineImageCount),
			zap.Int("file_image_count", fileImageCount),
		)
	}

	render.Done(c)

	err := resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "close_response_body_failed"), "close_response_body_failed", http.StatusInternalServerError), "", nil
	}

	return nil, responseText, geminiUsageMetadataToOpenAIUsage(usageMetadata)
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

	outputCounts := countGeminiOutputImages(&geminiResponse)
	if outputCounts.Total > 0 {
		recordGeminiOutputImageCount(c, outputCounts.Total)
		lg.Debug("gemini output images counted",
			zap.String("model", modelName),
			zap.Int("image_count", outputCounts.Total),
			zap.Int("inline_image_count", outputCounts.Inline),
			zap.Int("file_image_count", outputCounts.File),
		)
	}

	fullTextResponse := responseGeminiChat2OpenAI(c, &geminiResponse)
	fullTextResponse.Model = modelName

	// Prioritize usageMetadata from Gemini response
	var usage model.Usage
	if geminiResponse.UsageMetadata != nil &&
		geminiResponse.UsageMetadata.TotalTokenCount > 0 {
		// Use Gemini's provided token counts. The helper keeps PromptTokens at the full
		// promptTokenCount (which includes cached tokens) while surfacing the cached portion
		// via PromptTokensDetails so it is billed at the discounted CachedInputRatio.
		usage = *geminiUsageMetadataToOpenAIUsage(geminiResponse.UsageMetadata)
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
// Parameters: c is the current request context, resp is the upstream Gemini HTTP response, and promptTokens is the preflight input token total used as a fallback when Gemini omits usage.
// Returns: an API error when processing fails, otherwise the usage statistics written back to the client response.
func EmbeddingHandler(c *gin.Context, resp *http.Response, promptTokens int) (*model.ErrorWithStatusCode, *model.Usage) {
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
	fullTextResponse := embeddingResponseGemini2OpenAI(&geminiEmbeddingResponse, promptTokens, embeddingPromptTokensDetailsFromContext(c))
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "marshal_response_body_failed"), "marshal_response_body_failed", http.StatusInternalServerError), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	return nil, &fullTextResponse.Usage
}
