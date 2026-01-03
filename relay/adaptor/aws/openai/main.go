package aws

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/tracing"
	"github.com/songquanpeng/one-api/relay/adaptor/aws/internal/streamfinalizer"
	"github.com/songquanpeng/one-api/relay/adaptor/aws/utils"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// AwsModelIDMap provides mapping for OpenAI OSS models via Converse API
// https://docs.aws.amazon.com/bedrock/latest/userguide/model-ids.html
var AwsModelIDMap = map[string]string{
	"gpt-oss-20b":  "openai.gpt-oss-20b-1:0",
	"gpt-oss-120b": "openai.gpt-oss-120b-1:0",
}

func awsModelID(requestModel string) (string, error) {
	if awsModelID, ok := AwsModelIDMap[requestModel]; ok {
		return awsModelID, nil
	}
	return "", errors.Errorf("model %s not found", requestModel)
}

// ConvertMessages converts OpenAI messages to OpenAI OSS messages
func ConvertMessages(messages []relaymodel.Message) []Message {
	var openaiMessages []Message

	for _, msg := range messages {
		openaiMsg := Message{
			Role:    msg.Role,
			Content: msg.StringContent(),
		}
		openaiMessages = append(openaiMessages, openaiMsg)
	}

	return openaiMessages
}

// ConvertRequest converts OpenAI request to OpenAI OSS request
func ConvertRequest(textRequest relaymodel.GeneralOpenAIRequest) *Request {
	openaiReq := &Request{
		Messages: ConvertMessages(textRequest.Messages),
	}

	// Handle inference parameters
	if textRequest.MaxTokens == 0 {
		openaiReq.MaxTokens = config.DefaultMaxToken
	} else {
		openaiReq.MaxTokens = textRequest.MaxTokens
	}

	if textRequest.Temperature != nil {
		openaiReq.Temperature = textRequest.Temperature
	}

	if textRequest.TopP != nil {
		openaiReq.TopP = textRequest.TopP
	}

	if textRequest.Stop != nil {
		if stopSlice, ok := textRequest.Stop.([]any); ok {
			stopSequences := make([]string, len(stopSlice))
			for i, stop := range stopSlice {
				if stopStr, ok := stop.(string); ok {
					stopSequences[i] = stopStr
				}
			}
			openaiReq.Stop = stopSequences
		} else if stopStr, ok := textRequest.Stop.(string); ok {
			openaiReq.Stop = []string{stopStr}
		} else if stopSlice, ok := textRequest.Stop.([]string); ok {
			openaiReq.Stop = stopSlice
		}
	}

	return openaiReq
}

// Handler handles non-streaming requests using Converse API
func Handler(c *gin.Context, awsCli *bedrockruntime.Client, modelName string) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	awsModelName, err := awsModelID(c.GetString(ctxkey.RequestModel))
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "awsModelID")), nil
	}
	awsModelName = utils.ConvertModelID2CrossRegionProfile(gmw.Ctx(c), awsModelName, awsCli.Options().Region)

	openaiReq, ok := c.Get(ctxkey.ConvertedRequest)
	if !ok {
		return utils.WrapErr(errors.New("request not found")), nil
	}

	// Convert OpenAI OSS request to Converse API format
	req, ok := openaiReq.(*Request)
	if !ok {
		return utils.WrapErr(errors.New("invalid converted request type")), nil
	}
	if len(req.Messages) == 0 {
		return utils.WrapErr(errors.New("empty messages")), nil
	}
	converseReq, err := convertOpenAIToConverseRequest(req, awsModelName)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "convert to converse request")), nil
	}

	// Use Converse API to get actual token counts
	awsResp, err := awsCli.Converse(gmw.Ctx(c), converseReq)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "Converse")), nil
	}

	// Convert Converse response to custom OpenAI OSS format
	openaiResp := convertConverseResponseToOpenAI(c, awsResp, modelName)

	// Convert OpenAI OSS usage to relaymodel.Usage for billing
	var usage relaymodel.Usage
	usage.PromptTokens = openaiResp.Usage.InputTokens
	usage.CompletionTokens = openaiResp.Usage.OutputTokens
	usage.TotalTokens = openaiResp.Usage.TotalTokens

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

	openaiReq, ok := c.Get(ctxkey.ConvertedRequest)
	if !ok {
		return utils.WrapErr(errors.New("request not found")), nil
	}

	req, ok := openaiReq.(*Request)
	if !ok {
		return utils.WrapErr(errors.New("invalid converted request type")), nil
	}
	if len(req.Messages) == 0 {
		return utils.WrapErr(errors.New("empty messages")), nil
	}

	converseReq, err := convertOpenAIToConverseStreamRequest(req, awsModelName)
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
				// Create a unified response structure to handle both text and reasoning content
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
				case *types.ContentBlockDeltaMemberReasoningContent:
					// Handle reasoning content delta - OpenAI OSS models support reasoning like DeepSeek
					if deltaValue.Value != nil {
						switch reasoningDelta := deltaValue.Value.(type) {
						case *types.ReasoningContentBlockDeltaMemberText:
							if reasoningText := reasoningDelta.Value; reasoningText != "" {
								// Create OpenAI-compatible streaming response with reasoning content as separate field
								response = &openai.ChatCompletionsStreamResponse{
									Id:      id,
									Object:  "chat.completion.chunk",
									Created: createdTime,
									Model:   c.GetString(ctxkey.RequestModel),
									Choices: []openai.ChatCompletionsStreamResponseChoice{
										{
											Index: 0,
											Delta: relaymodel.Message{
												Role:             "assistant",
												ReasoningContent: &reasoningText,
											},
										},
									},
								}
							}
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

// convertStopReason converts AWS stop reason to OpenAI format
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
		result = awsReason // Return the actual AWS response instead of hardcoded "stop"
	}

	return &result
}

// convertOpenAIToConverseRequest converts OpenAI OSS request to Converse API format for non-streaming
func convertOpenAIToConverseRequest(openaiReq *Request, modelID string) (*bedrockruntime.ConverseInput, error) {
	var converseMessages []types.Message
	var systemMessages []types.SystemContentBlock

	// Convert messages using standard Converse API format
	for _, msg := range openaiReq.Messages {
		switch msg.Role {
		case "system":
			// System messages go to the system field in Converse API
			systemMessages = append(systemMessages, &types.SystemContentBlockMemberText{
				Value: msg.Content,
			})
		case "user":
			// User messages use standard Converse API format
			contentBlocks := []types.ContentBlock{
				&types.ContentBlockMemberText{
					Value: msg.Content,
				},
			}

			converseMessages = append(converseMessages, types.Message{
				Role:    types.ConversationRole(msg.Role),
				Content: contentBlocks,
			})
		case "assistant":
			// Assistant messages don't need special formatting
			contentBlocks := []types.ContentBlock{
				&types.ContentBlockMemberText{
					Value: msg.Content,
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
	if openaiReq.MaxTokens != 0 {
		inferenceConfig.MaxTokens = aws.Int32(int32(openaiReq.MaxTokens))
	} else {
		inferenceConfig.MaxTokens = aws.Int32(int32(config.DefaultMaxToken))
	}

	if openaiReq.Temperature != nil {
		inferenceConfig.Temperature = aws.Float32(float32(*openaiReq.Temperature))
	}
	if openaiReq.TopP != nil {
		inferenceConfig.TopP = aws.Float32(float32(*openaiReq.TopP))
	}
	if len(openaiReq.Stop) > 0 {
		stopSequences := make([]string, len(openaiReq.Stop))
		copy(stopSequences, openaiReq.Stop)
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

// convertOpenAIToConverseStreamRequest converts OpenAI OSS request to ConverseStream API format
func convertOpenAIToConverseStreamRequest(openaiReq *Request, modelID string) (*bedrockruntime.ConverseStreamInput, error) {
	// Convert to regular converse request first
	converseReq, err := convertOpenAIToConverseRequest(openaiReq, modelID)
	if err != nil {
		return nil, errors.Wrap(err, "convert openai request for stream")
	}

	// Create stream request
	streamReq := &bedrockruntime.ConverseStreamInput{
		ModelId:                      converseReq.ModelId,
		Messages:                     converseReq.Messages,
		InferenceConfig:              converseReq.InferenceConfig,
		System:                       converseReq.System,
		ToolConfig:                   converseReq.ToolConfig,
		AdditionalModelRequestFields: converseReq.AdditionalModelRequestFields,
	}

	return streamReq, nil
}

func convertConverseResponseToOpenAI(c *gin.Context, awsResp *bedrockruntime.ConverseOutput, modelName string) *Response {
	// Convert AWS Converse response to OpenAI OSS format
	// Similar to DeepSeek implementation but for OpenAI OSS models

	var content string
	var finishReason string

	// Extract response content from Converse API response
	if awsResp.Output != nil {
		switch outputValue := awsResp.Output.(type) {
		case *types.ConverseOutputMemberMessage:
			if len(outputValue.Value.Content) > 0 {
				// Process content blocks
				for _, contentBlock := range outputValue.Value.Content {
					switch contentValue := contentBlock.(type) {
					case *types.ContentBlockMemberText:
						// Add text content
						if contentValue.Value != "" {
							content += contentValue.Value
						}
					case *types.ContentBlockMemberReasoningContent:
						// Handle reasoning content for OpenAI OSS models
						if contentValue.Value != nil {
							switch reasoningBlock := contentValue.Value.(type) {
							case *types.ReasoningContentBlockMemberReasoningText:
								if reasoningBlock.Value.Text != nil && *reasoningBlock.Value.Text != "" {
									// For OpenAI OSS, we can include reasoning in content or handle separately
									content += *reasoningBlock.Value.Text
								}
							}
						}
					}
				}
			}
		}
	}

	// Convert stop reason
	if stopReason := convertStopReason(string(awsResp.StopReason)); stopReason != nil {
		finishReason = *stopReason
	}

	choice := Choice{
		Index: 0,
		Message: ResponseMessage{
			Role:    "assistant",
			Content: content,
		},
		FinishReason: finishReason,
	}

	// Convert usage information
	var usage Usage
	if awsResp.Usage != nil {
		if awsResp.Usage.InputTokens != nil {
			usage.InputTokens = int(*awsResp.Usage.InputTokens)
		}
		if awsResp.Usage.OutputTokens != nil {
			usage.OutputTokens = int(*awsResp.Usage.OutputTokens)
		}
		if awsResp.Usage.TotalTokens != nil {
			usage.TotalTokens = int(*awsResp.Usage.TotalTokens)
		} else {
			// Calculate total if not provided
			usage.TotalTokens = usage.InputTokens + usage.OutputTokens
		}
	}

	response := &Response{
		ID:      tracing.GenerateChatCompletionIDFromContext(c),
		Object:  "chat.completion",
		Created: helper.GetTimestamp(),
		Model:   modelName,
		Choices: []Choice{choice},
		Usage:   usage,
	}

	return response
}
