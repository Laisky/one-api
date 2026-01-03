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
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
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

// AwsModelIDMap provides mapping for DeepSeek models via Converse API
// https://docs.aws.amazon.com/bedrock/latest/userguide/model-ids.html
var AwsModelIDMap = map[string]string{
	"deepseek-r1":   "deepseek.r1-v1:0",
	"deepseek-v3.1": "deepseek.v3-v1:0",
}

func awsModelID(requestModel string) (string, error) {
	if awsModelID, ok := AwsModelIDMap[requestModel]; ok {
		return awsModelID, nil
	}
	return "", errors.Errorf("model %s not found", requestModel)
}

// ConvertMessages converts OpenAI messages to DeepSeek messages
func ConvertMessages(messages []relaymodel.Message) []Message {
	var deepseekMessages []Message

	for _, msg := range messages {
		deepseekMsg := Message{
			Role:    msg.Role,
			Content: msg.StringContent(),
		}
		deepseekMessages = append(deepseekMessages, deepseekMsg)
	}

	return deepseekMessages
}

// ConvertRequest converts OpenAI request to DeepSeek request
func ConvertRequest(textRequest relaymodel.GeneralOpenAIRequest) *Request {
	deepseekReq := &Request{
		Messages: ConvertMessages(textRequest.Messages),
	}

	// Handle inference parameters
	if textRequest.MaxTokens == 0 {
		deepseekReq.MaxTokens = config.DefaultMaxToken
	} else {
		deepseekReq.MaxTokens = textRequest.MaxTokens
	}

	if textRequest.Temperature != nil {
		deepseekReq.Temperature = textRequest.Temperature
	}

	if textRequest.TopP != nil {
		deepseekReq.TopP = textRequest.TopP
	}

	if textRequest.Stop != nil {
		if stopSlice, ok := textRequest.Stop.([]any); ok {
			stopSequences := make([]string, len(stopSlice))
			for i, stop := range stopSlice {
				if stopStr, ok := stop.(string); ok {
					stopSequences[i] = stopStr
				}
			}
			deepseekReq.Stop = stopSequences
		} else if stopStr, ok := textRequest.Stop.(string); ok {
			deepseekReq.Stop = []string{stopStr}
		} else if stopSlice, ok := textRequest.Stop.([]string); ok {
			deepseekReq.Stop = stopSlice
		}
	}

	// Handle reasoning_effort parameter for DeepSeek models
	if textRequest.ReasoningEffort != nil {
		deepseekReq.ReasoningEffort = textRequest.ReasoningEffort
	}

	return deepseekReq
}

// Handler handles non-streaming requests using Converse API
func Handler(c *gin.Context, awsCli *bedrockruntime.Client, modelName string) (*relaymodel.ErrorWithStatusCode, *relaymodel.Usage) {
	awsModelName, err := awsModelID(c.GetString(ctxkey.RequestModel))
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "awsModelID")), nil
	}
	awsModelName = utils.ConvertModelID2CrossRegionProfile(gmw.Ctx(c), awsModelName, awsCli.Options().Region)

	deepseekReq, ok := c.Get(ctxkey.ConvertedRequest)
	if !ok {
		return utils.WrapErr(errors.New("request not found")), nil
	}

	// Convert DeepSeek request to Converse API format
	req, ok := deepseekReq.(*Request)
	if !ok {
		return utils.WrapErr(errors.New("invalid converted request type")), nil
	}
	if len(req.Messages) == 0 {
		return utils.WrapErr(errors.New("empty messages")), nil
	}
	converseReq, err := convertDeepSeekToConverseRequest(req, awsModelName)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "convert to converse request")), nil
	}

	// Use Converse API to get actual token counts
	awsResp, err := awsCli.Converse(gmw.Ctx(c), converseReq)
	if err != nil {
		return utils.WrapErr(errors.Wrap(err, "Converse")), nil
	}

	// Convert Converse response to custom DeepSeek format
	deepseekResp := convertConverseResponseToDeepSeek(c, awsResp, modelName)

	// Convert DeepSeek usage to relaymodel.Usage for billing
	var usage relaymodel.Usage
	usage.PromptTokens = deepseekResp.Usage.InputTokens
	usage.CompletionTokens = deepseekResp.Usage.OutputTokens
	usage.TotalTokens = deepseekResp.Usage.TotalTokens

	c.JSON(http.StatusOK, deepseekResp)
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

	deepseekReq, ok := c.Get(ctxkey.ConvertedRequest)
	if !ok {
		return utils.WrapErr(errors.New("request not found")), nil
	}

	// guard against invalid type and empty messages to avoid panics in the streaming path
	req, ok := deepseekReq.(*Request)
	if !ok {
		return utils.WrapErr(errors.New("invalid converted request type")), nil
	}
	if len(req.Messages) == 0 {
		return utils.WrapErr(errors.New("empty messages")), nil
	}

	// Convert DeepSeek request to Converse API format
	converseReq, err := convertDeepSeekToConverseStreamRequest(req, awsModelName)
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

			// Note: This reasoning and content are streamed together,
			// so ensure that client-side implementations handle them correctly,
			// especially in chatbots.
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
					// Handle reasoning content delta - now unified within ContentBlockDelta
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

// convertStopReason converts DeepSeek stop reason to OpenAI format
func convertStopReason(deepseekReason string) *string {
	if deepseekReason == "" {
		return nil
	}

	var result string
	switch deepseekReason {
	case "stop", "end_turn":
		result = "stop"
	case "length", "max_tokens":
		result = "length"
	default:
		result = "stop"
	}
	return &result
}

// convertDeepSeekToConverseRequest converts DeepSeek request to Converse API format for non-streaming
func convertDeepSeekToConverseRequest(deepseekReq *Request, modelID string) (*bedrockruntime.ConverseInput, error) {
	var converseMessages []types.Message
	var systemMessages []types.SystemContentBlock

	// Convert messages using standard Converse API format
	for _, msg := range deepseekReq.Messages {
		switch msg.Role {
		case "system":
			// System messages go to the system field in Converse API
			systemMessages = append(systemMessages, &types.SystemContentBlockMemberText{
				Value: msg.Content,
			})
		case "user":
			// User messages use standard Converse API format (no special formatting needed)
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
	if deepseekReq.MaxTokens != 0 {
		inferenceConfig.MaxTokens = aws.Int32(int32(deepseekReq.MaxTokens))
	} else {
		inferenceConfig.MaxTokens = aws.Int32(int32(config.DefaultMaxToken))
	}

	if deepseekReq.Temperature != nil {
		inferenceConfig.Temperature = aws.Float32(float32(*deepseekReq.Temperature))
	}
	if deepseekReq.TopP != nil {
		inferenceConfig.TopP = aws.Float32(float32(*deepseekReq.TopP))
	}
	if len(deepseekReq.Stop) > 0 {
		stopSequences := make([]string, len(deepseekReq.Stop))
		copy(stopSequences, deepseekReq.Stop)
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

	// Add additional model request fields for reasoning_effort
	if deepseekReq.ReasoningEffort != nil {
		// Convert reasoning_effort to AWS Bedrock's additional-model-request-fields
		// with reasoning_config based on the value of reasoning_effort (low, medium, high)
		//
		// Note: The current known reasoning_config in DeepSeek V3 is associated with the reasoning_effort.
		// When set to "high", it displays the reasoning content.
		// This implementation supports the reasoning_effort parameter and converts it into the reasoning_config design
		// for non-explicit cases (e.g., setting hardcoded it to "high").
		reasoningConfig := map[string]any{
			"reasoning_config": *deepseekReq.ReasoningEffort,
		}
		// Convert to document.Interface using bedrockruntime document package
		docInput := document.NewLazyDocument(reasoningConfig)
		converseReq.AdditionalModelRequestFields = docInput
	}

	return converseReq, nil
}

// convertConverseResponseToDeepSeek converts AWS Converse response to AWS Bedrock format
func convertConverseResponseToDeepSeek(c *gin.Context, converseResp *bedrockruntime.ConverseOutput, modelName string) *DeepSeekBedrockResponse {
	var contentBlocks []DeepSeekBedrockContentBlock
	var finishReason string

	// Extract response content from Converse API response
	if converseResp.Output != nil {
		switch outputValue := converseResp.Output.(type) {
		case *types.ConverseOutputMemberMessage:
			if len(outputValue.Value.Content) > 0 {
				// Process content blocks, keeping text and reasoning content separate in content array
				for _, contentBlock := range outputValue.Value.Content {
					switch contentValue := contentBlock.(type) {
					case *types.ContentBlockMemberText:
						// Add text content block
						if contentValue.Value != "" {
							textValue := contentValue.Value
							contentBlocks = append(contentBlocks, DeepSeekBedrockContentBlock{
								Text: &textValue,
							})
						}
					case *types.ContentBlockMemberReasoningContent:
						// Handle reasoning content blocks as separate content block
						if contentValue.Value != nil {
							switch reasoningBlock := contentValue.Value.(type) {
							case *types.ReasoningContentBlockMemberReasoningText:
								if reasoningBlock.Value.Text != nil && *reasoningBlock.Value.Text != "" {
									contentBlocks = append(contentBlocks, DeepSeekBedrockContentBlock{
										ReasoningContent: &DeepSeekReasoningContent{
											ReasoningText: *reasoningBlock.Value.Text,
										},
									})
								}
							}
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
	message := DeepSeekBedrockMessage{
		Role:    "assistant",
		Content: contentBlocks,
	}

	// Create AWS Bedrock format choice
	choice := DeepSeekBedrockChoice{
		Index:        0,
		Message:      message,
		FinishReason: finishReason,
	}

	// Convert usage to DeepSeek format
	var usage DeepSeekUsage
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

	return &DeepSeekBedrockResponse{
		ID:      tracing.GenerateChatCompletionIDFromContext(c),
		Object:  "chat.completion",
		Created: helper.GetTimestamp(),
		Model:   modelName,
		Choices: []DeepSeekBedrockChoice{choice},
		Usage:   usage,
	}
}

// convertDeepSeekToConverseStreamRequest converts DeepSeek request to Converse API format for streaming
func convertDeepSeekToConverseStreamRequest(deepseekReq *Request, modelID string) (*bedrockruntime.ConverseStreamInput, error) {
	var converseMessages []types.Message
	var systemMessages []types.SystemContentBlock

	// Convert messages using standard Converse API format
	for _, msg := range deepseekReq.Messages {
		switch msg.Role {
		case "system":
			// System messages go to the system field in Converse API
			systemMessages = append(systemMessages, &types.SystemContentBlockMemberText{
				Value: msg.Content,
			})
		case "user":
			// User messages use standard Converse API format (no special formatting needed)
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
	if deepseekReq.MaxTokens != 0 {
		inferenceConfig.MaxTokens = aws.Int32(int32(deepseekReq.MaxTokens))
	} else {
		inferenceConfig.MaxTokens = aws.Int32(int32(config.DefaultMaxToken))
	}

	if deepseekReq.Temperature != nil {
		inferenceConfig.Temperature = aws.Float32(float32(*deepseekReq.Temperature))
	}
	if deepseekReq.TopP != nil {
		inferenceConfig.TopP = aws.Float32(float32(*deepseekReq.TopP))
	}
	if len(deepseekReq.Stop) > 0 {
		stopSequences := make([]string, len(deepseekReq.Stop))
		copy(stopSequences, deepseekReq.Stop)
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

	// Add additional model request fields for reasoning_effort
	if deepseekReq.ReasoningEffort != nil {
		// Convert reasoning_effort to AWS Bedrock's additional-model-request-fields
		// with reasoning_config based on the value of reasoning_effort (low, medium, high)
		//
		// Note: The current known reasoning_config in DeepSeek V3 is associated with the reasoning_effort.
		// When set to "high", it displays the reasoning content.
		// This implementation supports the reasoning_effort parameter and converts it into the reasoning_config design
		// for non-explicit cases (e.g., setting hardcoded it to "high").
		reasoningConfig := map[string]any{
			"reasoning_config": *deepseekReq.ReasoningEffort,
		}
		// Convert to document.Interface using bedrockruntime document package
		docInput := document.NewLazyDocument(reasoningConfig)
		converseReq.AdditionalModelRequestFields = docInput
	}

	return converseReq, nil
}
