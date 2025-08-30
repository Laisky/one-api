package aws

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
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
	"github.com/songquanpeng/one-api/relay/adaptor/aws/utils"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// AwsModelIDMap provides mapping for Cohere Command R models via Converse API
var AwsModelIDMap = map[string]string{
	"command-r":      "cohere.command-r-v1:0",
	"command-r-plus": "cohere.command-r-plus-v1:0",
}

func awsModelID(requestModel string) (string, error) {
	if awsModelID, ok := AwsModelIDMap[requestModel]; ok {
		return awsModelID, nil
	}
	return "", errors.Errorf("model %s not found", requestModel)
}

// ConvertRequest converts OpenAI request to Cohere request
func ConvertRequest(textRequest relaymodel.GeneralOpenAIRequest) *Request {
	cohereReq := &Request{
		Messages: ConvertMessages(textRequest.Messages),
	}

	// Handle inference parameters
	if textRequest.MaxTokens == 0 {
		cohereReq.MaxTokens = config.DefaultMaxToken
	} else {
		cohereReq.MaxTokens = textRequest.MaxTokens
	}

	if textRequest.Temperature != nil {
		cohereReq.Temperature = textRequest.Temperature
	}

	if textRequest.TopP != nil {
		cohereReq.TopP = textRequest.TopP
	}

	if textRequest.Stop != nil {
		if stopSlice, ok := textRequest.Stop.([]interface{}); ok {
			stopSequences := make([]string, len(stopSlice))
			for i, stop := range stopSlice {
				if stopStr, ok := stop.(string); ok {
					stopSequences[i] = stopStr
				}
			}
			cohereReq.Stop = stopSequences
		} else if stopStr, ok := textRequest.Stop.(string); ok {
			cohereReq.Stop = []string{stopStr}
		} else if stopSlice, ok := textRequest.Stop.([]string); ok {
			cohereReq.Stop = stopSlice
		}
	}

	return cohereReq
}

// Handler handles non-streaming requests using Converse API
func Handler(c *gin.Context, awsCli *bedrockruntime.Client, modelName string) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	awsModelName, err := awsModelID(c.GetString(ctxkey.RequestModel))
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "awsModelID")), nil
	}
	awsModelName = utils.ConvertModelID2CrossRegionProfile(awsModelName, awsCli.Options().Region)

	cohereReq, ok := c.Get(ctxkey.ConvertedRequest)
	if !ok {
		return utils.WrapErr(errors.New("request not found")), nil
	}

	// Convert Cohere request to Converse API format
	converseReq, err := convertCohereToConverseRequest(cohereReq.(*Request), awsModelName)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "convert to converse request")), nil
	}

	// Use Converse API to get actual token counts
	awsResp, err := awsCli.Converse(gmw.Ctx(c), converseReq)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "Converse")), nil
	}

	// Convert Converse response to custom Cohere format
	cohereResp := convertConverseResponseToCohere(c, awsResp, modelName)

	// Convert Cohere usage to relaymodel.Usage for billing
	var usage relaymodel.Usage
	usage.PromptTokens = cohereResp.Usage.InputTokens
	usage.CompletionTokens = cohereResp.Usage.OutputTokens
	usage.TotalTokens = cohereResp.Usage.TotalTokens

	c.JSON(http.StatusOK, cohereResp)
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
	awsModelName = utils.ConvertModelID2CrossRegionProfile(awsModelName, awsCli.Options().Region)

	cohereReq, ok := c.Get(ctxkey.ConvertedRequest)
	if !ok {
		return utils.WrapErr(errors.New("request not found")), nil
	}

	// Convert Cohere request to Converse API format
	converseReq, err := convertCohereToConverseStreamRequest(cohereReq.(*Request), awsModelName)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "convert to converse request")), nil
	}

	awsResp, err := awsCli.ConverseStream(gmw.Ctx(c), converseReq)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "ConverseStream")), nil
	}
	stream := awsResp.GetStream()
	defer stream.Close()

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	// This change addresses an issue with nginx that could be annoying regarding buffering.
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Header().Set("Pragma", "no-cache") // This is for legacy HTTP; I'm pretty sure.

	var usage relaymodel.Usage
	var id string

	c.Stream(func(w io.Writer) bool {
		event, ok := <-stream.Events()
		if !ok {
			c.Render(-1, common.CustomEvent{Data: "data: [DONE]"})
			return false
		}

		switch v := event.(type) {
		case *types.ConverseStreamOutputMemberMessageStart:
			// Handle message start
			id = fmt.Sprintf("chatcmpl-oneapi-%s", tracing.GetTraceIDFromContext(c))
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
							Model:   c.GetString(ctxkey.OriginalModel),
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
			// Handle message stop with OpenAI-compatible response structure
			finishReason := convertStopReason(string(v.Value.StopReason))
			response := &openai.ChatCompletionsStreamResponse{
				Id:      id,
				Object:  "chat.completion.chunk",
				Created: createdTime,
				Model:   c.GetString(ctxkey.OriginalModel),
				Choices: []openai.ChatCompletionsStreamResponseChoice{
					{
						Index:        0,
						Delta:        relaymodel.Message{},
						FinishReason: finishReason,
					},
				},
			}

			jsonStr, err := json.Marshal(response)
			if err != nil {
				lg.Error("error marshalling final stream response", zap.Error(err))
				return false
			}
			c.Render(-1, common.CustomEvent{Data: "data: " + string(jsonStr)})
			return true

		case *types.ConverseStreamOutputMemberMetadata:
			// Handle metadata (usage)
			if streamUsage := v.Value.Usage; streamUsage != nil {
				if streamUsage.InputTokens != nil {
					usage.PromptTokens = int(*streamUsage.InputTokens)
				}
				if streamUsage.OutputTokens != nil {
					usage.CompletionTokens = int(*streamUsage.OutputTokens)
				}
				if streamUsage.TotalTokens != nil {
					usage.TotalTokens = int(*streamUsage.TotalTokens)
				}
			}
			return true

		default:
			// Handle other event types
			return true
		}
	})

	return nil, &usage
}

func convertStopReason(cohereReason string) *string {
	if cohereReason == "" {
		return nil
	}

	var result string
	switch cohereReason {
	case "stop", "end_turn":
		result = "stop"
	case "length", "max_tokens":
		result = "length"
	default:
		result = "stop"
	}
	return &result
}

// convertCohereToConverseRequest converts Cohere request to Converse API format for non-streaming
func convertCohereToConverseRequest(cohereReq *Request, modelID string) (*bedrockruntime.ConverseInput, error) {
	var converseMessages []types.Message
	var systemMessages []types.SystemContentBlock

	// Convert messages using standard Converse API format
	for _, msg := range cohereReq.Messages {
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
			// Assistant messages
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
	if cohereReq.MaxTokens != 0 {
		inferenceConfig.MaxTokens = aws.Int32(int32(cohereReq.MaxTokens))
	} else {
		inferenceConfig.MaxTokens = aws.Int32(int32(config.DefaultMaxToken))
	}

	if cohereReq.Temperature != nil {
		inferenceConfig.Temperature = aws.Float32(float32(*cohereReq.Temperature))
	}
	if cohereReq.TopP != nil {
		inferenceConfig.TopP = aws.Float32(float32(*cohereReq.TopP))
	}
	if len(cohereReq.Stop) > 0 {
		stopSequences := make([]string, len(cohereReq.Stop))
		copy(stopSequences, cohereReq.Stop)
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

// convertConverseResponseToCohere converts AWS Converse response to AWS Bedrock format
func convertConverseResponseToCohere(c *gin.Context, converseResp *bedrockruntime.ConverseOutput, modelName string) *CohereBedrockResponse {
	var contentBlocks []CohereBedrockContentBlock
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
						// Add text content block
						if contentValue.Value != "" {
							textValue := contentValue.Value
							contentBlocks = append(contentBlocks, CohereBedrockContentBlock{
								Text: &textValue,
							})
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

	// Create AWS Bedrock format message with content array
	message := CohereBedrockMessage{
		Role:    "assistant",
		Content: contentBlocks,
	}

	// Create AWS Bedrock format choice
	choice := CohereBedrockChoice{
		Index:        0,
		Message:      message,
		FinishReason: finishReason,
	}

	// Convert usage to Cohere format
	var usage CohereUsage
	if converseResp.Usage != nil {
		if converseResp.Usage.InputTokens != nil {
			usage.InputTokens = int(*converseResp.Usage.InputTokens)
		}
		if converseResp.Usage.OutputTokens != nil {
			usage.OutputTokens = int(*converseResp.Usage.OutputTokens)
		}
		if converseResp.Usage.TotalTokens != nil {
			usage.TotalTokens = int(*converseResp.Usage.TotalTokens)
		} else {
			// Calculate total if not provided
			usage.TotalTokens = usage.InputTokens + usage.OutputTokens
		}
	}

	return &CohereBedrockResponse{
		ID:      fmt.Sprintf("chatcmpl-oneapi-%s", tracing.GetTraceIDFromContext(c)),
		Object:  "chat.completion",
		Created: helper.GetTimestamp(),
		Model:   modelName,
		Choices: []CohereBedrockChoice{choice},
		Usage:   usage,
	}
}

// convertCohereToConverseStreamRequest converts Cohere request to Converse API format for streaming
func convertCohereToConverseStreamRequest(cohereReq *Request, modelID string) (*bedrockruntime.ConverseStreamInput, error) {
	var converseMessages []types.Message
	var systemMessages []types.SystemContentBlock

	// Convert messages using standard Converse API format
	for _, msg := range cohereReq.Messages {
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
			// Assistant messages
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
	if cohereReq.MaxTokens != 0 {
		inferenceConfig.MaxTokens = aws.Int32(int32(cohereReq.MaxTokens))
	} else {
		inferenceConfig.MaxTokens = aws.Int32(int32(config.DefaultMaxToken))
	}

	if cohereReq.Temperature != nil {
		inferenceConfig.Temperature = aws.Float32(float32(*cohereReq.Temperature))
	}
	if cohereReq.TopP != nil {
		inferenceConfig.TopP = aws.Float32(float32(*cohereReq.TopP))
	}
	if len(cohereReq.Stop) > 0 {
		stopSequences := make([]string, len(cohereReq.Stop))
		copy(stopSequences, cohereReq.Stop)
		inferenceConfig.StopSequences = stopSequences
	}

	converseReq := &bedrockruntime.ConverseStreamInput{
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

// ConvertMessages converts relay model message to Cohere message
func ConvertMessages(messages []relaymodel.Message) []Message {
	cohereMessages := make([]Message, 0, len(messages))
	for _, msg := range messages {
		cohereMessages = append(cohereMessages, Message{
			Role:    msg.Role,
			Content: msg.StringContent(),
		})
	}
	return cohereMessages
}
