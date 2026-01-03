// Package aws provides the AWS adaptor for the relay service.
package aws

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/image"
	"github.com/songquanpeng/one-api/common/tracing"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/relay/adaptor/aws/internal/streamfinalizer"
	"github.com/songquanpeng/one-api/relay/adaptor/aws/utils"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// Support for Llama 3, 3.1, 3.2, 3.3, and 4.0 instruction models
// https://docs.aws.amazon.com/bedrock/latest/userguide/model-ids.html
var AwsModelIDMap = map[string]string{
	// Llama 3 models
	"llama3-8b-8192":  "meta.llama3-8b-instruct-v1:0",
	"llama3-70b-8192": "meta.llama3-70b-instruct-v1:0",

	// Llama 3.1 models
	"llama3-1-8b-128k":  "meta.llama3-1-8b-instruct-v1:0",
	"llama3-1-70b-128k": "meta.llama3-1-70b-instruct-v1:0",

	// Llama 3.2 models
	"llama3-2-90b-128k":        "meta.llama3-2-90b-instruct-v1:0",
	"llama3-2-3b-131k":         "meta.llama3-2-3b-instruct-v1:0",
	"llama3-2-1b-131k":         "meta.llama3-2-1b-instruct-v1:0",
	"llama3-2-11b-vision-131k": "meta.llama3-2-11b-instruct-v1:0",

	// Llama 3.3 models
	"llama3-3-70b-128k": "meta.llama3-3-70b-instruct-v1:0",

	// Llama 4 models
	"llama4-scout-17b-3.5m":  "meta.llama4-scout-17b-instruct-v1:0",
	"llama4-maverick-17b-1m": "meta.llama4-maverick-17b-instruct-v1:0",
}

func awsModelID(requestModel string) (string, error) {
	if awsModelID, ok := AwsModelIDMap[requestModel]; ok {
		return awsModelID, nil
	}

	return "", errors.Errorf("model %s not found", requestModel)
}

func ConvertRequest(textRequest relaymodel.GeneralOpenAIRequest) *Request {
	llamaRequest := &Request{
		Messages:    textRequest.Messages,
		Temperature: textRequest.Temperature,
		TopP:        textRequest.TopP,
	}

	// Handle max tokens
	if textRequest.MaxTokens == 0 {
		llamaRequest.MaxTokens = config.DefaultMaxToken
	} else {
		llamaRequest.MaxTokens = textRequest.MaxTokens
	}

	// Handle stop sequences
	if textRequest.Stop != nil {
		if stopSlice, ok := textRequest.Stop.([]any); ok {
			stopSequences := make([]string, 0, len(stopSlice))
			for _, stop := range stopSlice {
				if stopStr, ok := stop.(string); ok && stopStr != "" {
					stopSequences = append(stopSequences, stopStr)
				}
			}
			if len(stopSequences) > 0 {
				llamaRequest.Stop = stopSequences
			}
		} else if stopStr, ok := textRequest.Stop.(string); ok {
			if stopStr != "" {
				llamaRequest.Stop = []string{stopStr}
			}
		} else if stopSlice, ok := textRequest.Stop.([]string); ok {
			filt := stopSlice[:0]
			for _, s := range stopSlice {
				if s != "" {
					filt = append(filt, s)
				}
			}
			if len(filt) > 0 {
				llamaRequest.Stop = filt
			}
		}
	}

	return llamaRequest
}

// convertLlamaToConverseRequest converts Llama request to Converse API format
func convertLlamaToConverseRequest(c *gin.Context, llamaReq *Request, modelID string) (*bedrockruntime.ConverseInput, error) {
	var converseMessages []types.Message
	var systemMessages []types.SystemContentBlock

	// Convert messages using standard Converse API format
	for _, msg := range llamaReq.Messages {
		switch msg.Role {
		case "system":
			// System messages go to the system field in Converse API
			systemMessages = append(systemMessages, &types.SystemContentBlockMemberText{
				Value: msg.StringContent(),
			})
		case "user":
			// User messages use standard Converse API format with support for images
			var contentBlocks []types.ContentBlock

			// Handle different content types using ParseContent
			contents := msg.ParseContent()
			if len(contents) == 0 {
				// Simple text content
				contentBlocks = append(contentBlocks, &types.ContentBlockMemberText{
					Value: msg.StringContent(),
				})
			} else {
				// Structured content - handle text and images
				for _, content := range contents {
					switch content.Type {
					case relaymodel.ContentTypeText:
						if content.Text != nil {
							contentBlocks = append(contentBlocks, &types.ContentBlockMemberText{
								Value: *content.Text,
							})
						}
					case relaymodel.ContentTypeImageURL:
						// Handle image content for vision models
						if content.ImageURL != nil {
							// Use the existing downloadImageFromURL function from utils
							imageData, imageFormat, err := utils.DownloadImageFromURL(gmw.Ctx(c), content.ImageURL.Url)
							if err != nil {
								// If image download fails, generate a fallback image with error text
								// This improves the response for vision-capable models as they can "see" the error message
								lg := gmw.GetLogger(c)
								lg.Warn("image download failed", zap.Error(err))

								// Generate fallback image with error message
								fallbackText := fmt.Sprintf("Image download failed: %s", err)
								fallbackImageData, _, imgErr := image.GenerateTextImage(fallbackText)
								if imgErr != nil {
									// If fallback image generation also fails, use text content as last resort
									contentBlocks = append(contentBlocks, &types.ContentBlockMemberText{
										Value: fallbackText,
									})
								} else {
									// Create ImageBlock with fallback image
									imageBlock := &types.ContentBlockMemberImage{
										Value: types.ImageBlock{
											Format: types.ImageFormatPng,
											Source: &types.ImageSourceMemberBytes{
												Value: fallbackImageData,
											},
										},
									}
									contentBlocks = append(contentBlocks, imageBlock)
								}
							} else {
								// Create ImageBlock with actual image data
								imageBlock := &types.ContentBlockMemberImage{
									Value: types.ImageBlock{
										Format: imageFormat,
										Source: &types.ImageSourceMemberBytes{
											Value: imageData,
										},
									},
								}
								contentBlocks = append(contentBlocks, imageBlock)
							}
						}
					default:
						// For unknown content types, convert to text representation
						contentBlocks = append(contentBlocks, &types.ContentBlockMemberText{
							Value: msg.StringContent(),
						})
					}
				}
			}

			converseMessages = append(converseMessages, types.Message{
				Role:    types.ConversationRole(msg.Role),
				Content: contentBlocks,
			})
		case "assistant":
			// Assistant messages use standard Converse API format
			contentBlocks := []types.ContentBlock{
				&types.ContentBlockMemberText{
					Value: msg.StringContent(),
				},
			}

			converseMessages = append(converseMessages, types.Message{
				Role:    types.ConversationRole(msg.Role),
				Content: contentBlocks,
			})
		}
	}

	// Create inference configuration
	inferenceConfig := &types.InferenceConfiguration{}
	if llamaReq.MaxTokens != 0 {
		inferenceConfig.MaxTokens = aws.Int32(int32(llamaReq.MaxTokens))
	}

	if llamaReq.Temperature != nil {
		inferenceConfig.Temperature = aws.Float32(float32(*llamaReq.Temperature))
	}
	if llamaReq.TopP != nil {
		inferenceConfig.TopP = aws.Float32(float32(*llamaReq.TopP))
	}
	if len(llamaReq.Stop) > 0 {
		stopSequences := make([]string, len(llamaReq.Stop))
		copy(stopSequences, llamaReq.Stop)
		inferenceConfig.StopSequences = stopSequences
	}

	converseReq := &bedrockruntime.ConverseInput{
		ModelId:         aws.String(modelID),
		Messages:        converseMessages,
		InferenceConfig: inferenceConfig,
	}

	// Add system messages if any
	if len(systemMessages) > 0 {
		converseReq.System = systemMessages
	}

	return converseReq, nil
}

// convertLlamaToConverseStreamRequest converts Llama request to Converse Stream API format
func convertLlamaToConverseStreamRequest(c *gin.Context, llamaReq *Request, modelID string) (*bedrockruntime.ConverseStreamInput, error) {
	converseReq, err := convertLlamaToConverseRequest(c, llamaReq, modelID)
	if err != nil {
		return nil, errors.Wrap(err, "convert llama request for stream")
	}

	return &bedrockruntime.ConverseStreamInput{
		ModelId:         converseReq.ModelId,
		Messages:        converseReq.Messages,
		System:          converseReq.System,
		InferenceConfig: converseReq.InferenceConfig,
	}, nil
}

// convertStopReason converts AWS converse stop reason to OpenAI format
func convertStopReason(awsReason string) *string {
	if awsReason == "" {
		return nil
	}

	var result string
	switch awsReason {
	case "max_tokens":
		result = "length"
	case "end_turn", "stop_sequence":
		result = "stop"
	case "content_filtered":
		result = "content_filter"
	default:
		// Fallback to "stop" to match OpenAI schema expectations.
		// result = "stop"

		result = awsReason // Return the actual AWS response instead of hardcoded "stop"
	}

	return &result
}

// convertConverseResponseToOpenAI converts AWS Converse response to OpenAI format
func convertConverseResponseToOpenAI(c *gin.Context, converseResp *bedrockruntime.ConverseOutput, modelName string) *openai.TextResponse {
	var responseText string
	var finishReason string

	// Extract response content from Converse API response
	if converseResp.Output != nil {
		switch outputValue := converseResp.Output.(type) {
		case *types.ConverseOutputMemberMessage:
			if len(outputValue.Value.Content) > 0 {
				// Process content blocks
				for _, contentBlock := range outputValue.Value.Content {
					switch contentValue := contentBlock.(type) {
					case *types.ContentBlockMemberText:
						// Add text content
						if contentValue.Value != "" {
							responseText = contentValue.Value
							break
						}
					}
				}
			}
			// Convert stop reason
			if stopReason := convertStopReason(string(converseResp.StopReason)); stopReason != nil {
				finishReason = *stopReason
			}
		}
	}

	// Create OpenAI-compatible choice
	choice := openai.TextResponseChoice{
		Index: 0,
		Message: relaymodel.Message{
			Role:    "assistant",
			Content: responseText,
			Name:    nil,
		},
		FinishReason: finishReason,
	}

	// Create OpenAI-compatible response
	fullTextResponse := openai.TextResponse{
		Id:      tracing.GenerateChatCompletionIDFromContext(c),
		Object:  "chat.completion",
		Created: helper.GetTimestamp(),
		Model:   modelName,
		Choices: []openai.TextResponseChoice{choice},
	}

	return &fullTextResponse
}

// Handler handles non-streaming requests using Converse API
func Handler(c *gin.Context, awsCli *bedrockruntime.Client, modelName string) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	awsModelName, err := awsModelID(c.GetString(ctxkey.RequestModel))
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "awsModelID")), nil
	}
	awsModelName = utils.ConvertModelID2CrossRegionProfile(gmw.Ctx(c), awsModelName, awsCli.Options().Region)

	llamaReq, ok := c.Get(ctxkey.ConvertedRequest)
	if !ok {
		return utils.WrapErr(errors.New("request not found")), nil
	}

	// Convert Llama request to Converse API format
	converseReq, err := convertLlamaToConverseRequest(c, llamaReq.(*Request), awsModelName)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "convert to converse request")), nil
	}

	// Use Converse API to get actual token counts
	awsResp, err := awsCli.Converse(gmw.Ctx(c), converseReq)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "Converse")), nil
	}

	// Convert Converse response to OpenAI format
	openaiResp := convertConverseResponseToOpenAI(c, awsResp, modelName)

	// Convert usage to relaymodel.Usage for billing
	var usage relaymodel.Usage
	if awsResp.Usage != nil {
		if awsResp.Usage.InputTokens != nil {
			usage.PromptTokens = int(*awsResp.Usage.InputTokens)
		}
		if awsResp.Usage.OutputTokens != nil {
			usage.CompletionTokens = int(*awsResp.Usage.OutputTokens)
		}
		if awsResp.Usage.TotalTokens != nil {
			usage.TotalTokens = int(*awsResp.Usage.TotalTokens)
		} else {
			// Calculate total if not provided
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		}
	}

	c.JSON(http.StatusOK, openaiResp)
	return nil, &usage
}

// StreamHandler handles streaming requests using Converse API
func StreamHandler(c *gin.Context, awsCli *bedrockruntime.Client) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	lg := gmw.GetLogger(c)
	createdTime := helper.GetTimestamp()
	awsModelName, err := awsModelID(c.GetString(ctxkey.RequestModel))
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "awsModelID")), nil
	}
	awsModelName = utils.ConvertModelID2CrossRegionProfile(gmw.Ctx(c), awsModelName, awsCli.Options().Region)

	llamaReq, ok := c.Get(ctxkey.ConvertedRequest)
	if !ok {
		return utils.WrapErr(errors.New("request not found")), nil
	}

	// Convert Llama request to Converse API format
	converseReq, err := convertLlamaToConverseStreamRequest(c, llamaReq.(*Request), awsModelName)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "convert to converse request")), nil
	}

	awsResp, err := awsCli.ConverseStream(gmw.Ctx(c), converseReq)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "ConverseStream")), nil
	}
	stream := awsResp.GetStream()
	defer stream.Close()

	// Set response headers for SSE
	common.SetEventStreamHeaders(c)

	var usage relaymodel.Usage
	var id string
	finalizer := streamfinalizer.NewFinalizer(
		c.GetString(ctxkey.RequestModel),
		createdTime,
		&usage,
		lg,
		func(payload []byte) bool {
			c.Render(-1, common.CustomEvent{Data: "data: " + string(payload)})
			return true
		},
	)

	c.Stream(func(w io.Writer) bool {
		event, ok := <-stream.Events()
		if !ok {
			if !finalizer.FinalizeOnClose() {
				return false
			}
			c.Render(-1, common.CustomEvent{Data: "data: [DONE]"})
			return false
		}

		switch v := event.(type) {
		case *types.ConverseStreamOutputMemberMessageStart:
			// Handle message start
			id = tracing.GenerateChatCompletionIDFromContext(c)
			finalizer.SetID(id)
			return true

		case *types.ConverseStreamOutputMemberContentBlockDelta:
			// Handle content delta - this is where the actual text content comes from ConverseStream
			if v.Value.Delta != nil {
				var response *openai.ChatCompletionsStreamResponse

				// Check if this is a text delta (AWS SDK union type pattern)
				switch deltaValue := v.Value.Delta.(type) {
				case *types.ContentBlockDeltaMemberText:
					if textDelta := deltaValue.Value; textDelta != "" {
						// Create OpenAI-compatible streaming response with simple string content
						response = &openai.ChatCompletionsStreamResponse{
							Id:      id,
							Object:  "chat.completion.chunk",
							Created: createdTime,
							Model:   c.GetString(ctxkey.RequestModel),
							Choices: []openai.ChatCompletionsStreamResponseChoice{
								{
									Index: 0,
									Delta: relaymodel.Message{
										Role:    "assistant",
										Content: textDelta,
									},
								},
							},
						}
					}
				}

				// Send the response if we have one
				if response != nil {
					jsonStr, err := json.Marshal(response)
					if err != nil {
						lg.Error("error marshalling stream response", zap.Error(err))
						return true
					}
					c.Render(-1, common.CustomEvent{Data: "data: " + string(jsonStr)})
				}
			}
			return true

		case *types.ConverseStreamOutputMemberMessageStop:
			return finalizer.RecordStop(convertStopReason(string(v.Value.StopReason)))

		case *types.ConverseStreamOutputMemberMetadata:
			return finalizer.RecordMetadata(v.Value.Usage)

		default:
			// Handle other event types
			return true
		}
	})

	return nil, &usage
}
