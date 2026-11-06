package gemini

import (
	"context"
	"encoding/json"
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
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
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

// ConvertRequest converts an OpenAI-compatible chat completion request to Gemini's native ChatRequest format.
// It transforms messages, handles tool definitions, sets safety settings based on configuration,
// and configures generation parameters like temperature, top_p, max_tokens, etc.
// Safety settings are configured to the lowest possible thresholds by default.
func ConvertRequest(textRequest model.GeneralOpenAIRequest) *ChatRequest {
	converted, _ := convertRequest(context.Background(), textRequest, false)
	return converted
}

// convertRequest builds a native request and optionally rejects invalid input images.
// Parameters: ctx cancels remote image downloads; textRequest is the source request;
// strictImages enables controlled image failures.
// Returns: the converted request or a sanitized image validation error.
func convertRequest(ctx context.Context, textRequest model.GeneralOpenAIRequest, strictImages bool) (*ChatRequest, error) {
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

	if textRequest.MaxCompletionTokens != nil {
		geminiRequest.GenerationConfig.MaxOutputTokens = *textRequest.MaxCompletionTokens
	}
	if geminiRequest.GenerationConfig.MaxOutputTokens == 0 {
		geminiRequest.GenerationConfig.MaxOutputTokens = config.DefaultMaxToken
	}
	if err := configureNanoBananaThinking(&geminiRequest, textRequest); err != nil {
		return nil, errors.Wrap(err, "validation failed: reasoning effort")
	}
	if textRequest.ResponseFormat != nil {
		if mimeType, ok := mimeTypeMap[textRequest.ResponseFormat.Type]; ok {
			geminiRequest.GenerationConfig.ResponseMimeType = mimeType
		}
		if textRequest.ResponseFormat.JsonSchema != nil {
			// Clean the schema to remove unsupported properties for Gemini
			cleanedSchema := cleanJsonSchemaForGemini(textRequest.ResponseFormat.JsonSchema.Schema)
			// ResponseSchema is an `any` field, so omitempty will not drop an empty
			// map: cleaning a schema whose top level holds only unsupported keys
			// (a $ref/$defs wrapper, which pydantic and zod emit constantly) yields
			// an empty map, and `"responseSchema": {}` alongside a JSON mime type is
			// rejected by Gemini. Send the mime type without a schema instead.
			if schemaMap, ok := cleanedSchema.(map[string]any); ok && len(schemaMap) == 0 {
				cleanedSchema = nil
			}
			if cleanedSchema != nil {
				geminiRequest.GenerationConfig.ResponseSchema = cleanedSchema
			}
			geminiRequest.GenerationConfig.ResponseMimeType = mimeTypeMap["json_object"]
		}
	}

	// remove temperature & top_p for Gemini image models; only enforce response modalities when allowed
	if geminiOpenaiCompatible.IsGeminiImageGenerationModel(textRequest.Model) {
		geminiRequest.GenerationConfig.Temperature = nil
		geminiRequest.GenerationConfig.TopP = nil
		if geminiRequest.GenerationConfig.ResponseModalities != nil {
			geminiRequest.GenerationConfig.ResponseModalities = []string{"TEXT", "IMAGE"}
		}
	}

	// FIX(https://github.com/Laisky/one-api/issues/60):
	// Gemini's function call supports fewer parameters than OpenAI's,
	// so a conversion is needed here to keep only the parameters supported by Gemini.
	// len() rather than != nil: an explicitly empty "tools": [] is non-nil, and it
	// used to produce "function_declarations": [], which Gemini rejects with 400
	// INVALID_ARGUMENT. FunctionDeclarations is an `any` field, so omitempty does
	// not drop the empty slice for us.
	if len(textRequest.Tools) > 0 {
		convertedGeminiFunctions := make([]model.Function, 0, len(textRequest.Tools))
		for _, tool := range textRequest.Tools {
			// Tool.Function is a pointer filled from the request body, so a tool entry
			// without a "function" object must be skipped rather than dereferenced.
			if tool.Function == nil {
				continue
			}
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
		if len(convertedGeminiFunctions) > 0 {
			geminiRequest.Tools = []ChatTools{
				{
					FunctionDeclarations: convertedGeminiFunctions,
				},
			}
		}
	} else if len(textRequest.Functions) > 0 {
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
	var additionalInlineBytes int64
	for _, message := range textRequest.Messages {
		// Only prefill scalar content. Multipart text stays in its original
		// position relative to media and must not be concatenated/duplicated.
		initialText := ""
		if message.IsStringContent() {
			initialText = message.StringContent()
		}
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
				// Parse the arguments from JSON string to interface{}.
				// ArgumentsJSON keeps a client-supplied object, a missing `function`
				// or absent arguments from panicking here.
				var args any
				rawArguments, argErr := toolCall.Function.ArgumentsJSON()
				if argErr != nil {
					rawArguments = "{}"
				}
				if err := json.Unmarshal([]byte(rawArguments), &args); err != nil {
					// If parsing fails, use the raw string
					args = rawArguments
				}

				parts = append(parts, Part{
					FunctionCall: &FunctionCall{
						FunctionName: toolCall.FunctionName(),
						Arguments:    args,
					},
				})
			}
		}

		// Parse structured content and add additional parts
		openaiContent, err := message.ParseContentChecked()
		if err != nil {
			return nil, errors.Wrap(err, "validation failed: native Gemini content")
		}
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
				mimeType, data, imageErr := image.GetImageFromUrlWithContext(ctx, part.ImageURL.Url)
				if strictImages {
					if imageErr != nil {
						return nil, errors.New("validation failed: invalid or inaccessible input image")
					}
					var err error
					mimeType, err = validateNativeImageData(mimeType, data)
					if err != nil {
						return nil, errors.Wrap(err, "validate native image input")
					}
				}
				parts = append(parts, Part{
					InlineData: &InlineData{
						MimeType: mimeType,
						Data:     data,
					},
				})
			} else if part.Type == model.ContentTypeVideoURL || part.Type == model.ContentTypeFile || part.Type == model.ContentTypeInputAudio {
				converted, size, err := convertNativeMediaPart(textRequest.Model, part)
				if err != nil {
					return nil, errors.Wrap(err, "validation failed: native Gemini media")
				}
				additionalInlineBytes += size
				if additionalInlineBytes > nativeInlineMediaLimit {
					return nil, errors.New("validation failed: aggregate inline video/audio/file input exceeds the relay limit")
				}
				parts = append(parts, converted)
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

	return &geminiRequest, nil
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
			streamErr = errors.Wrap(err, "unmarshal stream")
			break
		}
		seen := make(map[int64]bool, len(geminiResponse.Candidates))
		for _, candidate := range geminiResponse.Candidates {
			if candidate.Index < 0 || int64(int(candidate.Index)) != candidate.Index || seen[candidate.Index] {
				streamErr = errors.New("invalid or duplicate Gemini candidate index")
				break
			}
			seen[candidate.Index] = true
		}
		if streamErr != nil {
			break
		}
		if geminiHasUnrepresentedMedia(&geminiResponse) {
			// A partial media projection cannot safely claim the entire
			// aggregate IMAGE-token receipt was separately image-billed.
			c.Set(ctxkey.GeminiOutputIncomplete, true)
		}
		if geminiResponse.UsageMetadata != nil && geminiResponse.UsageMetadata.TotalTokenCount > 0 {
			usageMetadata = geminiResponse.UsageMetadata
		}
		response := streamResponseGeminiChat2OpenAI(c, &geminiResponse)
		if response == nil {
			continue
		}
		for _, choice := range response.Choices {
			responseText += geminiStreamText(choice.Delta)
		}
		counts := countGeminiStreamImages(&geminiResponse)
		err = openai_compatible.RenderStreamChunkWithBridge(c, response)
		if rw := openai_compatible.StreamRewriterFromContext(c); rw != nil {
			if reporter, ok := rw.(interface{ DeliveredImagesInLastChunk() int }); ok {
				outputImageCount += reporter.DeliveredImagesInLastChunk()
			} else if err == nil {
				outputImageCount += counts.Total
			}
		} else if err == nil {
			outputImageCount += counts.Total
		}
		if err != nil {
			streamErr = errors.Wrap(err, "render stream")
			break
		}
		inlineImageCount += counts.Inline
		fileImageCount += counts.File
	}
	if streamErr != nil {
		c.Set(ctxkey.GeminiOutputIncomplete, true)
		render.LogHeartbeatLineReaderError(c, lg, errors.Wrap(streamErr, "gemini stream"), hbr)
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

	usage := geminiUsageMetadataToOpenAIUsage(usageMetadata)

	if streamErr == nil {
		streamErr = openai_compatible.FinalizeStreamWithBridgeChecked(c, usage)
		if streamErr != nil {
			c.Set(ctxkey.GeminiOutputIncomplete, true)
		}
	}

	err := resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(errors.Wrap(err, "close_response_body_failed"), "close_response_body_failed", http.StatusInternalServerError), "", nil
	}

	if streamErr != nil {
		return openai.ErrorWrapper(streamErr, "gemini_stream_failed", http.StatusBadGateway), responseText, usage
	}
	return nil, responseText, usage
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

	if geminiHasUnrepresentedMedia(&geminiResponse) {
		c.Set(ctxkey.GeminiOutputIncomplete, true)
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
			BillingEstimateReason: "gemini_response_missing_usage_receipt",
			PromptTokens:          promptTokens,
			CompletionTokens:      completionTokens,
			TotalTokens:           promptTokens + completionTokens,
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
	if err != nil {
		return openai.ErrorWrapper(errors.WithStack(err), "write_response_body_failed", http.StatusInternalServerError), &usage
	}
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
	if err != nil {
		return openai.ErrorWrapper(errors.WithStack(err), "write_response_body_failed", http.StatusInternalServerError), &fullTextResponse.Usage
	}
	return nil, &fullTextResponse.Usage
}
