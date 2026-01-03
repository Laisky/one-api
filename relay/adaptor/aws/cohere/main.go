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

// ConvertRequest converts OpenAI request to Cohere request with tool support
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
		if stopSlice, ok := textRequest.Stop.([]any); ok {
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

	// Handle tools conversion
	if len(textRequest.Tools) > 0 {
		cohereTools := make([]CohereTool, 0, len(textRequest.Tools))
		for _, tool := range textRequest.Tools {
			if tool.Function == nil {
				continue
			}

			cohereTools = append(cohereTools, CohereTool{
				Type: "function",
				Function: CohereToolSpec{
					Name:        tool.Function.Name,
					Description: tool.Function.Description,
					Parameters:  tool.Function.Parameters,
				},
			})
		}
		cohereReq.Tools = cohereTools

		// Handle tool choice
		if textRequest.ToolChoice != nil {
			cohereReq.ToolChoice = textRequest.ToolChoice
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
	awsModelName = utils.ConvertModelID2CrossRegionProfile(gmw.Ctx(c), awsModelName, awsCli.Options().Region)

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

	// Extract usage directly from response (already relaymodel.Usage)
	usage := cohereResp.Usage

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
	awsModelName = utils.ConvertModelID2CrossRegionProfile(gmw.Ctx(c), awsModelName, awsCli.Options().Region)

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
		// Return the actual AWS response instead of hardcoded "stop"
		result = awsReason
	}

	return &result
}

// convertMessages converts Cohere messages to AWS Converse format
func convertMessages(messages []Message) ([]types.Message, []types.SystemContentBlock, error) {
	var converseMessages []types.Message
	var systemMessages []types.SystemContentBlock

	// Convert messages using standard Converse API format
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			// System messages go to the system field in Converse API
			systemMessages = append(systemMessages, &types.SystemContentBlockMemberText{
				Value: msg.Content,
			})
		case "user":
			// User messages use standard Converse API format
			var contentBlocks []types.ContentBlock

			// Handle tool results for user messages (tool role messages)
			if msg.ToolCallID != "" {
				// This is a tool result message - ONLY add tool result block
				toolResult := &types.ContentBlockMemberToolResult{
					Value: types.ToolResultBlock{
						ToolUseId: &msg.ToolCallID,
						Content: []types.ToolResultContentBlock{
							&types.ToolResultContentBlockMemberText{
								Value: msg.Content,
							},
						},
						Status: types.ToolResultStatusSuccess,
					},
				}
				contentBlocks = append(contentBlocks, toolResult)
			} else if msg.Content != "" {
				// Regular user message - add text content only
				contentBlocks = append(contentBlocks, &types.ContentBlockMemberText{
					Value: msg.Content,
				})
			}

			if len(contentBlocks) > 0 {
				converseMessages = append(converseMessages, types.Message{
					Role:    types.ConversationRole("user"),
					Content: contentBlocks,
				})
			}
		case "assistant":
			var contentBlocks []types.ContentBlock

			// Add text content if present
			if msg.Content != "" {
				contentBlocks = append(contentBlocks, &types.ContentBlockMemberText{
					Value: msg.Content,
				})
			}

			// Handle tool calls from assistant messages
			for _, toolCall := range msg.ToolCalls {
				// Parse tool call arguments
				var inputData map[string]any
				if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &inputData); err != nil {
					return nil, nil, errors.Wrapf(err, "unmarshal tool call arguments for tool %s", toolCall.Function.Name)
				}

				// Convert to document.Interface using bedrockruntime document package
				docInput := document.NewLazyDocument(inputData)

				toolUse := &types.ContentBlockMemberToolUse{
					Value: types.ToolUseBlock{
						ToolUseId: &toolCall.ID,
						Name:      &toolCall.Function.Name,
						Input:     docInput,
					},
				}
				contentBlocks = append(contentBlocks, toolUse)
			}

			if len(contentBlocks) > 0 {
				converseMessages = append(converseMessages, types.Message{
					Role:    types.ConversationRole("assistant"),
					Content: contentBlocks,
				})
			}
		}
	}

	return converseMessages, systemMessages, nil
}

// createInferenceConfig creates AWS inference configuration from Cohere request
func createInferenceConfig(cohereReq *Request) *types.InferenceConfiguration {
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

	return inferenceConfig
}

// createToolConfig creates AWS tool configuration from Cohere tools
func createToolConfig(cohereTools []CohereTool, toolChoice any) *types.ToolConfiguration {
	if len(cohereTools) == 0 {
		return nil // Return nil when no tools - this optimizes for normal Converse API
	}

	var awsTools []types.Tool
	for _, tool := range cohereTools {
		// Convert tool parameters to document.Interface for InputSchema
		var inputSchemaDoc document.Interface
		if tool.Function.Parameters != nil {
			inputSchemaDoc = document.NewLazyDocument(tool.Function.Parameters)
		}

		toolSpec := &types.ToolMemberToolSpec{
			Value: types.ToolSpecification{
				Name:        aws.String(tool.Function.Name),
				Description: aws.String(tool.Function.Description),
				InputSchema: &types.ToolInputSchemaMemberJson{
					Value: inputSchemaDoc,
				},
			},
		}
		awsTools = append(awsTools, toolSpec)
	}

	toolConfig := &types.ToolConfiguration{
		Tools: awsTools,
	}

	// Handle tool choice
	if toolChoice != nil {
		if toolChoiceMap, ok := toolChoice.(map[string]any); ok {
			if funcMap, ok := toolChoiceMap["function"].(map[string]any); ok {
				if funcName, ok := funcMap["name"].(string); ok {
					toolConfig.ToolChoice = &types.ToolChoiceMemberTool{
						Value: types.SpecificToolChoice{
							Name: aws.String(funcName),
						},
					}
				}
			}
		} else if toolChoiceStr, ok := toolChoice.(string); ok {
			switch toolChoiceStr {
			case "auto":
				toolConfig.ToolChoice = &types.ToolChoiceMemberAuto{}
			case "any":
				toolConfig.ToolChoice = &types.ToolChoiceMemberAny{}
			}
		}
	} else {
		// Default to auto if not specified
		toolConfig.ToolChoice = &types.ToolChoiceMemberAuto{}
	}

	return toolConfig
}

// convertCohereToConverseRequest converts Cohere request to Converse API format for non-streaming
func convertCohereToConverseRequest(cohereReq *Request, modelID string) (*bedrockruntime.ConverseInput, error) {
	// Convert messages using shared helper
	converseMessages, systemMessages, err := convertMessages(cohereReq.Messages)
	if err != nil {
		return nil, errors.Wrap(err, "convert messages for Cohere request")
	}

	// Create inference configuration using shared helper
	inferenceConfig := createInferenceConfig(cohereReq)

	converseReq := &bedrockruntime.ConverseInput{
		ModelId:         aws.String(modelID),
		Messages:        converseMessages,
		InferenceConfig: inferenceConfig,
	}

	// Add system messages if any
	if len(systemMessages) > 0 {
		converseReq.System = systemMessages
	}

	// Add tool configuration only if tools are present (optimizes for normal Converse)
	if toolConfig := createToolConfig(cohereReq.Tools, cohereReq.ToolChoice); toolConfig != nil {
		converseReq.ToolConfig = toolConfig
	}

	return converseReq, nil
}

// convertConverseResponseToCohere converts AWS Converse response to OpenAI-compatible format
func convertConverseResponseToCohere(c *gin.Context, converseResp *bedrockruntime.ConverseOutput, modelName string) *CohereResponse {
	var content string
	var toolCalls []CohereToolCallResponse
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
							content += contentValue.Value
						}
					case *types.ContentBlockMemberToolUse:
						// Handle tool use content blocks - convert to OpenAI tool call format
						toolUse := contentValue.Value
						if toolUse.ToolUseId != nil && toolUse.Name != nil {
							// Convert document.Interface input back to JSON string
							var inputJSON string
							if toolUse.Input != nil {
								if inputBytes, err := json.Marshal(toolUse.Input); err == nil {
									inputJSON = string(inputBytes)
								}
							}

							// Create proper OpenAI tool call
							toolCall := CohereToolCallResponse{
								ID:   *toolUse.ToolUseId,
								Type: "function",
								Function: CohereToolFunction{
									Name:      *toolUse.Name,
									Arguments: inputJSON,
								},
							}
							toolCalls = append(toolCalls, toolCall)
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

	// Create OpenAI-compatible message with proper tool_calls support
	message := CohereResponseMessage{
		Role:      "assistant",
		Content:   content,
		ToolCalls: toolCalls,
	}

	// Create OpenAI-compatible choice
	choice := CohereResponseChoice{
		Index:        0,
		Message:      message,
		FinishReason: finishReason,
	}

	// Map usage to project-unified Usage (OpenAI-compatible fields)
	var usage relaymodel.Usage
	if converseResp.Usage != nil {
		if converseResp.Usage.InputTokens != nil {
			usage.PromptTokens = int(*converseResp.Usage.InputTokens)
		}
		if converseResp.Usage.OutputTokens != nil {
			usage.CompletionTokens = int(*converseResp.Usage.OutputTokens)
		}
		if converseResp.Usage.TotalTokens != nil {
			usage.TotalTokens = int(*converseResp.Usage.TotalTokens)
		} else {
			// Calculate total if not provided
			usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		}
	}

	return &CohereResponse{
		ID:      tracing.GenerateChatCompletionID(c),
		Object:  "chat.completion",
		Created: helper.GetTimestamp(),
		Model:   modelName,
		Choices: []CohereResponseChoice{choice},
		Usage:   usage,
	}
}

// getTraceIDSafe retrieves trace ID and never panics even if gin-middlewares tracing is not initialized.
func getTraceIDSafe(c *gin.Context) (traceID string) {
	defer func() {
		if r := recover(); r != nil {
			traceID = ""
		}
	}()
	return tracing.GetTraceIDFromContext(c)
}

// convertCohereToConverseStreamRequest converts Cohere request to Converse API format for streaming
func convertCohereToConverseStreamRequest(cohereReq *Request, modelID string) (*bedrockruntime.ConverseStreamInput, error) {
	// Convert messages using shared helper
	converseMessages, systemMessages, err := convertMessages(cohereReq.Messages)
	if err != nil {
		return nil, errors.Wrap(err, "convert messages for Cohere stream request")
	}

	// Create inference configuration using shared helper
	inferenceConfig := createInferenceConfig(cohereReq)

	converseReq := &bedrockruntime.ConverseStreamInput{
		ModelId:         aws.String(modelID),
		Messages:        converseMessages,
		InferenceConfig: inferenceConfig,
	}

	// Add system messages if any
	if len(systemMessages) > 0 {
		converseReq.System = systemMessages
	}

	// Add tool configuration only if tools are present (optimizes for normal Converse)
	if toolConfig := createToolConfig(cohereReq.Tools, cohereReq.ToolChoice); toolConfig != nil {
		converseReq.ToolConfig = toolConfig
	}

	return converseReq, nil
}

// ConvertMessages converts relay model messages to Cohere messages with tool support
func ConvertMessages(messages []relaymodel.Message) []Message {
	cohereMessages := make([]Message, 0, len(messages))

	for _, message := range messages {
		cohereMessage := Message{
			Role:    message.Role,
			Content: message.StringContent(),
		}

		// Handle tool calls in assistant messages
		if len(message.ToolCalls) > 0 {
			toolCalls := make([]CohereToolCall, 0, len(message.ToolCalls))
			for _, toolCall := range message.ToolCalls {
				arguments := ""
				if toolCall.Function.Arguments != nil {
					if argStr, ok := toolCall.Function.Arguments.(string); ok {
						arguments = argStr
					}
				}

				toolCalls = append(toolCalls, CohereToolCall{
					ID:   toolCall.Id,
					Type: "function",
					Function: CohereToolFunction{
						Name:      toolCall.Function.Name,
						Arguments: arguments,
					},
				})
			}
			cohereMessage.ToolCalls = toolCalls
		}

		// Handle tool results (tool role messages) - convert to user messages for AWS Bedrock compatibility
		if message.Role == "tool" {
			cohereMessage.Role = "user" // AWS Bedrock requires tool results to be user messages
			cohereMessage.ToolCallID = message.ToolCallId
		}

		cohereMessages = append(cohereMessages, cohereMessage)
	}

	return cohereMessages
}
